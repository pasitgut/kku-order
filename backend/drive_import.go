package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"gorm.io/gorm"
)

const drivePageSize = 1000

type GoogleDriveImporter struct {
	db       *gorm.DB
	service  *drive.Service
	settings SettingsStore
	logger   *log.Logger
	syncMu   sync.Mutex
}

func NewGoogleDriveImporter(db *gorm.DB, logger *log.Logger) (*GoogleDriveImporter, error) {
	credentialsFile := googleDriveCredentialsFile()
	if credentialsFile == "" {
		logger.Print("Google Drive automation awaits user OAuth credentials")
		return nil, nil
	}
	contents, err := osReadDriveCredentials()
	if err != nil {
		return nil, fmt.Errorf("read Google Drive credentials: %w", err)
	}
	if err := validateDriveCredentials(contents); err != nil {
		return nil, err
	}
	service, err := drive.NewService(context.Background(), option.WithAuthCredentialsJSON(option.AuthorizedUser, contents), option.WithScopes(drive.DriveScope))
	if err != nil {
		return nil, fmt.Errorf("initialize Google Drive client: %w", err)
	}
	return &GoogleDriveImporter{db: db, service: service, settings: NewSettingsStore(db), logger: logger}, nil
}

func googleDriveCredentialsFile() string {
	path := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_CREDENTIALS_FILE"))
	if path == "" {
		path = strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	}
	return path
}

func osReadDriveCredentials() ([]byte, error) {
	file, err := os.Open(googleDriveCredentialsFile())
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, 1<<20))
}

func googleDriveCredentialsConfigured() bool {
	contents, err := osReadDriveCredentials()
	if err != nil {
		return false
	}
	return validateDriveCredentials(contents) == nil
}

func (i *GoogleDriveImporter) configuredFolderID(folderURL string) (string, error) {
	folderID := normalizeDriveFolderID(folderURL)
	if folderID == "" {
		folderID = normalizeDriveFolderID(loadOCRSettingsOrDefault(i.settings).DriveFolderURL)
	}
	if folderID == "" {
		folderID = normalizeDriveFolderID(os.Getenv("GOOGLE_DRIVE_FOLDER_ID"))
	}
	if folderID == "" {
		return "", fmt.Errorf("กรุณากำหนดลิงก์โฟลเดอร์ Google Drive")
	}
	return folderID, nil
}

func (i *GoogleDriveImporter) TestConnection(ctx context.Context, folderURL string) (string, error) {
	folderID := normalizeDriveFolderID(folderURL)
	if folderID == "" {
		return "", fmt.Errorf("กรุณาใช้ลิงก์โฟลเดอร์ Google Drive ที่ถูกต้อง")
	}
	resourceKey := ""
	if parsed, err := url.Parse(folderURL); err == nil {
		resourceKey = parsed.Query().Get("resourcekey")
	}
	folder, err := i.getDriveFile(ctx, folderID, resourceKey)
	if err != nil {
		return "", fmt.Errorf("ตรวจสอบสิทธิ์เข้าถึงโฟลเดอร์ Google Drive ไม่สำเร็จ: %w", err)
	}
	if folder.MimeType != driveFolderMime || folder.Trashed {
		return "", fmt.Errorf("ลิงก์นี้ไม่ใช่โฟลเดอร์ Google Drive ที่ใช้งานได้")
	}
	if folder.Capabilities != nil && !folder.Capabilities.CanEdit {
		return "", fmt.Errorf("บัญชี Gmail ระบบต้องมีสิทธิ์ Editor ที่โฟลเดอร์ Google Drive เพื่อเปลี่ยนชื่อไฟล์")
	}
	// Verify listing permission as well as folder metadata access.
	call := i.service.Files.List().Q(fmt.Sprintf("'%s' in parents and trashed = false", escapeDriveQueryValue(folderID))).PageSize(1).Fields("files(id)").SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Context(ctx)
	driveResourceHeader(call.Header(), folderID, resourceKey)
	if _, err := call.Do(); err != nil {
		return "", fmt.Errorf("ไม่สามารถอ่านรายการไฟล์ในโฟลเดอร์: %w", err)
	}
	return folder.Name, nil
}

func (i *GoogleDriveImporter) downloadFile(ctx context.Context, file *drive.File, exportMime string) (*http.Response, error) {
	return driveRetry(ctx, func() (*http.Response, error) {
		if exportMime != "" {
			call := i.service.Files.Export(file.Id, exportMime).Context(ctx)
			driveResourceHeader(call.Header(), file.Id, file.ResourceKey)
			return call.Download()
		}
		call := i.service.Files.Get(file.Id).SupportsAllDrives(true).Context(ctx)
		driveResourceHeader(call.Header(), file.Id, file.ResourceKey)
		return call.Download()
	})
}

func runGoogleDriveSync(importer *GoogleDriveImporter, logger *log.Logger, phase string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	imported, err := importer.SyncIfDue(ctx)
	if err != nil {
		logger.Printf("%s Google Drive sync: %v", phase, err)
	}
	if imported > 0 {
		logger.Printf("%s Google Drive batch imported=%d", phase, imported)
	}
}

