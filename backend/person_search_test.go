package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestAppointmentStateFollowsCancellationAndTerm(t *testing.T) {
	today := *day("2026-10-18")
	for _, tc := range []struct {
		status string
		expiry *time.Time
		want   string
	}{
		{"CONFIRMED", nil, appointmentActive},
		{"CONFIRMED", day("2027-10-18"), appointmentActive},
		{"CONFIRMED", day("2027-01-17"), appointmentExpiring},
		{"CONFIRMED", day("2026-10-18"), appointmentExpiring},
		{"CONFIRMED", day("2026-10-17"), appointmentExpired},
		{documentCancelled, day("2027-10-18"), appointmentCancelled},
	} {
		if got := appointmentState(tc.status, tc.expiry, today); got != tc.want {
			t.Errorf("%s / %v = %s, want %s", tc.status, tc.expiry, got, tc.want)
		}
	}
}

func searchStamp() string {
	now := time.Now()
	return fmt.Sprintf("%s%06d", now.Format("150405"), now.Nanosecond()/1000)
}

// orderFor สร้างเอกสารที่มีสถานะและวันที่ตามต้องการ พร้อมแถวรายชื่อที่ผูกกับคนคนนี้
func orderFor(t *testing.T, db *gorm.DB, personID uint, status, orderNo string, issued, expiry *time.Time, role string) Document {
	t.Helper()
	document, _ := createReviewDocument(t, db, map[string]string{})
	updates := map[string]any{"status": status, "order_no": orderNo, "issued_date": issued, "expiry_date": expiry}
	if err := db.Model(&Document{}).Where("id = ?", document.ID).Updates(updates).Error; err != nil {
		t.Fatal(err)
	}
	id := personID
	row := Appointment{DocumentID: document.ID, FullName: "ทดสอบ", CommitteeRole: role, DirectoryUserID: &id, NameMatchMethod: matchMethodExact, BoundingBoxJSON: "null"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	return document
}

func searchPerson(t *testing.T, db *gorm.DB, name string) DirectoryPerson {
	t.Helper()
	person := DirectoryPerson{FirstName: name, LastName: "ค้นหา", Status: "A", PositionTitle: "อาจารย์", SyncedAt: time.Now()}
	if err := db.Create(&person).Error; err != nil {
		t.Fatal(err)
	}
	return person
}

func getJSON(t *testing.T, route, path string, handler gin.HandlerFunc, target any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(route, handler)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s", path, recorder.Code, recorder.Body.String())
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatal(err)
	}
}

func TestPersonHistoryShowsConfirmedAndCancelledOrdersNewestFirst(t *testing.T) {
	db := testDatabase(t)
	stamp := searchStamp()
	person := searchPerson(t, db, "ประวัติ"+stamp)
	soon := time.Now().AddDate(0, 0, 30)
	orderFor(t, db, person.ID, "CONFIRMED", "1/"+stamp, day("2025-11-01"), day("2099-01-01"), "กรรมการ")
	orderFor(t, db, person.ID, "CONFIRMED", "2/"+stamp, day("2026-01-10"), &soon, "ประธานกรรมการ")
	cancelled := orderFor(t, db, person.ID, documentCancelled, "3/"+stamp, day("2025-12-01"), nil, "เลขานุการ")
	db.Model(&Document{}).Where("id = ?", cancelled.ID).Update("confirmed_at", time.Now())
	orderFor(t, db, person.ID, "REVIEW", "4/"+stamp, day("2026-02-01"), nil, "กรรมการ")
	orderFor(t, db, person.ID, "CONFIRMED", "5/"+stamp, day("2024-03-01"), day("2025-03-01"), "กรรมการ")

	var result struct {
		Data personHistory `json:"data"`
	}
	route := "/api/v1/search/people/:id"
	base := fmt.Sprintf("/api/v1/search/people/%d", person.ID)
	getJSON(t, route, base+"?from=2025-10-01&to=2026-09-30", personHistoryHandler(db), &result)
	got := []string{}
	for _, row := range result.Data.Appointments {
		got = append(got, row.OrderNo+":"+row.State)
	}
	want := fmt.Sprintf("[2/%[1]s:EXPIRING 3/%[1]s:CANCELLED 1/%[1]s:ACTIVE]", stamp)
	if fmt.Sprint(got) != want {
		t.Fatalf("rows = %v, want %s", got, want)
	}
	card := result.Data.Person
	if card.Orders != 3 || card.Active != 2 || card.Expiring != 1 || card.Name == "" {
		t.Fatalf("card = %+v, want 3 confirmed orders, 2 still active, 1 expiring", card)
	}

	result.Data = personHistory{}
	getJSON(t, route, base+"?activeOnly=1", personHistoryHandler(db), &result)
	if len(result.Data.Appointments) != 2 {
		t.Fatalf("active only = %+v", result.Data.Appointments)
	}
}

