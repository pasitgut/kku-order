package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	testWebKey = "WEB-SECRET-KEY-0001"
	testEnvKey = "ENV-SECRET-KEY-0002"
)

func apiKeyRouter(store SettingsStore, cfg ocrAPIConfig, role string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(authUserKey, "admin01")
		c.Set(authRoleKey, role)
		c.Next()
	})
	router.GET("/api/v1/settings/ocr", getOCRSettingsHandler(store, cfg))
	router.PUT("/api/v1/settings/ocr/api-key", requireRole("ADMIN"), saveOCRAPIKeyHandler(store))
	router.DELETE("/api/v1/settings/ocr/api-key", requireRole("ADMIN"), deleteOCRAPIKeyHandler(store))
	router.POST("/api/v1/settings/ocr/test", requireRole("ADMIN"), testOCRConnectionHandler(store, cfg))
	return router
}

func serve(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &buf
}

func TestEffectiveOCRAPIKeyPrecedence(t *testing.T) {
	env := ocrAPIConfig{APIKey: testEnvKey}
	cases := []struct {
		name       string
		store      SettingsStore
		cfg        ocrAPIConfig
		wantKey    string
		wantSource string
	}{
		{"web beats env", &fakeSettingsStore{webKey: testWebKey}, env, testWebKey, "web"},
		{"env when nothing saved", &fakeSettingsStore{}, env, testEnvKey, "env"},
		{"none", &fakeSettingsStore{}, ocrAPIConfig{}, "", "none"},
		{"bad ciphertext falls back to env", &fakeSettingsStore{webKeyErr: errors.New("message authentication failed")}, env, testEnvKey, "env"},
		{"bad ciphertext without env is none", &fakeSettingsStore{webKeyErr: errors.New("boom")}, ocrAPIConfig{}, "", "none"},
		{"nil store uses env", nil, env, testEnvKey, "env"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLog(t)
			key, source := effectiveOCRAPIKey(tc.store, tc.cfg)
			if key != tc.wantKey || source != tc.wantSource {
				t.Fatalf("effectiveOCRAPIKey() = (%q, %q), want (%q, %q)", key, source, tc.wantKey, tc.wantSource)
			}
			if strings.Contains(logs.String(), testEnvKey) || strings.Contains(logs.String(), testWebKey) {
				t.Fatalf("log leaked a key: %s", logs.String())
			}
		})
	}
}

func TestProcessorBuildsExtractorWithEffectiveKeyAtCallTime(t *testing.T) {
	var seen ocrAPIConfig
	p := &DocumentProcessor{
		settings:  &fakeSettingsStore{webKey: testWebKey},
		ocrConfig: ocrAPIConfig{APIKey: testEnvKey, Timeout: time.Minute, Concurrency: 1},
		newExtractor: func(_ OCRSettings, cfg ocrAPIConfig) (Extractor, error) {
			seen = cfg
			return nil, errors.New("stop here")
		},
	}
	_, _ = p.buildExtractor(defaultOCRSettings())
	if seen.APIKey != testWebKey || seen.Timeout != time.Minute {
		t.Fatalf("extractor got key %q timeout %v, want the web key and the env timeout", seen.APIKey, seen.Timeout)
	}
}

func TestSaveAPIKeyIsAdminOnly(t *testing.T) {
	for _, role := range []string{"VIEWER", "STAFF"} {
		store := &fakeSettingsStore{writable: true}
		rec := serve(apiKeyRouter(store, ocrAPIConfig{}, role), http.MethodPut, "/api/v1/settings/ocr/api-key", `{"apiKey":"`+testWebKey+`"}`)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want 403", role, rec.Code)
		}
		if store.webKey != "" {
			t.Fatalf("%s: rejected request must not save", role)
		}
		rec = serve(apiKeyRouter(store, ocrAPIConfig{}, role), http.MethodDelete, "/api/v1/settings/ocr/api-key", "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: delete status = %d, want 403", role, rec.Code)
		}
	}
}

