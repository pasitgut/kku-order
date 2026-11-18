package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSplitMigrationSQLHandlesCommentsAndLiterals(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"two statements", "CREATE TABLE a (x INT);\nCREATE TABLE b (x INT);", []string{"CREATE TABLE a (x INT)", "CREATE TABLE b (x INT)"}},
		{"CRLF and trailing blank", "CREATE TABLE a (x INT);\r\n\r\n  \n", []string{"CREATE TABLE a (x INT)"}},
		{"comment line before statement", "-- create a\nCREATE TABLE a (x INT);", []string{"CREATE TABLE a (x INT)"}},
		{"comment-only tail", "CREATE TABLE a (x INT);\n-- end", []string{"CREATE TABLE a (x INT)"}},
		{"semicolon in comment", "-- a; b\nCREATE TABLE a (x INT);", []string{"CREATE TABLE a (x INT)"}},
		{"hash comment", "# note; here\nCREATE TABLE a (x INT);", []string{"CREATE TABLE a (x INT)"}},
		{"block comment", "/* a; b */ CREATE TABLE a (x INT);", []string{"CREATE TABLE a (x INT)"}},
		{"semicolon in string literal", "INSERT INTO t VALUES ('a;b', \"c;d\");", []string{"INSERT INTO t VALUES ('a;b', \"c;d\")"}},
		{"escaped quotes", "INSERT INTO t VALUES ('it''s;', 'x\\';y');", []string{"INSERT INTO t VALUES ('it''s;', 'x\\';y')"}},
		{"dashes inside literal", "INSERT INTO t VALUES ('--;');", []string{"INSERT INTO t VALUES ('--;')"}},
		{"backtick identifier", "CREATE TABLE `a;b` (x INT);", []string{"CREATE TABLE `a;b` (x INT)"}},
		{"no trailing semicolon", "CREATE TABLE a (x INT)", []string{"CREATE TABLE a (x INT)"}},
	}
	for _, c := range cases {
		if got := splitMigrationSQL(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: splitMigrationSQL = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestValidateMigrationHistoryRejectsLaterVersionWithoutBaseline(t *testing.T) {
	known := map[string]bool{baselineInProgress: true, baselineMigration: true, "0002_confirmation_names.sql": true}
	for _, history := range [][]string{
		{"0002_confirmation_names.sql"},
		{baselineInProgress, "0002_confirmation_names.sql"},
	} {
		if _, err := validateMigrationHistory(history, known); err == nil {
			t.Errorf("history %v was accepted although the baseline is not applied", history)
		}
	}
	for _, history := range [][]string{nil, {baselineInProgress}, {baselineMigration}, {baselineMigration, "0002_confirmation_names.sql"}} {
		if _, err := validateMigrationHistory(history, known); err != nil {
			t.Errorf("history %v: %v", history, err)
		}
	}
}

// TestMigrateDatabaseWaitsForTheMigrationLock: replica สองตัวที่เริ่มพร้อมกันต้องรอกัน ไม่ใช่รัน DDL ซ้อนกัน
func TestMigrateDatabaseWaitsForTheMigrationLock(t *testing.T) {
	db := testDatabase(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	holder, err := sqlDB.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	var got int
	if err := holder.QueryRowContext(t.Context(), "SELECT GET_LOCK("+migrationLockName+", 5)").Scan(&got); err != nil || got != 1 {
		t.Fatalf("take lock = %d, %v", got, err)
	}
	done := make(chan error, 1)
	go func() { done <- migrateDatabase(db) }()
	select {
	case err := <-done:
		t.Fatalf("migrateDatabase finished while another process held the migration lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := holder.ExecContext(t.Context(), "SELECT RELEASE_LOCK("+migrationLockName+")"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("migrateDatabase did not continue after the lock was released")
	}
}

// freshMigrationDatabase สร้างฐานว่างชั่วคราว ต้องตั้ง DOCFLOW_TEST_ADMIN_DSN เป็นผู้ใช้ที่สร้างฐานได้
// เช่น root:root@tcp(127.0.0.1:3312)/ จึงจะรัน
func freshMigrationDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	return freshMigrationDatabaseNamed(t, fmt.Sprintf("docflow_migrate_%d", time.Now().UnixNano()))
}

func freshMigrationDatabaseNamed(t *testing.T, name string) *gorm.DB {
	t.Helper()
	admin := os.Getenv("DOCFLOW_TEST_ADMIN_DSN")
	if admin == "" {
		t.Skip("set DOCFLOW_TEST_ADMIN_DSN to run migration tests on throwaway databases")
	}
	options := "?charset=utf8mb4&parseTime=True&loc=Local"
	root, err := gorm.Open(mysql.Open(admin+options), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Exec("DROP DATABASE IF EXISTS " + name) })
	db, err := gorm.Open(mysql.Open(admin+name+options), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func migrationHistory(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var versions []string
	if err := db.Table("schema_migrations").Order("version").Pluck("version", &versions).Error; err != nil {
		t.Fatal(err)
	}
	return strings.Join(versions, ",")
}

const fullMigrationHistory = "0001_clean_baseline.sql,0002_confirmation_names.sql"

func TestConcurrentStartupMigratesAnEmptyDatabaseOnce(t *testing.T) {
	db := freshMigrationDatabase(t)
	const replicas = 6
	errs := make([]error, replicas)
	var wg sync.WaitGroup
	for i := range replicas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = migrateDatabase(db)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("replica %d: %v", i, err)
		}
	}
	if got := migrationHistory(t, db); got != fullMigrationHistory {
		t.Fatalf("history = %s", got)
	}
}

func startBaselineInProgress(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, statement := range []string{
		"CREATE TABLE schema_migrations (version VARCHAR(255) NOT NULL PRIMARY KEY, applied_at DATETIME(3) NOT NULL)",
		"INSERT INTO schema_migrations VALUES ('" + baselineInProgress + "', NOW(3))",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestInterruptedBaselineResumes(t *testing.T) {
	db := freshMigrationDatabase(t)
	startBaselineInProgress(t, db)
	contents, err := migrationFiles.ReadFile("migrations/" + baselineMigration)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range splitMigrationSQL(string(contents))[:5] {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateDatabase(db); err != nil {
		t.Fatalf("resume baseline: %v", err)
	}
	if got := migrationHistory(t, db); got != fullMigrationHistory {
		t.Fatalf("history = %s", got)
	}
}

func TestInterruptedBaselineRejectsForeignTables(t *testing.T) {
	db := freshMigrationDatabase(t)
	startBaselineInProgress(t, db)
	if err := db.Exec("CREATE TABLE orders (id INT PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateDatabase(db); err == nil || !strings.Contains(err.Error(), "orders") {
		t.Fatalf("migrateDatabase = %v, want a refusal naming the foreign table", err)
	}
	if got := migrationHistory(t, db); got != baselineInProgress {
		t.Fatalf("history = %s, the baseline must not be recorded", got)
	}
}

func TestInterruptedBaselineRejectsDriftedTable(t *testing.T) {
	db := freshMigrationDatabase(t)
	startBaselineInProgress(t, db)
	if err := db.Exec("CREATE TABLE documents (id BIGINT AUTO_INCREMENT PRIMARY KEY, title VARCHAR(10))").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateDatabase(db); err == nil || !strings.Contains(err.Error(), "documents") {
		t.Fatalf("migrateDatabase = %v, want a refusal naming the drifted table", err)
	}
	if got := migrationHistory(t, db); got != baselineInProgress {
		t.Fatalf("history = %s, the baseline must not be recorded", got)
	}
}

// คอมเมนต์ /*! ... */ และ optimizer hint /*+ ... */ เป็นส่วนของคำสั่งที่ MySQL รันจริง ต้องเก็บไว้
func TestSplitMigrationSQLKeepsExecutableComments(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"versioned statement", "/*!40101 SET NAMES utf8mb4 */;\nCREATE TABLE a (x INT);", []string{"/*!40101 SET NAMES utf8mb4 */", "CREATE TABLE a (x INT)"}},
		{"inside a statement", "CREATE TABLE a (x INT) /*!50100 ENGINE=InnoDB */;", []string{"CREATE TABLE a (x INT) /*!50100 ENGINE=InnoDB */"}},
		{"optimizer hint", "SELECT /*+ MAX_EXECUTION_TIME(1000) */ 1;", []string{"SELECT /*+ MAX_EXECUTION_TIME(1000) */ 1"}},
		{"plain comment still stripped", "/* note; */ CREATE TABLE a (x INT) /*!50100 ENGINE=InnoDB */;", []string{"CREATE TABLE a (x INT) /*!50100 ENGINE=InnoDB */"}},
	}
	for _, c := range cases {
		if got := splitMigrationSQL(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: splitMigrationSQL = %q, want %q", c.name, got, c.want)
		}
	}
}

// ชื่อ lock ของ MySQL ยาวได้ไม่เกิน 64 ตัวอักษร ฐานที่ชื่อยาวต้องยัง migrate ได้
func TestMigrateDatabaseWithLongDatabaseName(t *testing.T) {
	name := fmt.Sprintf("docflow_migrate_%d_", time.Now().UnixNano())
	name += strings.Repeat("x", 64-len(name))
	db := freshMigrationDatabaseNamed(t, name)
	if err := migrateDatabase(db); err != nil {
		t.Fatalf("migrate %d-char database name: %v", len(name), err)
	}
	if got := migrationHistory(t, db); got != fullMigrationHistory {
		t.Fatalf("history = %s", got)
	}
}
