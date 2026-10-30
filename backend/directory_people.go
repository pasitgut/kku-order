package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type DirectoryPersonRevision struct {
	ID                uint                    `json:"id" gorm:"primaryKey"`
	DirectoryPersonID uint                    `json:"personId" gorm:"column:directory_person_id;not null;index"`
	ChangesJSON       string                  `json:"-" gorm:"column:changes;type:json;not null"`
	ChangedBy         string                  `json:"changedBy" gorm:"size:255;not null"`
	ChangedAt         time.Time               `json:"changedAt" gorm:"column:changed_at;not null"`
	Changes           []DirectoryPersonChange `json:"changes" gorm:"-"`
}

func (DirectoryPersonRevision) TableName() string { return "directory_person_revisions" }

type DirectoryPersonChange struct {
	Field    string `json:"field"`
	OldValue string `json:"oldValue"`
	NewValue string `json:"newValue"`
}

var errDirectoryPersonNameConflict = errors.New("มีบุคคลชื่อนี้ในระบบแล้ว")

type directoryLocalOverrideFlags map[string]bool

func decodeDirectoryLocalOverrides(raw *string) directoryLocalOverrideFlags {
	flags := directoryLocalOverrideFlags{}
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return flags
	}
	if err := json.Unmarshal([]byte(*raw), &flags); err != nil || flags == nil {
		return directoryLocalOverrideFlags{}
	}
	return flags
}

func recordDirectoryPersonRevision(tx *gorm.DB, personID uint, changes []DirectoryPersonChange, changedBy string, changedAt time.Time) error {
	if len(changes) == 0 {
		return nil
	}
	encoded, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	return tx.Create(&DirectoryPersonRevision{
		DirectoryPersonID: personID,
		ChangesJSON:       string(encoded),
		ChangedBy:         changedBy,
		ChangedAt:         changedAt,
	}).Error
}

