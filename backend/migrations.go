package main

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const baselineMigration = "0001_clean_baseline.sql"
const baselineInProgress = "__baseline_in_progress__"

func migrateDatabase(db *gorm.DB) error {
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
        version VARCHAR(255) NOT NULL PRIMARY KEY,
        applied_at DATETIME(3) NOT NULL
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`).Error; err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migration files: %w", err)
	}
	versions := make([]string, 0, len(entries))
	known := map[string]bool{baselineInProgress: true}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			versions = append(versions, entry.Name())
			known[entry.Name()] = true
		}
	}
	sort.Strings(versions)
	if len(versions) == 0 || versions[0] != baselineMigration {
		return fmt.Errorf("clean baseline migration is missing")
	}
	var appliedVersions []string
	if err := db.Table("schema_migrations").Pluck("version", &appliedVersions).Error; err != nil {
		return fmt.Errorf("read migration history: %w", err)
	}
	applied, err := validateMigrationHistory(appliedVersions, known)
	if err != nil {
		return err
	}
	if !applied[baselineMigration] && !applied[baselineInProgress] {
		var count int64
		if err := db.Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES
            WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME <> 'schema_migrations'`).Scan(&count).Error; err != nil {
			return fmt.Errorf("check database is empty: %w", err)
		}
		if count != 0 {
			return fmt.Errorf("database is not empty: use an empty database for the clean baseline")
		}
		if err := db.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, CURRENT_TIMESTAMP(3))", baselineInProgress).Error; err != nil {
			return fmt.Errorf("start baseline migration: %w", err)
		}
	}
	for _, version := range versions {
		if applied[version] {
			continue
		}
		contents, err := migrationFiles.ReadFile("migrations/" + version)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}
		for _, statement := range splitMigrationSQL(string(contents)) {
			if err := db.Exec(statement).Error; err != nil {
				return fmt.Errorf("execute %s: %w", version, err)
			}
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, CURRENT_TIMESTAMP(3))", version).Error; err != nil {
				return err
			}
			if version == baselineMigration {
				return tx.Exec("DELETE FROM schema_migrations WHERE version = ?", baselineInProgress).Error
			}
			return nil
		}); err != nil {
			return fmt.Errorf("record migration %s: %w", version, err)
		}
	}
	return nil
}

func validateMigrationHistory(versions []string, known map[string]bool) (map[string]bool, error) {
	applied := make(map[string]bool, len(versions))
	for _, version := range versions {
		if !known[version] {
			return nil, fmt.Errorf("unsupported migration history %q: use an empty database for the clean baseline", version)
		}
		applied[version] = true
	}
	if applied[baselineMigration] && applied[baselineInProgress] {
		return nil, fmt.Errorf("inconsistent baseline migration history")
	}
	return applied, nil
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
