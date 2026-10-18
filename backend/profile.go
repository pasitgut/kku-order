package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxProfilePhotoBytes = 2 << 20

var profilePhotoTypes = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}

// UserProfile keeps what DocFlow adds on top of the SSO account.
type UserProfile struct {
	UserID         string     `gorm:"column:user_id;primaryKey;size:255"`
	PhotoPath      string     `gorm:"column:photo_path;size:500"`
	PhotoUpdatedAt *time.Time `gorm:"column:photo_updated_at"`
}

func (UserProfile) TableName() string { return "user_profiles" }

type meResponse struct {
	UserID          string `json:"userId"`
	Role            string `json:"role"`
	DisplayName     string `json:"displayName"`
	Email           string `json:"email"`
	PositionTitle   string `json:"positionTitle"`
	Department      string `json:"department"`
	DirectoryUserID *uint  `json:"directoryUserId,omitempty"`
	HasPhoto        bool   `json:"hasPhoto"`
	PhotoVersion    int64  `json:"photoVersion"`
}

type activityStats struct {
	Imported  int `json:"imported"`
	Confirmed int `json:"confirmed"`
	Edited    int `json:"edited"`
}

type activityItem struct {
	ID            uint      `json:"id"`
	Action        string    `json:"action"`
	Label         string    `json:"label"`
	DocumentID    *uint     `json:"documentId,omitempty"`
	DocumentTitle string    `json:"documentTitle"`
	Details       string    `json:"details"`
	CreatedAt     time.Time `json:"createdAt"`
}

var activityLabels = map[string]string{
	"DOCUMENT_UPLOADED":            "นำเข้า",
	"DOCUMENT_FOLDER_IMPORTED":     "นำเข้า",
	"DOCUMENT_OVERWRITTEN":         "บันทึกทับ",
	"DOCUMENT_FILE_REPLACED":       "อัปโหลดไฟล์ทับ",
	"DOCUMENT_UPDATED":             "แก้ไข",
	"DOCUMENT_CANCELLED":           "ยกเลิกเอกสาร",
	"DOCUMENT_DELETED":             "ลบเอกสาร",
	"DOCUMENT_CONFIRMED":           "ยืนยัน",
	"DOCUMENT_RETURNED":            "ส่งกลับไปตรวจสอบ",
	"AUTO_CONFIRM_CHANGED":         "ตั้งค่ายืนยันอัตโนมัติ",
	"DIRECTORY_USER_CREATED":       "เพิ่มบุคคล",
	"POSITION_HOLDER_SET":          "กำหนดผู้ดำรงตำแหน่ง",
	"SETTINGS_OCR_UPDATED":         "ตั้งค่า OCR",
	"OCR_API_KEY_UPDATED":          "ตั้งค่า API key OCR",
	"OCR_API_KEY_REMOVED":          "ลบ API key OCR",
	"DOCUMENT_RESCAN_REQUESTED":    "สแกนใหม่",
	"DOCUMENT_REPROCESS_REQUESTED": "ประมวลผลใหม่",
	"DOCUMENT_EXTRACTION_FAILED":   "ประมวลผลไม่สำเร็จ",
	"DOCUMENT_PROCESSED":           "ประมวลผลเสร็จ",
	"DOCUMENT_IMPORTED":            "นำเข้า",
	"DOCUMENT_DRIVE_IMPORTED":      "นำเข้าจาก Google Drive",
	"DRIVE_SYNC_STARTED":           "เริ่ม sync Google Drive",
	"DRIVE_SYNC_FINISHED":          "sync Google Drive เสร็จ",
	"DRIVE_SYNC_PAUSED":            "sync Google Drive หยุดรอ",
	"DIRECTORY_SYNCED":             "sync รายชื่อบุคลากร",
	"PROFILE_PHOTO_UPDATED":        "เปลี่ยนรูปโปรไฟล์",
	"PROFILE_PHOTO_REMOVED":        "ลบรูปโปรไฟล์",
	"EXPIRY_NOTICE_CREATED":        "สร้างการแจ้งเตือนหมดวาระ",
	"EXPIRY_EMAIL_SENT":            "ส่งอีเมลแจ้งหมดวาระ",
	"EXPIRY_EMAIL_SKIPPED":         "ข้ามอีเมลแจ้งหมดวาระ",
}

