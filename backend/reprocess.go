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
	errDocumentNotFound = errors.New("document not found")
	errDocumentBusy     = errors.New("document is being processed")
)

// rescanBusyStages are the stages in which a worker already owns the document;
// a second enqueue there would process the file twice.
var rescanBusyStages = []string{"QUEUED", "EXTRACTING", "PREPARING"}

type reprocessStore interface {
	RequestReprocess(id uint, actor string, now time.Time) error
}

type gormReprocessStore struct {
	db *gorm.DB
}

// RequestReprocess ประมวลผลไฟล์เดิมใหม่ได้ทุกสถานะ ยกเว้นยกเลิกแล้วและกำลังประมวลผลอยู่
// เอกสารที่ยืนยันแล้วจะถูกถอนการยืนยัน เพราะผลสแกนใหม่ยังไม่มีคนตรวจ
func (s gormReprocessStore) RequestReprocess(id uint, actor string, now time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// The row lock also rejects cancelled documents without a race.
		if err := lockEditableDocument(tx, id); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errDocumentNotFound
			}
			return err
		}
		var doc Document
		if err := tx.Select("id", "processing_stage").First(&doc, id).Error; err != nil {
			return err
		}
		action := "DOCUMENT_RESCAN_REQUESTED"
		if doc.ProcessingStage == "FAILED" || doc.ProcessingStage == "RETRY_WAIT" {
			action = "DOCUMENT_REPROCESS_REQUESTED"
		}
		// One conditional UPDATE is the gate: a second click finds the stage already QUEUED and changes nothing.
		result := tx.Model(&Document{}).
			Where("id = ? AND status <> ? AND processing_stage NOT IN ?", id, documentCancelled, rescanBusyStages).
			Updates(rescanResetUpdates())
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errDocumentBusy
		}
		var updated Document
		if err := tx.First(&updated, id).Error; err != nil {
			return err
		}
		if err := syncCoreConfirmation(tx, &updated); err != nil {
			return err
		}
		docID := id
		return tx.Create(&AuditLog{DocumentID: &docID, UserID: actor, Action: action, CreatedAt: now}).Error
	})
}

// rescanResetUpdates คืนค่าที่ทำให้เอกสารกลับเข้าคิวเพื่อสแกนไฟล์เดิมใหม่
// ล้างการยืนยัน (เหมือนอัปโหลดทับ) และรีเซ็ตการลองใหม่ ไม่ลบข้อมูลที่สกัดไว้ จะถูกแทนเมื่อผลใหม่ถูกบันทึก
func rescanResetUpdates() map[string]any {
	updates := map[string]any{
		"status":                "PROCESSING",
		"processing_stage":      "QUEUED",
		"ocr_error":             "",
		"processing_started_at": nil,
		"confirmed_by":          "",
		"confirmed_at":          nil,
	}
	for key, value := range retryResetUpdates() {
		updates[key] = value
	}
	return updates
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
			case errors.Is(err, errDocumentBusy):
				c.JSON(http.StatusConflict, gin.H{"error": "เอกสารกำลังประมวลผลอยู่ กรุณารอให้เสร็จก่อนสแกนใหม่"})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "ประมวลผลใหม่ไม่สำเร็จ"})
			}
			return
		}
		enqueue(uint(id))
		c.JSON(http.StatusAccepted, gin.H{"id": id, "processingStage": "QUEUED"})
	}
}
