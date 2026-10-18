package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func postJSON(t *testing.T, handler gin.HandlerFunc, route, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST(route, handler)
	payload, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func returnRequest(t *testing.T, db *gorm.DB, id uint, note string) *httptest.ResponseRecorder {
	return postJSON(t, returnDocument(db), "/api/v1/documents/:id/return", "/api/v1/documents/"+strconv.FormatUint(uint64(id), 10)+"/return", map[string]string{"note": note})
}

// A confirmed order can be sent back to be fixed; it leaves the reports until confirmed again.
func TestSendingAConfirmedDocumentBackForReview(t *testing.T) {
	db := testDatabase(t)
	document, _ := createReviewDocument(t, db, map[string]string{"issued_date": "8 กันยายน 2566"})
	if response := confirmRequest(t, db, document.ID); response.Code != http.StatusOK {
		t.Fatalf("confirm = %d %s", response.Code, response.Body.String())
	}
	response := returnRequest(t, db, document.ID, "วันที่ครบวาระผิด")
	if response.Code != http.StatusOK {
		t.Fatalf("return = %d %s", response.Code, response.Body.String())
	}
	var returned Document
	db.First(&returned, document.ID)
	if returned.Status != documentReturned || returned.ConfirmedAt != nil {
		t.Fatalf("returned document = status %q confirmedAt %v", returned.Status, returned.ConfirmedAt)
	}
	var revision DocumentRevision
	if err := db.Where("document_id = ? AND field_key = ?", document.ID, "status").Order("id DESC").First(&revision).Error; err != nil || revision.NewValue != documentReturned {
		t.Fatalf("status revision = %+v, %v", revision, err)
	}

	if response := patchDocument(t, db, document.ID, map[string]any{"reviewNotes": "แก้แล้ว"}); response.Code != http.StatusOK {
		t.Fatalf("save = %d %s", response.Code, response.Body.String())
	}
	if response := patchDocument(t, db, document.ID, map[string]any{"fields": []map[string]any{{"fieldKey": "order_no", "value": "1/2569"}}}); response.Code != http.StatusOK {
		t.Fatalf("save = %d %s", response.Code, response.Body.String())
	}
	db.First(&returned, document.ID)
	if returned.Status != documentReturned {
		t.Fatalf("saving a returned document should keep it returned, got %q", returned.Status)
	}
	if response := confirmRequest(t, db, document.ID); response.Code != http.StatusOK {
		t.Fatalf("re-confirm = %d %s", response.Code, response.Body.String())
	}
}

func TestCancelledDocumentsCannotBeSentBack(t *testing.T) {
	db := testDatabase(t)
	document, _ := createReviewDocument(t, db, map[string]string{})
	db.Model(&Document{}).Where("id = ?", document.ID).Update("status", documentCancelled)
	if response := returnRequest(t, db, document.ID, ""); response.Code != http.StatusConflict {
		t.Fatalf("return cancelled = %d %s", response.Code, response.Body.String())
	}
}

func setFieldConfidence(t *testing.T, db *gorm.DB, documentID uint, key, value string, confidence float32) {
	t.Helper()
	field := ExtractedField{DocumentID: documentID, FieldKey: key, FieldLabel: key, Value: value, SourceText: value, Confidence: confidence, BoundingBoxJSON: "null"}
	if err := db.Create(&field).Error; err != nil {
		t.Fatal(err)
	}
}

func TestAutoConfirmTakesOnlyDocumentsWithNothingToFix(t *testing.T) {
	db := testDatabase(t)
	clean, _ := createReviewDocument(t, db, map[string]string{})
	setFieldConfidence(t, db, clean.ID, "order_no", "1/2569", 0.98)
	setFieldConfidence(t, db, clean.ID, "issued_date", "2026-10-11", 0.97)
	setFieldConfidence(t, db, clean.ID, "effective_date", "ตั้งแต่บัดนี้เป็นต้นไป", 0.95)
	shaky, _ := createReviewDocument(t, db, map[string]string{})
	setFieldConfidence(t, db, shaky.ID, "order_no", "1/2569", 0.98)
	setFieldConfidence(t, db, shaky.ID, "issued_date", "2026-10-11", 0.60)
	unlinked, _ := createReviewDocument(t, db, map[string]string{})
	setFieldConfidence(t, db, unlinked.ID, "order_no", "2/2569", 0.98)
	setFieldConfidence(t, db, unlinked.ID, "issued_date", "2026-10-11", 0.98)
	db.Create(&Appointment{DocumentID: unlinked.ID, FullName: "ดร.วิไล ศรีสุข", BoundingBoxJSON: "null"})

	for _, tc := range []struct {
		name     string
		document Document
		want     bool
	}{{"clean", clean, true}, {"low confidence date", shaky, false}, {"unlinked name", unlinked, false}} {
		got, err := readyToAutoConfirm(db, tc.document.ID, 0.9)
		if err != nil || got != tc.want {
			t.Errorf("%s: readyToAutoConfirm = %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
}

func TestTurningOnAutoConfirmConfirmsTheReadyBacklog(t *testing.T) {
	db := testDatabase(t)
	ready, _ := createReviewDocument(t, db, map[string]string{})
	setFieldConfidence(t, db, ready.ID, "order_no", "3/2569", 0.99)
	setFieldConfidence(t, db, ready.ID, "issued_date", "2026-10-11", 0.99)
	confirmed, err := setAutoConfirm(db, true, "admin@kku.ac.th", 0.9)
	if err != nil || confirmed < 1 {
		t.Fatalf("setAutoConfirm = %d, %v", confirmed, err)
	}
	var document Document
	db.First(&document, ready.ID)
	if document.Status != "CONFIRMED" || document.ConfirmedBy != autoConfirmActor {
		t.Fatalf("document = %q by %q", document.Status, document.ConfirmedBy)
	}
	if enabled, _ := autoConfirmEnabled(db); !enabled {
		t.Fatal("setting should be saved as on")
	}
	if _, err := setAutoConfirm(db, false, "admin@kku.ac.th", 0.9); err != nil {
		t.Fatal(err)
	}
}
