package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// reportOrder สร้างเอกสารสำหรับรายงาน พร้อมแถวรายชื่อหนึ่งแถว
func reportOrder(t *testing.T, db *gorm.DB, updates map[string]any, effectiveText string, row Appointment) Document {
	t.Helper()
	document, _ := createReviewDocument(t, db, map[string]string{"effective_date": effectiveText})
	if err := db.Model(&Document{}).Where("id = ?", document.ID).Updates(updates).Error; err != nil {
		t.Fatal(err)
	}
	row.DocumentID = document.ID
	row.BoundingBoxJSON = "null"
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	return document
}

func TestReportRowsFollowTheSampleColumns(t *testing.T) {
	db := testDatabase(t)
	stamp := searchStamp()
	person := DirectoryPerson{Prefix: "นางสาว", FirstName: "รายงาน" + stamp, LastName: "ทดสอบ", Status: "A", SyncedAt: time.Now()}
	db.Create(&person)
	id := person.ID
	confirmedAt := time.Now()
	reportOrder(t, db, map[string]any{"status": "CONFIRMED", "order_no": "12/2569", "title": "แต่งตั้งคณะกรรมการ ก", "issuing_unit": "คำสั่งวิทยาลัยการคอมพิวเตอร์", "issued_date": day("2026-01-10"), "effective_date": day("2026-01-10")}, "ตั้งแต่บัดนี้เป็นต้นไป",
		Appointment{FullName: "ชื่อในเอกสาร", CommitteeRole: "กรรมการและเลขานุการ", DirectoryUserID: &id, NameMatchMethod: matchMethodExact})
	reportOrder(t, db, map[string]any{"status": "CONFIRMED", "order_no": "3/2569", "title": "แต่งตั้งคณะกรรมการ ข", "issuing_unit": "คำสั่งมหาวิทยาลัยขอนแก่น", "issued_date": day("2026-01-10"), "effective_date": day("2026-02-01"), "expiry_date": day("2027-01-31")}, "1 กุมภาพันธ์ 2569",
		Appointment{FullName: "ชื่อในเอกสาร", CommitteeRole: "กรรมการ", Responsibilities: "ดูแลการรับสมัคร", DirectoryUserID: &id, NameMatchMethod: matchMethodExact})
	reportOrder(t, db, map[string]any{"status": documentCancelled, "confirmed_at": confirmedAt, "order_no": "5/2569", "title": "แต่งตั้งคณะกรรมการ ค", "issued_date": day("2025-12-01"), "replaced_by_order_no": "40/2569", "cancellation_reason": "ใช้คำสั่งเลขที่ 40/2569 แทน"}, "",
		Appointment{FullName: "ชื่อในเอกสาร", CommitteeRole: "เป็นประธาน", DirectoryUserID: &id, NameMatchMethod: matchMethodExact})
	reportOrder(t, db, map[string]any{"status": documentCancelled, "confirmed_at": confirmedAt, "order_no": "1/2569", "title": "แต่งตั้งคณะกรรมการ ง", "issued_date": day("2025-11-01"), "cancellation_reason": "ออกซ้ำ"}, "",
		Appointment{FullName: "ชื่อในเอกสาร", DirectoryUserID: &id, NameMatchMethod: matchMethodExact})

	report, err := buildAppointmentReport(db, reportFilter{PersonID: person.ID}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	name := "นางสาวรายงาน" + stamp + " ทดสอบ" // คำนำหน้าเขียนติดชื่อแบบหนังสือราชการ
	want := []reportRow{
		{No: 1, Name: name, OrderType: "-", OrderNo: "1/2569", Subject: "แต่งตั้งคณะกรรมการ ง", Duty: "-", Issued: "1 พฤศจิกายน 2568", Effective: "-", Expiry: "-", Note: "ยกเลิก: ออกซ้ำ"},
		{No: 2, Name: name, OrderType: "-", OrderNo: "5/2569", Subject: "แต่งตั้งคณะกรรมการ ค", Duty: "เป็นประธาน", Issued: "1 ธันวาคม 2568", Effective: "-", Expiry: "-", Note: "ยกเลิก ใช้คำสั่ง 40/2569 แทน"},
		{No: 3, Name: name, OrderType: "คำสั่งมหาวิทยาลัยขอนแก่น", OrderNo: "3/2569", Subject: "แต่งตั้งคณะกรรมการ ข", Duty: "ดูแลการรับสมัคร", Issued: "10 มกราคม 2569", Effective: "ทั้งนี้ ตั้งแต่วันที่ 1 กุมภาพันธ์ 2569 เป็นต้นไป", Expiry: "31 มกราคม 2570", Note: "-"},
		{No: 4, Name: name, OrderType: "คำสั่งวิทยาลัยการคอมพิวเตอร์", OrderNo: "12/2569", Subject: "แต่งตั้งคณะกรรมการ ก", Duty: "เป็นกรรมการและเลขานุการ", Issued: "10 มกราคม 2569", Effective: "ทั้งนี้ ตั้งแต่บัดนี้เป็นต้นไป", Expiry: "-", Note: "-"},
	}
	if fmt.Sprint(report.Rows) != fmt.Sprint(want) {
		t.Fatalf("rows:\n%v\nwant:\n%v", report.Rows, want)
	}
	if report.Scope == "" || report.PersonName != name {
		t.Fatalf("the report should say whose orders it lists, got %q / %q", report.Scope, report.PersonName)
	}
	empty, _ := buildAppointmentReport(db, reportFilter{PersonID: person.ID, From: day("2099-01-01")}, time.Now())
	if len(empty.Rows) != 0 || empty.PersonName != name {
		t.Fatalf("a period without orders still names the person, got %q", empty.PersonName)
	}

	ranged, _ := buildAppointmentReport(db, reportFilter{PersonID: person.ID, From: day("2026-01-01")}, time.Now())
	if len(ranged.Rows) != 2 || ranged.Rows[0].No != 1 || ranged.Rows[0].OrderNo != "3/2569" {
		t.Fatalf("date filter rows = %v", ranged.Rows)
	}
}

func TestCancellationNoteKeepsTheTypedReason(t *testing.T) {
	for _, tc := range []struct{ status, reason, replaced, want string }{
		{documentCancelled, "ใช้คำสั่งเลขที่ 40/2569 แทน", "40/2569", "ยกเลิก ใช้คำสั่ง 40/2569 แทน"},
		{documentCancelled, "ออกคำสั่งผิดบุคคล", "40/2569", "ยกเลิก ใช้คำสั่ง 40/2569 แทน (ออกคำสั่งผิดบุคคล)"},
		{documentCancelled, "ออกซ้ำ", "", "ยกเลิก: ออกซ้ำ"},
		{"CONFIRMED", "", "", "-"},
	} {
		if got := reportNote(tc.status, tc.reason, tc.replaced); got != tc.want {
			t.Errorf("reportNote(%q, %q) = %q, want %q", tc.reason, tc.replaced, got, tc.want)
		}
	}
}

func TestOfficialNamesJoinThaiPrefixesOnly(t *testing.T) {
	if got := officialName(DirectoryUser{Prefix: "ผศ.ดร.", FirstName: "สมชาย", LastName: "ใจดี"}); got != "ผศ.ดร.สมชาย ใจดี" {
		t.Errorf("thai = %q", got)
	}
	if got := officialName(DirectoryUser{Prefix: "Dr.", FirstName: "John", LastName: "Smith"}); got != "Dr. John Smith" {
		t.Errorf("latin = %q", got)
	}
}

func TestReportForEveryoneListsNamesNotLinkedToAPerson(t *testing.T) {
	db := testDatabase(t)
	stamp := searchStamp()
	year := 2101 + time.Now().Nanosecond()%290
	issued := day(fmt.Sprintf("%d-03-01", year))
	document := reportOrder(t, db, map[string]any{"status": "CONFIRMED", "order_no": "7/" + stamp, "issued_date": issued}, "",
		Appointment{FullName: "บุคคลภายนอก" + stamp, CommitteeRole: "กรรมการ"})
	db.Create(&Appointment{DocumentID: document.ID, Position: "คณบดีวิทยาลัยการคอมพิวเตอร์", CommitteeRole: "ประธานกรรมการ", NameMatchMethod: matchMethodExOfficio, BoundingBoxJSON: "null"})

	report, err := buildAppointmentReport(db, reportFilter{From: day(fmt.Sprintf("%d-01-01", year)), To: day(fmt.Sprintf("%d-12-31", year))}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, row := range report.Rows {
		if row.OrderNo == "7/"+stamp {
			names = append(names, row.Name)
		}
	}
	if fmt.Sprint(names) != fmt.Sprintf("[บุคคลภายนอก%s คณบดีวิทยาลัยการคอมพิวเตอร์]", stamp) {
		t.Fatalf("names = %v", names)
	}
}

func TestReportDutyAddsWhatReviewersWrote(t *testing.T) {
	for _, tc := range []struct{ responsibilities, extra, role, want string }{
		{"วางแผนและประสานงาน", "", "กรรมการ", "วางแผนและประสานงาน"},
		{"วางแผนและประสานงาน", "จัดทำรายงานสรุป\n\nดูแลงบประมาณ", "กรรมการ", "วางแผนและประสานงาน\nจัดทำรายงานสรุป\nดูแลงบประมาณ"},
		{"", "ดูแลการรับชำระค่าลงทะเบียน", "กรรมการ", "ดูแลการรับชำระค่าลงทะเบียน"},
		{"", "", "กรรมการ", "เป็นกรรมการ"},
	} {
		if got := reportDuty(tc.responsibilities, tc.extra, tc.role); got != tc.want {
			t.Errorf("reportDuty(%q, %q) = %q, want %q", tc.responsibilities, tc.extra, got, tc.want)
		}
	}
}

func TestSavingADocumentKeepsExtraDuties(t *testing.T) {
	db := testDatabase(t)
	document, _ := createReviewDocument(t, db, map[string]string{})
	row := Appointment{DocumentID: document.ID, FullName: "ทดสอบ หน้าที่", Department: "ฝ่ายทดสอบ", Responsibilities: "ตามคำสั่ง", BoundingBoxJSON: "null"}
	db.Create(&row)
	response := patchDocument(t, db, document.ID, map[string]any{"appointments": []map[string]any{{"id": row.ID, "fullName": row.FullName, "department": row.Department, "responsibilities": "ตามคำสั่ง", "extraDuties": "เขียนเพิ่ม", "boundingBox": "null"}}})
	if response.Code != 200 {
		t.Fatalf("save = %d %s", response.Code, response.Body.String())
	}
	saved := rowOf(t, db, document.ID)
	if saved.ExtraDuties != "เขียนเพิ่ม" || saved.Responsibilities != "ตามคำสั่ง" {
		t.Fatalf("saved = %q / %q", saved.Responsibilities, saved.ExtraDuties)
	}
}

func TestRescanKeepsDutiesReviewersWrote(t *testing.T) {
	previous := []Appointment{{Department: "ฝ่ายดำเนินงาน", ExtraDuties: "จัดทำรายงานสรุป"}, {Department: "ฝ่ายดำเนินงาน", ExtraDuties: "จัดทำรายงานสรุป"}, {Department: "ฝ่ายเก่า", ExtraDuties: "ไม่มีแล้ว"}}
	fresh := []Appointment{{Department: "ฝ่ายดำเนินงาน"}, {Department: " ฝ่ายดำเนินงาน "}, {Department: "ฝ่ายใหม่"}}
	carryExtraDuties(previous, fresh)
	if fresh[0].ExtraDuties != "จัดทำรายงานสรุป" || fresh[1].ExtraDuties != "จัดทำรายงานสรุป" || fresh[2].ExtraDuties != "" {
		t.Fatalf("after rescan = %+v", fresh)
	}
}

func TestMergedDutiesDoNotRepeatSharedLines(t *testing.T) {
	duties := []string{}
	for _, duty := range []string{"ประสานงาน\nงาน A", "ประสานงาน\nงาน B"} {
		duties = appendDutyLines(duties, duty)
	}
	if strings.Join(duties, "|") != "ประสานงาน|งาน A|งาน B" {
		t.Fatalf("duties = %v", duties)
	}
}
