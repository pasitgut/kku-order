package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// รูปแบบรายงานตามไฟล์ตัวอย่าง: A4 แนวนอน TH Sarabun New 12 หัวรายงานตัวหนา 14
const (
	reportEmptyText = "ไม่มีคำสั่งตามเงื่อนไขนี้"
	reportFont      = "TH Sarabun New"
	reportBodySize  = 12
	reportTitleSize = 14
)

func reportColumnName(index int) string {
	name, _ := excelize.ColumnNumberToName(index + 1)
	return name
}

func renderReportXLSX(report appointmentReport) ([]byte, error) {
	book := excelize.NewFile()
	defer book.Close()
	const sheet = "รายงาน"
	if err := book.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}
	border := []excelize.Border{{Type: "left", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1}, {Type: "bottom", Color: "000000", Style: 1}}
	center := &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true}
	titleStyle, err := book.NewStyle(&excelize.Style{Font: &excelize.Font{Family: reportFont, Size: reportTitleSize, Bold: true}, Alignment: center})
	if err != nil {
		return nil, err
	}
	scopeStyle, err := book.NewStyle(&excelize.Style{Font: &excelize.Font{Family: reportFont, Size: reportBodySize}, Alignment: center})
	if err != nil {
		return nil, err
	}
	headStyle, err := book.NewStyle(&excelize.Style{Font: &excelize.Font{Family: reportFont, Size: reportBodySize, Bold: true}, Alignment: center, Border: border})
	if err != nil {
		return nil, err
	}
	bodyStyle, err := book.NewStyle(&excelize.Style{Font: &excelize.Font{Family: reportFont, Size: reportBodySize}, Alignment: center, Border: border})
	if err != nil {
		return nil, err
	}

	last := reportColumnName(len(reportColumns) - 1)
	for row, text := range []string{report.Title, report.Scope} {
		ref := fmt.Sprintf("A%d", row+1)
		if err := book.MergeCell(sheet, ref, fmt.Sprintf("%s%d", last, row+1)); err != nil {
			return nil, err
		}
		_ = book.SetCellValue(sheet, ref, text)
		style := titleStyle
		if row == 1 {
			style = scopeStyle
		}
		_ = book.SetCellStyle(sheet, ref, fmt.Sprintf("%s%d", last, row+1), style)
	}
	_ = book.SetRowHeight(sheet, 1, 24)
	const headerRow = 3
	for i, column := range reportColumns {
		name := reportColumnName(i)
		_ = book.SetColWidth(sheet, name, name, column.Width)
		_ = book.SetCellValue(sheet, fmt.Sprintf("%s%d", name, headerRow), column.Title)
	}
	_ = book.SetCellStyle(sheet, fmt.Sprintf("A%d", headerRow), fmt.Sprintf("%s%d", last, headerRow), headStyle)
	for r, row := range report.Rows {
		line := headerRow + 1 + r
		for i, value := range row.cells() {
			ref := fmt.Sprintf("%s%d", reportColumnName(i), line)
			if i == 0 {
				_ = book.SetCellInt(sheet, ref, int64(row.No))
				continue
			}
			_ = book.SetCellStr(sheet, ref, value)
		}
		_ = book.SetCellStyle(sheet, fmt.Sprintf("A%d", line), fmt.Sprintf("%s%d", last, line), bodyStyle)
	}
	if len(report.Rows) == 0 {
		line := headerRow + 1
		_ = book.MergeCell(sheet, fmt.Sprintf("A%d", line), fmt.Sprintf("%s%d", last, line))
		_ = book.SetCellStr(sheet, fmt.Sprintf("A%d", line), reportEmptyText)
		_ = book.SetCellStyle(sheet, fmt.Sprintf("A%d", line), fmt.Sprintf("%s%d", last, line), bodyStyle)
	}

	landscape, a4, fitWidth, anyHeight := "landscape", 9, 1, 0
	if err := book.SetPageLayout(sheet, &excelize.PageLayoutOptions{Orientation: &landscape, Size: &a4, FitToWidth: &fitWidth, FitToHeight: &anyHeight}); err != nil {
		return nil, err
	}
	fit := true
	if err := book.SetSheetProps(sheet, &excelize.SheetPropsOptions{FitToPage: &fit}); err != nil {
		return nil, err
	}
	// หัวตารางพิมพ์ซ้ำทุกหน้า
	_ = book.SetDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Titles", RefersTo: fmt.Sprintf("'%s'!$%d:$%d", sheet, headerRow, headerRow), Scope: sheet})
	var buffer bytes.Buffer
	if err := book.Write(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// ---- Word (.docx) เขียน WordprocessingML เองโดยตรง ไม่ต้องพึ่งไลบรารีเพิ่ม ----

const (
	docxPageWidth  = 16838 // A4 แนวนอน หน่วย twip
	docxPageHeight = 11906
	docxMargin     = 720
)

var docxEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

// xmlSafe ตัดอักขระควบคุมที่ XML ไม่รับ (OCR บางครั้งติด form feed มา) ไม่งั้น Word จะบอกว่าไฟล์เสีย
func xmlSafe(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || (r >= 0x20 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) || r >= 0x10000 {
			return r
		}
		return -1
	}, text)
}

