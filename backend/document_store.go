package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

const maxImportedFileSize = 50 << 20

type StoredDocument struct {
	ID          uint64    `gorm:"column:id;primaryKey"`
	FileName    string    `gorm:"column:file_name"`
	StoragePath string    `gorm:"column:storage_path"`
	FileType    string    `gorm:"column:file_type"`
	FileHash    string    `gorm:"column:file_hash"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (StoredDocument) TableName() string { return "documents" }

// storeImportedDocument writes a validated file and its metadata using the
// current documents schema. Duplicate content is detected by SHA-256.
func storeImportedDocument(db *gorm.DB, source io.ReadSeeker, fileName, storageDir string, fileSize int64) (bool, error) {
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
	if err := db.Model(&StoredDocument{}).Where("file_hash = ?", hash).Count(&duplicates).Error; err != nil {
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
	storagePath := filepath.Join(storageDir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), fileName))
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
	document := StoredDocument{
		FileName: fileName, StoragePath: storagePath,
		FileType: strings.TrimPrefix(ext, "."), FileHash: hash,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&document).Error; err != nil {
		return false, fmt.Errorf("บันทึกเอกสาร: %w", err)
	}
	keepFile = true
	return true, nil
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

func hashReader(reader io.Reader) (string, error) {
	digest := sha256.New()
	if _, err := io.Copy(digest, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
