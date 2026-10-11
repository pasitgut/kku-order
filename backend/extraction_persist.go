package main

import (
	"fmt"
	"strings"
)

// ขนาดคอลัมน์ตาม struct ใน main.go (Document, Appointment)
const (
	colLimitAppointmentFullName = 500
	colLimitAppointmentPosition = 255
	colLimitAppointmentRole     = 255
	colLimitAppointmentDept     = 500
	colLimitDocumentOrderNo     = 100
	colLimitDocumentTitle       = 500
	colLimitDocumentCommittee   = 500
	colLimitDocumentSignerName  = 500
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
	for _, item := range []struct {
		key, label string
		v          ExtractedValue
	}{
		{"order_no", "เลขที่คำสั่ง", h.OrderNo},
		{"subject", "เรื่อง", h.Subject},
		{"issued_by", "ออกโดย", h.IssuedBy},
		{"issued_date", "วันที่ออกคำสั่ง", h.Date},
		{"effective_date", "วันที่มีผล", h.EffectiveDate},
		{"expiry_date", "วันที่สิ้นสุด", h.ExpiryDate},
		{"signer_name", "ผู้ลงนาม", h.Signer},
	} {
		track(item.v)
		if strings.TrimSpace(item.v.Text) == "" {
			continue
		}
		rec.Fields = append(rec.Fields, ExtractedField{
			FieldKey:        item.key,
			FieldLabel:      item.label,
			Value:           item.v.Text,
			SourceText:      item.v.Text,
			Confidence:      confidenceOf(item.v),
			PageNo:          pageOf(item.v),
			BoundingBoxJSON: firstBoxJSON(item.v.Boxes),
		})
	}

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
			BoundingBoxJSON: firstBoxJSON(ref.Boxes),
		})
	}

	rec.Updates["order_no"] = truncateRunes(h.OrderNo.Text, colLimitDocumentOrderNo)
	rec.Updates["title"] = truncateRunes(h.Subject.Text, colLimitDocumentTitle)
	rec.Updates["committee"] = truncateRunes(h.IssuedBy.Text, colLimitDocumentCommittee)
	rec.Updates["signer_name"] = truncateRunes(h.Signer.Text, colLimitDocumentSignerName)
	rec.Updates["additional_refs"] = strings.Join(refTexts, "\n")
	rec.Updates["order_type"] = inferOrderType(h.IssuedBy.Text)
	rec.Updates["order_year_be"] = findOrderYear(h.OrderNo.Text)
	rec.Updates["issued_date"] = parseThaiDateOrNil(h.Date.Text)
	rec.Updates["effective_date"] = parseThaiDateOrNil(h.EffectiveDate.Text)
	rec.Updates["expiry_date"] = parseThaiDateOrNil(h.ExpiryDate.Text)

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
			rec.Appointments = append(rec.Appointments, Appointment{
				FullName:         truncateRunes(m.Prefix.Text+m.Name.Text, colLimitAppointmentFullName),
				Position:         truncateRunes(m.Position.Text, colLimitAppointmentPosition),
				CommitteeRole:    truncateRunes(m.Role.Text, colLimitAppointmentRole),
				Department:       dept,
				Responsibilities: g.Duty.Text,
				NameMatchMethod:  matchMethodFor(m.ExOfficio),
				Confidence:       minPresentConfidence(m.Prefix, m.Name, m.Position, m.Role),
				PageNo:           pageOf(box),
				BoundingBoxJSON:  firstBoxJSON(box.Boxes),
			})
		}
	}

	var allLines []string
	for _, p := range result.Pages {
		width, height := p.Width, p.Height
		if width <= 0 || height <= 0 {
			width, height = 1, 1
		}
		pageTexts := make([]string, 0, len(p.Lines))
		for i, line := range p.Lines {
			pageTexts = append(pageTexts, line.Text)
			rec.OCRLines = append(rec.OCRLines, OCRLine{
				PageNo:          p.Page,
				LineIndex:       i,
				Text:            line.Text,
				Confidence:      confidenceOf(line),
				BoundingBoxJSON: boxesJSON(line.Boxes),
			})
		}
		allLines = append(allLines, pageTexts...)
		rec.OCRPages = append(rec.OCRPages, OCRPage{
			PageNo:      p.Page,
			ImageWidth:  width,
			ImageHeight: height,
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
		return "EX_OFFICIO"
	}
	return ""
}

func parseThaiDateOrNil(text string) any {
	if t := parseThaiDate(text); t != nil {
		return *t
	}
	return nil
}

func boxesJSON(boxes []Box) string {
	parts := make([]string, 0, len(boxes))
	for _, b := range boxes {
		parts = append(parts, fmt.Sprintf(`{"page":%d,"x0":%g,"y0":%g,"x1":%g,"y1":%g}`, b.Page, b.X0, b.Y0, b.X1, b.Y1))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// firstBoxJSON เก็บเฉพาะกรอบแรกของค่า ตาม spec
func firstBoxJSON(boxes []Box) string {
	if len(boxes) == 0 {
		return "[]"
	}
	return boxesJSON(boxes[:1])
}
