package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNormalizeUserItemsSupportsCommonEnvelopes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "array", body: `[{"id":"1","email":"one@kku.ac.th"},{"id":"2","email":"two@kku.ac.th"}]`, want: 2},
		{name: "data envelope", body: `{"data":[{"id":"1","name":"One"}]}`, want: 1},
		{name: "users envelope", body: `{"users":{"items":[{"userId":"1","name":"One"}]}}`, want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			items, err := normalizeUserItems([]byte(test.body))
			if err != nil {
				t.Fatalf("normalizeUserItems() error = %v", err)
			}
			if len(items) != test.want {
				t.Fatalf("normalizeUserItems() count = %d, want %d", len(items), test.want)
			}
		})
	}
}

func TestUserFullNameBuildsFromParts(t *testing.T) {
	item := map[string]any{"first_name": "สมชาย", "last_name": "ใจดี"}
	if got := userFullName(item); got != "สมชาย ใจดี" {
		t.Fatalf("userFullName() = %q, want %q", got, "สมชาย ใจดี")
	}
	actualItem := map[string]any{"prefix": "นาย", "user_fname": "สมชาย", "user_lname": "ใจดี"}
	if got := userFullName(actualItem); got != "นาย สมชาย ใจดี" {
		t.Fatalf("userFullName() actual API shape = %q, want %q", got, "นาย สมชาย ใจดี")
	}
}

func TestDirectoryUserFromActualExternalMapping(t *testing.T) {
	item := map[string]any{
		"user_id":            1001,
		"prefix":             "รศ. ดร.",
		"user_fname":         "งามนิจ",
		"user_lname":         "อาจอินทร์",
		"gender":             "female",
		"email":              "ngamnij@kku.ac.th",
		"tel":                "089-6222316",
		"tel_format":         "08 9622 2316",
		"position_title":     "รองศาสตราจารย์",
		"position_en":        "Associate Professor",
		"prefix_position_en": "Assoc. Prof.",
		"role_id":            1,
		"role_name":          "teacher",
		"is_active":          "A",
		"updated_at":         "2026-07-15T13:52:37+07:00",
	}
	user := directoryUserFromPayload(item, "1001", `{}`, "fingerprint", time.Now())
	if user.UserID != "1001" || user.FirstName != "งามนิจ" || user.LastName != "อาจอินทร์" {
		t.Fatalf("name mapping = %#v", user)
	}
	if user.PositionTitle != "รองศาสตราจารย์" || user.PositionEN != "Associate Professor" || user.PositionPrefixEN != "Assoc. Prof." {
		t.Fatalf("position mapping = %#v", user)
	}
	if user.Phone != "089-6222316" || user.PhoneFormatted != "08 9622 2316" || user.RoleID != 1 || user.RoleName != "teacher" || user.IsActive != "A" {
		t.Fatalf("contact/status mapping = %#v", user)
	}
}

func TestFirstBoolUsesConfiguredFallback(t *testing.T) {
	if got := firstBool(map[string]any{}, true, "active"); !got {
		t.Fatal("firstBool() fallback = false, want true")
	}
	if got := firstBool(map[string]any{"active": "inactive"}, true, "active"); got {
		t.Fatal("firstBool() inactive = true, want false")
	}
	if got := firstBool(map[string]any{"active": json.Number("0")}, true, "active"); got {
		t.Fatal("firstBool() numeric inactive = true, want false")
	}
	if got := firstBool(map[string]any{"active": "A"}, false, "active"); !got {
		t.Fatal("firstBool() API active code = false, want true")
	}
	if got := firstBool(map[string]any{"active": "I"}, true, "active"); got {
		t.Fatal("firstBool() API inactive code = true, want false")
	}
}

func TestNormalizeUsersEndpoint(t *testing.T) {
	if got := normalizeUsersEndpoint("https://fs.computing.kku.ac.th/api/ext/v1/users"); got != "https://fs.computing.kku.ac.th/api/ext/v1/users" {
		t.Fatalf("full endpoint = %q", got)
	}
	if got := normalizeUsersEndpoint("https://fs.computing.kku.ac.th/api/ext/v1/"); got != "https://fs.computing.kku.ac.th/api/ext/v1/users" {
		t.Fatalf("base endpoint = %q", got)
	}
}

