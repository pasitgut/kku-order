package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

func sampleReport() appointmentReport {
	return appointmentReport{Title: reportTitle, Scope: "ของ นางสาวจิราภรณ์ หอมอ่อน", Rows: []reportRow{
		{No: 1, Name: "นางสาวจิราภรณ์ หอมอ่อน", OrderType: "คำสั่งมหาวิทยาลัยขอนแก่น", OrderNo: "1/2569", Subject: "แต่งตั้งคณะกรรมการ R&D <ทดสอบ>", Duty: "เป็นกรรมการและเลขานุการ", Issued: "11 ตุลาคม 2569", Effective: "ทั้งนี้ ตั้งแต่วันที่ 11 ตุลาคม 2569 เป็นต้นไป", Expiry: "-", Note: "ยกเลิก ใช้คำสั่ง 2/2569 แทน"},
		{No: 2, Name: "นางสาวจิราภรณ์ หอมอ่อน", OrderType: "คำสั่งวิทยาลัยการคอมพิวเตอร์", OrderNo: "123/2569", Subject: "แต่งตั้งคณะกรรมการ\x0c", Duty: "ดูแล\nรับสมัคร", Issued: "12 ตุลาคม 2569", Effective: "ทั้งนี้ ตั้งแต่บัดนี้เป็นต้นไป", Expiry: "-", Note: "-"},
	}}
}

func TestExcelReportMatchesTheSampleLayout(t *testing.T) {
	data, err := renderReportXLSX(sampleReport())
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	sheet := book.GetSheetName(0)
	cell := func(ref string) string { value, _ := book.GetCellValue(sheet, ref); return value }
	if cell("A1") != reportTitle || cell("A2") != "ของ นางสาวจิราภรณ์ หอมอ่อน" || cell("A3") != "ลำดับที่" || cell("J3") != "หมายเหตุ" {
		t.Fatalf("title/header = %q / %q / %q / %q", cell("A1"), cell("A2"), cell("A3"), cell("J3"))
	}
	if cell("A4") != "1" || cell("E4") != "แต่งตั้งคณะกรรมการ R&D <ทดสอบ>" || cell("D5") != "123/2569" || cell("F5") != "ดูแล\nรับสมัคร" {
		t.Fatalf("rows = %q %q %q %q", cell("A4"), cell("E4"), cell("D5"), cell("F5"))
	}
	merged, _ := book.GetMergeCells(sheet)
	if len(merged) < 1 || merged[0].GetStartAxis() != "A1" || merged[0].GetEndAxis() != "J1" {
		t.Fatalf("title should span the table, merged = %v", merged)
	}
	style, _ := book.GetCellStyle(sheet, "B4")
	detail, _ := book.GetStyle(style)
	if detail.Font == nil || detail.Font.Family != "TH Sarabun New" || detail.Font.Size != 12 || detail.Alignment == nil || !detail.Alignment.WrapText {
		t.Fatalf("body font = %+v", detail.Font)
	}
	layout, _ := book.GetPageLayout(sheet)
	if layout.Orientation == nil || *layout.Orientation != "landscape" || layout.Size == nil || *layout.Size != 9 {
		t.Fatalf("page should be A4 landscape, got %+v", layout)
	}
}

func TestWordReportIsA4LandscapeInSarabun(t *testing.T) {
	data, err := renderReportDOCX(sampleReport())
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, file := range archive.File {
		reader, _ := file.Open()
		content, _ := io.ReadAll(reader)
		reader.Close()
		files[file.Name] = string(content)
		if strings.HasSuffix(file.Name, ".xml") || strings.HasSuffix(file.Name, ".rels") {
			decoder := xml.NewDecoder(strings.NewReader(string(content)))
			for {
				if _, err := decoder.Token(); err == io.EOF {
					break
				} else if err != nil {
					t.Fatalf("%s is not valid XML: %v", file.Name, err)
				}
			}
		}
	}
	document := files["word/document.xml"]
	if strings.ContainsRune(document, '\x0c') {
		t.Fatal("a form feed from OCR made it into document.xml")
	}
	for _, want := range []string{`w:orient="landscape"`, `w:w="16838"`, `w:h="11906"`, `TH Sarabun New`, `<w:sz w:val="24"/>`, `<w:sz w:val="28"/>`, "ภาระ/หน้าที่ ที่ได้รับมอบหมาย", "R&amp;D &lt;ทดสอบ&gt;", "123/2569", "<w:tblHeader/>", "<w:br/>"} {
		if !strings.Contains(document, want) {
			t.Errorf("document.xml lacks %s", want)
		}
	}
	if files["[Content_Types].xml"] == "" || files["_rels/.rels"] == "" {
		t.Fatal("the package is missing its content types or relationships")
	}
}

