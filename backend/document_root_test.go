package main

import (
	"strings"
	"testing"
)

func TestBaselineCreatesOnlyCurrentTables(t *testing.T) {
	data, err := migrationFiles.ReadFile("migrations/" + baselineMigration)
	if err != nil {
		t.Fatal(err)
	}
	statements := splitMigrationSQL(string(data))
	if len(statements) != 19 {
		t.Fatalf("baseline has %d tables, want 19", len(statements))
	}
	for _, statement := range statements {
		if !strings.HasPrefix(statement, "CREATE TABLE IF NOT EXISTS ") {
			t.Fatalf("baseline contains a non-retryable statement: %q", statement)
		}
	}
	if !strings.Contains(string(data), "fk_appointments_document FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE CASCADE") {
		t.Fatal("appointments must reference the canonical documents table")
	}
}

func TestFinalSchemaRemovesDuplicateDocumentAndOCRTables(t *testing.T) {
	tables := migrationTableColumns(t)
	if _, ok := tables["documents"]; !ok {
		t.Fatal("canonical documents table is missing")
	}
	for _, name := range []string{"document_workflows", "orders", "order_types", "committees", "committee_members", "committee_roles", "users", "audit_logs", "ocr_pages", "ocr_lines"} {
		if _, ok := tables[name]; ok {
			t.Errorf("obsolete table %q remains", name)
		}
	}
	if tables["documents"]["ocr_text"] {
		t.Fatal("OCR text must not be stored in the documents table")
	}
}

func TestBaselineRejectsLegacyAndInconsistentMigrationHistory(t *testing.T) {
	known := map[string]bool{baselineMigration: true, baselineInProgress: true}
	for _, versions := range [][]string{{"0001_initial.sql"}, {baselineMigration, baselineInProgress}} {
		if _, err := validateMigrationHistory(versions, known); err == nil {
			t.Fatalf("accepted unsupported history %v", versions)
		}
	}
	for _, versions := range [][]string{nil, {baselineInProgress}, {baselineMigration}} {
		if _, err := validateMigrationHistory(versions, known); err != nil {
			t.Fatalf("rejected valid history %v: %v", versions, err)
		}
	}
}
