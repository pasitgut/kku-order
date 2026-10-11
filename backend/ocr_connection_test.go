package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// performRequest ส่ง request หนึ่งครั้งผ่าน gin router ที่ mount handlers บน route (รองรับ :param)
func performRequest(t *testing.T, method, route, requestPath string, body io.Reader, handlers ...gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Handle(method, route, handlers...)
	req := httptest.NewRequest(method, requestPath, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// fakeRole ตั้ง user และ role ใน context เหมือน middleware จริง (ดู auth.go)
func fakeRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(authUserKey, "tester")
		c.Set(authRoleKey, role)
		c.Next()
	}
}

func TestOCRHealthURLPerEngine(t *testing.T) {
	got, err := ocrHealthURL("thaidoc", "https://ocr.example:8443")
	if err != nil || got != "https://ocr.example:8443/health" {
		t.Fatalf("thaidoc = %q, %v", got, err)
	}
	got, err = ocrHealthURL("docflow-contract", "https://ocr.example/extract/v2")
	if err != nil || got != "https://ocr.example/health" {
		t.Fatalf("contract = %q, %v", got, err)
	}
}

func TestCheckOCRConnectionStatuses(t *testing.T) {
	cases := []struct {
		name     string
		handler  http.HandlerFunc
		wantStat string
	}{
		{"ok", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"ok","models_loaded":true}`))
		}, "ok"},
		{"unauthorized", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }, "unauthorized"},
		{"forbidden", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }, "unauthorized"},
		{"models loading", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"ok","models_loaded":false}`))
		}, "unreachable"},
		{"server error", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }, "unreachable"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(c.handler)
			defer srv.Close()
			got := checkOCRConnection(context.Background(), &http.Client{Timeout: time.Second}, srv.URL+"/health", "k")
			if got.Status != c.wantStat || got.Detail == "" {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestCheckOCRConnectionSendsBearerKey(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"models_loaded":true}`))
	}))
	defer srv.Close()
	checkOCRConnection(context.Background(), &http.Client{Timeout: time.Second}, srv.URL, "secret")
	if auth != "Bearer secret" {
		t.Fatalf("Authorization = %q", auth)
	}
}

func TestCheckOCRConnectionUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	u := srv.URL
	srv.Close()
	got := checkOCRConnection(context.Background(), &http.Client{Timeout: time.Second}, u+"/health", "")
	if got.Status != "unreachable" {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestOCRConnectionHandlerUsesUnsavedValuesAndHidesKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models_loaded":true}`))
	}))
	defer srv.Close()

	load := func() (OCRSettings, error) {
		return OCRSettings{Engine: "thaidoc", APIURL: "https://stored.example"}, nil
	}
	cfg := ocrAPIConfig{APIKey: "TOPSECRET-KEY", Timeout: time.Second}
	body := strings.NewReader(`{"engine":"thaidoc","apiUrl":"` + srv.URL + `"}`)
	rec := performRequest(t, http.MethodPost, "/api/v1/settings/ocr/test", "/api/v1/settings/ocr/test", body, testOCRConnectionHandler(load, cfg))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "TOPSECRET-KEY") {
		t.Fatal("response leaked the API key")
	}
	var out ocrConnectionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Status != "ok" {
		t.Fatalf("out = %+v err=%v", out, err)
	}
}

func TestOCRConnectionHandlerRejectsRetiredEngine(t *testing.T) {
	load := func() (OCRSettings, error) { return defaultOCRSettings(), nil }
	body := strings.NewReader(`{"engine":"one-ocr"}`)
	rec := performRequest(t, http.MethodPost, "/api/v1/settings/ocr/test", "/api/v1/settings/ocr/test", body, testOCRConnectionHandler(load, ocrAPIConfig{}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
}
