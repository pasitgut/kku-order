package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMatchDirectoryUserPrefersStoredAlias(t *testing.T) {
	users := []DirectoryUser{
		{ID: 7, FirstName: "สมชาย", LastName: "ใจดี"},
		{ID: 9, FirstName: "สมชาย", LastName: "ใจงาม"},
	}
	aliases := map[string]uint{normalizePersonName("นายสมชาย ใจงม"): 9}

	match := matchDirectoryUser("นายสมชาย ใจงม", users, aliases)

	if !match.Matched || match.Method != matchMethodAlias || match.User.ID != 9 || match.Score != 1 {
		t.Fatalf("alias match = %+v, want user 9 via alias with score 1", match)
	}
}

func TestMatchDirectoryUserReportsAmbiguousWhenTwoPeopleShareAName(t *testing.T) {
	users := []DirectoryUser{
		{ID: 3, FirstName: "สมชาย", LastName: "ใจดี", PositionTitle: "อาจารย์"},
		{ID: 4, FirstName: "สมชาย", LastName: "ใจดี", PositionTitle: "นักวิชาการ"},
	}

	match := matchDirectoryUser("นายสมชาย ใจดี", users, nil)

	if match.Matched || match.Method != matchMethodAmbiguous {
		t.Fatalf("match = %+v, want unmatched AMBIGUOUS", match)
	}
	if len(match.Candidates) != 2 {
		t.Fatalf("candidates = %d, want both people offered to the reviewer", len(match.Candidates))
	}
}

func TestMatchDirectoryUserReportsEmptyDirectorySeparatelyFromNotFound(t *testing.T) {
	match := matchDirectoryUser("นายสมชาย ใจดี", nil, nil)

	if match.Matched || match.Method != matchMethodEmpty {
		t.Fatalf("match = %+v, want DIRECTORY_EMPTY", match)
	}
}

func TestMatchDirectoryUserRejectsDivergentSurnameAndOffersCandidates(t *testing.T) {
	users := []DirectoryUser{{ID: 11, FirstName: "วชิราวุธ", LastName: "ธรรมวิเศษ", PositionTitle: "อาจารย์"}}

	match := matchDirectoryUser("นายวชิราวุธ ธรรมวิเชียร", users, nil)

	if match.Matched {
		t.Fatalf("match = %+v, want no automatic match: ธรรมวิเชียร and ธรรมวิเศษ are different people", match)
	}
	if match.Method != matchMethodNotFound {
		t.Fatalf("method = %q, want NOT_FOUND", match.Method)
	}
	if len(match.Candidates) != 1 || match.Candidates[0].UserID != 11 {
		t.Fatalf("candidates = %+v, want the near miss offered to the reviewer", match.Candidates)
	}
}

func TestMatchDirectoryUserStillAcceptsOneCharacterSurnameSlip(t *testing.T) {
	users := []DirectoryUser{{ID: 7, FirstName: "สมชาย", LastName: "ใจดี"}}

	match := matchDirectoryUser("นาย สมชาย ใจด", users, nil)

	if !match.Matched || match.Method != matchMethodFuzzy || match.User.ID != 7 {
		t.Fatalf("match = %+v, want a fuzzy match on a one-character OCR slip", match)
	}
}

func TestResolveAppointmentKeepsReviewerChoiceInsteadOfRematching(t *testing.T) {
	users := []DirectoryUser{{ID: 7, FirstName: "สมชาย", LastName: "ใจดี"}}
	chosen := uint(7)
	input := appointmentRequest{FullName: "ชอ ที่ OCR อ่านเพี้ยนจนจำไม่ได้", DirectoryUserID: &chosen}

	match := resolveAppointmentDirectoryUser(input, users, nil)

	if !match.Matched || match.Method != matchMethodManual || match.User.ID != 7 {
		t.Fatalf("match = %+v, want the reviewer's own choice kept verbatim", match)
	}
}

func TestResolveAppointmentFallsBackToMatchingWhenReviewerChoseNobody(t *testing.T) {
	users := []DirectoryUser{{ID: 7, FirstName: "สมชาย", LastName: "ใจดี"}}
	input := appointmentRequest{FullName: "นายสมชาย ใจดี"}

	match := resolveAppointmentDirectoryUser(input, users, nil)

	if !match.Matched || match.Method != matchMethodExact || match.User.ID != 7 {
		t.Fatalf("match = %+v, want the normal matching path", match)
	}
}

func TestConfirmBlockersNamesEveryAppointmentWithoutAPerson(t *testing.T) {
	linked := uint(7)
	appointments := []Appointment{
		{FullName: "นายสมชาย ใจดี", DirectoryUserID: &linked},
		{FullName: "นายไม่พบ ในระบบ"},
		{FullName: "นางสาวยังไม่ผูก ชื่อสกุล"},
	}

	blockers := confirmBlockers(appointments)

	if len(blockers) != 2 || blockers[0] != "นายไม่พบ ในระบบ" || blockers[1] != "นางสาวยังไม่ผูก ชื่อสกุล" {
		t.Fatalf("blockers = %#v, want the two unlinked rows in order", blockers)
	}
}

