package main

import (
	"strings"
	"testing"
	"time"
)

func TestStructuredExtractionTagsEngineAndKeepsPageSizes(t *testing.T) {
	result := workerResult{Pages: []workerPage{{
		PageNo: 1, ImageWidth: 1654, ImageHeight: 2339, FullText: "คำสั่งแต่งตั้ง",
		Lines: []workerLine{{Text: "คำสั่งแต่งตั้ง", Confidence: 0.9}},
	}}}
	got := structuredExtraction(result)
	if got.Engine != structuredImportEngine {
		t.Fatalf("Engine = %q", got.Engine)
	}
	if len(got.Pages) != 1 || got.Pages[0].Width != 1654 || len(got.Pages[0].Lines) != 1 {
		t.Fatalf("Pages = %+v", got.Pages)
	}
	if got.Header.Subject.Confidence == nil {
		t.Fatal("structured values must carry the page average confidence so the threshold rule applies")
	}
}

func TestThaiDateTextRoundTripsThroughParser(t *testing.T) {
	d := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	got := parseThaiDate(thaiDateText(&d))
	if got == nil || !got.Equal(d) {
		t.Fatalf("parseThaiDate(%q) = %v, want %v", thaiDateText(&d), got, d)
	}
}

// structuredResult สร้างผลแบบเดียวกับที่ ocr_worker.py ส่งมาจาก CSV/XLSX:
// หนึ่งบรรทัดต่อแถว คอลัมน์คั่นด้วย tab และไม่มี bounding box
func structuredResult(rows ...[]string) workerResult {
	lines := make([]workerLine, 0, len(rows))
	texts := make([]string, 0, len(rows))
	for _, row := range rows {
		text := strings.Join(row, "\t")
		texts = append(texts, text)
		lines = append(lines, workerLine{Text: text, Confidence: 1, BoundingBox: []float64{}})
	}
	return workerResult{Pages: []workerPage{{PageNo: 1, FullText: strings.Join(texts, "\n"), Lines: lines}}}
}

func TestStructuredCSVRowsSplitIntoNamePositionAndRole(t *testing.T) {
	result := structuredResult(
		[]string{"คำสั่งคณะวิทยาการคอมพิวเตอร์ มหาวิทยาลัยขอนแก่น"},
		[]string{"ที่ 124/2568"},
		[]string{"เรื่อง แต่งตั้งคณะกรรมการพัฒนาหลักสูตร QA"},
		[]string{"ลำดับ", "ชื่อ", "ตำแหน่ง", "บทบาท"},
		[]string{"1", "นายสมชาย ใจดี", "รองศาสตราจารย์", "ประธานกรรมการ"},
		[]string{"2", "นางสาวสมหญิง รักเรียน", "ผู้ช่วยศาสตราจารย์", "กรรมการ"},
		[]string{"3", "นายทดสอบ ระบบ", "อาจารย์", "กรรมการและเลขานุการ"},
		[]string{"สั่ง ณ วันที่ 1 มกราคม พ.ศ. 2568"},
	)
	meta := extractCommandMetadata(result.Pages, joinPageText(result.Pages))
	want := []struct{ name, position, role string }{
		{"นายสมชาย ใจดี", "รองศาสตราจารย์", "ประธานกรรมการ"},
		{"นางสาวสมหญิง รักเรียน", "ผู้ช่วยศาสตราจารย์", "กรรมการ"},
		{"นายทดสอบ ระบบ", "อาจารย์", "กรรมการและเลขานุการ"},
	}
	if len(meta.appointments) != len(want) {
		t.Fatalf("appointments = %+v, want %d rows", meta.appointments, len(want))
	}
	for i, w := range want {
		got := meta.appointments[i]
		if got.FullName != w.name || got.Position != w.position || got.CommitteeRole != w.role {
			t.Errorf("row %d = {%q %q %q}, want {%q %q %q}", i+1, got.FullName, got.Position, got.CommitteeRole, w.name, w.position, w.role)
		}
	}
	if meta.title != "แต่งตั้งคณะกรรมการพัฒนาหลักสูตร QA" {
		t.Errorf("title = %q, table text must not leak into it", meta.title)
	}
	if strings.Contains(meta.signerPosition, "บทบาท") || strings.Contains(meta.signerPosition, "\t") {
		t.Errorf("signerPosition = %q, the table header is not a signer position", meta.signerPosition)
	}
}

