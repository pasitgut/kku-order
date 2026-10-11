package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
)

type gormExtractionJobs struct {
	db       *gorm.DB
	logger   *log.Logger
	settings SettingsStore
	timeout  time.Duration
}

// Claim เปลี่ยนเป็น EXTRACTING แบบมีเงื่อนไข ถ้าไม่มีแถวตรงเงื่อนไขคืน ok=false (กันการส่งซ้ำ)
func (g gormExtractionJobs) Claim(id uint, now time.Time) (Document, bool, error) {
	res := g.db.Model(&Document{}).
		Where("id = ? AND processing_stage IN ? AND status <> ?", id, []string{"QUEUED", "RETRY_WAIT"}, documentCancelled).
		Updates(map[string]any{
			"processing_stage":      "EXTRACTING",
			"status":                "PROCESSING",
			"extraction_attempts":   gorm.Expr("extraction_attempts + 1"),
			"processing_started_at": now,
			"next_attempt_at":       nil,
		})
	if res.Error != nil {
		return Document{}, false, res.Error
	}
	if res.RowsAffected == 0 {
		return Document{}, false, nil
	}
	var doc Document
	if err := g.db.First(&doc, id).Error; err != nil {
		return Document{}, false, err
	}
	return doc, true, nil
}

// MarkRetry ตาม spec: RETRY_WAIT, next_attempt_at, extraction_last_status, ocr_error
func (g gormExtractionJobs) MarkRetry(id uint, plan failurePlan) error {
	return g.db.Model(&Document{}).Where("id = ? AND status <> ?", id, documentCancelled).Updates(map[string]any{
		"status":                 "PROCESSING",
		"processing_stage":       "RETRY_WAIT",
		"next_attempt_at":        plan.NextAttempt,
		"extraction_last_status": plan.LastStatus,
		"ocr_error":              plan.Failure.Message,
	}).Error
}

// MarkFailed ตั้ง NEEDS_REVIEW/FAILED และเขียน AuditLog ในทรานแซกชันเดียว
func (g gormExtractionJobs) MarkFailed(id uint, failure ExtractError, started time.Time) error {
	finished := time.Now()
	docID := id
	detail, _ := json.Marshal(failure)
	return g.db.Transaction(func(tx *gorm.DB) error {
		if err := lockEditableDocument(tx, id); err != nil {
			if errors.Is(err, errDocumentCancelled) {
				return nil
			}
			return err
		}
		if err := tx.Model(&Document{}).Where("id = ?", id).Updates(map[string]any{
			"status":                 "NEEDS_REVIEW",
			"processing_stage":       "FAILED",
			"next_attempt_at":        nil,
			"extraction_last_status": failure.Status,
			"ocr_error":              failure.Message,
			"processing_finished_at": finished,
			"processing_duration_ms": finished.Sub(started).Milliseconds(),
		}).Error; err != nil {
			return err
		}
		return tx.Create(&AuditLog{DocumentID: &docID, UserID: auditSystemUser("document-queue"), Action: "DOCUMENT_EXTRACTION_FAILED", Details: string(detail), CreatedAt: finished}).Error
	})
}

