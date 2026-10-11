package main

import (
	"errors"
	"time"
)

const maxExtractionAttempts = 5

var extractionBackoff = []time.Duration{
	time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	60 * time.Minute,
}

type failurePlan struct {
	Retry       bool
	NextAttempt time.Time
	LastStatus  int
	Failure     ExtractError
}

// planExtractionFailure ตัดสินว่าหลัง error หนึ่งครั้งควรลองใหม่หรือล้มถาวร
// attempts คือจำนวนครั้งที่เริ่มเรียก API แล้วรวมครั้งนี้; lastStatus คือ status ของความล้มเหลวก่อนหน้า
func planExtractionFailure(err error, attempts int, lastStatus int, now time.Time) failurePlan {
	var ee *ExtractError
	if !errors.As(err, &ee) {
		return failurePlan{Failure: ExtractError{Message: "ประมวลผลไม่สำเร็จ: " + err.Error()}}
	}
	fail := failurePlan{Failure: *ee, LastStatus: ee.Status}
	if !ee.Retryable || attempts >= maxExtractionAttempts {
		return fail
	}
	if ee.Status == 500 && lastStatus == 500 {
		return fail
	}
	if attempts < 1 || attempts > len(extractionBackoff) {
		return fail
	}
	fail.Retry = true
	fail.NextAttempt = now.Add(extractionBackoff[attempts-1])
	return fail
}

// retryResetUpdates คืนค่ารีเซ็ตการลองใหม่ ใช้ทุกเส้นทางที่ให้เอกสารเริ่มประมวลผลใหม่ (เช่น อัปโหลดทับ)
func retryResetUpdates() map[string]any {
	return map[string]any{
		"extraction_attempts":    0,
		"extraction_last_status": 0,
		"next_attempt_at":        nil,
	}
}
