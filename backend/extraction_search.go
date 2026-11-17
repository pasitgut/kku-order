package main

import (
	"os"
	"regexp"
	"strings"
	"sync"

	"gorm.io/gorm"
)

type cachedOCRText struct {
	id   uint
	text string
}

var ocrSearchCache = struct {
	sync.Mutex
	entries map[string]cachedOCRText
	bytes   int
}{entries: make(map[string]cachedOCRText)}

func indexedOCRText(path string, id uint, includeExtracted bool) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	ocrSearchCache.Lock()
	key := path
	if includeExtracted {
		key += "#unsaved"
	}
	entry, ok := ocrSearchCache.entries[key]
	ocrSearchCache.Unlock()
	if ok && entry.id == id {
		return entry.text, nil
	}
	snapshot, err := readExtractionFile(path, id)
	if err != nil {
		return "", err
	}
	if snapshot.OCRText == "" && snapshot.OCRPages == nil && len(snapshot.Result.Pages) > 0 {
		snapshot.OCRText, _ = extractionOCR(buildExtractionRecords(snapshot.Result, 0), id)
	}
	var builder strings.Builder
	builder.WriteString(snapshot.OCRText)
	if includeExtracted {
		for _, field := range snapshot.Fields {
			builder.WriteString("\n" + field.Value)
		}
		for _, appointment := range snapshot.Appointments {
			builder.WriteString("\n" + strings.Join([]string{appointment.FullName, appointment.Position, appointment.CommitteeRole, appointment.Responsibilities, appointment.Department}, " "))
		}
	}
	value := strings.ToLower(builder.String())
	const maxCacheBytes = 32 << 20
	if len(value) <= maxCacheBytes {
		ocrSearchCache.Lock()
		if ocrSearchCache.bytes+len(value) > maxCacheBytes {
			ocrSearchCache.entries = make(map[string]cachedOCRText)
			ocrSearchCache.bytes = 0
		}
		if old, exists := ocrSearchCache.entries[key]; exists {
			ocrSearchCache.bytes -= len(old.text)
		}
		ocrSearchCache.entries[key] = cachedOCRText{id: id, text: value}
		ocrSearchCache.bytes += len(value)
		ocrSearchCache.Unlock()
	}
	return value, nil
}

func searchOCRFiles(db *gorm.DB, query string) ([]uint, error) {
	var documents []Document
	if err := db.Select("id", "content_state", "extraction_json_path").Where("extraction_json_path <> ?", "").Find(&documents).Error; err != nil {
		return nil, err
	}
	query = strings.ToLower(query)
	var wildcard *regexp.Regexp
	if strings.ContainsAny(query, "%_") {
		pattern := regexp.QuoteMeta(query)
		pattern = strings.ReplaceAll(pattern, "%", ".*")
		pattern = strings.ReplaceAll(pattern, "_", ".")
		wildcard = regexp.MustCompile("(?s)" + pattern)
	}
	ids := make([]uint, 0)
	for _, doc := range documents {
		text, err := indexedOCRText(doc.ExtractionJSONPath, doc.ID, doc.ContentState == "UNSAVED")
		if err != nil {
			return nil, err
		}
		if (wildcard == nil && strings.Contains(text, query)) || (wildcard != nil && wildcard.MatchString(text)) {
			ids = append(ids, doc.ID)
		}
	}
	return ids, nil
}
