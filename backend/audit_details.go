package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// auditText renders one audited value. Empty and missing values read "(ว่าง)"
// so a change from nothing is still visible in the log.
func auditText(value any) string {
	if value == nil {
		return "(ว่าง)"
	}
	text := fmt.Sprint(value)
	if text == "" {
		return "(ว่าง)"
	}
	return text
}

// describeDocumentChanges is the Details of a DOCUMENT_UPDATED row: each
// revised field with its old and new value, and whether the people list changed.
func describeDocumentChanges(revisions []DocumentRevision, appointmentsChanged bool) string {
	header := "เจ้าหน้าที่แก้ไขข้อมูลก่อนยืนยัน"
	var parts []string
	for _, revision := range revisions {
		parts = append(parts, fmt.Sprintf("%s: %s → %s", revision.FieldKey, auditText(revision.OldValue), auditText(revision.NewValue)))
	}
	if appointmentsChanged {
		parts = append(parts, "รายชื่อผู้เกี่ยวข้องถูกแก้ไข")
	}
	if len(parts) == 0 {
		return header
	}
	return header + ": " + strings.Join(parts, "; ")
}

// settingsChangeDetails lists the OCR setting keys whose values changed, with
// old and new values. previous is empty on the first save. The stored JSON
// holds no secrets: the API key lives in its own settings row.
func settingsChangeDetails(previous, next string) string {
	before := map[string]any{}
	after := map[string]any{}
	if previous != "" {
		_ = json.Unmarshal([]byte(previous), &before)
	}
	_ = json.Unmarshal([]byte(next), &after)

	keys := make([]string, 0, len(after)+len(before))
	seen := map[string]bool{}
	for key := range before {
		keys = append(keys, key)
		seen[key] = true
	}
	for key := range after {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	var parts []string
	for _, key := range keys {
		old, new := before[key], after[key]
		if fmt.Sprint(old) == fmt.Sprint(new) {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s → %s", key, auditText(old), auditText(new)))
	}
	if len(parts) == 0 {
		return "ไม่มีการเปลี่ยนแปลงค่า"
	}
	return strings.Join(parts, "; ")
}

// syncRunAuditDetails is the Details of a DIRECTORY_SYNCED row.
func syncRunAuditDetails(result UserSyncResult, err error) string {
	if err != nil {
		return "สถานะ: ล้มเหลว; " + err.Error()
	}
	return fmt.Sprintf("สถานะ: สำเร็จ; ดึงมา %d, เพิ่ม %d, แก้ไข %d, ไม่เปลี่ยน %d", result.Fetched, result.Created, result.Updated, result.Unchanged)
}

// expiryNoticeDetails is the Details of an EXPIRY_NOTICE_CREATED row.
func expiryNoticeDetails(noticeType string, dueAt time.Time) string {
	return fmt.Sprintf("แจ้งเตือน %s ครบกำหนด %s", noticeType, dueAt.In(auditLocation).Format("2006-01-02"))
}
