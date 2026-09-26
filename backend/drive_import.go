package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"gorm.io/gorm"
)

const drivePageSize = 1000

type GoogleDriveImporter struct {
	db        *gorm.DB
	service   *drive.Service
	folderID  string
	statePath string
	logger    *log.Logger
}

type driveSyncState struct {
	ModifiedTimeByFileID map[string]string `json:"files"`
}

// NewGoogleDriveImporter returns nil when Drive sync is not configured.
func NewGoogleDriveImporter(db *gorm.DB, logger *log.Logger) (*GoogleDriveImporter, error) {
	folderID := normalizeDriveFolderID(os.Getenv("GOOGLE_DRIVE_FOLDER_ID"))
	if folderID == "" {
		logger.Print("Google Drive sync is disabled: GOOGLE_DRIVE_FOLDER_ID is empty")
		return nil, nil
	}
	credentialsFile := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_CREDENTIALS_FILE"))
	if credentialsFile == "" {
		credentialsFile = strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	}
	if credentialsFile == "" {
		return nil, fmt.Errorf("GOOGLE_DRIVE_CREDENTIALS_FILE must point to a service-account JSON file")
	}
	service, err := drive.NewService(context.Background(),
		option.WithCredentialsFile(credentialsFile),
		option.WithScopes(drive.DriveReadonlyScope),
	)
	if err != nil {
		return nil, fmt.Errorf("initialize Google Drive client: %w", err)
	}
	statePath := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_STATE_FILE"))
	if statePath == "" {
		statePath = filepath.Join(getenv("UPLOAD_DIR", "./storage"), ".google-drive-sync.json")
	}
	return &GoogleDriveImporter{db: db, service: service, folderID: folderID, statePath: statePath, logger: logger}, nil
}

// Sync imports new or changed files directly inside the configured Drive folder.
func (i *GoogleDriveImporter) Sync(ctx context.Context) (int, error) {
	state := i.loadState()
	imported := 0
	folders := []string{i.folderID}
	seenFolders := map[string]bool{i.folderID: true}
	for folderIndex := 0; folderIndex < len(folders); folderIndex++ {
		parentID := folders[folderIndex]
		pageToken := ""
		for {
			query := fmt.Sprintf("'%s' in parents and trashed = false", escapeDriveQueryValue(parentID))
			call := i.service.Files.List().
				Q(query).
				PageSize(drivePageSize).
				Fields("nextPageToken,files(id,name,mimeType,modifiedTime,size,capabilities(canDownload))").
				IncludeItemsFromAllDrives(true).
				SupportsAllDrives(true)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			page, err := call.Context(ctx).Do()
			if err != nil {
				return imported, fmt.Errorf("list files from Google Drive folder %q: %w", parentID, err)
			}
			for _, file := range page.Files {
				if ctx.Err() != nil {
					return imported, ctx.Err()
				}
				if file.Id == "" {
					continue
				}
				if file.MimeType == "application/vnd.google-apps.folder" {
					if !seenFolders[file.Id] {
						seenFolders[file.Id] = true
						folders = append(folders, file.Id)
					}
					continue
				}
				version := file.ModifiedTime
				if version == "" {
					version = "<unknown>"
				}
				if state.ModifiedTimeByFileID[file.Id] == version {
					continue
				}
				if file.Size > maxImportedFileSize {
					state.ModifiedTimeByFileID[file.Id] = version
					i.logger.Printf("Google Drive file exceeds the 50 MB import limit; skipped %q", file.Name)
					continue
				}
				if file.Capabilities != nil && !file.Capabilities.CanDownload {
					i.logger.Printf("Google Drive file cannot be downloaded; will retry: %q", file.Name)
					continue
				}

				outputName, exportMime, supported := driveOutputFile(file)
				if !supported {
					state.ModifiedTimeByFileID[file.Id] = version
					i.logger.Printf("Google Drive file type is unsupported; skipped %q (%s)", file.Name, file.MimeType)
					continue
				}
				created, err := i.importFile(ctx, file, outputName, exportMime)
				if err != nil {
					i.logger.Printf("Google Drive import failed for %q: %v", file.Name, err)
					continue
				}
				state.ModifiedTimeByFileID[file.Id] = version
				if created {
					imported++
				}
			}
			if page.NextPageToken == "" {
				break
			}
			pageToken = page.NextPageToken
		}
	}
	if err := i.saveState(state); err != nil {
		return imported, fmt.Errorf("save Google Drive sync state: %w", err)
	}
	return imported, nil
}

func (i *GoogleDriveImporter) importFile(ctx context.Context, file *drive.File, fileName, exportMime string) (bool, error) {
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
		response, err := i.service.Files.Export(file.Id, exportMime).Context(ctx).Download()
		if err != nil {
			return false, fmt.Errorf("export Drive document: %w", err)
		}
		responseBody = response.Body
	} else {
		response, err := i.service.Files.Get(file.Id).SupportsAllDrives(true).Context(ctx).Download()
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
	return storeImportedDocument(i.db, temporary, fileName, storageDir, bytesWritten)
}

func (i *GoogleDriveImporter) loadState() driveSyncState {
	state := driveSyncState{ModifiedTimeByFileID: make(map[string]string)}
	contents, err := os.ReadFile(i.statePath)
	if err != nil {
		if !os.IsNotExist(err) {
			i.logger.Printf("Google Drive sync state could not be read; rebuilding it: %v", err)
		}
		return state
	}
	if err := json.Unmarshal(contents, &state); err != nil || state.ModifiedTimeByFileID == nil {
		i.logger.Printf("Google Drive sync state is invalid; rebuilding it")
		return driveSyncState{ModifiedTimeByFileID: make(map[string]string)}
	}
	return state
}

func runGoogleDriveSync(importer *GoogleDriveImporter, logger *log.Logger, phase string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	imported, err := importer.Sync(ctx)
	if err != nil {
		logger.Printf("%s Google Drive sync failed: %v", phase, err)
		return
	}
	if imported > 0 {
		logger.Printf("%s Google Drive sync completed: imported=%d", phase, imported)
	}
}

func (i *GoogleDriveImporter) saveState(state driveSyncState) error {
	if err := os.MkdirAll(filepath.Dir(i.statePath), 0o750); err != nil {
		return err
	}
	contents, err := json.Marshal(state)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(i.statePath), ".google-drive-sync-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, i.statePath); err != nil {
		if _, statErr := os.Stat(i.statePath); statErr != nil {
			return err
		}
		if removeErr := os.Remove(i.statePath); removeErr != nil {
			return removeErr
		}
		if err := os.Rename(temporaryPath, i.statePath); err != nil {
			return err
		}
	}
	return nil
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
		return name, "", true
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
	return safeStoredFileName(base + extension)
}

func normalizeDriveFolderID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return value
	}
	if id := parsed.Query().Get("id"); id != "" {
		return id
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for index, part := range parts {
		if part == "folders" && index+1 < len(parts) {
			return parts[index+1]
		}
	}
	return value
}

func escapeDriveQueryValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `'`, `\'`)
}
