package main

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"sync"
	"testing"
	"time"
)

type fakeJobs struct {
	mu              sync.Mutex
	claimed         map[uint]int
	doc             Document
	claimOK         bool
	saved           int
	savedStructured int
	saveErr         error
	retries         []failurePlan
	failures        []ExtractError
	dueIDs          []uint
	dueStartup      []bool
}

// Claim คืน ok เฉพาะครั้งแรกของเอกสารเดียวกัน เหมือนเงื่อนไข WHERE ของ store จริง
func (f *fakeJobs) Claim(id uint, now time.Time) (Document, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimed == nil {
		f.claimed = map[uint]int{}
	}
	f.claimed[id]++
	if f.claimed[id] > 1 || !f.claimOK {
		return Document{}, false, nil
	}
	return f.doc, true, nil
}

func (f *fakeJobs) SaveExtraction(doc *Document, result ExtractionResult, started time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved++
	return f.saveErr
}

func (f *fakeJobs) SaveStructured(doc *Document, result workerResult, started time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.savedStructured++
	return f.saveErr
}

func (f *fakeJobs) MarkRetry(id uint, plan failurePlan) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retries = append(f.retries, plan)
	return nil
}

func (f *fakeJobs) MarkFailed(id uint, failure ExtractError, started time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures = append(f.failures, failure)
	return nil
}

func (f *fakeJobs) DueForQueue(now time.Time, startup bool) ([]uint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dueStartup = append(f.dueStartup, startup)
	return f.dueIDs, nil
}

type fakeExtractor struct {
	result ExtractionResult
	err    error
	calls  int
}

func (e *fakeExtractor) Name() string { return "fake" }
func (e *fakeExtractor) Extract(ctx context.Context, path, name string) (ExtractionResult, error) {
	e.calls++
	return e.result, e.err
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newTestProcessor(jobs *fakeJobs, ex Extractor, now time.Time) *DocumentProcessor {
	return &DocumentProcessor{
		logger:       log.New(io.Discard, "", 0),
		settings:     &fakeSettingsStore{settings: defaultOCRSettings()},
		jobs:         jobs,
		ocrConfig:    ocrAPIConfig{Timeout: time.Minute, Concurrency: 1},
		newExtractor: func(OCRSettings, ocrAPIConfig) (Extractor, error) { return ex, nil },
		pageCount:    func(context.Context, string) int { return 1 },
		importStructured: func(context.Context, string, string) (workerResult, error) {
			return workerResult{Pages: []workerPage{{PageNo: 1}}}, nil
		},
		now:   func() time.Time { return now },
		queue: make(chan uint, 8),
	}
}

func pdfDoc(t *testing.T) Document {
	t.Helper()
	path := t.TempDir() + "/a.pdf"
	writeFile(t, path, "%PDF-1.4")
	return Document{ID: 7, SourceType: "PDF", OriginalFilename: "a.pdf", StoragePath: path, ProcessingStage: "EXTRACTING"}
}

func TestProcessOneSavesSuccessOnce(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, doc: pdfDoc(t)}
	ex := &fakeExtractor{result: ExtractionResult{Engine: "fake"}}
	p := newTestProcessor(jobs, ex, t0)

	p.processOne(context.Background(), 7)
	p.processOne(context.Background(), 7) // ส่งซ้ำ แต่ claim ครั้งที่สองต้องคืน ok=false

	if jobs.saved != 1 {
		t.Fatalf("saved = %d, want 1", jobs.saved)
	}
	if ex.calls != 1 {
		t.Fatalf("extractor calls = %d, want 1", ex.calls)
	}
}

func TestProcessOneSkipsWhenClaimLost(t *testing.T) {
	jobs := &fakeJobs{claimOK: false}
	ex := &fakeExtractor{}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)
	if ex.calls != 0 {
		t.Fatal("extractor must not run when claim is lost")
	}
}

func TestProcessOne503SchedulesRetryOneMinute(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, doc: pdfDoc(t)}
	jobs.doc.ExtractionAttempts = 1
	ex := &fakeExtractor{err: &ExtractError{Retryable: true, Status: 503, Message: "busy"}}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)

	if len(jobs.retries) != 1 {
		t.Fatalf("retries = %d, want 1", len(jobs.retries))
	}
	if got := jobs.retries[0].NextAttempt; !got.Equal(t0.Add(time.Minute)) {
		t.Fatalf("NextAttempt = %v, want %v", got, t0.Add(time.Minute))
	}
}

func TestProcessOneFifthAttemptFails(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, doc: pdfDoc(t)}
	jobs.doc.ExtractionAttempts = 5
	ex := &fakeExtractor{err: &ExtractError{Retryable: true, Status: 503, Message: "busy"}}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)

	if len(jobs.retries) != 0 || len(jobs.failures) != 1 {
		t.Fatalf("retries=%d failures=%d", len(jobs.retries), len(jobs.failures))
	}
}

