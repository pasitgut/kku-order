package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"gorm.io/gorm"
)

func dashboardCounts(t *testing.T, db *gorm.DB) map[string]int {
	t.Helper()
	response := performRequest(t, http.MethodGet, "/dashboard", "/dashboard", nil, dashboardHandler(db))
	if response.Code != http.StatusOK {
		t.Fatalf("dashboard = %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Data map[string]int `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Data
}

// MarkFailed ทิ้งเอกสารไว้ที่สถานะ NEEDS_REVIEW โดย processing_stage เป็น FAILED
// การนับ status = 'FAILED' จึงได้ 0 เสมอ และคำเตือนไม่เคยขึ้น
func TestDashboardCountsDocumentsWhoseOCRFailed(t *testing.T) {
	db := testDatabase(t)
	before := dashboardCounts(t, db)
	failedDocument(t, db, "NEEDS_REVIEW")
	after := dashboardCounts(t, db)
	if after["failed"] != before["failed"]+1 {
		t.Fatalf("failed = %d, want %d", after["failed"], before["failed"]+1)
	}
	if after["needsReview"] != before["needsReview"] {
		t.Fatalf("a failed document must not also count as waiting for confirmation: needsReview %d -> %d", before["needsReview"], after["needsReview"])
	}
	// เมื่อมีคนพิมพ์ข้อมูลเองแล้ว เอกสารก็เป็นฉบับร่างธรรมดาที่รอยืนยัน
	typed := failedDocument(t, db, "NEEDS_REVIEW")
	if err := db.Model(&typed).Update("content_state", "DRAFT").Error; err != nil {
		t.Fatal(err)
	}
	final := dashboardCounts(t, db)
	if final["failed"] != after["failed"] || final["needsReview"] != after["needsReview"]+1 {
		t.Fatalf("typed-in failed document: failed %d -> %d, needsReview %d -> %d", after["failed"], final["failed"], after["needsReview"], final["needsReview"])
	}
}

// หน้าแรกแสดงรายการเอกสารที่นับรวมอยู่ในตัวเลข "รอยืนยัน" ทั้งตัวเลขและรายการต้องมาจากตัวกรองเดียวกัน
// ไม่งั้นการสแกนที่ล้มเหลวจะถูกนับเป็นล้มเหลว แต่ยังโผล่ในรายการรอยืนยัน
func TestAwaitingConfirmationListMatchesTheDashboardCount(t *testing.T) {
	db := testDatabase(t)
	failed := failedDocument(t, db, "NEEDS_REVIEW")
	waiting, _ := createReviewDocument(t, db, nil)
	response := performRequest(t, http.MethodGet, "/documents", "/documents?status="+awaitingConfirmationStatus+"&limit=100", nil, listDocuments(db))
	if response.Code != http.StatusOK {
		t.Fatalf("list = %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Data       []Document `json:"data"`
		Pagination struct {
			Total int `json:"total"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	listed := map[uint]bool{}
	for _, document := range body.Data {
		listed[document.ID] = true
	}
	if listed[failed.ID] || !listed[waiting.ID] {
		t.Fatalf("failed scan listed = %v (want false), waiting document listed = %v (want true)", listed[failed.ID], listed[waiting.ID])
	}
	counts := dashboardCounts(t, db)
	if body.Pagination.Total != counts["review"]+counts["needsReview"] {
		t.Fatalf("list total %d, dashboard review+needsReview %d", body.Pagination.Total, counts["review"]+counts["needsReview"])
	}
}
