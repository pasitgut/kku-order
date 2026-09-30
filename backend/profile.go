package main

import (
	"crypto/sha256"
	"encoding/hex"
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
	"DOCUMENT_UPLOADED":        "นำเข้า",
	"DOCUMENT_FOLDER_IMPORTED": "นำเข้า",
	"DOCUMENT_OVERWRITTEN":     "บันทึกทับ",
	"DOCUMENT_FILE_REPLACED":   "อัปโหลดไฟล์ทับ",
	"DOCUMENT_UPDATED":         "แก้ไข",
	"DOCUMENT_CONFIRMED":       "ยืนยัน",
	"DIRECTORY_USER_CREATED":   "เพิ่มบุคคล",
	"SETTINGS_OCR_UPDATED":     "ตั้งค่า OCR",
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
		case "DOCUMENT_UPLOADED", "DOCUMENT_FOLDER_IMPORTED", "DOCUMENT_OVERWRITTEN":
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
		var user DirectoryUser
		if err := db.Where("user_id = ? OR username = ? OR email = ?", userID, userID, userID).Take(&user).Error; err == nil {
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
		if err := db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&profile).Error; err != nil {
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
		if err := db.Model(&UserProfile{}).Where("user_id = ?", userID).Updates(map[string]any{"photo_path": "", "photo_updated_at": nil}).Error; err != nil {
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
			item := activityItem{ID: entry.ID, Action: entry.Action, Label: activityLabel(entry.Action), DocumentID: entry.DocumentID, Details: entry.Details, CreatedAt: entry.CreatedAt}
			if entry.DocumentID != nil {
				item.DocumentTitle = titles[*entry.DocumentID]
			}
			// Settings payloads are raw JSON and only useful in the audit table.
			if entry.Action == "SETTINGS_OCR_UPDATED" {
				item.Details = ""
			}
			items = append(items, item)
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"stats": summarizeActivity(monthLogs, now), "items": items}})
	}
}
