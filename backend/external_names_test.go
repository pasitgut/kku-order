package main

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

func documentWithStatus(t *testing.T, db *gorm.DB, status string) Document {
	t.Helper()
	document, _ := createReviewDocument(t, db, map[string]string{})
	if err := db.Model(&Document{}).Where("id = ?", document.ID).Update("status", status).Error; err != nil {
		t.Fatal(err)
	}
	document.Status = status
	return document
}

func nameRow(t *testing.T, db *gorm.DB, row Appointment) Appointment {
	t.Helper()
	row.BoundingBoxJSON = "null"
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func reloadRow(t *testing.T, db *gorm.DB, id uint) Appointment {
	t.Helper()
	var row Appointment
	if err := db.First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func newDirectoryPerson(t *testing.T, db *gorm.DB, first, last string) DirectoryPerson {
	t.Helper()
	person := DirectoryPerson{FirstName: first, LastName: last, SyncedAt: time.Now()}
	if err := db.Create(&person).Error; err != nil {
		t.Fatal(err)
	}
	return person
}

func TestLinkExternalNamesFillsConfirmedDocuments(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405000000")
	ocrName := "ดร.ภายนอก" + stamp + " ทดสอบ"
	first := documentWithStatus(t, db, "CONFIRMED")
	second := documentWithStatus(t, db, "CONFIRMED")
	rowA := nameRow(t, db, Appointment{DocumentID: first.ID, FullName: ocrName, NameMatchMethod: matchMethodNotFound})
	rowB := nameRow(t, db, Appointment{DocumentID: second.ID, FullName: ocrName, NameMatchMethod: matchMethodNotFound})

	person := newDirectoryPerson(t, db, "ภายนอก"+stamp, "ทดสอบ")
	linked, err := linkExternalNames(db, "sync")
	if err != nil {
		t.Fatal(err)
	}
	if linked < 2 {
		t.Fatalf("linked = %d, want at least the 2 rows of this test", linked)
	}
	for _, row := range []Appointment{rowA, rowB} {
		got := reloadRow(t, db, row.ID)
		if got.DirectoryUserID == nil || *got.DirectoryUserID != person.ID || got.NameMatchMethod != matchMethodExact {
			t.Fatalf("row %d = %+v, want linked to %d by EXACT", row.ID, got, person.ID)
		}
		if got.FullName != ocrName {
			t.Fatalf("the name as written in the order must stay, got %q", got.FullName)
		}
		var revisions []DocumentRevision
		db.Where("document_id = ? AND field_key = ?", row.DocumentID, "appointment_link").Find(&revisions)
		if len(revisions) != 1 || revisions[0].OldValue != ocrName || revisions[0].ChangedBy != "sync" || revisions[0].NewValue == "" {
			t.Fatalf("document %d revisions = %+v", row.DocumentID, revisions)
		}
	}
	var status string
	db.Model(&Document{}).Select("status").Where("id = ?", first.ID).Scan(&status)
	if status != "CONFIRMED" {
		t.Fatalf("a confirmed document stays confirmed, got %s", status)
	}
}

func TestLinkExternalNamesLeavesOtherRowsAlone(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405000000")
	document := documentWithStatus(t, db, "CONFIRMED")
	other := newDirectoryPerson(t, db, "คนเดิม"+stamp, "ผูกไว้แล้ว")
	exactName := "อ.เข้าใหม่" + stamp + " ทดสอบ"

	fuzzy := nameRow(t, db, Appointment{DocumentID: document.ID, FullName: "อ.เข้าใหม่" + stamp + " ทดสอง", NameMatchMethod: matchMethodNotFound})
	linkedElsewhere := nameRow(t, db, Appointment{DocumentID: document.ID, FullName: exactName, DirectoryUserID: &other.ID, NameMatchMethod: matchMethodManual})
	manualCleared := nameRow(t, db, Appointment{DocumentID: document.ID, FullName: exactName, NameMatchMethod: matchMethodManual})
	exOfficio := nameRow(t, db, Appointment{DocumentID: document.ID, Position: "คณบดี", NameMatchMethod: matchMethodExOfficio})

	newDirectoryPerson(t, db, "เข้าใหม่"+stamp, "ทดสอบ")
	if _, err := linkExternalNames(db, "sync"); err != nil {
		t.Fatal(err)
	}
	if got := reloadRow(t, db, fuzzy.ID); got.DirectoryUserID != nil {
		t.Fatalf("a name that is only close must not be linked, got %+v", got)
	}
	if got := reloadRow(t, db, linkedElsewhere.ID); got.DirectoryUserID == nil || *got.DirectoryUserID != other.ID {
		t.Fatalf("a row linked to someone else must keep that person, got %+v", got)
	}
	if got := reloadRow(t, db, manualCleared.ID); got.DirectoryUserID != nil {
		t.Fatalf("a row a reviewer cleared by hand must stay empty, got %+v", got)
	}
	if got := reloadRow(t, db, exOfficio.ID); got.DirectoryUserID != nil || got.NameMatchMethod != matchMethodExOfficio {
		t.Fatalf("an ex-officio row in a confirmed document must not be touched, got %+v", got)
	}
}

func TestLinkExternalNamesSkipsCancelledDocuments(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405000000")
	document := documentWithStatus(t, db, documentCancelled)
	row := nameRow(t, db, Appointment{DocumentID: document.ID, FullName: "อ.ยกเลิก" + stamp + " ทดสอบ", NameMatchMethod: matchMethodNotFound})
	newDirectoryPerson(t, db, "ยกเลิก"+stamp, "ทดสอบ")
	if _, err := linkExternalNames(db, "sync"); err != nil {
		t.Fatal(err)
	}
	if got := reloadRow(t, db, row.ID); got.DirectoryUserID != nil {
		t.Fatalf("a cancelled document must not change, got %+v", got)
	}
}

func TestLinkExternalNamesRetagsRowsReadBeforeTheFirstSync(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405000000")
	document := documentWithStatus(t, db, "REVIEW")
	setFieldConfidence(t, db, document.ID, "order_no", "11/2569", 0.98)
	setFieldConfidence(t, db, document.ID, "issued_date", "2026-10-11", 0.98)
	row := nameRow(t, db, Appointment{DocumentID: document.ID, FullName: "ศ.ดร.ไม่มีใคร" + stamp + " ต่างคณะ", NameMatchMethod: matchMethodEmpty})
	newDirectoryPerson(t, db, "คนอื่น"+stamp, "ในระบบ")

	if _, err := linkExternalNames(db, "sync"); err != nil {
		t.Fatal(err)
	}
	got := reloadRow(t, db, row.ID)
	if got.DirectoryUserID != nil || got.NameMatchMethod == matchMethodEmpty {
		t.Fatalf("a row read before the first sync must get its real match result once the directory has people, got %+v", got)
	}
	if ready, err := readyToAutoConfirm(db, document.ID, 0.9); err != nil || !ready {
		t.Fatalf("the document must no longer wait for a sync, ready = %v, %v", ready, err)
	}
}
