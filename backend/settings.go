package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const ocrSettingsKey = "ocr"

// OCRSettings are the OCR and auto-import options an admin can change from the
// web. Values missing from the database fall back to the .env defaults.
type OCRSettings struct {
	Engine                string  `json:"engine"`
	Language              string  `json:"language"`
	DPI                   int     `json:"dpi"`
	MaxSide               int     `json:"maxSide"`
	Preprocess            bool    `json:"preprocess"`
	ReviewThreshold       float64 `json:"reviewThreshold"`
	ImportEnabled         bool    `json:"importEnabled"`
	ImportSource          string  `json:"importSource"`
	ImportIntervalMinutes int     `json:"importIntervalMinutes"`
	// Drive fields are stored for the Google Drive importer, which is built separately.
	DriveFolderURL   string `json:"driveFolderUrl"`
	DriveAfterImport string `json:"driveAfterImport"`
}

var (
	allowedOCREngines   = map[string]bool{"one-ocr": true}
	allowedOCRLanguages = map[string]bool{"auto": true, "thai": true, "english": true}
	allowedOCRDPIs      = map[int]bool{150: true, 200: true, 300: true, 400: true}
	allowedOCRMaxSides  = map[int]bool{1024: true, 1536: true, 2048: true, 3072: true}
	allowedImportSource = map[string]bool{"folder": true, "drive": true}
	allowedAfterImport  = map[string]bool{"move": true, "keep": true}
)

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil {
		return fallback
	}
	return value
}

func defaultOCRSettings() OCRSettings {
	// .env values outside what the settings page offers fall back to safe defaults.
	language := getenv("OCR_LANGUAGE", "thai")
	if !allowedOCRLanguages[language] {
		language = "thai"
	}
	dpi := envInt("OCR_DPI", 200)
	if !allowedOCRDPIs[dpi] {
		dpi = 200
	}
	maxSide := envInt("OCR_MAX_SIDE", 1536)
	if !allowedOCRMaxSides[maxSide] {
		maxSide = 1536
	}
	return OCRSettings{
		Engine:                "one-ocr",
		Language:              language,
		DPI:                   dpi,
		MaxSide:               maxSide,
		Preprocess:            strings.EqualFold(getenv("OCR_PREPROCESS", "none"), "standard"),
		ReviewThreshold:       0.9,
		ImportEnabled:         true,
		ImportSource:          "folder",
		ImportIntervalMinutes: 5,
		DriveAfterImport:      "move",
	}
}

