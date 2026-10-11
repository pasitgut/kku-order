package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

const storageTimestamp int64 = 1791710207536707342

func TestStorageFileNameFitsLinuxNameLimitForLongThaiNames(t *testing.T) {
	cases := map[string]string{
		"exact thai name": "คำสั่ง มข.8901-2566 เรื่อง แต่งตั้งคณะกรรมการส่งเสริมจรรยาบรรณ ประจำวิทยาลัยการคอมพิวเตอร์.pdf",
		"300 thai chars":  strings.Repeat("ก", 300) + ".pdf",
	}
	for label, name := range cases {
		t.Run(label, func(t *testing.T) {
			stored := storageFileName(storageTimestamp, name)
			if len(stored) > 200 {
				t.Fatalf("stored name is %d bytes, want <= 200: %q", len(stored), stored)
			}
			if !utf8.ValidString(stored) {
				t.Fatalf("stored name is not valid UTF-8: %q", stored)
			}
			if !strings.HasPrefix(stored, "1791710207536707342-") {
				t.Fatalf("stored name lost the timestamp prefix: %q", stored)
			}
			if !strings.HasSuffix(stored, ".pdf") {
				t.Fatalf("stored name lost the extension: %q", stored)
			}
			if err := os.WriteFile(filepath.Join(t.TempDir(), stored), []byte("%PDF"), 0o640); err != nil {
				t.Fatalf("file system rejected stored name: %v", err)
			}
		})
	}
}

func TestStorageFileNameKeepsShortNamesUnchanged(t *testing.T) {
	got := storageFileName(storageTimestamp, "order.pdf")
	want := "1791710207536707342-order.pdf"
	if got != want {
		t.Fatalf("storageFileName() = %q, want %q", got, want)
	}
}

func TestSafeStoredFileNameKeepsOriginalNameComplete(t *testing.T) {
	name := strings.Repeat("ก", 300) + ".pdf"
	if got := safeStoredFileName(name); got != name {
		t.Fatalf("original name changed: got %d runes, want %d", len([]rune(got)), len([]rune(name)))
	}
}
