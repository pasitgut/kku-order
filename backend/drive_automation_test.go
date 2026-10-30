package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func TestDriveDailySchedule(t *testing.T) {
	settings := defaultOCRSettings()
	cases := []struct{ name, now, want string }{
		{"before five", "2026-10-11T04:59:00+07:00", "2026-10-10T05:00:00+07:00"},
		{"exactly five", "2026-10-11T05:00:00+07:00", "2026-10-11T05:00:00+07:00"},
		{"restart after five", "2026-10-11T12:00:00+07:00", "2026-10-11T05:00:00+07:00"},
		{"UTC host", "2026-10-10T22:00:00Z", "2026-10-11T05:00:00+07:00"},
		{"year boundary", "2027-01-01T04:00:00+07:00", "2026-12-31T05:00:00+07:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now, _ := time.Parse(time.RFC3339, tc.now)
			want, _ := time.Parse(time.RFC3339, tc.want)
			if got := driveScheduledOccurrence(now, settings); !got.Equal(want) {
				t.Fatalf("due %v, want %v", got, want)
			}
		})
	}
	settings.DriveTimezone = "America/New_York"
	settings.DriveSyncTime = "05:30"
	now, _ := time.Parse(time.RFC3339, "2026-03-08T04:00:00-04:00")
	want, _ := time.Parse(time.RFC3339, "2026-03-07T05:30:00-05:00")
	if got := driveScheduledOccurrence(now, settings); !got.Equal(want) {
		t.Fatalf("DST boundary: got %v, want %v", got, want)
	}
}

func TestDriveFolderLinks(t *testing.T) {
	for _, value := range []string{" abc-123_ ", "https://drive.google.com/drive/folders/abc-123_?usp=sharing&resourcekey=key", "https://drive.google.com/drive/u/1/folders/abc-123_", "https://drive.google.com/open?id=abc-123_"} {
		if got := normalizeDriveFolderID(value); got != "abc-123_" {
			t.Fatalf("normalize %q: %q", value, got)
		}
	}
	for _, value := range []string{"", "https://drive.google.com/file/d/abc/view", "https://drive.google.com/", "https://drive.google.com.evil.test/drive/folders/abc", "https://example.test/?id=abc", "https://drive.google.com/drive/folders/abc%27", "' or trashed=false"} {
		if got := normalizeDriveFolderID(value); got != "" {
			t.Fatalf("invalid folder link accepted: %q -> %q", value, got)
		}
	}
}

func TestDriveSettingsRejectInvalidSchedule(t *testing.T) {
	for _, clock := range []string{"", "5:00", "25:00", "05:60", "05:00:00"} {
		settings := defaultOCRSettings()
		settings.DriveSyncTime = clock
		if settings.Validate() == nil {
			t.Fatalf("accepted invalid clock %q", clock)
		}
	}
	settings := defaultOCRSettings()
	settings.DriveTimezone = "No/Such_Zone"
	if settings.Validate() == nil {
		t.Fatal("accepted unknown timezone")
	}
	merged, err := mergeOCRSettings(defaultOCRSettings(), `{"driveFolderUrl":"https://drive.google.com/drive/folders/abc","importSource":"drive"}`)
	if err != nil || merged.DriveSyncTime != "05:00" || merged.DriveTimezone != "Asia/Bangkok" {
		t.Fatalf("legacy settings: %+v, %v", merged, err)
	}
}

func TestDriveCredentialsRequireUserOAuth(t *testing.T) {
	for _, value := range []string{`{}`, `{invalid`, `{"type":"authorized_user"}`, `{"type":"service_account","client_email":"reader@project.iam.gserviceaccount.com","private_key":"key"}`, `{"type":"authorized_user","client_id":"client"}`} {
		if err := validateDriveCredentials([]byte(value)); err == nil {
			t.Fatalf("accepted invalid credentials %s", value)
		}
	}
	value := []byte(`{"type":"authorized_user","client_id":"client-id","client_secret":"client-secret","refresh_token":"refresh-token"}`)
	if err := validateDriveCredentials(value); err != nil {
		t.Fatalf("valid user OAuth credentials rejected: %v", err)
	}
}

func TestDrivePermanentErrorsDoNotRetry(t *testing.T) {
	for _, code := range []int{400, 401, 403, 404} {
		calls := 0
		_, err := driveRetry(context.Background(), func() (bool, error) { calls++; return false, &googleapi.Error{Code: code} })
		if err == nil || calls != 1 {
			t.Fatalf("HTTP %d: calls=%d, err=%v", code, calls, err)
		}
	}
	for _, code := range []int{429, 500, 502, 503} {
		if !driveRetryable(&googleapi.Error{Code: code}) {
			t.Fatalf("HTTP %d must retry", code)
		}
	}
	if !driveRetryable(&googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "userRateLimitExceeded"}}}) {
		t.Fatal("quota 403 must retry")
	}
}

func TestDriveRetryHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, err := driveRetry(ctx, func() (bool, error) { calls++; cancel(); return false, &googleapi.Error{Code: 429} })
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestDriveConnectionAndDownloadsUseResourceKeys(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("X-Goog-Drive-Resource-Keys") != "root/key" {
			t.Errorf("missing resource key: %v", r.Header)
		}
		if strings.HasSuffix(r.URL.Path, "/export") {
			if r.URL.Query().Get("mimeType") != "application/pdf" {
				t.Error("wrong export type")
			}
			_, _ = io.WriteString(w, "%PDF-1.7\nexport")
			return
		}
		if r.URL.Query().Get("alt") == "media" {
			if r.URL.Query().Get("supportsAllDrives") != "true" {
				t.Error("download missing shared Drive support")
			}
			_, _ = io.WriteString(w, "%PDF-1.7\ndownload")
			return
		}
		if strings.HasSuffix(r.URL.Path, "/files") {
			if r.URL.Query().Get("q") != "'root' in parents and trashed = false" {
				t.Error("wrong folder query")
			}
			if r.URL.Query().Get("includeItemsFromAllDrives") != "true" {
				t.Error("list missing shared Drive support")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"files": []any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "root", "name": "Shared root", "mimeType": driveFolderMime})
	}))
	defer server.Close()
	service, err := drive.NewService(context.Background(), option.WithEndpoint(server.URL+"/"), option.WithHTTPClient(server.Client()), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	importer := &GoogleDriveImporter{service: service, logger: log.New(io.Discard, "", 0)}
	name, err := importer.TestConnection(context.Background(), "https://drive.google.com/drive/folders/root?resourcekey=key")
	if err != nil || name != "Shared root" {
		t.Fatalf("name=%q err=%v", name, err)
	}
	for _, export := range []string{"", "application/pdf"} {
		response, err := importer.downloadFile(context.Background(), &drive.File{Id: "root", ResourceKey: "key"}, export)
		if err != nil {
			t.Fatal(err)
		}
		contents, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || !strings.HasPrefix(string(contents), "%PDF") {
			t.Fatalf("download %q: %v", contents, err)
		}
	}
	if requests != 4 {
		t.Fatalf("got %d requests, want 4", requests)
	}
}

func TestDriveWorkspaceExportsAndUnsupportedFiles(t *testing.T) {
	cases := []struct {
		mime, name, extension, export string
		supported                     bool
	}{
		{"application/pdf", "order.pdf", ".pdf", "", true},
		{"application/vnd.google-apps.document", "order", ".pdf", "application/pdf", true},
		{"application/vnd.google-apps.spreadsheet", "list", ".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", true},
		{"application/vnd.google-apps.presentation", "slides", ".pdf", "application/pdf", true},
		{"application/vnd.google-apps.drawing", "drawing", ".pdf", "application/pdf", true},
		{"image/jpeg", "photo.jpg", "", "", false},
	}
	for _, tc := range cases {
		name, export, supported := driveOutputFile(&drive.File{Name: tc.name, MimeType: tc.mime})
		if supported != tc.supported || export != tc.export || (supported && !strings.HasSuffix(name, tc.extension)) {
			t.Fatalf("%s: %q %q %v", tc.mime, name, export, supported)
		}
	}
}

func TestDrivePaginationPreservesEmptyPagesAndSharedDriveScope(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		query := r.URL.Query()
		for key, want := range map[string]string{"corpora": "drive", "driveId": "shared", "pageSize": "1000", "spaces": "drive", "q": "'nested' in parents and trashed = false", "supportsAllDrives": "true"} {
			if query.Get(key) != want {
				t.Errorf("%s=%q, want %q", key, query.Get(key), want)
			}
		}
		if r.Header.Get("X-Goog-Drive-Resource-Keys") != "nested/key" {
			t.Error("parent resource key missing")
		}
		if query.Get("pageToken") == "" {
			// Drive can return an empty page with another page still available.
			_, _ = io.WriteString(w, `{"files":[],"nextPageToken":"second"}`)
		} else if query.Get("pageToken") == "second" {
			_, _ = io.WriteString(w, `{"files":[{"id":"child","name":"folder","mimeType":"application/vnd.google-apps.folder","driveId":"shared"},{"id":"shortcut","mimeType":"application/vnd.google-apps.shortcut","shortcutDetails":{"targetId":"target","targetResourceKey":"key"}}]}`)
		} else {
			t.Errorf("unexpected page token %q", query.Get("pageToken"))
		}
	}))
	defer server.Close()
	service, err := drive.NewService(context.Background(), option.WithEndpoint(server.URL+"/"), option.WithHTTPClient(server.Client()), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	importer := &GoogleDriveImporter{service: service}
	folder := &DriveSyncFolder{FolderID: "nested", DriveID: "shared", ResourceKey: "key"}
	first, err := importer.listDriveFolderPage(context.Background(), folder)
	if err != nil || len(first.Files) != 0 || first.NextPageToken != "second" {
		t.Fatalf("first page: %+v, %v", first, err)
	}
	folder.PageToken = first.NextPageToken
	second, err := importer.listDriveFolderPage(context.Background(), folder)
	if err != nil || second.NextPageToken != "" || len(second.Files) != 2 {
		t.Fatalf("second page: %+v, %v", second, err)
	}
	if second.Files[0].DriveId != "shared" || second.Files[1].ShortcutDetails.TargetId != "target" {
		t.Fatal("lost folder/shortcut metadata")
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestDriveLongUnicodeNamesKeepFileExtension(t *testing.T) {
	for _, mime := range []string{"application/pdf", "application/vnd.google-apps.document"} {
		name, _, supported := driveOutputFile(&drive.File{Name: strings.Repeat("คำสั่ง", 150) + ".pdf", MimeType: mime})
		if !supported || !strings.HasSuffix(name, ".pdf") || len([]rune(name)) > 500 {
			t.Fatalf("invalid output filename: %q", name)
		}
	}
}
