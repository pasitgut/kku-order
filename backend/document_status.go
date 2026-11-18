package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// documentReturned คือ "รอตรวจสอบ": ผู้ใช้กดส่งเอกสารกลับมาแก้ ต่างจาก REVIEW/NEEDS_REVIEW ("รอยืนยัน") ที่ระบบตั้งให้
const documentReturned = "RETURNED"

const (
	autoConfirmSettingKey = "auto_confirm"
	autoConfirmActor      = "ระบบ (อัตโนมัติ)"
	autoConfirmDetails    = "ยืนยันอัตโนมัติ: อ่านข้อมูลคำสั่งได้มั่นใจ และไม่มีแถวโดยตำแหน่งที่ยังว่าง"
)

var errDocumentChanged = errors.New("document changed")
var errAmbiguousConfirmationName = errors.New("confirmation name matches multiple people")

// confirmNow ยืนยันเอกสาร และนับทุกรายชื่อในคำสั่งว่าตรวจแล้ว ใช้ทั้งตอนคนกดและตอนยืนยันอัตโนมัติ
func confirmNow(db *gorm.DB, document Document, confirmedBy, details string) (Document, error) {
	now := time.Now().Truncate(time.Millisecond)
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := lockEditableDocument(tx, document.ID); err != nil {
			return err
		}
		var current Document
		if err := tx.First(&current, document.ID).Error; err != nil {
			return err
		}
		automatic := confirmedBy == autoConfirmActor
		if current.Status == "CONFIRMED" && !automatic {
			document = current
			return nil
		}
		canConfirm := current.Status == "REVIEW" || current.Status == "NEEDS_REVIEW" || (!automatic && current.Status == documentReturned)
		if !canConfirm || !current.UpdatedAt.Equal(document.UpdatedAt) {
			if automatic {
				return errAutoConfirmNotReady
			}
			return errDocumentChanged
		}
		if err := checkConfirmationNames(tx, document.ID); err != nil {
			return err
		}
		if err := tx.Model(&document).Updates(map[string]any{"status": "CONFIRMED", "confirmed_by": confirmedBy, "confirmed_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&Appointment{}).Where("document_id = ?", document.ID).Updates(map[string]any{"verified": true, "verified_by": confirmedBy, "verified_at": now}).Error; err != nil {
			return err
		}
		document.Status = "CONFIRMED"
		document.ConfirmedBy = confirmedBy
		document.ConfirmedAt = &now
		if err := recordDocumentConfirmation(tx, document.ID, confirmedBy, now); err != nil {
			return err
		}

		return tx.Create(&AuditLog{DocumentID: &document.ID, UserID: confirmedBy, Action: "DOCUMENT_CONFIRMED", Details: details}).Error
	})
	return document, err
}

type returnDocumentRequest struct {
	Note string `json:"note"`
}

// returnDocument ส่งเอกสารกลับไป "รอตรวจสอบ" เอกสารที่ยืนยันแล้วจะออกจากรายงานและการแจ้งเตือนจนกว่าจะยืนยันใหม่
func returnDocument(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := documentIDParam(c)
		if !ok {
			return
		}
		var payload returnDocumentRequest
		_ = c.ShouldBindJSON(&payload)
		note := truncateRunes(strings.TrimSpace(payload.Note), 500)
		var document Document
		if err := db.First(&document, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบเอกสาร"})
			return
		}
		switch document.Status {
		case documentCancelled:
			writeCancelledConflict(c, errDocumentCancelled)
			return
		case documentReturned:
			c.JSON(http.StatusOK, gin.H{"message": "เอกสารนี้รอตรวจสอบอยู่แล้ว", "data": document})
			return
		case "REVIEW", "NEEDS_REVIEW", "CONFIRMED":
		default:
			c.JSON(http.StatusConflict, gin.H{"error": "เอกสารยังประมวลผลไม่เสร็จ ส่งกลับไปตรวจสอบไม่ได้", "status": document.Status})
			return
		}
		changedBy := currentUserID(c)
		previous := document.Status
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := lockEditableDocument(tx, document.ID); err != nil {
				return err
			}
			if err := tx.Model(&document).Updates(map[string]any{"status": documentReturned, "confirmed_by": "", "confirmed_at": nil}).Error; err != nil {
				return err
			}
			document.Status = documentReturned
			document.ConfirmedBy = ""
			document.ConfirmedAt = nil

			revision := DocumentRevision{DocumentID: document.ID, FieldKey: "status", OldValue: previous, NewValue: documentReturned, ChangedBy: changedBy, ChangedAt: time.Now()}
			if err := tx.Create(&revision).Error; err != nil {
				return err
			}
			details := "ส่งกลับไปตรวจสอบ"
			if note != "" {
				details += ": " + note
			}
			return tx.Create(&AuditLog{DocumentID: &document.ID, UserID: changedBy, Action: "DOCUMENT_RETURNED", Details: details}).Error
		}); err != nil {
			if writeCancelledConflict(c, err) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถส่งเอกสารกลับไปตรวจสอบได้"})
			return
		}
		if note != "" {
			db.Model(&document).Update("review_notes", note)
		}
		getDocument(db)(c)
	}
}

