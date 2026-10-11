package main

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fakeReprocessStore struct {
	err      error
	calledID uint
	actor    string
}

func (f *fakeReprocessStore) RequestReprocess(id uint, actor string, now time.Time) error {
	f.calledID = id
	f.actor = actor
	return f.err
}

func TestReprocessHandlerAcceptedEnqueuesDocument(t *testing.T) {
	store := &fakeReprocessStore{}
	var enqueued []uint
	h := reprocessDocumentHandler(store, func(id uint) { enqueued = append(enqueued, id) })
	rec := performRequest(t, http.MethodPost, "/api/v1/documents/:id/reprocess", "/api/v1/documents/12/reprocess", nil, fakeRole("ADMIN"), h)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(enqueued) != 1 || enqueued[0] != 12 || store.calledID != 12 || store.actor != "tester" {
		t.Fatalf("enqueued=%v store=%+v", enqueued, store)
	}
}

func TestReprocessHandlerStatusMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", errDocumentNotFound, http.StatusNotFound},
		{"busy", errDocumentBusy, http.StatusConflict},
		{"cancelled", errDocumentCancelled, http.StatusConflict},
		{"unexpected", errors.New("db"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			h := reprocessDocumentHandler(&fakeReprocessStore{err: c.err}, func(uint) {
				t.Fatal("must not enqueue on error")
			})
			rec := performRequest(t, http.MethodPost, "/api/v1/documents/:id/reprocess", "/api/v1/documents/3/reprocess", nil, fakeRole("ADMIN"), h)
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d", rec.Code, c.want)
			}
		})
	}
}

func TestReprocessHandlerRejectsBadID(t *testing.T) {
	h := reprocessDocumentHandler(&fakeReprocessStore{}, func(uint) {})
	rec := performRequest(t, http.MethodPost, "/api/v1/documents/:id/reprocess", "/api/v1/documents/abc/reprocess", nil, fakeRole("ADMIN"), h)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestReprocessRouteRejectsViewerAndAllowsStaff(t *testing.T) {
	route := "/api/v1/documents/:id/reprocess"
	handler := func() func(uint) { return func(uint) {} }
	viewer := performRequest(t, http.MethodPost, route, "/api/v1/documents/1/reprocess", nil,
		fakeRole("VIEWER"), requireRole("ADMIN", "STAFF"), reprocessDocumentHandler(&fakeReprocessStore{}, handler()))
	if viewer.Code != http.StatusForbidden {
		t.Fatalf("VIEWER status = %d, want 403", viewer.Code)
	}
	staff := performRequest(t, http.MethodPost, route, "/api/v1/documents/1/reprocess", nil,
		fakeRole("STAFF"), requireRole("ADMIN", "STAFF"), reprocessDocumentHandler(&fakeReprocessStore{}, handler()))
	if staff.Code != http.StatusAccepted {
		t.Fatalf("STAFF status = %d, want 202", staff.Code)
	}
}

func TestReprocessHandlerRefusalMessages(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		message string
	}{
		{"cancelled", errDocumentCancelled, "เอกสารถูกยกเลิกแล้ว ไม่สามารถประมวลผลใหม่ได้"},
		{"busy", errDocumentBusy, "เอกสารกำลังประมวลผลอยู่ กรุณารอให้เสร็จก่อนสแกนใหม่"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			h := reprocessDocumentHandler(&fakeReprocessStore{err: c.err}, func(uint) {
				t.Fatal("must not enqueue a refused document")
			})
			rec := performRequest(t, http.MethodPost, "/api/v1/documents/:id/reprocess", "/api/v1/documents/3/reprocess", nil, fakeRole("STAFF"), h)
			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), c.message) {
				t.Fatalf("body = %s, want message %q", rec.Body.String(), c.message)
			}
		})
	}
}

func TestReprocessRouteAllowsFinishedDocumentForAdminAndStaff(t *testing.T) {
	route := "/api/v1/documents/:id/reprocess"
	for _, role := range []string{"ADMIN", "STAFF"} {
		var enqueued []uint
		rec := performRequest(t, http.MethodPost, route, "/api/v1/documents/7/reprocess", nil,
			fakeRole(role), requireRole("ADMIN", "STAFF"), reprocessDocumentHandler(&fakeReprocessStore{}, func(id uint) { enqueued = append(enqueued, id) }))
		if rec.Code != http.StatusAccepted || len(enqueued) != 1 || enqueued[0] != 7 {
			t.Fatalf("%s: status = %d enqueued = %v", role, rec.Code, enqueued)
		}
	}
	viewer := performRequest(t, http.MethodPost, route, "/api/v1/documents/7/reprocess", nil,
		fakeRole("VIEWER"), requireRole("ADMIN", "STAFF"), reprocessDocumentHandler(&fakeReprocessStore{}, func(uint) {
			t.Fatal("viewer must not enqueue")
		}))
	if viewer.Code != http.StatusForbidden {
		t.Fatalf("VIEWER status = %d, want 403", viewer.Code)
	}
}

func TestRescanResetUpdatesClearsConfirmationAndRetryState(t *testing.T) {
	u := rescanResetUpdates()
	if u["status"] != "PROCESSING" || u["processing_stage"] != "QUEUED" || u["ocr_error"] != "" {
		t.Fatalf("status/stage/error not reset: %v", u)
	}
	if u["confirmed_by"] != "" {
		t.Fatalf("confirmed_by = %v, want empty", u["confirmed_by"])
	}
	if v, ok := u["confirmed_at"]; !ok || v != nil {
		t.Fatalf("confirmed_at = %v (present=%t), want nil", v, ok)
	}
	if u["extraction_attempts"] != 0 || u["extraction_last_status"] != 0 {
		t.Fatalf("retry counters not reset: %v", u)
	}
	if v, ok := u["next_attempt_at"]; !ok || v != nil {
		t.Fatalf("next_attempt_at = %v (present=%t), want nil", v, ok)
	}
}
