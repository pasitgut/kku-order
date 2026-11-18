package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func contentHash(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

// การอัปโหลดไฟล์ใหม่ไฟล์เดียวกันสองครั้งอาจผ่านการตรวจไฟล์ซ้ำทั้งคู่ก่อนที่ฝั่งใดจะ insert
// เดิมฝั่งที่แพ้ไปชน unique index ของ hash แล้วตอบ 500
func TestParallelUploadOfTheSameFileAnswersDuplicate(t *testing.T) {
	db := testDatabase(t)
	t.Setenv("UPLOAD_DIR", t.TempDir())
	content := []byte(fmt.Sprintf("order,name\n%d,race\n", time.Now().UnixNano()))
	hash := contentHash(content)

	// connection ที่สองซึ่งจะมี insert ของอีกคำขอแทรกเข้ามาก่อน insert ของตัวเองเสมอ
	racing, err := gorm.Open(mysql.Open(os.Getenv("DOCFLOW_TEST_DSN")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var winner Document
	inserted := false
	if err := racing.Callback().Create().Before("gorm:create").Register("test:race", func(tx *gorm.DB) {
		if inserted || tx.Statement.Table != "documents" {
			return
		}
		inserted = true
		winner = Document{Title: "race", OriginalFilename: "race.csv", StoragePath: "/tmp/race.csv", SourceType: "csv", MimeType: "text/csv", Status: "PROCESSING", ProcessingStage: "QUEUED", ContentState: "UNSAVED", FileHash: hash, ImportSource: "UPLOAD", ImportedBy: "other", QualityProfile: "STANDARD"}
		if err := db.Create(&winner).Error; err != nil {
			t.Errorf("insert winner: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	response := serveUpload(t, uploadDocument(racing, testProcessor()), "/documents", "/documents", "race.csv", content, false)
	if response.Code != http.StatusConflict {
		t.Fatalf("losing upload = %d %s, want 409", response.Code, response.Body.String())
	}
	var body struct {
		DuplicateDocumentID uint `json:"duplicateDocumentId"`
		CanOverwrite        bool `json:"canOverwrite"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	if body.DuplicateDocumentID != winner.ID || !body.CanOverwrite {
		t.Fatalf("duplicate body = %s, want the winner %d", response.Body.String(), winner.ID)
	}
}

// การเขียนทับไฟล์ซ้ำจะอ่านไฟล์ใหม่ตั้งแต่ต้น สิ่งที่อ่านได้จากรอบก่อนจึงต้องไม่หลงเหลืออยู่
// เหมือนกับการแทนที่ไฟล์
func TestOverwritingADuplicateClearsWhatTheOldRunRead(t *testing.T) {
	db := testDatabase(t)
	t.Setenv("UPLOAD_DIR", t.TempDir())
	content := []byte(fmt.Sprintf("order,name\n%d,overwrite\n", time.Now().UnixNano()))
	document, _ := createReviewDocument(t, db, map[string]string{"order_no": "1/2569"})
	issued := time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local)
	if err := db.Model(&document).Updates(map[string]any{
		"file_hash": contentHash(content), "order_type": "คำสั่ง", "order_no": "1/2569", "order_year_be": 2569,
		"committee": "คณะกรรมการเดิม", "issuing_unit": "หน่วยเดิม", "signer_name": "ผู้ลงนามเดิม", "signer_position": "คณบดี",
		"responsibilities": "หน้าที่เดิม", "additional_refs": "อ้างอิงเดิม", "issued_date": issued, "effective_date": issued, "expiry_date": issued.AddDate(1, 0, 0),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&ExpiryNotification{DocumentID: document.ID, NoticeType: "3M", DueAt: issued, Status: "PENDING"}).Error; err != nil {
		t.Fatal(err)
	}

	response := serveUpload(t, uploadDocument(db, testProcessor()), "/documents", "/documents", "overwrite.csv", content, true)
	if response.Code != http.StatusOK {
		t.Fatalf("overwrite = %d %s", response.Code, response.Body.String())
	}
	var saved Document
	if err := db.First(&saved, document.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.OrderType != "" || saved.OrderNo != "" || saved.OrderYearBE != 0 || saved.Committee != "" || saved.IssuingUnit != "" ||
		saved.SignerName != "" || saved.SignerPosition != "" || saved.Responsibilities != "" || saved.AdditionalRefs != "" ||
		saved.IssuedDate != nil || saved.EffectiveDate != nil || saved.ExpiryDate != nil {
		t.Fatalf("overwritten document kept old metadata: %+v", saved)
	}
	var notifications int64
	db.Model(&ExpiryNotification{}).Where("document_id = ?", document.ID).Count(&notifications)
	if notifications != 0 {
		t.Fatalf("expiry notifications of the old run = %d, want 0", notifications)
	}
}
