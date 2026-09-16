package main

import (
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type updateDocumentRequest struct {
	Title                *string               `json:"title"`
	OrderNo              *string               `json:"orderNo"`
	OrderYearBE          *int                  `json:"orderYearBE"`
	Committee            *string               `json:"committee"`
	SignerName           *string               `json:"signerName"`
	SignerPosition       *string               `json:"signerPosition"`
	Responsibilities     *string               `json:"responsibilities"`
	AdditionalReferences *string               `json:"additionalReferences"`
	IssuedDate           *string               `json:"issuedDate"`
	EffectiveDate        *string               `json:"effectiveDate"`
	ExpiryDate           *string               `json:"expiryDate"`
	ReviewNotes          *string               `json:"reviewNotes"`
	Fields               *[]fieldUpdateRequest `json:"fields"`
	Appointments         *[]appointmentRequest `json:"appointments"`
}

type fieldUpdateRequest struct {
	ID         uint   `json:"id"`
	FieldKey   string `json:"fieldKey"`
	FieldLabel string `json:"fieldLabel"`
	Value      string `json:"value"`
	Verified   bool   `json:"verified"`
}

type appointmentRequest struct {
	ID               uint    `json:"id"`
	FullName         string  `json:"fullName" binding:"required"`
	Position         string  `json:"position"`
	Department       string  `json:"department"`
	CommitteeRole    string  `json:"committeeRole"`
	Responsibilities string  `json:"responsibilities"`
	Confidence       float32 `json:"confidence"`
	PageNo           int     `json:"pageNo"`
	BoundingBox      string  `json:"boundingBox"`
	Verified         bool    `json:"verified"`
}

func dashboardHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var total, processing, review, confirmed, needsReview, expiring int64
		if err := db.Model(&Document{}).Count(&total).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดภาพรวมระบบได้"})
			return
		}
		if err := db.Model(&Document{}).Where("status = ?", "PROCESSING").Count(&processing).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดภาพรวมระบบได้"})
			return
		}
		_ = db.Model(&Document{}).Where("status = ?", "REVIEW").Count(&review).Error
		_ = db.Model(&Document{}).Where("status = ?", "CONFIRMED").Count(&confirmed).Error
		_ = db.Model(&Document{}).Where("status IN ?", []string{"NEEDS_REVIEW", "FAILED"}).Count(&needsReview).Error
		_ = db.Model(&Document{}).Where("expiry_date IS NOT NULL AND expiry_date BETWEEN CURRENT_TIMESTAMP AND DATE_ADD(CURRENT_TIMESTAMP, INTERVAL 3 MONTH)").Count(&expiring).Error
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"total": total, "processing": processing, "review": review, "confirmed": confirmed,
			"needsReview": needsReview, "expiring": expiring,
		}})
	}
}

func searchDocuments(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := applyDocumentFilters(documentQuery(db, c.Query("q"), c.Query("status")), c)
		var documents []Document
		if err := query.Preload("Fields").Preload("Appointments").Order("created_at DESC").Limit(parseLimit(c)).Offset(parseOffset(c)).Find(&documents).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถค้นหาข้อมูลได้"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": documents})
	}
}

func listDirectoryUsers(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var users []DirectoryUser
		query := db.Order("full_name ASC").Limit(100)
		if q := strings.TrimSpace(c.Query("q")); q != "" {
			like := "%" + q + "%"
			query = query.Where("full_name LIKE ? OR first_name LIKE ? OR last_name LIKE ? OR username LIKE ? OR email LIKE ? OR department LIKE ?", like, like, like, like, like, like)
		}
		if err := query.Find(&users).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดรายชื่อบุคลากรได้"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": users})
	}
}

func documentFile(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var document Document
		if err := db.Select("id", "storage_path", "original_filename", "mime_type").First(&document, c.Param("id")).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบเอกสาร"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดไฟล์เอกสารได้"})
			return
		}
		if _, err := os.Stat(document.StoragePath); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบไฟล์ต้นฉบับ"})
			return
		}
		c.Header("Content-Disposition", `inline; filename="`+strings.ReplaceAll(document.OriginalFilename, `"`, "")+`"`)
		if document.MimeType != "" {
			c.Header("Content-Type", document.MimeType)
		}
		c.File(document.StoragePath)
	}
}