func TestCandidatesToOfferGivesAnUnlinkedRowSomethingToClick(t *testing.T) {
	// A row can be unlinked while its name still resolves: the person was
	// added to the directory after the document was processed.
	match := directoryMatch{
		User:    DirectoryUser{ID: 1, FirstName: "สมชาย", LastName: "ใจดี", PositionTitle: "อาจารย์"},
		Score:   1,
		Method:  matchMethodExact,
		Matched: true,
	}

	offered := candidatesToOffer(match)

	if len(offered) != 1 || offered[0].UserID != 1 || offered[0].Score != 1 {
		t.Fatalf("offered = %+v, want the matched person offered as a one-click choice", offered)
	}
}

func TestCandidatesToOfferPassesThroughNearMissesWhenNothingMatched(t *testing.T) {
	match := directoryMatch{
		Method:     matchMethodNotFound,
		Candidates: []directoryCandidate{{UserID: 11, FullName: "วชิราวุธ ธรรมวิเศษ", Score: 0.78}},
	}

	offered := candidatesToOffer(match)

	if len(offered) != 1 || offered[0].UserID != 11 {
		t.Fatalf("offered = %+v, want the near miss kept", offered)
	}
}

func TestNewManualDirectoryUserStoresValidJSONPayload(t *testing.T) {
	// raw_payload is a MySQL JSON column: an empty string is rejected outright.
	user, err := newManualDirectoryUser(createDirectoryUserRequest{FirstName: "บุษบา", LastName: "จากคณะอื่น", PositionTitle: "อาจารย์"}, "staff@kku.ac.th", time.Now())
	if err != nil {
		t.Fatalf("newManualDirectoryUser() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(user.RawPayload), &decoded); err != nil {
		t.Fatalf("raw_payload = %q is not valid JSON: %v", user.RawPayload, err)
	}
	if user.Source != sourceManual || user.IsActive != activeUserCode || user.CreatedBy != "staff@kku.ac.th" {
		t.Fatalf("user = %+v, want a manual, active row attributed to the reviewer", user)
	}
}

func TestNormalizeBoundingBoxAlwaysProducesValidJSON(t *testing.T) {
	// bounding_box_json is a MySQL JSON column: "" is rejected, so an
	// appointment with no box on screen must still store a JSON document.
	for _, input := range []string{"", "   ", "not json", "{"} {
		if got := normalizeBoundingBox(input); got != "[]" {
			t.Fatalf("normalizeBoundingBox(%q) = %q, want %q", input, got, "[]")
		}
	}
	if got := normalizeBoundingBox("[120,160,260,30]"); got != "[120,160,260,30]" {
		t.Fatalf("normalizeBoundingBox() dropped a real box: %q", got)
	}
}

func TestReplacedFileUpdatesClearsEverythingDerivedFromTheOldFile(t *testing.T) {
	now := time.Now()
	updates := replacedFileUpdates(replacementFile{
		Title: "คำสั่งฉบับแก้ไข", Filename: "order-v2.pdf", StoragePath: "/storage/2-order-v2.pdf",
		SourceType: "pdf", MimeType: "application/pdf", SizeBytes: 2048, Hash: "abc123",
	}, "staff@kku.ac.th", now)

	if updates["status"] != "PROCESSING" || updates["processing_stage"] != "QUEUED" {
		t.Fatalf("status = %v/%v, want the document back in the queue", updates["status"], updates["processing_stage"])
	}
	// A confirmation belongs to the file that was reviewed, never to a new one.
	if updates["confirmed_by"] != "" || updates["confirmed_at"] != nil {
		t.Fatalf("confirmation = %v/%v, want it dropped", updates["confirmed_by"], updates["confirmed_at"])
	}
	for _, key := range []string{"ocr_text", "ocr_error"} {
		if updates[key] != "" {
			t.Fatalf("%s = %v, want cleared", key, updates[key])
		}
	}
	for _, key := range []string{"page_count", "person_count"} {
		if updates[key] != 0 {
			t.Fatalf("%s = %v, want reset", key, updates[key])
		}
	}
	if updates["file_hash"] != "abc123" || updates["original_filename"] != "order-v2.pdf" || updates["storage_path"] != "/storage/2-order-v2.pdf" {
		t.Fatalf("updates = %#v, want the new file's identity", updates)
	}
	if updates["imported_by"] != "staff@kku.ac.th" {
		t.Fatalf("imported_by = %v, want the reviewer who replaced it", updates["imported_by"])
	}
	// Metadata read out of the old file must not survive either: if the new
	// file fails OCR, a stale order number would be read as the new one's.
	for _, key := range []string{"order_type", "order_no", "committee", "signer_name", "signer_position", "responsibilities", "additional_refs"} {
		if updates[key] != "" {
			t.Fatalf("%s = %v, want cleared with the old file", key, updates[key])
		}
	}
	if updates["order_year_be"] != 0 {
		t.Fatalf("order_year_be = %v, want cleared", updates["order_year_be"])
	}
	for _, key := range []string{"issued_date", "effective_date", "expiry_date"} {
		if updates[key] != nil {
			t.Fatalf("%s = %v, want cleared", key, updates[key])
		}
	}
}