func TestSuggestGroupsPeopleAndOrders(t *testing.T) {
	db := testDatabase(t)
	stamp := searchStamp()
	person := searchPerson(t, db, "แนะนำ"+stamp)
	orderFor(t, db, person.ID, "CONFIRMED", "ค"+stamp+"/2569", day("2026-01-10"), nil, "กรรมการ")

	var result struct {
		Data searchSuggestions `json:"data"`
	}
	getJSON(t, "/api/v1/search/suggest", "/api/v1/search/suggest?q="+url.QueryEscape("แนะนำ"+stamp), searchSuggestHandler(db), &result)
	if len(result.Data.People) != 1 || result.Data.People[0].ID != person.ID || result.Data.People[0].Orders != 1 || result.Data.People[0].Position != "อาจารย์" {
		t.Fatalf("people = %+v", result.Data.People)
	}
	result.Data = searchSuggestions{}
	getJSON(t, "/api/v1/search/suggest", "/api/v1/search/suggest?q="+url.QueryEscape("ค"+stamp), searchSuggestHandler(db), &result)
	if len(result.Data.Orders) != 1 || result.Data.Orders[0].OrderNo != "ค"+stamp+"/2569" {
		t.Fatalf("orders = %+v", result.Data.Orders)
	}
}

func TestTopPeopleCountConfirmedOrdersInTheFiscalYear(t *testing.T) {
	db := testDatabase(t)
	stamp := searchStamp()
	busy := searchPerson(t, db, "งานเยอะ"+stamp)
	quiet := searchPerson(t, db, "งานน้อย"+stamp)
	// ใช้ปีที่ไกลมาก ข้อมูลจากเทสต์อื่นจะได้ไม่ปน
	year := 2101 + time.Now().Nanosecond()%290 // ต่ำกว่า 2400 ไม่งั้นถูกอ่านเป็นปี พ.ศ.
	inYear := day(fmt.Sprintf("%d-01-15", year))
	orderFor(t, db, busy.ID, "CONFIRMED", "a"+stamp, inYear, nil, "กรรมการ")
	orderFor(t, db, busy.ID, "CONFIRMED", "b"+stamp, inYear, nil, "กรรมการ")
	orderFor(t, db, busy.ID, documentCancelled, "c"+stamp, inYear, nil, "กรรมการ")
	orderFor(t, db, quiet.ID, "CONFIRMED", "d"+stamp, inYear, nil, "กรรมการ")
	orderFor(t, db, quiet.ID, "CONFIRMED", "e"+stamp, day(fmt.Sprintf("%d-11-01", year)), nil, "กรรมการ")

	var result struct {
		Data []personSummary `json:"data"`
	}
	path := fmt.Sprintf("/api/v1/search/top-people?from=%d-10-01&to=%d-09-30", year-1, year)
	getJSON(t, "/api/v1/search/top-people", path, topPeopleHandler(db), &result)
	// ฐานทดสอบสะสมข้อมูลจากรอบก่อน ปีอาจซ้ำกันได้ จึงดูเฉพาะสองคนของรอบนี้
	position := map[uint]int{}
	for i, person := range result.Data {
		if person.ID == busy.ID && person.Orders == 2 || person.ID == quiet.ID && person.Orders == 1 {
			position[person.ID] = i + 1
		}
	}
	if position[busy.ID] == 0 || position[quiet.ID] == 0 || position[busy.ID] > position[quiet.ID] {
		t.Fatalf("top people = %+v, want %d (2 orders) before %d (1 order)", result.Data, busy.ID, quiet.ID)
	}
}

func TestPersonHistoryKeepsEveryRoleAndSkipsOrdersCancelledBeforeConfirmation(t *testing.T) {
	db := testDatabase(t)
	stamp := searchStamp()
	person := searchPerson(t, db, "หลายบทบาท"+stamp)
	db.Model(&DirectoryPerson{}).Where("id = ?", person.ID).Update("status", "I")
	both := orderFor(t, db, person.ID, "CONFIRMED", "1/"+stamp, day("2026-01-10"), nil, "ประธานกรรมการ")
	id := person.ID
	db.Create(&Appointment{DocumentID: both.ID, FullName: "ทดสอบ", CommitteeRole: "กรรมการ", DirectoryUserID: &id, NameMatchMethod: matchMethodExact, BoundingBoxJSON: "null"})
	orderFor(t, db, person.ID, documentCancelled, "2/"+stamp, day("2026-02-10"), nil, "กรรมการ")
	afterConfirm := orderFor(t, db, person.ID, documentCancelled, "3/"+stamp, day("2026-03-10"), nil, "กรรมการ")
	db.Model(&Document{}).Where("id = ?", afterConfirm.ID).Update("confirmed_at", time.Now())

	var result struct {
		Data personHistory `json:"data"`
	}
	getJSON(t, "/api/v1/search/people/:id", fmt.Sprintf("/api/v1/search/people/%d", person.ID), personHistoryHandler(db), &result)
	got := []string{}
	for _, row := range result.Data.Appointments {
		got = append(got, row.OrderNo+":"+row.Role)
	}
	want := fmt.Sprintf("[3/%[1]s:กรรมการ 1/%[1]s:ประธานกรรมการ, กรรมการ]", stamp)
	if fmt.Sprint(got) != want {
		t.Fatalf("rows = %v, want %s (people who left still have a history)", got, want)
	}
}

