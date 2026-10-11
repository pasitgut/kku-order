package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakeAuditStore struct {
	rows       []auditLogRecord
	total      int64
	gotFilter  auditLogFilter
	gotOffset  int
	gotLimit   int
	exportRows int
	names      map[string]string
	err        error
}

func (f *fakeAuditStore) FindAuditLogs(filter auditLogFilter, offset, limit int) ([]auditLogRecord, int64, error) {
	f.gotFilter, f.gotOffset, f.gotLimit = filter, offset, limit
	return f.rows, f.total, f.err
}

func (f *fakeAuditStore) ExportAuditLogs(filter auditLogFilter, limit int) ([]auditLogRecord, error) {
	f.gotFilter, f.gotLimit = filter, limit
	return f.rows, f.err
}

func (f *fakeAuditStore) DirectoryNames(userIDs []string) map[string]string {
	return f.names
}

func auditTestRouter(store auditLogStore) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(authMiddleware())
	roles := requireRole("ADMIN", "DEVELOPER")
	router.GET("/api/v1/audit-logs", roles, listAuditLogsHandler(store))
	router.GET("/api/v1/audit-logs/actions", roles, auditActionsHandler())
	router.GET("/api/v1/audit-logs/export.csv", roles, exportAuditLogsHandler(store))
	return router
}

func getAs(router http.Handler, path, role string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("X-Auth-Request-User", "admin@kku.ac.th")
	request.Header.Set("X-Auth-Request-Role", role)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestAuditRolesAreAdminAndDeveloperOnly(t *testing.T) {
	router := auditTestRouter(&fakeAuditStore{})
	for _, role := range []string{"STAFF", "VIEWER"} {
		for _, path := range []string{"/api/v1/audit-logs", "/api/v1/audit-logs/actions", "/api/v1/audit-logs/export.csv"} {
			if response := getAs(router, path, role); response.Code != http.StatusForbidden {
				t.Errorf("%s %s status = %d, want 403", role, path, response.Code)
			}
		}
	}
	for _, role := range []string{"ADMIN", "DEVELOPER"} {
		if response := getAs(router, "/api/v1/audit-logs", role); response.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200", role, response.Code)
		}
	}
}

func TestAuditListRejectsBadParameters(t *testing.T) {
	router := auditTestRouter(&fakeAuditStore{})
	cases := []string{
		"from=2026-13-01",
		"to=11/10/2026",
		"from=2026-10-12&to=2026-10-11",
		"pageSize=0",
		"pageSize=201",
		"pageSize=abc",
		"page=0",
		"documentId=-3",
		"action=NOT_A_REAL_ACTION",
		"q=" + strings.Repeat("ก", 201),
	}
	for _, query := range cases {
		if response := getAs(router, "/api/v1/audit-logs?"+query, "ADMIN"); response.Code != http.StatusBadRequest {
			t.Errorf("?%s status = %d, want 400", query, response.Code)
		}
	}
}

func TestAuditListDefaultsAndPassesFilters(t *testing.T) {
	store := &fakeAuditStore{}
	router := auditTestRouter(store)
	response := getAs(router, "/api/v1/audit-logs?action=DOCUMENT_UPDATED,DOCUMENT_CONFIRMED&action=DOCUMENT_DELETED&documentId=7&userId=kit&q=ตรวจ&from=2026-10-11&to=2026-10-11", "ADMIN")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if store.gotOffset != 0 || store.gotLimit != 50 {
		t.Fatalf("offset=%d limit=%d, want 0 and 50", store.gotOffset, store.gotLimit)
	}
	filter := store.gotFilter
	if len(filter.Actions) != 3 || filter.DocumentID != 7 || filter.UserID != "kit" || filter.Search != "ตรวจ" {
		t.Fatalf("filter = %+v", filter)
	}
	if filter.From == nil || !filter.From.Equal(time.Date(2026, 10, 10, 17, 0, 0, 0, time.UTC)) {
		t.Fatalf("from = %v, want Bangkok midnight of 2026-10-11", filter.From)
	}
	if filter.To == nil || !filter.To.Equal(time.Date(2026, 10, 11, 17, 0, 0, 0, time.UTC)) {
		t.Fatalf("to = %v, want exclusive end of 2026-10-11 in Bangkok", filter.To)
	}
}