// readyToAutoConfirm: ยืนยันอัตโนมัติได้เฉพาะเอกสารที่ไม่มีอะไรต้องแก้ ไม่มีแถวโดยตำแหน่งที่ยังว่าง วันที่อ่านออก
// และเลขที่คำสั่ง วันที่ออกคำสั่ง วันที่มีผล อ่านได้มั่นใจถึงเกณฑ์เดียวกับที่ขึ้นป้าย "ควรตรวจ"
func readyToAutoConfirm(db *gorm.DB, documentID uint, threshold float64) (bool, error) {
	// ชื่อที่ผูกไม่ได้ส่วนใหญ่เป็นคนนอกคณะ เก็บไว้ตามเอกสารแล้วผูกเองเมื่อคนนั้นเข้าระบบ จึงไม่นับเป็นงานค้าง
	// ที่ยังต้องรอคนตรวจ: แถวโดยตำแหน่งที่ยังไม่มีผู้ดำรง และกรณียังไม่มีฐานบุคลากรเลย
	var unlinked int64
	if err := db.Model(&Appointment{}).
		Where("document_id = ? AND directory_user_id IS NULL", documentID).
		Where("TRIM(COALESCE(full_name, '')) = '' OR name_match_method IN ?", []string{matchMethodEmpty, matchMethodAmbiguous}).
		Count(&unlinked).Error; err != nil || unlinked > 0 {
		return false, err
	}
	// คำสั่งแต่งตั้งที่ไม่มีรายชื่อเลย หรือชื่อมีแท็บ/ขึ้นบรรทัดใหม่ (OCR ดึงคอลัมน์อื่นติดมา) ต้องให้คนตรวจ
	var names []string
	if err := db.Model(&Appointment{}).Where("document_id = ?", documentID).Pluck("full_name", &names).Error; err != nil || len(names) == 0 {
		return false, err
	}
	for _, name := range names {
		if strings.ContainsAny(name, "\t\r\n") {
			return false, nil
		}
	}
	var fields []ExtractedField
	if err := db.Where("document_id = ? AND field_key IN ?", documentID, []string{"order_no", "issued_date", "effective_date", "expiry_date"}).Find(&fields).Error; err != nil {
		return false, err
	}
	if len(unreadableDates(fields)) > 0 {
		return false, nil
	}
	seen := map[string]bool{}
	for _, field := range fields {
		value := strings.TrimSpace(field.Value)
		if field.FieldKey == "expiry_date" || (field.FieldKey == "effective_date" && value == "") {
			continue
		}
		if value == "" || float64(field.Confidence) < threshold {
			return false, nil
		}
		seen[field.FieldKey] = true
	}
	return seen["order_no"] && seen["issued_date"], nil
}

type autoConfirmSetting struct {
	Enabled bool `json:"enabled"`
}

// autoConfirmEnabled เปิดเป็นค่าเริ่มต้น: ถ้ายังไม่มีใครตั้งค่า เอกสารที่ผ่านเกณฑ์จะถูกยืนยันเองโดยไม่ต้องคลิก
// ADMIN ปิดได้ในหน้าตั้งค่า OCR และการนำเข้า และเมื่อปิดแล้วจะปิดอยู่อย่างนั้น
func autoConfirmEnabled(db *gorm.DB) (bool, error) {
	var row AppSetting
	err := db.Where("setting_key = ?", autoConfirmSettingKey).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var setting autoConfirmSetting
	if err := json.Unmarshal([]byte(row.Value), &setting); err != nil {
		return false, err
	}
	return setting.Enabled, nil
}

