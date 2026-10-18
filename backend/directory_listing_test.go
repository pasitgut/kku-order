package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestDirectoryListingPaginationAndFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users := make([]DirectoryUser, 130)
	for i := range users {
		users[i] = DirectoryUser{ID: uint(i + 1), FullName: fmt.Sprintf("บุคลากร %d", i+1), Source: sourceSynced}
	}
	users[129].Source = sourceManual
	users[129].Department = "หน่วยงานภายนอก"
	users[129].Phone = "0812345678"
	router := gin.New()
	router.GET("/", func(c *gin.Context) { respondDirectoryUsers(c, users) })
	for _, tc := range []struct {
		name, query        string
		count, total, page int
		firstID            uint
	}{
		{"legacy picker retains limit", "", 100, 0, 0, 1},
		{"first page includes newest", "?page=1", 25, 130, 1, 130},
		{"past old 100 row limit", "?page=6", 5, 130, 6, 5},
		{"clamp out of range", "?page=999999999&limit=25", 5, 130, 6, 5},
		{"manual filter before pagination", "?page=1&source=MANUAL", 1, 1, 1, 130},
		{"synced filter", "?page=1&source=SYNCED", 25, 129, 1, 129},
		{"department search", "?page=1&q=" + url.QueryEscape("หน่วยงานภายนอก"), 1, 1, 1, 130},
		{"phone search", "?page=1&q=0812345678", 1, 1, 1, 130},
		{"combined filters", "?page=1&source=SYNCED&q=0812345678", 0, 0, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/"+tc.query, nil))
			var result struct {
				Data       []DirectoryUser                  `json:"data"`
				Pagination struct{ Total, Page, Limit int } `json:"pagination"`
			}
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body)
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Data == nil || len(result.Data) != tc.count || result.Pagination.Total != tc.total || result.Pagination.Page != tc.page {
				t.Fatalf("response = %s", response.Body)
			}
			if tc.count > 0 && result.Data[0].ID != tc.firstID {
				t.Fatalf("first ID = %d, want %d", result.Data[0].ID, tc.firstID)
			}
		})
	}
	for _, query := range []string{"?page=0", "?page=no", "?page=1&limit=101", "?page=1&limit=0", "?page=1&source=unknown"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/"+query, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d", query, response.Code)
		}
	}
}

func TestManualTeacherProfileAvailableForDocumentMatching(t *testing.T) {
	now := time.Now()
	person, profile, err := newManualDirectoryPerson(createDirectoryUserRequest{
		Prefix: " ผศ.ดร. ", FirstName: " สมชาย ", LastName: " ใจดี ", PositionTitle: " อาจารย์ ",
		Department: " วิทยาลัยการคอมพิวเตอร์ ", Email: " teacher@example.com ", Phone: " 0812345678 ",
	}, "staff@example.com", now)
	if err != nil {
		t.Fatal(err)
	}
	person.ID = 131
	user := directoryPersonAPI(person, profile)
	if user.FullName != "ผศ.ดร. สมชาย ใจดี" || user.Source != sourceManual || user.IsActive != activeUserCode || user.CreatedBy != "staff@example.com" || user.Department != "วิทยาลัยการคอมพิวเตอร์" || user.Email != "teacher@example.com" || user.Phone != "0812345678" {
		t.Fatalf("user = %+v", user)
	}
	if !json.Valid([]byte(profile.RawPayload)) {
		t.Fatalf("invalid profile JSON: %q", profile.RawPayload)
	}
	match := matchDirectoryUser("สมชาย ใจดี", []DirectoryUser{user}, nil)
	if !match.Matched || match.User.ID != 131 {
		t.Fatalf("new teacher did not match document name: %+v", match)
	}
}