func TestAuditListShapesRowsWithTitlesAndNames(t *testing.T) {
	docID := uint(9)
	created := time.Date(2026, 10, 11, 3, 0, 0, 0, time.UTC)
	store := &fakeAuditStore{
		total: 3,
		rows: []auditLogRecord{
			{ID: 3, CreatedAt: created, UserID: "kit", Action: "DOCUMENT_UPDATED", DocumentID: &docID, DocumentTitle: "คำสั่งแต่งตั้ง", DocumentExists: true, Details: "title: a → b"},
			{ID: 2, CreatedAt: created, UserID: "kit", Action: "DOCUMENT_DELETED", Details: `{"title":"คำสั่งที่ถูกลบ","originalFilename":"a.pdf"}`},
			{ID: 1, CreatedAt: created, UserID: "system:folder-import", Action: "DOCUMENT_FOLDER_IMPORTED", DocumentID: &docID, Details: "a.pdf"},
		},
		names: map[string]string{"kit": "กิตติ ทดสอบ"},
	}
	response := getAs(auditTestRouter(store), "/api/v1/audit-logs?page=2&pageSize=2", "ADMIN")
	var body struct {
		Data []struct {
			ID             uint   `json:"id"`
			UserName       string `json:"userName"`
			ActionLabel    string `json:"actionLabel"`
			DocumentID     *uint  `json:"documentId"`
			DocumentTitle  string `json:"documentTitle"`
			DocumentExists bool   `json:"documentExists"`
		} `json:"data"`
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"pageSize"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if store.gotOffset != 2 || store.gotLimit != 2 {
		t.Fatalf("offset=%d limit=%d, want 2 and 2", store.gotOffset, store.gotLimit)
	}
	if body.Total != 3 || body.Page != 2 || body.PageSize != 2 || len(body.Data) != 3 {
		t.Fatalf("envelope = %+v", body)
	}
	updated := body.Data[0]
	if updated.UserName != "กิตติ ทดสอบ" || updated.ActionLabel != "แก้ไข" || updated.DocumentTitle != "คำสั่งแต่งตั้ง" || !updated.DocumentExists {
		t.Fatalf("updated row = %+v", updated)
	}
	deleted := body.Data[1]
	if deleted.DocumentTitle != "คำสั่งที่ถูกลบ" || deleted.DocumentExists || deleted.DocumentID != nil || deleted.UserName != "กิตติ ทดสอบ" {
		t.Fatalf("deleted row = %+v", deleted)
	}
	imported := body.Data[2]
	if imported.UserName != "" || imported.DocumentExists {
		t.Fatalf("system row = %+v", imported)
	}
}

func TestAuditActionsEndpointListsEveryLabel(t *testing.T) {
	response := getAs(auditTestRouter(&fakeAuditStore{}), "/api/v1/audit-logs/actions", "ADMIN")
	var body struct {
		Data []struct {
			Action string `json:"action"`
			Label  string `json:"label"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != len(activityLabels) {
		t.Fatalf("got %d actions, want %d", len(body.Data), len(activityLabels))
	}
	for _, item := range body.Data {
		if item.Label != activityLabels[item.Action] || item.Label == "" {
			t.Fatalf("action %s label = %q", item.Action, item.Label)
		}
	}
}

func TestAuditExportWritesBOMAndCSVRows(t *testing.T) {
	docID := uint(4)
	store := &fakeAuditStore{rows: []auditLogRecord{
		{ID: 5, CreatedAt: time.Date(2026, 10, 11, 4, 30, 0, 0, time.UTC), UserID: "kit", Action: "DOCUMENT_CONFIRMED", DocumentID: &docID, DocumentTitle: "คำสั่ง, ฉบับที่ 2", DocumentExists: true, Details: "เจ้าหน้าที่ตรวจสอบ\nแล้ว"},
	}}
	response := getAs(auditTestRouter(store), "/api/v1/audit-logs/export.csv?q=ยืนยัน", "DEVELOPER")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/csv") {
		t.Fatalf("content type = %q", got)
	}
	if !strings.HasPrefix(response.Body.String(), "\xEF\xBB\xBF") {
		t.Fatal("export must start with a UTF-8 BOM so Excel shows Thai text")
	}
	if store.gotLimit != auditExportMaxRows || store.gotFilter.Search != "ยืนยัน" {
		t.Fatalf("limit=%d filter=%+v", store.gotLimit, store.gotFilter)
	}
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(response.Body.Bytes(), []byte("\xEF\xBB\xBF")))).ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	if len(records) != 2 || records[0][0] != "เวลา" || records[1][3] != "ยืนยัน" || records[1][4] != "คำสั่ง, ฉบับที่ 2" || records[1][5] != "เจ้าหน้าที่ตรวจสอบ\nแล้ว" {
		t.Fatalf("records = %q", records)
	}
}

func TestAuditExportReportsStoreFailure(t *testing.T) {
	store := &fakeAuditStore{err: errors.New("db down")}
	if response := getAs(auditTestRouter(store), "/api/v1/audit-logs/export.csv", "ADMIN"); response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
}

func TestAuditDocumentTitleFallsBackToDeletedTitle(t *testing.T) {
	deleted := auditLogRecord{Action: "DOCUMENT_DELETED", Details: `{"title":"ก"}`}
	if got := auditDocumentTitle(deleted); got != "ก" {
		t.Fatalf("title = %q", got)
	}
	if got := auditDocumentTitle(auditLogRecord{Action: "DOCUMENT_UPDATED", Details: "x"}); got != "" {
		t.Fatalf("title = %q, want empty", got)
	}
	if got := auditDocumentTitle(auditLogRecord{Action: "DOCUMENT_DELETED", Details: "not json"}); got != "" {
		t.Fatalf("title = %q, want empty for unreadable details", got)
	}
}