func TestNormalizeAPIKey(t *testing.T) {
	if got := normalizeAPIKey("  \"secret-key\"\r\n"); got != "secret-key" {
		t.Fatalf("normalizeAPIKey() = %q, want %q", got, "secret-key")
	}
}

func TestFindDirectoryUserPrefersExactAndSupportsFuzzyName(t *testing.T) {
	users := []DirectoryUser{
		{ID: 7, FullName: "นาย สมชาย ใจดี", JobTitle: "อาจารย์"},
		{ID: 8, FullName: "นางสาว สมหญิง ใจดี", JobTitle: "นักวิชาการ"},
	}
	matched, score, method, ok := findDirectoryUser("นายสมชาย ใจดี", users)
	if !ok || matched.ID != 7 || score != 1 || method != "EXACT" {
		t.Fatalf("exact match = %#v, score=%v, method=%q, ok=%v", matched, score, method, ok)
	}
	matched, score, method, ok = findDirectoryUser("นาย สมชาย ใจด", users)
	if !ok || matched.ID != 7 || method != "FUZZY" || score < 0.78 {
		t.Fatalf("fuzzy match = %#v, score=%v, method=%q, ok=%v", matched, score, method, ok)
	}
}

func TestFindDirectoryUserMatchesFirstAndLastNameWithoutPrefix(t *testing.T) {
	users := []DirectoryUser{{
		FullName:  "นาย สมชาย ใจดี",
		FirstName: "สมชาย",
		LastName:  "ใจดี",
	}}
	matched, score, method, ok := findDirectoryUser("รศ. ดร. สมชาย ใจดี", users)
	if !ok || matched.FirstName != "สมชาย" || score != 1 || method != "EXACT" {
		t.Fatalf("prefix-independent match = %#v, score=%v, method=%q, ok=%v", matched, score, method, ok)
	}
}

func TestExtractCommandMetadataFromThaiOCR(t *testing.T) {
	result := extractCommandMetadata([]workerPage{{
		PageNo:   1,
		FullText: "ที่ 125 /2568\nเรื่อง แต่งตั้งคณะกรรมการจัดเตรียมงาน\n1.1 นายวชิราวุธ ธรรมวิเศษ เป็นกรรมการ",
		Lines:    []workerLine{{Text: "ที่ 125 /2568", Confidence: 0.98}, {Text: "เรื่อง แต่งตั้งคณะกรรมการจัดเตรียมงาน", Confidence: 0.98}, {Text: "1.1 นายวชิราวุธ ธรรมวิเศษ เป็นกรรมการ", Confidence: 0.95}},
	}}, "ที่ 125 /2568\nเรื่อง แต่งตั้งคณะกรรมการจัดเตรียมงาน\n1.1 นายวชิราวุธ ธรรมวิเศษ เป็นกรรมการ")
	if result.orderNo != "125 /2568" || result.orderYearBE != 2568 {
		t.Fatalf("unexpected order metadata: %#v", result)
	}
	if result.title == "" || len(result.appointments) != 1 || result.appointments[0].CommitteeRole != "กรรมการ" {
		t.Fatalf("unexpected extracted metadata: %#v", result)
	}
}

func TestExtractCommandMetadataNormalizesThaiDigits(t *testing.T) {
	result := extractCommandMetadata([]workerPage{{
		PageNo:   1,
		FullText: "ที่ ๑๒๕/๒๕๖๘\nเรื่อง แต่งตั้งคณะกรรมการ",
		Lines:    []workerLine{{Text: "ที่ ๑๒๕/๒๕๖๘", Confidence: 0.98}},
	}}, "ที่ ๑๒๕/๒๕๖๘\nเรื่อง แต่งตั้งคณะกรรมการ")
	if result.orderNo != "125/2568" || result.orderYearBE != 2568 {
		t.Fatalf("Thai digit normalization = %q/%d", result.orderNo, result.orderYearBE)
	}
}

