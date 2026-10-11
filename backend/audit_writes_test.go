package main

import (
	"strings"
	"testing"
	"time"
)

func TestDescribeDocumentChangesNamesFieldAndOldToNew(t *testing.T) {
	got := describeDocumentChanges([]DocumentRevision{
		{FieldKey: "title", OldValue: "เก่า", NewValue: "ใหม่"},
		{FieldKey: "expiry_date", OldValue: "", NewValue: "2027-01-31"},
	}, false)
	want := "เจ้าหน้าที่แก้ไขข้อมูลก่อนยืนยัน: title: เก่า → ใหม่; expiry_date: (ว่าง) → 2027-01-31"
	if got != want {
		t.Fatalf("describeDocumentChanges() = %q, want %q", got, want)
	}
}

func TestDescribeDocumentChangesMentionsAppointments(t *testing.T) {
	got := describeDocumentChanges(nil, true)
	if !strings.Contains(got, "รายชื่อ") {
		t.Fatalf("describeDocumentChanges() = %q, want a note that the people list changed", got)
	}
}

func TestSettingsChangeDetailsListsOnlyChangedFields(t *testing.T) {
	previous := `{"apiUrl":"https://a.example","dpi":300,"importEnabled":false}`
	next := `{"apiUrl":"https://b.example","dpi":300,"importEnabled":true}`
	got := settingsChangeDetails(previous, next)
	want := "apiUrl: https://a.example → https://b.example; importEnabled: false → true"
	if got != want {
		t.Fatalf("settingsChangeDetails() = %q, want %q", got, want)
	}
}

func TestSettingsChangeDetailsWithoutChanges(t *testing.T) {
	same := `{"dpi":300}`
	if got := settingsChangeDetails(same, same); got != "ไม่มีการเปลี่ยนแปลงค่า" {
		t.Fatalf("settingsChangeDetails() = %q", got)
	}
}

func TestSettingsChangeDetailsFirstSaveRecordsValues(t *testing.T) {
	got := settingsChangeDetails("", `{"dpi":300}`)
	if got != "dpi: (ว่าง) → 300" {
		t.Fatalf("settingsChangeDetails() = %q", got)
	}
}

func TestSyncRunAuditDetailsReportsCountsAndError(t *testing.T) {
	ok := syncRunAuditDetails(UserSyncResult{Fetched: 10, Created: 1, Updated: 2, Unchanged: 7}, nil)
	if ok != "สถานะ: สำเร็จ; ดึงมา 10, เพิ่ม 1, แก้ไข 2, ไม่เปลี่ยน 7" {
		t.Fatalf("success details = %q", ok)
	}
	failed := syncRunAuditDetails(UserSyncResult{}, errTestAudit("users API returned 403"))
	if !strings.HasPrefix(failed, "สถานะ: ล้มเหลว; users API returned 403") {
		t.Fatalf("failure details = %q", failed)
	}
}

func TestExpiryNoticeDetailsNamesTypeAndDueDate(t *testing.T) {
	due := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	got := expiryNoticeDetails("3_MONTHS", due)
	if got != "แจ้งเตือน 3_MONTHS ครบกำหนด 2026-10-01" {
		t.Fatalf("expiryNoticeDetails() = %q", got)
	}
}

type errTestAudit string

func (e errTestAudit) Error() string { return string(e) }
