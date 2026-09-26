package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

// FolderImporter imports stable files from the configured drop folder. Source
// files remain in place; the file hash makes subsequent scans idempotent.
type FolderImporter struct {
	db        *gorm.DB
	processor *DocumentProcessor
	logger    *log.Logger
	settings  SettingsStore
	lastScan  time.Time
}

func NewFolderImporter(db *gorm.DB, processor *DocumentProcessor, logger *log.Logger) *FolderImporter {
	return &FolderImporter{db: db, processor: processor, logger: logger, settings: NewSettingsStore(db)}
}

// ScanIfDue runs on a one-minute tick and applies the admin's on/off switch,
// source and interval. Google Drive is imported by a separate component.
func (i *FolderImporter) ScanIfDue(ctx context.Context) (int, error) {
	settings := loadOCRSettingsOrDefault(i.settings)
	if !settings.ImportEnabled || settings.ImportSource != "folder" {
		return 0, nil
	}
	now := time.Now()
	if !importDue(i.lastScan, now, settings.ImportIntervalMinutes) {
		return 0, nil
	}
	i.lastScan = now
	return i.Scan(ctx)
}

func (i *FolderImporter) Scan(ctx context.Context) (int, error) {
	directory := getenv("IMPORT_DIR", "./incoming")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return 0, fmt.Errorf("เตรียมโฟลเดอร์นำเข้า: %w", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, fmt.Errorf("อ่านโฟลเดอร์นำเข้า: %w", err)
	}

	imported := 0
	for _, entry := range entries {
		if ctx.Err() != nil {
			return imported, ctx.Err()
		}
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		if !allowedExtensions[strings.ToLower(filepath.Ext(entry.Name()))] {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil {
			i.logger.Printf("folder import stat failed for %s: %v", entry.Name(), statErr)
			continue
		}
		// Avoid ingesting a file while another process is still copying it.
		if info.ModTime().After(time.Now().Add(-30 * time.Second)) {
			continue
		}
		created, importErr := i.importFile(path)
		if importErr != nil {
			i.logger.Printf("folder import failed for %s: %v", entry.Name(), importErr)
			continue
		}
		if created {
			imported++
		}
	}
	return imported, nil
}

func (i *FolderImporter) importFile(sourcePath string) (bool, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() > 50<<20 {
		return false, errors.New("ไฟล์ต้องมีขนาดไม่เกิน 50 MB")
	}
	hash, err := hashReader(file)
	if err != nil {
		return false, fmt.Errorf("คำนวณ hash: %w", err)
	}
	ext := strings.ToLower(filepath.Ext(sourcePath))
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	if err := validateFileSignature(file, ext); err != nil {
		return false, err
	}
	var duplicate Document
	if err := i.db.Where("file_hash = ?", hash).First(&duplicate).Error; err == nil {
		return false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, fmt.Errorf("ตรวจสอบไฟล์ซ้ำ: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return false, err
	}

	storageDir := getenv("UPLOAD_DIR", "./storage")
	if err := os.MkdirAll(storageDir, 0o750); err != nil {
		return false, fmt.Errorf("เตรียมพื้นที่จัดเก็บ: %w", err)
	}
	filename := filepath.Base(sourcePath)
	storagePath := filepath.Join(storageDir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), filename))
	destination, err := os.Create(storagePath)
	if err != nil {
		return false, fmt.Errorf("สร้างไฟล์จัดเก็บ: %w", err)
	}
	if _, err := io.Copy(destination, file); err != nil {
		destination.Close()
		_ = os.Remove(storagePath)
		return false, fmt.Errorf("คัดลอกไฟล์: %w", err)
	}
	if err := destination.Close(); err != nil {
		_ = os.Remove(storagePath)
		return false, err
	}

	retentionUntil := time.Now().AddDate(10, 0, 0)
	document := Document{
		Title: strings.TrimSuffix(filename, ext), OriginalFilename: filename, StoragePath: storagePath,
		SourceType: strings.TrimPrefix(ext, "."), MimeType: mime.TypeByExtension(ext), Status: "PROCESSING",
		ProcessingStage: "QUEUED", FileSizeBytes: info.Size(), FileHash: hash,
		RetentionUntil: &retentionUntil, ImportSource: "FOLDER", ImportedBy: "folder-import", QualityProfile: "STANDARD",
	}
	if document.MimeType == "" {
		document.MimeType = "application/octet-stream"
	}
	if err := i.db.Create(&document).Error; err != nil {
		_ = os.Remove(storagePath)
		return false, fmt.Errorf("บันทึกเอกสาร: %w", err)
	}
	if err := i.db.Create(&AuditLog{DocumentID: &document.ID, UserID: document.ImportedBy, Action: "DOCUMENT_FOLDER_IMPORTED", Details: filename}).Error; err != nil {
		i.logger.Printf("folder import audit failed for document %d: %v", document.ID, err)
	}
	i.processor.Enqueue(document.ID)
	return true, nil
}

func hashReader(reader io.Reader) (string, error) {
	digest := sha256.New()
	if _, err := io.Copy(digest, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
