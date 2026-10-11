package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
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

type StoredUser struct {
	ID    uint64 `gorm:"primaryKey"`
	Email string `gorm:"size:255"`
}

func (StoredUser) TableName() string { return "users" }

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

type StoredCommittee struct {
	ID          uint64 `gorm:"primaryKey"`
	OrderID     uint64 `gorm:"not null"`
	Name        string `gorm:"size:500"`
	Description string `gorm:"type:text"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (StoredCommittee) TableName() string { return "committees" }

type StoredCommitteeRole struct {
	ID   uint64 `gorm:"primaryKey"`
	Code string `gorm:"size:50;uniqueIndex;not null"`
	Name string `gorm:"size:255;uniqueIndex;not null"`
}

func (StoredCommitteeRole) TableName() string { return "committee_roles" }

type StoredCommitteeMember struct {
	ID              uint64  `gorm:"primaryKey"`
	CommitteeID     uint64  `gorm:"not null"`
	PeopleID        *uint64 `gorm:"column:people_id"`
	CommitteeRoleID uint64  `gorm:"not null"`
	DisplayOrder    *int    `gorm:"column:display_order"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (StoredCommitteeMember) TableName() string { return "committee_members" }

// createDocumentBundle writes the core document, its required order and the
// workflow row in one transaction. Every id is the one the database assigns on
// insert; none is predicted from information_schema, which MySQL caches.
//
// documents.id references orders.document_id, but the order needs the document
// id, so the document row goes first with foreign key checks relaxed for this
// session only. The checks are restored before the transaction ends, and a
// failure rolls back all three rows.
func createDocumentBundle(db *gorm.DB, core *StoredDocument, workflow *Document) error {
	return db.Transaction(func(tx *gorm.DB) (err error) {
		if err := tx.Exec("SET FOREIGN_KEY_CHECKS = 0").Error; err != nil {
			return fmt.Errorf("relax foreign key checks: %w", err)
		}
		defer func() {
			if restoreErr := tx.Exec("SET FOREIGN_KEY_CHECKS = 1").Error; restoreErr != nil && err == nil {
				err = fmt.Errorf("restore foreign key checks: %w", restoreErr)
			}
		}()

		orderType := StoredOrderType{Name: "PENDING"}
		if err := tx.Where("name = ?", orderType.Name).FirstOrCreate(&orderType).Error; err != nil {
			return fmt.Errorf("create pending order type: %w", err)
		}
		if err := tx.Create(core).Error; err != nil {
			return fmt.Errorf("create core document row: %w", err)
		}
		order := pendingOrderForDocument(core.ID, orderType.ID, workflow, time.Now())
		if err := tx.Create(&order).Error; err != nil {
			return fmt.Errorf("create required order row: %w", err)
		}
		workflow.ID = uint(core.ID)
		if err := tx.Create(workflow).Error; err != nil {
			return fmt.Errorf("create document workflow row: %w", err)
		}
		return nil
	})
}

// pendingOrderForDocument builds the placeholder order that every document
// carries until its order details are confirmed.
func pendingOrderForDocument(documentID, orderTypeID uint64, workflow *Document, now time.Time) StoredOrder {
	return StoredOrder{
		DocumentID: documentID, OrderTypeID: orderTypeID,
		OrderNumber: fmt.Sprintf("PENDING-%d", documentID), OrderYear: now.Year() + 543,
		Title: truncateRunes(workflow.Title, 255), CreatedAt: now, UpdatedAt: now,
	}
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
		actor := strings.TrimSpace(document.ConfirmedBy)
		if actor == "" || len([]rune(actor)) > 255 {
			return fmt.Errorf("confirmed user identifier must contain 1-255 characters")
		}
		user := StoredUser{Email: actor}
		if err := tx.Where("email = ?", actor).FirstOrCreate(&user).Error; err != nil {
			return fmt.Errorf("resolve confirming user: %w", err)
		}
		updates["confirmed_by"] = user.ID
	}
	return tx.Model(&StoredDocument{}).Where("id = ?", document.ID).Updates(updates).Error
}

func syncCoreCommitteeFromWorkflow(tx *gorm.DB, document *Document) error {
	var order StoredOrder
	if err := tx.Where("document_id = ?", document.ID).First(&order).Error; err != nil {
		return fmt.Errorf("load committee order: %w", err)
	}
	var committees []StoredCommittee
	if err := tx.Where("order_id = ?", order.ID).Order("id ASC").Find(&committees).Error; err != nil {
		return err
	}
	var appointments []Appointment
	if err := tx.Where("document_id = ?", document.ID).Order("id ASC").Find(&appointments).Error; err != nil {
		return err
	}
	name := truncateRunes(strings.TrimSpace(document.Committee), 500)
	if name == "" && len(appointments) == 0 {
		if len(committees) > 0 {
			committeeIDs := make([]uint64, 0, len(committees))
			for _, committee := range committees {
				committeeIDs = append(committeeIDs, committee.ID)
			}
			if err := tx.Where("committee_id IN ?", committeeIDs).Delete(&StoredCommitteeMember{}).Error; err != nil {
				return err
			}
			return tx.Where("order_id = ?", order.ID).Delete(&StoredCommittee{}).Error
		}
		return nil
	}

	var committee StoredCommittee
	if len(committees) > 0 {
		committee = committees[0]
		if err := tx.Model(&committee).Update("name", name).Error; err != nil {
			return err
		}
		if len(committees) > 1 {
			committeeIDs := make([]uint64, 0, len(committees)-1)
			for _, extra := range committees[1:] {
				committeeIDs = append(committeeIDs, extra.ID)
			}
			if err := tx.Where("committee_id IN ?", committeeIDs).Delete(&StoredCommitteeMember{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", committeeIDs).Delete(&StoredCommittee{}).Error; err != nil {
				return err
			}
		}
	} else {
		committee = StoredCommittee{OrderID: order.ID, Name: name}
		if err := tx.Create(&committee).Error; err != nil {
			return err
		}
	}
	if err := tx.Where("committee_id = ?", committee.ID).Delete(&StoredCommitteeMember{}).Error; err != nil {
		return err
	}

	seenPeople := make(map[uint64]struct{}, len(appointments))
	for index, appointment := range appointments {
		roleName := strings.TrimSpace(appointment.CommitteeRole)
		if roleName == "" {
			roleName = "ไม่ระบุบทบาท"
		}
		roleName = truncateRunes(roleName, 255)
		roleHash := sha256.Sum256([]byte(strings.ToLower(roleName)))
		role := StoredCommitteeRole{Code: "ROLE_" + hex.EncodeToString(roleHash[:20]), Name: roleName}
		if err := tx.Where("name = ?", roleName).FirstOrCreate(&role).Error; err != nil {
			return fmt.Errorf("resolve committee role %q: %w", roleName, err)
		}
		var peopleID *uint64
		if appointment.DirectoryUserID != nil {
			id := uint64(*appointment.DirectoryUserID)
			if _, exists := seenPeople[id]; exists {
				continue
			}
			seenPeople[id] = struct{}{}
			peopleID = &id
		}
		displayOrder := index + 1
		member := StoredCommitteeMember{
			CommitteeID: committee.ID, PeopleID: peopleID, CommitteeRoleID: role.ID,
			DisplayOrder: &displayOrder,
		}
		if err := tx.Create(&member).Error; err != nil {
			return fmt.Errorf("create committee member: %w", err)
		}
	}
	return nil
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