func (s OCRSettings) Validate() error {
	switch {
	case !allowedOCREngines[s.Engine]:
		return fmt.Errorf("ยังไม่รองรับเครื่องมือ OCR %q", s.Engine)
	case !allowedOCRLanguages[s.Language]:
		return fmt.Errorf("ไม่รองรับภาษา %q", s.Language)
	case !allowedOCRDPIs[s.DPI]:
		return fmt.Errorf("ความละเอียดต้องเป็น 150, 200, 300 หรือ 400 DPI")
	case !allowedOCRMaxSides[s.MaxSide]:
		return fmt.Errorf("ขนาดภาพสูงสุดไม่ถูกต้อง")
	case s.ReviewThreshold < 0.5 || s.ReviewThreshold > 1:
		return fmt.Errorf("เกณฑ์ความมั่นใจต้องอยู่ระหว่าง 50%% ถึง 100%%")
	case !allowedImportSource[s.ImportSource]:
		return fmt.Errorf("แหล่งไฟล์ต้องเป็นโฟลเดอร์บนเซิร์ฟเวอร์หรือ Google Drive")
	case s.ImportIntervalMinutes < 1 || s.ImportIntervalMinutes > 1440:
		return fmt.Errorf("รอบการดึงไฟล์ต้องอยู่ระหว่าง 1 นาทีถึง 1 วัน")
	case !allowedAfterImport[s.DriveAfterImport]:
		return fmt.Errorf("ตัวเลือกหลังนำเข้าไม่ถูกต้อง")
	}
	if s.ImportEnabled && s.ImportSource == "drive" && s.DriveFolderURL == "" {
		return fmt.Errorf("กรุณาใส่ลิงก์โฟลเดอร์ Google Drive")
	}
	if s.DriveFolderURL != "" {
		parsed, err := url.Parse(s.DriveFolderURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host != "drive.google.com" {
			return fmt.Errorf("ลิงก์โฟลเดอร์ต้องเป็นลิงก์จาก drive.google.com")
		}
	}
	return nil
}

// mergeOCRSettings overlays stored JSON on the defaults. A broken or invalid
// stored value must never stop document processing, so it returns the defaults.
func mergeOCRSettings(defaults OCRSettings, stored string) (OCRSettings, error) {
	merged := defaults
	if strings.TrimSpace(stored) == "" {
		return defaults, nil
	}
	if err := json.Unmarshal([]byte(stored), &merged); err != nil {
		return defaults, fmt.Errorf("อ่านค่าตั้งค่า OCR: %w", err)
	}
	if err := merged.Validate(); err != nil {
		return defaults, err
	}
	return merged, nil
}

func (s OCRSettings) preprocessMode() string {
	if s.Preprocess {
		return "standard"
	}
	return "none"
}

func (s OCRSettings) renderArgs() []string {
	return []string{"-png", "-r", strconv.Itoa(s.DPI), "-f", "1"}
}

func (s OCRSettings) workerArgs(script, inputDir, outputJSON, modelsDir string) []string {
	return []string{script, "--input-dir", inputDir, "--output-json", outputJSON, "--models-dir", modelsDir, "--language", s.Language, "--max-side", strconv.Itoa(s.MaxSide), "--preprocess", s.preprocessMode()}
}

func needsReview(averageConfidence float32, lowConfidence bool, threshold float64) bool {
	return lowConfidence || float64(averageConfidence) < threshold
}

// lowConfidenceAt re-checks extracted fields and names against the admin's
// threshold; extraction itself flags them at the fixed 0.90 default.
func lowConfidenceAt(metadata commandMetadata, threshold float64) bool {
	for _, field := range metadata.fields {
		if float64(field.Confidence) < threshold {
			return true
		}
	}
	for _, appointment := range metadata.appointments {
		// Names missing from the directory, or only fuzzily matched, always need a reviewer.
		if float64(appointment.Confidence) < threshold || appointment.DirectoryUserID == nil || appointment.NameMatchScore < 0.90 {
			return true
		}
	}
	return false
}

func importDue(lastScan, now time.Time, intervalMinutes int) bool {
	// Cron ticks land a few ms after the minute; allow 30s so jitter never skips a tick.
	return lastScan.IsZero() || now.Sub(lastScan) >= time.Duration(intervalMinutes)*time.Minute-30*time.Second
}

// SettingsStore hides the database so handlers and the processor can be tested.
type SettingsStore interface {
	LoadOCR() (OCRSettings, error)
	SaveOCR(settings OCRSettings, userID string) error
}

type AppSetting struct {
	SettingKey string    `gorm:"column:setting_key;primaryKey;size:100"`
	Value      string    `gorm:"column:value;type:json;not null"`
	UpdatedBy  string    `gorm:"column:updated_by;size:255"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (AppSetting) TableName() string { return "app_settings" }

type dbSettingsStore struct{ db *gorm.DB }

func NewSettingsStore(db *gorm.DB) SettingsStore { return &dbSettingsStore{db: db} }

func (s *dbSettingsStore) LoadOCR() (OCRSettings, error) {
	defaults := defaultOCRSettings()
	var row AppSetting
	err := s.db.Where("setting_key = ?", ocrSettingsKey).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	return mergeOCRSettings(defaults, row.Value)
}

func (s *dbSettingsStore) SaveOCR(settings OCRSettings, userID string) error {
	value, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	row := AppSetting{SettingKey: ocrSettingsKey, Value: string(value), UpdatedBy: userID, UpdatedAt: time.Now()}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error; err != nil {
			return err
		}
		return tx.Create(&AuditLog{UserID: userID, Action: "SETTINGS_OCR_UPDATED", Details: string(value)}).Error
	})
}

// loadOCRSettingsOrDefault never fails: processing keeps working on .env values.
func loadOCRSettingsOrDefault(store SettingsStore) OCRSettings {
	if store == nil {
		return defaultOCRSettings()
	}
	settings, err := store.LoadOCR()
	if err != nil {
		return defaultOCRSettings()
	}
	return settings
}

func getOCRSettingsHandler(store SettingsStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": loadOCRSettingsOrDefault(store)})
	}
}

func updateOCRSettingsHandler(store SettingsStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var settings OCRSettings
		if err := c.ShouldBindJSON(&settings); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลการตั้งค่าไม่ถูกต้อง"})
			return
		}
		if err := settings.Validate(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := store.SaveOCR(settings, currentUserID(c)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "บันทึกการตั้งค่าไม่สำเร็จ"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": settings})
	}
}

// runOCRWorker renders nothing itself; it runs the OCR worker on page-*.png
// files already in inputDir using the given settings.
func runOCRWorker(ctx context.Context, settings OCRSettings, inputDir string) (workerResult, error) {
	outputJSON := filepath.Join(inputDir, "ocr.json")
	script := getenv("OCR_SCRIPT", filepath.Join(".", "ocr_worker.py"))
	modelsDir := getenv("OCR_MODELS_DIR", filepath.Join(".", "one-ocr", "models"))
	command := exec.CommandContext(ctx, getenv("OCR_PYTHON", "python"), settings.workerArgs(script, inputDir, outputJSON, modelsDir)...)
	command.Env = append(os.Environ(), "PYTHONUTF8=1")
	if output, err := command.CombinedOutput(); err != nil {
		return workerResult{}, fmt.Errorf("เรียก %s: %w: %s", settings.Engine, err, strings.TrimSpace(string(output)))
	}
	contents, err := os.ReadFile(outputJSON)
	if err != nil {
		return workerResult{}, fmt.Errorf("อ่านผลลัพธ์ %s: %w", settings.Engine, err)
	}
	var result workerResult
	if err := json.Unmarshal(contents, &result); err != nil {
		return workerResult{}, fmt.Errorf("แปลงผลลัพธ์ %s: %w", settings.Engine, err)
	}
	if len(result.Pages) == 0 {
		return workerResult{}, fmt.Errorf("%s ไม่พบข้อความในเอกสาร", settings.Engine)
	}
	return result, nil
}

type ocrTestLine struct {
	Text       string  `json:"text"`
	Confidence float32 `json:"confidence"`
}

type ocrTestResult struct {
	Confidence float32       `json:"confidence"`
	Seconds    float64       `json:"seconds"`
	LineCount  int           `json:"lineCount"`
	Lines      []ocrTestLine `json:"lines"`
}

const maxOCRTestBytes = 20 << 20

// testOCRSettingsHandler reads the first page of a sample file with the
// submitted (unsaved) settings so an admin can compare before saving.
func testOCRSettingsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		settings := defaultOCRSettings()
		if raw := c.PostForm("settings"); raw != "" {
			if err := json.Unmarshal([]byte(raw), &settings); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลการตั้งค่าไม่ถูกต้อง"})
				return
			}
		}
		if err := settings.Validate(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		header, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาเลือกไฟล์ตัวอย่าง"})
			return
		}
		if header.Size > maxOCRTestBytes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ไฟล์ตัวอย่างต้องไม่เกิน 20 MB"})
			return
		}
		extension := strings.ToLower(filepath.Ext(header.Filename))
		if extension != ".pdf" && extension != ".png" && extension != ".jpg" && extension != ".jpeg" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รองรับไฟล์ PDF, PNG หรือ JPG"})
			return
		}
		tempDir, err := os.MkdirTemp("", "docflow-ocr-test-")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "สร้างพื้นที่ทดสอบไม่สำเร็จ"})
			return
		}
		defer os.RemoveAll(tempDir)
		sourcePath := filepath.Join(tempDir, "source"+extension)
		if err := c.SaveUploadedFile(header, sourcePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "บันทึกไฟล์ตัวอย่างไม่สำเร็จ"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
		defer cancel()
		started := time.Now()
		if extension == ".pdf" {
			args := append(settings.renderArgs(), "-l", "1", sourcePath, filepath.Join(tempDir, "page"))
			if output, err := exec.CommandContext(ctx, getenv("PDFTOPPM_BIN", "pdftoppm"), args...).CombinedOutput(); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("อ่านไฟล์ PDF ไม่สำเร็จ: %s", strings.TrimSpace(string(output)))})
				return
			}
		} else if err := os.Rename(sourcePath, filepath.Join(tempDir, "page-1.png")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "เตรียมภาพตัวอย่างไม่สำเร็จ"})
			return
		}
		result, err := runOCRWorker(ctx, settings, tempDir)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": summarizeOCRTest(result, time.Since(started))})
	}
}

func summarizeOCRTest(result workerResult, elapsed time.Duration) ocrTestResult {
	summary := ocrTestResult{Confidence: averageOCRConfidence(result.Pages), Seconds: math.Round(elapsed.Seconds()*10) / 10, Lines: []ocrTestLine{}}
	for _, page := range result.Pages {
		for _, line := range page.Lines {
			summary.LineCount++
			if len(summary.Lines) < 40 {
				summary.Lines = append(summary.Lines, ocrTestLine{Text: line.Text, Confidence: line.Confidence})
			}
		}
	}
	return summary
}

// SystemSettings are read-only values from .env. Secrets are reported only as
// "configured" flags, and URLs are reduced to their host.
type SystemSettings struct {
	UserSyncCron      string `json:"userSyncCron"`
	UserSyncTimezone  string `json:"userSyncTimezone"`
	ExternalUsersHost string `json:"externalUsersHost"`
	APIKeyConfigured  bool   `json:"apiKeyConfigured"`
	ExpiryCron        string `json:"expiryCron"`
	SMTPHost          string `json:"smtpHost"`
	SMTPPort          string `json:"smtpPort"`
	SMTPFrom          string `json:"smtpFrom"`
	SMTPConfigured    bool   `json:"smtpConfigured"`
	ImportDir         string `json:"importDir"`
	AuthRequired      bool   `json:"authRequired"`
}

func hostOnly(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func currentSystemSettings() SystemSettings {
	externalURL := getenv("EXTERNAL_USERS_URL", getenv("EXTERNAL_USERS_BASE_URL", defaultExternalUsersURL))
	smtpHost := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	return SystemSettings{
		UserSyncCron:      getenv("USER_SYNC_CRON", "0 5 * * *"),
		UserSyncTimezone:  getenv("USER_SYNC_TZ", "Asia/Bangkok"),
		ExternalUsersHost: hostOnly(externalURL),
		APIKeyConfigured:  strings.TrimSpace(os.Getenv("API_KEY")) != "",
		ExpiryCron:        getenv("EXPIRY_CRON", "10 5 * * *"),
		SMTPHost:          smtpHost,
		SMTPPort:          getenv("SMTP_PORT", "587"),
		SMTPFrom:          strings.TrimSpace(os.Getenv("SMTP_FROM")),
		SMTPConfigured:    smtpHost != "",
		ImportDir:         getenv("IMPORT_DIR", "./incoming"),
		AuthRequired:      authRequired(),
	}
}

func systemSettingsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": currentSystemSettings()})
	}
}