// SaveExtraction เขียนผลตาม extractionRecords ในทรานแซกชันเดียว แล้วล้าง retry state
// ลำดับ: Updates ของ document -> ลบแถวเดิม -> สร้าง fields, appointments, references, ocr_pages (แล้ว ocr_lines ที่ผูก ocr_page_id) -> sync core
func (g gormExtractionJobs) SaveExtraction(doc *Document, result ExtractionResult, started time.Time) error {
	threshold := loadOCRSettingsOrDefault(g.settings).ReviewThreshold
	rec := buildExtractionRecords(result, threshold)
	matchRecordAppointments(g.db, rec.Appointments)
	status, stage := extractionStatus(rec, threshold)
	docID := doc.ID

	return g.db.Transaction(func(tx *gorm.DB) error {
		if err := lockEditableDocument(tx, docID); err != nil {
			if errors.Is(err, errDocumentCancelled) {
				return nil
			}
			return err
		}
		rec.Updates["status"] = status
		rec.Updates["processing_stage"] = stage
		rec.Updates["ocr_provider"] = result.Engine
		rec.Updates["ocr_error"] = ""
		rec.Updates["processing_finished_at"] = time.Now()
		rec.Updates["processing_duration_ms"] = time.Since(started).Milliseconds()
		for key, value := range retryResetUpdates() {
			rec.Updates[key] = value
		}
		if err := tx.Model(&Document{}).Where("id = ?", docID).Updates(rec.Updates).Error; err != nil {
			return err
		}
		for _, model := range []any{&OCRLine{}, &OCRPage{}, &ExtractedField{}, &Appointment{}, &DocumentReference{}} {
			if err := tx.Where("document_id = ?", docID).Delete(model).Error; err != nil {
				return err
			}
		}
		for i := range rec.Fields {
			rec.Fields[i].DocumentID = docID
		}
		for i := range rec.Appointments {
			rec.Appointments[i].DocumentID = docID
		}
		for i := range rec.References {
			rec.References[i].DocumentID = docID
		}
		if len(rec.Fields) > 0 {
			if err := tx.Create(&rec.Fields).Error; err != nil {
				return err
			}
		}
		if len(rec.Appointments) > 0 {
			if err := tx.Create(&rec.Appointments).Error; err != nil {
				return err
			}
		}
		if len(rec.References) > 0 {
			if err := tx.Create(&rec.References).Error; err != nil {
				return err
			}
		}
		pageIDs := make(map[int]uint, len(rec.OCRPages))
		for i := range rec.OCRPages {
			rec.OCRPages[i].DocumentID = docID
			if err := tx.Create(&rec.OCRPages[i]).Error; err != nil {
				return err
			}
			pageIDs[rec.OCRPages[i].PageNo] = rec.OCRPages[i].ID
		}
		for i := range rec.OCRLines {
			rec.OCRLines[i].DocumentID = docID
			rec.OCRLines[i].OCRPageID = pageIDs[rec.OCRLines[i].PageNo]
			if err := tx.Create(&rec.OCRLines[i]).Error; err != nil {
				return err
			}
		}
		var updated Document
		if err := tx.First(&updated, docID).Error; err != nil {
			return err
		}
		if err := syncCoreOrderFromWorkflow(tx, &updated); err != nil {
			return err
		}
		if err := syncCoreCommitteeFromWorkflow(tx, &updated); err != nil {
			return err
		}
		return tx.Create(&AuditLog{DocumentID: &docID, UserID: auditSystemUser("document-queue"), Action: "DOCUMENT_PROCESSED", Details: fmt.Sprintf("engine=%s pages=%d fields=%d appointments=%d", result.Engine, len(rec.OCRPages), len(rec.Fields), len(rec.Appointments))}).Error
	})
}

// SaveStructured บันทึกผล DOCX/XLSX/CSV ทางเดียวกับ PDF (engine = structured-import)
func (g gormExtractionJobs) SaveStructured(doc *Document, result workerResult, started time.Time) error {
	return g.SaveExtraction(doc, structuredExtraction(result), started)
}

// matchRecordAppointments จับคู่ชื่อกับ directory ก่อนเขียน; โหลด directory ไม่ได้ก็ข้ามการจับคู่
func matchRecordAppointments(db *gorm.DB, appointments []Appointment) {
	users, aliases, err := loadDirectory(db)
	if err != nil {
		return
	}
	matchAppointments(appointments, users, aliases)
}

// DueForQueue ตามกฎ spec: ดึง EXTRACTING/PREPARING ที่ค้างกลับเป็น QUEUED แล้วคืน id ที่ถึงเวลา
func (g gormExtractionJobs) DueForQueue(now time.Time, startup bool) ([]uint, error) {
	cutoff := staleBefore(now, g.timeout, startup)
	if err := g.db.Model(&Document{}).
		Where("processing_stage IN ? AND processing_started_at < ?", []string{"EXTRACTING", "PREPARING"}, cutoff).
		Update("processing_stage", "QUEUED").Error; err != nil {
		return nil, err
	}
	var ids []uint
	queuedBefore := now.Add(-time.Minute)
	query := g.db.Model(&Document{}).Select("id").Where(
		"(processing_stage = ? AND next_attempt_at <= ?) OR (processing_stage = ? AND (updated_at < ? OR ?))",
		"RETRY_WAIT", now, "QUEUED", queuedBefore, startup,
	)
	if err := query.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}