func TestStructuredRowsWithDottedIndexKeepDepartmentAndSplitColumns(t *testing.T) {
	result := structuredResult(
		[]string{"เรื่อง แต่งตั้งคณะกรรมการคอลัมน์ QA"},
		[]string{"1. คณะกรรมการอำนวยการ"},
		[]string{"1.1", "นายสมปอง ทดสอบคอลัมน์", "ผู้ช่วยศาสตราจารย์", "ประธานกรรมการ"},
		[]string{"1.2", "มาลี คอลัมน์สอง", "อาจารย์", "กรรมการและเลขานุการ"},
	)
	meta := extractCommandMetadata(result.Pages, joinPageText(result.Pages))
	if len(meta.appointments) != 2 {
		t.Fatalf("appointments = %+v", meta.appointments)
	}
	first, second := meta.appointments[0], meta.appointments[1]
	if first.FullName != "นายสมปอง ทดสอบคอลัมน์" || first.Position != "ผู้ช่วยศาสตราจารย์" || first.CommitteeRole != "ประธานกรรมการ" || first.Department != "คณะกรรมการ อำนวยการ" {
		t.Errorf("first = %+v", first)
	}
	if second.FullName != "มาลี คอลัมน์สอง" || second.Position != "อาจารย์" || second.CommitteeRole != "กรรมการและเลขานุการ" {
		t.Errorf("second = %+v", second)
	}
}

func TestOCRRowsStillNeedDottedIndex(t *testing.T) {
	// แถวจาก OCR (มีพิกัด) ที่ขึ้นต้นด้วยเลขจำนวนเต็มเป็นย่อหน้าเนื้อความ ไม่ใช่รายชื่อ
	page := workerPage{PageNo: 1, Lines: []workerLine{
		{Text: "1 นายสมชาย ใจดี เป็นประธานกรรมการ", Confidence: 0.95, BoundingBox: []float64{10, 100, 500, 20}},
	}}
	meta := extractCommandMetadata([]workerPage{page}, page.Lines[0].Text)
	if len(meta.appointments) != 0 {
		t.Fatalf("OCR row without a dotted index must stay a body line, got %+v", meta.appointments)
	}
}

func TestExpiryAndEffectiveDatesNeedTheirOwnLabel(t *testing.T) {
	text := "เรื่อง แต่งตั้งคณะกรรมการ\nสั่ง ณ วันที่ 15 มกราคม พ.ศ. 2568"
	meta := extractCommandMetadata([]workerPage{{PageNo: 1, FullText: text}}, text)
	if meta.issuedDate == nil || formatDate(meta.issuedDate) != "2025-01-15" {
		t.Fatalf("issuedDate = %v", meta.issuedDate)
	}
	if meta.expiryDate != nil {
		t.Errorf("expiryDate = %v, a document without an expiry label has no expiry", meta.expiryDate)
	}
	withExpiry := text + "\nหมดวาระ 31 ธันวาคม 2569"
	meta = extractCommandMetadata([]workerPage{{PageNo: 1, FullText: withExpiry}}, withExpiry)
	if formatDate(meta.expiryDate) != "2026-12-31" {
		t.Errorf("labelled expiry = %v", meta.expiryDate)
	}
}

func TestEffectiveDateDoesNotPickAnUnlabelledDate(t *testing.T) {
	// วันที่แรกในข้อความเป็นวันที่ของเอกสารอ้างอิง ไม่ใช่วันที่มีผล
	text := "ตามหนังสือ 3 มีนาคม 2567\nลงวันที่ 15 มกราคม 2568"
	meta := extractCommandMetadata([]workerPage{{PageNo: 1, FullText: text}}, text)
	if formatDate(meta.effectiveDate) != formatDate(meta.issuedDate) || formatDate(meta.issuedDate) != "2025-01-15" {
		t.Fatalf("effective = %v issued = %v, an unlabelled order takes effect on its issue date", meta.effectiveDate, meta.issuedDate)
	}
}

// แถว ลำดับ | คณะกรรมการ... (มีแค่ลำดับกับหัวกลุ่ม) คือหัวฝ่าย ใช้กฎเดียวกับ parseDepartmentHeader ไม่ใช่รายชื่อคน
func TestTabularSectionHeaderRowIsADepartment(t *testing.T) {
	result := structuredResult(
		[]string{"เรื่อง แต่งตั้งคณะกรรมการจัดงาน"},
		[]string{"1", "คณะกรรมการอำนวยการ"},
		[]string{"1", "นายสมปอง ทดสอบ", "อาจารย์", "ประธานกรรมการ"},
		[]string{"2", "ฝ่ายสถานที่"},
		[]string{"1", "มาลี สอง", "อาจารย์", "กรรมการ"},
		[]string{"2", "นายเดี่ยว คนเดียว"},
	)
	meta := extractCommandMetadata(result.Pages, joinPageText(result.Pages))
	want := []struct{ name, department string }{
		{"นายสมปอง ทดสอบ", "คณะกรรมการ อำนวยการ"},
		{"มาลี สอง", "ฝ่าย สถานที่"},
		{"นายเดี่ยว คนเดียว", "ฝ่าย สถานที่"},
	}
	if len(meta.appointments) != len(want) {
		t.Fatalf("appointments = %+v, want %d people", meta.appointments, len(want))
	}
	for i, w := range want {
		if got := meta.appointments[i]; got.FullName != w.name || got.Department != w.department {
			t.Errorf("row %d = {%q %q}, want {%q %q}", i+1, got.FullName, got.Department, w.name, w.department)
		}
	}
}
