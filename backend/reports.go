package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type workloadReportRow struct {
	FullName         string `json:"fullName"`
	Position         string `json:"position"`
	DocumentCount    int64  `json:"documentCount"`
	AppointmentCount int64  `json:"appointmentCount"`
	VerifiedCount    int64  `json:"verifiedCount"`
}

func workloadReport(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows := make([]workloadReportRow, 0)
		query := db.Table("appointments AS a").
			Select("COALESCE(MAX(u.full_name), MAX(a.full_name)) AS full_name, COALESCE(MAX(u.position_title), MAX(a.position)) AS position, COUNT(DISTINCT a.document_id) AS document_count, COUNT(*) AS appointment_count, SUM(CASE WHEN a.verified = 1 THEN 1 ELSE 0 END) AS verified_count").
			Joins("JOIN documents AS d ON d.id = a.document_id").
			Joins("LEFT JOIN directory_users AS u ON u.id = a.directory_user_id").
			Where("d.status = ?", "CONFIRMED")
		if q := strings.TrimSpace(c.Query("q")); q != "" {
			like := "%" + q + "%"
			query = query.Where("a.full_name LIKE ? OR a.position LIKE ?", like, like)
		}
		if raw := strings.TrimSpace(c.Query("dateFrom")); raw != "" {
			if parsed, err := parseDateInput(raw); err == nil && parsed != nil {
				query = query.Where("d.issued_date >= ?", *parsed)
			}
		}
		if raw := strings.TrimSpace(c.Query("dateTo")); raw != "" {
			if parsed, err := parseDateInput(raw); err == nil && parsed != nil {
				query = query.Where("d.issued_date < ?", parsed.AddDate(0, 0, 1))
			}
		}
		if err := query.Group("COALESCE(CAST(a.directory_user_id AS CHAR), a.full_name)").Order("appointment_count DESC, full_name ASC").Limit(100).Scan(&rows).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถสร้างรายงานภาระงานได้"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": rows})
	}
}
