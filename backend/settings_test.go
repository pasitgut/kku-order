package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestDefaultOCRSettingsReadEnvironment(t *testing.T) {
	t.Setenv("OCR_DPI", "300")
	t.Setenv("OCR_LANGUAGE", "auto")
	t.Setenv("OCR_MAX_SIDE", "2048")
	t.Setenv("OCR_PREPROCESS", "standard")

	settings := defaultOCRSettings()

	if settings.Engine != "thaidoc" || settings.DPI != 300 || settings.Language != "auto" || settings.MaxSide != 2048 || !settings.Preprocess {
		t.Fatalf("defaults = %+v, want values from environment", settings)
	}
	if settings.ReviewThreshold != 0.9 || !settings.ImportEnabled || settings.ImportSource != "folder" || settings.ImportIntervalMinutes != 5 {
		t.Fatalf("defaults = %+v, want review 0.9 and folder import every 5 minutes", settings)
	}
}

func TestOCRSettingsValidateRejectsUnsupportedValues(t *testing.T) {
	cases := map[string]func(*OCRSettings){
		"engine":    func(s *OCRSettings) { s.Engine = "tesseract" },
		"dpi":       func(s *OCRSettings) { s.DPI = 123 },
		"language":  func(s *OCRSettings) { s.Language = "klingon" },
		"max side":  func(s *OCRSettings) { s.MaxSide = 999 },
		"too low":   func(s *OCRSettings) { s.ReviewThreshold = 0.3 },
		"too high":  func(s *OCRSettings) { s.ReviewThreshold = 1.2 },
		"interval":  func(s *OCRSettings) { s.ImportIntervalMinutes = 0 },
		"source":    func(s *OCRSettings) { s.ImportSource = "ftp" },
		"after":     func(s *OCRSettings) { s.DriveAfterImport = "delete" },
		"drive url": func(s *OCRSettings) { s.DriveFolderURL = "https://example.com/folder" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			settings := defaultOCRSettings()
			mutate(&settings)
			if err := settings.Validate(); err == nil {
				t.Fatalf("Validate() accepted %+v", settings)
			}
		})
	}
	if err := defaultOCRSettings().Validate(); err != nil {
		t.Fatalf("default settings invalid: %v", err)
	}
	withDrive := defaultOCRSettings()
	withDrive.ImportSource = "drive"
	withDrive.DriveFolderURL = "https://drive.google.com/drive/folders/abc123"
	if err := withDrive.Validate(); err != nil {
		t.Fatalf("drive folder link rejected: %v", err)
	}
}

func TestMergeStoredOCRSettingsKeepsDefaultsForMissingKeys(t *testing.T) {
	defaults := defaultOCRSettings()
	merged, err := mergeOCRSettings(defaults, `{"dpi":300,"reviewThreshold":0.95}`)
	if err != nil {
		t.Fatalf("mergeOCRSettings() error = %v", err)
	}
	if merged.DPI != 300 || merged.ReviewThreshold != 0.95 || merged.MaxSide != defaults.MaxSide || merged.Language != defaults.Language {
		t.Fatalf("merged = %+v, want stored dpi/threshold over defaults", merged)
	}

	fallback, err := mergeOCRSettings(defaults, `{broken`)
	if err == nil || fallback != defaults {
		t.Fatalf("broken JSON: got %+v, err %v; want defaults and an error", fallback, err)
	}
	invalid, err := mergeOCRSettings(defaults, `{"dpi":123}`)
	if err == nil || invalid != defaults {
		t.Fatalf("invalid stored value: got %+v, err %v; want defaults and an error", invalid, err)
	}
}

func TestNeedsReviewUsesConfiguredThreshold(t *testing.T) {
	if !needsReview(0.92, false, 0.95) {
		t.Fatal("0.92 should need review at threshold 0.95")
	}
	if needsReview(0.92, false, 0.90) {
		t.Fatal("0.92 should pass at threshold 0.90")
	}
	if !needsReview(0.99, true, 0.90) {
		t.Fatal("a low-confidence field should always need review")
	}
}