func TestProcessOneSecond500FailsPermanently(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, doc: pdfDoc(t)}
	jobs.doc.ExtractionAttempts = 2
	jobs.doc.ExtractionLastStatus = 500
	ex := &fakeExtractor{err: &ExtractError{Retryable: true, Status: 500, Message: "boom"}}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)

	if len(jobs.failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(jobs.failures))
	}
}

func TestProcessOneFirst500Retries(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, doc: pdfDoc(t)}
	jobs.doc.ExtractionAttempts = 1
	ex := &fakeExtractor{err: &ExtractError{Retryable: true, Status: 500, Message: "boom"}}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)

	if len(jobs.retries) != 1 || jobs.retries[0].LastStatus != 500 {
		t.Fatalf("retries = %+v", jobs.retries)
	}
}

func TestProcessOne400FailsImmediately(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, doc: pdfDoc(t)}
	jobs.doc.ExtractionAttempts = 1
	ex := &fakeExtractor{err: &ExtractError{Retryable: false, Status: 400, Message: "bad"}}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)

	if len(jobs.failures) != 1 || len(jobs.retries) != 0 {
		t.Fatalf("failures=%d retries=%d", len(jobs.failures), len(jobs.retries))
	}
}

func TestProcessOneSaveErrorFailsDocument(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, doc: pdfDoc(t), saveErr: errors.New("db")}
	ex := &fakeExtractor{result: ExtractionResult{Engine: "fake"}}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)

	if len(jobs.failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(jobs.failures))
	}
}

func TestProcessOneOversizePDFFailsBeforeCallingAPI(t *testing.T) {
	doc := pdfDoc(t)
	if err := os.Truncate(doc.StoragePath, maxPDFBytes+1); err != nil {
		t.Fatal(err)
	}
	jobs := &fakeJobs{claimOK: true, doc: doc}
	ex := &fakeExtractor{}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)

	if ex.calls != 0 || len(jobs.failures) != 1 || len(jobs.retries) != 0 {
		t.Fatalf("calls=%d failures=%d retries=%d", ex.calls, len(jobs.failures), len(jobs.retries))
	}
}

func TestProcessOneNonPDFUsesStructuredImportWithoutRetry(t *testing.T) {
	doc := pdfDoc(t)
	doc.SourceType = "DOCX"
	jobs := &fakeJobs{claimOK: true, doc: doc}
	ex := &fakeExtractor{}
	p := newTestProcessor(jobs, ex, t0)
	structuredCalls := 0
	p.importStructured = func(context.Context, string, string) (workerResult, error) {
		structuredCalls++
		return workerResult{Pages: []workerPage{{PageNo: 1}}}, nil
	}
	p.processOne(context.Background(), 7)

	if structuredCalls != 1 || jobs.savedStructured != 1 || ex.calls != 0 || len(jobs.retries) != 0 {
		t.Fatalf("structured=%d savedStructured=%d extractorCalls=%d retries=%d", structuredCalls, jobs.savedStructured, ex.calls, len(jobs.retries))
	}
}

func TestSweepEnqueuesDueIDs(t *testing.T) {
	jobs := &fakeJobs{dueIDs: []uint{3, 9}}
	p := newTestProcessor(jobs, &fakeExtractor{}, t0)
	p.Sweep()
	got := []uint{<-p.queue, <-p.queue}
	if got[0] != 3 || got[1] != 9 {
		t.Fatalf("queued = %v", got)
	}
	if jobs.dueStartup[0] {
		t.Fatal("Sweep must call DueForQueue with startup=false")
	}
}

func TestRecoverRequeuesOrphansAtStartup(t *testing.T) {
	jobs := &fakeJobs{dueIDs: []uint{4}}
	p := newTestProcessor(jobs, &fakeExtractor{}, t0)
	p.Recover()
	if got := <-p.queue; got != 4 {
		t.Fatalf("queued = %d, want 4", got)
	}
	if !jobs.dueStartup[0] {
		t.Fatal("Recover must call DueForQueue with startup=true")
	}
}

func TestStaleBeforeUsesTimeoutPlusTwoMinutes(t *testing.T) {
	got := staleBefore(t0, 10*time.Minute, false)
	if !got.Equal(t0.Add(-12 * time.Minute)) {
		t.Fatalf("staleBefore = %v", got)
	}
	if got := staleBefore(t0, 10*time.Minute, true); !got.After(t0) {
		t.Fatalf("startup cutoff should be in the future so every EXTRACTING row is stale, got %v", got)
	}
}
