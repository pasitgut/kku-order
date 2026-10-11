package main

import (
	"errors"
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
	if err := db.Where("status <> ?", documentCancelled).Where("expiry_date IS NOT NULL AND expiry_date >= CURRENT_TIMESTAMP").Find(&documents).Error; err != nil {
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
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := lockEditableDocument(tx, documentID); err != nil {
			return err
		}
		query := tx.Model(&ExpiryNotification{}).Where("document_id = ? AND notice_type = ?", documentID, noticeType)
		if appointmentID == nil {
			query = query.Where("appointment_id IS NULL")
		} else {
			query = query.Where("appointment_id = ?", *appointmentID)
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.Create(&ExpiryNotification{DocumentID: documentID, AppointmentID: appointmentID, NoticeType: noticeType, DueAt: dueAt, Status: "PENDING"}).Error; err != nil {
			return err
		}
		docID := documentID
		return tx.Create(&AuditLog{DocumentID: &docID, UserID: auditSystemUser("expiry-notice"), Action: "EXPIRY_NOTICE_CREATED", Details: expiryNoticeDetails(noticeType, dueAt)}).Error
	})
	if err != nil && !errors.Is(err, errDocumentCancelled) {
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
	if err := db.Where("document_id IN (?)", db.Model(&Document{}).Select("id").Where("status <> ?", documentCancelled)).Where("status = ? AND due_at <= CURRENT_TIMESTAMP", "PENDING").Order("due_at ASC").Limit(100).Find(&pending).Error; err != nil {
		logger.Printf("load pending expiry emails failed: %v", err)
		return
	}
	for _, notification := range pending {
		// Dispatch and cancellation take the same document lock. A notification
		// fetched before cancellation cannot be sent after cancellation succeeds.
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := lockEditableDocument(tx, notification.DocumentID); err != nil {
				return err
			}
			var current ExpiryNotification
			if err := tx.First(&current, notification.ID).Error; err != nil {
				return err
			}
			if current.Status != "PENDING" {
				return nil
			}
			recipient, subject, body, err := expiryEmailContent(tx, current)
			if err != nil {
				return err
			}
			actor := auditSystemUser("expiry-notice")
			if recipient == "" {
				if err := tx.Model(&current).Update("status", "SKIPPED").Error; err != nil {
					return err
				}
				return tx.Create(&AuditLog{DocumentID: &current.DocumentID, UserID: actor, Action: "EXPIRY_EMAIL_SKIPPED", Details: "ไม่มีอีเมลผู้รับ"}).Error
			}
			message := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", from, recipient, subject, body))
			if err := smtp.SendMail(host+":"+port, auth, from, []string{recipient}, message); err != nil {
				return err
			}
			if err := tx.Model(&current).Updates(map[string]any{"status": "SENT", "sent_at": time.Now()}).Error; err != nil {
				return err
			}
			return tx.Create(&AuditLog{DocumentID: &current.DocumentID, UserID: actor, Action: "EXPIRY_EMAIL_SENT", Details: "ส่งถึง " + recipient}).Error
		})
		if err != nil && !errors.Is(err, errDocumentCancelled) {
			logger.Printf("dispatch expiry email %d failed: %v", notification.ID, err)
		}
	}
}

func expiryEmailContent(db *gorm.DB, notification ExpiryNotification) (string, string, string, error) {
	var document Document
	if err := db.Select("id", "title", "order_no", "expiry_date", "status").First(&document, notification.DocumentID).Error; err != nil {
		return "", "", "", err
	}
	if document.Status == documentCancelled {
		return "", "", "", nil
	}
	recipient := ""
	personName := "ผู้เกี่ยวข้อง"
	if notification.AppointmentID != nil {
		var appointment Appointment
		if err := db.First(&appointment, *notification.AppointmentID).Error; err != nil {
			return "", "", "", err
		}
		personName = appointment.FullName
		var person DirectoryPerson
		if appointment.DirectoryUserID == nil || db.Select("email").First(&person, *appointment.DirectoryUserID).Error != nil || person.Email == nil {
			return "", "", "", nil
		}
		recipient = strings.TrimSpace(*person.Email)
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
		if err := db.Where("document_id IN (?)", db.Model(&Document{}).Select("id").Where("status <> ?", documentCancelled)).Where("status = ?", "PENDING").Order("due_at ASC").Limit(100).Find(&notifications).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดการแจ้งเตือนวาระได้"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": notifications})
	}
}