func TestImportDueRespectsInterval(t *testing.T) {
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	if !importDue(time.Time{}, now, 5) {
		t.Fatal("first scan should run immediately")
	}
	if importDue(now.Add(-2*time.Minute), now, 5) {
		t.Fatal("scan ran before the interval elapsed")
	}
	if !importDue(now.Add(-5*time.Minute), now, 5) {
		t.Fatal("scan should run once the interval elapsed")
	}
	// Cron fires a few ms after the minute, so the previous scan may look slightly too recent.
	if !importDue(now.Add(-4*time.Minute-59*time.Second), now, 5) {
		t.Fatal("tick jitter must not push the scan back a whole minute")
	}
}

func TestSystemSettingsDoNotExposeSecrets(t *testing.T) {
	t.Setenv("API_KEY", "secret-key")
	t.Setenv("SMTP_PASSWORD", "smtp-secret")
	t.Setenv("SMTP_HOST", "smtp.example.ac.th")
	t.Setenv("EXTERNAL_USERS_URL", "https://user:pw@api.example.com/users?token=abc")

	settings := currentSystemSettings()
	payload, _ := json.Marshal(settings)
	for _, secret := range []string{"secret-key", "smtp-secret", ":pw@", "token=abc"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("system settings leaked %q: %s", secret, payload)
		}
	}
	if !settings.APIKeyConfigured || settings.ExternalUsersHost != "api.example.com" || settings.SMTPHost != "smtp.example.ac.th" {
		t.Fatalf("system settings = %+v, want configured flags and hosts", settings)
	}
}

func TestSystemSettingsShowTheDefaultSyncHost(t *testing.T) {
	t.Setenv("EXTERNAL_USERS_URL", "")
	t.Setenv("EXTERNAL_USERS_BASE_URL", "")
	if got := currentSystemSettings().ExternalUsersHost; got != hostOnly(defaultExternalUsersURL) || got == "" {
		t.Fatalf("host = %q, want the host sync actually calls", got)
	}
}

type fakeSettingsStore struct {
	settings OCRSettings
	savedBy  string
	err      error
}

func (s *fakeSettingsStore) LoadOCR() (OCRSettings, error) { return s.settings, s.err }
func (s *fakeSettingsStore) SaveOCR(settings OCRSettings, userID string) error {
	s.settings = settings
	s.savedBy = userID
	return s.err
}

func settingsRouter(store SettingsStore, role string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(authUserKey, "admin01")
		c.Set(authRoleKey, role)
		c.Next()
	})
	router.GET("/api/v1/settings/ocr", getOCRSettingsHandler(store, ocrAPIConfig{}))
	router.PUT("/api/v1/settings/ocr", requireRole("ADMIN"), updateOCRSettingsHandler(store))
	return router
}

func TestUpdateOCRSettingsSavesValidPayload(t *testing.T) {
	store := &fakeSettingsStore{settings: defaultOCRSettings()}
	body := `{"engine":"thaidoc","language":"thai","dpi":300,"maxSide":1536,"preprocess":true,"reviewThreshold":0.85,"importEnabled":false,"importSource":"folder","importIntervalMinutes":15,"driveFolderUrl":"","driveAfterImport":"move"}`
	request := httptest.NewRequest(http.MethodPut, "/api/v1/settings/ocr", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	settingsRouter(store, "ADMIN").ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body.String())
	}
	if store.settings.DPI != 300 || store.settings.ReviewThreshold != 0.85 || store.settings.ImportEnabled || store.savedBy != "admin01" {
		t.Fatalf("saved %+v by %q", store.settings, store.savedBy)
	}
}

