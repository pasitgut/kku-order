package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const maxImportedFileSize = 50 << 20

type StoredDocument struct {
	ID          uint64     `gorm:"column:id;primaryKey"`
	FileName    string     `gorm:"column:file_name"`
	StoragePath string     `gorm:"column:storage_path"`
	FileType    string     `gorm:"column:file_type"`
	FileHash    string     `gorm:"column:file_hash"`
	ConfirmedAt *time.Time `gorm:"column:confirmed_at"`
	ConfirmedBy *uint64    `gorm:"column:confirmed_by"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
}

func (StoredDocument) TableName() string { return "documents" }

type StoredOrderType struct {
	ID   uint64 `gorm:"primaryKey"`
	Name string `gorm:"size:255;uniqueIndex;not null"`
}

func (StoredOrderType) TableName() string { return "order_types" }

type StoredOrder struct {
	ID            uint64 `gorm:"primaryKey"`
	DocumentID    uint64 `gorm:"uniqueIndex;not null"`
	OrderTypeID   uint64 `gorm:"not null"`
	OrderNumber   string `gorm:"size:50;not null"`
	OrderYear     int    `gorm:"not null"`
	Title         string `gorm:"size:255"`
	IssuedDate    *time.Time
	EffectiveDate *time.Time
	ExpiryDate    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (StoredOrder) TableName() string { return "orders" }

// createDocumentBundle reserves one document id while holding a MySQL named
// lock, then creates the required order row before the document row. The
// supplied initial schema makes documents.id reference orders.document_id.
func createDocumentBundle(db *gorm.DB, core *StoredDocument, workflow *Document) error {
	const lockName = "docflow:document-order-insert"
	return db.Transaction(func(tx *gorm.DB) error {
		var locked sql.NullInt64
		if err := tx.Raw("SELECT GET_LOCK(?, 10)", lockName).Scan(&locked).Error; err != nil {
			return fmt.Errorf("lock document id allocation: %w", err)
		}
		if !locked.Valid || locked.Int64 != 1 {
			return fmt.Errorf("could not lock document id allocation")
		}
		defer func() {
			var released sql.NullInt64
			_ = tx.Raw("SELECT RELEASE_LOCK(?)", lockName).Scan(&released).Error
		}()

		var nextID sql.NullInt64
		if err := tx.Raw(`SELECT AUTO_INCREMENT FROM information_schema.TABLES
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'documents'`).Scan(&nextID).Error; err != nil {
			return fmt.Errorf("read next document id: %w", err)
		}
		if !nextID.Valid || nextID.Int64 < 1 {
			return fmt.Errorf("database did not return the next document id")
		}
		documentID := uint64(nextID.Int64)

		orderType := StoredOrderType{Name: "PENDING"}
		if err := tx.Where("name = ?", orderType.Name).FirstOrCreate(&orderType).Error; err != nil {
			return fmt.Errorf("create pending order type: %w", err)
		}
		now := time.Now()
		order := StoredOrder{
			DocumentID: documentID, OrderTypeID: orderType.ID,
			OrderNumber: fmt.Sprintf("PENDING-%d", documentID), OrderYear: now.Year() + 543,
			Title: truncateRunes(workflow.Title, 255), CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&order).Error; err != nil {
			return fmt.Errorf("create required order row: %w", err)
		}
		core.ID = documentID
		if err := tx.Create(core).Error; err != nil {
			return fmt.Errorf("create core document row: %w", err)
		}
		workflow.ID = uint(documentID)
		if err := tx.Create(workflow).Error; err != nil {
			return fmt.Errorf("create document workflow row: %w", err)
		}
		return nil
	})
}

func syncCoreOrderFromWorkflow(tx *gorm.DB, document *Document) error {
	name := strings.TrimSpace(document.OrderType)
	if name == "" {
		name = "PENDING"
	}
	orderType := StoredOrderType{Name: name}
	if err := tx.Where("name = ?", name).FirstOrCreate(&orderType).Error; err != nil {
		return fmt.Errorf("resolve order type: %w", err)
	}
	var order StoredOrder
	if err := tx.Where("document_id = ?", document.ID).First(&order).Error; err != nil {
		return fmt.Errorf("load core order: %w", err)
	}
	order.OrderTypeID = orderType.ID
	order.OrderNumber = truncateRunes(strings.TrimSpace(document.OrderNo), 50)
	if order.OrderNumber == "" {
		order.OrderNumber = fmt.Sprintf("PENDING-%d", document.ID)
	}
	order.OrderYear = document.OrderYearBE
	if order.OrderYear <= 0 {
		order.OrderYear = time.Now().Year() + 543
	}
	order.Title = truncateRunes(strings.TrimSpace(document.Title), 255)
	order.IssuedDate = document.IssuedDate
	order.EffectiveDate = document.EffectiveDate
	order.ExpiryDate = document.ExpiryDate
	return tx.Save(&order).Error
}

func syncCoreDocumentFile(tx *gorm.DB, document *Document) error {
	return tx.Model(&StoredDocument{}).Where("id = ?", document.ID).Updates(map[string]any{
		"file_name":    document.OriginalFilename,
		"storage_path": document.StoragePath,
		"file_type":    document.SourceType,
		"file_hash":    document.FileHash,
		"updated_at":   time.Now(),
	}).Error
}

func syncCoreConfirmation(tx *gorm.DB, document *Document) error {
	updates := map[string]any{"confirmed_at": document.ConfirmedAt, "confirmed_by": nil}
	if document.ConfirmedBy != "" {
		userID, err := strconv.ParseUint(strings.TrimSpace(document.ConfirmedBy), 10, 64)
		if err == nil && userID > 0 {
			var count int64
			if err := tx.Table("users").Where("id = ?", userID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				updates["confirmed_by"] = userID
			}
		}
	}
	return tx.Model(&StoredDocument{}).Where("id = ?", document.ID).Updates(updates).Error
}

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
	core := StoredDocument{
		FileName: fileName, StoragePath: storagePath,
		FileType: strings.TrimPrefix(ext, "."), FileHash: hash,
		CreatedAt: now, UpdatedAt: now,
	}
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	title := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	workflow := Document{
		Title: title, OriginalFilename: fileName, StoragePath: storagePath,
		SourceType: strings.TrimPrefix(ext, "."), MimeType: mimeType,
		Status: "NEEDS_REVIEW", ProcessingStage: "IMPORTED", FileSizeBytes: fileSize,
		FileHash: hash, ImportSource: importSource, ImportedBy: "system:" + strings.ToLower(importSource),
		QualityProfile: "STANDARD", CreatedAt: now, UpdatedAt: now,
	}
	if err := createDocumentBundle(db, &core, &workflow); err != nil {
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
