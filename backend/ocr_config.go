package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	maxPDFBytes = 50 << 20
	maxPDFPages = 100
)

// pageCounter นับหน้า PDF ตอนใช้งานจริงคือ detectPDFPageCount (pdfinfo) เทสต์ส่งฟังก์ชันปลอมแทน
type pageCounter func(ctx context.Context, path string) int

type ocrAPIConfig struct {
	DefaultURL  string
	APIKey      string
	Timeout     time.Duration
	Concurrency int
}

func loadOCRAPIConfig(getenv func(string) string, readFile func(string) ([]byte, error)) ocrAPIConfig {
	cfg := ocrAPIConfig{
		DefaultURL:  strings.TrimSpace(getenv("OCR_API_URL")),
		APIKey:      strings.TrimSpace(getenv("OCR_API_KEY")),
		Timeout:     10 * time.Minute,
		Concurrency: 1,
	}
	if cfg.APIKey == "" {
		if path := strings.TrimSpace(getenv("OCR_API_KEY_FILE")); path != "" {
			if raw, err := readFile(path); err == nil {
				cfg.APIKey = strings.TrimSpace(string(raw))
			}
		}
	}
	if d, err := time.ParseDuration(getenv("OCR_API_TIMEOUT")); err == nil && d > 0 {
		cfg.Timeout = d
	}
	if n, err := strconv.Atoi(getenv("OCR_API_CONCURRENCY")); err == nil {
		cfg.Concurrency = min(max(n, 1), 8)
	}
	return cfg
}

// loadOCRAPIConfigFromEnv ใช้ตอน start ระบบจริง
func loadOCRAPIConfigFromEnv() ocrAPIConfig {
	return loadOCRAPIConfig(os.Getenv, os.ReadFile)
}

// ocrBaseURL ใช้ URL ที่บันทึกไว้ก่อน ถ้าว่างใช้ค่าจาก env (OCR_API_URL)
func ocrBaseURL(settingsURL, defaultURL string) string {
	base := strings.TrimRight(strings.TrimSpace(settingsURL), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(defaultURL), "/")
	}
	return base
}

func newExtractor(settings OCRSettings, cfg ocrAPIConfig) (Extractor, error) {
	base := ocrBaseURL(settings.APIURL, cfg.DefaultURL)
	if base == "" {
		return nil, &ExtractError{Message: "ยังไม่ได้ตั้งค่า URL ของ OCR API"}
	}
	client := &http.Client{Timeout: cfg.Timeout}
	switch settings.Engine {
	case "thaidoc":
		return &httpExtractor{
			engine:   "thaidoc",
			endpoint: base + "/api/v1/extract-json",
			apiKey:   cfg.APIKey,
			client:   client,
			decode:   parseThaidocResponse,
		}, nil
	case "docflow-contract":
		return &httpExtractor{
			engine:   "docflow-contract",
			endpoint: base,
			apiKey:   cfg.APIKey,
			client:   client,
			decode:   parseContractResponse,
		}, nil
	default:
		return nil, fmt.Errorf("unknown OCR engine %q", settings.Engine)
	}
}

// checkPDFLimits ตรวจก่อนส่ง: ไม่มีไฟล์, เกิน 50 MB หรือเกิน 100 หน้าเป็นถาวร (ไม่ลองใหม่)
func checkPDFLimits(ctx context.Context, path string, countPages pageCounter) error {
	info, err := os.Stat(path)
	if err != nil {
		return &ExtractError{Message: "ไม่พบไฟล์ PDF"}
	}
	if info.Size() > maxPDFBytes {
		return &ExtractError{Status: 413, Message: "ไฟล์ PDF ใหญ่เกิน 50 MB"}
	}
	if pages := countPages(ctx, path); pages > maxPDFPages {
		return &ExtractError{Status: 413, Message: fmt.Sprintf("PDF มี %d หน้า เกินขอบเขตสูงสุด %d หน้า", pages, maxPDFPages)}
	}
	return nil
}
