package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthMiddlewareRejectsMissingIdentityWhenRequired(t *testing.T) {
	t.Setenv("AUTH_REQUIRED", "true")
	router := gin.New()
	router.Use(authMiddleware())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddlewareAllowsLocalAdminForDevelopment(t *testing.T) {
	t.Setenv("AUTH_REQUIRED", "false")
	router := gin.New()
	router.Use(authMiddleware(), requireRole("ADMIN", "STAFF"))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestAuthMiddlewareViewerCannotWrite(t *testing.T) {
	t.Setenv("AUTH_REQUIRED", "true")
	router := gin.New()
	router.Use(authMiddleware(), requireRole("ADMIN", "STAFF"))
	router.POST("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.Header.Set("X-Auth-Request-User", "viewer@example.ac.th")
	request.Header.Set("X-Auth-Request-Role", "VIEWER")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}
