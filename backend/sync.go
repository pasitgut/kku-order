package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const defaultExternalUsersURL = "https://fs.computing.kku.ac.th/api/ext/v1/users"

type UserSyncResult struct {
	RunID     uint `json:"runId"`
	Fetched   int  `json:"fetched"`
	Created   int  `json:"created"`
	Updated   int  `json:"updated"`
	Unchanged int  `json:"unchanged"`
}

type UserSyncService struct {
	db      *gorm.DB
	client  *http.Client
	baseURL string
	apiKey  string
	logger  *log.Logger
	maxBody int64
}

func NewUserSyncService(db *gorm.DB, logger *log.Logger) *UserSyncService {
	return &UserSyncService{
		db:      db,
		client:  &http.Client{Timeout: 60 * time.Second},
		baseURL: normalizeUsersEndpoint(getenv("EXTERNAL_USERS_URL", getenv("EXTERNAL_USERS_BASE_URL", defaultExternalUsersURL))),
		apiKey:  normalizeAPIKey(getenv("API_KEY", "")),
		logger:  logger,
		maxBody: 20 << 20,
	}
}

// SyncUsers runs one directory sync. actor is the audit UserID: the signed-in
// account for a manual sync, or auditSystemUser("user-sync") for the schedule.
func (s *UserSyncService) SyncUsers(ctx context.Context, actor string) (UserSyncResult, error) {
	started := time.Now()
	run := UserSyncRun{StartedAt: started, Status: "RUNNING"}
	if err := s.db.Create(&run).Error; err != nil {
		return UserSyncResult{}, fmt.Errorf("create sync run: %w", err)
	}
	result := UserSyncResult{RunID: run.ID}

	finish := func(status string, syncErr error) (UserSyncResult, error) {
		completed := time.Now()
		run.CompletedAt = &completed
		run.Status = status
		run.Fetched = result.Fetched
		run.Created = result.Created
		run.Updated = result.Updated
		run.Unchanged = result.Unchanged
		if syncErr != nil {
			run.ErrorMessage = syncErr.Error()
		}
		if err := s.db.Save(&run).Error; err != nil && syncErr == nil {
			return result, fmt.Errorf("save sync run: %w", err)
		}
		if err := s.db.Create(&AuditLog{UserID: actor, Action: "DIRECTORY_SYNCED", Details: syncRunAuditDetails(result, syncErr)}).Error; err != nil {
			s.logger.Printf("directory sync audit not written: %v", err)
		}
		return result, syncErr
	}

	if strings.TrimSpace(s.apiKey) == "" {
		return finish("FAILED", errors.New("API_KEY is not configured"))
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL, nil)
	if err != nil {
		return finish("FAILED", fmt.Errorf("build users request: %w", err))
	}
	request.Header.Set("Authorization", "Bearer "+s.apiKey)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "DocFlow/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return finish("FAILED", fmt.Errorf("request users API: %w", err))
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		if response.StatusCode == http.StatusForbidden {
			return finish("FAILED", fmt.Errorf("users API returned 403 Forbidden: ตรวจสอบ API_KEY, สิทธิ์ของ key และ IP allowlist ของระบบภายนอก"))
		}
		return finish("FAILED", fmt.Errorf("users API returned %s: %s", response.Status, strings.TrimSpace(string(body))))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, s.maxBody+1))
	if err != nil {
		return finish("FAILED", fmt.Errorf("read users API response: %w", err))
	}
	if int64(len(body)) > s.maxBody {
		return finish("FAILED", fmt.Errorf("users API response exceeds %d bytes", s.maxBody))
	}

	items, err := normalizeUserItems(body)
	if err != nil {
		return finish("FAILED", fmt.Errorf("parse users API response: %w", err))
	}
	result.Fetched = len(items)

	for _, item := range items {
		created, updated, err := s.upsertUser(item)
		if err != nil {
			return finish("FAILED", err)
		}
		if created {
			result.Created++
		} else if updated {
			result.Updated++
		} else {
			result.Unchanged++
		}
	}
	// ตำแหน่งบริหารเปลี่ยนตามข้อมูล sync และเอกสารที่ค้างอยู่ได้ผูกคนให้ทันที
	if err := syncPositionHolders(s.db); err != nil {
		s.logger.Printf("sync position holders: %v", err)
	} else if err := refreshPendingMatches(s.db); err != nil {
		s.logger.Printf("refresh pending matches after sync: %v", err)
	}
	return finish("SUCCESS", nil)
}

