package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mime/multipart"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type Document struct {
	ID                   uint                `json:"id" gorm:"primaryKey"`
	Title                string              `json:"title" gorm:"size:500;not null"`
	OriginalFilename     string              `json:"originalFilename" gorm:"size:255;not null"`
	StoragePath          string              `json:"-" gorm:"size:500;not null"`
	SourceType           string              `json:"sourceType" gorm:"size:20;not null"`
	MimeType             string              `json:"mimeType" gorm:"size:150;not null"`
	OrderType            string              `json:"orderType" gorm:"size:50;index"`
	OrderYearBE          int                 `json:"orderYearBE" gorm:"index"`
	Status               string              `json:"status" gorm:"size:30;not null;index"`
	OrderNo              string              `json:"orderNo" gorm:"size:100;index"`
	Committee            string              `json:"committee" gorm:"size:500"`
	SignerName           string              `json:"signerName" gorm:"size:500"`
	SignerPosition       string              `json:"signerPosition" gorm:"size:255"`
	Responsibilities     string              `json:"responsibilities" gorm:"type:text"`
	AdditionalRefs       string              `json:"additionalReferences" gorm:"type:text"`
	Confidentiality      string              `json:"confidentiality" gorm:"size:30;index"`
	QualityProfile       string              `json:"qualityProfile" gorm:"size:30"`
	PageCount            int                 `json:"pageCount"`
	PersonCount          int                 `json:"personCount"`
	FileSizeBytes        int64               `json:"fileSizeBytes"`
	FileHash             string              `json:"fileHash" gorm:"size:64;index"`
	RetentionUntil       *time.Time          `json:"retentionUntil"`
	ImportSource         string              `json:"importSource" gorm:"size:30;not null;default:UPLOAD"`
	ProcessingStage      string              `json:"processingStage" gorm:"size:50"`
	ProcessingStartedAt  *time.Time          `json:"processingStartedAt"`
	ProcessingFinishedAt *time.Time          `json:"processingFinishedAt"`
	ProcessingDurationMs int64               `json:"processingDurationMs"`
	ReviewNotes          string              `json:"reviewNotes" gorm:"type:text"`
	IssuedDate           *time.Time          `json:"issuedDate"`
	EffectiveDate        *time.Time          `json:"effectiveDate"`
	ExpiryDate           *time.Time          `json:"expiryDate"`
	Confidence           float32             `json:"confidence"`
	ImportedBy           string              `json:"importedBy" gorm:"size:255"`
	ConfirmedBy          string              `json:"confirmedBy" gorm:"size:255"`
	ConfirmedAt          *time.Time          `json:"confirmedAt"`
	OCRProvider          string              `json:"ocrProvider" gorm:"size:100"`
	OCRText              string              `json:"ocrText" gorm:"type:longtext"`
	OCRError             string              `json:"ocrError" gorm:"type:longtext"`
	Fields               []ExtractedField    `json:"fields" gorm:"foreignKey:DocumentID"`
	OCRPages             []OCRPage           `json:"ocrPages" gorm:"foreignKey:DocumentID"`
	Appointments         []Appointment       `json:"appointments" gorm:"foreignKey:DocumentID"`
	References           []DocumentReference `json:"references" gorm:"foreignKey:DocumentID"`
	CreatedAt            time.Time           `json:"createdAt"`
	UpdatedAt            time.Time           `json:"updatedAt"`
}

