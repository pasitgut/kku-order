package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

const maxImportedFileSize = 50 << 20

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

// storeImportedDocument writes a validated file and its metadata using the
// current documents schema. Duplicate content is detected by SHA-256.
func storeImportedDocument(db *gorm.DB, source io.ReadSeeker, fileName, storageDir string, fileSize int64, importSource string) (bool, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	if !allowedExtensions[ext] {
		return false, fmt.Errorf("รองรับเฉพาะ PDF, DOCX, XLSX และ CSV")
	}
	if fileSize <= 0 {
		return false, fmt.Errorf("ไฟล์ว่าง")
	}
	if fileSize > maxImportedFileSize {
		return false, fmt.Errorf("ไฟล์ต้องมีขนาดไม่เกิน 50 MB")
	}

	hash, err := hashReader(source)
	if err != nil {
		return false, fmt.Errorf("คำนวณ hash: %w", err)
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	if err := validateFileSignature(source, ext); err != nil {
		return false, err
	}

	var duplicates int64
	if err := db.Model(&Document{}).Where("file_hash = ?", hash).Count(&duplicates).Error; err != nil {
		return false, fmt.Errorf("ตรวจสอบไฟล์ซ้ำ: %w", err)
	}
	if duplicates > 0 {
		return false, nil
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	if err := os.MkdirAll(storageDir, 0o750); err != nil {
		return false, fmt.Errorf("เตรียมพื้นที่จัดเก็บ: %w", err)
	}

	fileName = safeStoredFileName(fileName)
	storagePath := filepath.Join(storageDir, storageFileName(time.Now().UnixNano(), fileName))
	if len([]rune(storagePath)) > 1000 {
		return false, fmt.Errorf("ที่อยู่ไฟล์ยาวเกินขนาดที่ฐานข้อมูลรองรับ")
	}
	destination, err := os.OpenFile(storagePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return false, fmt.Errorf("สร้างไฟล์จัดเก็บ: %w", err)
	}
	keepFile := false
	defer func() {
		if !keepFile {
			_ = os.Remove(storagePath)
		}
	}()
	if _, err := io.Copy(destination, source); err != nil {
		_ = destination.Close()
		return false, fmt.Errorf("คัดลอกไฟล์: %w", err)
	}
	if err := destination.Close(); err != nil {
		return false, err
	}

	now := time.Now()
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	title := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	workflow := Document{
		Title: title, OriginalFilename: fileName, StoragePath: storagePath,
		ContentState: "UNSAVED",
		SourceType:   strings.TrimPrefix(ext, "."), MimeType: mimeType,
		Status: "NEEDS_REVIEW", ProcessingStage: "IMPORTED", FileSizeBytes: fileSize,
		FileHash: hash, ImportSource: importSource, ImportedBy: "system:" + strings.ToLower(importSource),
		QualityProfile: "STANDARD", CreatedAt: now, UpdatedAt: now,
	}
	// The audit row commits with the document, so a stored file always has its entry.
	audit := importAuditFor(importSource)
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&workflow).Error; err != nil {
			return err
		}
		return tx.Create(&AuditLog{DocumentID: &workflow.ID, UserID: audit.actor, Action: audit.action, Details: fileName}).Error
	})
	if err != nil {
		return false, fmt.Errorf("บันทึกเอกสาร: %w", err)
	}
	keepFile = true
	return true, nil
}

type importAudit struct {
	action string
	actor  string
}

// importAuditFor names the audit entry for each import source. Unknown sources
// get a generic action, so a new source is still recorded.
func importAuditFor(importSource string) importAudit {
	switch importSource {
	case "FOLDER":
		return importAudit{action: "DOCUMENT_FOLDER_IMPORTED", actor: auditSystemUser("folder-import")}
	case "GOOGLE_DRIVE":
		return importAudit{action: "DOCUMENT_DRIVE_IMPORTED", actor: auditSystemUser("google-drive-import")}
	default:
		return importAudit{action: "DOCUMENT_IMPORTED", actor: auditSystemUser(strings.ToLower(importSource))}
	}
}

func safeStoredFileName(name string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	if name == "" || name == "." {
		name = "document"
	}
	runes := []rune(name)
	if len(runes) > 500 {
		name = string(runes[:500])
	}
	return name
}

// maxStoredBaseNameBytes keeps the on-disk base name well under the 255-byte
// file name limit of common file systems. Thai characters take 3 bytes each.
const maxStoredBaseNameBytes = 200

// storageFileName builds the on-disk name "<unixnano>-<name>". The timestamp
// prefix and the extension are kept; the stem is cut on a UTF-8 boundary so the
// whole base name fits in maxStoredBaseNameBytes. The original name stays in the
// database and is never shortened.
func storageFileName(timestamp int64, originalName string) string {
	name := safeStoredFileName(originalName)
	extension := filepath.Ext(name)
	stem := strings.TrimSuffix(name, extension)
	prefix := strconv.FormatInt(timestamp, 10) + "-"
	stem = truncateUTF8Bytes(stem, maxStoredBaseNameBytes-len(prefix)-len(extension))
	return prefix + stem + extension
}

func truncateUTF8Bytes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
}

func hashReader(reader io.Reader) (string, error) {
	digest := sha256.New()
	if _, err := io.Copy(digest, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