// pendingAutoConfirm คืนเอกสาร "รอยืนยัน" ที่ยืนยันอัตโนมัติได้ทันที (ไม่รวมที่ผู้ใช้ส่งกลับมาตรวจสอบ)
func pendingAutoConfirm(db *gorm.DB, threshold float64) ([]Document, error) {
	var pending []Document
	if err := db.Where("status IN ? AND content_state <> ? AND pending_extraction_json_path = ?", []string{"REVIEW", "NEEDS_REVIEW"}, "UNSAVED", "").Find(&pending).Error; err != nil {
		return nil, err
	}
	ready := make([]Document, 0, len(pending))
	for _, document := range pending {
		ok, err := readyToAutoConfirm(db, document.ID, threshold)
		if err != nil {
			return nil, err
		}
		if ok {
			ready = append(ready, document)
		}
	}
	return ready, nil
}

// setAutoConfirm บันทึกสวิตช์ ตอนเปิดจะยืนยันเอกสารที่ค้าง "รอยืนยัน" และผ่านเกณฑ์ให้ทันที แล้วคืนจำนวนที่ยืนยัน
func setAutoConfirm(db *gorm.DB, enabled bool, userID string, threshold float64) (int, error) {
	value, _ := json.Marshal(autoConfirmSetting{Enabled: enabled})
	row := AppSetting{SettingKey: autoConfirmSettingKey, Value: string(value), UpdatedBy: userID, UpdatedAt: time.Now()}
	if err := db.Save(&row).Error; err != nil {
		return 0, err
	}
	if !enabled {
		return 0, nil
	}
	return confirmReadyPending(db, threshold)
}

// autoConfirmPendingIfEnabled ยืนยันเอกสารที่ค้าง "รอยืนยัน" และผ่านเกณฑ์แล้ว เฉพาะเมื่อเปิดสวิตช์ยืนยันอัตโนมัติไว้
// เรียกตอน backend เริ่มทำงาน และหลังผูกชื่อคนนอกย้อนหลัง
func autoConfirmPendingIfEnabled(db *gorm.DB) (int, error) {
	enabled, err := autoConfirmEnabled(db)
	if err != nil || !enabled {
		return 0, err
	}
	return confirmReadyPending(db, reviewThresholdOf(NewSettingsStore(db)))
}

// confirmReadyPending ยืนยันเอกสาร "รอยืนยัน" ทุกฉบับที่ผ่านเกณฑ์ คืนจำนวนที่ยืนยัน
func confirmReadyPending(db *gorm.DB, threshold float64) (int, error) {
	var unsaved []Document
	if err := db.Where("status IN ? AND content_state = ? AND extraction_json_path <> ?", []string{"REVIEW", "NEEDS_REVIEW"}, "UNSAVED", "").Find(&unsaved).Error; err != nil {
		return 0, err
	}
	confirmed := 0
	for _, document := range unsaved {
		ok, err := autoConfirmSnapshot(db, document, threshold)
		if err != nil {
			return confirmed, err
		}
		if ok {
			confirmed++
		}
	}
	ready, err := pendingAutoConfirm(db, threshold)
	if err != nil {
		return 0, err
	}
	for _, document := range ready {
		if _, err := confirmNow(db, document, autoConfirmActor, autoConfirmDetails); err != nil {
			if errors.Is(err, errAutoConfirmNotReady) || errors.Is(err, errAmbiguousConfirmationName) {
				continue
			}
			return confirmed, err
		}
		confirmed++
	}
	return confirmed, nil
}

