package main

import (
	"embed"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func migrateDatabase(db *gorm.DB) error {
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
        version VARCHAR(255) NOT NULL PRIMARY KEY,
        applied_at DATETIME(3) NOT NULL
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`).Error; err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	if err := reconcileInitialMigrationHistory(db); err != nil {
		return fmt.Errorf("reconcile initial migration history: %w", err)
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migration files: %w", err)
	}
	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			versions = append(versions, entry.Name())
		}
	}
	sort.Strings(versions)

	for _, version := range versions {
		var applied int64
		if err := db.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&applied).Error; err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if applied > 0 {
			continue
		}

		contents, err := migrationFiles.ReadFile("migrations/" + version)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}
		statements := splitMigrationSQL(string(contents))
		for _, statement := range statements {
			if err := executeMigrationStatement(db, statement); err != nil {
				return fmt.Errorf("execute %s: %w", version, err)
			}
		}
		if err := db.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, CURRENT_TIMESTAMP(3))", version).Error; err != nil {
			return fmt.Errorf("record migration %s: %w", version, err)
		}
	}
	return nil
}

func reconcileInitialMigrationHistory(db *gorm.DB) error {
	var hasNewDocumentColumns int64
	if err := db.Raw(`
		SELECT COUNT(*)
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'documents' AND COLUMN_NAME = 'file_name'`).Scan(&hasNewDocumentColumns).Error; err != nil {
		return err
	}

	if hasNewDocumentColumns == 0 {
		// Legacy databases recorded 0001 under the same name, so clear their
		// history before applying the replacement initial schema.
		return db.Exec("DELETE FROM schema_migrations").Error
	}

	var resetApplied int64
	if err := db.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", "0006_initial_design_reset.sql").Scan(&resetApplied).Error; err != nil {
		return err
	}
	if resetApplied == 0 {
		return nil
	}

	// Keep the already-reset schema while collapsing its migration history to
	// the new initial version.
	legacyVersions := []string{
		"0002_add_ocr_and_mime.sql",
		"0003_tor_controls.sql",
		"0004_committee_sections_and_signer.sql",
		"0005_directory_users_external_mapping.sql",
		"0006_initial_design_reset.sql",
	}
	if err := db.Exec("DELETE FROM schema_migrations WHERE version IN ?", legacyVersions).Error; err != nil {
		return err
	}
	return db.Exec("INSERT IGNORE INTO schema_migrations (version, applied_at) VALUES (?, CURRENT_TIMESTAMP(3))", "0001_initial.sql").Error
}

var (
	addColumnIfMissingPattern = regexp.MustCompile(`(?i)^ALTER TABLE ([A-Za-z0-9_]+) ADD COLUMN IF NOT EXISTS ([A-Za-z0-9_]+) (.+)$`)
	addKeyPattern             = regexp.MustCompile(`(?i)^ALTER TABLE ([A-Za-z0-9_]+) ADD KEY ([A-Za-z0-9_]+) (.+)$`)
)

// executeMigrationStatement keeps migrations portable across MySQL/MariaDB
// versions. Some servers reject ADD COLUMN IF NOT EXISTS even though the
// operation is supported by newer versions.
func executeMigrationStatement(db *gorm.DB, statement string) error {
	if matches := addColumnIfMissingPattern.FindStringSubmatch(strings.TrimSpace(statement)); matches != nil {
		var count int64
		if err := db.Raw(`
			SELECT COUNT(*)
			FROM INFORMATION_SCHEMA.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, matches[1], matches[2]).Scan(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		return db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", matches[1], matches[2], matches[3])).Error
	}

	if matches := addKeyPattern.FindStringSubmatch(strings.TrimSpace(statement)); matches != nil {
		var count int64
		if err := db.Raw(`
			SELECT COUNT(*)
			FROM INFORMATION_SCHEMA.STATISTICS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?`, matches[1], matches[2]).Scan(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		return db.Exec(fmt.Sprintf("ALTER TABLE %s ADD KEY %s %s", matches[1], matches[2], matches[3])).Error
	}

	return db.Exec(statement).Error
}

func splitMigrationSQL(contents string) []string {
	statements := make([]string, 0)
	for _, statement := range strings.Split(contents, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" || strings.HasPrefix(statement, "--") {
			continue
		}
		statements = append(statements, statement)
	}
	return statements
}
