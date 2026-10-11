package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func samplePNG(t *testing.T) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 8, 8))
	canvas.Set(1, 1, color.RGBA{R: 28, G: 117, B: 188, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestValidateProfilePhotoAcceptsImagesOnly(t *testing.T) {
	extension, err := validateProfilePhoto(samplePNG(t))
	if err != nil || extension != ".png" {
		t.Fatalf("png: extension %q, err %v", extension, err)
	}
	if _, err := validateProfilePhoto([]byte("%PDF-1.4 not an image")); err == nil {
		t.Fatal("a PDF must be rejected")
	}
	if _, err := validateProfilePhoto(append(samplePNG(t), make([]byte, maxProfilePhotoBytes)...)); err == nil {
		t.Fatal("photos over 2 MB must be rejected")
	}
}

func TestProfilePhotoPathIsSafeForAnyUserID(t *testing.T) {
	first := profilePhotoPath("/data/photos", "../../etc/passwd", ".png")
	if !strings.HasPrefix(first, "/data/photos/") || strings.Contains(first, "..") {
		t.Fatalf("path %q escapes the photo directory", first)
	}
	if first != profilePhotoPath("/data/photos", "../../etc/passwd", ".png") {
		t.Fatal("the same user must always map to the same file")
	}
	if first == profilePhotoPath("/data/photos", "someone-else", ".png") {
		t.Fatal("different users must not share a file")
	}
}

func TestSummarizeActivityCountsThisMonthOnly(t *testing.T) {
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.Local)
	logs := []AuditLog{
		{Action: "DOCUMENT_UPLOADED", CreatedAt: now.Add(-time.Hour)},
		{Action: "DOCUMENT_FOLDER_IMPORTED", CreatedAt: now.Add(-48 * time.Hour)},
		{Action: "DOCUMENT_CONFIRMED", CreatedAt: now.Add(-2 * time.Hour)},
		{Action: "DOCUMENT_UPDATED", CreatedAt: now.Add(-3 * time.Hour)},
		{Action: "DOCUMENT_UPDATED", CreatedAt: time.Date(2026, 8, 31, 23, 0, 0, 0, time.Local)},
	}
	stats := summarizeActivity(logs, now)
	if stats.Imported != 2 || stats.Confirmed != 1 || stats.Edited != 1 {
		t.Fatalf("stats = %+v, want imported 2, confirmed 1, edited 1 (August excluded)", stats)
	}
}

func TestActivityLabelNamesKnownActions(t *testing.T) {
	if activityLabel("DOCUMENT_CONFIRMED") != "ยืนยัน" || activityLabel("DOCUMENT_UPLOADED") != "นำเข้า" {
		t.Fatal("known actions need Thai labels")
	}
	if activityLabel("SOMETHING_NEW") != "SOMETHING_NEW" {
		t.Fatal("unknown actions fall back to the raw action")
	}
}

// writtenAuditActions scans the non-test backend sources for audit action
// literals, so a new action cannot ship without a Thai label.
func writtenAuditActions(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`(?:Action:\s*|\baction\s*:?=\s*)"([A-Z][A-Z_]+)"`)
	actions := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range pattern.FindAllStringSubmatch(string(source), -1) {
			actions[match[1]] = true
		}
	}
	return actions
}

func TestEveryWrittenAuditActionHasThaiLabel(t *testing.T) {
	actions := writtenAuditActions(t)
	for _, known := range []string{"DOCUMENT_UPLOADED", "DOCUMENT_RESCAN_REQUESTED", "DOCUMENT_REPROCESS_REQUESTED"} {
		if !actions[known] {
			t.Fatalf("source scan missed %s; the scanner is broken", known)
		}
	}
	for action := range actions {
		if label := activityLabel(action); label == action {
			t.Errorf("audit action %s is written but has no Thai label", action)
		}
	}
}

func TestRescanAndReprocessLabelsAreThai(t *testing.T) {
	if activityLabel("DOCUMENT_RESCAN_REQUESTED") != "สแกนใหม่" {
		t.Fatalf("rescan label = %q", activityLabel("DOCUMENT_RESCAN_REQUESTED"))
	}
	if activityLabel("DOCUMENT_REPROCESS_REQUESTED") != "ประมวลผลใหม่" {
		t.Fatalf("reprocess label = %q", activityLabel("DOCUMENT_REPROCESS_REQUESTED"))
	}
}
