package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestPositionRank(t *testing.T) {
	cases := map[string]int{
		"คณบดี": 0,
		"คณบดีวิทยาลัยการคอมพิวเตอร์":        0,
		"รองคณบดีฝ่ายวิชาการ":                1,
		"ผู้ช่วยคณบดีฝ่ายวิจัย":              2,
		"หัวหน้าสาขาวิชาวิทยาการคอมพิวเตอร์": 3,
		"เลขานุการวิทยาลัยการคอมพิวเตอร์":    4,
		// รักษาการแทนอยู่ลำดับเดียวกับตำแหน่งที่รักษาการ
		"รักษาการแทนคณบดี":             0,
		"รักษาราชการแทนคณบดี":          0,
		"รักษาการแทนรองคณบดีฝ่ายวิจัย": 1,
	}
	for title, want := range cases {
		if got := positionRank(title); got != want {
			t.Errorf("positionRank(%q) = %d, want %d", title, got, want)
		}
	}
}

func TestSortCurrentHolders(t *testing.T) {
	items := []CurrentPositionHolder{
		{PositionTitle: "เลขานุการวิทยาลัย"},
		{PositionTitle: "หัวหน้าสาขา"},
		{PositionTitle: "รองคณบดีฝ่ายวิจัย"},
		{PositionTitle: "รองคณบดีฝ่ายวิชาการ"},
		{PositionTitle: "คณบดี"},
	}
	sortCurrentHolders(items)
	want := []string{"คณบดี", "รองคณบดีฝ่ายวิจัย", "รองคณบดีฝ่ายวิชาการ", "หัวหน้าสาขา", "เลขานุการวิทยาลัย"}
	for i, title := range want {
		if items[i].PositionTitle != title {
			t.Fatalf("order[%d] = %q, want %q (all: %+v)", i, items[i].PositionTitle, title, items)
		}
	}
}

func testPerson(t *testing.T, db *gorm.DB, prefix, first, status string) DirectoryPerson {
	t.Helper()
	person := DirectoryPerson{Prefix: prefix, FirstName: first, LastName: "ทดสอบตำแหน่ง", Status: status, SyncedAt: time.Now()}
	if err := db.Create(&person).Error; err != nil {
		t.Fatal(err)
	}
	return person
}

func testHolder(t *testing.T, db *gorm.DB, title string, personID uint, source string, startsOn, endsOn *time.Time) {
	t.Helper()
	holder := PositionHolder{PositionKey: positionKey(title), PositionTitle: title, DirectoryPersonID: personID, StartsOn: startsOn, EndsOn: endsOn, Source: source, CreatedBy: "test"}
	if err := db.Create(&holder).Error; err != nil {
		t.Fatal(err)
	}
}

func localDay(year int, month time.Month, d int) *time.Time {
	value := time.Date(year, month, d, 0, 0, 0, 0, time.Local)
	return &value
}

func currentByTitle(t *testing.T, db *gorm.DB, today time.Time) map[string][]CurrentPositionHolder {
	t.Helper()
	rows, err := currentPositionHolders(db, today)
	if err != nil {
		t.Fatal(err)
	}
	byTitle := map[string][]CurrentPositionHolder{}
	for _, row := range rows {
		byTitle[row.PositionTitle] = append(byTitle[row.PositionTitle], row)
	}
	return byTitle
}

