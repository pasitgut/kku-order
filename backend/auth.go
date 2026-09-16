package main

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	authUserKey = "docflow.auth.user"
	authRoleKey = "docflow.auth.role"
)

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userHeader := getenv("AUTH_USER_HEADER", "X-Auth-Request-User")
		roleHeader := getenv("AUTH_ROLE_HEADER", "X-Auth-Request-Role")
		userID := strings.TrimSpace(c.GetHeader(userHeader))
		role := normalizeRole(c.GetHeader(roleHeader))
		if userID == "" && authRequired() {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "กรุณาเข้าสู่ระบบด้วยบัญชีมหาวิทยาลัย"})
			return
		}
		if userID == "" {
			userID = "local-user"
		}
		if role == "" {
			fallbackRole := "ADMIN"
			if authRequired() {
				fallbackRole = "VIEWER"
			}
			role = normalizeRole(getenv("AUTH_DEFAULT_ROLE", fallbackRole))
		}
		c.Set(authUserKey, userID)
		c.Set(authRoleKey, role)
		c.Next()
	}
}

func requireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, role := range roles {
		allowed[normalizeRole(role)] = true
	}
	return func(c *gin.Context) {
		value, exists := c.Get(authRoleKey)
		role, _ := value.(string)
		if !exists || !allowed[normalizeRole(role)] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "ไม่มีสิทธิ์ดำเนินการนี้"})
			return
		}
		c.Next()
	}
}

func currentUserID(c *gin.Context) string {
	if value, exists := c.Get(authUserKey); exists {
		if userID, ok := value.(string); ok && userID != "" {
			return userID
		}
	}
	return "local-user"
}

func normalizeRole(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func authRequired() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("AUTH_REQUIRED")))
	return value == "1" || value == "true" || value == "yes"
}