func documentPageImage(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		pageNo, err := strconv.Atoi(c.Param("page"))
		if err != nil || pageNo < 1 || pageNo > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "หมายเลขหน้าไม่ถูกต้อง"})
			return
		}
		var document Document
		if err := db.Select("id", "storage_path", "source_type").First(&document, c.Param("id")).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบเอกสาร"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดเอกสารได้"})
			return
		}
		if !strings.EqualFold(document.SourceType, "pdf") {
			c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "เอกสารนี้ไม่มีหน้า PDF สำหรับแสดงผล"})
			return
		}
		if _, err := os.Stat(document.StoragePath); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบไฟล์ต้นฉบับ"})
			return
		}

		tempDir, err := os.MkdirTemp("", "docflow-page-")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถเตรียมพื้นที่แสดงเอกสารได้"})
			return
		}
		defer os.RemoveAll(tempDir)
		outputBase := filepath.Join(tempDir, "page")
		args := []string{"-png", "-r", getenvInt("OCR_DPI", 200), "-f", strconv.Itoa(pageNo), "-l", strconv.Itoa(pageNo), "-singlefile", document.StoragePath, outputBase}
		if output, err := exec.CommandContext(c.Request.Context(), getenv("PDFTOPPM_BIN", "pdftoppm"), args...).CombinedOutput(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถ render หน้าเอกสารได้", "details": strings.TrimSpace(string(output))})
			return
		}
		imagePath := outputBase + ".png"
		if _, err := os.Stat(imagePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่พบภาพหน้าที่ render แล้ว"})
			return
		}
		c.Header("Cache-Control", "private, max-age=300")
		c.Header("Content-Type", "image/png")
		c.File(imagePath)
	}
}