func (s *UserSyncService) upsertUser(item map[string]any) (created bool, updated bool, err error) {
	now := time.Now()
	raw, err := json.Marshal(item)
	if err != nil {
		return false, false, fmt.Errorf("marshal user payload: %w", err)
	}
	externalIDText := firstString(item, "user_id", "userId", "id", "employee_id", "employeeId", "external_id", "externalId")
	if externalIDText == "" {
		return false, false, errors.New("users API item is missing a numeric id required by directory_people.external_id")
	}
	fingerprintHash := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(fingerprintHash[:])
	externalID, err := strconv.ParseInt(externalIDText, 10, 64)
	if err != nil || externalID <= 0 {
		return false, false, fmt.Errorf("external user id %q cannot be stored in directory_people.external_id (BIGINT)", externalIDText)
	}

	user := directoryUserFromPayload(item, externalIDText, string(raw), fingerprint, now)
	if user.FirstName == "" || user.LastName == "" {
		parts := strings.Fields(stripThaiNamePrefixes(user.FullName))
		if len(parts) >= 2 {
			if user.FirstName == "" {
				user.FirstName = strings.Join(parts[:len(parts)-1], " ")
			}
			if user.LastName == "" {
				user.LastName = parts[len(parts)-1]
			}
		}
	}
	if strings.TrimSpace(user.FirstName) == "" || strings.TrimSpace(user.LastName) == "" {
		return false, false, fmt.Errorf("external user %s has no first/last name required by directory_people", externalIDText)
	}
	person := DirectoryPerson{
		ExternalID: &externalID, Prefix: strings.TrimSpace(user.Prefix),
		FirstName: strings.TrimSpace(user.FirstName), LastName: strings.TrimSpace(user.LastName),
		Email: nonEmptySQLValue(user.Email), Phone: nonEmptySQLValue(user.Phone),
		PositionTitle: strings.TrimSpace(user.PositionTitle), Status: normalizePersonStatus(user.IsActive),
		CreatedAt: now, UpdatedAt: now, SyncedAt: now,
	}
	profile := directoryPersonProfileFromPayload(item, sourceSynced, "", fingerprint, string(raw))

	var existing DirectoryPerson
	queryErr := s.db.Where("external_id = ?", externalID).First(&existing).Error
	if errors.Is(queryErr, gorm.ErrRecordNotFound) {
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			return createDirectoryPersonWithProfile(tx, &person, &profile)
		}); err != nil {
			return false, false, fmt.Errorf("create directory person %s: %w", externalIDText, err)
		}
		return true, false, nil
	}
	if queryErr != nil {
		return false, false, fmt.Errorf("find directory person %s: %w", externalIDText, queryErr)
	}
	var existingProfile DirectoryPersonProfile
	profileErr := s.db.First(&existingProfile, "person_id = ?", existing.ID).Error
	if profileErr != nil && !errors.Is(profileErr, gorm.ErrRecordNotFound) {
		return false, false, fmt.Errorf("find directory person profile %s: %w", externalIDText, profileErr)
	}
	person.ID = existing.ID
	person.CreatedAt = existing.CreatedAt
	unchanged := sameOptionalString(existing.Email, person.Email) && sameOptionalString(existing.Phone, person.Phone) &&
		existing.Prefix == person.Prefix && existing.FirstName == person.FirstName && existing.LastName == person.LastName &&
		existing.PositionTitle == person.PositionTitle && existing.Status == person.Status &&
		existingProfile.Department == profile.Department && existingProfile.Faculty == profile.Faculty
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		personUpdates := map[string]any{
			"prefix": person.Prefix, "first_name": person.FirstName, "last_name": person.LastName,
			"email": person.Email, "phone": person.Phone, "position_title": person.PositionTitle,
			"status": person.Status, "synced_at": now,
		}
		if !unchanged {
			personUpdates["updated_at"] = now
		}
		if err := tx.Model(&DirectoryPerson{}).Where("id = ?", person.ID).Updates(personUpdates).Error; err != nil {
			return err
		}
		profile.PersonID = person.ID
		profileUpdates := map[string]any{
			"department": profile.Department, "faculty": profile.Faculty, "source": sourceSynced,
			"source_updated_at": profile.SourceUpdatedAt, "raw_payload": profile.RawPayload,
			"fingerprint": profile.Fingerprint,
		}
		if errors.Is(profileErr, gorm.ErrRecordNotFound) {
			return tx.Create(&profile).Error
		}
		return tx.Model(&DirectoryPersonProfile{}).Where("person_id = ?", person.ID).Updates(profileUpdates).Error
	}); err != nil {
		return false, false, fmt.Errorf("update directory person %s: %w", externalIDText, err)
	}
	return false, !unchanged, nil
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func directoryUserFromPayload(item map[string]any, externalID, rawPayload, fingerprint string, now time.Time) DirectoryUser {
	firstName := firstString(item, "first_name", "firstName", "given_name", "givenName", "user_fname")
	lastName := firstString(item, "last_name", "lastName", "family_name", "familyName", "surname", "user_lname")
	positionTitle := firstString(item, "position_title", "positionTitle", "job_title", "jobTitle", "position", "manage_position", "title")
	roleName := firstString(item, "role_name", "roleName", "role", "user_role", "userRole", "type")
	return DirectoryUser{
		UserID:           firstString(item, "user_id", "userId"),
		ExternalID:       externalID,
		Prefix:           firstString(item, "prefix", "title_prefix"),
		Username:         firstString(item, "username", "user_name", "login", "account"),
		FullName:         userFullName(item),
		FirstName:        firstName,
		LastName:         lastName,
		Gender:           firstString(item, "gender", "sex"),
		Email:            firstString(item, "email", "mail", "email_address", "emailAddress"),
		Phone:            firstString(item, "tel", "phone", "telephone", "mobile", "mobile_phone", "mobilePhone"),
		PhoneFormatted:   firstString(item, "tel_format", "telFormat", "phone_formatted", "phoneFormatted"),
		Department:       firstString(item, "department", "department_name", "departmentName", "unit"),
		Faculty:          firstString(item, "faculty", "faculty_name", "facultyName"),
		PositionTitle:    positionTitle,
		JobTitle:         positionTitle,
		PositionEN:       firstString(item, "position_en", "positionEn"),
		PositionPrefixEN: firstString(item, "prefix_position_en", "positionPrefixEn"),
		ManagePosition:   firstString(item, "manage_position", "managePosition"),
		NameEN:           firstString(item, "name_en", "nameEn"),
		SuffixEN:         firstString(item, "suffix_en", "suffixEn"),
		ScopusID:         firstString(item, "scopus_id", "scopusId"),
		ScholarAuthorID:  firstString(item, "scholar_author_id", "scholarAuthorId"),
		LabName:          firstString(item, "lab_name", "labName"),
		Room:             firstString(item, "room"),
		CPWebID:          firstString(item, "cp_web_id", "cpWebId"),
		RoleID:           firstInt64(item, "role_id", "roleId"),
		RoleName:         roleName,
		Role:             roleName,
		IsActive:         firstActiveCode(item),
		SourceUpdatedAt:  firstString(item, "updated_at", "updatedAt"),
		RawPayload:       rawPayload,
		Fingerprint:      fingerprint,
		LastSeenAt:       now,
		SyncedAt:         now,
	}
}

