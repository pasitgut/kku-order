package main

import (
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveExtractionWritesJSONSnapshot(t *testing.T) {
	db := testDatabase(t).Begin()
	if db.Error != nil {
		t.Fatal(db.Error)
	}
	defer db.Rollback()
	doc, _ := createReviewDocument(t, db, nil)
	doc.StoragePath = filepath.Join(t.TempDir(), "original.pdf")
	doc.ContentState = "UNSAVED"
	if err := db.Model(&doc).Updates(map[string]any{"storage_path": doc.StoragePath, "content_state": "UNSAVED"}).Error; err != nil {
		t.Fatal(err)
	}
	jobs := gormExtractionJobs{db: db, logger: log.New(io.Discard, "", 0), settings: NewSettingsStore(db)}
	result := ExtractionResult{Engine: "test", Header: ExtractedHeader{Subject: ExtractedValue{Text: "ทดสอบ"}}, Pages: []ExtractedPage{{Page: 1, Width: 100, Height: 200, Lines: []ExtractedValue{{Text: "ข้อความ OCR"}}}}}
	if err := jobs.SaveExtraction(&doc, result, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&doc, doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := readExtractionFile(doc.ExtractionJSONPath, doc.ID)
	if err != nil || snapshot.Result.Header.Subject.Text != "ทดสอบ" || len(snapshot.Fields) == 0 || snapshot.OCRText != "ข้อความ OCR" || len(snapshot.OCRPages) != 1 {
		t.Fatalf("snapshot = %#v, error = %v", snapshot, err)
	}
	var count int64
	if db.Migrator().HasTable("ocr_pages") || db.Migrator().HasTable("ocr_lines") || db.Migrator().HasColumn(&Document{}, "ocr_text") {
		t.Fatal("legacy OCR tables or column still exist")
	}
	if err := db.Model(&ExtractedField{}).Where("document_id = ?", doc.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("fields before save = %d, error = %v", count, err)
	}
	fields := make([]map[string]any, len(snapshot.Fields))
	for i, field := range snapshot.Fields {
		fields[i] = map[string]any{"id": i + 1, "fieldKey": field.FieldKey, "fieldLabel": field.FieldLabel, "value": field.Value}
	}
	response := patchDocument(t, db, doc.ID, map[string]any{"extractionVersion": extractionVersion(doc.ExtractionJSONPath), "expectedUpdatedAt": doc.UpdatedAt, "fields": fields, "appointments": []any{}})
	if response.Code != 200 {
		t.Fatalf("save draft = %d: %s", response.Code, response.Body.String())
	}
	if err := db.Model(&ExtractedField{}).Where("document_id = ?", doc.ID).Count(&count).Error; err != nil || count != int64(len(snapshot.Fields)) {
		t.Fatalf("fields after save = %d, error = %v", count, err)
	}
}

func TestRescanKeepsSavedFieldsUntilNewExtractionIsChosen(t *testing.T) {
	db := testDatabase(t).Begin()
	if db.Error != nil {
		t.Fatal(db.Error)
	}
	defer db.Rollback()
	doc, _ := createReviewDocument(t, db, map[string]string{"order_no": "เดิม"})
	doc.StoragePath = filepath.Join(t.TempDir(), "original.pdf")
	if err := db.Model(&doc).Update("storage_path", doc.StoragePath).Error; err != nil {
		t.Fatal(err)
	}
	jobs := gormExtractionJobs{db: db, logger: log.New(io.Discard, "", 0), settings: NewSettingsStore(db)}
	result := ExtractionResult{Engine: "test", Header: ExtractedHeader{OrderNo: ExtractedValue{Text: "ใหม่"}}}
	if err := jobs.SaveExtraction(&doc, result, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&doc, doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	var field ExtractedField
	if err := db.Where("document_id = ? AND field_key = ?", doc.ID, "order_no").First(&field).Error; err != nil || field.Value != "เดิม" {
		t.Fatalf("saved field = %#v, error = %v", field, err)
	}
	if doc.PendingExtractionJSONPath == "" {
		t.Fatal("new extraction was not kept pending")
	}
	snapshot, err := readExtractionFile(doc.PendingExtractionJSONPath, doc.ID)
	if err != nil || snapshot.Result.Header.OrderNo.Text != "ใหม่" {
		t.Fatalf("pending snapshot = %#v, error = %v", snapshot, err)
	}
}
