package main

import (
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm/schema"
)

// migrationTableColumns reads every embedded migration in order and returns
// the column names each table ends up with: CREATE TABLE bodies plus any
// ALTER TABLE ... ADD [COLUMN] clauses. No database is needed.
func migrationTableColumns(t *testing.T) map[string]map[string]bool {
	t.Helper()
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	createPattern := regexp.MustCompile(`(?is)^CREATE TABLE(?:\s+IF NOT EXISTS)?\s+` + "`?" + `(\w+)` + "`?" + `\s*\((.*)\)[^)]*$`)
	alterPattern := regexp.MustCompile(`(?is)^ALTER TABLE\s+` + "`?" + `(\w+)` + "`?" + `\s+(.*)$`)
	addColumnPattern := regexp.MustCompile(`(?is)^ADD\s+(?:COLUMN\s+)?(?:IF NOT EXISTS\s+)?` + "`?" + `(\w+)` + "`?")
	renamePattern := regexp.MustCompile(`(?is)^RENAME TABLE\s+(.+)$`)
	renamePairPattern := regexp.MustCompile(`(?is)^\s*(\w+)\s+TO\s+(\w+)\s*$`)
	dropTablePattern := regexp.MustCompile(`(?is)^DROP TABLE\s+(?:IF EXISTS\s+)?(\w+)$`)
	dropColumnPattern := regexp.MustCompile(`(?is)^DROP COLUMN\s+(?:IF EXISTS\s+)?(\w+)$`)
	constraintWords := map[string]bool{"PRIMARY": true, "KEY": true, "INDEX": true, "UNIQUE": true, "CONSTRAINT": true, "FOREIGN": true, "FULLTEXT": true, "CHECK": true}

	tables := map[string]map[string]bool{}
	ensure := func(name string) map[string]bool {
		if tables[name] == nil {
			tables[name] = map[string]bool{}
		}
		return tables[name]
	}

	for _, entry := range entries {
		raw, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		for _, statement := range splitMigrationSQL(string(raw)) {
			statement = strings.TrimSpace(statement)
			if m := dropTablePattern.FindStringSubmatch(statement); m != nil {
				delete(tables, m[1])
				continue
			}
			if m := renamePattern.FindStringSubmatch(statement); m != nil {
				for _, pair := range strings.Split(m[1], ",") {
					if p := renamePairPattern.FindStringSubmatch(pair); p != nil {
						tables[p[2]] = tables[p[1]]
						delete(tables, p[1])
					}
				}
				continue
			}
			if m := createPattern.FindStringSubmatch(statement); m != nil {
				columns := ensure(m[1])
				for _, item := range splitTopLevelCommas(m[2]) {
					word := firstWord(item)
					if word == "" || constraintWords[strings.ToUpper(word)] {
						continue
					}
					columns[word] = true
				}
				continue
			}
			if m := alterPattern.FindStringSubmatch(statement); m != nil {
				columns := ensure(m[1])
				for _, item := range splitTopLevelCommas(m[2]) {
					if d := dropColumnPattern.FindStringSubmatch(strings.TrimSpace(item)); d != nil {
						delete(columns, d[1])
						continue
					}
					if a := addColumnPattern.FindStringSubmatch(strings.TrimSpace(item)); a != nil && !constraintWords[strings.ToUpper(a[1])] {
						columns[a[1]] = true
					}
				}
			}
		}
	}
	return tables
}

// splitTopLevelCommas splits on commas that are not inside parentheses, so
// DECIMAL(10,2) and composite KEY (a,b) stay in one piece.
func splitTopLevelCommas(body string) []string {
	var parts []string
	depth, start := 0, 0
	for i, r := range body {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(body[start:i]))
				start = i + 1
			}
		}
	}
	return append(parts, strings.TrimSpace(body[start:]))
}

func firstWord(item string) string {
	fields := strings.Fields(strings.ReplaceAll(item, "`", ""))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// TestGormModelsMatchMigrationColumns guards against gorm's default column
// names drifting from the columns the migrations create. Every persisted
// field must map to a column that exists in its table.
func TestGormModelsMatchMigrationColumns(t *testing.T) {
	tables := migrationTableColumns(t)

	models := []struct {
		model any
		table string
	}{
		{Document{}, "documents"},
		{ExtractedField{}, "extracted_fields"},
		{Appointment{}, "appointments"},
		{DocumentReference{}, "document_references"},
		{DocumentRevision{}, "document_revisions"},
		{AuditLog{}, "application_audit_logs"},
		{ExpiryNotification{}, "expiry_notifications"},
		{DirectoryPerson{}, "directory_people"},
		{DirectoryPersonProfile{}, "directory_person_profiles"},
		{PersonNameAlias{}, "person_name_aliases"},
		{DirectoryPreviousName{}, "directory_previous_names"},
		{DocumentConfirmation{}, "document_confirmations"},
		{UserSyncRun{}, "user_sync_runs"},
		{AppSetting{}, "app_settings"},
		{UserProfile{}, "user_profiles"},
		{PositionHolder{}, "position_holders"},
		{DirectoryPersonRevision{}, "directory_person_revisions"},
		{DriveSyncRun{}, "drive_sync_runs"},
		{DriveSyncFolder{}, "drive_sync_folders"},
		{DriveSyncItem{}, "drive_sync_items"},
		{DriveImportedFile{}, "drive_imported_files"},
	}

	for _, m := range models {
		parsed, err := schema.Parse(m.model, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("parse %T: %v", m.model, err)
		}
		if parsed.Table != m.table {
			t.Errorf("%T maps to table %q, want %q", m.model, parsed.Table, m.table)
			continue
		}
		columns, ok := tables[m.table]
		if !ok {
			t.Errorf("table %q is not created by any migration", m.table)
			continue
		}
		for _, field := range parsed.Fields {
			if field.DBName == "" || field.TagSettings["-"] != "" {
				continue
			}
			if !columns[field.DBName] {
				t.Errorf("%T.%s maps to column %q, which table %q does not have", m.model, field.Name, field.DBName, m.table)
			}
		}
	}
}

// Update maps written to documents must only name columns the
// migrations create; a missing column fails the UPDATE with a 500 at runtime.
func TestWorkflowUpdateMapsUseMigratedColumns(t *testing.T) {
	columns := migrationTableColumns(t)["documents"]
	maps := map[string]map[string]any{
		"replacedFileUpdates": replacedFileUpdates(replacementFile{}, "user", time.Now()),
		"rescanResetUpdates":  rescanResetUpdates(),
		"retryResetUpdates":   retryResetUpdates(),
	}
	for name, updates := range maps {
		for key := range updates {
			if !columns[key] {
				t.Errorf("%s writes column %q, which documents does not have", name, key)
			}
		}
	}
}
