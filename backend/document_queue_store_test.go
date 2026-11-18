package main

import (
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"
)

// งาน OCR ที่เริ่มกับไฟล์เดิมต้องไม่เขียนผลทับเอกสารที่ถูกอัปโหลดไฟล์ใหม่ระหว่างทาง
func TestSaveExtractionDiscardsResultOfReplacedFile(t *testing.T) {
	db := testDatabase(t).Begin()
	if db.Error != nil {
		t.Fatal(db.Error)
	}
	defer db.Rollback()
	doc, _ := createReviewDocument(t, db, nil)
	dir := t.TempDir()
	doc.StoragePath = filepath.Join(dir, "old.pdf")
	if err := db.Model(&doc).Updates(map[string]any{"storage_path": doc.StoragePath, "status": "PROCESSING", "processing_stage": "EXTRACTING", "content_state": "UNSAVED"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&doc, doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	stale := doc
	// ผู้ใช้อัปโหลดไฟล์ใหม่ทับ เอกสารกลับเข้าคิวรออ่านไฟล์ใหม่
	newPath := filepath.Join(dir, "new.pdf")
	if err := db.Model(&Document{}).Where("id = ?", doc.ID).Updates(map[string]any{"storage_path": newPath, "file_hash": doc.FileHash + "-new", "status": "PROCESSING", "processing_stage": "QUEUED"}).Error; err != nil {
		t.Fatal(err)
	}
	jobs := gormExtractionJobs{db: db, logger: log.New(io.Discard, "", 0), settings: NewSettingsStore(db)}
	result := ExtractionResult{Engine: "test", Header: ExtractedHeader{Subject: ExtractedValue{Text: "ผลของไฟล์เก่า"}}}
	if err := jobs.SaveExtraction(&stale, result, time.Now()); err != nil {
		t.Fatalf("a stale result is dropped quietly, not reported as a failure: %v", err)
	}
	var current Document
	if err := db.First(&current, doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.ProcessingStage != "QUEUED" || current.Status != "PROCESSING" || current.ExtractionJSONPath != "" || current.PendingExtractionJSONPath != "" {
		t.Fatalf("stale job overwrote the new file's state: stage=%s status=%s json=%q pending=%q", current.ProcessingStage, current.Status, current.ExtractionJSONPath, current.PendingExtractionJSONPath)
	}
	var processed int64
	if err := db.Model(&AuditLog{}).Where("document_id = ? AND action = ?", doc.ID, "DOCUMENT_PROCESSED").Count(&processed).Error; err != nil || processed != 0 {
		t.Fatalf("DOCUMENT_PROCESSED audit rows = %d, err = %v", processed, err)
	}
}

// claimedThenReplaced คืนงานที่ถูก claim กับไฟล์เดิม แล้วจำลองว่าผู้ใช้อัปโหลดไฟล์ใหม่ทับ เอกสารกลับเข้าคิว
func claimedThenReplaced(t *testing.T, db *gorm.DB) (stale Document, jobs gormExtractionJobs) {
	t.Helper()
	doc, _ := createReviewDocument(t, db, nil)
	dir := t.TempDir()
	if err := db.Model(&doc).Updates(map[string]any{"storage_path": filepath.Join(dir, "old.pdf"), "status": "PROCESSING", "processing_stage": "EXTRACTING", "ocr_error": ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&stale, doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&Document{}).Where("id = ?", doc.ID).Updates(map[string]any{"storage_path": filepath.Join(dir, "new.pdf"), "file_hash": stale.FileHash + "-new", "status": "PROCESSING", "processing_stage": "QUEUED"}).Error; err != nil {
		t.Fatal(err)
	}
	return stale, gormExtractionJobs{db: db, logger: log.New(io.Discard, "", 0), settings: NewSettingsStore(db)}
}

func assertStillQueuedForNewFile(t *testing.T, db *gorm.DB, id uint) Document {
	t.Helper()
	var current Document
	if err := db.First(&current, id).Error; err != nil {
		t.Fatal(err)
	}
	if current.ProcessingStage != "QUEUED" || current.Status != "PROCESSING" || current.OCRError != "" || current.NextAttemptAt != nil {
		t.Fatalf("stale job changed the new file's state: stage=%s status=%s ocr_error=%q next=%v", current.ProcessingStage, current.Status, current.OCRError, current.NextAttemptAt)
	}
	return current
}

// ความล้มเหลวของงานไฟล์เก่าต้องไม่ทำให้เอกสารที่อัปโหลดไฟล์ใหม่แล้วกลายเป็น FAILED (Claim จะไม่หยิบงานใหม่อีก)
func TestMarkFailedIgnoresJobOfReplacedFile(t *testing.T) {
	db := testDatabase(t).Begin()
	if db.Error != nil {
		t.Fatal(db.Error)
	}
	defer db.Rollback()
	stale, jobs := claimedThenReplaced(t, db)
	if err := jobs.MarkFailed(&stale, ExtractError{Message: "ไฟล์เก่าอ่านไม่ได้"}, time.Now()); err != nil {
		t.Fatalf("a stale failure is dropped quietly: %v", err)
	}
	current := assertStillQueuedForNewFile(t, db, stale.ID)
	var failedAudits int64
	if err := db.Model(&AuditLog{}).Where("document_id = ? AND action = ?", stale.ID, "DOCUMENT_EXTRACTION_FAILED").Count(&failedAudits).Error; err != nil || failedAudits != 0 {
		t.Fatalf("DOCUMENT_EXTRACTION_FAILED audit rows = %d, err = %v", failedAudits, err)
	}
	// งานของไฟล์ปัจจุบันยังตั้ง FAILED ได้ตามปกติ
	if err := jobs.MarkFailed(&current, ExtractError{Message: "ไฟล์ใหม่อ่านไม่ได้"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&current, stale.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.ProcessingStage != "FAILED" || current.Status != "NEEDS_REVIEW" || current.OCRError != "ไฟล์ใหม่อ่านไม่ได้" {
		t.Fatalf("current job failure not recorded: stage=%s status=%s ocr_error=%q", current.ProcessingStage, current.Status, current.OCRError)
	}
}

func TestMarkRetryIgnoresJobOfReplacedFile(t *testing.T) {
	db := testDatabase(t).Begin()
	if db.Error != nil {
		t.Fatal(db.Error)
	}
	defer db.Rollback()
	stale, jobs := claimedThenReplaced(t, db)
	plan := failurePlan{Retry: true, NextAttempt: time.Now().Add(time.Hour), LastStatus: 503, Failure: ExtractError{Message: "บริการ OCR ไม่ว่าง"}}
	if err := jobs.MarkRetry(&stale, plan); err != nil {
		t.Fatalf("a stale retry is dropped quietly: %v", err)
	}
	current := assertStillQueuedForNewFile(t, db, stale.ID)
	if err := jobs.MarkRetry(&current, plan); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&current, stale.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.ProcessingStage != "RETRY_WAIT" || current.NextAttemptAt == nil || current.OCRError != "บริการ OCR ไม่ว่าง" {
		t.Fatalf("current job retry not recorded: stage=%s next=%v ocr_error=%q", current.ProcessingStage, current.NextAttemptAt, current.OCRError)
	}
}
