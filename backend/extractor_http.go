package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type httpExtractor struct {
	engine   string
	endpoint string
	apiKey   string
	client   *http.Client
	decode   func([]byte) (ExtractionResult, error)
}

func (e *httpExtractor) Name() string { return e.engine }

func (e *httpExtractor) Extract(ctx context.Context, pdfPath, filename string) (ExtractionResult, error) {
	file, err := os.Open(pdfPath)
	if err != nil {
		return ExtractionResult{}, &ExtractError{Message: "เปิดไฟล์ PDF ไม่ได้: " + err.Error()}
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return ExtractionResult{}, &ExtractError{Message: "สร้างคำขอไม่ได้: " + err.Error()}
	}
	if _, err := io.Copy(part, file); err != nil {
		return ExtractionResult{}, &ExtractError{Message: "อ่านไฟล์ไม่ได้: " + err.Error()}
	}
	if err := writer.Close(); err != nil {
		return ExtractionResult{}, &ExtractError{Message: "สร้างคำขอไม่ได้: " + err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, body)
	if err != nil {
		return ExtractionResult{}, &ExtractError{Message: "URL ของ OCR API ไม่ถูกต้อง"}
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		// การเชื่อมต่อล้มหรือ timeout เป็นปัญหาชั่วคราว ลองใหม่ได้
		return ExtractionResult{}, &ExtractError{Retryable: true, Message: "เชื่อมต่อ OCR API ไม่ได้: " + err.Error()}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return ExtractionResult{}, &ExtractError{Retryable: true, Message: "อ่านคำตอบจาก OCR API ไม่ครบ: " + err.Error()}
	}
	if resp.StatusCode/100 != 2 {
		return ExtractionResult{}, classifyHTTPFailure(resp.StatusCode, raw)
	}
	result, err := e.decode(raw)
	if err != nil {
		return ExtractionResult{}, &ExtractError{Message: "คำตอบจาก OCR API อ่านไม่ได้: " + err.Error()}
	}
	return result, nil
}

// classifyHTTPFailure ตามตาราง "การแบ่ง error" ใน spec
// 500 คืนเป็น Retryable เสมอ คิวจะทำให้ครั้งที่สองของเอกสารเป็นถาวรจาก extraction_last_status
func classifyHTTPFailure(status int, body []byte) *ExtractError {
	ee := &ExtractError{Status: status}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		ee.Message = "API key ไม่ถูกต้อง หรือไม่มีสิทธิ์เรียกใช้ OCR"
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity:
		ee.Message = fmt.Sprintf("OCR API ปฏิเสธไฟล์ (%d)", status)
		if detail := apiDetail(body); detail != "" {
			ee.Message += ": " + detail
		}
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		ee.Retryable = true
		ee.Message = fmt.Sprintf("OCR API ยังไม่พร้อมให้บริการ (%d)", status)
	case http.StatusInternalServerError:
		ee.Retryable = true
		ee.Message = "OCR API เกิดข้อผิดพลาดภายใน (500)"
	default:
		ee.Message = fmt.Sprintf("OCR API ตอบสถานะที่ไม่คาดคิด (%d) ตรวจ URL ที่ตั้งค่าไว้", status)
	}
	return ee
}

// apiDetail อ่านฟิลด์ detail ของคำตอบ error (รูปแบบ FastAPI) ถ้ามี
func apiDetail(body []byte) string {
	var v struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return ""
	}
	return strings.TrimSpace(v.Detail)
}
