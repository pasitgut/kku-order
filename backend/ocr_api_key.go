package main

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
)

const (
	ocrAPIKeySettingKey = "ocr_api_key"
	ocrAPIKeyMinLength  = 8
	ocrAPIKeyMaxLength  = 512

	ocrKeySourceWeb  = "web"
	ocrKeySourceEnv  = "env"
	ocrKeySourceNone = "none"

	errAPIKeyNotWritableMessage = "ต้องตั้งค่า SETTINGS_ENCRYPTION_KEY บนเซิร์ฟเวอร์ก่อน จึงจะบันทึก API key จากหน้าเว็บได้"
	errAPIKeyInvalidMessage     = "API key ต้องยาว 8–512 ตัวอักษร และไม่มีช่องว่างหรือขึ้นบรรทัดใหม่"
)

// validateOCRAPIKey trims the key and checks its shape. The key itself is never echoed back.
func validateOCRAPIKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if len(key) < ocrAPIKeyMinLength || len(key) > ocrAPIKeyMaxLength || strings.ContainsFunc(key, unicode.IsSpace) {
		return "", errors.New(errAPIKeyInvalidMessage)
	}
	return key, nil
}

// effectiveOCRAPIKey returns the key the OCR API should use right now, and where it came from:
// a key saved from the web wins, then the env key, then nothing. A saved key that cannot be
// read is logged (never with its value) and the env key is used instead.
func effectiveOCRAPIKey(store SettingsStore, cfg ocrAPIConfig) (string, string) {
	if store != nil {
		key, err := store.LoadWebAPIKey()
		switch {
		case err != nil:
			log.Printf("OCR API key saved from the web could not be read, using the env key instead: %v", err)
		case key != "":
			return key, ocrKeySourceWeb
		}
	}
	if cfg.APIKey != "" {
		return cfg.APIKey, ocrKeySourceEnv
	}
	return "", ocrKeySourceNone
}

// ocrAPIKeyHint shows the last four characters only. Shorter keys get no hint, so the whole key is never shown.
func ocrAPIKeyHint(key string) string {
	if len(key) < ocrAPIKeyMinLength {
		return ""
	}
	return key[len(key)-4:]
}

// ocrAPIKeyAuditDetails is what the audit log keeps about a key change: at most the last four characters.
func ocrAPIKeyAuditDetails(key string) string {
	hint := ocrAPIKeyHint(key)
	if hint == "" {
		return ""
	}
	return "ลงท้าย " + hint
}

type ocrAPIKeyRequest struct {
	APIKey string `json:"apiKey"`
}

// saveOCRAPIKeyHandler stores the key from the web, encrypted. Admin only (route guard).
func saveOCRAPIKeyHandler(store SettingsStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ocrAPIKeyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รูปแบบคำขอไม่ถูกต้อง"})
			return
		}
		if !store.APIKeyWritable() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": errAPIKeyNotWritableMessage})
			return
		}
		key, err := validateOCRAPIKey(req.APIKey)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := store.SaveWebAPIKey(key, currentUserID(c)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "บันทึก API key ไม่สำเร็จ"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// deleteOCRAPIKeyHandler removes the key saved from the web, so the env key applies again.
func deleteOCRAPIKeyHandler(store SettingsStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := store.DeleteWebAPIKey(currentUserID(c)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ลบ API key ไม่สำเร็จ"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