func updateDocument(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload updateDocumentRequest
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
			return
		}
		var document Document
		if err := db.First(&document, c.Param("id")).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบเอกสาร"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดเอกสารได้"})
			return
		}
		changedBy := currentUserID(c)
		updates := map[string]any{}
		revisions := make([]DocumentRevision, 0)
		addChange := func(key string, oldValue, newValue string, databaseKey string) {
			if oldValue == newValue {
				return
			}
			updates[databaseKey] = newValue
			revisions = append(revisions, DocumentRevision{DocumentID: document.ID, FieldKey: key, OldValue: oldValue, NewValue: newValue, ChangedBy: changedBy, ChangedAt: time.Now()})
		}
		if payload.Title != nil {
			addChange("title", document.Title, *payload.Title, "title")
		}
		if payload.OrderNo != nil {
			addChange("order_no", document.OrderNo, *payload.OrderNo, "order_no")
		}
		if payload.OrderYearBE != nil && document.OrderYearBE != *payload.OrderYearBE {
			updates["order_year_be"] = *payload.OrderYearBE
			revisions = append(revisions, DocumentRevision{DocumentID: document.ID, FieldKey: "order_year_be", OldValue: strconv.Itoa(document.OrderYearBE), NewValue: strconv.Itoa(*payload.OrderYearBE), ChangedBy: changedBy, ChangedAt: time.Now()})
		}
		if payload.Committee != nil {
			addChange("committee", document.Committee, *payload.Committee, "committee")
		}
		if payload.SignerName != nil {
			addChange("signer_name", document.SignerName, *payload.SignerName, "signer_name")
		}
		if payload.SignerPosition != nil {
			addChange("signer_position", document.SignerPosition, *payload.SignerPosition, "signer_position")
		}
		if payload.Responsibilities != nil {
			addChange("responsibilities", document.Responsibilities, *payload.Responsibilities, "responsibilities")
		}
		if payload.AdditionalReferences != nil {
			addChange("additional_references", document.AdditionalRefs, *payload.AdditionalReferences, "additional_refs")
		}
		if payload.ReviewNotes != nil {
			addChange("review_notes", document.ReviewNotes, *payload.ReviewNotes, "review_notes")
		}
		dateUpdates := map[string]*string{"issued_date": payload.IssuedDate, "effective_date": payload.EffectiveDate, "expiry_date": payload.ExpiryDate}
		dateOld := map[string]*time.Time{"issued_date": document.IssuedDate, "effective_date": document.EffectiveDate, "expiry_date": document.ExpiryDate}
		for key, raw := range dateUpdates {
			if raw == nil {
				continue
			}
			parsed, err := parseDateInput(*raw)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			updates[key] = parsed
			if formatDate(dateOld[key]) != formatDate(parsed) {
				revisions = append(revisions, DocumentRevision{DocumentID: document.ID, FieldKey: key, OldValue: formatDate(dateOld[key]), NewValue: formatDate(parsed), ChangedBy: changedBy, ChangedAt: time.Now()})
			}
		}
		if payload.Fields != nil || payload.Appointments != nil {
			updates["status"] = "NEEDS_REVIEW"
		}
		if payload.Fields != nil {
			for _, input := range *payload.Fields {
				switch input.FieldKey {
				case "order_type":
					updates["order_type"] = input.Value
				case "order_no":
					updates["order_no"] = input.Value
				case "order_year_be":
					if value, err := strconv.Atoi(strings.TrimSpace(input.Value)); err == nil {
						updates["order_year_be"] = value
					}
				case "committee_name":
					updates["title"] = input.Value
					updates["committee"] = input.Value
				case "issued_date", "effective_date", "expiry_date":
					parsed, err := parseDateInput(input.Value)
					if err != nil {
						c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
						return
					}
					updates[input.FieldKey] = parsed
				case "signer_name":
					updates["signer_name"] = input.Value
				case "signer_position":
					updates["signer_position"] = input.Value
				case "responsibilities":
					updates["responsibilities"] = input.Value
				case "additional_references":
					updates["additional_refs"] = input.Value
				}
			}
		}
		if len(updates) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่มีข้อมูลที่เปลี่ยนแปลง"})
			return
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if len(updates) > 0 {
				if err := tx.Model(&document).Updates(updates).Error; err != nil {
					return err
				}
			}
			if payload.Fields != nil {
				for _, input := range *payload.Fields {
					if input.ID == 0 {
						continue
					}
					var existingField ExtractedField
					if err := tx.Where("id = ? AND document_id = ?", input.ID, document.ID).First(&existingField).Error; err == nil && existingField.Value != input.Value {
						revisions = append(revisions, DocumentRevision{DocumentID: document.ID, FieldKey: input.FieldKey, OldValue: existingField.Value, NewValue: input.Value, ChangedBy: changedBy, ChangedAt: time.Now()})
					}
					if err := tx.Model(&ExtractedField{}).Where("id = ? AND document_id = ?", input.ID, document.ID).Updates(map[string]any{"value": input.Value, "verified": input.Verified, "verified_by": changedBy, "verified_at": time.Now()}).Error; err != nil {
						return err
					}
				}
			}
			if payload.Appointments != nil {
				if err := tx.Where("document_id = ?", document.ID).Delete(&Appointment{}).Error; err != nil {
					return err
				}
				var directoryUsers []DirectoryUser
				if err := tx.Where("is_active = ?", true).Find(&directoryUsers).Error; err != nil {
					return err
				}
				for _, input := range *payload.Appointments {
					appointment := Appointment{DocumentID: document.ID, FullName: input.FullName, Position: input.Position, Department: input.Department, CommitteeRole: input.CommitteeRole, Responsibilities: input.Responsibilities, Confidence: input.Confidence, PageNo: input.PageNo, BoundingBoxJSON: input.BoundingBox, Verified: input.Verified, VerifiedBy: changedBy, NameMatchMethod: "UNMATCHED"}
					if matched, score, method, ok := findDirectoryUser(input.FullName, directoryUsers); ok {
						appointment.DirectoryUserID = &matched.ID
						appointment.NameMatchMethod = method
						appointment.NameMatchScore = score
						if score < 0.90 {
							appointment.Confidence = minConfidence(appointment.Confidence, score)
						}
					}
					if err := tx.Create(&appointment).Error; err != nil {
						return err
					}
				}
			}
			for _, revision := range revisions {
				if err := tx.Create(&revision).Error; err != nil {
					return err
				}
			}
			return tx.Create(&AuditLog{DocumentID: &document.ID, UserID: changedBy, Action: "DOCUMENT_UPDATED", Details: "เจ้าหน้าที่แก้ไขข้อมูลก่อนยืนยัน"}).Error
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกการแก้ไขได้"})
			return
		}
		getDocument(db)(c)
	}
}

