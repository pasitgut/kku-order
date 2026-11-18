package main

import (
	"archive/zip"
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// เทสต์นี้รัน ocr_worker.py จริง จึงต้องมี python3 ในเครื่อง
func structuredWorkerProcessor(t *testing.T) *DocumentProcessor {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to run ocr_worker.py")
	}
	t.Setenv("OCR_PYTHON", python)
	t.Setenv("OCR_SCRIPT", filepath.Join(".", "ocr_worker.py"))
	return &DocumentProcessor{logger: log.New(io.Discard, "", 0)}
}

func TestStructuredImportFailureHidesWorkerTraceback(t *testing.T) {
	p := structuredWorkerProcessor(t)
	path := filepath.Join(t.TempDir(), "bin.csv")
	if err := os.WriteFile(path, []byte{0xff, 0xfe, 0x00, 0x81, 't', 'r', 'i', 'a', 'g', 'e', 0x9f, 0xfa}, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := p.runStructuredImport(context.Background(), path, "csv")
	if err == nil {
		t.Fatal("a non-UTF-8 CSV must fail")
	}
	message := err.Error()
	for _, leak := range []string{"Traceback", "ocr_worker.py", "File \"", "exit status"} {
		if strings.Contains(message, leak) {
			t.Fatalf("error shown to users leaks worker internals (%q): %s", leak, message)
		}
	}
	if !strings.Contains(message, "UTF-8") {
		t.Fatalf("error = %q, want a hint that the CSV must be UTF-8", message)
	}
}

func TestStructuredImportReadsCSVColumnsEndToEnd(t *testing.T) {
	p := structuredWorkerProcessor(t)
	path := filepath.Join(t.TempDir(), "order.csv")
	csv := "เรื่อง แต่งตั้งคณะกรรมการพัฒนาหลักสูตร QA\nลำดับ,ชื่อ,ตำแหน่ง,บทบาท\n1,นายสมชาย ใจดี,รองศาสตราจารย์,ประธานกรรมการ\n"
	if err := os.WriteFile(path, []byte(csv), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := p.runStructuredImport(context.Background(), path, "csv")
	if err != nil {
		t.Fatal(err)
	}
	extraction := structuredExtraction(result)
	if len(extraction.Groups) != 1 {
		t.Fatalf("groups = %+v", extraction.Groups)
	}
	member := extraction.Groups[0].Members[0]
	if member.Name.Text != "นายสมชาย ใจดี" || member.Position.Text != "รองศาสตราจารย์" || member.Role.Text != "ประธานกรรมการ" {
		t.Fatalf("member = %+v", member)
	}
}

// XLSX ไม่เขียนเซลล์ว่างลงไฟล์ worker ต้องวางค่าตามคอลัมน์ในแอตทริบิวต์ r ไม่งั้นคอลัมน์ถัดไปจะเลื่อนมาแทนที่
func TestStructuredImportKeepsXLSXColumnsWhenCellsAreOmitted(t *testing.T) {
	p := structuredWorkerProcessor(t)
	cell := func(ref, text string) string {
		return `<c r="` + ref + `" t="inlineStr"><is><t>` + text + `</t></is></c>`
	}
	sheet := `<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>` +
		`<row r="1">` + cell("A1", "ลำดับ") + cell("B1", "ชื่อ") + cell("C1", "ตำแหน่ง") + cell("D1", "บทบาท") + `</row>` +
		`<row r="2">` + cell("A2", "1") + cell("C2", "คณบดีวิทยาลัยการคอมพิวเตอร์") + cell("D2", "ประธานกรรมการ") + `</row>` +
		`</sheetData></worksheet>`
	path := filepath.Join(t.TempDir(), "order.xlsx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, err := archive.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(sheet)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	result, err := p.runStructuredImport(context.Background(), path, "xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Pages[0].FullText, "1\t\tคณบดีวิทยาลัยการคอมพิวเตอร์\tประธานกรรมการ") {
		t.Fatalf("the omitted name cell must stay an empty column, lines = %q", result.Pages[0].FullText)
	}
}

// ตารางใน DOCX ต้องออกมาเป็นแถวเดียวคั่นคอลัมน์ด้วย tab เหมือน CSV ไม่ใช่หนึ่งบรรทัดต่อเซลล์
func TestStructuredImportReadsDOCXTableRows(t *testing.T) {
	p := structuredWorkerProcessor(t)
	paragraph := func(text string) string { return "<w:p><w:r><w:t>" + text + "</w:t></w:r></w:p>" }
	row := func(cells ...string) string {
		out := "<w:tr>"
		for _, cell := range cells {
			out += "<w:tc>" + paragraph(cell) + "</w:tc>"
		}
		return out + "</w:tr>"
	}
	document := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		paragraph("เรื่อง แต่งตั้งคณะกรรมการ DOCX") +
		"<w:tbl>" + row("ลำดับ", "ชื่อ", "ตำแหน่ง", "บทบาท") + row("1", "นายสมชาย ใจดี", "อาจารย์", "ประธานกรรมการ") + "</w:tbl>" +
		paragraph("สั่ง ณ วันที่ 1 มกราคม พ.ศ. 2568") + `</w:body></w:document>`
	path := filepath.Join(t.TempDir(), "order.docx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(document)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	result, err := p.runStructuredImport(context.Background(), path, "docx")
	if err != nil {
		t.Fatal(err)
	}
	extraction := structuredExtraction(result)
	if len(extraction.Groups) != 1 {
		t.Fatalf("groups = %+v, lines = %q", extraction.Groups, result.Pages[0].FullText)
	}
	member := extraction.Groups[0].Members[0]
	if member.Name.Text != "นายสมชาย ใจดี" || member.Position.Text != "อาจารย์" || member.Role.Text != "ประธานกรรมการ" {
		t.Fatalf("member = %+v", member)
	}
	if extraction.Header.Subject.Text != "แต่งตั้งคณะกรรมการ DOCX" {
		t.Fatalf("subject = %q", extraction.Header.Subject.Text)
	}
}
