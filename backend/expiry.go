package main

import (
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func refreshExpiryNotifications(db *gorm.DB, logger *log.Logger) {
	var documents []Document
	if err := db.Where("expiry_date IS NOT NULL AND expiry_date >= CURRENT_TIMESTAMP").Find(&documents).Error; err != nil {
		logger.Printf("load expiry dates failed: %v", err)
		return
	}
	now := time.Now()
	for _, document := range documents {
		if document.ExpiryDate == nil {
			continue
		}
		var appointments []Appointment
		if err := db.Where("document_id = ?", document.ID).Find(&appointments).Error; err != nil {
			logger.Printf("load appointments for expiry notification failed: %v", err)
			continue
		}
		for _, months := range []int{3, 2, 1} {
			dueAt := document.ExpiryDate.AddDate(0, -months, 0)
			if dueAt.After(now) {
				continue
			}
			noticeType := map[int]string{3: "3_MONTHS", 2: "2_MONTHS", 1: "1_MONTH"}[months]
			if len(appointments) == 0 {
				ensureExpiryNotification(db, logger, document.ID, nil, noticeType, dueAt)
				continue
			}
			for _, appointment := range appointments {
				appointmentID := appointment.ID
				ensureExpiryNotification(db, logger, document.ID, &appointmentID, noticeType, dueAt)
			}
		}
	}
	sendPendingExpiryEmails(db, logger)
}

func ensureExpiryNotification(db *gorm.DB, logger *log.Logger, documentID uint, appointmentID *uint, noticeType string, dueAt time.Time) {
	query := db.Model(&ExpiryNotification{}).Where("document_id = ? AND notice_type = ?", documentID, noticeType)
	if appointmentID == nil {
		query = query.Where("appointment_id IS NULL")
	} else {
		query = query.Where("appointment_id = ?", *appointmentID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		logger.Printf("check expiry notification failed: %v", err)
		return
	}
	if count > 0 {
		return
	}
	if err := db.Create(&ExpiryNotification{DocumentID: documentID, AppointmentID: appointmentID, NoticeType: noticeType, DueAt: dueAt, Status: "PENDING"}).Error; err != nil {
		logger.Printf("create expiry notification failed: %v", err)
	}
}

func sendPendingExpiryEmails(db *gorm.DB, logger *log.Logger) {
	host := strings.TrimSpace(getenv("SMTP_HOST", ""))
	if host == "" {
		return
	}
	port := getenv("SMTP_PORT", "587")
	from := strings.TrimSpace(getenv("SMTP_FROM", ""))
	if from == "" {
		logger.Printf("expiry email skipped: SMTP_FROM is not configured")
		return
	}
	username := getenv("SMTP_USERNAME", "")
	password := getenv("SMTP_PASSWORD", "")
	var auth smtp.Auth
	if username != "" {
		auth = smtp.PlainAuth("", username, password, host)
	}

	var pending []ExpiryNotification
	if err := db.Where("status = ? AND due_at <= CURRENT_TIMESTAMP", "PENDING").Order("due_at ASC").Limit(100).Find(&pending).Error; err != nil {
		logger.Printf("load pending expiry emails failed: %v", err)
		return
	}
	for _, notification := range pending {
		recipient, subject, body, err := expiryEmailContent(db, notification)
		if err != nil || recipient == "" {
			if err != nil {
				logger.Printf("build expiry email %d failed: %v", notification.ID, err)
			}
			continue
		}
		message := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", from, recipient, subject, body))
		if err := smtp.SendMail(host+":"+port, auth, from, []string{recipient}, message); err != nil {
			logger.Printf("send expiry email %d failed: %v", notification.ID, err)
			continue
		}
		now := time.Now()
		if err := db.Model(&notification).Updates(map[string]any{"status": "SENT", "sent_at": now}).Error; err != nil {
			logger.Printf("mark expiry email %d sent failed: %v", notification.ID, err)
		}
	}
}

func expiryEmailContent(db *gorm.DB, notification ExpiryNotification) (string, string, string, error) {
	var document Document
	if err := db.Select("id", "title", "order_no", "expiry_date").First(&document, notification.DocumentID).Error; err != nil {
		return "", "", "", err
	}
	recipient := ""
	personName := "ผู้เกี่ยวข้อง"
	if notification.AppointmentID != nil {
		var appointment Appointment
		if err := db.First(&appointment, *notification.AppointmentID).Error; err != nil {
			return "", "", "", err
		}
		personName = appointment.FullName
		var user DirectoryUser
		if appointment.DirectoryUserID == nil || db.Select("email").First(&user, *appointment.DirectoryUserID).Error != nil {
			return "", "", "", nil
		}
		recipient = strings.TrimSpace(user.Email)
	}
	if recipient == "" {
		return "", "", "", nil
	}
	subject := fmt.Sprintf("DocFlow: แจ้งเตือนคำสั่งใกล้หมดวาระ (%s)", notification.NoticeType)
	body := fmt.Sprintf("เรียน %s\n\nคำสั่ง %s (%s) จะหมดวาระวันที่ %s\nกรุณาตรวจสอบรายละเอียดในระบบ DocFlow\n", personName, document.OrderNo, document.Title, formatDate(document.ExpiryDate))
	return recipient, subject, body, nil
}

func listExpiryNotifications(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var notifications []ExpiryNotification
		if err := db.Where("status = ?", "PENDING").Order("due_at ASC").Limit(100).Find(&notifications).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดการแจ้งเตือนวาระได้"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": notifications})
	}
}
