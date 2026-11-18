package main

import "testing"

// แถวตาราง CSV/XLSX/DOCX ที่คอลัมน์ชื่อว่าง (ตำแหน่งโดยตำแหน่ง) ต้องคงตำแหน่งคอลัมน์ไว้
// ตำแหน่งต้องไม่เลื่อนไปอยู่ในช่องชื่อ และแถวต้องเป็นแถวบทบาทล้วน (ไม่มีชื่อ มีตำแหน่ง)
func TestTabularRowWithEmptyNameColumnStaysRoleOnly(t *testing.T) {
	result := structuredResult(
		[]string{"เรื่อง แต่งตั้งคณะกรรมการประจำวิทยาลัย"},
		[]string{"ลำดับ", "ชื่อ", "ตำแหน่ง", "บทบาท"},
		[]string{"1", "", "คณบดีวิทยาลัยการคอมพิวเตอร์", "ประธานกรรมการ"},
		[]string{"2", "นายสมชาย ใจดี", "", "กรรมการ"},
		[]string{"3", "นางสาวสมหญิง รักเรียน", "อาจารย์", "กรรมการและเลขานุการ"},
	)
	meta := extractCommandMetadata(result.Pages, joinPageText(result.Pages))
	want := []struct{ name, position, role string }{
		{"", "คณบดีวิทยาลัยการคอมพิวเตอร์", "ประธานกรรมการ"},
		{"นายสมชาย ใจดี", "", "กรรมการ"},
		{"นางสาวสมหญิง รักเรียน", "อาจารย์", "กรรมการและเลขานุการ"},
	}
	if len(meta.appointments) != len(want) {
		t.Fatalf("appointments = %+v, want %d rows", meta.appointments, len(want))
	}
	for i, w := range want {
		got := meta.appointments[i]
		if got.FullName != w.name || got.Position != w.position || got.CommitteeRole != w.role {
			t.Errorf("row %d = {%q %q %q}, want {%q %q %q}", i+1, got.FullName, got.Position, got.CommitteeRole, w.name, w.position, w.role)
		}
	}

	extraction := structuredExtraction(result)
	if len(extraction.Groups) == 0 || len(extraction.Groups[0].Members) != 1 || !extraction.Groups[0].Members[0].ExOfficio {
		t.Fatalf("role-only row must be imported as ex officio, groups = %+v", extraction.Groups)
	}
}

// แถวที่มีแค่ลำดับกับบทบาท (ไม่มีทั้งชื่อและตำแหน่ง) ไม่ใช่รายชื่อ
func TestTabularRowWithoutNameOrPositionIsSkipped(t *testing.T) {
	result := structuredResult(
		[]string{"เรื่อง แต่งตั้งคณะกรรมการ"},
		[]string{"1", "", "", "กรรมการ"},
	)
	meta := extractCommandMetadata(result.Pages, joinPageText(result.Pages))
	if len(meta.appointments) != 0 {
		t.Fatalf("appointments = %+v, want none", meta.appointments)
	}
}
