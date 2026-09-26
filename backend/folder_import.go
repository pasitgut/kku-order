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
	db     *gorm.DB
	logger *log.Logger
}

func NewFolderImporter(db *gorm.DB, logger *log.Logger) *FolderImporter {
	return &FolderImporter{db: db, logger: logger}
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
