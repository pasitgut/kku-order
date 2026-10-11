package main

import (
	"io"
	"log"
	"testing"

	"github.com/gin-gonic/gin"
)

// The router is built by the same function main() uses. Gin panics on a
// duplicate method+path, so a second registration fails this test.
func TestAPIRouterRegistersEachRouteOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := log.New(io.Discard, "", 0)
	processor := NewDocumentProcessor(nil, logger, ocrAPIConfig{})
	router := newAPIRouter(apiRouteDeps{processor: processor})

	seen := map[string]bool{}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		if seen[key] {
			t.Errorf("route registered twice: %s", key)
		}
		seen[key] = true
	}
	if !seen["POST /api/v1/documents/:id/reprocess"] {
		t.Fatalf("reprocess route missing")
	}
}