func deleteDocument(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var document Document
		if err := db.Select("id", "storage_path", "original_filename").First(&document, c.Param("id")).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบเอกสาร"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดเอกสารก่อนลบได้"})
			return
		}

		err := db.Transaction(func(tx *gorm.DB) error {
			// Delete children explicitly because the schema is intentionally
			// portable and does not rely on database-level cascade constraints.
			for _, model := range []any{
				&OCRLine{}, &OCRPage{}, &ExtractedField{}, &Appointment{},
				&DocumentReference{}, &DocumentRevision{}, &ExpiryNotification{}, &AuditLog{},
			} {
				if err := tx.Where("document_id = ?", document.ID).Delete(model).Error; err != nil {
					return err
				}
			}
			return tx.Delete(&Document{}, document.ID).Error
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบข้อมูลเอกสารได้"})
			return
		}
		if document.StoragePath != "" {
			if err := os.Remove(document.StoragePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				// The database deletion succeeded. Return success and expose the
				// storage cleanup issue for developer diagnostics.
				c.JSON(http.StatusOK, gin.H{"message": "ลบข้อมูลเอกสารแล้ว แต่ลบไฟล์ต้นฉบับไม่สำเร็จ", "storageCleanupError": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"message": "ลบข้อมูลเอกสารและข้อมูลที่เกี่ยวข้องแล้ว", "documentId": document.ID})
	}
}

func documentQuery(db *gorm.DB, rawQuery, rawStatus string) *gorm.DB {
	query := db.Model(&Document{})
	if q := strings.TrimSpace(rawQuery); q != "" {
		like := "%" + q + "%"
		query = query.Where("title LIKE ? OR order_no LIKE ? OR order_type LIKE ? OR committee LIKE ? OR signer_name LIKE ? OR responsibilities LIKE ? OR additional_refs LIKE ? OR ocr_text LIKE ? OR id IN (?)", like, like, like, like, like, like, like, like, db.Model(&Appointment{}).Select("document_id").Where("full_name LIKE ? OR position LIKE ? OR committee_role LIKE ? OR responsibilities LIKE ?", like, like, like, like))
	}
	if status := strings.TrimSpace(rawStatus); status != "" && status != "ALL" {
		query = query.Where("status = ?", status)
	}
	return query
}

func applyDocumentFilters(query *gorm.DB, c *gin.Context) *gorm.DB {
	if orderType := strings.TrimSpace(c.Query("orderType")); orderType != "" {
		query = query.Where("order_type = ?", orderType)
	}
	if position := strings.TrimSpace(c.Query("position")); position != "" {
		like := "%" + position + "%"
		appointmentQuery := query.Session(&gorm.Session{NewDB: true}).Model(&Appointment{}).Select("document_id").Where("position LIKE ? OR committee_role LIKE ?", like, like)
		query = query.Where("id IN (?)", appointmentQuery)
	}
	if raw := strings.TrimSpace(c.Query("dateFrom")); raw != "" {
		if parsed, err := parseDateInput(raw); err == nil && parsed != nil {
			query = query.Where("issued_date >= ?", *parsed)
		}
	}
	if raw := strings.TrimSpace(c.Query("dateTo")); raw != "" {
		if parsed, err := parseDateInput(raw); err == nil && parsed != nil {
			query = query.Where("issued_date < ?", parsed.AddDate(0, 0, 1))
		}
	}
	return query
}

func parseLimit(c *gin.Context) int {
	value, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
	if value < 1 {
		return 25
	}
	if value > 100 {
		return 100
	}
	return value
}

func parsePage(c *gin.Context) int {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		return 1
	}
	return page
}

func parseOffset(c *gin.Context) int {
	return (parsePage(c) - 1) * parseLimit(c)
}

func parseDateInput(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, errors.New("วันที่ต้องอยู่ในรูปแบบ YYYY-MM-DD")
	}
	return &parsed, nil
}
