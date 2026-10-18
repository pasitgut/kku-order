package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
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
	want := "order_no,committee_name,issued_by,issued_date,effective_date,expiry_date,signer_name,signer_position,additional_references,order_type,issuing_unit,order_year_be"
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

func fieldByKey(t *testing.T, rec extractionRecords, key string) ExtractedField {
	t.Helper()
	for _, f := range rec.Fields {
		if f.FieldKey == key {
			return f
		}
	}
	t.Fatalf("no field row %q", key)
	return ExtractedField{}
}

func assertBox(t *testing.T, raw string, want ...float64) {
	t.Helper()
	var got []float64
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("box %q is not a JSON number array: %v", raw, err)
	}
	if len(got) != len(want) {
		t.Fatalf("box = %s, want %v", raw, want)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("box = %s, want %v", raw, want)
		}
	}
}

func TestBuildExtractionRecordsStoresFirstBoxInPixelsOfItsPage(t *testing.T) {
	r := sampleResult()
	r.Groups[0].Members[0].Name.Boxes = []Box{{Page: 1, X0: 0.1, Y0: 0.5, X1: 0.3, Y1: 0.52}}
	r.Pages[0].Lines = []ExtractedValue{{Text: "บรรทัด", Boxes: []Box{{Page: 1, X0: 0.1, Y0: 0.2, X1: 0.5, Y1: 0.3}}}}
	rec := buildExtractionRecords(r, 0.8)

	assertBox(t, fieldByKey(t, rec, "order_no").BoundingBoxJSON, 661.6, 233.9, 330.8, 46.78)
	if got := fieldByKey(t, rec, "order_no").PageNo; got != 1 {
		t.Fatalf("order_no PageNo = %d, want 1", got)
	}
	assertBox(t, rec.Appointments[0].BoundingBoxJSON, 165.4, 1169.5, 330.8, 46.78)
	if rec.Appointments[0].PageNo != 1 {
		t.Fatalf("appointment PageNo = %d, want 1", rec.Appointments[0].PageNo)
	}
	assertBox(t, rec.OCRLines[0].BoundingBoxJSON, 165.4, 467.8, 661.6, 233.9)
	if rec.OCRPages[0].ImageWidth != 1654 || rec.OCRPages[0].ImageHeight != 2339 {
		t.Fatalf("page size = %dx%d, want 1654x2339", rec.OCRPages[0].ImageWidth, rec.OCRPages[0].ImageHeight)
	}
}

func TestBuildExtractionRecordsUnknownPageSizeKeepsNormalizedBox(t *testing.T) {
	r := sampleResult()
	r.Pages = []ExtractedPage{{Page: 1}}
	rec := buildExtractionRecords(r, 0.8)

	assertBox(t, fieldByKey(t, rec, "order_no").BoundingBoxJSON, 0.4, 0.1, 0.2, 0.02)
	if rec.OCRPages[0].ImageWidth != 1 || rec.OCRPages[0].ImageHeight != 1 {
		t.Fatalf("unknown size should store 1x1, got %+v", rec.OCRPages[0])
	}
}

func TestBuildExtractionRecordsDerivesOrderTypeYearAndCommitteeFields(t *testing.T) {
	r := sampleResult()
	r.Header.IssuedBy.Text = "ประกาศคณะกรรมการบริหาร"
	r.Header.IssuedBy.Boxes = []Box{{Page: 1, X0: 0.1, Y0: 0.2, X1: 0.5, Y1: 0.22}}
	rec := buildExtractionRecords(r, 0.8)

	ot := fieldByKey(t, rec, "order_type")
	if ot.Value != "ประกาศ" || ot.FieldLabel != "ประเภทเอกสาร" {
		t.Fatalf("order_type = %+v", ot)
	}
	assertBox(t, ot.BoundingBoxJSON, 165.4, 467.8, 661.6, 46.78)

	yr := fieldByKey(t, rec, "order_year_be")
	if yr.Value != "2569" || yr.FieldLabel != "ปีที่ออกคำสั่ง (พ.ศ.)" || yr.FieldType != "number" {
		t.Fatalf("order_year_be = %+v", yr)
	}
	assertBox(t, yr.BoundingBoxJSON, 661.6, 233.9, 330.8, 46.78)

	cn := fieldByKey(t, rec, "committee_name")
	if cn.Value != "แต่งตั้งคณะกรรมการ" || cn.FieldLabel != "ชื่อเรื่อง/ชื่อคณะกรรมการ" {
		t.Fatalf("committee_name = %+v", cn)
	}
}

func TestBuildExtractionRecordsDerivedFieldsEmptyWithoutSource(t *testing.T) {
	r := sampleResult()
	r.Header.OrderNo = ExtractedValue{}
	r.Header.IssuedBy = ExtractedValue{}
	rec := buildExtractionRecords(r, 0.8)

	if got := fieldByKey(t, rec, "order_type").Value; got != "" {
		t.Fatalf("order_type = %q, want empty without issued_by", got)
	}
	if got := fieldByKey(t, rec, "order_year_be").Value; got != "" {
		t.Fatalf("order_year_be = %q, want empty without order number", got)
	}
}

func fieldValue(rec extractionRecords, key string) (string, string) {
	for _, f := range rec.Fields {
		if f.FieldKey == key {
			return f.Value, f.SourceText
		}
	}
	return "", ""
}

func TestBuildExtractionRecordsStoresReadableDatesAsISOAndKeepsTheOCRText(t *testing.T) {
	r := sampleResult()
	r.Header.Date = ExtractedValue{Text: "8 กันยายน 2566"}
	rec := buildExtractionRecords(r, 0.8)
	value, source := fieldValue(rec, "issued_date")
	if value != "2023-09-08" || source != "8 กันยายน 2566" {
		t.Fatalf("issued_date field = %q (source %q)", value, source)
	}
}

func TestBuildExtractionRecordsFromNowTakesEffectOnTheIssueDate(t *testing.T) {
	r := sampleResult()
	r.Header.Date = ExtractedValue{Text: "8 กันยายน 2566"}
	r.Header.EffectiveDate = ExtractedValue{Text: "ตั้งแต่บัดนี้เป็นต้นไป"}
	rec := buildExtractionRecords(r, 0.8)
	effective, ok := rec.Updates["effective_date"].(time.Time)
	if !ok || effective.Format("2006-01-02") != "2023-09-08" {
		t.Fatalf("effective_date = %v", rec.Updates["effective_date"])
	}
	if value, _ := fieldValue(rec, "effective_date"); value != "ตั้งแต่บัดนี้เป็นต้นไป" {
		t.Fatalf("the 'from now' wording should stay for the report, got %q", value)
	}
}
