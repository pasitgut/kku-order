package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// thaiMonths ไล่ชื่อยาวก่อน เพื่อให้ "มีนาคม" ไม่ถูกจับเป็นตัวย่ออื่น และผลเหมือนเดิมทุกครั้ง
var thaiMonths = []struct {
	pattern string
	month   time.Month
}{
	{"พฤศจิกายน", 11}, {"กุมภาพันธ์", 2}, {"กรกฎาคม", 7}, {"มิถุนายน", 6}, {"พฤษภาคม", 5}, {"สิงหาคม", 8},
	{"ธันวาคม", 12}, {"มกราคม", 1}, {"มีนาคม", 3}, {"เมษายน", 4}, {"กันยายน", 9}, {"ตุลาคม", 10},
	{`มี\.ค\.?`, 3}, {`เม\.ย\.?`, 4}, {`มิ\.ย\.?`, 6}, {`พ\.ย\.?`, 11}, {`ม\.ค\.?`, 1}, {`ก\.พ\.?`, 2},
	{`พ\.ค\.?`, 5}, {`ก\.ค\.?`, 7}, {`ส\.ค\.?`, 8}, {`ก\.ย\.?`, 9}, {`ต\.ค\.?`, 10}, {`ธ\.ค\.?`, 12},
}

var (
	thaiMonthDatePattern = func() *regexp.Regexp {
		names := make([]string, len(thaiMonths))
		for i, m := range thaiMonths {
			names[i] = m.pattern
		}
		return regexp.MustCompile(`(?:^|\D)(\d{1,2})\s*(` + strings.Join(names, "|") + `)\s*(\d{4}|\d{2})(?:\D|$)`)
	}()
	// ขอบตัวเลขกันไม่ให้หยิบวันที่จากเลขที่ยาวกว่า เช่น "125 มกราคม" หรือปีที่ OCR อ่านขาด "256"
	slashDatePattern = regexp.MustCompile(`(?:^|\D)(\d{1,2})/(\d{1,2})/(\d{4}|\d{2})(?:\D|$)`)
	isoDatePattern   = regexp.MustCompile(`(?:^|\D)(\d{4})-(\d{1,2})-(\d{1,2})(?:\D|$)`)
	monthPatterns    = func() []*regexp.Regexp {
		compiled := make([]*regexp.Regexp, len(thaiMonths))
		for i, m := range thaiMonths {
			compiled[i] = regexp.MustCompile(`^` + m.pattern + `$`)
		}
		return compiled
	}()
	eraWordPattern = regexp.MustCompile(`พ\.\s*ศ\.?|ค\.\s*ศ\.?|เดือน`)
)

type foundDate struct {
	at   int
	date time.Time
}

func monthFromName(name string) time.Month {
	for i, pattern := range monthPatterns {
		if pattern.MatchString(name) {
			return thaiMonths[i].month
		}
	}
	return 0
}

// toGregorianYear: ปีเกิน 2400 เป็น พ.ศ. ส่วนปี 2 หลัก ตั้งแต่ 50 ขึ้นไปเป็น พ.ศ. 25xx ต่ำกว่านั้นเป็น ค.ศ. 20xx
func toGregorianYear(raw string) int {
	year, _ := strconv.Atoi(raw)
	if len(raw) == 2 {
		if year >= 50 {
			year += 2500
		} else {
			year += 2000
		}
	}
	if year > 2400 {
		year -= 543
	}
	return year
}

// validDate ไม่ยอมให้ 31 ก.พ. ปัดไปเป็นวันในเดือนถัดไป
func validDate(year int, month time.Month, day int) (time.Time, bool) {
	if month < 1 || month > 12 || day < 1 {
		return time.Time{}, false
	}
	value := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	return value, value.Day() == day && value.Month() == month
}