func userSyncHandler(service *UserSyncService) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := service.SyncUsers(c.Request.Context(), currentUserID(c))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "runId": result.RunID})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": result})
	}
}

func listSyncRuns(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var runs []UserSyncRun
		limit := 20
		if err := db.Order("created_at DESC").Limit(limit).Find(&runs).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดประวัติการ sync ได้"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": runs})
	}
}

func normalizeUserItems(body []byte) ([]map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	items := collectUserItems(payload)
	if len(items) == 0 {
		return nil, errors.New("response does not contain a user list")
	}
	return items, nil
}

func collectUserItems(value any) []map[string]any {
	switch typed := value.(type) {
	case []any:
		items := make([]map[string]any, 0, len(typed))
		for _, child := range typed {
			if object, ok := child.(map[string]any); ok {
				items = append(items, object)
			}
		}
		return items
	case map[string]any:
		if looksLikeUser(typed) {
			return []map[string]any{typed}
		}
		for _, key := range []string{"data", "users", "items", "results", "records"} {
			if child, ok := typed[key]; ok {
				if items := collectUserItems(child); len(items) > 0 {
					return items
				}
			}
		}
	}
	return nil
}

func looksLikeUser(item map[string]any) bool {
	for _, key := range []string{"id", "user_id", "userId", "uuid", "username", "cp_web_id", "email", "full_name", "fullName", "first_name", "firstName", "user_fname"} {
		if _, ok := item[key]; ok {
			return true
		}
	}
	return false
}

