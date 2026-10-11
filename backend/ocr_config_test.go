package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func fakeEnv(values map[string]string) func(string) string {
	return func(k string) string { return values[k] }
}

func fixedPageCount(n int) pageCounter {
	return func(context.Context, string) int { return n }
}

func TestLoadOCRAPIConfigDefaults(t *testing.T) {
	cfg := loadOCRAPIConfig(fakeEnv(nil), func(string) ([]byte, error) { return nil, errors.New("none") })
	if cfg.Timeout != 10*time.Minute || cfg.Concurrency != 1 || cfg.APIKey != "" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadOCRAPIConfigReadsKeyFileAndClampsConcurrency(t *testing.T) {
	env := map[string]string{
		"OCR_API_KEY_FILE":    "/run/secrets/ocr",
		"OCR_API_TIMEOUT":     "3m",
		"OCR_API_CONCURRENCY": "99",
	}
	cfg := loadOCRAPIConfig(fakeEnv(env), func(p string) ([]byte, error) {
		if p != "/run/secrets/ocr" {
			return nil, errors.New("unexpected path")
		}
		return []byte("  key-from-file\n"), nil
	})
	if cfg.APIKey != "key-from-file" || cfg.Timeout != 3*time.Minute || cfg.Concurrency != 8 {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestOCRBaseURLPrefersSettingsThenDefault(t *testing.T) {
	if got := ocrBaseURL(" https://a.example/ ", "https://b.example"); got != "https://a.example" {
		t.Fatalf("settings url = %q", got)
	}
	if got := ocrBaseURL("", "https://b.example/"); got != "https://b.example" {
		t.Fatalf("default url = %q", got)
	}
}

func TestNewExtractorBuildsEndpointPerEngine(t *testing.T) {
	cfg := ocrAPIConfig{DefaultURL: "https://ocr.example", Timeout: time.Minute, Concurrency: 1}
	ex, err := newExtractor(OCRSettings{Engine: "thaidoc"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := ex.(*httpExtractor)
	if h.endpoint != "https://ocr.example/api/v1/extract-json" || h.engine != "thaidoc" {
		t.Fatalf("thaidoc endpoint = %q", h.endpoint)
	}
	ex, err = newExtractor(OCRSettings{Engine: "docflow-contract", APIURL: "https://x.example/extract"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if ex.(*httpExtractor).endpoint != "https://x.example/extract" {
		t.Fatalf("contract endpoint = %q", ex.(*httpExtractor).endpoint)
	}
}

func TestNewExtractorRejectsMissingURLAndUnknownEngine(t *testing.T) {
	if _, err := newExtractor(OCRSettings{Engine: "thaidoc"}, ocrAPIConfig{}); err == nil {
		t.Fatal("expected error when no URL is configured")
	}
	if _, err := newExtractor(OCRSettings{Engine: "nope", APIURL: "https://x"}, ocrAPIConfig{}); err == nil {
		t.Fatal("expected error for unknown engine")
	}
}

func TestCheckPDFLimitsRejectsMissingFile(t *testing.T) {
	err := checkPDFLimits(context.Background(), "/nonexistent/file.pdf", fixedPageCount(1))
	var ee *ExtractError
	if !errors.As(err, &ee) || ee.Retryable {
		t.Fatalf("err = %#v, want permanent ExtractError", err)
	}
}

func TestCheckPDFLimitsRejectsOversizeFile(t *testing.T) {
	path := writeTempPDF(t)
	if err := os.Truncate(path, maxPDFBytes+1); err != nil {
		t.Fatal(err)
	}
	err := checkPDFLimits(context.Background(), path, fixedPageCount(1))
	var ee *ExtractError
	if !errors.As(err, &ee) || ee.Retryable || ee.Status != 413 {
		t.Fatalf("err = %#v, want permanent 413", err)
	}
}

func TestCheckPDFLimitsRejectsMoreThan100Pages(t *testing.T) {
	err := checkPDFLimits(context.Background(), writeTempPDF(t), fixedPageCount(101))
	var ee *ExtractError
	if !errors.As(err, &ee) || ee.Retryable || ee.Status != 413 {
		t.Fatalf("err = %#v, want permanent 413 for 101 pages", err)
	}
}

func TestCheckPDFLimitsAcceptsExactly100Pages(t *testing.T) {
	if err := checkPDFLimits(context.Background(), writeTempPDF(t), fixedPageCount(100)); err != nil {
		t.Fatalf("100 pages must be accepted: %v", err)
	}
}