// autoConfirmAfterExtraction เรียกหลัง OCR อ่านเสร็จ ถ้าเปิดสวิตช์ไว้และเอกสารพร้อม ระบบยืนยันให้เลย
func autoConfirmAfterExtraction(db *gorm.DB, documentID uint, threshold float64) {
	enabled, err := autoConfirmEnabled(db)
	if err != nil || !enabled {
		return
	}
	var document Document
	if err := db.First(&document, documentID).Error; err != nil || (document.Status != "REVIEW" && document.Status != "NEEDS_REVIEW") {
		return
	}
	if document.ContentState == "UNSAVED" {
		_, _ = autoConfirmSnapshot(db, document, threshold)
		return
	}
	if document.PendingExtractionJSONPath != "" {
		return
	}
	if ok, err := readyToAutoConfirm(db, documentID, threshold); err == nil && ok {
		_, _ = confirmNow(db, document, autoConfirmActor, autoConfirmDetails)
	}
}

var errAutoConfirmNotReady = errors.New("auto confirmation not ready")

func autoConfirmSnapshot(db *gorm.DB, document Document, threshold float64) (bool, error) {
	snapshot, err := readExtractionFile(document.ExtractionJSONPath, document.ID)
	if err != nil {
		// ไฟล์ผลสกัดหายหรืออ่านไม่ได้: เอกสารนี้ยังไม่พร้อมยืนยัน แต่ฉบับอื่นในรอบเดียวกันต้องไปต่อได้
		log.Printf("auto-confirm skipped unreadable extraction: document_id=%d: %v", document.ID, err)
		return false, nil
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := lockEditableDocument(tx, document.ID); err != nil {
			return err
		}
		var current Document
		if err := tx.First(&current, document.ID).Error; err != nil {
			return err
		}
		if current.ContentState != "UNSAVED" || current.ExtractionJSONPath != document.ExtractionJSONPath || (current.Status != "REVIEW" && current.Status != "NEEDS_REVIEW") {
			return errAutoConfirmNotReady
		}
		fields := make([]fieldUpdateRequest, len(snapshot.Fields))
		for i, field := range snapshot.Fields {
			fields[i] = fieldUpdateRequest{ID: uint(i + 1), FieldKey: field.FieldKey}
		}
		appointments := make([]appointmentRequest, len(snapshot.Appointments))
		for i := range appointments {
			appointments[i].ID = uint(i + 1)
		}
		if err := materializeExtraction(tx, document.ID, snapshot, &updateDocumentRequest{Fields: &fields, Appointments: &appointments}); err != nil {
			return err
		}
		ready, err := readyToAutoConfirm(tx, document.ID, threshold)
		if err != nil {
			return err
		}
		if !ready {
			return errAutoConfirmNotReady
		}
		updates := buildExtractionRecords(snapshot.Result, threshold).Updates
		delete(updates, "ocr_text")
		delete(updates, "ocr_provider")
		updates["content_state"] = "CONFIRMED"
		if err := tx.Model(&Document{}).Where("id = ?", document.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.First(&current, document.ID).Error; err != nil {
			return err
		}

		_, err = confirmNow(tx, current, autoConfirmActor, autoConfirmDetails)
		return err
	})
	if errors.Is(err, errAutoConfirmNotReady) || errors.Is(err, errAmbiguousConfirmationName) {
		return false, nil
	}
	return err == nil, err
}

func reviewThresholdOf(store SettingsStore) float64 {
	if settings, err := store.LoadOCR(); err == nil && settings.ReviewThreshold > 0 {
		return settings.ReviewThreshold
	}
	return defaultOCRSettings().ReviewThreshold
}

func getAutoConfirmHandler(db *gorm.DB, store SettingsStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		enabled, err := autoConfirmEnabled(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดการตั้งค่าได้"})
			return
		}
		ready, err := pendingAutoConfirm(db, reviewThresholdOf(store))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถนับเอกสารที่พร้อมยืนยันได้"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"enabled": enabled, "readyNow": len(ready)}})
	}
}

func updateAutoConfirmHandler(db *gorm.DB, store SettingsStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload autoConfirmSetting
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
			return
		}
		confirmed, err := setAutoConfirm(db, payload.Enabled, currentUserID(c), reviewThresholdOf(store))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกการตั้งค่าได้"})
			return
		}
		state := "ปิด"
		if payload.Enabled {
			state = "เปิด"
		}
		_ = db.Create(&AuditLog{UserID: currentUserID(c), Action: "AUTO_CONFIRM_CHANGED", Details: state + "การยืนยันเอกสารอัตโนมัติ"}).Error
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"enabled": payload.Enabled, "confirmed": confirmed}})
	}
}
