package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestDocumentDeletedActivityShowsTitleWithoutLink(t *testing.T) {
	details, err := json.Marshal(map[string]string{"title": "คำสั่งทดสอบ", "originalFilename": "คำสั่งทดสอบ.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	item := buildActivityItem(AuditLog{ID: 7, UserID: "u1", Action: "DOCUMENT_DELETED", Details: string(details), CreatedAt: time.Now()}, nil)
	if item.DocumentID != nil {
		t.Fatalf("deleted document must not link to a document, got %v", *item.DocumentID)
	}
	if item.DocumentTitle != "คำสั่งทดสอบ" {
		t.Fatalf("documentTitle = %q, want the title from details", item.DocumentTitle)
	}
	if item.Label != "ลบเอกสาร" {
		t.Fatalf("label = %q", item.Label)
	}
	if item.Details != "คำสั่งทดสอบ.pdf" {
		t.Fatalf("details = %q, want the original file name", item.Details)
	}
}

func TestActivityDropsLinkToMissingDocument(t *testing.T) {
	documentID := uint(42)
	item := buildActivityItem(AuditLog{ID: 8, UserID: "u1", Action: "DOCUMENT_UPLOADED", DocumentID: &documentID}, map[uint]string{})
	if item.DocumentID != nil {
		t.Fatal("a link to a document that no longer exists must be removed")
	}
}

func TestActivityKeepsLinkToExistingDocument(t *testing.T) {
	documentID := uint(42)
	item := buildActivityItem(AuditLog{ID: 9, UserID: "u1", Action: "DOCUMENT_UPLOADED", DocumentID: &documentID}, map[uint]string{42: "เอกสาร"})
	if item.DocumentID == nil || *item.DocumentID != 42 || item.DocumentTitle != "เอกสาร" {
		t.Fatalf("existing document link lost: %+v", item)
	}
}

func TestBaselinePreservesAuditHistoryOnDocumentDeletion(t *testing.T) {
	contents, err := migrationFiles.ReadFile("migrations/" + baselineMigration)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "fk_application_audit_logs_document FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE SET NULL") {
		t.Fatal("audit history must survive document deletion")
	}
}

// TestDeleteDocumentKeepsAuditHistory runs against a real MySQL when
// DOCFLOW_TEST_DSN is set, for example a local container. It is skipped otherwise.
func TestDeleteDocumentKeepsAuditHistory(t *testing.T) {
	dsn := os.Getenv("DOCFLOW_TEST_DSN")
	if dsn == "" {
		t.Skip("set DOCFLOW_TEST_DSN to run the database-backed deletion test")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateDatabase(db); err != nil {
		t.Fatal(err)
	}
	title := "ทดสอบลบ-" + time.Now().Format("150405.000000000")
	storagePath := t.TempDir() + "/" + title + ".pdf"
	if err := os.WriteFile(storagePath, []byte("%PDF"), 0o640); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	document := Document{Title: title, OriginalFilename: title + ".pdf", StoragePath: storagePath, SourceType: "pdf", MimeType: "application/pdf", Status: "NEEDS_REVIEW", ProcessingStage: "IMPORTED", FileHash: title, ImportSource: "UPLOAD", ImportedBy: "tester", QualityProfile: "STANDARD", CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&document).Error; err != nil {
		t.Fatal(err)
	}
	upload := AuditLog{DocumentID: &document.ID, UserID: "tester", Action: "DOCUMENT_UPLOADED", Details: title, CreatedAt: time.Now()}
	if err := db.Create(&upload).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/api/v1/documents/:id", deleteDocument(db))
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/documents/"+strconv.FormatUint(uint64(document.ID), 10), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	var kept AuditLog
	if err := db.First(&kept, upload.ID).Error; err != nil {
		t.Fatalf("upload history row was deleted: %v", err)
	}
	if kept.DocumentID != nil {
		t.Fatalf("upload history should keep its row with document_id NULL, got %d", *kept.DocumentID)
	}
	var deleted AuditLog
	if err := db.Where("action = ? AND details LIKE ?", "DOCUMENT_DELETED", "%"+title+"%").Order("id DESC").First(&deleted).Error; err != nil {
		t.Fatalf("no DOCUMENT_DELETED row: %v", err)
	}
	if deleted.DocumentID != nil || deleted.UserID != "local-user" {
		t.Fatalf("deletion row = %+v", deleted)
	}
}