func TestSuggestPutsNamesThatStartWithTheQueryFirstAndTreatsOrderWildcardsAsText(t *testing.T) {
	db := testDatabase(t)
	stamp := searchStamp()
	starts := searchPerson(t, db, "ตรง"+stamp)
	busy := searchPerson(t, db, "ไม่ตรง"+stamp)
	for i := 0; i < 3; i++ {
		orderFor(t, db, busy.ID, "CONFIRMED", fmt.Sprintf("%d/%s", i, stamp), day("2026-01-10"), nil, "กรรมการ")
	}
	var result struct {
		Data searchSuggestions `json:"data"`
	}
	getJSON(t, "/api/v1/search/suggest", "/api/v1/search/suggest?q="+url.QueryEscape("ตรง"+stamp), searchSuggestHandler(db), &result)
	if len(result.Data.People) != 2 || result.Data.People[0].ID != starts.ID {
		t.Fatalf("people = %+v, want the name that starts with the query first", result.Data.People)
	}
	result.Data = searchSuggestions{}
	getJSON(t, "/api/v1/search/suggest", "/api/v1/search/suggest?q="+url.QueryEscape("%_"+stamp), searchSuggestHandler(db), &result)
	// ชื่อคนตัดเครื่องหมายออกก่อนเทียบอยู่แล้ว ส่วนเลขที่คำสั่งและชื่อเรื่องต้องถือ % และ _ เป็นตัวอักษร
	if len(result.Data.Orders) != 0 {
		t.Fatalf("%% and _ must match only themselves, got %+v", result.Data.Orders)
	}
}

func TestNamesWrittenInOrdersButNotLinkedCanBeFound(t *testing.T) {
	db := testDatabase(t)
	stamp := searchStamp()
	name := "ชญาดา" + stamp + " สุระวนิชกุล"
	unlinked := func(status, orderNo, issued string) Document {
		document := reportOrder(t, db, map[string]any{"status": status, "order_no": orderNo, "issued_date": day(issued)}, "",
			Appointment{FullName: name, Position: "รองศาสตราจารย์", CommitteeRole: "กรรมการ", NameMatchMethod: "NOT_FOUND"})
		return document
	}
	unlinked("CONFIRMED", "1/"+stamp, "2026-01-10")
	unlinked("CONFIRMED", "2/"+stamp, "2026-02-10")
	unlinked("REVIEW", "3/"+stamp, "2026-03-10")
	// OCR บางครั้งใส่ช่องว่างพิเศษ (NBSP) กลางชื่อ ยังต้องนับเป็นคนเดียวกัน
	reportOrder(t, db, map[string]any{"status": "CONFIRMED", "order_no": "4/" + stamp, "issued_date": day("2026-04-10")}, "",
		Appointment{FullName: "ชญาดา" + stamp + "\u00a0สุระวนิชกุล", Position: "ศาสตราจารย์", CommitteeRole: "กรรมการ", NameMatchMethod: "NOT_FOUND"})

	var suggest struct {
		Data searchSuggestions `json:"data"`
	}
	getJSON(t, "/api/v1/search/suggest", "/api/v1/search/suggest?q="+url.QueryEscape("ชญาดา"+stamp), searchSuggestHandler(db), &suggest)
	if len(suggest.Data.Names) != 1 || suggest.Data.Names[0].Name != name || suggest.Data.Names[0].Orders != 3 || suggest.Data.Names[0].Position != "ศาสตราจารย์" {
		t.Fatalf("names = %+v", suggest.Data.Names)
	}

	var history struct {
		Data personHistory `json:"data"`
	}
	getJSON(t, "/api/v1/search/names", "/api/v1/search/names?name="+url.QueryEscape(" ชญาดา"+stamp+"  สุระวนิชกุล"), nameHistoryHandler(db), &history)
	got := []string{}
	for _, row := range history.Data.Appointments {
		got = append(got, row.OrderNo)
	}
	if fmt.Sprint(got) != fmt.Sprintf("[4/%[1]s 2/%[1]s 1/%[1]s]", stamp) || history.Data.Person.Name != name || history.Data.Person.Orders != 3 {
		t.Fatalf("history = %+v rows %v", history.Data.Person, got)
	}

	report, err := buildAppointmentReport(db, reportFilter{Name: name}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 3 || report.PersonName != name || report.Rows[0].Name != name {
		t.Fatalf("report = %q %v", report.PersonName, report.Rows)
	}
	missing, _ := buildAppointmentReport(db, reportFilter{Name: "ไม่มีชื่อนี้" + stamp}, time.Now())
	if len(missing.Rows) != 0 || missing.PersonName != "ไม่มีชื่อนี้"+stamp {
		t.Fatalf("an unknown name must not read as a report of everyone: %q", missing.Scope)
	}
}
