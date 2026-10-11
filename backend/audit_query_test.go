package main

import (
	"reflect"
	"testing"
	"time"
)

func TestBuildAuditLogWhereIsEmptyWithoutFilters(t *testing.T) {
	where, args := buildAuditLogWhere(auditLogFilter{})
	if where != "" || len(args) != 0 {
		t.Fatalf("where=%q args=%v", where, args)
	}
}

func TestBuildAuditLogWhereCombinesEveryFilter(t *testing.T) {
	from := time.Date(2026, 10, 10, 17, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 11, 17, 0, 0, 0, time.UTC)
	where, args := buildAuditLogWhere(auditLogFilter{
		Search: "ตรวจ", UserID: "kit", Actions: []string{"DOCUMENT_UPDATED", "DOCUMENT_CONFIRMED"},
		DocumentID: 7, From: &from, To: &to,
	})
	want := "(a.details LIKE ? ESCAPE '!' OR w.title LIKE ? ESCAPE '!') AND a.user_id LIKE ? ESCAPE '!' AND a.action IN (?,?) AND a.document_id = ? AND a.created_at >= ? AND a.created_at < ?"
	if where != want {
		t.Fatalf("where = %q\nwant   %q", where, want)
	}
	wantArgs := []any{"%ตรวจ%", "%ตรวจ%", "%kit%", "DOCUMENT_UPDATED", "DOCUMENT_CONFIRMED", uint(7), from, to}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v\nwant   %#v", args, wantArgs)
	}
}

func TestBuildAuditLogWhereEscapesLikeWildcards(t *testing.T) {
	_, args := buildAuditLogWhere(auditLogFilter{Search: `50%_off!`, UserID: "a_b"})
	if args[0] != `%50!%!_off!!%` || args[1] != `%50!%!_off!!%` || args[2] != `%a!_b%` {
		t.Fatalf("args = %#v", args)
	}
}

func TestParseAuditDayUsesBangkokDayBounds(t *testing.T) {
	start, err := parseAuditDay("2026-10-11")
	if err != nil {
		t.Fatal(err)
	}
	if !start.Equal(time.Date(2026, 10, 10, 17, 0, 0, 0, time.UTC)) {
		t.Fatalf("start = %v", start)
	}
	if _, err := parseAuditDay("11-10-2026"); err == nil {
		t.Fatal("non ISO date must be rejected")
	}
}

func TestAuditCSVTimeIsBangkokLocal(t *testing.T) {
	got := formatAuditTime(time.Date(2026, 10, 10, 17, 5, 0, 0, time.UTC))
	if got != "2026-10-11 00:05:00" {
		t.Fatalf("formatAuditTime = %q", got)
	}
}
