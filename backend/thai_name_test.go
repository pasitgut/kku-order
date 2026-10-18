package main

import (
	"testing"
	"time"
)

func TestSplitThaiHonorificSeparatesRankFromName(t *testing.T) {
	for _, tc := range []struct{ in, rank, name string }{
		{"ผู้ช่วยศาสตราจารย์ภาณุพงษ์ วันจันทึก", "ผู้ช่วยศาสตราจารย์", "ภาณุพงษ์ วันจันทึก"},
		{"รองศาสตราจารย์ ดร.สมชาย ใจดี", "รองศาสตราจารย์", "ดร.สมชาย ใจดี"},
		{"ผศ.ดร. สมหญิง รักเรียน", "ผู้ช่วยศาสตราจารย์", "ดร.สมหญิง รักเรียน"},
		{"รศ. ดร.วิชัย มั่นคง", "รองศาสตราจารย์", "ดร.วิชัย มั่นคง"},
		{"ศ.ดร.ปรีชา ฉลาด", "ศาสตราจารย์", "ดร.ปรีชา ฉลาด"},
		{"อ. ดร.กมล ศรีสุข", "อาจารย์", "ดร.กมล ศรีสุข"},
		{"อาจารย์วิไล ดี", "อาจารย์", "วิไล ดี"},
		{"นางสาวจิราภรณ์ หอมอ่อน", "", "จิราภรณ์ หอมอ่อน"},
		{"น.ส. จิราภรณ์ หอมอ่อน", "", "จิราภรณ์ หอมอ่อน"},
		{"นายชานนท์ เดชสุภา", "", "ชานนท์ เดชสุภา"},
		{"นางมาลี สุขใจ", "", "มาลี สุขใจ"},
		{"ชานนท์ เดชสุภา", "", "ชานนท์ เดชสุภา"},
		{"", "", ""},
	} {
		rank, name := splitThaiHonorific(tc.in)
		if rank != tc.rank || name != tc.name {
			t.Errorf("splitThaiHonorific(%q) = %q, %q; want %q, %q", tc.in, rank, name, tc.rank, tc.name)
		}
	}
}

func TestBuildExtractionRecordsMovesRankOutOfTheName(t *testing.T) {
	r := sampleResult()
	r.Groups[0].Members[0].Prefix = ExtractedValue{Text: "ผู้ช่วยศาสตราจารย์"}
	r.Groups[0].Members[0].Name = ExtractedValue{Text: "ภาณุพงษ์ วันจันทึก"}
	r.Groups[0].Members[0].Position = ExtractedValue{}
	rec := buildExtractionRecords(r, 0.8)
	got := rec.Appointments[0]
	if got.FullName != "ภาณุพงษ์ วันจันทึก" || got.Position != "ผู้ช่วยศาสตราจารย์" {
		t.Fatalf("appointment = %q / %q", got.FullName, got.Position)
	}
}

func TestTidyAppointmentNamesFixesRowsAlreadyImported(t *testing.T) {
	db := testDatabase(t)
	document, _ := createReviewDocument(t, db, map[string]string{})
	glued := Appointment{DocumentID: document.ID, FullName: "ผู้ช่วยศาสตราจารย์ภาณุพงษ์ วันจันทึก", BoundingBoxJSON: "null"}
	clean := Appointment{DocumentID: document.ID, FullName: "ชานนท์ เดชสุภา", Position: "รองศาสตราจารย์", BoundingBoxJSON: "null"}
	for _, row := range []*Appointment{&glued, &clean} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tidyAppointmentNames(db); err != nil {
		t.Fatal(err)
	}
	db.First(&glued, glued.ID)
	db.First(&clean, clean.ID)
	if glued.FullName != "ภาณุพงษ์ วันจันทึก" || glued.Position != "ผู้ช่วยศาสตราจารย์" {
		t.Fatalf("glued row = %q / %q", glued.FullName, glued.Position)
	}
	if clean.FullName != "ชานนท์ เดชสุภา" || clean.Position != "รองศาสตราจารย์" {
		t.Fatalf("clean row changed: %q / %q", clean.FullName, clean.Position)
	}
}

func TestMatchingStillIgnoresEveryTitle(t *testing.T) {
	if got := stripThaiNamePrefixes("ผศ.ดร.สมชาย ใจดี"); got != "สมชาย ใจดี" {
		t.Fatalf("stripThaiNamePrefixes = %q", got)
	}
}

func TestRankStaysWithTheNameWhenOCRFoundAPosition(t *testing.T) {
	r := sampleResult()
	r.Groups[0].Members[0].Prefix = ExtractedValue{Text: "ผศ.ดร."}
	r.Groups[0].Members[0].Name = ExtractedValue{Text: "กมล ศรีสุข"}
	r.Groups[0].Members[0].Position = ExtractedValue{Text: "หัวหน้าภาควิชา"}
	got := buildExtractionRecords(r, 0.8).Appointments[0]
	if got.FullName != "ผศ.ดร.กมล ศรีสุข" || got.Position != "หัวหน้าภาควิชา" {
		t.Fatalf("appointment = %q / %q", got.FullName, got.Position)
	}
}

func TestTidyLeavesConfirmedDocumentsAlone(t *testing.T) {
	db := testDatabase(t)
	document, _ := createReviewDocument(t, db, map[string]string{})
	if err := db.Model(&Document{}).Where("id = ?", document.ID).Update("status", "CONFIRMED").Error; err != nil {
		t.Fatal(err)
	}
	row := Appointment{DocumentID: document.ID, FullName: "ผู้ช่วยศาสตราจารย์ภาณุพงษ์ วันจันทึก", BoundingBoxJSON: "null"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := tidyAppointmentNames(db); err != nil {
		t.Fatal(err)
	}
	db.First(&row, row.ID)
	if row.FullName != "ผู้ช่วยศาสตราจารย์ภาณุพงษ์ วันจันทึก" {
		t.Fatalf("confirmed row changed to %q", row.FullName)
	}
}

func TestRefreshPendingMatchesLinksExactNamesAndUnlinksGuesses(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405.000000")
	person := DirectoryPerson{FirstName: "ทดสอบ" + stamp, LastName: "ตรงเป๊ะ", Status: "A", SyncedAt: time.Now()}
	if err := db.Create(&person).Error; err != nil {
		t.Fatal(err)
	}
	document, _ := createReviewDocument(t, db, map[string]string{})
	exact := Appointment{DocumentID: document.ID, FullName: person.FirstName + " " + person.LastName, NameMatchMethod: matchMethodNotFound, BoundingBoxJSON: "null"}
	guess := Appointment{DocumentID: document.ID, FullName: "ทดสอบ" + stamp + " ตรงเป", DirectoryUserID: &person.ID, NameMatchMethod: matchMethodFuzzy, BoundingBoxJSON: "null"}
	for _, row := range []*Appointment{&exact, &guess} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := refreshPendingMatches(db); err != nil {
		t.Fatal(err)
	}
	db.First(&exact, exact.ID)
	db.First(&guess, guess.ID)
	if exact.DirectoryUserID == nil || *exact.DirectoryUserID != person.ID || exact.NameMatchMethod != matchMethodExact {
		t.Fatalf("exact row = %+v", exact)
	}
	if guess.DirectoryUserID != nil {
		t.Fatalf("a near-miss guess should wait for the reviewer, got %+v", guess)
	}
}