func TestUpdateOCRSettingsRejectsInvalidPayloadAndNonAdmins(t *testing.T) {
	store := &fakeSettingsStore{settings: defaultOCRSettings()}
	invalid := httptest.NewRequest(http.MethodPut, "/api/v1/settings/ocr", strings.NewReader(`{"engine":"thaidoc","dpi":123}`))
	response := httptest.NewRecorder()
	settingsRouter(store, "ADMIN").ServeHTTP(response, invalid)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid payload status = %d, want 400", response.Code)
	}

	staff := httptest.NewRequest(http.MethodPut, "/api/v1/settings/ocr", strings.NewReader(`{}`))
	response = httptest.NewRecorder()
	settingsRouter(store, "STAFF").ServeHTTP(response, staff)
	if response.Code != http.StatusForbidden {
		t.Fatalf("staff status = %d, want 403", response.Code)
	}
	if store.savedBy != "" {
		t.Fatal("rejected requests must not save")
	}
}

func TestGetOCRSettingsFallsBackToDefaultsWhenStoreFails(t *testing.T) {
	store := &fakeSettingsStore{err: errors.New("db down")}
	response := httptest.NewRecorder()
	settingsRouter(store, "VIEWER").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/settings/ocr", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var payload struct {
		Data OCRSettings `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Data.Engine != "thaidoc" {
		t.Fatalf("payload = %s (err %v), want defaults", response.Body.String(), err)
	}
}

func TestLowConfidenceAtUsesThreshold(t *testing.T) {
	userID := uint(7)
	metadata := commandMetadata{
		fields:       []ExtractedField{{FieldKey: "order_no", Confidence: 0.93}},
		appointments: []Appointment{{FullName: "ผศ.ดร.สมชาย ใจดี", Confidence: 0.97, DirectoryUserID: &userID, NameMatchScore: 1}},
	}
	if lowConfidenceAt(metadata, 0.90) {
		t.Fatal("0.93 and 0.97 pass a 0.90 threshold")
	}
	if !lowConfidenceAt(metadata, 0.95) {
		t.Fatal("a 0.93 field is below a 0.95 threshold")
	}
}

func TestLowConfidenceAtFlagsNamesMissingFromDirectory(t *testing.T) {
	userID := uint(7)
	matched := commandMetadata{appointments: []Appointment{{FullName: "ผศ.ดร.สมชาย ใจดี", Confidence: 0.99, DirectoryUserID: &userID, NameMatchScore: 1}}}
	if lowConfidenceAt(matched, 0.90) {
		t.Fatal("a confident, matched name passes")
	}
	unmatched := commandMetadata{appointments: []Appointment{{FullName: "นางสาวพิมพ์ชนก อินทร์แก้ว", Confidence: 0.99}}}
	if !lowConfidenceAt(unmatched, 0.90) {
		t.Fatal("a name missing from the directory must send the document to review")
	}
	fuzzy := commandMetadata{appointments: []Appointment{{FullName: "อ.ธนพล แก้วมณี", Confidence: 0.99, DirectoryUserID: &userID, NameMatchScore: 0.86}}}
	if !lowConfidenceAt(fuzzy, 0.90) {
		t.Fatal("a fuzzy directory match must send the document to review")
	}
}

func TestDefaultOCRSettingsIgnoreUnsupportedEnvironmentValues(t *testing.T) {
	t.Setenv("OCR_DPI", "250")
	t.Setenv("OCR_LANGUAGE", "th")
	t.Setenv("OCR_MAX_SIDE", "999")
	settings := defaultOCRSettings()
	if settings.DPI != 200 || settings.Language != "thai" || settings.MaxSide != 1536 {
		t.Fatalf("defaults = %+v, want supported fallbacks", settings)
	}
	if err := settings.Validate(); err != nil {
		t.Fatalf("defaults must always validate: %v", err)
	}
}

func TestDriveImportNeedsAFolderLink(t *testing.T) {
	settings := defaultOCRSettings()
	settings.ImportSource = "drive"
	if err := settings.Validate(); err == nil {
		t.Fatal("an enabled Drive import without a folder link must be rejected")
	}
	settings.ImportEnabled = false
	if err := settings.Validate(); err != nil {
		t.Fatalf("a disabled Drive import may omit the link: %v", err)
	}
}

func TestMergeOCRSettingsMapsLegacyOneOCRWithoutLosingImportSettings(t *testing.T) {
	defaults := defaultOCRSettings()
	defaults.APIURL = "https://default.example"
	stored := `{"engine":"one-ocr","reviewThreshold":0.7,"apiUrl":"https://kept.example","importIntervalMinutes":30}`
	got, err := mergeOCRSettings(defaults, stored)
	if err != nil {
		t.Fatalf("mergeOCRSettings: %v", err)
	}
	if got.Engine != "thaidoc" {
		t.Fatalf("Engine = %q, want thaidoc", got.Engine)
	}
	if got.APIURL != "https://kept.example" || got.ReviewThreshold != 0.7 || got.ImportIntervalMinutes != 30 {
		t.Fatalf("stored values were wiped: %+v", got)
	}
}

func TestMergeOCRSettingsEmptyStoredURLFallsBackToDefault(t *testing.T) {
	defaults := defaultOCRSettings()
	defaults.APIURL = "https://env.example"
	got, err := mergeOCRSettings(defaults, `{"engine":"thaidoc","apiUrl":""}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.APIURL != "https://env.example" {
		t.Fatalf("APIURL = %q", got.APIURL)
	}
}

