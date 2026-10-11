package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTempPDF(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4 test"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestHTTPExtractor(url, key string) *httpExtractor {
	return &httpExtractor{
		engine:   "docflow-contract",
		endpoint: url,
		apiKey:   key,
		client:   &http.Client{Timeout: 2 * time.Second},
		decode: func(raw []byte) (ExtractionResult, error) {
			var r ExtractionResult
			err := json.Unmarshal(raw, &r)
			return r, err
		},
	}
}

func TestHTTPExtractorSendsBearerOnlyWhenKeySet(t *testing.T) {
	var gotAuth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"engine":"x","groups":[]}`))
	}))
	defer srv.Close()
	pdf := writeTempPDF(t)

	if _, err := newTestHTTPExtractor(srv.URL, "secret").Extract(context.Background(), pdf, "a.pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err := newTestHTTPExtractor(srv.URL, "").Extract(context.Background(), pdf, "a.pdf"); err != nil {
		t.Fatal(err)
	}
	if gotAuth[0] != "Bearer secret" || gotAuth[1] != "" {
		t.Fatalf("Authorization headers = %q", gotAuth)
	}
}

func TestHTTPExtractorSendsMultipartFileField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		f.Close()
		_, _ = w.Write([]byte(`{"engine":"x","groups":[]}`))
	}))
	defer srv.Close()
	if _, err := newTestHTTPExtractor(srv.URL, "").Extract(context.Background(), writeTempPDF(t), "a.pdf"); err != nil {
		t.Fatalf("multipart field file missing: %v", err)
	}
}

func TestHTTPExtractorClassifiesStatusCodes(t *testing.T) {
	cases := []struct {
		status    int
		retryable bool
	}{
		{http.StatusTooManyRequests, true},
		{http.StatusBadGateway, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusGatewayTimeout, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadRequest, false},
		{http.StatusRequestEntityTooLarge, false},
		{http.StatusUnsupportedMediaType, false},
		{http.StatusUnprocessableEntity, false},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusNotFound, false},
	}
	for _, c := range cases {
		c := c
		t.Run(http.StatusText(c.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
			}))
			defer srv.Close()
			_, err := newTestHTTPExtractor(srv.URL, "").Extract(context.Background(), writeTempPDF(t), "a.pdf")
			ee, ok := err.(*ExtractError)
			if !ok {
				t.Fatalf("err = %T %v, want *ExtractError", err, err)
			}
			if ee.Retryable != c.retryable || ee.Status != c.status || ee.Message == "" {
				t.Fatalf("got %+v", ee)
			}
		})
	}
}

func TestClassifyHTTPFailureIncludesAPIDetail(t *testing.T) {
	ee := classifyHTTPFailure(http.StatusBadRequest, []byte(`{"detail":"a.txt: not a readable PDF or image"}`))
	if ee.Retryable || !strings.Contains(ee.Message, "not a readable PDF") {
		t.Fatalf("got %+v", ee)
	}
}

func TestHTTPExtractorConnectionFailureIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	_, err := newTestHTTPExtractor(url, "").Extract(context.Background(), writeTempPDF(t), "a.pdf")
	ee, ok := err.(*ExtractError)
	if !ok || !ee.Retryable || ee.Status != 0 {
		t.Fatalf("err = %#v, want retryable ExtractError with status 0", err)
	}
}

func TestHTTPExtractorTimeoutIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()
	ex := newTestHTTPExtractor(srv.URL, "")
	ex.client = &http.Client{Timeout: 50 * time.Millisecond}
	_, err := ex.Extract(context.Background(), writeTempPDF(t), "a.pdf")
	ee, ok := err.(*ExtractError)
	if !ok || !ee.Retryable {
		t.Fatalf("err = %#v, want retryable timeout", err)
	}
}

func TestHTTPExtractorBadBodyIsPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	_, err := newTestHTTPExtractor(srv.URL, "").Extract(context.Background(), writeTempPDF(t), "a.pdf")
	ee, ok := err.(*ExtractError)
	if !ok || ee.Retryable {
		t.Fatalf("err = %#v, want permanent ExtractError", err)
	}
}
