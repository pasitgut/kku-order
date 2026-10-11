package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func f32(v float32) *float32 { return &v }

func sampleResult() ExtractionResult {
	return ExtractionResult{
		Engine: "thaidoc",
		Header: ExtractedHeader{
			OrderNo:       ExtractedValue{Text: "1234/2569", Boxes: []Box{{Page: 1, X0: 0.4, Y0: 0.1, X1: 0.6, Y1: 0.12}}},
			Subject:       ExtractedValue{Text: "แต่งตั้งคณะกรรมการ"},
			IssuedBy:      ExtractedValue{Text: "คณะกรรมการบริหาร"},
			Date:          ExtractedValue{Text: "1 ตุลาคม 2569"},
			EffectiveDate: ExtractedValue{Text: "ไม่ใช่วันที่"},
			Signer:        ExtractedValue{Text: "นายตัวอย่าง ทดสอบ"},
			References:    []ExtractedValue{{Text: "ที่ ศธ 0001/2569"}},
		},
		Groups: []ExtractedGroup{{
			Department: ExtractedValue{Text: "สำนักงาน"},
			Duty:       ExtractedValue{Text: "กรรมการ"},
			Members: []ExtractedMember{
				{Name: ExtractedValue{Text: "สมชาย ใจดี"}, Position: ExtractedValue{Text: "ผู้อำนวยการ"}, Role: ExtractedValue{Text: "ประธาน"}},
				{Position: ExtractedValue{Text: "เลขานุการ"}, ExOfficio: true},
			},
		}},
		Pages: []ExtractedPage{{Page: 1, Width: 1654, Height: 2339}},
	}
}

func TestBuildExtractionRecordsNilConfidenceNeedsReview(t *testing.T) {
	rec := buildExtractionRecords(sampleResult(), 0.8)
	if !rec.MissingConfidence {
		t.Fatal("MissingConfidence should be true when API gives no confidence")
	}
	status, stage := extractionStatus(rec, 0.8)
	if status != "NEEDS_REVIEW" || stage != "EXTRACTED" {
		t.Fatalf("status/stage = %s/%s", status, stage)
	}
	if rec.Updates["confidence"] != float32(0) {
		t.Fatalf("confidence = %v, want 0 when nothing has a value", rec.Updates["confidence"])
	}
}

func TestBuildExtractionRecordsAverageAndThreshold(t *testing.T) {
	r := sampleResult()
	r.Header.OrderNo.Confidence = f32(0.9)
	r.Header.Subject.Confidence = f32(0.9)
	r.Header.IssuedBy.Confidence = f32(0.9)
	r.Header.Date.Confidence = f32(0.9)
	r.Header.EffectiveDate.Confidence = f32(0.9)
	r.Header.Signer.Confidence = f32(0.9)
	r.Header.References[0].Confidence = f32(0.9)
	r.Groups[0].Members[0].Name.Confidence = f32(0.9)
	r.Groups[0].Members[0].Position.Confidence = f32(0.9)
	r.Groups[0].Members[0].Role.Confidence = f32(0.9)
	r.Groups[0].Members[1].Position.Confidence = f32(0.5)
	rec := buildExtractionRecords(r, 0.8)
	if rec.MissingConfidence {
		t.Fatal("MissingConfidence should be false: every non-empty Header/Member value has confidence (Department and Duty are not counted)")
	}
	if !rec.LowConfidence {
		t.Fatal("LowConfidence should be true when a value is below threshold")
	}
	status, _ := extractionStatus(rec, 0.8)
	if status != "NEEDS_REVIEW" {
		t.Fatalf("status = %s, want NEEDS_REVIEW because of low confidence", status)
	}
}

func TestBuildExtractionRecordsAppointmentsAndExOfficio(t *testing.T) {
	rec := buildExtractionRecords(sampleResult(), 0.8)
	if len(rec.Appointments) != 2 {
		t.Fatalf("appointments = %d, want 2", len(rec.Appointments))
	}
	if rec.Appointments[0].FullName != "สมชาย ใจดี" || rec.Appointments[0].Department != "สำนักงาน" {
		t.Fatalf("first appointment = %+v", rec.Appointments[0])
	}
	if rec.Appointments[1].FullName != "" || rec.Appointments[1].Position != "เลขานุการ" || rec.Appointments[1].NameMatchMethod != "EX_OFFICIO" {
		t.Fatalf("ex-officio appointment = %+v", rec.Appointments[1])
	}
	if got := personLabel(rec.Appointments[1]); got != "เลขานุการ (โดยตำแหน่ง)" {
		t.Fatalf("personLabel = %q", got)
	}
}

func TestBuildExtractionRecordsExOfficioConfidenceComesFromPosition(t *testing.T) {
	r := sampleResult()
	r.Groups[0].Members[1].Position.Confidence = f32(0.95)
	rec := buildExtractionRecords(r, 0.8)
	if got := rec.Appointments[1].Confidence; got != 0.95 {
		t.Fatalf("ex-officio confidence = %v, want the position confidence 0.95", got)
	}
}

func TestBuildExtractionRecordsWritesEveryHeaderFieldEvenWhenEmpty(t *testing.T) {
	rec := buildExtractionRecords(sampleResult(), 0.8)
	keys := make([]string, 0, len(rec.Fields))
	for _, f := range rec.Fields {
		keys = append(keys, f.FieldKey)
	}
	want := "order_no,subject,issued_by,issued_date,effective_date,expiry_date,signer_name,signer_position,additional_references"
	if got := strings.Join(keys, ","); got != want {
		t.Fatalf("field keys = %s, want %s", got, want)
	}
	for _, f := range rec.Fields {
		if (f.FieldKey == "expiry_date" || f.FieldKey == "signer_position") && (f.Value != "" || f.Confidence != 0 || f.PageNo != 0 || f.BoundingBoxJSON != "[]") {
			t.Fatalf("empty field row %s = %+v, want empty value, confidence 0, PageNo 0, box []", f.FieldKey, f)
		}
	}
}