type ExtractedField struct {
	ID              uint       `json:"id" gorm:"primaryKey"`
	DocumentID      uint       `json:"documentId" gorm:"index;not null"`
	FieldKey        string     `json:"fieldKey" gorm:"size:100;not null"`
	FieldLabel      string     `json:"fieldLabel" gorm:"size:255;not null"`
	FieldType       string     `json:"fieldType" gorm:"size:50"`
	Value           string     `json:"value" gorm:"type:text"`
	SourceText      string     `json:"sourceText" gorm:"type:text"`
	Confidence      float32    `json:"confidence"`
	PageNo          int        `json:"pageNo"`
	BoundingBoxJSON string     `json:"boundingBox" gorm:"type:json"`
	Verified        bool       `json:"verified"`
	VerifiedBy      string     `json:"verifiedBy" gorm:"size:255"`
	VerifiedAt      *time.Time `json:"verifiedAt"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type OCRPage struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	DocumentID  uint      `json:"documentId" gorm:"index;not null"`
	PageNo      int       `json:"pageNo"`
	ImageWidth  int       `json:"imageWidth"`
	ImageHeight int       `json:"imageHeight"`
	ImageAngle  float32   `json:"imageAngle"`
	RawText     string    `json:"rawText" gorm:"type:longtext"`
	Status      string    `json:"status" gorm:"size:30"`
	Lines       []OCRLine `json:"lines" gorm:"foreignKey:OCRPageID"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type OCRLine struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	DocumentID      uint      `json:"documentId" gorm:"index;not null"`
	OCRPageID       uint      `json:"ocrPageId" gorm:"index;not null"`
	PageNo          int       `json:"pageNo"`
	LineIndex       int       `json:"lineIndex"`
	Text            string    `json:"text" gorm:"type:longtext;not null"`
	Confidence      float32   `json:"confidence"`
	BoundingBoxJSON string    `json:"boundingBox" gorm:"type:json"`
	RawJSON         string    `json:"-" gorm:"type:json"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type AuditLog struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	DocumentID *uint     `json:"documentId" gorm:"index"`
	UserID     string    `json:"userId" gorm:"size:255;not null"`
	Action     string    `json:"action" gorm:"size:100;not null"`
	Details    string    `json:"details" gorm:"type:text"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Appointment stores the person-level rows extracted from a command.
// DirectoryUserID is optional because OCR may produce a name before a match exists.
type Appointment struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	DocumentID       uint           `json:"documentId" gorm:"index;not null"`
	DirectoryUserID  *uint          `json:"directoryUserId" gorm:"index"`
	DirectoryUser    *DirectoryUser `json:"directoryUser,omitempty" gorm:"foreignKey:DirectoryUserID"`
	FullName         string         `json:"fullName" gorm:"size:500;not null"`
	Position         string         `json:"position" gorm:"size:255"`
	Department       string         `json:"department" gorm:"size:500"`
	CommitteeRole    string         `json:"committeeRole" gorm:"size:255"`
	Responsibilities string         `json:"responsibilities" gorm:"type:text"`
	NameMatchMethod  string         `json:"nameMatchMethod" gorm:"size:50"`
	// Candidates is computed per request, never stored: the directory
	// changes, and a stale suggestion is worse than none.
	Candidates      []directoryCandidate `json:"candidates" gorm:"-"`
	NameMatchScore  float32              `json:"nameMatchScore"`
	Confidence      float32              `json:"confidence"`
	PageNo          int                  `json:"pageNo"`
	BoundingBoxJSON string               `json:"boundingBox" gorm:"type:json"`
	Verified        bool                 `json:"verified"`
	VerifiedBy      string               `json:"verifiedBy" gorm:"size:255"`
	VerifiedAt      *time.Time           `json:"verifiedAt"`
	CreatedAt       time.Time            `json:"createdAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
}

type DocumentReference struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	DocumentID      uint      `json:"documentId" gorm:"index;not null"`
	ReferenceType   string    `json:"referenceType" gorm:"size:100"`
	ReferenceText   string    `json:"referenceText" gorm:"type:text"`
	PageNo          int       `json:"pageNo"`
	BoundingBoxJSON string    `json:"boundingBox" gorm:"type:json"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type DocumentRevision struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	DocumentID uint      `json:"documentId" gorm:"index;not null"`
	FieldKey   string    `json:"fieldKey" gorm:"size:100;not null"`
	OldValue   string    `json:"oldValue" gorm:"type:text"`
	NewValue   string    `json:"newValue" gorm:"type:text"`
	ChangedBy  string    `json:"changedBy" gorm:"size:255;not null"`
	ChangedAt  time.Time `json:"changedAt"`
}

