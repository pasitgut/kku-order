package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ocrConnectionResult struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// ocrHealthURL: thaidoc uses {base}/health; docflow-contract uses the origin + /health.
func ocrHealthURL(engine, base string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid url %q", base)
	}
	switch engine {
	case "thaidoc":
		return trimmed + "/health", nil
	case "docflow-contract":
		return u.Scheme + "://" + u.Host + "/health", nil
	default:
		return "", fmt.Errorf("unknown OCR engine %q", engine)
	}
}

// checkOCRConnection calls the engine's health endpoint. thaidoc answers 200 even while
// its models are still loading, so models_loaded=false is reported as not ready.
func checkOCRConnection(ctx context.Context, client *http.Client, healthURL, apiKey string) ocrConnectionResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return ocrConnectionResult{Status: "unreachable", Detail: "URL ไม่ถูกต้อง"}
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return ocrConnectionResult{Status: "unreachable", Detail: "เชื่อมต่อไม่ได้: " + err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ocrConnectionResult{Status: "unauthorized", Detail: "API key ไม่ถูกต้อง หรือไม่มีสิทธิ์"}
	case resp.StatusCode/100 != 2:
		return ocrConnectionResult{Status: "unreachable", Detail: fmt.Sprintf("OCR API ตอบสถานะ %d", resp.StatusCode)}
	}
	var health struct {
		ModelsLoaded *bool `json:"models_loaded"`
	}
	_ = json.Unmarshal(raw, &health)
	if health.ModelsLoaded != nil && !*health.ModelsLoaded {
		return ocrConnectionResult{Status: "unreachable", Detail: "OCR API กำลังโหลดโมเดล ลองใหม่อีกสักครู่"}
	}
	return ocrConnectionResult{Status: "ok", Detail: "เชื่อมต่อได้และพร้อมใช้งาน"}
}

type ocrConnectionRequest struct {
	Engine string `json:"engine"`
	APIURL string `json:"apiUrl"`
}

// testOCRConnectionHandler checks the engine using the submitted (unsaved) engine and URL,
// falling back to the saved values. The API key comes from the environment and is never returned.
func testOCRConnectionHandler(load func() (OCRSettings, error), cfg ocrAPIConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, err := load()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "อ่านค่าตั้งค่าไม่ได้"})
			return
		}
		var req ocrConnectionRequest
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รูปแบบคำขอไม่ถูกต้อง"})
			return
		}
		if req.Engine != "" {
			settings.Engine = req.Engine
		}
		if req.APIURL != "" {
			settings.APIURL = req.APIURL
		}
		if !allowedOCREngines[settings.Engine] {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("ยังไม่รองรับเครื่องมือ OCR %q", settings.Engine)})
			return
		}
		if err := validateOCRAPIURL(settings.APIURL); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		healthURL, err := ocrHealthURL(settings.Engine, ocrBaseURL(settings.APIURL, cfg.DefaultURL))
		if err != nil {
			c.JSON(http.StatusOK, ocrConnectionResult{Status: "unreachable", Detail: "ยังไม่ได้ตั้งค่า URL ของ OCR API"})
			return
		}
		client := &http.Client{Timeout: 10 * time.Second}
		c.JSON(http.StatusOK, checkOCRConnection(c.Request.Context(), client, healthURL, cfg.APIKey))
	}
}
