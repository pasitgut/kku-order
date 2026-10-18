package main

import (
	"encoding/json"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestPositionKeyIgnoresTheCollegeNameAndSpacing(t *testing.T) {
	for _, in := range []string{"คณบดีวิทยาลัยการคอมพิวเตอร์", "คณบดี", " คณบดี  วิทยาลัยการคอมพิวเตอร์ มหาวิทยาลัยขอนแก่น"} {
		if got := positionKey(in); got != "คณบดี" {
			t.Errorf("positionKey(%q) = %q, want คณบดี", in, got)
		}
	}
	if positionKey("รองคณบดีฝ่ายวิชาการ") == positionKey("ผู้ช่วยคณบดีฝ่ายวิชาการ") {
		t.Fatal("deputy and assistant dean must stay different positions")
	}
}

func TestExOfficioPositionLosesTheTrailingRoleWords(t *testing.T) {
	position, role := splitExOfficioPosition("หัวหน้าศูนย์ความเป็นเลิศด้านการวิจัยปัญญาประดิษฐ์ เป็น")
	if position != "หัวหน้าศูนย์ความเป็นเลิศด้านการวิจัยปัญญาประดิษฐ์" || role != "" {
		t.Fatalf("got %q / %q", position, role)
	}
	position, role = splitExOfficioPosition("คณบดีวิทยาลัยการคอมพิวเตอร์ เป็นประธานกรรมการ")
	if position != "คณบดีวิทยาลัยการคอมพิวเตอร์" || role != "ประธานกรรมการ" {
		t.Fatalf("got %q / %q", position, role)
	}
	position, _ = splitExOfficioPosition("หัวหน้าศูนย์ความเป็นเลิศด้านการวิจัย")
	if position != "หัวหน้าศูนย์ความเป็นเลิศด้านการวิจัย" {
		t.Fatalf("a 'เป็น' inside a word must stay, got %q", position)
	}
}

func day(value string) *time.Time {
	parsed, _ := time.ParseInLocation("2006-01-02", value, time.Local)
	return &parsed
}

func TestPickHolderTrustsStaffDatesThenTheCurrentHolderThenWhatReviewersPicked(t *testing.T) {
	manual := PositionHolder{ID: 1, DirectoryPersonID: 10, StartsOn: day("2022-10-01"), Source: positionSourceManual}
	learned := PositionHolder{ID: 2, DirectoryPersonID: 20, StartsOn: day("2025-10-01"), Source: positionSourceLearned}
	synced := PositionHolder{ID: 3, DirectoryPersonID: 30, Source: positionSourceSync}
	staffNow := PositionHolder{ID: 4, DirectoryPersonID: 40, Source: positionSourceManual}
	for _, tc := range []struct {
		name    string
		holders []PositionHolder
		on      string
		want    uint
	}{
		{"staff dates cover the order", []PositionHolder{manual, learned, synced}, "2024-03-01", 10},
		{"a staff date holds until staff enter the next one", []PositionHolder{manual, learned, synced}, "2026-01-15", 10},
		{"before any staff date the current holder is used", []PositionHolder{manual, learned, synced}, "2020-01-01", 30},
		{"one old reviewer pick does not override sync", []PositionHolder{learned, synced}, "2026-01-15", 30},
		{"staff entry without a date overrides sync", []PositionHolder{synced, staffNow}, "2026-01-15", 40},
		{"reviewer picks fill positions sync does not know", []PositionHolder{learned}, "2026-01-15", 20},
		{"nobody known for that date", []PositionHolder{learned}, "2020-01-01", 0},
	} {
		got := pickHolder(tc.holders, day(tc.on))
		if (tc.want == 0 && got != nil) || (tc.want != 0 && (got == nil || got.DirectoryPersonID != tc.want)) {
			t.Errorf("%s: got %+v, want person %d", tc.name, got, tc.want)
		}
	}
}

func TestLinkByPositionFillsExOfficioRowsOnly(t *testing.T) {
	holders := map[string][]PositionHolder{"คณบดี": {{DirectoryPersonID: 10, Source: positionSourceSync}}}
	rows := []Appointment{
		{FullName: "", Position: "คณบดีวิทยาลัยการคอมพิวเตอร์"},
		{FullName: "", Position: "ผู้ช่วยคณบดีฝ่ายวิชาการ"},
		{FullName: "สมชาย ใจดี", Position: "คณบดี"},
	}
	if linked := linkByPosition(rows, holders, nil); linked != 1 {
		t.Fatalf("linked = %d", linked)
	}
	if rows[0].DirectoryUserID == nil || *rows[0].DirectoryUserID != 10 || rows[0].NameMatchMethod != matchMethodPosition || rows[0].FullName != "" {
		t.Fatalf("dean row = %+v", rows[0])
	}
	if rows[1].DirectoryUserID != nil || rows[2].DirectoryUserID != nil {
		t.Fatalf("only the vacant ex-officio row should be linked: %+v / %+v", rows[1], rows[2])
	}
}

func TestBuildExtractionRecordsMovesTrailingRoleOutOfAnExOfficioPosition(t *testing.T) {
	r := sampleResult()
	r.Groups[0].Members[0] = ExtractedMember{Position: ExtractedValue{Text: "คณบดีวิทยาลัยการคอมพิวเตอร์ เป็นประธานกรรมการ"}, ExOfficio: true}
	got := buildExtractionRecords(r, 0.8).Appointments[0]
	if got.Position != "คณบดีวิทยาลัยการคอมพิวเตอร์" || got.CommitteeRole != "ประธานกรรมการ" {
		t.Fatalf("appointment = %q / %q", got.Position, got.CommitteeRole)
	}
}

func TestReviewerChoiceForAPositionIsRememberedForTheNextOrder(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405.000000")
	person := DirectoryPerson{FirstName: "ผู้ดำรง" + stamp, LastName: "ทดสอบ", Status: "A", SyncedAt: time.Now()}
	if err := db.Create(&person).Error; err != nil {
		t.Fatal(err)
	}
	title := "ผู้ช่วยคณบดีฝ่ายทดสอบ" + stamp
	first, _ := createReviewDocument(t, db, map[string]string{})
	db.Model(&Document{}).Where("id = ?", first.ID).Update("issued_date", day("2026-10-01"))
	row := Appointment{DocumentID: first.ID, Position: title, NameMatchMethod: matchMethodExOfficio, BoundingBoxJSON: "null"}
	db.Create(&row)
	response := patchDocument(t, db, first.ID, map[string]any{"appointments": []map[string]any{{"id": row.ID, "directoryUserId": person.ID, "fullName": "", "position": title, "boundingBox": "null"}}})
	if response.Code != 200 {
		t.Fatalf("save = %d %s", response.Code, response.Body.String())
	}

	second, _ := createReviewDocument(t, db, map[string]string{})
	db.Model(&Document{}).Where("id = ?", second.ID).Update("issued_date", day("2026-11-01"))
	next := Appointment{DocumentID: second.ID, Position: title + "วิทยาลัยการคอมพิวเตอร์", NameMatchMethod: matchMethodExOfficio, BoundingBoxJSON: "null"}
	db.Create(&next)
	if err := refreshPendingMatches(db); err != nil {
		t.Fatal(err)
	}
	db.First(&next, next.ID)
	if next.DirectoryUserID == nil || *next.DirectoryUserID != person.ID || next.NameMatchMethod != matchMethodPosition {
		t.Fatalf("next order row = %+v", next)
	}
}

func TestCommitteeRolesAreNeverFilledByPosition(t *testing.T) {
	for _, title := range []string{"ประธานกรรมการ", "รองประธานกรรมการ", "กรรมการ", "กรรมการและเลขานุการ", "เลขานุการ", "ผู้ช่วยเลขานุการ", "ที่ปรึกษา"} {
		if !committeeRoleOnly(title) {
			t.Errorf("%q is a committee role", title)
		}
	}
	for _, title := range []string{"คณบดีวิทยาลัยการคอมพิวเตอร์", "ประธานหลักสูตรวิทยาการคอมพิวเตอร์", "หัวหน้าสาขาวิชาปัญญาประดิษฐ์", "เลขานุการวิทยาลัยการคอมพิวเตอร์"} {
		if committeeRoleOnly(title) {
			t.Errorf("%q is an organisation position", title)
		}
	}
	holders := map[string][]PositionHolder{"ประธานกรรมการ": {{DirectoryPersonID: 10, Source: positionSourceLearned}}}
	rows := []Appointment{{Position: "ประธานกรรมการ"}}
	if linkByPosition(rows, holders, nil) != 0 || rows[0].DirectoryUserID != nil {
		t.Fatalf("a committee role must be picked by a person, got %+v", rows[0])
	}
}

func pendingExOfficioRow(t *testing.T, db *gorm.DB, title, issued string) Appointment {
	t.Helper()
	document, _ := createReviewDocument(t, db, map[string]string{})
	db.Model(&Document{}).Where("id = ?", document.ID).Update("issued_date", day(issued))
	row := Appointment{DocumentID: document.ID, Position: title, NameMatchMethod: matchMethodExOfficio, BoundingBoxJSON: "null"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

// rowOf อ่านแถวรายชื่อแถวเดียวของเอกสาร (บันทึกเอกสารสร้างแถวใหม่ทุกครั้ง id จึงเปลี่ยน)
func rowOf(t *testing.T, db *gorm.DB, documentID uint) Appointment {
	t.Helper()
	var row Appointment
	if err := db.Where("document_id = ?", documentID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func TestVacantPositionsCountPendingDocumentsPerPosition(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405.000000")
	title := "หัวหน้าศูนย์ทดสอบว่าง" + stamp
	pendingExOfficioRow(t, db, title, "2026-10-01")
	pendingExOfficioRow(t, db, title+"วิทยาลัยการคอมพิวเตอร์ เป็นประธานกรรมการ", "2026-10-02")
	confirmed := pendingExOfficioRow(t, db, title, "2026-10-03")
	db.Model(&Document{}).Where("id = ?", confirmed.DocumentID).Update("status", "CONFIRMED")
	pendingExOfficioRow(t, db, "ประธานกรรมการ", "2026-10-04")

	vacant, err := vacantPositions(db)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, item := range vacant {
		if item.PositionTitle == "ประธานกรรมการ" {
			t.Fatal("committee roles are not vacancies to fill")
		}
		if positionKey(item.PositionTitle) == positionKey(title) {
			found++
			if item.Documents != 2 || item.PositionTitle != title {
				t.Fatalf("vacancy = %+v", item)
			}
		}
	}
	if found != 1 {
		t.Fatalf("vacancy for %s listed %d times: %+v", title, found, vacant)
	}
}

func TestSettingAPositionHolderLinksRowsWaitingForConfirmation(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405.000000")
	person := DirectoryPerson{FirstName: "ผู้ดำรงใหม่" + stamp, LastName: "ทดสอบ", Status: "A", SyncedAt: time.Now()}
	if err := db.Create(&person).Error; err != nil {
		t.Fatal(err)
	}
	title := "ผู้ช่วยคณบดีฝ่ายกำหนดเอง" + stamp
	row := pendingExOfficioRow(t, db, title+"วิทยาลัยการคอมพิวเตอร์", "2026-10-05")
	before := pendingExOfficioRow(t, db, title, "2025-01-01")

	const path = "/api/v1/position-holders"
	for _, bad := range []struct {
		body map[string]any
		code int
	}{
		{map[string]any{"positionTitle": "ประธานกรรมการ", "directoryUserId": person.ID}, 400},
		{map[string]any{"positionTitle": "", "directoryUserId": person.ID}, 400},
		{map[string]any{"positionTitle": title, "directoryUserId": person.ID, "startsOn": "ยังไม่รู้"}, 400},
		{map[string]any{"positionTitle": title, "directoryUserId": 999999999}, 404},
	} {
		if response := postJSON(t, setPositionHolder(db), path, path, bad.body); response.Code != bad.code {
			t.Fatalf("%v = %d %s, want %d", bad.body, response.Code, response.Body.String(), bad.code)
		}
	}

	response := postJSON(t, setPositionHolder(db), path, path, map[string]any{"positionTitle": title, "directoryUserId": person.ID, "startsOn": "1 ต.ค. 2568"})
	if response.Code != 201 {
		t.Fatalf("save = %d %s", response.Code, response.Body.String())
	}
	var result struct {
		Linked int `json:"linked"`
	}
	json.Unmarshal(response.Body.Bytes(), &result)
	if result.Linked != 1 {
		t.Fatalf("linked = %d, want 1 (body %s)", result.Linked, response.Body.String())
	}
	db.First(&row, row.ID)
	if row.DirectoryUserID == nil || *row.DirectoryUserID != person.ID || row.NameMatchMethod != matchMethodPosition {
		t.Fatalf("row after save = %+v", row)
	}
	db.First(&before, before.ID)
	if before.DirectoryUserID != nil {
		t.Fatalf("an order issued before the holder started must stay open, got %+v", before)
	}
	var saved PositionHolder
	db.Where("position_key = ? AND directory_person_id = ?", positionKey(title), person.ID).Take(&saved)
	if saved.Source != positionSourceManual || saved.StartsOn == nil || saved.StartsOn.Format("2006-01-02") != "2025-10-01" {
		t.Fatalf("holder = %+v", saved)
	}
}

func TestAddingATeacherWithAnExecutivePositionRecordsTheHolder(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405.000000")
	title := "หัวหน้างานทดสอบบุคคลใหม่" + stamp
	row := pendingExOfficioRow(t, db, title, "2026-10-06")
	const path = "/api/v1/directory-users"

	response := postJSON(t, createDirectoryUser(db), path, path, map[string]any{"firstName": "ใหม่" + stamp, "lastName": "มีตำแหน่ง", "managePosition": title, "managePositionSince": "ทุกวัน"})
	if response.Code != 400 {
		t.Fatalf("an unreadable start date must be rejected before anyone is created, got %d %s", response.Code, response.Body.String())
	}
	response = postJSON(t, createDirectoryUser(db), path, path, map[string]any{"firstName": "ใหม่" + stamp, "lastName": "มีตำแหน่ง", "managePosition": title, "managePositionSince": "2026-10-01"})
	if response.Code != 201 {
		t.Fatalf("create = %d %s", response.Code, response.Body.String())
	}
	var created struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(response.Body.Bytes(), &created)
	db.First(&row, row.ID)
	if row.DirectoryUserID == nil || *row.DirectoryUserID != created.Data.ID || row.NameMatchMethod != matchMethodPosition {
		t.Fatalf("row after adding the holder = %+v (new person %d)", row, created.Data.ID)
	}
}

func TestCorrectingAHolderRelinksRowsTheSystemLinked(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405.000000")
	wrong := DirectoryPerson{FirstName: "ผิดคน" + stamp, LastName: "ทดสอบ", Status: "A", SyncedAt: time.Now()}
	right := DirectoryPerson{FirstName: "ถูกคน" + stamp, LastName: "ทดสอบ", Status: "A", SyncedAt: time.Now()}
	db.Create(&wrong)
	db.Create(&right)
	title := "รองคณบดีฝ่ายแก้ไข" + stamp
	row := pendingExOfficioRow(t, db, title, "2026-10-05")
	const path = "/api/v1/position-holders"
	for _, person := range []DirectoryPerson{wrong, right} {
		response := postJSON(t, setPositionHolder(db), path, path, map[string]any{"positionTitle": title, "directoryUserId": person.ID, "startsOn": "1 ต.ค. 2568"})
		if response.Code != 201 {
			t.Fatalf("save = %d %s", response.Code, response.Body.String())
		}
	}
	db.First(&row, row.ID)
	if row.DirectoryUserID == nil || *row.DirectoryUserID != right.ID || row.NameMatchMethod != matchMethodPosition {
		t.Fatalf("row after correction = %+v, want person %d", row, right.ID)
	}
}

func TestReviewerPicksOnAnExOfficioRowStayByPosition(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405.000000")
	first := DirectoryPerson{FirstName: "เลือกแรก" + stamp, LastName: "ทดสอบ", Status: "A", SyncedAt: time.Now()}
	second := DirectoryPerson{FirstName: "เลือกสอง" + stamp, LastName: "ทดสอบ", Status: "A", SyncedAt: time.Now()}
	db.Create(&first)
	db.Create(&second)
	title := "หัวหน้างานเลือกซ้ำ" + stamp
	row := pendingExOfficioRow(t, db, title, "2026-10-01")
	pick := func(personID uint) {
		t.Helper()
		row = rowOf(t, db, row.DocumentID)
		response := patchDocument(t, db, row.DocumentID, map[string]any{"appointments": []map[string]any{{"id": row.ID, "directoryUserId": personID, "fullName": row.FullName, "position": title, "boundingBox": "null"}}})
		if response.Code != 200 {
			t.Fatalf("save = %d %s", response.Code, response.Body.String())
		}
	}
	pick(first.ID)
	pick(first.ID)
	pick(second.ID)
	row = rowOf(t, db, row.DocumentID)
	if row.FullName != "" || row.DirectoryUserID == nil || *row.DirectoryUserID != second.ID {
		t.Fatalf("row = %+v, want an ex-officio row linked to the second pick", row)
	}
	var aliases int64
	db.Model(&PersonNameAlias{}).Where("directory_person_id = ?", second.ID).Count(&aliases)
	if aliases != 0 {
		t.Fatalf("picking a holder must not teach a name alias, got %d", aliases)
	}
	holders, _ := loadPositionHolders(db)
	if got := pickHolder(holders[positionKey(title)], day("2026-10-01")); got == nil || got.DirectoryPersonID != second.ID {
		t.Fatalf("learned holder = %+v, want the latest pick", got)
	}
}

func TestReviewerCanLeaveAnExOfficioRowEmpty(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405.000000")
	person := DirectoryPerson{FirstName: "รักษาการ" + stamp, LastName: "ทดสอบ", Status: "A", SyncedAt: time.Now()}
	db.Create(&person)
	title := "ผู้อำนวยการทดสอบว่าง" + stamp
	const path = "/api/v1/position-holders"
	row := pendingExOfficioRow(t, db, title, "2026-10-05")
	if response := postJSON(t, setPositionHolder(db), path, path, map[string]any{"positionTitle": title, "directoryUserId": person.ID}); response.Code != 201 {
		t.Fatalf("save holder = %d %s", response.Code, response.Body.String())
	}
	response := patchDocument(t, db, row.DocumentID, map[string]any{"appointments": []map[string]any{{"id": row.ID, "directoryUserId": nil, "fullName": "", "position": title, "boundingBox": "null"}}})
	if response.Code != 200 {
		t.Fatalf("clear = %d %s", response.Code, response.Body.String())
	}
	if err := refreshPendingMatches(db); err != nil {
		t.Fatal(err)
	}
	row = rowOf(t, db, row.DocumentID)
	if row.DirectoryUserID != nil {
		t.Fatalf("the system put the holder back after the reviewer removed them: %+v", row)
	}
}