func TestBuildExtractionRecordsEmptyOrderNoStillGetsFieldRow(t *testing.T) {
	r := sampleResult()
	r.Header.OrderNo = ExtractedValue{}
	for _, v := range []*ExtractedValue{&r.Header.Subject, &r.Header.IssuedBy, &r.Header.Date, &r.Header.EffectiveDate, &r.Header.Signer, &r.Header.References[0], &r.Groups[0].Members[0].Name, &r.Groups[0].Members[0].Position, &r.Groups[0].Members[0].Role, &r.Groups[0].Members[1].Position} {
		v.Confidence = f32(0.9)
	}
	rec := buildExtractionRecords(r, 0.8)
	var found bool
	for _, f := range rec.Fields {
		if f.FieldKey == "order_no" {
			found = true
			if f.Value != "" {
				t.Fatalf("order_no value = %q, want empty", f.Value)
			}
		}
	}
	if !found {
		t.Fatal("empty order number must still produce an order_no field row so the reviewer can type it")
	}
	if rec.MissingConfidence {
		t.Fatal("an empty value must not count toward the confidence rule")
	}
}

func TestBuildExtractionRecordsUnparsedDateKeepsRawText(t *testing.T) {
	rec := buildExtractionRecords(sampleResult(), 0.8)
	var found bool
	for _, f := range rec.Fields {
		if strings.Contains(f.Value, "ไม่ใช่วันที่") {
			found = true
		}
	}
	if !found {
		t.Fatal("unparsed effective date should be kept as raw text in a field row")
	}
	if rec.Updates["effective_date"] != nil {
		t.Fatalf("effective_date column = %v, want NULL", rec.Updates["effective_date"])
	}
}

func TestBuildExtractionRecordsCommitteeFromIssuer(t *testing.T) {
	rec := buildExtractionRecords(sampleResult(), 0.8)
	if rec.Updates["committee"] != "คณะกรรมการบริหาร" || rec.Updates["additional_refs"] != "ที่ ศธ 0001/2569" {
		t.Fatalf("updates = %v", rec.Updates)
	}
	if len(rec.References) != 1 || rec.References[0].ReferenceType != "ADDITIONAL_REFERENCE" {
		t.Fatalf("references = %+v", rec.References)
	}
}

func TestBuildExtractionRecordsCreatesOnePageRowPerResultPage(t *testing.T) {
	r := sampleResult()
	r.Pages = append(r.Pages, ExtractedPage{Page: 2}) // ไม่รู้ขนาด
	rec := buildExtractionRecords(r, 0.8)
	if len(rec.OCRPages) != 2 {
		t.Fatalf("OCRPages = %d, want 2", len(rec.OCRPages))
	}
	if rec.OCRPages[1].ImageWidth != 1 || rec.OCRPages[1].ImageHeight != 1 {
		t.Fatalf("unknown page size should default to 1x1, got %+v", rec.OCRPages[1])
	}
}

func TestBuildExtractionRecordsKeepsOCRLinesAndText(t *testing.T) {
	r := sampleResult()
	r.Pages[0].Lines = []ExtractedValue{{Text: "บรรทัด 1"}, {Text: "บรรทัด 2"}}
	rec := buildExtractionRecords(r, 0.8)
	if rec.Updates["ocr_text"] != "บรรทัด 1\nบรรทัด 2" {
		t.Fatalf("ocr_text = %v", rec.Updates["ocr_text"])
	}
	if len(rec.OCRLines) != 2 || rec.OCRLines[1].PageNo != 1 || rec.OCRLines[1].LineIndex != 1 {
		t.Fatalf("OCRLines = %+v", rec.OCRLines)
	}
	if rec.OCRPages[0].RawText != "บรรทัด 1\nบรรทัด 2" {
		t.Fatalf("page raw text = %q", rec.OCRPages[0].RawText)
	}
}

func TestBuildExtractionRecordsEmptyOCRTextWithoutLines(t *testing.T) {
	rec := buildExtractionRecords(sampleResult(), 0.8)
	if rec.Updates["ocr_text"] != "" {
		t.Fatalf("ocr_text = %v, want empty string", rec.Updates["ocr_text"])
	}
}

func TestBuildExtractionRecordsTruncatesOverlongValues(t *testing.T) {
	r := sampleResult()
	long := strings.Repeat("ก", 600)
	r.Groups[0].Members[0].Position.Text = long
	r.Header.Subject.Text = long
	rec := buildExtractionRecords(r, 0.8)

	if got := rec.Appointments[0].Position; utf8.RuneCountInString(got) != 255 {
		t.Fatalf("Position runes = %d, want 255", utf8.RuneCountInString(got))
	}
	if got := rec.Updates["title"].(string); utf8.RuneCountInString(got) != 500 {
		t.Fatalf("title runes = %d, want 500", utf8.RuneCountInString(got))
	}
}

func TestTruncateRunesDoesNotSplitThaiCharacters(t *testing.T) {
	if got := truncateRunes("กขค", 2); got != "กข" {
		t.Fatalf("truncateRunes = %q", got)
	}
	if got := truncateRunes("abc", 10); got != "abc" {
		t.Fatalf("short string changed: %q", got)
	}
}