func TestExtractAppointmentsRebuildsTableRowsWithoutPhantomRoles(t *testing.T) {
	page := workerPage{
		PageNo: 2,
		Lines: []workerLine{
			{Text: "1. ฝ่ายวิชาการ", Confidence: 0.98, BoundingBox: []float64{120, 100, 420, 30}},
			{Text: "1.1 รองคณบดี", Confidence: 0.96, BoundingBox: []float64{120, 160, 260, 30}},
			{Text: "นายสมชาย ใจดี", Confidence: 0.96, BoundingBox: []float64{390, 160, 300, 30}},
			{Text: "เป็นกรรมการ", Confidence: 0.96, BoundingBox: []float64{760, 160, 180, 30}},
			{Text: "1.2 นางสาวสมหญิง ใจดี", Confidence: 0.95, BoundingBox: []float64{120, 210, 500, 30}},
			{Text: "เป็นเลขานุการ", Confidence: 0.95, BoundingBox: []float64{760, 210, 180, 30}},
			{Text: "1.3 นายไม่พบในฐานข้อมูล", Confidence: 0.82, BoundingBox: []float64{120, 260, 500, 30}},
			{Text: "หมายเหตุ: โปรดตรวจสอบรายชื่อ", Confidence: 0.99, BoundingBox: []float64{120, 280, 500, 30}},
		},
	}
	result := extractCommandMetadata([]workerPage{page}, "")
	if len(result.appointments) != 3 {
		t.Fatalf("appointments = %d, want 3: %#v", len(result.appointments), result.appointments)
	}
	if result.appointments[0].FullName != "นายสมชาย ใจดี" || result.appointments[0].CommitteeRole != "กรรมการ" {
		t.Fatalf("first appointment = %#v", result.appointments[0])
	}
	if result.appointments[1].FullName != "นางสาวสมหญิง ใจดี" || result.appointments[1].CommitteeRole != "เลขานุการ" {
		t.Fatalf("second appointment = %#v", result.appointments[1])
	}
	if result.appointments[2].CommitteeRole != "กรรมการ" {
		t.Fatalf("role fallback = %#v", result.appointments[2])
	}
}

func TestExtractAppointmentsJoinsFixedSampleRowColumns(t *testing.T) {
	page := workerPage{PageNo: 1, Lines: []workerLine{
		{Text: "1.1", Confidence: 0.98, BoundingBox: []float64{100, 200, 20, 10}},
		{Text: "รองคณบดีฝ่ายวิชาการ", Confidence: 0.96, BoundingBox: []float64{130, 215, 250, 10}},
		{Text: "เป็นรองประธานกรรมการ", Confidence: 0.96, BoundingBox: []float64{600, 217, 180, 10}},
	}}
	result := extractCommandMetadata([]workerPage{page}, "")
	if len(result.appointments) != 1 {
		t.Fatalf("appointments = %d, want 1: %#v", len(result.appointments), result.appointments)
	}
	if result.appointments[0].CommitteeRole != "รองประธานกรรมการ" {
		t.Fatalf("role = %q, want รองประธานกรรมการ", result.appointments[0].CommitteeRole)
	}
}

func TestFixedSampleProfileKeepsOnlyTableRegion(t *testing.T) {
	page := workerPage{PageNo: 1, ImageWidth: 1000, ImageHeight: 1000, Lines: []workerLine{
		{Text: "1. บรรทัดหัวกระดาษ", Confidence: 0.99, BoundingBox: []float64{100, 120, 300, 20}},
		{Text: "1.1", Confidence: 0.98, BoundingBox: []float64{120, 520, 30, 12}},
		{Text: "นายทดสอบ ตำแหน่ง", Confidence: 0.96, BoundingBox: []float64{160, 532, 300, 12}},
		{Text: "เป็นกรรมการ", Confidence: 0.96, BoundingBox: []float64{650, 534, 150, 12}},
	}}
	result := extractCommandMetadataForDocument([]workerPage{page}, "", "kku_2.pdf")
	if len(result.appointments) != 1 || result.appointments[0].FullName == "" {
		t.Fatalf("fixed profile appointments = %#v, want one table member", result.appointments)
	}
}