func docxEscape(text string) string {
	return docxEscaper.Replace(xmlSafe(text))
}

func docxRun(text string, bold bool, size int) string {
	props := fmt.Sprintf(`<w:rPr><w:rFonts w:ascii="%[1]s" w:hAnsi="%[1]s" w:cs="%[1]s" w:eastAsia="%[1]s"/>`, reportFont)
	if bold {
		props += `<w:b/><w:bCs/>`
	}
	props += fmt.Sprintf(`<w:sz w:val="%[1]d"/><w:szCs w:val="%[1]d"/></w:rPr>`, size*2)
	lines := strings.Split(text, "\n")
	var body strings.Builder
	for i, line := range lines {
		if i > 0 {
			body.WriteString(`<w:br/>`)
		}
		body.WriteString(`<w:t xml:space="preserve">` + docxEscape(line) + `</w:t>`)
	}
	return `<w:r>` + props + body.String() + `</w:r>`
}

func docxParagraph(text string, bold bool, size int) string {
	return `<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:before="0" w:after="0"/></w:pPr>` + docxRun(text, bold, size) + `</w:p>`
}

func docxRow(cells []string, widths []int, header bool) string {
	var row strings.Builder
	row.WriteString(`<w:tr>`)
	if header {
		row.WriteString(`<w:trPr><w:tblHeader/><w:cantSplit/></w:trPr>`)
	} else {
		row.WriteString(`<w:trPr><w:cantSplit/></w:trPr>`)
	}
	for i, cell := range cells {
		row.WriteString(fmt.Sprintf(`<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/><w:vAlign w:val="center"/></w:tcPr>`, widths[i]))
		row.WriteString(docxParagraph(cell, header, reportBodySize))
		row.WriteString(`</w:tc>`)
	}
	row.WriteString(`</w:tr>`)
	return row.String()
}