// deletedDocumentDetails is the Details payload of a DOCUMENT_DELETED row. The
// document row is gone by then, so the title is stored with the entry.
type deletedDocumentDetails struct {
	Title            string `json:"title"`
	OriginalFilename string `json:"originalFilename"`
}

// buildActivityItem turns an audit row into a profile item. A document link is
// only kept when the document still exists.
func buildActivityItem(entry AuditLog, titles map[uint]string) activityItem {
	item := activityItem{ID: entry.ID, Action: entry.Action, Label: activityLabel(entry.Action), DocumentID: entry.DocumentID, Details: entry.Details, CreatedAt: entry.CreatedAt}
	if entry.DocumentID != nil {
		if title, ok := titles[*entry.DocumentID]; ok {
			item.DocumentTitle = title
		} else {
			item.DocumentID = nil
		}
	}
	switch entry.Action {
	case "DOCUMENT_DELETED":
		var deleted deletedDocumentDetails
		if err := json.Unmarshal([]byte(entry.Details), &deleted); err == nil {
			item.DocumentTitle = firstNonEmpty(deleted.Title, deleted.OriginalFilename)
			item.Details = deleted.OriginalFilename
		}
	case "SETTINGS_OCR_UPDATED":
		// Settings payloads are raw JSON and only useful in the audit table.
		item.Details = ""
	}
	return item
}

func activityLabel(action string) string {
	if label, ok := activityLabels[action]; ok {
		return label
	}
	return action
}

func summarizeActivity(logs []AuditLog, now time.Time) activityStats {
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	var stats activityStats
	for _, entry := range logs {
		if entry.CreatedAt.Before(monthStart) {
			continue
		}
		switch entry.Action {
		case "DOCUMENT_UPLOADED", "DOCUMENT_FOLDER_IMPORTED", "DOCUMENT_DRIVE_IMPORTED", "DOCUMENT_OVERWRITTEN":
			stats.Imported++
		case "DOCUMENT_CONFIRMED":
			stats.Confirmed++
		case "DOCUMENT_UPDATED", "DOCUMENT_FILE_REPLACED":
			stats.Edited++
		}
	}
	return stats
}

func validateProfilePhoto(data []byte) (string, error) {
	if len(data) > maxProfilePhotoBytes {
		return "", errors.New("รูปต้องมีขนาดไม่เกิน 2 MB")
	}
	extension, ok := profilePhotoTypes[http.DetectContentType(data)]
	if !ok {
		return "", errors.New("รองรับเฉพาะรูป JPG, PNG หรือ WebP")
	}
	return extension, nil
}

// profilePhotoPath hashes the SSO user ID so any ID maps to one safe file name.
func profilePhotoPath(directory, userID, extension string) string {
	sum := sha256.Sum256([]byte(userID))
	return filepath.Join(directory, hex.EncodeToString(sum[:16])+extension)
}

func profilePhotoDir() string {
	return filepath.Join(getenv("UPLOAD_DIR", "./storage"), "profile-photos")
}

func loadUserProfile(db *gorm.DB, userID string) (UserProfile, bool) {
	var profile UserProfile
	if err := db.Where("user_id = ?", userID).Take(&profile).Error; err != nil {
		return UserProfile{UserID: userID}, false
	}
	return profile, profile.PhotoPath != ""
}

func meHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := currentUserID(c)
		role, _ := c.Get(authRoleKey)
		response := meResponse{UserID: userID, Role: fmt.Sprint(role), DisplayName: userID}

		// SSO sends an account ID; the directory tells us the person's name.
		if user, found := loadDirectoryUserForAccount(db, userID); found {
			response.DirectoryUserID = &user.ID
			if name := strings.TrimSpace(displayDirectoryName(user)); name != "" {
				response.DisplayName = name
			}
			response.Email = user.Email
			response.PositionTitle = firstNonEmpty(user.PositionTitle, user.JobTitle)
			response.Department = firstNonEmpty(user.Department, user.Faculty)
		}
		if profile, ok := loadUserProfile(db, userID); ok && profile.PhotoUpdatedAt != nil {
			response.HasPhoto = true
			response.PhotoVersion = profile.PhotoUpdatedAt.Unix()
		}
		c.JSON(http.StatusOK, gin.H{"data": response})
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func getMyPhotoHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		profile, ok := loadUserProfile(db, currentUserID(c))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "ยังไม่มีรูปโปรไฟล์"})
			return
		}
		c.Header("Cache-Control", "private, max-age=0, must-revalidate")
		c.File(profile.PhotoPath)
	}
}

func uploadMyPhotoHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		header, err := c.FormFile("photo")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาเลือกรูป"})
			return
		}
		file, err := header.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "อ่านรูปไม่สำเร็จ"})
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxProfilePhotoBytes+1))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "อ่านรูปไม่สำเร็จ"})
			return
		}
		extension, err := validateProfilePhoto(data)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		userID := currentUserID(c)
		directory := profilePhotoDir()
		if err := os.MkdirAll(directory, 0o750); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "เตรียมที่เก็บรูปไม่สำเร็จ"})
			return
		}
		previous, hadPhoto := loadUserProfile(db, userID)
		path := profilePhotoPath(directory, userID, extension)
		if err := os.WriteFile(path, data, 0o640); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "บันทึกรูปไม่สำเร็จ"})
			return
		}
		now := time.Now()
		profile := UserProfile{UserID: userID, PhotoPath: path, PhotoUpdatedAt: &now}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&profile).Error; err != nil {
				return err
			}
			return tx.Create(&AuditLog{UserID: userID, Action: "PROFILE_PHOTO_UPDATED", Details: extension}).Error
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "บันทึกรูปไม่สำเร็จ"})
			return
		}
		// A new file type leaves the old file behind under another extension.
		if hadPhoto && previous.PhotoPath != path {
			_ = os.Remove(previous.PhotoPath)
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"hasPhoto": true, "photoVersion": now.Unix()}})
	}
}

func deleteMyPhotoHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := currentUserID(c)
		profile, ok := loadUserProfile(db, userID)
		if ok {
			_ = os.Remove(profile.PhotoPath)
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&UserProfile{}).Where("user_id = ?", userID).Updates(map[string]any{"photo_path": "", "photo_updated_at": nil}).Error; err != nil {
				return err
			}
			return tx.Create(&AuditLog{UserID: userID, Action: "PROFILE_PHOTO_REMOVED"}).Error
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ลบรูปไม่สำเร็จ"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"hasPhoto": false}})
	}
}

func myActivityHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := currentUserID(c)
		now := time.Now()
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

		var monthLogs []AuditLog
		if err := db.Where("user_id = ? AND created_at >= ?", userID, monthStart).Find(&monthLogs).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดกิจกรรมไม่สำเร็จ"})
			return
		}
		var recent []AuditLog
		if err := db.Where("user_id = ?", userID).Order("created_at DESC").Limit(10).Find(&recent).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดกิจกรรมไม่สำเร็จ"})
			return
		}

		documentIDs := make([]uint, 0, len(recent))
		for _, entry := range recent {
			if entry.DocumentID != nil {
				documentIDs = append(documentIDs, *entry.DocumentID)
			}
		}
		titles := map[uint]string{}
		if len(documentIDs) > 0 {
			var documents []Document
			if err := db.Select("id", "title", "original_filename").Where("id IN ?", documentIDs).Find(&documents).Error; err == nil {
				for _, document := range documents {
					titles[document.ID] = firstNonEmpty(document.Title, document.OriginalFilename)
				}
			}
		}
		items := make([]activityItem, 0, len(recent))
		for _, entry := range recent {
			items = append(items, buildActivityItem(entry, titles))
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"stats": summarizeActivity(monthLogs, now), "items": items}})
	}
}
