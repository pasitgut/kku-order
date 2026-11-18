package main

import (
	"database/sql"
	"embed"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const baselineMigration = "0001_clean_baseline.sql"
const baselineInProgress = "__baseline_in_progress__"

// migrationLockTimeoutSeconds คือเวลาที่ replica หนึ่งรออีกตัวที่กำลัง migrate อยู่
const migrationLockTimeoutSeconds = 300

// migrationLockName เป็นนิพจน์ SQL ของชื่อ advisory lock ต่อฐาน ใช้ SHA1 ของชื่อฐานแทนชื่อเต็ม
// เพราะ MySQL จำกัดชื่อ lock ไว้ 64 ตัวอักษร (ERROR 4163) ชื่อนี้ยาว 16+40 = 56 ตัวเสมอ และยังต่างกันในแต่ละฐาน
const migrationLockName = "CONCAT('docflow_migrate:', SHA1(DATABASE()))"

// migrateDatabase ถือ advisory lock ของฐานนี้ตลอดการ migrate เพื่อให้ replica ที่เริ่มพร้อมกันทำทีละตัว
// GET_LOCK ผูกกับ connection จึงต้องทำทุกคำสั่งบน connection เดียว
func migrateDatabase(db *gorm.DB) error {
	return db.Connection(func(conn *gorm.DB) error {
		var locked sql.NullInt64
		if err := conn.Raw("SELECT GET_LOCK("+migrationLockName+", ?)", migrationLockTimeoutSeconds).Scan(&locked).Error; err != nil {
			return fmt.Errorf("take migration lock: %w", err)
		}
		if !locked.Valid || locked.Int64 != 1 {
			return fmt.Errorf("take migration lock: another process is still migrating this database")
		}
		defer conn.Exec("SELECT RELEASE_LOCK(" + migrationLockName + ")")
		return migrateLocked(conn)
	})
}

func migrateLocked(db *gorm.DB) error {
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
	if !applied[baselineMigration] {
		if err := checkBaselineTarget(db, applied[baselineInProgress]); err != nil {
			return err
		}
		if !applied[baselineInProgress] {
			if err := db.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, CURRENT_TIMESTAMP(3))", baselineInProgress).Error; err != nil {
				return fmt.Errorf("start baseline migration: %w", err)
			}
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

// checkBaselineTarget ยอมให้ลง baseline เฉพาะฐานว่าง หรือฐานที่ baseline รอบก่อนค้างไว้กลางทาง
// กรณีค้างกลางทาง ตารางที่มีอยู่ต้องเป็นตารางของ baseline และมีคอลัมน์ตรงกับ baseline ทุกตัว
// เพราะ CREATE TABLE IF NOT EXISTS จะข้ามตารางที่มีชื่อซ้ำแต่โครงสร้างต่างไปโดยไม่แจ้ง
func checkBaselineTarget(db *gorm.DB, inProgress bool) error {
	var tables []string
	if err := db.Raw(`SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES
            WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME <> 'schema_migrations' ORDER BY TABLE_NAME`).Scan(&tables).Error; err != nil {
		return fmt.Errorf("check database is empty: %w", err)
	}
	if len(tables) == 0 {
		return nil
	}
	if !inProgress {
		return fmt.Errorf("database is not empty: use an empty database for the clean baseline")
	}
	contents, err := migrationFiles.ReadFile("migrations/" + baselineMigration)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", baselineMigration, err)
	}
	expected := createdTableColumns(splitMigrationSQL(string(contents)))
	for _, table := range tables {
		want, ok := expected[table]
		if !ok {
			return fmt.Errorf("database is not empty: table %s is not part of the clean baseline", table)
		}
		var columns []string
		if err := db.Raw(`SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS
                WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? ORDER BY COLUMN_NAME`, table).Scan(&columns).Error; err != nil {
			return fmt.Errorf("check table %s: %w", table, err)
		}
		if strings.Join(columns, ",") != strings.Join(want, ",") {
			return fmt.Errorf("table %s does not match the clean baseline: use an empty database", table)
		}
	}
	return nil
}

var createTablePattern = regexp.MustCompile("(?is)^CREATE TABLE(?:\\s+IF NOT EXISTS)?\\s+`?(\\w+)`?\\s*\\((.*)\\)[^)]*$")

// createdTableColumns อ่านชื่อคอลัมน์ (เรียงตามตัวอักษร) จากคำสั่ง CREATE TABLE
func createdTableColumns(statements []string) map[string][]string {
	constraintWords := map[string]bool{"PRIMARY": true, "KEY": true, "INDEX": true, "UNIQUE": true, "CONSTRAINT": true, "FOREIGN": true, "FULLTEXT": true, "SPATIAL": true, "CHECK": true}
	tables := map[string][]string{}
	for _, statement := range statements {
		match := createTablePattern.FindStringSubmatch(strings.TrimSpace(statement))
		if match == nil {
			continue
		}
		columns := make([]string, 0)
		depth, start := 0, 0
		body := match[2]
		for index := 0; index <= len(body); index++ {
			if index < len(body) {
				switch body[index] {
				case '(':
					depth++
					continue
				case ')':
					depth--
					continue
				case ',':
					if depth != 0 {
						continue
					}
				default:
					continue
				}
			}
			fields := strings.Fields(body[start:index])
			start = index + 1
			if len(fields) == 0 || constraintWords[strings.ToUpper(fields[0])] {
				continue
			}
			columns = append(columns, strings.Trim(fields[0], "`"))
		}
		sort.Strings(columns)
		tables[match[1]] = columns
	}
	return tables
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
	if !applied[baselineMigration] {
		for version := range applied {
			if version != baselineInProgress {
				return nil, fmt.Errorf("inconsistent migration history: %s is recorded but the clean baseline is not", version)
			}
		}
	}
	return applied, nil
}

// splitMigrationSQL แยกคำสั่งด้วย ; ที่อยู่นอกสตริงและคอมเมนต์ และตัดคอมเมนต์ (--, #, /* */) ออก
// ยกเว้น /*! */ และ /*+ */ ซึ่งเป็นส่วนของคำสั่ง จึงเก็บไว้ตามเดิม
func splitMigrationSQL(contents string) []string {
	statements := make([]string, 0)
	var current strings.Builder
	flush := func() {
		if statement := strings.TrimSpace(current.String()); statement != "" {
			statements = append(statements, statement)
		}
		current.Reset()
	}
	for index := 0; index < len(contents); index++ {
		char := contents[index]
		switch {
		case char == '\'' || char == '"' || char == '`':
			end := index + 1
			for end < len(contents) {
				if contents[end] == '\\' && char != '`' {
					end += 2
					continue
				}
				if contents[end] == char {
					// เครื่องหมายคำพูดซ้อนสองตัว ('') คืออักขระ ' ในสตริง
					if end+1 < len(contents) && contents[end+1] == char {
						end += 2
						continue
					}
					break
				}
				end++
			}
			if end >= len(contents) {
				end = len(contents) - 1
			}
			current.WriteString(contents[index : end+1])
			index = end
		case char == '#' || (char == '-' && strings.HasPrefix(contents[index:], "--") && (index+2 == len(contents) || contents[index+2] == ' ' || contents[index+2] == '\t' || contents[index+2] == '\n' || contents[index+2] == '\r')):
			for index < len(contents) && contents[index] != '\n' {
				index++
			}
			current.WriteByte('\n')
		case strings.HasPrefix(contents[index:], "/*!") || strings.HasPrefix(contents[index:], "/*+"):
			// /*! ... */ (executable comment) และ /*+ ... */ (optimizer hint) MySQL รันจริง เก็บไว้ทั้งก้อน
			end := strings.Index(contents[index+3:], "*/")
			if end < 0 {
				current.WriteString(contents[index:])
				index = len(contents)
			} else {
				current.WriteString(contents[index : index+end+5])
				index += end + 4
			}
		case char == '/' && strings.HasPrefix(contents[index:], "/*"):
			end := strings.Index(contents[index+2:], "*/")
			if end < 0 {
				index = len(contents)
			} else {
				index += end + 3
			}
			current.WriteByte(' ')
		case char == ';':
			flush()
		default:
			current.WriteByte(char)
		}
	}
	flush()
	return statements
}
