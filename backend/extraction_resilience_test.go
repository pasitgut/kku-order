package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// เดิมไฟล์ผลสกัดที่หายไปแค่ไฟล์เดียวทำให้ทุกรายการ การค้นหา คำแนะนำ และตัวกรองตอบ 500
// และทำให้รอบยืนยันอัตโนมัติ (auto-confirm sweep) หยุดทำงาน
func TestOneUnreadableExtractionDoesNotBreakOtherRequests(t *testing.T) {
	db := testDatabase(t)
	broken, _ := createReviewDocument(t, db, nil)
	missing := filepath.Join(t.TempDir(), "gone", "extraction.json")
	if err := db.Model(&broken).Updates(map[string]any{"status": "NEEDS_REVIEW", "content_state": "UNSAVED", "extraction_json_path": missing}).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Delete(&Document{}, broken.ID) })

	for _, request := range []struct {
		name, route, path string
	}{
		{name: "list", route: "/documents", path: "/documents?limit=100"},
		{name: "list search", route: "/documents", path: "/documents?q=zzzqa"},
		{name: "list filter", route: "/documents", path: "/documents?orderType=x"},
		{name: "search", route: "/search", path: "/search?q=zzzqa"},
		{name: "suggest", route: "/search/suggest", path: "/search/suggest?q=zzzqa"},
	} {
		handler := listDocuments(db)
		switch request.route {
		case "/search":
			handler = searchDocuments(db)
		case "/search/suggest":
			handler = searchSuggestHandler(db)
		}
		response := performRequest(t, http.MethodGet, request.route, request.path, nil, handler)
		if response.Code != http.StatusOK {
			t.Errorf("%s = %d %s, want 200", request.name, response.Code, response.Body.String())
		}
		if request.name == "list" && !strings.Contains(response.Body.String(), fmt.Sprintf(`"id":%d,`, broken.ID)) {
			t.Errorf("the document with the missing file should still be listed")
		}
	}
	if _, err := confirmReadyPending(db, 0.9); err != nil {
		t.Fatalf("auto-confirm sweep stopped at the unreadable file: %v", err)
	}
}

// คำสั่งที่ยังไม่บันทึกอาจตรงกับการค้นหา OCR แล้วไฟล์หายไปก่อนที่รายการคำแนะนำจะอ่าน
// รายการคำแนะนำต้องข้ามคำสั่งนั้นไป แทนที่จะตอบ 500
func TestSuggestionsSkipAnOrderWhoseFileVanishesMidRequest(t *testing.T) {
	db := testDatabase(t)
	document, _ := createReviewDocument(t, db, nil)
	t.Cleanup(func() { db.Delete(&Document{}, document.ID) })
	word := fmt.Sprintf("หายกลางทาง%d", time.Now().UnixNano())
	path, err := writeExtractionFile(filepath.Join(t.TempDir(), "original.pdf"), extractionFile{DocumentID: document.ID, OCRText: "คำสั่ง " + word})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&document).Updates(map[string]any{"status": "NEEDS_REVIEW", "content_state": "UNSAVED", "extraction_json_path": path}).Error; err != nil {
		t.Fatal(err)
	}

	// connection ที่สองลบไฟล์ทันทีหลังโหลดรายการที่ยังไม่บันทึกที่ตรงกันเสร็จ
	// ซึ่งเป็นจังหวะเดียวกับที่ race จริงจะเกิด
	racing, err := gorm.Open(mysql.Open(os.Getenv("DOCFLOW_TEST_DSN")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	removed := false
	if err := racing.Callback().Query().After("gorm:query").Register("test:vanish", func(tx *gorm.DB) {
		if removed || tx.Statement.Table != "documents" || !strings.Contains(tx.Statement.SQL.String(), "id IN") {
			return
		}
		removed = true
		if err := os.Remove(path); err != nil {
			t.Errorf("remove extraction file: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	response := performRequest(t, http.MethodGet, "/search/suggest", "/search/suggest?q="+word, nil, searchSuggestHandler(racing))
	if !removed {
		t.Fatal("the file was never removed, so the skip was not exercised")
	}
	if response.Code != http.StatusOK {
		t.Fatalf("suggest = %d %s, want 200", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), fmt.Sprintf(`"id":%d,`, document.ID)) {
		t.Fatalf("the order whose file vanished should be left out: %s", response.Body.String())
	}
}
