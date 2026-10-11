package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var (
	errDocumentNotFound    = errors.New("document not found")
	errReprocessNotAllowed = errors.New("reprocess not allowed")
)

type reprocessStore interface {
	RequestReprocess(id uint, actor string, now time.Time) error
}

type gormReprocessStore struct {
	db *gorm.DB
}

// RequestReprocess รับเฉพาะ FAILED หรือ RETRY_WAIT รีเซ็ตคอลัมน์ลองใหม่ และบันทึก AuditLog
func (s gormReprocessStore) RequestReprocess(id uint, actor string, now time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var doc Document
		if err := tx.Select("id", "processing_stage", "status").First(&doc, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errDocumentNotFound
			}
			return err
		}
		// A cancelled document stays cancelled; reprocessing it would restore workflow state.
		if doc.Status == documentCancelled {
			return errDocumentCancelled
		}
		if doc.ProcessingStage != "FAILED" && doc.ProcessingStage != "RETRY_WAIT" {
			return errReprocessNotAllowed
		}
		updates := map[string]any{
			"status":                "PROCESSING",
			"processing_stage":      "QUEUED",
			"ocr_error":             "",
			"processing_started_at": nil,
		}
		for key, value := range retryResetUpdates() {
			updates[key] = value
		}
		if err := tx.Model(&Document{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		docID := id
		return tx.Create(&AuditLog{DocumentID: &docID, UserID: actor, Action: "DOCUMENT_REPROCESS_REQUESTED", CreatedAt: now}).Error
	})
}

func reprocessDocumentHandler(store reprocessStore, enqueue func(uint)) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รหัสเอกสารไม่ถูกต้อง"})
			return
		}
		if err := store.RequestReprocess(uint(id), currentUserID(c), time.Now()); err != nil {
			switch {
			case errors.Is(err, errDocumentNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบเอกสาร"})
			case errors.Is(err, errDocumentCancelled):
				c.JSON(http.StatusConflict, gin.H{"error": "เอกสารถูกยกเลิกแล้ว ไม่สามารถประมวลผลใหม่ได้", "status": documentCancelled})
			case errors.Is(err, errReprocessNotAllowed):
				c.JSON(http.StatusConflict, gin.H{"error": "ประมวลผลใหม่ได้เฉพาะเอกสารที่ล้มเหลวหรือรอลองใหม่"})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "ประมวลผลใหม่ไม่สำเร็จ"})
			}
			return
		}
		enqueue(uint(id))
		c.JSON(http.StatusAccepted, gin.H{"id": id, "processingStage": "QUEUED"})
	}
}