// DirectoryUser is the local copy of the university user directory returned by the external API.
type DirectoryUser struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	UserID           string    `json:"userId" gorm:"column:user_id;size:255;index"`
	ExternalID       string    `json:"externalId" gorm:"size:255;uniqueIndex;not null"`
	Prefix           string    `json:"prefix" gorm:"size:100"`
	Username         string    `json:"username" gorm:"size:255;index"`
	FullName         string    `json:"fullName" gorm:"size:500"`
	FirstName        string    `json:"firstName" gorm:"size:255"`
	LastName         string    `json:"lastName" gorm:"size:255"`
	Gender           string    `json:"gender" gorm:"size:30"`
	Email            string    `json:"email" gorm:"size:255;index"`
	Phone            string    `json:"phone" gorm:"size:100"`
	PhoneFormatted   string    `json:"phoneFormatted" gorm:"column:tel_format;size:100"`
	Department       string    `json:"department" gorm:"size:255"`
	Faculty          string    `json:"faculty" gorm:"size:255"`
	PositionTitle    string    `json:"positionTitle" gorm:"size:255"`
	JobTitle         string    `json:"jobTitle" gorm:"size:255"`
	PositionEN       string    `json:"positionEn" gorm:"column:position_en;size:255"`
	PositionPrefixEN string    `json:"positionPrefixEn" gorm:"column:prefix_position_en;size:100"`
	ManagePosition   string    `json:"managePosition" gorm:"column:manage_position;size:255"`
	NameEN           string    `json:"nameEn" gorm:"column:name_en;size:500"`
	SuffixEN         string    `json:"suffixEn" gorm:"column:suffix_en;size:100"`
	ScopusID         string    `json:"scopusId" gorm:"column:scopus_id;size:100"`
	ScholarAuthorID  string    `json:"scholarAuthorId" gorm:"column:scholar_author_id;size:255"`
	LabName          string    `json:"labName" gorm:"column:lab_name;size:500"`
	Room             string    `json:"room" gorm:"size:100"`
	CPWebID          string    `json:"cpWebId" gorm:"column:cp_web_id;size:500"`
	RoleID           int64     `json:"roleId" gorm:"column:role_id"`
	RoleName         string    `json:"roleName" gorm:"column:role_name;size:100"`
	Role             string    `json:"role" gorm:"size:100"`
	IsActive         string    `json:"isActive" gorm:"size:20"`
	Source           string    `json:"source" gorm:"size:20;not null;default:SYNCED;index"`
	CreatedBy        string    `json:"createdBy" gorm:"size:255"`
	SourceUpdatedAt  string    `json:"sourceUpdatedAt" gorm:"column:source_updated_at;size:50"`
	RawPayload       string    `json:"-" gorm:"type:json"`
	Fingerprint      string    `json:"-" gorm:"size:64;index"`
	LastSeenAt       time.Time `json:"lastSeenAt"`
	SyncedAt         time.Time `json:"syncedAt"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// PersonNameAlias remembers that a reviewer bound a particular OCR spelling
// to a person, so the next document carrying the same slip matches on its own.
type PersonNameAlias struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	NormalizedName  string    `json:"normalizedName" gorm:"size:255;uniqueIndex;not null"`
	DirectoryUserID uint      `json:"directoryUserId" gorm:"index;not null"`
	SourceText      string    `json:"sourceText" gorm:"size:500"`
	CreatedBy       string    `json:"createdBy" gorm:"size:255"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type UserSyncRun struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	StartedAt    time.Time  `json:"startedAt"`
	CompletedAt  *time.Time `json:"completedAt"`
	Status       string     `json:"status" gorm:"size:30;index"`
	Fetched      int        `json:"fetched"`
	Created      int        `json:"created"`
	Updated      int        `json:"updated"`
	Unchanged    int        `json:"unchanged"`
	ErrorMessage string     `json:"errorMessage" gorm:"type:text"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type ExpiryNotification struct {
	ID            uint       `json:"id" gorm:"primaryKey"`
	DocumentID    uint       `json:"documentId" gorm:"index;not null"`
	AppointmentID *uint      `json:"appointmentId" gorm:"index"`
	NoticeType    string     `json:"noticeType" gorm:"size:20;not null"`
	DueAt         time.Time  `json:"dueAt"`
	Status        string     `json:"status" gorm:"size:30;index"`
	SentAt        *time.Time `json:"sentAt"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

