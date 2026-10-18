package main

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// ขนาดคอลัมน์ตาม struct ใน main.go (Document, Appointment)
const (
	colLimitAppointmentFullName    = 500
	colLimitAppointmentPosition    = 255
	colLimitAppointmentRole        = 255
	colLimitAppointmentDept        = 500
	colLimitDocumentOrderNo        = 100
	colLimitDocumentTitle          = 500
	colLimitDocumentCommittee      = 500
	colLimitDocumentSignerName     = 500
	colLimitDocumentSignerPosition = 255
	colLimitDocumentOrderType      = 50
)

// extractionRecords คือแถวที่จะเขียนลงฐานข้อมูลจาก ExtractionResult หนึ่งชุด
type extractionRecords struct {
	Fields            []ExtractedField
	Appointments      []Appointment
	OCRPages          []OCRPage
	OCRLines          []OCRLine
	References        []DocumentReference
	Updates           map[string]any
	AverageConfidence float32
	MissingConfidence bool
	LowConfidence     bool
}

// pageSize คือขนาดภาพของหน้าเป็นพิกเซล ใช้แปลงกรอบแบบ normalized เป็นพิกเซล
type pageSize struct {
	width  float64
	height float64
}

// pageSizes แยกตามหมายเลขหน้า หน้าที่ไม่รู้ขนาดใช้ 1x1 (กรอบยังคงเป็นค่า normalized)
type pageSizes map[int]pageSize

func newPageSizes(pages []ExtractedPage) pageSizes {
	sizes := make(pageSizes, len(pages))
	for _, p := range pages {
		sizes[p.Page] = effectivePageSize(p)
	}
	return sizes
}

func effectivePageSize(p ExtractedPage) pageSize {
	if p.Width <= 0 || p.Height <= 0 {
		return pageSize{width: 1, height: 1}
	}
	return pageSize{width: float64(p.Width), height: float64(p.Height)}
}

func (s pageSizes) of(page int) pageSize {
	if size, ok := s[page]; ok {
		return size
	}
	return pageSize{width: 1, height: 1}
}

