package main

import (
	"context"
	"fmt"
	"log"
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
	storageDir := getenv("UPLOAD_DIR", "./storage")
	filename := filepath.Base(sourcePath)
	return storeImportedDocument(i.db, file, filename, storageDir, info.Size(), "FOLDER")
}
