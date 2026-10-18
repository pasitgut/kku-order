package main

import (
	"encoding/json"
	"errors"
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
)

// confirmNow ยืนยันเอกสาร และนับทุกรายชื่อในคำสั่งว่าตรวจแล้ว ใช้ทั้งตอนคนกดและตอนยืนยันอัตโนมัติ
func confirmNow(db *gorm.DB, document Document, confirmedBy, details string) (Document, error) {
	now := time.Now()
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := lockEditableDocument(tx, document.ID); err != nil {
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
		if err := syncCoreConfirmation(tx, &document); err != nil {
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
		var payload returnDocumentRequest
		_ = c.ShouldBindJSON(&payload)
		note := truncateRunes(strings.TrimSpace(payload.Note), 500)
		var document Document
		if err := db.First(&document, c.Param("id")).Error; err != nil {
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
			if err := syncCoreConfirmation(tx, &document); err != nil {
				return err
			}
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

// readyToAutoConfirm: ยืนยันอัตโนมัติได้เฉพาะเอกสารที่ไม่มีอะไรต้องแก้ ผูกชื่อครบ วันที่อ่านออก
// และเลขที่คำสั่ง วันที่ออกคำสั่ง วันที่มีผล อ่านได้มั่นใจถึงเกณฑ์เดียวกับที่ขึ้นป้าย "ควรตรวจ"
func readyToAutoConfirm(db *gorm.DB, documentID uint, threshold float64) (bool, error) {
	var unlinked int64
	if err := db.Model(&Appointment{}).Where("document_id = ? AND directory_user_id IS NULL", documentID).Count(&unlinked).Error; err != nil || unlinked > 0 {
		return false, err
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

func autoConfirmEnabled(db *gorm.DB) (bool, error) {
	var row AppSetting
	err := db.Where("setting_key = ?", autoConfirmSettingKey).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
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
	if err := db.Where("status IN ?", []string{"REVIEW", "NEEDS_REVIEW"}).Find(&pending).Error; err != nil {
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
	ready, err := pendingAutoConfirm(db, threshold)
	if err != nil {
		return 0, err
	}
	confirmed := 0
	for _, document := range ready {
		if _, err := confirmNow(db, document, autoConfirmActor, "ยืนยันอัตโนมัติ: ผูกรายชื่อครบและอ่านข้อมูลคำสั่งได้มั่นใจ"); err != nil {
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
	if ok, err := readyToAutoConfirm(db, documentID, threshold); err == nil && ok {
		_, _ = confirmNow(db, document, autoConfirmActor, "ยืนยันอัตโนมัติ: ผูกรายชื่อครบและอ่านข้อมูลคำสั่งได้มั่นใจ")
	}
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
