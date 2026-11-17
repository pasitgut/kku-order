package main

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestConfirmationNamesSurviveRenameAndConfirmAgain(t *testing.T) {
	db := testDatabase(t)
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	firstName := "ทดสอบ" + stamp
	oldName := firstName + " ใจดี"
	newName := firstName + " มีสุข"
	sync := &UserSyncService{db: db}
	user := map[string]any{"user_id": stamp, "user_fname": firstName, "user_lname": "ใจดี", "is_active": "A"}
	if _, _, err := sync.upsertUser(user); err != nil {
		t.Fatal(err)
	}
	var person DirectoryPerson
	if err := db.Where("external_id = ?", stamp).First(&person).Error; err != nil {
		t.Fatal(err)
	}
	doc, _ := createReviewDocument(t, db, nil)
	confirm := func(document Document, name string, manual bool) {
		t.Helper()
		appointment := appointmentRequest{FullName: name, ManualMatch: manual}
		if manual {
			appointment.DirectoryUserID = &person.ID
		}
		result := patchDocument(t, db, document.ID, map[string]any{
			"confirm": true, "expectedUpdatedAt": document.UpdatedAt.Truncate(time.Millisecond),
			"appointments": []appointmentRequest{appointment},
		})
		if result.Code != http.StatusOK {
			t.Fatalf("confirm = %d %s", result.Code, result.Body.String())
		}
	}
	confirm(doc, oldName, false)
	if response := patchDocument(t, db, doc.ID, map[string]any{"reviewNotes": "stale", "expectedUpdatedAt": doc.UpdatedAt.Truncate(time.Millisecond)}); response.Code != http.StatusConflict {
		t.Fatalf("stale save = %d %s", response.Code, response.Body.String())
	}
	user["user_lname"] = "มีสุข"
	if _, _, err := sync.upsertUser(user); err != nil {
		t.Fatal(err)
	}
	users, aliases, err := loadDirectory(db)
	if err != nil {
		t.Fatal(err)
	}
	if match := matchDirectoryUser(oldName, users, aliases); !match.Matched || match.User.ID != person.ID {
		t.Fatalf("previous-name match = %+v", match)
	}
	var search struct{ Data searchSuggestions }
	getJSON(t, "/api/v1/search/suggest", "/api/v1/search/suggest?q="+url.QueryEscape(oldName), searchSuggestHandler(db), &search)
	if len(search.Data.People) != 1 || search.Data.People[0].ID != person.ID {
		t.Fatalf("previous-name search = %+v", search.Data.People)
	}
	report, err := buildAppointmentReport(db, reportFilter{PersonID: person.ID}, time.Now())
	if err != nil || len(report.Rows) != 1 || report.Rows[0].Name != oldName {
		t.Fatalf("report after rename = %+v, %v", report.Rows, err)
	}
	if result := returnRequest(t, db, doc.ID, "ตรวจชื่ออีกครั้ง"); result.Code != http.StatusOK {
		t.Fatalf("return = %d %s", result.Code, result.Body.String())
	}
	if err := db.First(&doc, doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	confirm(doc, newName, false)
	var rounds []DocumentConfirmation
	if err := db.Where("document_id = ?", doc.ID).Order("id").Find(&rounds).Error; err != nil {
		t.Fatal(err)
	}
	if len(rounds) != 2 || len(rounds[0].Names) != 1 || len(rounds[1].Names) != 1 ||
		rounds[0].Names[0].displayName() != oldName || rounds[0].Names[0].LastName != "ใจดี" ||
		rounds[1].Names[0].displayName() != newName || rounds[1].Names[0].LastName != "มีสุข" {
		t.Fatalf("confirmation rounds = %+v", rounds)
	}
	if err := db.Create(&DirectoryPerson{FirstName: firstName, LastName: "ใจดี", Status: "A", SyncedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	otherDoc, _ := createReviewDocument(t, db, nil)
	result := patchDocument(t, db, otherDoc.ID, map[string]any{
		"confirm": true, "appointments": []appointmentRequest{{FullName: oldName}},
	})
	if result.Code != http.StatusConflict {
		t.Fatalf("ambiguous confirm = %d %s", result.Code, result.Body.String())
	}
	if err := db.Create(&Appointment{DocumentID: otherDoc.ID, DirectoryUserID: &person.ID, FullName: oldName, NameMatchMethod: matchMethodAlias, BoundingBoxJSON: "null"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&otherDoc, otherDoc.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := confirmNow(db, otherDoc, autoConfirmActor, "test"); !errors.Is(err, errAmbiguousConfirmationName) {
		t.Fatalf("automatic confirm with an ambiguous old link = %v", err)
	}
	confirm(otherDoc, oldName, true)
}