func findThaiDates(text string) []foundDate {
	text = eraWordPattern.ReplaceAllString(replaceThaiDigits(text), " ")
	var found []foundDate
	for _, m := range thaiMonthDatePattern.FindAllStringSubmatchIndex(text, -1) {
		day, _ := strconv.Atoi(text[m[2]:m[3]])
		if value, ok := validDate(toGregorianYear(text[m[6]:m[7]]), monthFromName(text[m[4]:m[5]]), day); ok {
			found = append(found, foundDate{m[0], value})
		}
	}
	for _, m := range slashDatePattern.FindAllStringSubmatchIndex(text, -1) {
		day, _ := strconv.Atoi(text[m[2]:m[3]])
		month, _ := strconv.Atoi(text[m[4]:m[5]])
		if value, ok := validDate(toGregorianYear(text[m[6]:m[7]]), time.Month(month), day); ok {
			found = append(found, foundDate{m[0], value})
		}
	}
	for _, m := range isoDatePattern.FindAllStringSubmatchIndex(text, -1) {
		month, _ := strconv.Atoi(text[m[4]:m[5]])
		day, _ := strconv.Atoi(text[m[6]:m[7]])
		if value, ok := validDate(toGregorianYear(text[m[2]:m[3]]), time.Month(month), day); ok {
			found = append(found, foundDate{m[0], value})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].at < found[j].at })
	return found
}

// normalizeThaiDate อ่านวันที่แรกในข้อความ ข้อความว่างคือ "ไม่มีวันที่" (ok) ส่วนข้อความที่อ่านไม่ออกคือ !ok
func normalizeThaiDate(text string) (*time.Time, bool) {
	switch strings.TrimSpace(text) {
	case "", "-", "–", "—", "ไม่ระบุ", "ไม่มี":
		return nil, true
	}
	found := findThaiDates(text)
	if len(found) == 0 {
		return nil, false
	}
	return &found[0].date, true
}

// thaiDateRange ใช้กับช่วง "ตั้งแต่ ... ถึง ...": วันแรกเป็นวันเริ่ม วันสุดท้ายเป็นวันสิ้นสุด
func thaiDateRange(text string) (first, last *time.Time) {
	found := findThaiDates(text)
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0].date, &found[len(found)-1].date
}

// parseDateForField: วันครบวาระที่เขียนเป็นช่วงใช้วันสุดท้าย ส่วนวันออกคำสั่งและวันมีผลใช้วันแรก
func parseDateForField(key, text string) (*time.Time, bool) {
	parsed, ok := normalizeThaiDate(text)
	if key != "expiry_date" || !ok || parsed == nil {
		return parsed, ok
	}
	_, last := thaiDateRange(text)
	return last, true
}

func isFromNow(text string) bool {
	return strings.Contains(text, "บัดนี้")
}

var dateFieldLabels = map[string]string{"issued_date": "วันที่ออกคำสั่ง", "effective_date": "วันที่มีผล", "expiry_date": "วันที่สิ้นสุด"}

// unreadableDates คืนข้อความวันที่ที่ยังอ่านไม่ออก ต้องแก้ก่อนยืนยัน เพราะวันครบวาระและการแจ้งเตือนคำนวณจากวันที่เหล่านี้
func unreadableDates(fields []ExtractedField) []string {
	var problems []string
	for _, field := range fields {
		label, isDate := dateFieldLabels[field.FieldKey]
		if !isDate || (field.FieldKey == "effective_date" && isFromNow(field.Value)) {
			continue
		}
		if _, ok := parseDateForField(field.FieldKey, field.Value); !ok {
			problems = append(problems, label+" \""+strings.TrimSpace(field.Value)+"\"")
		}
	}
	return problems
}

// annotateDateFields: "read" อ่านได้ (ReadDate = YYYY-MM-DD), "none" ว่างหรือไม่ระบุ, "fromNow" ตั้งแต่บัดนี้, "unreadable" อ่านไม่ออก
func annotateDateFields(fields []ExtractedField) {
	for i := range fields {
		field := &fields[i]
		if _, isDate := dateFieldLabels[field.FieldKey]; !isDate {
			continue
		}
		if field.FieldKey == "effective_date" && isFromNow(field.Value) {
			field.DateStatus = "fromNow"
			continue
		}
		parsed, ok := parseDateForField(field.FieldKey, field.Value)
		switch {
		case !ok:
			field.DateStatus = "unreadable"
		case parsed == nil:
			field.DateStatus = "none"
		default:
			field.DateStatus = "read"
			field.ReadDate = parsed.Format("2006-01-02")
		}
	}
}
