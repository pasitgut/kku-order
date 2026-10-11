package main

import (
	"context"
	"errors"
	"strings"
	"time"
)

type extractionJobs interface {
	Claim(id uint, now time.Time) (Document, bool, error)
	SaveExtraction(doc *Document, result ExtractionResult, started time.Time) error
	SaveStructured(doc *Document, result workerResult, started time.Time) error
	MarkRetry(id uint, plan failurePlan) error
	MarkFailed(id uint, failure ExtractError, started time.Time) error
	DueForQueue(now time.Time, startup bool) ([]uint, error)
}

// staleBefore คืนเวลาที่ EXTRACTING/PREPARING เก่ากว่านี้ถือว่าค้าง
// ตอน startup ทุกแถวที่ค้างถือว่าค้าง จึงคืนเวลาในอนาคต
func staleBefore(now time.Time, timeout time.Duration, startup bool) time.Time {
	if startup {
		return now.Add(time.Hour * 24 * 365)
	}
	return now.Add(-(timeout + 2*time.Minute))
}

// Enqueue ส่ง id เข้าคิวแบบไม่บล็อก ถ้าคิวเต็มปล่อยให้ Sweep หยิบภายหลัง
func (p *DocumentProcessor) Enqueue(documentID uint) {
	select {
	case p.queue <- documentID:
	default:
		p.logger.Printf("ocr queue full, document %d will be picked up by sweep", documentID)
	}
}

// Start เปิด worker เท่ากับ ocrConfig.Concurrency และหยุดเมื่อ ctx ยกเลิก
func (p *DocumentProcessor) Start(ctx context.Context) {
	n := p.ocrConfig.Concurrency
	if n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case id := <-p.queue:
					p.processOne(ctx, id)
				}
			}
		}()
	}
}

// Sweep ทำงานทุก 1 นาทีจาก cron
func (p *DocumentProcessor) Sweep() {
	ids, err := p.jobs.DueForQueue(p.now(), false)
	if err != nil {
		p.logger.Printf("sweep failed: %v", err)
		return
	}
	for _, id := range ids {
		p.Enqueue(id)
	}
}

// Recover ทำตอนเริ่มระบบ: งานที่ค้างทุกชนิดถูกหยิบกลับเข้าคิว
func (p *DocumentProcessor) Recover() {
	ids, err := p.jobs.DueForQueue(p.now(), true)
	if err != nil {
		p.logger.Printf("recover failed: %v", err)
		return
	}
	for _, id := range ids {
		p.Enqueue(id)
	}
}

func (p *DocumentProcessor) processOne(ctx context.Context, id uint) {
	now := p.now()
	doc, ok, err := p.jobs.Claim(id, now)
	if err != nil {
		p.logger.Printf("claim document %d: %v", id, err)
		return
	}
	if !ok {
		return
	}
	started := now

	if !strings.EqualFold(doc.SourceType, "pdf") {
		result, err := p.importStructured(ctx, doc.StoragePath, doc.SourceType)
		if err != nil {
			p.markExtractionFailed(doc, ExtractError{Message: "อ่านไฟล์ไม่สำเร็จ: " + err.Error()}, started)
			return
		}
		if err := p.jobs.SaveStructured(&doc, result, started); err != nil {
			p.markExtractionFailed(doc, ExtractError{Message: "บันทึกผลการอ่านไม่สำเร็จ"}, started)
		}
		return
	}

	if err := checkPDFLimits(ctx, doc.StoragePath, p.pageCount); err != nil {
		p.markExtractionFailed(doc, asExtractError(err), started)
		return
	}
	settings, err := p.settings.LoadOCR()
	if err != nil {
		p.markExtractionFailed(doc, ExtractError{Message: "อ่านค่าตั้งค่า OCR ไม่ได้"}, started)
		return
	}
	extractor, err := p.buildExtractor(settings)
	if err != nil {
		p.markExtractionFailed(doc, asExtractError(err), started)
		return
	}

	result, err := extractor.Extract(ctx, doc.StoragePath, doc.OriginalFilename)
	if err != nil {
		plan := planExtractionFailure(err, doc.ExtractionAttempts, doc.ExtractionLastStatus, p.now())
		if plan.Retry {
			if mErr := p.jobs.MarkRetry(id, plan); mErr != nil {
				p.logger.Printf("mark retry %d: %v", id, mErr)
			}
			return
		}
		p.markExtractionFailed(doc, plan.Failure, started)
		return
	}

	if err := p.jobs.SaveExtraction(&doc, result, started); err != nil {
		p.markExtractionFailed(doc, ExtractError{Message: "บันทึกผลการอ่านไม่สำเร็จ"}, started)
	}
}

// markExtractionFailed เป็นจุดเดียวที่ตั้งสถานะ FAILED (งานอีเมลแจ้งแอดมินจะต่อที่นี่)
func (p *DocumentProcessor) markExtractionFailed(doc Document, failure ExtractError, started time.Time) {
	if err := p.jobs.MarkFailed(doc.ID, failure, started); err != nil {
		p.logger.Printf("mark failed %d: %v", doc.ID, err)
	}
}

func asExtractError(err error) ExtractError {
	var ee *ExtractError
	if errors.As(err, &ee) {
		return *ee
	}
	return ExtractError{Message: err.Error()}
}