var allowedExtensions = map[string]bool{".pdf": true, ".docx": true, ".xlsx": true, ".csv": true}

func main() {
	logger := log.New(os.Stdout, "[docflow] ", log.LstdFlags|log.LUTC)
	dsn := getenv("DB_DSN", "docflow:docflow@tcp(localhost:3307)/docflow?charset=utf8mb4&parseTime=True&loc=Local")
	logger.Printf("database target: %s", redactDSN(dsn))
	var db *gorm.DB
	var err error
	for attempt := 1; attempt <= 30; attempt++ {
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
		if err == nil {
			if sqlDB, pingErr := db.DB(); pingErr == nil && sqlDB.Ping() == nil {
				break
			} else if pingErr != nil {
				err = pingErr
			} else {
				err = errors.New("database ping failed")
			}
		}
		logger.Printf("database is not ready (attempt %d/30): %v", attempt, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		panic(fmt.Errorf("connect database: %w", err))
	}
	if err := migrateDatabase(db); err != nil {
		panic(fmt.Errorf("migrate database: %w", err))
	}

	processor := NewDocumentProcessor(db, logger)
	userSync := NewUserSyncService(db, logger)
	logger.Printf("external users endpoint: %s (api key configured=%t)", userSync.baseURL, strings.TrimSpace(userSync.apiKey) != "")
	location := loadTimezone(getenv("USER_SYNC_TZ", "Asia/Bangkok"), logger)
	folderImporter := NewFolderImporter(db, processor, logger)
	if imported, importErr := folderImporter.ScanIfDue(context.Background()); importErr != nil {
		logger.Printf("initial folder import failed: %v", importErr)
	} else if imported > 0 {
		logger.Printf("initial folder import completed: imported=%d", imported)
	}
	scheduler := cron.New(cron.WithLocation(location), cron.WithChain(cron.DelayIfStillRunning(cron.DefaultLogger), cron.Recover(cron.DefaultLogger)))
	if _, err := scheduler.AddFunc(getenv("USER_SYNC_CRON", "0 5 * * *"), func() {
		result, syncErr := userSync.SyncUsers(context.Background())
		if syncErr != nil {
			logger.Printf("scheduled user sync failed: %v", syncErr)
			return
		}
		logger.Printf("scheduled user sync completed: fetched=%d created=%d updated=%d unchanged=%d", result.Fetched, result.Created, result.Updated, result.Unchanged)
	}); err != nil {
		panic(fmt.Errorf("configure user sync schedule: %w", err))
	}
	if _, err := scheduler.AddFunc(getenv("EXPIRY_CRON", "10 5 * * *"), func() {
		refreshExpiryNotifications(db, logger)
	}); err != nil {
		panic(fmt.Errorf("configure expiry notification schedule: %w", err))
	}
	// Ticks every minute; the interval itself comes from the OCR settings page.
	if _, err := scheduler.AddFunc(getenv("IMPORT_CRON", "* * * * *"), func() {
		imported, importErr := folderImporter.ScanIfDue(context.Background())
		if importErr != nil {
			logger.Printf("scheduled folder import failed: %v", importErr)
			return
		}
		if imported > 0 {
			logger.Printf("scheduled folder import completed: imported=%d", imported)
		}
	}); err != nil {
		panic(fmt.Errorf("configure folder import schedule: %w", err))
	}
	refreshExpiryNotifications(db, logger)
	scheduler.Start()
	defer scheduler.Stop()

	router := gin.Default()
	router.MaxMultipartMemory = 50 << 20
	router.Use(corsMiddleware())
	router.Use(authMiddleware())
	router.GET("/health", healthHandler(db))
	router.GET("/api/v1/dashboard", dashboardHandler(db))
	router.GET("/api/v1/notifications/expiry", listExpiryNotifications(db))
	router.GET("/api/v1/search", searchDocuments(db))
	router.GET("/api/v1/reports/workload", workloadReport(db))
	router.GET("/api/v1/directory-users", listDirectoryUsers(db))
	router.POST("/api/v1/directory-users", requireRole("ADMIN", "STAFF"), createDirectoryUser(db))
	router.GET("/api/v1/documents", listDocuments(db))
	router.GET("/api/v1/documents/:id", getDocument(db))
	router.GET("/api/v1/documents/:id/file", documentFile(db))
	router.GET("/api/v1/documents/:id/pages/:page/image", documentPageImage(db))
	router.GET("/api/v1/documents/:id/revisions", listDocumentRevisions(db))
	router.GET("/api/v1/documents/:id/audit", listDocumentAuditLogs(db))
	router.POST("/api/v1/documents", requireRole("ADMIN", "STAFF"), uploadDocument(db, processor))
	router.POST("/api/v1/documents/:id/file", requireRole("ADMIN", "STAFF"), replaceDocumentFile(db, processor))
	router.PATCH("/api/v1/documents/:id", requireRole("ADMIN", "STAFF"), updateDocument(db))
	router.DELETE("/api/v1/documents/:id", requireRole("ADMIN", "DEVELOPER"), deleteDocument(db))
	router.POST("/api/v1/documents/:id/confirm", requireRole("ADMIN", "STAFF"), confirmDocument(db))
	router.POST("/api/v1/sync/users", requireRole("ADMIN", "STAFF"), userSyncHandler(userSync))
	router.GET("/api/v1/sync/runs", listSyncRuns(db))
	settingsStore := NewSettingsStore(db)
	router.GET("/api/v1/settings/ocr", getOCRSettingsHandler(settingsStore))
	router.PUT("/api/v1/settings/ocr", requireRole("ADMIN"), updateOCRSettingsHandler(settingsStore))
	router.POST("/api/v1/settings/ocr/test", requireRole("ADMIN"), testOCRSettingsHandler())
	router.GET("/api/v1/settings/system", requireRole("ADMIN", "STAFF", "DEVELOPER"), systemSettingsHandler())
	router.GET("/api/v1/me", meHandler(db))
	router.GET("/api/v1/me/activity", myActivityHandler(db))
	router.GET("/api/v1/me/photo", getMyPhotoHandler(db))
	router.PUT("/api/v1/me/photo", uploadMyPhotoHandler(db))
	router.DELETE("/api/v1/me/photo", deleteMyPhotoHandler(db))

	port := getenv("PORT", "8080")
	_ = router.Run(":" + port)
}

func redactDSN(dsn string) string {
	parts := strings.SplitN(dsn, "@", 2)
	if len(parts) != 2 {
		return "<invalid DB_DSN>"
	}
	credentials := strings.SplitN(parts[0], ":", 2)
	if len(credentials) == 2 && credentials[0] != "" {
		return credentials[0] + ":<redacted>@" + parts[1]
	}
	return "<redacted>@" + parts[1]
}

func healthHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.Ping() != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

func listDocuments(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var documents []Document
		query := applyDocumentFilters(documentQuery(db, c.Query("q"), c.Query("status")), c)
		var total int64
		if err := query.Count(&total).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถนับรายการเอกสารได้"})
			return
		}
		query = query.Preload("Fields").Preload("Appointments").Order("created_at DESC").Limit(parseLimit(c)).Offset(parseOffset(c))
		if err := query.Find(&documents).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดเอกสารได้"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": documents, "pagination": gin.H{"total": total, "page": parsePage(c), "limit": parseLimit(c)}})
	}
}

