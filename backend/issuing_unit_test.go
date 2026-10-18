package main

import (
	"fmt"
	"testing"
)

func TestOrderIssuerNamesTheUnitTheWayTheReportDoes(t *testing.T) {
	for in, want := range map[string]string{
		"คำสั่งวิทยาลัยการคอมพิวเตอร์ มหาวิทยาลัยขอนแก่น": "คำสั่งวิทยาลัยการคอมพิวเตอร์",
		"วิทยาลัยการคอมพิวเตอร์":                          "คำสั่งวิทยาลัยการคอมพิวเตอร์",
		"คำสั่งมหาวิทยาลัยขอนแก่น":                        "คำสั่งมหาวิทยาลัยขอนแก่น",
		"มหาวิทยาลัยขอนแก่น":                              "คำสั่งมหาวิทยาลัยขอนแก่น",
		" คณะวิศวกรรมศาสตร์  มหาวิทยาลัยขอนแก่น ":         "คำสั่งคณะวิศวกรรมศาสตร์",
		"": "",
		"ประกาศมหาวิทยาลัยขอนแก่น": "ประกาศมหาวิทยาลัยขอนแก่น",
		"แต่งตั้งคณะกรรมการจัดงาน": "",
		"คณะกรรมการบริหาร":         "",
	} {
		if got := orderIssuer(in); got != want {
			t.Errorf("orderIssuer(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractionKeepsTheIssuingUnit(t *testing.T) {
	r := sampleResult()
	r.Header.IssuedBy = ExtractedValue{Text: "คำสั่งวิทยาลัยการคอมพิวเตอร์ มหาวิทยาลัยขอนแก่น"}
	rec := buildExtractionRecords(r, 0.8)
	if rec.Updates["issuing_unit"] != "คำสั่งวิทยาลัยการคอมพิวเตอร์" {
		t.Fatalf("issuing_unit = %v", rec.Updates["issuing_unit"])
	}
	for _, field := range rec.Fields {
		if field.FieldKey == "issuing_unit" {
			if field.Value != "คำสั่งวิทยาลัยการคอมพิวเตอร์" || field.FieldLabel != "หน่วยงานที่ออกคำสั่ง" {
				t.Fatalf("field = %+v", field)
			}
			return
		}
	}
	t.Fatal("no issuing_unit field for the reviewer to correct")
}

func TestReviewerCanCorrectTheIssuingUnitAndOldDocumentsGetOne(t *testing.T) {
	db := testDatabase(t)
	document, ids := createReviewDocument(t, db, map[string]string{"issued_by": "มหาวิทยาลัยขอนแก่น"})
	if err := backfillIssuingUnits(db); err != nil {
		t.Fatal(err)
	}
	var stored Document
	db.First(&stored, document.ID)
	if stored.IssuingUnit != "คำสั่งมหาวิทยาลัยขอนแก่น" {
		t.Fatalf("backfilled unit = %q", stored.IssuingUnit)
	}
	var field ExtractedField
	if err := db.Where("document_id = ? AND field_key = ?", document.ID, "issuing_unit").Take(&field).Error; err != nil {
		t.Fatalf("old documents need an issuing_unit field to edit: %v", err)
	}
	_ = ids
	response := patchDocument(t, db, document.ID, map[string]any{"fields": []map[string]any{{"id": field.ID, "fieldKey": "issuing_unit", "value": "คำสั่งวิทยาลัยการคอมพิวเตอร์"}}})
	if response.Code != 200 {
		t.Fatalf("save = %d %s", response.Code, response.Body.String())
	}
	db.First(&stored, document.ID)
	if stored.IssuingUnit != "คำสั่งวิทยาลัยการคอมพิวเตอร์" {
		t.Fatalf("saved unit = %q", stored.IssuingUnit)
	}
}

func cancelWith(t *testing.T, documentID uint, body map[string]string) (int, string) {
	t.Helper()
	db := testDatabase(t)
	path := fmt.Sprintf("/api/v1/documents/%d/cancel", documentID)
	response := postJSON(t, cancelDocument(db), "/api/v1/documents/:id/cancel", path, body)
	return response.Code, response.Body.String()
}

func TestCancellingCanNameTheOrderThatReplacesIt(t *testing.T) {
	db := testDatabase(t)
	document, _ := createReviewDocument(t, db, map[string]string{})
	if code, body := cancelWith(t, document.ID, map[string]string{}); code != 400 {
		t.Fatalf("a cancellation needs a reason or a replacement, got %d %s", code, body)
	}
	if code, body := cancelWith(t, document.ID, map[string]string{"replacedByOrderNo": " 12/2569 "}); code != 200 {
		t.Fatalf("cancel = %d %s", code, body)
	}
	var stored Document
	db.First(&stored, document.ID)
	if stored.Status != documentCancelled || stored.ReplacedByOrderNo != "12/2569" || stored.CancellationReason != "ใช้คำสั่งเลขที่ 12/2569 แทน" {
		t.Fatalf("stored = %q / %q / %q", stored.Status, stored.ReplacedByOrderNo, stored.CancellationReason)
	}
}
