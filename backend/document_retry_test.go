package main

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

func retryable(status int) error {
	return &ExtractError{Retryable: true, Status: status, Message: "temp"}
}

func permanent(status int) error {
	return &ExtractError{Retryable: false, Status: status, Message: "perm"}
}

func TestPlanExtractionFailureBackoffTable(t *testing.T) {
	cases := []struct {
		attempts int
		wantWait time.Duration
	}{
		{1, time.Minute},
		{2, 5 * time.Minute},
		{3, 15 * time.Minute},
		{4, 60 * time.Minute},
	}
	for _, c := range cases {
		plan := planExtractionFailure(retryable(503), c.attempts, 0, t0)
		if !plan.Retry {
			t.Fatalf("attempts=%d: want retry", c.attempts)
		}
		if got := plan.NextAttempt.Sub(t0); got != c.wantWait {
			t.Fatalf("attempts=%d: wait = %v, want %v", c.attempts, got, c.wantWait)
		}
		if plan.LastStatus != 503 {
			t.Fatalf("attempts=%d: LastStatus = %d", c.attempts, plan.LastStatus)
		}
	}
}

func TestPlanExtractionFailureStopsAtFifthAttempt(t *testing.T) {
	plan := planExtractionFailure(retryable(503), maxExtractionAttempts, 0, t0)
	if plan.Retry {
		t.Fatal("5th attempt must fail permanently")
	}
	if plan.Failure.Message != "temp" {
		t.Fatalf("Failure = %+v", plan.Failure)
	}
}

func TestPlanExtractionFailurePermanentErrorFailsImmediately(t *testing.T) {
	plan := planExtractionFailure(permanent(400), 1, 0, t0)
	if plan.Retry {
		t.Fatal("400 must not retry")
	}
}

func TestPlanExtractionFailureFirst500RetriesSecondFails(t *testing.T) {
	first := planExtractionFailure(retryable(500), 1, 0, t0)
	if !first.Retry || first.LastStatus != 500 {
		t.Fatalf("first 500: %+v", first)
	}
	second := planExtractionFailure(retryable(500), 2, first.LastStatus, t0)
	if second.Retry {
		t.Fatal("second consecutive 500 must fail")
	}
}

func TestPlanExtractionFailureNon500ResetsStatusMemory(t *testing.T) {
	plan := planExtractionFailure(retryable(503), 2, 500, t0)
	if !plan.Retry {
		t.Fatal("503 after a 500 should still retry")
	}
	next := planExtractionFailure(retryable(500), 3, plan.LastStatus, t0)
	if !next.Retry {
		t.Fatal("first 500 after a 503 should retry")
	}
}

func TestPlanExtractionFailureNonExtractErrorFails(t *testing.T) {
	plan := planExtractionFailure(errors.New("db down"), 1, 0, t0)
	if plan.Retry {
		t.Fatal("unknown error must not retry")
	}
	if plan.Failure.Message == "" {
		t.Fatal("failure message must be set")
	}
}

func TestRetryResetUpdatesClearsAllRetryColumns(t *testing.T) {
	u := retryResetUpdates()
	if u["extraction_attempts"] != 0 || u["extraction_last_status"] != 0 {
		t.Fatalf("updates = %v", u)
	}
	if v, ok := u["next_attempt_at"]; !ok || v != nil {
		t.Fatalf("next_attempt_at = %v (present=%t), want nil", v, ok)
	}
}

// เทสต์นี้เป็นข้อบังคับ: การอัปโหลดทับต้องรีเซ็ตการลองใหม่ (spec ส่วนที่ 3)
func TestReplacedFileUpdatesResetsRetryState(t *testing.T) {
	u := replacedFileUpdates(replacementFile{Filename: "b.pdf"}, "tester", t0)
	if u["extraction_attempts"] != 0 || u["extraction_last_status"] != 0 {
		t.Fatalf("retry counters not reset: %v", u)
	}
	if v, ok := u["next_attempt_at"]; !ok || v != nil {
		t.Fatalf("next_attempt_at = %v (present=%t), want nil", v, ok)
	}
}

// ตรวจว่า statement ใน migration 0008 อยู่ในรูปแบบที่ runner รองรับ (ไม่ต้องใช้ DB)
func TestMigration0008StatementsUseSupportedForms(t *testing.T) {
	raw, err := os.ReadFile("migrations/0008_extraction_retry.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	count := 0
	for _, statement := range strings.Split(string(raw), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		count++
		if !addColumnIfMissingPattern.MatchString(statement) && !addKeyPattern.MatchString(statement) {
			t.Fatalf("statement not supported by migration runner: %s", statement)
		}
	}
	if count != 4 {
		t.Fatalf("statements = %d, want 4", count)
	}
}