func TestOCRSettingsValidateChecksAPIURL(t *testing.T) {
	base := defaultOCRSettings()
	base.Engine = "thaidoc"
	for _, bad := range []string{"ftp://x", "https://"} {
		s := base
		s.APIURL = bad
		if err := s.Validate(); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	for _, good := range []string{"", "https://ocr.example", "http://10.0.0.5:8000"} {
		s := base
		s.APIURL = good
		if err := s.Validate(); err != nil {
			t.Fatalf("%q rejected: %v", good, err)
		}
	}
	s := base
	s.Engine = "one-ocr"
	if err := s.Validate(); err == nil {
		t.Fatal("one-ocr must not be a valid engine after the change")
	}
}

func TestGetOCRSettingsReportsKeyConfiguredWithoutValue(t *testing.T) {
	h := getOCRSettingsHandler(&fakeSettingsStore{settings: defaultOCRSettings()}, ocrAPIConfig{APIKey: "SECRET-XYZ"})
	rec := performRequest(t, http.MethodGet, "/api/v1/settings/ocr", "/api/v1/settings/ocr", nil, h)
	if strings.Contains(rec.Body.String(), "SECRET-XYZ") {
		t.Fatal("GET leaked the key")
	}
	var out struct {
		OCRAPIKeyConfigured bool `json:"ocrApiKeyConfigured"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || !out.OCRAPIKeyConfigured {
		t.Fatalf("body = %s err=%v", rec.Body.String(), err)
	}
}

func TestDefaultOCRSettingsUseEnvAPIURL(t *testing.T) {
	t.Setenv("OCR_API_URL", " https://thaidoc.example ")

	if got := defaultOCRSettings().APIURL; got != "https://thaidoc.example" {
		t.Fatalf("APIURL = %q, want the trimmed OCR_API_URL", got)
	}
}

func TestGetOCRSettingsReturnsEnvAPIURLWhenNothingStored(t *testing.T) {
	t.Setenv("OCR_API_URL", "https://thaidoc.example")
	// A nil store stands for "no row in app_settings": the handler must report the env URL.
	h := getOCRSettingsHandler(nil, ocrAPIConfig{DefaultURL: "https://thaidoc.example"})
	rec := performRequest(t, http.MethodGet, "/api/v1/settings/ocr", "/api/v1/settings/ocr", nil, h)

	var out struct {
		Data OCRSettings `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body = %s err=%v", rec.Body.String(), err)
	}
	if out.Data.APIURL != "https://thaidoc.example" {
		t.Fatalf("apiUrl = %q, want env OCR_API_URL", out.Data.APIURL)
	}
}