type updateDirectoryUserRequest struct {
	Prefix        string `json:"prefix"`
	FirstName     string `json:"firstName"`
	LastName      string `json:"lastName"`
	PositionTitle string `json:"positionTitle"`
	Department    string `json:"department"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
}

func updateDirectoryUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		personID, ok := parseDirectoryPersonID(c.Param("id"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รหัสบุคคลไม่ถูกต้อง"})
			return
		}
		var payload updateDirectoryUserRequest
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
			return
		}
		payload.Prefix = strings.TrimSpace(payload.Prefix)
		payload.FirstName = strings.TrimSpace(payload.FirstName)
		payload.LastName = strings.TrimSpace(payload.LastName)
		payload.PositionTitle = strings.TrimSpace(payload.PositionTitle)
		payload.Department = strings.TrimSpace(payload.Department)
		payload.Email = strings.TrimSpace(payload.Email)
		payload.Phone = strings.TrimSpace(payload.Phone)
		if payload.FirstName == "" || payload.LastName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาระบุชื่อและนามสกุล"})
			return
		}
		for _, field := range []struct {
			name  string
			value string
			limit int
		}{
			{"คำนำหน้า", payload.Prefix, 50},
			{"ชื่อ", payload.FirstName, 255},
			{"นามสกุล", payload.LastName, 255},
			{"ตำแหน่ง", payload.PositionTitle, 255},
			{"หน่วยงาน", payload.Department, 255},
			{"อีเมล", payload.Email, 255},
			{"เบอร์โทรศัพท์", payload.Phone, 20},
		} {
			if len([]rune(field.value)) > field.limit {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%sยาวเกิน %d ตัวอักษร", field.name, field.limit)})
				return
			}
		}

		changedBy := currentUserID(c)
		changedAt := time.Now()
		var updated DirectoryUser
		err := db.Transaction(func(tx *gorm.DB) error {
			var person DirectoryPerson
			if err := tx.First(&person, personID).Error; err != nil {
				return err
			}
			var profile DirectoryPersonProfile
			profileErr := tx.First(&profile, "person_id = ?", personID).Error
			if profileErr != nil && !errors.Is(profileErr, gorm.ErrRecordNotFound) {
				return profileErr
			}
			if errors.Is(profileErr, gorm.ErrRecordNotFound) {
				source := sourceSynced
				if person.ExternalID == nil {
					source = sourceManual
				}
				profile = DirectoryPersonProfile{PersonID: person.ID, Source: source}
			}

			before := directoryPersonAPI(person, profile)
			if directoryUserKey(before) != normalizePersonName(payload.FirstName+" "+payload.LastName) {
				users, _, err := loadDirectory(tx)
				if err != nil {
					return err
				}
				newNameKey := normalizePersonName(payload.FirstName + " " + payload.LastName)
				for _, existing := range users {
					if existing.ID != personID && directoryUserKey(existing) == newNameKey {
						return errDirectoryPersonNameConflict
					}
				}
			}
			changes := make([]DirectoryPersonChange, 0, 7)
			flags := decodeDirectoryLocalOverrides(profile.LocalOverrides)
			addChange := func(field, oldValue, newValue string) {
				if oldValue != newValue {
					changes = append(changes, DirectoryPersonChange{Field: field, OldValue: oldValue, NewValue: newValue})
					flags[field] = true
				}
			}
			addChange("prefix", before.Prefix, payload.Prefix)
			addChange("firstName", before.FirstName, payload.FirstName)
			addChange("lastName", before.LastName, payload.LastName)
			addChange("positionTitle", before.PositionTitle, payload.PositionTitle)
			addChange("department", before.Department, payload.Department)
			addChange("email", before.Email, payload.Email)
			addChange("phone", before.Phone, payload.Phone)
			if len(changes) == 0 {
				updated = before
				return nil
			}

			encodedFlags, err := json.Marshal(flags)
			if err != nil {
				return err
			}
			encodedFlagsValue := string(encodedFlags)
			person.Prefix = payload.Prefix
			person.FirstName = payload.FirstName
			person.LastName = payload.LastName
			person.PositionTitle = payload.PositionTitle
			person.Email = nonEmptySQLValue(payload.Email)
			person.Phone = nonEmptySQLValue(payload.Phone)
			person.UpdatedAt = changedAt
			person.FullName = ""
			personUpdates := map[string]any{
				"prefix": payload.Prefix, "first_name": payload.FirstName, "last_name": payload.LastName,
				"position_title": payload.PositionTitle, "email": person.Email, "phone": person.Phone,
				"updated_at": changedAt,
			}
			if err := tx.Model(&DirectoryPerson{}).Where("id = ?", personID).Updates(personUpdates).Error; err != nil {
				return err
			}
			profile.Department = payload.Department
			profile.LocalOverrides = &encodedFlagsValue
			if errors.Is(profileErr, gorm.ErrRecordNotFound) {
				if err := tx.Create(&profile).Error; err != nil {
					return err
				}
			} else if err := tx.Model(&DirectoryPersonProfile{}).Where("person_id = ?", personID).Updates(map[string]any{
				"department": profile.Department, "local_overrides": encodedFlagsValue,
			}).Error; err != nil {
				return err
			}
			if err := recordDirectoryPersonRevision(tx, personID, changes, changedBy, changedAt); err != nil {
				return err
			}
			fields := make([]string, 0, len(changes))
			for _, change := range changes {
				fields = append(fields, change.Field)
			}
			if err := tx.Create(&AuditLog{UserID: changedBy, Action: "DIRECTORY_USER_UPDATED", Details: fmt.Sprintf("personId=%d fields=%s", personID, strings.Join(fields, ","))}).Error; err != nil {
				return err
			}
			updated = directoryPersonAPI(person, profile)
			return nil
		})
		if errors.Is(err, errDirectoryPersonNameConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": errDirectoryPersonNameConflict.Error()})
			return
		}
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบบุคคลนี้ในระบบ"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกข้อมูลอาจารย์ได้"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": updated})
	}
}

func listDirectoryPersonRevisions(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		personID, ok := parseDirectoryPersonID(c.Param("id"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รหัสบุคคลไม่ถูกต้อง"})
			return
		}
		var person DirectoryPerson
		if err := db.Select("id").First(&person, personID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบบุคคลนี้ในระบบ"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบข้อมูลอาจารย์ได้"})
			return
		}
		var revisions []DirectoryPersonRevision
		if err := db.Where("directory_person_id = ?", personID).Order("changed_at DESC").Order("id DESC").Limit(50).Find(&revisions).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดประวัติการแก้ไขได้"})
			return
		}
		for index := range revisions {
			if err := json.Unmarshal([]byte(revisions[index].ChangesJSON), &revisions[index].Changes); err != nil {
				revisions[index].Changes = []DirectoryPersonChange{}
			}
			revisions[index].ChangesJSON = ""
		}
		c.JSON(http.StatusOK, gin.H{"data": revisions})
	}
}

func parseDirectoryPersonID(raw string) (uint, bool) {
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	return uint(value), err == nil && value > 0
}
