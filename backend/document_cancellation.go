package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const documentCancelled = "CANCELLED"

var errDocumentCancelled = errors.New("document is cancelled")

// Every workflow writer takes the same row lock as cancellation so an in-flight
// edit or OCR result cannot restore a cancelled document.
func lockEditableDocument(tx *gorm.DB, id uint) error {
	var document Document
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&document, id).Error; err != nil {
		return err
	}
	if document.Status == documentCancelled {
		return errDocumentCancelled
	}
	return nil
}

func writeCancelledConflict(c *gin.Context, err error) bool {
	if !errors.Is(err, errDocumentCancelled) {
		return false
	}
	c.JSON(http.StatusConflict, gin.H{"error": "เอกสารถูกยกเลิกแล้ว ไม่สามารถแก้ไขหรือยืนยันได้", "status": documentCancelled})
	return true
}

func cancelDocument(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รหัสเอกสารไม่ถูกต้อง"})
			return
		}
		var payload struct {
			Reason string `json:"reason"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาระบุเหตุผลที่ยกเลิกเอกสาร"})
			return
		}
		reason := strings.TrimSpace(payload.Reason)
		if reason == "" || utf8.RuneCountInString(reason) > 2000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาระบุเหตุผลที่ยกเลิก 1–2,000 ตัวอักษร"})
			return
		}
		actor := currentUserID(c)
		err = db.Transaction(func(tx *gorm.DB) error {
			var document Document
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&document, uint(id)).Error; err != nil {
				return err
			}
			// Retried requests preserve the first reason, actor and timestamp.
			if document.Status == documentCancelled {
				return nil
			}
			now := time.Now()
			previousStatus := document.Status
			if err := tx.Model(&document).Updates(map[string]any{
				"status": documentCancelled, "cancellation_reason": reason,
				"cancelled_by": actor, "cancelled_at": now,
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&ExpiryNotification{}).Where("document_id = ? AND status = ?", id, "PENDING").Update("status", documentCancelled).Error; err != nil {
				return err
			}
			for _, revision := range []DocumentRevision{
				{DocumentID: document.ID, FieldKey: "status", OldValue: previousStatus, NewValue: documentCancelled, ChangedBy: actor, ChangedAt: now},
				{DocumentID: document.ID, FieldKey: "cancellation_reason", NewValue: reason, ChangedBy: actor, ChangedAt: now},
			} {
				if err := tx.Create(&revision).Error; err != nil {
					return err
				}
			}
			return tx.Create(&AuditLog{DocumentID: &document.ID, UserID: actor, Action: "DOCUMENT_CANCELLED", Details: reason}).Error
		})
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบเอกสาร"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถยกเลิกเอกสารได้"})
			return
		}
		getDocument(db)(c)
	}
}