func getDocument(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var document Document
		if err := db.Preload("Fields").Preload("Appointments").Preload("Appointments.DirectoryUser").Preload("References").Preload("OCRPages.Lines").First(&document, c.Param("id")).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบเอกสาร"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดเอกสารได้"})
			return
		}
		attachAppointmentCandidates(db, &document)
		c.JSON(http.StatusOK, gin.H{"data": document})
	}
}

// attachAppointmentCandidates suggests who each unlinked row might be. It is
// computed per request rather than stored, so a suggestion never outlives the
// directory it came from.
func attachAppointmentCandidates(db *gorm.DB, document *Document) {
	needed := false
	for _, appointment := range document.Appointments {
		if appointment.DirectoryUserID == nil {
			needed = true
			break
		}
	}
	if !needed {
		return
	}
	users, aliases, err := loadDirectory(db)
	if err != nil {
		return
	}
	for index := range document.Appointments {
		if document.Appointments[index].DirectoryUserID != nil {
			continue
		}
		match := matchDirectoryUser(document.Appointments[index].FullName, users, aliases)
		document.Appointments[index].Candidates = candidatesToOffer(match)
		document.Appointments[index].NameMatchMethod = match.Method
	}
}

func uploadDocument(db *gorm.DB, processor *DocumentProcessor) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาเลือกไฟล์"})
			return
		}
		defer file.Close()
		ext := strings.ToLower(filepath.Ext(header.Filename))
		if !allowedExtensions[ext] {
			c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "รองรับเฉพาะ PDF, DOCX, XLSX และ CSV"})
			return
		}
		if header.Size > 50<<20 {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "ไฟล์ต้องมีขนาดไม่เกิน 50 MB"})
			return
		}
		fileHash, err := hashUpload(file)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่สามารถอ่านไฟล์เพื่อตรวจสอบซ้ำได้"})
			return
		}
		if err := validateFileSignature(file, ext); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var duplicate Document
		if err := db.Where("file_hash = ?", fileHash).First(&duplicate).Error; err == nil {
			if !strings.EqualFold(c.PostForm("overwrite"), "true") {
				c.JSON(http.StatusConflict, gin.H{"error": "พบไฟล์เอกสารที่เคยนำเข้าแล้ว", "duplicateDocumentId": duplicate.ID, "canOverwrite": true})
				return
			}
			if duplicate.Status == "PROCESSING" {
				c.JSON(http.StatusConflict, gin.H{"error": "ไฟล์เดิมกำลังประมวลผลอยู่ กรุณารอให้เสร็จก่อนบันทึกทับ", "duplicateDocumentId": duplicate.ID})
				return
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบไฟล์ซ้ำได้"})
			return
		}

		storageDir := getenv("UPLOAD_DIR", "./storage")
		if err := os.MkdirAll(storageDir, 0o750); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถเตรียมพื้นที่จัดเก็บได้"})
			return
		}
		safeName := fmt.Sprintf("%d-%s", time.Now().UnixNano(), filepath.Base(header.Filename))
		storagePath := filepath.Join(storageDir, safeName)
		if err := c.SaveUploadedFile(header, storagePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกไฟล์ได้"})
			return
		}

		retentionUntil := time.Now().AddDate(10, 0, 0)
		document := Document{Title: strings.TrimSuffix(filepath.Base(header.Filename), ext), OriginalFilename: header.Filename, StoragePath: storagePath, SourceType: strings.TrimPrefix(ext, "."), MimeType: header.Header.Get("Content-Type"), Status: "PROCESSING", ProcessingStage: "QUEUED", FileSizeBytes: header.Size, FileHash: fileHash, RetentionUntil: &retentionUntil, ImportSource: "UPLOAD", ImportedBy: currentUserID(c), QualityProfile: "STANDARD"}
		if document.MimeType == "" {
			document.MimeType = "application/octet-stream"
		}
		if duplicate.ID != 0 {
			retentionUntil := time.Now().AddDate(10, 0, 0)
			updates := map[string]any{
				"title": strings.TrimSuffix(filepath.Base(header.Filename), ext), "original_filename": header.Filename,
				"storage_path": storagePath, "source_type": strings.TrimPrefix(ext, "."), "mime_type": document.MimeType,
				"status": "PROCESSING", "processing_stage": "QUEUED", "file_size_bytes": header.Size, "file_hash": fileHash,
				"retention_until": retentionUntil, "import_source": "UPLOAD", "imported_by": currentUserID(c),
				"confirmed_by": "", "confirmed_at": nil, "ocr_error": "", "ocr_text": "", "page_count": 0,
				"person_count": 0, "confidence": 0, "processing_started_at": nil, "processing_finished_at": nil,
				"processing_duration_ms": 0,
			}
			if err := db.Transaction(func(tx *gorm.DB) error {
				for _, model := range []any{&OCRLine{}, &OCRPage{}, &ExtractedField{}, &Appointment{}, &DocumentReference{}} {
					if err := tx.Where("document_id = ?", duplicate.ID).Delete(model).Error; err != nil {
						return err
					}
				}
				if err := tx.Model(&duplicate).Updates(updates).Error; err != nil {
					return err
				}
				return tx.Create(&AuditLog{DocumentID: &duplicate.ID, UserID: currentUserID(c), Action: "DOCUMENT_OVERWRITTEN", Details: header.Filename}).Error
			}); err != nil {
				_ = os.Remove(storagePath)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกทับเอกสารได้"})
				return
			}
			_ = os.Remove(duplicate.StoragePath)
			processor.Enqueue(duplicate.ID)
			if err := db.First(&duplicate, duplicate.ID).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดเอกสารที่บันทึกทับได้"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": duplicate, "overwritten": true})
			return
		}
		if err := db.Create(&document).Error; err != nil {
			_ = os.Remove(storagePath)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกข้อมูลเอกสารได้"})
			return
		}
		_ = db.Create(&AuditLog{DocumentID: &document.ID, UserID: document.ImportedBy, Action: "DOCUMENT_UPLOADED", Details: header.Filename}).Error
		processor.Enqueue(document.ID)
		c.JSON(http.StatusCreated, gin.H{"data": document})
	}
}

