package main

import (
	"os"
	"testing"
)

func loadThaidocFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/extraction/thaidoc-extract-json.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return raw
}

func TestParseThaidocResponseMapsHeaderAndBoxes(t *testing.T) {
	got, err := parseThaidocResponse(loadThaidocFixture(t))
	if err != nil {
		t.Fatalf("parseThaidocResponse: %v", err)
	}
	if got.Engine != "thaidoc" {
		t.Fatalf("Engine = %q, want thaidoc", got.Engine)
	}
	if got.Header.Subject.Text != "ระบบจัดการเอกสารคำสั่งแต่งตั้ง" {
		t.Fatalf("Subject.Text = %q", got.Header.Subject.Text)
	}
	if got.Header.Subject.Confidence != nil {
		t.Fatalf("Subject.Confidence = %v, want nil", *got.Header.Subject.Confidence)
	}
	box := got.Header.Subject.Boxes
	if len(box) != 1 || box[0].Page != 1 || box[0].X0 != 0.4171 || box[0].Y1 != 0.1929 {
		t.Fatalf("Subject.Boxes = %+v", box)
	}
	if got.Header.IssuedBy.Text != "ข้อกำหนดและขอบเขตของงาน" {
		t.Fatalf("IssuedBy.Text = %q (order -> IssuedBy)", got.Header.IssuedBy.Text)
	}
	if got.Header.Signer.Text != "ตัวพิมพ์" {
		t.Fatalf("Signer.Text = %q", got.Header.Signer.Text)
	}
	if got.Header.OrderNo.Text != "" {
		t.Fatalf("OrderNo.Text = %q, want empty (number is empty in fixture)", got.Header.OrderNo.Text)
	}
}

func TestParseThaidocResponseMapsGroupsAndMembers(t *testing.T) {
	got, err := parseThaidocResponse(loadThaidocFixture(t))
	if err != nil {
		t.Fatalf("parseThaidocResponse: %v", err)
	}
	if len(got.Groups) != 6 {
		t.Fatalf("len(Groups) = %d, want 6", len(got.Groups))
	}
	total := 0
	for _, g := range got.Groups {
		total += len(g.Members)
	}
	if total != 6 {
		t.Fatalf("total members = %d, want 6", total)
	}
	first := got.Groups[0].Members[0]
	if first.Name.Text != "วัชรวิศว์ ชัยปลื้ม" {
		t.Fatalf("first member name = %q", first.Name.Text)
	}
	if len(first.Name.Boxes) != 1 || first.Name.Boxes[0].X0 != 0.1903 {
		t.Fatalf("first member name boxes = %+v", first.Name.Boxes)
	}
	if first.Name.Confidence != nil || first.ExOfficio {
		t.Fatalf("first member: confidence/exOfficio = %v/%v", first.Name.Confidence, first.ExOfficio)
	}
}

func TestParseThaidocResponseMarksExOfficioWhenNameEmpty(t *testing.T) {
	got, err := parseThaidocResponse(loadThaidocFixture(t))
	if err != nil {
		t.Fatalf("parseThaidocResponse: %v", err)
	}
	found := false
	for _, g := range got.Groups {
		for _, m := range g.Members {
			if m.Name.Text == "" && m.Position.Text != "" {
				found = true
				if !m.ExOfficio {
					t.Fatalf("member with empty name and position %q not ExOfficio", m.Position.Text)
				}
			}
		}
	}
	if !found {
		t.Fatal("fixture has no ex-officio member; check fixture facts")
	}
}

func TestParseThaidocResponseMapsPageSize(t *testing.T) {
	got, err := parseThaidocResponse(loadThaidocFixture(t))
	if err != nil {
		t.Fatalf("parseThaidocResponse: %v", err)
	}
	if len(got.Pages) != 14 {
		t.Fatalf("len(Pages) = %d, want 14", len(got.Pages))
	}
	if got.Pages[0].Width != 1654 || got.Pages[0].Height != 2339 {
		t.Fatalf("page 1 size = %dx%d", got.Pages[0].Width, got.Pages[0].Height)
	}
}

func TestParseThaidocResponseRequiresDataKey(t *testing.T) {
	_, err := parseThaidocResponse([]byte(`{"subject":"x","pages":[]}`))
	if err == nil {
		t.Fatal("expected error when data key is missing")
	}
}