func renderReportDOCX(report appointmentReport) ([]byte, error) {
	usable := docxPageWidth - 2*docxMargin
	total := 0.0
	for _, column := range reportColumns {
		total += column.Width
	}
	widths := make([]int, len(reportColumns))
	titles := make([]string, len(reportColumns))
	var grid strings.Builder
	for i, column := range reportColumns {
		widths[i] = int(float64(usable) * column.Width / total)
		titles[i] = column.Title
		grid.WriteString(fmt.Sprintf(`<w:gridCol w:w="%d"/>`, widths[i]))
	}
	var body strings.Builder
	body.WriteString(docxParagraph(report.Title, true, reportTitleSize))
	body.WriteString(docxParagraph(report.Scope, false, reportBodySize))
	body.WriteString(`<w:p/>`)
	body.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="` + fmt.Sprint(usable) + `" w:type="dxa"/><w:jc w:val="center"/><w:tblLayout w:type="fixed"/><w:tblBorders>`)
	for _, side := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		body.WriteString(`<w:` + side + ` w:val="single" w:sz="4" w:space="0" w:color="000000"/>`)
	}
	body.WriteString(`</w:tblBorders><w:tblCellMar><w:left w:w="57" w:type="dxa"/><w:right w:w="57" w:type="dxa"/></w:tblCellMar></w:tblPr><w:tblGrid>` + grid.String() + `</w:tblGrid>`)
	body.WriteString(docxRow(titles, widths, true))
	for _, row := range report.Rows {
		body.WriteString(docxRow(row.cells(), widths, false))
	}
	if len(report.Rows) == 0 {
		body.WriteString(fmt.Sprintf(`<w:tr><w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/><w:gridSpan w:val="%d"/></w:tcPr>`, usable, len(reportColumns)) + docxParagraph(reportEmptyText, false, reportBodySize) + `</w:tc></w:tr>`)
	}
	body.WriteString(`</w:tbl>`)
	section := fmt.Sprintf(`<w:sectPr><w:pgSz w:w="%d" w:h="%d" w:orient="landscape"/><w:pgMar w:top="%[3]d" w:right="%[3]d" w:bottom="%[3]d" w:left="%[3]d" w:header="360" w:footer="360" w:gutter="0"/></w:sectPr>`, docxPageWidth, docxPageHeight, docxMargin)
	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body.String() + section + `</w:body></w:document>`
	styles := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:docDefaults><w:rPrDefault><w:rPr>` +
		fmt.Sprintf(`<w:rFonts w:ascii="%[1]s" w:hAnsi="%[1]s" w:cs="%[1]s" w:eastAsia="%[1]s"/><w:sz w:val="%[2]d"/><w:szCs w:val="%[2]d"/><w:lang w:val="th-TH" w:bidi="th-TH"/>`, reportFont, reportBodySize*2) +
		`</w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults></w:styles>`
	parts := []struct{ name, content string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`},
		{"word/document.xml", document},
		{"word/styles.xml", styles},
	}
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for _, part := range parts {
		writer, err := archive.Create(part.name)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write([]byte(part.content)); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// ---- PDF: เขียน HTML แล้วให้ WeasyPrint (report_pdf.py) พิมพ์ เพราะสระและวรรณยุกต์ไทยต้องจัดตำแหน่งด้วย HarfBuzz ----

func htmlCell(text string) string {
	lines := strings.Split(xmlSafe(text), "\n")
	for i, line := range lines {
		lines[i] = html.EscapeString(line)
	}
	return strings.Join(lines, "<br>")
}

func renderReportHTML(report appointmentReport, fontsDir string) string {
	total := 0.0
	for _, column := range reportColumns {
		total += column.Width
	}
	fontURL := func(name string) string {
		return "file://" + filepath.ToSlash(filepath.Join(fontsDir, name))
	}
	var page strings.Builder
	page.WriteString(`<!doctype html><html lang="th"><head><meta charset="utf-8"><style>`)
	page.WriteString(fmt.Sprintf(`@font-face { font-family: "%s"; src: url("%s"); font-weight: normal; }`, reportFont, fontURL("THSarabunNew.ttf")))
	page.WriteString(fmt.Sprintf(`@font-face { font-family: "%s"; src: url("%s"); font-weight: bold; }`, reportFont, fontURL("THSarabunNew Bold.ttf")))
	page.WriteString(`@page { size: A4 landscape; margin: 12mm 10mm 14mm; @bottom-right { content: "หน้า " counter(page) " / " counter(pages); font-family: "TH Sarabun New"; font-size: 11pt; } }`)
	page.WriteString(`body { font-family: "TH Sarabun New"; font-size: 12pt; line-height: 1.15; margin: 0; }`)
	page.WriteString(`h1 { font-size: 14pt; font-weight: bold; text-align: center; margin: 0; } p.scope { text-align: center; margin: 0 0 6pt; }`)
	page.WriteString(`table { width: 100%; border-collapse: collapse; table-layout: fixed; } thead { display: table-header-group; } tr { break-inside: avoid; }`)
	page.WriteString(`th, td { border: 0.6pt solid #000; padding: 2pt 3pt; text-align: center; vertical-align: middle; overflow-wrap: anywhere; } th { font-weight: bold; }`)
	page.WriteString(`</style></head><body>`)
	page.WriteString(`<h1>` + html.EscapeString(report.Title) + `</h1><p class="scope">` + html.EscapeString(report.Scope) + `</p><table><colgroup>`)
	for _, column := range reportColumns {
		page.WriteString(fmt.Sprintf(`<col style="width: %.2f%%">`, column.Width*100/total))
	}
	page.WriteString(`</colgroup><thead><tr>`)
	for _, column := range reportColumns {
		page.WriteString(`<th>` + html.EscapeString(column.Title) + `</th>`)
	}
	page.WriteString(`</tr></thead><tbody>`)
	for _, row := range report.Rows {
		page.WriteString(`<tr>`)
		for _, value := range row.cells() {
			page.WriteString(`<td>` + htmlCell(value) + `</td>`)
		}
		page.WriteString(`</tr>`)
	}
	if len(report.Rows) == 0 {
		page.WriteString(fmt.Sprintf(`<tr><td colspan="%d">%s</td></tr>`, len(reportColumns), reportEmptyText))
	}
	page.WriteString(`</tbody></table></body></html>`)
	return page.String()
}

// pdfSlots จำกัดจำนวน WeasyPrint ที่ทำงานพร้อมกัน แต่ละตัวใช้ CPU และหน่วยความจำมาก
var pdfSlots = make(chan struct{}, 2)

// renderReportPDF ส่ง HTML ให้ report_pdf.py (WeasyPrint) ทาง stdin แล้วอ่าน PDF กลับจาก stdout
// ผู้ใช้ปิดหน้าไประหว่างรอ (ctx ถูกยกเลิก) โปรเซสจะถูกหยุดด้วย
func renderReportPDF(ctx context.Context, report appointmentReport) ([]byte, error) {
	select {
	case pdfSlots <- struct{}{}:
		defer func() { <-pdfSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	fontsDir, err := filepath.Abs(getenv("REPORT_FONTS_DIR", "fonts"))
	if err != nil {
		return nil, err
	}
	script := getenv("REPORT_PDF_SCRIPT", "report_pdf.py")
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, getenv("REPORT_PDF_PYTHON", "python"), script)
	command.Stdin = strings.NewReader(renderReportHTML(report, fontsDir))
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("report_pdf.py: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func withoutContext(render func(appointmentReport) ([]byte, error)) func(context.Context, appointmentReport) ([]byte, error) {
	return func(_ context.Context, report appointmentReport) ([]byte, error) { return render(report) }
}

var reportFormats = map[string]struct {
	contentType string
	render      func(context.Context, appointmentReport) ([]byte, error)
}{
	"pdf":  {"application/pdf", renderReportPDF},
	"docx": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", withoutContext(renderReportDOCX)},
	"xlsx": {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", withoutContext(renderReportXLSX)},
}

// appointmentReportHandler ส่งรายงานตามตัวกรองบนหน้าค้นหา: format=pdf|docx|xlsx, person, from, to, activeOnly=1
func appointmentReportHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		format := strings.ToLower(c.Query("format"))
		output, ok := reportFormats[format]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รูปแบบไฟล์ต้องเป็น PDF, Word หรือ Excel"})
			return
		}
		filter := reportFilter{ActiveOnly: c.Query("activeOnly") == "1"}
		if raw := strings.TrimSpace(c.Query("person")); raw != "" {
			id, err := strconv.ParseUint(raw, 10, 64)
			if err != nil || id == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "รหัสบุคคลไม่ถูกต้อง"})
				return
			}
			filter.PersonID = uint(id)
		}
		filter.From, filter.To = requestedDates(c)
		filter.Name = strings.TrimSpace(c.Query("name"))
		report, err := buildAppointmentReport(db, filter, time.Now())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถรวบรวมข้อมูลรายงานได้"})
			return
		}
		data, err := output.render(c.Request.Context(), report)
		if err != nil {
			log.Printf("render %s report: %v", format, err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "สร้างไฟล์รายงานไม่สำเร็จ ลองอีกครั้ง หรือเลือกรูปแบบอื่น"})
			return
		}
		name := "รายงานคำสั่งแต่งตั้ง"
		if report.PersonName != "" {
			name += "-" + report.PersonName
		}
		name += "." + format
		// RFC 5987: เข้ารหัสทุกอย่างนอกจากตัวอักษร ตัวเลข และ -_.~ (ช่องว่างเป็น %20)
		encoded := strings.ReplaceAll(url.QueryEscape(name), "+", "%20")
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="docflow-report.%s"; filename*=UTF-8''%s`, format, encoded))
		c.Data(http.StatusOK, output.contentType, data)
	}
}