func TestSaveAPIKeyStoresTrimmedKeyForAdmin(t *testing.T) {
	store := &fakeSettingsStore{writable: true}
	rec := serve(apiKeyRouter(store, ocrAPIConfig{}, "ADMIN"), http.MethodPut, "/api/v1/settings/ocr/api-key", `{"apiKey":"  `+testWebKey+`  "}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	if store.webKey != testWebKey || store.keySavedBy != "admin01" {
		t.Fatalf("saved %q by %q", store.webKey, store.keySavedBy)
	}
	if strings.Contains(rec.Body.String(), testWebKey) {
		t.Fatal("save response leaked the key")
	}
}

func TestSaveAPIKeyValidatesShape(t *testing.T) {
	bad := []string{
		`{"apiKey":"short"}`,
		`{"apiKey":""}`,
		`{"apiKey":"has space inside"}`,
		`{"apiKey":"line\nbreak-key-1"}`,
		`{"apiKey":"` + strings.Repeat("k", 513) + `"}`,
		`{}`,
	}
	for _, body := range bad {
		store := &fakeSettingsStore{writable: true}
		rec := serve(apiKeyRouter(store, ocrAPIConfig{}, "ADMIN"), http.MethodPut, "/api/v1/settings/ocr/api-key", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, rec.Code)
		}
		if store.webKey != "" {
			t.Errorf("body %s: invalid key was saved", body)
		}
	}
}

func TestSaveAPIKeyRefusedWhenNotWritable(t *testing.T) {
	store := &fakeSettingsStore{writable: false}
	rec := serve(apiKeyRouter(store, ocrAPIConfig{}, "ADMIN"), http.MethodPut, "/api/v1/settings/ocr/api-key", `{"apiKey":"`+testWebKey+`"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ต้องตั้งค่า SETTINGS_ENCRYPTION_KEY") {
		t.Fatalf("body = %s, want the Thai server message", rec.Body.String())
	}
	if store.webKey != "" {
		t.Fatal("refused request must not save")
	}
}

func TestDeleteAPIKeyRemovesWebKeyForAdmin(t *testing.T) {
	store := &fakeSettingsStore{writable: true, webKey: testWebKey}
	rec := serve(apiKeyRouter(store, ocrAPIConfig{}, "ADMIN"), http.MethodDelete, "/api/v1/settings/ocr/api-key", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	if store.webKey != "" || !store.keyRemoved {
		t.Fatal("web key was not removed")
	}
}

func TestGetOCRSettingsReportsKeySourceAndHint(t *testing.T) {
	cases := []struct {
		name       string
		store      SettingsStore
		cfg        ocrAPIConfig
		wantSource string
		wantHint   string
	}{
		{"web", &fakeSettingsStore{webKey: testWebKey, writable: true}, ocrAPIConfig{APIKey: testEnvKey}, "web", "0001"},
		{"env", &fakeSettingsStore{writable: true}, ocrAPIConfig{APIKey: testEnvKey}, "env", "0002"},
		{"none", &fakeSettingsStore{}, ocrAPIConfig{}, "none", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serve(apiKeyRouter(tc.store, tc.cfg, "VIEWER"), http.MethodGet, "/api/v1/settings/ocr", "")
			var out struct {
				Configured bool   `json:"ocrApiKeyConfigured"`
				Source     string `json:"ocrApiKeySource"`
				Hint       string `json:"ocrApiKeyHint"`
				Writable   bool   `json:"ocrApiKeyWritable"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("body %s: %v", rec.Body.String(), err)
			}
			if out.Source != tc.wantSource || out.Hint != tc.wantHint || out.Configured != (tc.wantSource != "none") {
				t.Fatalf("got %+v, want source %q hint %q", out, tc.wantSource, tc.wantHint)
			}
			if out.Writable != (tc.store != nil && tc.store.APIKeyWritable()) {
				t.Fatalf("writable = %v", out.Writable)
			}
			if strings.Contains(rec.Body.String(), testWebKey) || strings.Contains(rec.Body.String(), testEnvKey) {
				t.Fatal("GET leaked a key")
			}
		})
	}
}

func TestConnectionTestUsesExplicitKeyForThatTestOnly(t *testing.T) {
	var sawAuth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = append(sawAuth, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") == "Bearer typed-key-9999" || r.Header.Get("Authorization") == "Bearer "+testWebKey {
			_, _ = w.Write([]byte(`{"models_loaded":true}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	store := &fakeSettingsStore{settings: OCRSettings{Engine: "thaidoc", APIURL: srv.URL}, webKey: testWebKey, writable: true}
	cfg := ocrAPIConfig{APIKey: testEnvKey, Timeout: time.Second}
	router := apiKeyRouter(store, cfg, "ADMIN")

	rec := serve(router, http.MethodPost, "/api/v1/settings/ocr/test", `{"apiKey":"typed-key-9999"}`)
	if !strings.Contains(rec.Body.String(), `"ok"`) || sawAuth[0] != "Bearer typed-key-9999" {
		t.Fatalf("typed key test: body %s, auth %v", rec.Body.String(), sawAuth)
	}
	if store.webKey != testWebKey {
		t.Fatal("testing an unsaved key must not replace the saved key")
	}
	if strings.Contains(rec.Body.String(), "typed-key-9999") {
		t.Fatal("connection test leaked the typed key")
	}

	rec = serve(router, http.MethodPost, "/api/v1/settings/ocr/test", `{}`)
	if !strings.Contains(rec.Body.String(), `"ok"`) || sawAuth[1] != "Bearer "+testWebKey {
		t.Fatalf("without a typed key the saved web key must be used: body %s, auth %v", rec.Body.String(), sawAuth)
	}
}

func TestAPIKeyAuditDetailsKeepOnlyLastFour(t *testing.T) {
	got := ocrAPIKeyAuditDetails("sk-abcdefgh-WXYZ")
	if got != "ลงท้าย WXYZ" || strings.Contains(got, "abcdefgh") {
		t.Fatalf("details = %q", got)
	}
}

func TestAPIKeysNeverAppearInResponsesOrLogs(t *testing.T) {
	logs := captureLog(t)
	store := &fakeSettingsStore{writable: true, webKeyErr: errors.New("message authentication failed")}
	cfg := ocrAPIConfig{APIKey: testEnvKey}
	router := apiKeyRouter(store, cfg, "ADMIN")

	responses := []*httptest.ResponseRecorder{
		serve(router, http.MethodPut, "/api/v1/settings/ocr/api-key", `{"apiKey":"`+testWebKey+`"}`),
		serve(router, http.MethodGet, "/api/v1/settings/ocr", ""),
		serve(router, http.MethodDelete, "/api/v1/settings/ocr/api-key", ""),
		serve(router, http.MethodGet, "/api/v1/settings/ocr", ""),
	}
	for i, rec := range responses {
		for _, secret := range []string{testWebKey, testEnvKey} {
			if strings.Contains(rec.Body.String(), secret) {
				t.Fatalf("response %d leaked %q: %s", i, secret, rec.Body.String())
			}
		}
	}
	for _, secret := range []string{testWebKey, testEnvKey} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("log leaked %q: %s", secret, logs.String())
		}
	}
	if strings.Contains(ocrAPIKeyAuditDetails(testWebKey), "SECRET") {
		t.Fatal("audit details leaked the key")
	}
}
