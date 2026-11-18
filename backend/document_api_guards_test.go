package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// เดิมรหัสเอกสารใน path ส่งถึง GORM เป็นสตริงดิบ ซึ่ง GORM อ่านเป็นเงื่อนไข SQL
// "5 AND 1=1" หรือ "id=5" จึงโหลด (หรือลบ) แถวได้
func TestDocumentRoutesRejectIDsThatAreNotNumbers(t *testing.T) {
	db := testDatabase(t)
	document, _ := createReviewDocument(t, db, nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/d/:id", getDocument(db))
	router.GET("/d/:id/file", documentFile(db))
	router.GET("/d/:id/pages/:page/image", documentPageImage(db))
	router.PATCH("/d/:id", updateDocument(db))
	router.DELETE("/d/:id", deleteDocument(db))
	router.POST("/d/:id/confirm", confirmDocument(db))
	router.POST("/d/:id/return", returnDocument(db))
	router.POST("/d/:id/file", replaceDocumentFile(db, nil))
	router.POST("/d/:id/cancel", cancelDocument(db))
	router.POST("/d/:id/reprocess", reprocessDocumentHandler(gormReprocessStore{db: db}, func(uint) {}))
	router.GET("/d/:id/revisions", listDocumentRevisions(db))
	router.GET("/d/:id/audit", listDocumentAuditLogs(db))

	id := strconv.FormatUint(uint64(document.ID), 10)
	for _, raw := range []string{id + " AND 1=1", "id=" + id, "abc", id + ".5", "0"} {
		escaped := url.PathEscape(raw)
		for _, route := range []struct{ method, path string }{
			{http.MethodGet, "/d/" + escaped},
			{http.MethodGet, "/d/" + escaped + "/file"},
			{http.MethodGet, "/d/" + escaped + "/pages/1/image"},
			{http.MethodPatch, "/d/" + escaped},
			{http.MethodDelete, "/d/" + escaped},
			{http.MethodPost, "/d/" + escaped + "/confirm"},
			{http.MethodPost, "/d/" + escaped + "/return"},
			{http.MethodPost, "/d/" + escaped + "/file"},
			{http.MethodPost, "/d/" + escaped + "/cancel"},
			{http.MethodPost, "/d/" + escaped + "/reprocess"},
			{http.MethodGet, "/d/" + escaped + "/revisions"},
			{http.MethodGet, "/d/" + escaped + "/audit"},
		} {
			request := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"title":"x"}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "รหัสเอกสารไม่ถูกต้อง") {
				t.Errorf("%s %q = %d %s, want 400", route.method, raw, recorder.Code, recorder.Body.String())
			}
		}
	}
	var still Document
	if err := db.First(&still, document.ID).Error; err != nil {
		t.Fatalf("the document must survive the injected ids: %v", err)
	}
}

func multipartUpload(t *testing.T, filename string, content []byte, overwrite bool) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if overwrite {
		_ = writer.WriteField("overwrite", "true")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body, writer.FormDataContentType()
}

func serveUpload(t *testing.T, handler gin.HandlerFunc, route, path, filename string, content []byte, overwrite bool) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST(route, handler)
	body, contentType := multipartUpload(t, filename, content, overwrite)
	request := httptest.NewRequest(http.MethodPost, path, body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func testProcessor() *DocumentProcessor {
	return NewDocumentProcessor(nil, log.New(io.Discard, "", 0), ocrAPIConfig{})
}

// ไฟล์ว่างไม่มีอะไรให้อ่าน แต่เดิมกลับถูกเก็บไว้และตั้งเป็น EXTRACTED
func TestEmptyFilesAreRejected(t *testing.T) {
	db := testDatabase(t)
	t.Setenv("UPLOAD_DIR", t.TempDir())
	filename := fmt.Sprintf("empty-%d.csv", os.Getpid())
	if response := serveUpload(t, uploadDocument(db, testProcessor()), "/documents", "/documents", filename, nil, false); response.Code != http.StatusBadRequest {
		t.Fatalf("upload of an empty file = %d %s, want 400", response.Code, response.Body.String())
	}
	document, _ := createReviewDocument(t, db, nil)
	path := "/documents/" + strconv.FormatUint(uint64(document.ID), 10) + "/file"
	if response := serveUpload(t, replaceDocumentFile(db, testProcessor()), "/documents/:id/file", path, filename, nil, false); response.Code != http.StatusBadRequest {
		t.Fatalf("replacing with an empty file = %d %s, want 400", response.Code, response.Body.String())
	}
}

// เดิมชื่อที่ยาวเกินคอลัมน์ไปล้มใน MySQL แล้วตอบ 500
func TestOverlongDirectoryNamesAreRejectedBeforeSaving(t *testing.T) {
	db := testDatabase(t)
	response := postJSON(t, createDirectoryUser(db), "/directory-users", "/directory-users", map[string]string{"firstName": strings.Repeat("ก", 600), "lastName": "ทดสอบ"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("create = %d %s, want 400", response.Code, response.Body.String())
	}
}

// minimalPDF คือ PDF หนึ่งหน้าที่ pdftoppm แปลงเป็นภาพได้
const minimalPDF = "%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 72 72]>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n"

func pdfDocument(t *testing.T, db *gorm.DB, pageCount int) Document {
	t.Helper()
	document, _ := createReviewDocument(t, db, nil)
	path := filepath.Join(t.TempDir(), "one-page.pdf")
	if err := os.WriteFile(path, []byte(minimalPDF), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&document).Updates(map[string]any{"storage_path": path, "source_type": "pdf", "page_count": pageCount}).Error; err != nil {
		t.Fatal(err)
	}
	return document
}

func pageImage(t *testing.T, db *gorm.DB, id uint, page int) *httptest.ResponseRecorder {
	t.Helper()
	return performRequest(t, http.MethodGet, "/documents/:id/pages/:page/image", fmt.Sprintf("/documents/%d/pages/%d/image", id, page), nil, documentPageImage(db))
}

// การขอหน้าที่เกินหน้าสุดท้ายคือการขอสิ่งที่ไม่มีอยู่ ไม่ใช่ความผิดพลาดของเซิร์ฟเวอร์
func TestPageImageAfterTheLastPageIsNotFound(t *testing.T) {
	if _, err := exec.LookPath(getenv("PDFTOPPM_BIN", "pdftoppm")); err != nil {
		t.Skip("pdftoppm is not installed")
	}
	db := testDatabase(t)
	for _, pageCount := range []int{1, 0} {
		document := pdfDocument(t, db, pageCount)
		response := pageImage(t, db, document.ID, 99)
		var body map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &body)
		if response.Code != http.StatusNotFound || body["error"] != "ไม่พบหน้านี้ในเอกสาร" {
			t.Errorf("page_count %d: page 99 = %d %s, want 404", pageCount, response.Code, response.Body.String())
		}
	}
}