func driveCreatedDatePrefix(file *drive.File) (string, error) {
	if strings.TrimSpace(file.CreatedTime) == "" {
		return "", fmt.Errorf("Google Drive ไม่ส่งวันที่สร้างไฟล์")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, file.CreatedTime)
	if err != nil {
		return "", fmt.Errorf("อ่านวันที่สร้างไฟล์จาก Google Drive ไม่สำเร็จ: %w", err)
	}
	bangkok := time.FixedZone("Asia/Bangkok", 7*60*60)
	return createdAt.In(bangkok).Format("2006_01_02"), nil
}

func datedDriveFilename(file *drive.File, outputName string) (string, error) {
	datePrefix, err := driveCreatedDatePrefix(file)
	if err != nil {
		return "", err
	}
	datePrefix += "_"
	if !strings.HasPrefix(outputName, datePrefix) {
		outputName = datePrefix + outputName
	}
	if len([]rune(outputName)) > 500 {
		return "", fmt.Errorf("ชื่อไฟล์หลังเติมวันที่ยาวเกิน 500 ตัวอักษร")
	}
	return outputName, nil
}

func driveWebViewLink(file *drive.File) string {
	link := strings.TrimSpace(file.WebViewLink)
	if link == "" {
		link = "https://drive.google.com/open?id=" + url.QueryEscape(file.Id)
	}
	if file.ResourceKey != "" {
		if parsed, err := url.Parse(link); err == nil {
			query := parsed.Query()
			if query.Get("resourcekey") == "" {
				query.Set("resourcekey", file.ResourceKey)
				parsed.RawQuery = query.Encode()
				link = parsed.String()
			}
		}
	}
	return link
}

func (i *GoogleDriveImporter) renameDriveFile(ctx context.Context, file *drive.File, outputName string) (*drive.File, string, error) {
	targetName, err := datedDriveFilename(file, outputName)
	if err != nil {
		return nil, "", err
	}
	if file.Name == targetName {
		return file, driveWebViewLink(file), nil
	}
	if file.Capabilities != nil && !file.Capabilities.CanEdit {
		return nil, "", fmt.Errorf("บัญชี Gmail ระบบต้องมีสิทธิ์ Editor ที่ไฟล์ %q เพื่อเปลี่ยนชื่อ", file.Name)
	}
	renamed, err := driveRetry(ctx, func() (*drive.File, error) {
		call := i.service.Files.Update(file.Id, &drive.File{Name: targetName}).SupportsAllDrives(true).
			Fields(googleapi.Field(driveFileFields)).Context(ctx)
		driveResourceHeader(call.Header(), file.Id, file.ResourceKey)
		return call.Do()
	})
	if err != nil {
		var apiError *googleapi.Error
		if errors.As(err, &apiError) && apiError.Code == http.StatusForbidden {
			return nil, "", fmt.Errorf("เปลี่ยนชื่อไม่ได้ ตรวจสิทธิ์ Editor ของ Gmail ระบบและสิทธิ์แก้ไขไฟล์ใน Google Drive: %w", err)
		}
		return nil, "", fmt.Errorf("เปลี่ยนชื่อไฟล์ใน Google Drive ไม่สำเร็จ: %w", err)
	}
	if renamed.ResourceKey == "" {
		renamed.ResourceKey = file.ResourceKey
	}
	if renamed.CreatedTime == "" {
		renamed.CreatedTime = file.CreatedTime
	}
	return renamed, driveWebViewLink(renamed), nil
}

func (i *GoogleDriveImporter) updateImportedDriveSource(ctx context.Context, imported *DriveImportedFile, fileName, webViewLink string) error {
	return i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if imported.WebViewLink != webViewLink {
			if err := tx.Model(imported).Update("web_view_link", webViewLink).Error; err != nil {
				return err
			}
			imported.WebViewLink = webViewLink
		}
		var document Document
		if err := tx.First(&document, imported.DocumentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		updates := map[string]any{}
		if document.DriveWebViewLink != webViewLink {
			updates["drive_web_view_link"] = webViewLink
		}
		if document.ImportSource == "GOOGLE_DRIVE" {
			if document.OriginalFilename != fileName {
				updates["original_filename"] = fileName
				oldTitle := strings.TrimSuffix(document.OriginalFilename, filepath.Ext(document.OriginalFilename))
				if document.Title == oldTitle {
					updates["title"] = strings.TrimSuffix(fileName, filepath.Ext(fileName))
				}
				extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")
				if extension != "" {
					if err := tx.Model(&StoredDocument{}).Where("id = ?", document.ID).Updates(map[string]any{
						"file_name": fileName, "file_type": extension, "updated_at": time.Now(),
					}).Error; err != nil {
						return err
					}
				}
			}
		}
		if len(updates) == 0 {
			return nil
		}
		updates["updated_at"] = time.Now()
		return tx.Model(&Document{}).Where("id = ?", document.ID).Updates(updates).Error
	})
}

