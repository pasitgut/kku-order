package main

import (
	"regexp"
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// dryRunDB builds SQL without connecting to MySQL.
func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "docflow:x@tcp(127.0.0.1:1)/docflow",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// The profile page must read the table the user sync writes (directory_people),
// and every table a lookup touches must be created by a migration.
func TestDirectoryAccountLookupReadsMigratedTables(t *testing.T) {
	tables := migrationTableColumns(t)
	db := dryRunDB(t)
	statement := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return directoryAccountQuery(tx, "someone@kku.ac.th").Take(&DirectoryPerson{})
	})

	fromPattern := regexp.MustCompile("(?i)(?:FROM|JOIN)\\s+`?(\\w+)`?")
	matches := fromPattern.FindAllStringSubmatch(statement, -1)
	if len(matches) == 0 {
		t.Fatalf("no table found in %q", statement)
	}
	for _, match := range matches {
		if _, ok := tables[match[1]]; !ok {
			t.Errorf("lookup reads table %q, which no migration creates: %s", match[1], statement)
		}
	}
	if !strings.Contains(statement, "`directory_people`") {
		t.Fatalf("lookup must read the synced directory_people table: %s", statement)
	}
}
