package main

import "testing"

func TestNormalizeThaiDateAcceptsTheShapesOCRProduces(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"8 กันยายน 2566", "2023-09-08"},
		{"๘ กันยายน ๒๕๖๖", "2023-09-08"},
		{"11ตุลาคม2569", "2026-10-11"},
		{"11 ต.ค. 2569", "2026-10-11"},
		{"11 ต.ค 2569", "2026-10-11"},
		{"11 ต.ค. 69", "2026-10-11"},
		{"11 ตุลาคม พ.ศ. 2569", "2026-10-11"},
		{"วันที่ 11 เดือนตุลาคม พ.ศ.2569", "2026-10-11"},
		{"1 ต.ค. 2023", "2023-10-01"},
		{"สั่ง ณ วันที่ 3 มีนาคม 2569", "2026-03-03"},
		{"8/9/2566", "2023-09-08"},
		{"08/09/2023", "2023-09-08"},
		{"2023-09-08", "2023-09-08"},
		{"2566-09-08", "2023-09-08"},
	} {
		got, ok := normalizeThaiDate(tc.in)
		if !ok || got == nil || got.Format("2006-01-02") != tc.want {
			t.Errorf("normalizeThaiDate(%q) = %v, %v; want %s", tc.in, got, ok, tc.want)
		}
	}
}

func TestNormalizeThaiDateRejectsWhatIsNotADate(t *testing.T) {
	for _, in := range []string{"31 กุมภาพันธ์ 2569", "บัดนี้", "ตั้งแต่บัดนี้เป็นต้นไป", "สิบเอ็ด ตุลา", "30/2/2569"} {
		if got, ok := normalizeThaiDate(in); ok || got != nil {
			t.Errorf("normalizeThaiDate(%q) = %v, %v; want no date", in, got, ok)
		}
	}
}

func TestEmptyDateIsAcceptedAsNoDate(t *testing.T) {
	if got, ok := normalizeThaiDate("  "); !ok || got != nil {
		t.Fatalf("empty = %v, %v; want nil, true", got, ok)
	}
}

func TestDateRangeGivesFirstAndLastDate(t *testing.T) {
	first, last := thaiDateRange("ตั้งแต่ 1 ตุลาคม 2566 ถึง 30 กันยายน 2567")
	if first == nil || last == nil || first.Format("2006-01-02") != "2023-10-01" || last.Format("2006-01-02") != "2024-09-30" {
		t.Fatalf("range = %v, %v", first, last)
	}
}

func TestFromNowPhrase(t *testing.T) {
	for _, in := range []string{"บัดนี้", "ทั้งนี้ ตั้งแต่บัดนี้เป็นต้นไป"} {
		if !isFromNow(in) {
			t.Errorf("isFromNow(%q) = false", in)
		}
	}
	if isFromNow("1 ตุลาคม 2566") {
		t.Error("a date is not 'from now'")
	}
}

func TestNotSpecifiedMeansNoDate(t *testing.T) {
	for _, in := range []string{"-", "ไม่ระบุ", "ไม่มี", " – "} {
		if got, ok := normalizeThaiDate(in); !ok || got != nil {
			t.Errorf("normalizeThaiDate(%q) = %v, %v; want nil, true", in, got, ok)
		}
	}
}

func TestNumbersInsideLongerNumbersAreNotDates(t *testing.T) {
	for _, in := range []string{"125 มกราคม 2566", "5 ม.ค. 256"} {
		if got, ok := normalizeThaiDate(in); ok || got != nil {
			t.Errorf("normalizeThaiDate(%q) = %v, %v; want no date", in, got, ok)
		}
	}
}

func TestTwoDigitSlashYearsBelowFiftyAreChristianEra(t *testing.T) {
	if got, ok := normalizeThaiDate("8/9/23"); !ok || got.Format("2006-01-02") != "2023-09-08" {
		t.Fatalf("8/9/23 = %v, %v", got, ok)
	}
	if got, ok := normalizeThaiDate("8/9/69"); !ok || got.Format("2006-01-02") != "2026-09-08" {
		t.Fatalf("8/9/69 = %v, %v", got, ok)
	}
}

func TestExpiryUsesTheEndOfARange(t *testing.T) {
	text := "ตั้งแต่ 1 ม.ค. 2566 ถึง 31 ธ.ค. 2567"
	if got, _ := parseDateForField("expiry_date", text); got == nil || got.Format("2006-01-02") != "2024-12-31" {
		t.Fatalf("expiry = %v", got)
	}
	if got, _ := parseDateForField("effective_date", text); got == nil || got.Format("2006-01-02") != "2023-01-01" {
		t.Fatalf("effective = %v", got)
	}
}
