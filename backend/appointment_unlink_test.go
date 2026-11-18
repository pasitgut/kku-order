package main

import (
	"net/http"
	"testing"
	"time"
)

// ผู้ตรวจที่เอาคนออกจากแถวที่มีชื่อต้องเห็นแถวนั้นว่างอยู่ต่อไป เดิมตอนบันทึกระบบจับคู่ชื่อซ้ำอีกรอบ
// ผ่านชื่อแฝง (alias) ที่ได้จากการผูกครั้งแรกหรือชื่อที่ตรงกันทุกตัวอักษร การเอาคนออกจึงไม่เคยมีผล
func TestReviewerCanUnlinkANamedRow(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405000000")
	person := newDirectoryPerson(t, db, "ถอดออก"+stamp, "ทดสอบ")
	aliasName := "ดร.ชื่อเรียน" + stamp + " ถอดออก"
	if err := db.Create(&PersonNameAlias{NormalizedName: normalizePersonName(aliasName), DirectoryPersonID: person.ID, SourceText: aliasName, CreatedBy: "test"}).Error; err != nil {
		t.Fatal(err)
	}
	exactName := "ถอดออก" + stamp + " ทดสอบ"
	for _, tc := range []struct {
		name, fullName, method, renamed string
	}{
		{"learned alias", aliasName, matchMethodAlias, exactName},
		{"exact name", exactName, matchMethodExact, aliasName},
	} {
		document := documentWithStatus(t, db, "NEEDS_REVIEW")
		row := nameRow(t, db, Appointment{DocumentID: document.ID, FullName: tc.fullName, DirectoryUserID: &person.ID, NameMatchMethod: tc.method, NameMatchScore: 1})
		for save := 1; save <= 2; save++ {
			response := patchDocument(t, db, document.ID, map[string]any{"appointments": []map[string]any{{"id": row.ID, "fullName": tc.fullName, "directoryUserId": nil, "manualMatch": false, "boundingBox": "null"}}})
			if response.Code != http.StatusOK {
				t.Fatalf("%s save %d = %d %s", tc.name, save, response.Code, response.Body.String())
			}
			row = rowOf(t, db, document.ID)
			if row.DirectoryUserID != nil || row.NameMatchMethod != matchMethodManual {
				t.Fatalf("%s save %d: row = user %v method %q, want unlinked MANUAL", tc.name, save, row.DirectoryUserID, row.NameMatchMethod)
			}
		}
		// การเปลี่ยนชื่อในแถวถือเป็นเรื่องใหม่ จึงจับคู่ชื่อใหม่อีกครั้ง
		response := patchDocument(t, db, document.ID, map[string]any{"appointments": []map[string]any{{"id": row.ID, "fullName": tc.renamed, "directoryUserId": nil, "boundingBox": "null"}}})
		if response.Code != http.StatusOK {
			t.Fatalf("%s rename = %d %s", tc.name, response.Code, response.Body.String())
		}
		row = rowOf(t, db, document.ID)
		if row.DirectoryUserID == nil || *row.DirectoryUserID != person.ID {
			t.Fatalf("%s: a renamed row should be matched again, got %+v", tc.name, row)
		}
	}
}

// การยกเลิกการผูกจะลืมคนนั้นเฉพาะในเอกสารนี้ ชื่อแฝงที่ได้จากการผูกครั้งแรกยังอยู่
// ชื่อเดียวกันในเอกสารอื่นจึงยังผูกได้ตามเดิม
func TestUnlinkingKeepsTheLearnedAliasForOtherDocuments(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405000000")
	person := newDirectoryPerson(t, db, "จำไว้"+stamp, "ทดสอบ")
	aliasName := "ดร.ชื่อเรียน" + stamp + " จำไว้"
	if err := db.Create(&PersonNameAlias{NormalizedName: normalizePersonName(aliasName), DirectoryPersonID: person.ID, SourceText: aliasName, CreatedBy: "test"}).Error; err != nil {
		t.Fatal(err)
	}
	unlinked := documentWithStatus(t, db, "NEEDS_REVIEW")
	row := nameRow(t, db, Appointment{DocumentID: unlinked.ID, FullName: aliasName, DirectoryUserID: &person.ID, NameMatchMethod: matchMethodAlias, NameMatchScore: 1})
	if response := patchDocument(t, db, unlinked.ID, map[string]any{"appointments": []map[string]any{{"id": row.ID, "fullName": aliasName, "directoryUserId": nil, "boundingBox": "null"}}}); response.Code != http.StatusOK {
		t.Fatalf("unlink = %d %s", response.Code, response.Body.String())
	}
	if row = rowOf(t, db, unlinked.ID); row.DirectoryUserID != nil {
		t.Fatalf("unlinked row = %+v, want no person", row)
	}
	var aliases int64
	db.Model(&PersonNameAlias{}).Where("normalized_name = ? AND directory_person_id = ?", normalizePersonName(aliasName), person.ID).Count(&aliases)
	if aliases != 1 {
		t.Fatalf("aliases after unlink = %d, want the learned alias kept", aliases)
	}

	other := documentWithStatus(t, db, "NEEDS_REVIEW")
	otherRow := nameRow(t, db, Appointment{DocumentID: other.ID, FullName: aliasName})
	if response := patchDocument(t, db, other.ID, map[string]any{"appointments": []map[string]any{{"id": otherRow.ID, "fullName": aliasName, "directoryUserId": nil, "boundingBox": "null"}}}); response.Code != http.StatusOK {
		t.Fatalf("other save = %d %s", response.Code, response.Body.String())
	}
	otherRow = rowOf(t, db, other.ID)
	if otherRow.DirectoryUserID == nil || *otherRow.DirectoryUserID != person.ID || otherRow.NameMatchMethod != matchMethodAlias {
		t.Fatalf("other document row = user %v method %q, want linked to %d by ALIAS", otherRow.DirectoryUserID, otherRow.NameMatchMethod, person.ID)
	}
}
