package main

import (
	"fmt"
	"strings"
	"time"
)

const structuredImportEngine = "structured-import"

var thaiMonthNames = [...]string{"มกราคม", "กุมภาพันธ์", "มีนาคม", "เมษายน", "พฤษภาคม", "มิถุนายน", "กรกฎาคม", "สิงหาคม", "กันยายน", "ตุลาคม", "พฤศจิกายน", "ธันวาคม"}

// thaiDateText เขียนวันที่เป็นข้อความไทย (พ.ศ.) ให้ parseThaiDate อ่านกลับได้
func thaiDateText(t *time.Time) string {
	if t == nil {
		return ""
	}
	return fmt.Sprintf("%d %s %d", t.Day(), thaiMonthNames[t.Month()-1], t.Year()+543)
}

// structuredExtraction แปลงผลของ ocr_worker.py (DOCX/XLSX/CSV) เป็น ExtractionResult
// ใช้ extractCommandMetadata แบบไม่มี profile และตั้งความมั่นใจของทุกค่าเป็นค่าเฉลี่ยของเอกสาร
// เพื่อให้กฎ NEEDS_REVIEW ตาม threshold ยังใช้ได้เหมือนเดิม
// เอนจินเดิม committee = ชื่อเรื่อง จึงตั้ง IssuedBy = title
func structuredExtraction(result workerResult) ExtractionResult {
	pages := result.Pages
	meta := extractCommandMetadata(pages, joinPageText(pages))
	avg := averageOCRConfidence(pages)
	val := func(text string) ExtractedValue {
		c := avg
		return ExtractedValue{Text: text, Confidence: &c}
	}

	out := ExtractionResult{
		Engine: structuredImportEngine,
		Header: ExtractedHeader{
			OrderNo:       val(meta.orderNo),
			Subject:       val(meta.title),
			IssuedBy:      val(meta.title),
			Date:          val(thaiDateText(meta.issuedDate)),
			EffectiveDate: val(thaiDateText(meta.effectiveDate)),
			ExpiryDate:    val(thaiDateText(meta.expiryDate)),
			Signer:        val(meta.signer),
		},
	}
	if meta.additionalRefs != "" {
		out.Header.References = []ExtractedValue{val(meta.additionalRefs)}
	}
	for _, a := range meta.appointments {
		out.Groups = append(out.Groups, ExtractedGroup{
			Department: val(a.Department),
			Duty:       val(a.Responsibilities),
			Members: []ExtractedMember{{
				Name:      val(a.FullName),
				Position:  val(a.Position),
				Role:      val(a.CommitteeRole),
				ExOfficio: strings.TrimSpace(a.FullName) == "" && strings.TrimSpace(a.Position) != "",
			}},
		})
	}
	for _, p := range pages {
		page := ExtractedPage{Page: p.PageNo, Width: p.ImageWidth, Height: p.ImageHeight}
		for _, line := range p.Lines {
			conf := line.Confidence
			page.Lines = append(page.Lines, ExtractedValue{Text: line.Text, Confidence: &conf})
		}
		out.Pages = append(out.Pages, page)
	}
	return out
}

func joinPageText(pages []workerPage) string {
	texts := make([]string, 0, len(pages))
	for _, p := range pages {
		texts = append(texts, p.FullText)
	}
	return strings.TrimSpace(strings.Join(texts, "\n"))
}