func TestEmptyReportsSayThereIsNothingToList(t *testing.T) {
	empty := appointmentReport{Title: reportTitle, Scope: "ของ ทดสอบ"}
	xlsx, _ := renderReportXLSX(empty)
	book, _ := excelize.OpenReader(bytes.NewReader(xlsx))
	if value, _ := book.GetCellValue(book.GetSheetName(0), "A4"); value != reportEmptyText {
		t.Fatalf("excel A4 = %q", value)
	}
	docx, _ := renderReportDOCX(empty)
	archive, _ := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	for _, file := range archive.File {
		if file.Name == "word/document.xml" {
			reader, _ := file.Open()
			content, _ := io.ReadAll(reader)
			if !strings.Contains(string(content), reportEmptyText) || !strings.Contains(string(content), `<w:gridSpan w:val="10"/>`) {
				t.Fatal("word report lacks the empty-table row")
			}
		}
	}
	if !strings.Contains(renderReportHTML(empty, "/fonts"), reportEmptyText) {
		t.Fatal("pdf report lacks the empty-table row")
	}
}

func TestPDFReportHTMLIsA4LandscapeInSarabun(t *testing.T) {
	page := renderReportHTML(sampleReport(), "/app/fonts")
	for _, want := range []string{"size: A4 landscape", "TH Sarabun New", "THSarabunNew.ttf", "THSarabunNew Bold.ttf", "R&amp;D &lt;ทดสอบ&gt;", "<thead>", "ดูแล<br>รับสมัคร"} {
		if !strings.Contains(page, want) {
			t.Errorf("html lacks %q", want)
		}
	}
}

// เทสต์นี้ต้องมี Python ที่ติดตั้ง WeasyPrint: ตั้ง REPORT_PDF_PYTHON ให้ชี้ไปที่ python นั้น
func TestPDFReportRendersWithWeasyPrint(t *testing.T) {
	python := os.Getenv("REPORT_PDF_PYTHON")
	if python == "" {
		t.Skip("set REPORT_PDF_PYTHON to a python that has weasyprint")
	}
	t.Setenv("REPORT_PDF_PYTHON", python)
	data, err := renderReportPDF(t.Context(), sampleReport())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("not a PDF: %q", data[:min(len(data), 20)])
	}
}

func TestReportDownloadNamesTheFileAndRejectsUnknownFormats(t *testing.T) {
	db := testDatabase(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/reports/appointments", appointmentReportHandler(db))
	request := func(query string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/reports/appointments?"+query, nil))
		return recorder
	}
	if response := request("format=csv"); response.Code != http.StatusBadRequest {
		t.Fatalf("unknown format = %d", response.Code)
	}
	if response := request("format=xlsx&person=abc"); response.Code != http.StatusBadRequest {
		t.Fatalf("bad person = %d", response.Code)
	}
	for format, contentType := range map[string]string{
		"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	} {
		response := request("format=" + format + "&from=2025-10-01&to=2026-09-30")
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != contentType {
			t.Fatalf("%s = %d %s", format, response.Code, response.Header().Get("Content-Type"))
		}
		disposition := response.Header().Get("Content-Disposition")
		if !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, "filename*=UTF-8''") || !strings.HasSuffix(disposition, "."+format) {
			t.Fatalf("%s disposition = %q", format, disposition)
		}
	}
}