func (i *GoogleDriveImporter) importFile(ctx context.Context, file *drive.File, fileName, exportMime, webViewLink string, item *DriveSyncItem) (bool, error) {
	storageDir := getenv("UPLOAD_DIR", "./storage")
	if err := os.MkdirAll(storageDir, 0o750); err != nil {
		return false, fmt.Errorf("prepare storage directory: %w", err)
	}
	temporary, err := os.CreateTemp(storageDir, ".drive-import-*")
	if err != nil {
		return false, fmt.Errorf("create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer temporary.Close()
	defer os.Remove(temporaryPath)

	var responseBody io.ReadCloser
	if exportMime != "" {
		response, err := i.downloadFile(ctx, file, exportMime)
		if err != nil {
			return false, fmt.Errorf("export Drive document: %w", err)
		}
		responseBody = response.Body
	} else {
		response, err := i.downloadFile(ctx, file, "")
		if err != nil {
			return false, fmt.Errorf("download Drive file: %w", err)
		}
		responseBody = response.Body
	}
	defer responseBody.Close()

	bytesWritten, err := io.Copy(temporary, io.LimitReader(responseBody, maxImportedFileSize+1))
	if err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("write downloaded file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return false, fmt.Errorf("close downloaded file: %w", err)
	}
	if bytesWritten > maxImportedFileSize {
		return false, fmt.Errorf("ไฟล์ต้องมีขนาดไม่เกิน 50 MB")
	}

	temporary, err = os.Open(temporaryPath)
	if err != nil {
		return false, err
	}
	defer temporary.Close()
	hash, err := hashReader(temporary)
	if err != nil {
		return false, err
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	created := false
	savedPath := ""
	err = i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var known DriveImportedFile
		lookup := tx.Where("file_id = ?", file.Id).Limit(1).Find(&known)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected > 0 {
			return tx.Model(item).Updates(map[string]any{"status": "EXISTING", "last_error": "", "retry_at": nil}).Error
		}
		var storeErr error
		created, storeErr = storeImportedDocument(tx, temporary, fileName, storageDir, bytesWritten, "GOOGLE_DRIVE")
		if storeErr != nil {
			return storeErr
		}
		var document StoredDocument
		if err := tx.Where("file_hash = ?", hash).Take(&document).Error; err != nil {
			return err
		}
		if created {
			savedPath = document.StoragePath
		}
		if err := tx.Model(&Document{}).Where("id = ?", document.ID).Updates(map[string]any{
			"drive_web_view_link": webViewLink,
			"updated_at":          time.Now(),
		}).Error; err != nil {
			return err
		}
		if err := tx.Create(&DriveImportedFile{FileID: file.Id, DocumentID: document.ID, FileHash: hash, WebViewLink: webViewLink, ImportedAt: time.Now()}).Error; err != nil {
			return err
		}
		status := "DUPLICATE"
		if created {
			status = "IMPORTED"
		}
		return tx.Model(item).Updates(map[string]any{"status": status, "last_error": "", "retry_at": nil}).Error
	})
	if err != nil && savedPath != "" {
		_ = os.Remove(savedPath)
	}
	return created, err
}

func driveOutputFile(file *drive.File) (name, exportMime string, supported bool) {
	name = strings.TrimSpace(file.Name)
	switch file.MimeType {
	case "application/vnd.google-apps.document":
		return withExtension(name, ".pdf"), "application/pdf", true
	case "application/vnd.google-apps.spreadsheet":
		return withExtension(name, ".xlsx"), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", true
	case "application/vnd.google-apps.presentation", "application/vnd.google-apps.drawing":
		return withExtension(name, ".pdf"), "application/pdf", true
	}
	if allowedExtensions[strings.ToLower(filepath.Ext(name))] {
		return withExtension(name, filepath.Ext(name)), "", true
	}
	if extension := extensionForMime(file.MimeType); extension != "" {
		return withExtension(name, extension), "", true
	}
	return "", "", false
}

func extensionForMime(mimeType string) string {
	switch strings.ToLower(mimeType) {
	case "application/pdf":
		return ".pdf"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	case "text/csv", "application/csv":
		return ".csv"
	default:
		return ""
	}
}

func withExtension(name, extension string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if base == "" {
		base = "drive-file"
	}
	base = safeStoredFileName(base)
	runes := []rune(base)
	limit := 500 - len([]rune(extension))
	if len(runes) > limit {
		base = string(runes[:limit])
	}
	return base + extension
}

func normalizeDriveFolderID(value string) string {
	value = strings.TrimSpace(value)
	if validDriveID(value) {
		return value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "drive.google.com" {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for index, part := range parts {
		if part == "folders" && index+1 < len(parts) && validDriveID(parts[index+1]) {
			return parts[index+1]
		}
	}
	if (parsed.Path == "/open" || parsed.Path == "/folderview") && validDriveID(parsed.Query().Get("id")) {
		return parsed.Query().Get("id")
	}
	return ""
}

func escapeDriveQueryValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `'`, `\'`)
}
