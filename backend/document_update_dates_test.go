package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// testDatabase connects to a real MySQL when DOCFLOW_TEST_DSN is set, and skips otherwise.
func testDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DOCFLOW_TEST_DSN")
	if dsn == "" {
		t.Skip("set DOCFLOW_TEST_DSN to run database-backed tests")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateDatabase(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func createReviewDocument(t *testing.T, db *gorm.DB, fields map[string]string) (Document, map[string]uint) {
	t.Helper()
	title := "ทดสอบวันที่-" + time.Now().Format("150405.000000000")
	now := time.Now()
	core := StoredDocument{FileName: title + ".pdf", StoragePath: "/tmp/" + title + ".pdf", FileType: "pdf", FileHash: title, CreatedAt: now, UpdatedAt: now}
	document := Document{Title: title, OriginalFilename: title + ".pdf", StoragePath: core.StoragePath, SourceType: "pdf", MimeType: "application/pdf", Status: "REVIEW", ProcessingStage: "COMPLETED", FileHash: title, ImportSource: "UPLOAD", ImportedBy: "tester", QualityProfile: "STANDARD", CreatedAt: now, UpdatedAt: now}
	if err := createDocumentBundle(db, &core, &document); err != nil {
		t.Fatal(err)
	}
	ids := map[string]uint{}
	for key, value := range fields {
		field := ExtractedField{DocumentID: document.ID, FieldKey: key, FieldLabel: key, FieldType: "date", Value: value, SourceText: value, BoundingBoxJSON: "null"}
		if err := db.Create(&field).Error; err != nil {
			t.Fatal(err)
		}
		ids[key] = field.ID
	}
	return document, ids
}

func patchDocument(t *testing.T, db *gorm.DB, id uint, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PATCH("/api/v1/documents/:id", updateDocument(db))
	payload, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/documents/"+strconv.FormatUint(uint64(id), 10), bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// Saving a document whose OCR dates are Thai text used to fail with
// "วันที่ต้องอยู่ในรูปแบบ YYYY-MM-DD" even when nothing was edited.
func TestSavingThaiDatesIsNeverRejected(t *testing.T) {
	db := testDatabase(t)
	document, ids := createReviewDocument(t, db, map[string]string{
		"issued_date":    "8 กันยายน 2566",
		"effective_date": "ตั้งแต่บัดนี้เป็นต้นไป",
		"expiry_date":    "ไม่ระบุ",
	})
	fields := []map[string]any{}
	for key, id := range ids {
		var field ExtractedField
		db.First(&field, id)
		fields = append(fields, map[string]any{"id": id, "fieldKey": key, "value": field.Value})
	}
	response := patchDocument(t, db, document.ID, map[string]any{"fields": fields})
	if response.Code != http.StatusOK {
		t.Fatalf("save status = %d body = %s", response.Code, response.Body.String())
	}
	var saved Document
	if err := db.First(&saved, document.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.IssuedDate == nil || saved.IssuedDate.Format("2006-01-02") != "2023-09-08" {
		t.Fatalf("issued date = %v", saved.IssuedDate)
	}
	if saved.EffectiveDate == nil || saved.EffectiveDate.Format("2006-01-02") != "2023-09-08" {
		t.Fatalf("'from now' should take effect on the issue date, got %v", saved.EffectiveDate)
	}
	if saved.ExpiryDate != nil {
		t.Fatalf("an unreadable expiry must not invent a date, got %v", saved.ExpiryDate)
	}
	var expiry ExtractedField
	db.First(&expiry, ids["expiry_date"])
	if expiry.Value != "ไม่ระบุ" {
		t.Fatalf("the unreadable text must be kept for the reviewer, got %q", expiry.Value)
	}
}

func TestOverlongTextIsTrimmedInsteadOfFailingTheSave(t *testing.T) {
	db := testDatabase(t)
	document, _ := createReviewDocument(t, db, map[string]string{})
	long := ""
	for len([]rune(long)) < 300 {
		long += "ตำแหน่งยาวมาก"
	}
	response := patchDocument(t, db, document.ID, map[string]any{"fields": []map[string]any{{"fieldKey": "signer_position", "value": long}, {"fieldKey": "order_year_be", "value": "พ.ศ. ๒๕๖๙"}}})
	if response.Code != http.StatusOK {
		t.Fatalf("save status = %d body = %s", response.Code, response.Body.String())
	}
	var saved Document
	db.First(&saved, document.ID)
	if len([]rune(saved.SignerPosition)) > colLimitDocumentSignerPosition {
		t.Fatalf("signer position kept %d characters", len([]rune(saved.SignerPosition)))
	}
	if saved.OrderYearBE != 2569 {
		t.Fatalf("order year = %d", saved.OrderYearBE)
	}
}

func confirmRequest(t *testing.T, db *gorm.DB, id uint) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/documents/:id/confirm", confirmDocument(db))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+strconv.FormatUint(uint64(id), 10)+"/confirm", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestConfirmAsksForADateItCannotRead(t *testing.T) {
	db := testDatabase(t)
	document, _ := createReviewDocument(t, db, map[string]string{"issued_date": "8 กันยายน 2566", "expiry_date": "สิ้นปีงบ"})
	response := confirmRequest(t, db, document.ID)
	if response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte("สิ้นปีงบ")) {
		t.Fatalf("confirm status = %d body = %s", response.Code, response.Body.String())
	}
}