func hashUpload(file multipart.File) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func confirmDocument(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var document Document
		if err := db.First(&document, c.Param("id")).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบเอกสาร"})
			return
		}
		if document.Status == "CONFIRMED" {
			c.JSON(http.StatusOK, gin.H{"message": "เอกสารนี้ยืนยันแล้ว", "data": document})
			return
		}
		if document.Status != "REVIEW" && document.Status != "NEEDS_REVIEW" {
			c.JSON(http.StatusConflict, gin.H{"error": "เอกสารต้องผ่านขั้นตอนตรวจสอบก่อนยืนยัน", "status": document.Status})
			return
		}
		var appointments []Appointment
		if err := db.Where("document_id = ?", document.ID).Find(&appointments).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบรายชื่อก่อนยืนยันได้"})
			return
		}
		if blockers := confirmBlockers(appointments); len(blockers) > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "ยังมีรายชื่อที่ยังไม่ได้ผูกกับบุคคลในระบบ", "unmatchedNames": blockers})
			return
		}
		now := time.Now()
		confirmedBy := currentUserID(c)
		updates := map[string]any{"status": "CONFIRMED", "confirmed_by": confirmedBy, "confirmed_at": now}
		if err := db.Model(&document).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถยืนยันเอกสารได้"})
			return
		}
		_ = db.Create(&AuditLog{DocumentID: &document.ID, UserID: confirmedBy, Action: "DOCUMENT_CONFIRMED", Details: "เจ้าหน้าที่ตรวจสอบและยืนยันข้อมูล"}).Error
		document.Status = "CONFIRMED"
		document.ConfirmedBy = confirmedBy
		document.ConfirmedAt = &now
		c.JSON(http.StatusOK, gin.H{"message": "ยืนยันข้อมูลเรียบร้อยแล้ว", "data": document})
	}
}

func listDocumentRevisions(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var revisions []DocumentRevision
		query := db.Where("document_id = ?", c.Param("id")).Order("changed_at DESC").Limit(parseLimit(c))
		if err := query.Find(&revisions).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดประวัติการแก้ไขได้"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": revisions})
	}
}

func listDocumentAuditLogs(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var logs []AuditLog
		query := db.Where("document_id = ?", c.Param("id")).Order("created_at DESC").Limit(parseLimit(c))
		if err := query.Find(&logs).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดประวัติการทำรายการได้"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": logs})
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", getenv("WEB_ORIGIN", "http://localhost:3000"))
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Auth-Request-User, X-Auth-Request-Role")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