// firstBoxJSON แปลงกรอบแรกเป็น [left, top, width, height] ตามพิกเซลของหน้าที่กรอบอยู่ (UI อ่านรูปแบบนี้)
func (s pageSizes) firstBoxJSON(boxes []Box) string {
	if len(boxes) == 0 {
		return "[]"
	}
	box := boxes[0]
	size := s.of(box.Page)
	values := []float64{
		roundTwoDecimals(box.X0 * size.width),
		roundTwoDecimals(box.Y0 * size.height),
		roundTwoDecimals((box.X1 - box.X0) * size.width),
		roundTwoDecimals((box.Y1 - box.Y0) * size.height),
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func roundTwoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}

// personLabel ใช้แสดงแถวในรายการ blocker: กรรมการโดยตำแหน่งที่ไม่มีชื่อแสดงเป็นตำแหน่ง
func personLabel(a Appointment) string {
	if strings.TrimSpace(a.FullName) != "" {
		return a.FullName
	}
	return a.Position + " (โดยตำแหน่ง)"
}

// buildExtractionRecords แปลง ExtractionResult เป็นแถวที่จะเขียน ไม่แตะ DB
func buildExtractionRecords(result ExtractionResult, threshold float64) extractionRecords {
	rec := extractionRecords{Updates: map[string]any{}}
	var sum float32
	var count int
	sizes := newPageSizes(result.Pages)

	// นับความมั่นใจเฉพาะค่าที่ไม่ว่าง (spec: Department และ Duty ไม่นับ)
	track := func(v ExtractedValue) {
		if strings.TrimSpace(v.Text) == "" {
			return
		}
		if v.Confidence == nil {
			rec.MissingConfidence = true
			return
		}
		sum += *v.Confidence
		count++
		if float64(*v.Confidence) < threshold {
			rec.LowConfidence = true
		}
	}

	h := result.Header

	refTexts := make([]string, 0, len(h.References))
	for _, ref := range h.References {
		track(ref)
		if strings.TrimSpace(ref.Text) == "" {
			continue
		}
		refTexts = append(refTexts, ref.Text)
		rec.References = append(rec.References, DocumentReference{
			ReferenceType:   "ADDITIONAL_REFERENCE",
			ReferenceText:   ref.Text,
			PageNo:          pageOf(ref),
			BoundingBoxJSON: sizes.firstBoxJSON(ref.Boxes),
		})
	}

	// ทุกคีย์มีแถวเสมอ แม้ค่าว่าง เพื่อให้ผู้ตรวจพิมพ์ค่าที่ OCR พลาดได้ (ข้อยกเว้นของ controller)
	// ค่าว่างไม่นับเข้าเกณฑ์ความมั่นใจ (track ข้ามค่าว่างอยู่แล้ว)
	for _, item := range []struct {
		key, label string
		v          ExtractedValue
	}{
		{"order_no", "เลขที่คำสั่ง", h.OrderNo},
		{"committee_name", "ชื่อเรื่อง/ชื่อคณะกรรมการ", h.Subject},
		{"issued_by", "ออกโดย", h.IssuedBy},
		{"issued_date", "วันที่ออกคำสั่ง", h.Date},
		{"effective_date", "วันที่มีผล", h.EffectiveDate},
		{"expiry_date", "วันที่สิ้นสุด", h.ExpiryDate},
		{"signer_name", "ผู้ลงนาม", h.Signer},
		{"signer_position", "ตำแหน่งผู้ลงนาม", ExtractedValue{}},
	} {
		track(item.v)
		rec.Fields = append(rec.Fields, fieldRow(item.key, item.label, headerFieldValue(item.key, item.v.Text), item.v, sizes))
	}
	// อ้างอิงแต่ละรายการถูกนับความมั่นใจไปแล้วด้านบน แถวรวมนี้จึงไม่นับซ้ำ
	rec.Fields = append(rec.Fields, fieldRow("additional_references", "เอกสารอ้างอิง", strings.Join(refTexts, "\n"), ExtractedValue{Text: strings.Join(refTexts, "\n")}, sizes))

	// คีย์ที่ UI อ่าน (order_type, order_year_be) ได้มาจากค่าในส่วนหัว และใช้กรอบเดียวกับค่าต้นทาง
	orderType := ""
	if strings.TrimSpace(h.IssuedBy.Text) != "" {
		orderType = inferOrderType(h.IssuedBy.Text)
	}
	rec.Fields = append(rec.Fields, fieldRow("order_type", "ประเภทเอกสาร", orderType, h.IssuedBy, sizes))
	orderYear := ""
	if year := findOrderYear(h.OrderNo.Text); year != 0 {
		orderYear = strconv.Itoa(year)
	}
	yearRow := fieldRow("order_year_be", "ปีที่ออกคำสั่ง (พ.ศ.)", orderYear, h.OrderNo, sizes)
	yearRow.FieldType = "number"
	rec.Fields = append(rec.Fields, yearRow)

	rec.Updates["order_no"] = truncateRunes(h.OrderNo.Text, colLimitDocumentOrderNo)
	rec.Updates["title"] = truncateRunes(h.Subject.Text, colLimitDocumentTitle)
	rec.Updates["committee"] = truncateRunes(h.IssuedBy.Text, colLimitDocumentCommittee)
	rec.Updates["signer_name"] = truncateRunes(h.Signer.Text, colLimitDocumentSignerName)
	rec.Updates["additional_refs"] = strings.Join(refTexts, "\n")
	rec.Updates["order_type"] = orderType
	rec.Updates["order_year_be"] = findOrderYear(h.OrderNo.Text)
	rec.Updates["issued_date"] = parseThaiDateOrNil(h.Date.Text)
	rec.Updates["effective_date"] = parseThaiDateOrNil(h.EffectiveDate.Text)
	// "ตั้งแต่บัดนี้เป็นต้นไป" มีผลตั้งแต่วันที่ออกคำสั่ง
	if isFromNow(h.EffectiveDate.Text) {
		rec.Updates["effective_date"] = rec.Updates["issued_date"]
	}
	if expiry, _ := parseDateForField("expiry_date", h.ExpiryDate.Text); expiry != nil {
		rec.Updates["expiry_date"] = *expiry
	} else {
		rec.Updates["expiry_date"] = nil
	}

	for _, g := range result.Groups {
		dept := truncateRunes(g.Department.Text, colLimitAppointmentDept)
		for _, m := range g.Members {
			track(m.Prefix)
			track(m.Name)
			track(m.Position)
			track(m.Role)
			// กรอบของสมาชิกที่ชื่อว่าง (กรรมการโดยตำแหน่ง) ใช้กรอบของตำแหน่งแทน
			box := m.Name
			if len(box.Boxes) == 0 {
				box = m.Position
			}
			// ตำแหน่งวิชาการที่ติดมากับชื่อย้ายไปช่องตำแหน่ง ทุกแถวจึงหน้าตาเหมือนกัน
			fullName, position := nameAndPosition(m.Prefix.Text+" "+m.Name.Text, m.Position.Text)
			rec.Appointments = append(rec.Appointments, Appointment{
				FullName:         truncateRunes(fullName, colLimitAppointmentFullName),
				Position:         truncateRunes(position, colLimitAppointmentPosition),
				CommitteeRole:    truncateRunes(m.Role.Text, colLimitAppointmentRole),
				Department:       dept,
				Responsibilities: g.Duty.Text,
				NameMatchMethod:  matchMethodFor(m.ExOfficio),
				Confidence:       minPresentConfidence(m.Prefix, m.Name, m.Position, m.Role),
				PageNo:           pageOf(box),
				BoundingBoxJSON:  sizes.firstBoxJSON(box.Boxes),
			})
		}
	}

	var allLines []string
	for _, p := range result.Pages {
		pageTexts := make([]string, 0, len(p.Lines))
		for i, line := range p.Lines {
			pageTexts = append(pageTexts, line.Text)
			rec.OCRLines = append(rec.OCRLines, OCRLine{
				PageNo:          p.Page,
				LineIndex:       i,
				Text:            line.Text,
				Confidence:      confidenceOf(line),
				BoundingBoxJSON: sizes.firstBoxJSON(line.Boxes),
			})
		}
		allLines = append(allLines, pageTexts...)
		size := effectivePageSize(p)
		rec.OCRPages = append(rec.OCRPages, OCRPage{
			PageNo:      p.Page,
			ImageWidth:  int(size.width),
			ImageHeight: int(size.height),
			RawText:     strings.Join(pageTexts, "\n"),
			Status:      "COMPLETED",
		})
	}
	rec.Updates["ocr_text"] = strings.Join(allLines, "\n")
	rec.Updates["ocr_provider"] = result.Engine

	if count > 0 {
		rec.AverageConfidence = sum / float32(count)
	}
	rec.Updates["confidence"] = rec.AverageConfidence
	rec.Updates["page_count"] = len(result.Pages)
	rec.Updates["person_count"] = len(rec.Appointments)
	return rec
}

// extractionStatus ตาม spec: ไม่มีค่าความมั่นใจ หรือ needsReview เดิมที่ threshold ให้ NEEDS_REVIEW
func extractionStatus(rec extractionRecords, threshold float64) (string, string) {
	if rec.MissingConfidence || needsReview(rec.AverageConfidence, rec.LowConfidence, threshold) {
		return "NEEDS_REVIEW", "EXTRACTED"
	}
	return "REVIEW", "EXTRACTED"
}

// minPresentConfidence คือค่าต่ำสุดของค่าที่ไม่ว่างและมีความมั่นใจ ใช้เป็นความมั่นใจของแถว
func minPresentConfidence(values ...ExtractedValue) float32 {
	var lowest float32
	found := false
	for _, v := range values {
		if strings.TrimSpace(v.Text) == "" || v.Confidence == nil {
			continue
		}
		if !found || *v.Confidence < lowest {
			lowest = *v.Confidence
			found = true
		}
	}
	return lowest
}

// fieldRow สร้างแถวฟิลด์: value คือค่าที่แสดง, src คือค่าต้นทางที่ให้ความมั่นใจ/กรอบ/ข้อความอ้างอิง
func fieldRow(key, label, value string, src ExtractedValue, sizes pageSizes) ExtractedField {
	return ExtractedField{
		FieldKey:        key,
		FieldLabel:      label,
		Value:           value,
		SourceText:      src.Text,
		Confidence:      confidenceOf(src),
		PageNo:          pageOf(src),
		BoundingBoxJSON: sizes.firstBoxJSON(src.Boxes),
	}
}

func confidenceOf(v ExtractedValue) float32 {
	if v.Confidence == nil {
		return 0
	}
	return *v.Confidence
}

func pageOf(v ExtractedValue) int {
	if len(v.Boxes) > 0 {
		return v.Boxes[0].Page
	}
	return 0
}

func matchMethodFor(exOfficio bool) string {
	if exOfficio {
		return matchMethodExOfficio
	}
	return ""
}

// headerFieldValue เก็บวันที่ที่อ่านออกเป็น YYYY-MM-DD ส่วนข้อความต้นฉบับอยู่ใน SourceText
// วันที่ที่อ่านไม่ออก และคำว่า "บัดนี้" เก็บตามข้อความเดิมไว้ให้ผู้ตรวจและรายงาน
func headerFieldValue(key, text string) string {
	if _, isDate := dateFieldLabels[key]; !isDate || isFromNow(text) {
		return text
	}
	if parsed, ok := parseDateForField(key, text); ok && parsed != nil {
		return parsed.Format("2006-01-02")
	}
	return text
}

func parseThaiDateOrNil(text string) any {
	if t := parseThaiDate(text); t != nil {
		return *t
	}
	return nil
}
