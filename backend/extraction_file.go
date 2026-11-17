package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/gorm"
)

var errExtractionChanged = errors.New("extraction changed")
var errUnreadableExtractionDates = errors.New("unreadable extraction dates")

func materializeExtraction(tx *gorm.DB, id uint, snapshot extractionFile, payload *updateDocumentRequest) error {
	for _, model := range []any{&ExtractedField{}, &Appointment{}, &DocumentReference{}} {
		if err := tx.Where("document_id = ?", id).Delete(model).Error; err != nil {
			return err
		}
	}
	fieldIDs := make(map[uint]uint, len(snapshot.Fields))
	for i, field := range snapshot.Fields {
		field.ID, field.DocumentID = 0, id
		if err := tx.Create(&field).Error; err != nil {
			return err
		}
		fieldIDs[uint(i+1)] = field.ID
	}
	for i := range *payload.Fields {
		input := &(*payload.Fields)[i]
		if input.ID == 0 || int(input.ID) > len(snapshot.Fields) || snapshot.Fields[input.ID-1].FieldKey != input.FieldKey {
			return errExtractionChanged
		}
		input.ID = fieldIDs[input.ID]
	}
	appointmentIDs := make(map[uint]uint, len(snapshot.Appointments))
	for i, appointment := range snapshot.Appointments {
		appointment.ID, appointment.DocumentID = 0, id
		appointment.DirectoryPerson = nil
		if err := tx.Create(&appointment).Error; err != nil {
			return err
		}
		appointmentIDs[uint(i+1)] = appointment.ID
	}
	for i := range *payload.Appointments {
		input := &(*payload.Appointments)[i]
		input.ID = appointmentIDs[input.ID]
	}
	for _, ref := range snapshot.References {
		ref.ID, ref.DocumentID = 0, id
		if err := tx.Create(&ref).Error; err != nil {
			return err
		}
	}
	return nil
}

func extractionVersion(path string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(path)))
}

func applyExtractionPreview(doc *Document, snapshot extractionFile, path string) error {
	keys := map[string]string{
		"order_no": "orderNo", "title": "title", "committee": "committee", "signer_name": "signerName",
		"additional_refs": "additionalReferences", "order_type": "orderType", "issuing_unit": "issuingUnit",
		"order_year_be": "orderYearBE", "issued_date": "issuedDate", "effective_date": "effectiveDate",
		"expiry_date": "expiryDate", "confidence": "confidence", "page_count": "pageCount", "person_count": "personCount",
	}
	values := make(map[string]any, len(keys))
	for source, target := range keys {
		if value, ok := snapshot.Updates[source]; ok {
			values[target] = value
		}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, doc); err != nil {
		return err
	}
	doc.Fields = snapshot.Fields
	for i := range doc.Fields {
		doc.Fields[i].ID, doc.Fields[i].DocumentID = uint(i+1), doc.ID
	}
	doc.Appointments = snapshot.Appointments
	for i := range doc.Appointments {
		doc.Appointments[i].ID, doc.Appointments[i].DocumentID = uint(i+1), doc.ID
	}
	doc.References = snapshot.References
	doc.OCRText, doc.OCRPages = snapshot.OCRText, snapshot.OCRPages
	doc.ExtractionVersion = extractionVersion(path)
	return nil
}

func hydrateUnsavedDocuments(documents []Document) error {
	for i := range documents {
		doc := &documents[i]
		if doc.ContentState != "UNSAVED" || doc.ExtractionJSONPath == "" {
			continue
		}
		snapshot, err := readExtractionFile(doc.ExtractionJSONPath, doc.ID)
		if err != nil {
			return err
		}
		if err := applyExtractionPreview(doc, snapshot, doc.ExtractionJSONPath); err != nil {
			return err
		}
	}
	return nil
}

// extractionFile stores the provider-independent result and its review data.
type extractionFile struct {
	Version      int                 `json:"version"`
	DocumentID   uint                `json:"documentId"`
	Result       ExtractionResult    `json:"result"`
	Fields       []ExtractedField    `json:"fields"`
	Appointments []Appointment       `json:"appointments"`
	References   []DocumentReference `json:"references"`
	Updates      map[string]any      `json:"updates"`
	OCRText      string              `json:"ocrText"`
	OCRPages     []OCRPage           `json:"ocrPages"`
	OCRLineRaw   map[uint]string     `json:"ocrLineRaw,omitempty"`
}

func extractionOCR(records extractionRecords, documentID uint) (string, []OCRPage) {
	pages := records.OCRPages
	pageIndex := make(map[int]int, len(pages))
	for i := range pages {
		pages[i].ID = uint(i + 1)
		pages[i].DocumentID = documentID
		pages[i].Lines = []OCRLine{}
		pageIndex[pages[i].PageNo] = i
	}
	for i, line := range records.OCRLines {
		if index, ok := pageIndex[line.PageNo]; ok {
			line.ID = uint(i + 1)
			line.DocumentID = documentID
			line.OCRPageID = pages[index].ID
			pages[index].Lines = append(pages[index].Lines, line)
		}
	}
	text, _ := records.Updates["ocr_text"].(string)
	return text, pages
}

// writeExtractionFile creates a new immutable file beside the original document.
// The caller must publish its path only after the database transaction commits.
func writeExtractionFile(documentPath string, snapshot extractionFile) (path string, err error) {
	if documentPath == "" || snapshot.DocumentID == 0 {
		return "", fmt.Errorf("missing document path or ID")
	}
	snapshot.Version = 1
	dir := extractionDirectory(documentPath, snapshot.DocumentID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create extraction directory: %w", err)
	}
	f, err := os.CreateTemp(dir, fmt.Sprintf(".document-%d-*.tmp", snapshot.DocumentID))
	if err != nil {
		return "", fmt.Errorf("create extraction file: %w", err)
	}
	path = f.Name()
	temporaryPath := path
	defer func() {
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = f.Chmod(0o600); err != nil {
		_ = f.Close()
		return "", err
	}
	if err = json.NewEncoder(f).Encode(snapshot); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("encode extraction file: %w", err)
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("sync extraction file: %w", err)
	}
	if err = f.Close(); err != nil {
		return "", fmt.Errorf("close extraction file: %w", err)
	}
	path = temporaryPath + ".json"
	if err = os.Rename(temporaryPath, path); err != nil {
		return "", fmt.Errorf("publish extraction file: %w", err)
	}
	return path, nil
}

func extractionDirectory(documentPath string, documentID uint) string {
	return filepath.Join(filepath.Dir(documentPath), "extractions", fmt.Sprintf("%d", documentID))
}

func readExtractionFile(path string, documentID uint) (extractionFile, error) {
	var snapshot extractionFile
	contents, err := os.ReadFile(path)
	if err != nil {
		return snapshot, fmt.Errorf("read extraction file: %w", err)
	}
	if err := json.Unmarshal(contents, &snapshot); err != nil {
		return extractionFile{}, fmt.Errorf("decode extraction file: %w", err)
	}
	if snapshot.Version != 1 || documentID == 0 || snapshot.DocumentID != documentID {
		return extractionFile{}, fmt.Errorf("invalid extraction file version or document ID")
	}
	return snapshot, nil
}
