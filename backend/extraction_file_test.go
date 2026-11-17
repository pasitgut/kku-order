package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractionFileRoundTripAndValidation(t *testing.T) {
	dir := t.TempDir()
	snapshot := extractionFile{DocumentID: 42, Result: ExtractionResult{Engine: "test"}, Fields: []ExtractedField{{FieldKey: "title", Value: "คำสั่ง"}}}
	path, err := writeExtractionFile(filepath.Join(dir, "original.pdf"), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readExtractionFile(path, 42)
	if err != nil || got.Result.Engine != "test" || len(got.Fields) != 1 || got.Fields[0].Value != "คำสั่ง" {
		t.Fatalf("snapshot = %#v, error = %v", got, err)
	}
	if _, err := readExtractionFile(path, 43); err == nil {
		t.Fatal("accepted another document's extraction")
	}
	if err := os.WriteFile(path, []byte(`{"version":2,"documentId":42}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readExtractionFile(path, 42); err == nil {
		t.Fatal("accepted an unsupported version")
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readExtractionFile(path, 42); err == nil {
		t.Fatal("accepted invalid JSON")
	}
}

func TestExtractionOCRAndSearchFromFile(t *testing.T) {
	result := ExtractionResult{Pages: []ExtractedPage{{Page: 1, Width: 100, Height: 100, Lines: []ExtractedValue{{Text: "ข้อความ OCR"}}}}}
	records := buildExtractionRecords(result, 0)
	text, pages := extractionOCR(records, 42)
	if text != "ข้อความ OCR" || len(pages) != 1 || len(pages[0].Lines) != 1 || pages[0].Lines[0].Text != text {
		t.Fatalf("text = %q, pages = %#v", text, pages)
	}
	path, err := writeExtractionFile(filepath.Join(t.TempDir(), "original.pdf"), extractionFile{DocumentID: 42, Result: result, OCRText: text, OCRPages: pages})
	if err != nil {
		t.Fatal(err)
	}
	got, err := indexedOCRText(path, 42, false)
	if err != nil || !strings.Contains(got, "ข้อความ") {
		t.Fatalf("search text = %q, error = %v", got, err)
	}
	if _, err := indexedOCRText(path, 43, false); err == nil {
		t.Fatal("accepted a different document ID from cache")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := indexedOCRText(path, 42, false); err == nil {
		t.Fatal("accepted a missing cached file")
	}
}

func TestExtractionFileDoesNotLeavePartialFile(t *testing.T) {
	dir := t.TempDir()
	documentPath := filepath.Join(dir, "original.pdf")
	_, err := writeExtractionFile(documentPath, extractionFile{DocumentID: 9, Updates: map[string]any{"invalid": make(chan int)}})
	if err == nil {
		t.Fatal("expected encoding to fail")
	}
	files, err := os.ReadDir(extractionDirectory(documentPath, 9))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("partial files remain: %v", files)
	}
}

func TestExtractionPreviewKeepsDocumentIdentityAndAssignsStableIDs(t *testing.T) {
	doc := Document{ID: 42, Title: "เดิม", ContentState: "UNSAVED"}
	snapshot := extractionFile{
		DocumentID:   42,
		Fields:       []ExtractedField{{FieldKey: "order_no", Value: "123"}},
		Appointments: []Appointment{{FullName: "สมชาย"}},
		Updates:      map[string]any{"title": "คำสั่งใหม่", "order_no": "123", "order_year_be": 2569},
	}
	if err := applyExtractionPreview(&doc, snapshot, "test.json"); err != nil {
		t.Fatal(err)
	}
	if doc.ID != 42 || doc.Title != "คำสั่งใหม่" || doc.OrderNo != "123" || doc.OrderYearBE != 2569 || len(doc.Fields) != 1 || doc.Fields[0].ID != 1 || len(doc.Appointments) != 1 || doc.Appointments[0].ID != 1 || doc.ExtractionVersion != extractionVersion("test.json") {
		t.Fatalf("preview = %#v", doc)
	}
	if extractionVersion("test.json") == extractionVersion("other.json") {
		t.Fatal("different snapshots have the same version")
	}
}

func TestUnsavedExtractionSearchIncludesFieldsAndPeople(t *testing.T) {
	path, err := writeExtractionFile(filepath.Join(t.TempDir(), "original.pdf"), extractionFile{DocumentID: 5, Fields: []ExtractedField{{Value: "123/2569"}}, Appointments: []Appointment{{FullName: "สมชาย ทดสอบ"}}})
	if err != nil {
		t.Fatal(err)
	}
	text, err := indexedOCRText(path, 5, true)
	if err != nil || !strings.Contains(text, "123/2569") || !strings.Contains(text, "สมชาย") {
		t.Fatalf("text = %q, error = %v", text, err)
	}
}