func firstString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key]; ok && value != nil {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return strings.TrimSpace(typed)
				}
			default:
				candidate := strings.TrimSpace(fmt.Sprint(typed))
				if candidate != "" && candidate != "<nil>" {
					return candidate
				}
			}
		}
	}
	return ""
}

func firstBool(item map[string]any, fallback bool, keys ...string) bool {
	for _, key := range keys {
		value, ok := item[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case json.Number:
			return typed != "0"
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "true", "1", "yes", "active", "a":
				return true
			case "false", "0", "no", "inactive", "i":
				return false
			}
		}
	}
	return fallback
}

func firstInt64(item map[string]any, keys ...string) int64 {
	value := firstString(item, keys...)
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func firstActiveCode(item map[string]any) string {
	value := strings.TrimSpace(firstString(item, "is_active", "isActive", "active", "enabled"))
	if value == "" {
		return "A"
	}
	switch strings.ToLower(value) {
	case "a", "active", "true", "1", "yes", "y":
		return "A"
	case "i", "inactive", "false", "0", "no", "n":
		return "I"
	default:
		return value
	}
}

func normalizeUsersEndpoint(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return defaultExternalUsersURL
	}
	if strings.HasSuffix(strings.ToLower(value), "/users") {
		return value
	}
	return value + "/users"
}

func normalizeAPIKey(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			value = strings.TrimSpace(value[1 : len(value)-1])
		}
	}
	return value
}

func userFullName(item map[string]any) string {
	if value := firstString(item, "full_name", "fullName", "display_name", "displayName", "name", "name_th", "nameEn"); value != "" {
		return value
	}
	prefix := firstString(item, "prefix", "title_prefix")
	first := firstString(item, "first_name", "firstName", "given_name", "givenName", "user_fname")
	last := firstString(item, "last_name", "lastName", "family_name", "familyName", "surname", "user_lname")
	return strings.TrimSpace(strings.Join([]string{prefix, first, last}, " "))
}

func loadTimezone(name string, logger *log.Logger) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		logger.Printf("invalid timezone %q, falling back to UTC: %v", name, err)
		return time.UTC
	}
	return location
}
