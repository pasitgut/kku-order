package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("%PDF-1.4"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// An overwrite must delete the file the database used to point at, never
// the file it now points at.
func TestRemoveReplacedUploadDeletesOnlyThePreviousFile(t *testing.T) {
	dir := t.TempDir()
	previous := filepath.Join(dir, "1-old.pdf")
	replacement := filepath.Join(dir, "2-new.pdf")
	writeTestFile(t, previous)
	writeTestFile(t, replacement)

	removeReplacedUpload(previous, replacement)

	if _, err := os.Stat(previous); !os.IsNotExist(err) {
		t.Fatal("the previous file should be removed")
	}
	if _, err := os.Stat(replacement); err != nil {
		t.Fatalf("the replacement file must survive: %v", err)
	}
}

func TestRemoveReplacedUploadKeepsFileWhenPathUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "1-same.pdf")
	writeTestFile(t, path)

	removeReplacedUpload(path, path)

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the only copy of the document must not be deleted: %v", err)
	}
}
