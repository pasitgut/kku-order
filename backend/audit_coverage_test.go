package main

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestEveryMutatingRouteDeclaresAuditCoverage walks the real router. A new
// POST/PUT/PATCH/DELETE route fails here until it is listed as audited (with
// the actions it writes) or as unaudited (with the reason it changes nothing).
func TestEveryMutatingRouteDeclaresAuditCoverage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newAPIRouter(apiRouteDeps{})
	live := map[string]bool{}
	for _, route := range router.Routes() {
		switch route.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			continue
		}
		key := route.Method + " " + route.Path
		live[key] = true
		_, audited := auditedRoutes[key]
		_, unaudited := unauditedRoutes[key]
		if !audited && !unaudited {
			t.Errorf("%s changes data or may, but is neither in auditedRoutes nor unauditedRoutes (audit_coverage.go)", key)
		}
		if audited && unaudited {
			t.Errorf("%s is listed as both audited and unaudited", key)
		}
	}
	for key := range auditedRoutes {
		if !live[key] {
			t.Errorf("auditedRoutes lists %s, which is not a mutating route in newAPIRouter", key)
		}
	}
	for key := range unauditedRoutes {
		if !live[key] {
			t.Errorf("unauditedRoutes lists %s, which is not a mutating route in newAPIRouter", key)
		}
	}
}

func TestEveryDeclaredAuditActionHasThaiLabel(t *testing.T) {
	declared := map[string][]string{}
	for key, actions := range auditedRoutes {
		declared[key] = actions
	}
	for job, actions := range auditedJobs {
		declared["job "+job] = actions
	}
	for owner, actions := range declared {
		if len(actions) == 0 {
			t.Errorf("%s declares no audit actions", owner)
		}
		for _, action := range actions {
			if activityLabel(action) == action {
				t.Errorf("%s declares %s, which has no Thai label", owner, action)
			}
		}
	}
}
