package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// failedDocument คือสิ่งที่ MarkFailed ทิ้งไว้ OCR ยอมแพ้แล้ว จึงไม่มีไฟล์ผลสกัด
// และยังไม่มีอะไรถูกบันทึก
func failedDocument(t *testing.T, db *gorm.DB, status string) Document {
	t.Helper()
	document, _ := createReviewDocument(t, db, nil)
	if err := db.Model(&document).Updates(map[string]any{"status": status, "processing_stage": "FAILED", "content_state": "UNSAVED", "extraction_json_path": ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&document, document.ID).Error; err != nil {
		t.Fatal(err)
	}
	return document
}

// เอกสารที่ OCR ล้มเหลวต้องยังพิมพ์ข้อมูลเอง บันทึก และยืนยันได้
func TestDocumentWhoseOCRFailedCanBeEnteredByHand(t *testing.T) {
	db := testDatabase(t)
	for _, status := range []string{"NEEDS_REVIEW", documentReturned} {
		document := failedDocument(t, db, status)
		response := patchDocument(t, db, document.ID, map[string]any{
			"expectedUpdatedAt": document.UpdatedAt,
			"title":             "พิมพ์เอง",
			"fields": []map[string]any{
				{"fieldKey": "order_no", "fieldLabel": "เลขที่คำสั่ง", "value": "12/2569"},
				{"fieldKey": "issued_date", "fieldLabel": "วันที่ออกคำสั่ง", "value": "2026-10-11"},
			},
			"appointments": []map[string]any{{"fullName": "คนนอก ทดสอบพิมพ์เอง", "position": "กรรมการ"}},
		})
		if response.Code != http.StatusOK {
			t.Fatalf("%s: save = %d %s, want 200", status, response.Code, response.Body.String())
		}
		var saved Document
		db.First(&saved, document.ID)
		if saved.ContentState != "DRAFT" || saved.Title != "พิมพ์เอง" || saved.OrderNo != "12/2569" {
			t.Fatalf("%s: saved = state %q title %q order %q", status, saved.ContentState, saved.Title, saved.OrderNo)
		}
		// เมื่อมีคนพิมพ์ข้อมูลคำสั่งแล้ว ก็ไม่ใช่การสแกนที่ล้มเหลวอีกต่อไป
		// รายการต้องเลิกขึ้น "ประมวลผลไม่สำเร็จ" และการสแกนใหม่ต้องกดยืนยันสองขั้น
		if saved.ProcessingStage != "EXTRACTED" {
			t.Fatalf("%s: processing_stage = %q, want EXTRACTED like any saved document", status, saved.ProcessingStage)
		}
		var fields []ExtractedField
		db.Where("document_id = ?", document.ID).Order("id").Find(&fields)
		if len(fields) != 2 || fields[0].FieldKey != "order_no" || fields[0].Value != "12/2569" {
			t.Fatalf("%s: typed fields must be stored as rows, got %+v", status, fields)
		}
		var appointments int64
		db.Model(&Appointment{}).Where("document_id = ?", document.ID).Count(&appointments)
		if appointments != 1 {
			t.Fatalf("%s: appointments = %d", status, appointments)
		}
		confirm := postJSON(t, confirmDocument(db), "/documents/:id/confirm", "/documents/"+strconv.FormatUint(uint64(document.ID), 10)+"/confirm", nil)
		if confirm.Code != http.StatusOK {
			t.Fatalf("%s: confirm = %d %s, want 200", status, confirm.Code, confirm.Body.String())
		}
	}
}

// การพิมพ์เองใช้ได้กับการสแกนที่ล้มเหลวเท่านั้น เอกสารนำเข้าที่ยังรอ OCR ห้ามพิมพ์ทับ
// ไม่งั้นจะถูกตั้งเป็น EXTRACTED และไม่เข้าคิว OCR อีกเลย
func TestHandEntryIsOnlyForFailedScans(t *testing.T) {
	db := testDatabase(t)
	for _, stage := range []string{"IMPORTED", "QUEUED", "EXTRACTING"} {
		document := failedDocument(t, db, "NEEDS_REVIEW")
		t.Cleanup(func() { db.Delete(&Document{}, document.ID) })
		if err := db.Model(&document).Update("processing_stage", stage).Error; err != nil {
			t.Fatal(err)
		}
		db.First(&document, document.ID)
		response := patchDocument(t, db, document.ID, map[string]any{"expectedUpdatedAt": document.UpdatedAt, "title": "พิมพ์เอง", "fields": []any{}, "appointments": []any{}})
		if response.Code != http.StatusConflict {
			t.Fatalf("%s: save = %d %s, want 409", stage, response.Code, response.Body.String())
		}
		var after Document
		db.First(&after, document.ID)
		if after.ProcessingStage != stage || after.ContentState != "UNSAVED" {
			t.Fatalf("%s: document changed to stage %q state %q", stage, after.ProcessingStage, after.ContentState)
		}
	}
}

// ผล OCR ที่มาถึงช้าระหว่างโหลดกับบันทึกต้องไม่ถูกการบันทึกที่พิมพ์เองเขียนทับ
func TestHandEntryIsRejectedOnceAnExtractionArrives(t *testing.T) {
	db := testDatabase(t)
	document := failedDocument(t, db, "NEEDS_REVIEW")
	// เทสต์อื่นไล่จัดการเอกสาร UNSAVED ทุกฉบับที่มีไฟล์ เอกสารนี้จึงต้องไม่ค้างอยู่หลังเทสต์จบ
	t.Cleanup(func() { db.Delete(&Document{}, document.ID) })
	late := filepath.Join(t.TempDir(), "late.json")
	if err := os.WriteFile(late, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&document).Updates(map[string]any{"extraction_json_path": late}).Error; err != nil {
		t.Fatal(err)
	}
	db.First(&document, document.ID)
	response := patchDocument(t, db, document.ID, map[string]any{"expectedUpdatedAt": document.UpdatedAt, "title": "พิมพ์เอง", "fields": []any{}, "appointments": []any{}})
	if response.Code != http.StatusConflict {
		t.Fatalf("save = %d %s, want 409", response.Code, response.Body.String())
	}
}

// ฟิลด์ที่ผู้ตรวจพิมพ์ลงคีย์ที่มีแถวอยู่แล้วต้องอัปเดตแถวเดิม ไม่เพิ่มแถวที่สอง
func TestHandEntryUpdatesAFieldRowThatAlreadyExists(t *testing.T) {
	db := testDatabase(t)
	document := failedDocument(t, db, "NEEDS_REVIEW")
	existing := ExtractedField{DocumentID: document.ID, FieldKey: "order_no", FieldLabel: "เลขที่คำสั่ง", FieldType: "text", Value: "1/2569", BoundingBoxJSON: "null"}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	response := patchDocument(t, db, document.ID, map[string]any{
		"expectedUpdatedAt": document.UpdatedAt,
		"fields":            []map[string]any{{"fieldKey": "order_no", "fieldLabel": "เลขที่คำสั่ง", "value": "2/2569"}},
		"appointments":      []any{},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("save = %d %s, want 200", response.Code, response.Body.String())
	}
	var rows []ExtractedField
	db.Where("document_id = ? AND field_key = ?", document.ID, "order_no").Find(&rows)
	if len(rows) != 1 || rows[0].ID != existing.ID || rows[0].Value != "2/2569" {
		t.Fatalf("order_no rows = %+v, want the existing row %d holding 2/2569", rows, existing.ID)
	}
	var revisions int64
	db.Model(&DocumentRevision{}).Where("document_id = ? AND field_key = ? AND old_value = ? AND new_value = ?", document.ID, "order_no", "1/2569", "2/2569").Count(&revisions)
	if revisions != 1 {
		t.Fatalf("revisions for the overwritten row = %d, want 1", revisions)
	}
}

// หลังบันทึกด้วยการพิมพ์เอง การสแกนใหม่คือการสแกนเอกสารที่บันทึกแล้ว ไม่ใช่การลองใหม่ของเอกสารที่ล้มเหลว
func TestRescanAfterHandEntryIsARescanOfASavedDocument(t *testing.T) {
	db := testDatabase(t)
	document := failedDocument(t, db, "NEEDS_REVIEW")
	response := patchDocument(t, db, document.ID, map[string]any{
		"expectedUpdatedAt": document.UpdatedAt,
		"title":             "พิมพ์เองแล้วสแกนใหม่",
		"fields":            []map[string]any{{"fieldKey": "order_no", "fieldLabel": "เลขที่คำสั่ง", "value": "3/2569"}},
		"appointments":      []any{},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("save = %d %s, want 200", response.Code, response.Body.String())
	}
	if err := (gormReprocessStore{db: db}).RequestReprocess(document.ID, "tester", time.Now()); err != nil {
		t.Fatal(err)
	}
	var log AuditLog
	if err := db.Where("document_id = ? AND action IN ?", document.ID, []string{"DOCUMENT_RESCAN_REQUESTED", "DOCUMENT_REPROCESS_REQUESTED"}).Order("id DESC").First(&log).Error; err != nil {
		t.Fatal(err)
	}
	if log.Action != "DOCUMENT_RESCAN_REQUESTED" {
		t.Fatalf("rescan after hand entry logged %q, want DOCUMENT_RESCAN_REQUESTED", log.Action)
	}
}

// การบันทึกแค่ชื่อเรื่องหรือหมายเหตุการตรวจของการสแกนที่ล้มเหลวยังไม่ถือว่าพิมพ์ข้อมูลคำสั่ง
// เอกสารต้องยังเป็นการสแกนที่ล้มเหลว ไม่งั้นภาพรวมและรายการจะเลิกแสดงว่าล้มเหลว
func TestHandEntryWithoutTypedContentKeepsTheFailedScan(t *testing.T) {
	db := testDatabase(t)
	for name, body := range map[string]map[string]any{
		"title only":        {"title": "ชื่อที่พิมพ์เอง", "fields": []any{}, "appointments": []any{}},
		"review notes only": {"reviewNotes": "สแกนไม่ชัด"},
		"blank fields":      {"fields": []map[string]any{{"fieldKey": "order_no", "fieldLabel": "เลขที่คำสั่ง", "value": "  "}}, "appointments": []any{}},
	} {
		document := failedDocument(t, db, "NEEDS_REVIEW")
		body["expectedUpdatedAt"] = document.UpdatedAt
		response := patchDocument(t, db, document.ID, body)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: save = %d %s, want 200", name, response.Code, response.Body.String())
		}
		var saved Document
		db.First(&saved, document.ID)
		if saved.ContentState != "UNSAVED" || saved.ProcessingStage != "FAILED" {
			t.Fatalf("%s: state %q stage %q, want UNSAVED/FAILED", name, saved.ContentState, saved.ProcessingStage)
		}
		if name == "title only" && saved.Title != "ชื่อที่พิมพ์เอง" {
			t.Fatalf("%s: title = %q, the plain columns must still be saved", name, saved.Title)
		}
		if name == "review notes only" && saved.ReviewNotes != "สแกนไม่ชัด" {
			t.Fatalf("%s: review notes = %q", name, saved.ReviewNotes)
		}
	}
}

// เอกสารที่สแกนล้มเหลวและยังไม่มีใครพิมพ์ข้อมูล ห้ามยืนยัน ไม่งั้นจะได้คำสั่งที่ยืนยันแล้วแต่ไม่มีข้อมูล
func TestEmptyFailedScanCannotBeConfirmed(t *testing.T) {
	db := testDatabase(t)
	document := failedDocument(t, db, "NEEDS_REVIEW")
	t.Cleanup(func() { db.Delete(&Document{}, document.ID) })
	response := patchDocument(t, db, document.ID, map[string]any{
		"expectedUpdatedAt": document.UpdatedAt, "confirm": true, "fields": []any{}, "appointments": []any{},
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("confirm empty failed scan = %d %s, want 400", response.Code, response.Body.String())
	}
	var after Document
	db.First(&after, document.ID)
	if after.Status == "CONFIRMED" || after.ContentState != "UNSAVED" || after.ProcessingStage != "FAILED" {
		t.Fatalf("document changed to status %q state %q stage %q", after.Status, after.ContentState, after.ProcessingStage)
	}
}

// พิมพ์การแต่งตั้งเข้าไปแค่รายการเดียวก็นับเป็นเนื้อหาแล้ว เอกสารจึงกลายเป็นฉบับร่าง
func TestHandEntryWithOnlyAnAppointmentBecomesADraft(t *testing.T) {
	db := testDatabase(t)
	document := failedDocument(t, db, "NEEDS_REVIEW")
	response := patchDocument(t, db, document.ID, map[string]any{
		"expectedUpdatedAt": document.UpdatedAt,
		"fields":            []any{},
		"appointments":      []map[string]any{{"fullName": "คนนอก ทดสอบพิมพ์เอง", "position": "กรรมการ"}},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("save = %d %s, want 200", response.Code, response.Body.String())
	}
	var saved Document
	db.First(&saved, document.ID)
	if saved.ContentState != "DRAFT" || saved.ProcessingStage != "EXTRACTED" {
		t.Fatalf("state %q stage %q, want DRAFT/EXTRACTED", saved.ContentState, saved.ProcessingStage)
	}
}

// การพิมพ์เองตรวจหน้าเว็บที่ข้อมูลเก่า (stale) แบบเดียวกับการบันทึกจากหน้าพรีวิว
func TestHandEntryRequiresTheLoadedVersion(t *testing.T) {
	db := testDatabase(t)
	typed := []map[string]any{{"fieldKey": "order_no", "fieldLabel": "เลขที่คำสั่ง", "value": "12/2569"}}
	for name, expected := range map[string]any{"missing": nil, "stale": time.Now().Add(-time.Hour)} {
		document := failedDocument(t, db, "NEEDS_REVIEW")
		body := map[string]any{"title": "พิมพ์เอง", "fields": typed, "appointments": []any{}}
		if expected != nil {
			body["expectedUpdatedAt"] = expected
		}
		response := patchDocument(t, db, document.ID, body)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "เอกสารเปลี่ยนแล้ว กรุณาโหลดเอกสารใหม่") {
			t.Fatalf("%s expectedUpdatedAt: save = %d %s, want 409 stale-save", name, response.Code, response.Body.String())
		}
		var after Document
		db.First(&after, document.ID)
		if after.Title == "พิมพ์เอง" || after.ContentState != "UNSAVED" {
			t.Fatalf("%s expectedUpdatedAt: document changed: title %q state %q", name, after.Title, after.ContentState)
		}
	}
}