func TestCurrentHoldersFollowPickHolder(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405.000000")
	today := *localDay(2026, 11, 15)

	syncPerson := testPerson(t, db, "", "ซิงก์"+stamp, "")
	manualPerson := testPerson(t, db, "ผศ.ดร.", "กรอก"+stamp, "")
	inactivePerson := testPerson(t, db, "", "ลาออก"+stamp, "INACTIVE")

	manualWins := "หัวหน้างานทดสอบกรอกชนะ" + stamp
	testHolder(t, db, manualWins, syncPerson.ID, positionSourceSync, nil, nil)
	testHolder(t, db, manualWins, manualPerson.ID, positionSourceManual, nil, nil)

	endsToday := "หัวหน้างานทดสอบจบวันนี้" + stamp
	testHolder(t, db, endsToday, manualPerson.ID, positionSourceManual, localDay(2026, 1, 1), localDay(2026, 11, 15))

	ended := "หัวหน้างานทดสอบหมดวาระ" + stamp
	testHolder(t, db, ended, manualPerson.ID, positionSourceManual, nil, localDay(2026, 11, 14))

	future := "หัวหน้างานทดสอบยังไม่เริ่ม" + stamp
	testHolder(t, db, future, manualPerson.ID, positionSourceManual, localDay(2026, 12, 1), nil)

	inactive := "หัวหน้างานทดสอบคนลาออก" + stamp
	testHolder(t, db, inactive, inactivePerson.ID, positionSourceManual, nil, nil)

	learned := "หัวหน้างานทดสอบจากเอกสาร" + stamp
	testHolder(t, db, learned, syncPerson.ID, positionSourceLearned, localDay(2026, 6, 1), nil)

	got := currentByTitle(t, db, today)

	if rows := got[manualWins]; len(rows) != 1 || rows[0].DirectoryUserID != manualPerson.ID || rows[0].Source != positionSourceManual {
		t.Fatalf("staff-entered holder must win over sync, got %+v", rows)
	}
	if name := got[manualWins][0].FullName; name != "ผศ.ดร. กรอก"+stamp+" ทดสอบตำแหน่ง" {
		t.Fatalf("full name = %q", name)
	}
	if rows := got[endsToday]; len(rows) != 1 || rows[0].EndsOn == nil || !rows[0].EndsOn.Equal(*localDay(2026, 11, 15)) {
		t.Fatalf("a term ending today still counts today, got %+v", rows)
	}
	for _, title := range []string{ended, future, inactive} {
		if rows := got[title]; len(rows) != 0 {
			t.Fatalf("%s must not be listed, got %+v", title, rows)
		}
	}
	if rows := got[learned]; len(rows) != 1 || rows[0].Source != positionSourceLearned || rows[0].DirectoryUserID != syncPerson.ID {
		t.Fatalf("a holder learned from reviewed documents must be listed, got %+v", rows)
	}

	// เงื่อนไขหลัก: ทุกแถวต้องเป็นคนเดียวกับที่ระบบผูกให้เอกสารที่ออกวันนี้
	holders, err := loadPositionHolders(db)
	if err != nil {
		t.Fatal(err)
	}
	for title, rows := range got {
		holder := holderFor(title, holders, &today)
		if holder == nil || holder.DirectoryPersonID != rows[0].DirectoryUserID {
			t.Fatalf("%s: page shows person %d but documents link %+v", title, rows[0].DirectoryUserID, holder)
		}
	}
}

func TestCurrentHoldersListEveryPositionOfOnePerson(t *testing.T) {
	db := testDatabase(t)
	stamp := time.Now().Format("150405.000000")
	person := testPerson(t, db, "รศ.", "สองตำแหน่ง"+stamp, "")
	first := "หัวหน้างานทดสอบหนึ่ง" + stamp
	second := "หัวหน้างานทดสอบสอง" + stamp
	testHolder(t, db, first, person.ID, positionSourceManual, nil, nil)
	testHolder(t, db, second, person.ID, positionSourceManual, nil, nil)

	got := currentByTitle(t, db, *localDay(2026, 11, 15))
	if len(got[first]) != 1 || len(got[second]) != 1 || got[first][0].FullName != got[second][0].FullName {
		t.Fatalf("one person with two positions must appear on two rows, got %+v / %+v", got[first], got[second])
	}
}

func TestCurrentHoldersHandlerReturnsAnArray(t *testing.T) {
	db := testDatabase(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/position-holders/current", listCurrentPositionHolders(db))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/position-holders/current", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body.Data), "[") {
		t.Fatalf("data must be an array, got %s", body.Data)
	}
}
