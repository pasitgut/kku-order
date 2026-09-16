package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRedactDSNDoesNotExposePassword(t *testing.T) {
	got := redactDSN("docflow:super-secret@tcp(localhost:3306)/docflow?parseTime=True")
	want := "docflow:<redacted>@tcp(localhost:3306)/docflow?parseTime=True"
	if got != want {
		t.Fatalf("redactDSN() = %q, want %q", got, want)
	}
	if strings.Contains(got, "super-secret") {
		t.Fatal("redactDSN exposed the database password")
	}
}

func TestRedactDSNHandlesInvalidValue(t *testing.T) {
	if got := redactDSN("not-a-dsn"); got != "<invalid DB_DSN>" {
		t.Fatalf("redactDSN() = %q, want invalid marker", got)
	}
}

func TestCORSPreflightAllowsDeveloperDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(corsMiddleware())
	router.DELETE("/api/v1/documents/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/documents/1", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Access-Control-Request-Method", http.MethodDelete)
	request.Header.Set("Access-Control-Request-Headers", "content-type,x-auth-request-role")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if !strings.Contains(response.Header().Get("Access-Control-Allow-Methods"), http.MethodDelete) {
		t.Fatalf("allow methods = %q, want DELETE", response.Header().Get("Access-Control-Allow-Methods"))
	}
	allowHeaders := response.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(strings.ToLower(allowHeaders), "x-auth-request-role") {
		t.Fatalf("allow headers = %q, want auth headers", allowHeaders)
	}
}
