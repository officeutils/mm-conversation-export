package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestServeHTTPRequiresAuthenticatedOwnerAndPreservesToken(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 1, 1)
	token, err := store.Put("owner", "export.html", []byte("private export"))
	if err != nil {
		t.Fatalf("Put returned an error: %v", err)
	}
	plugin := &Plugin{exportStore: store}

	for _, test := range []struct {
		name   string
		userID string
		status int
	}{
		{name: "anonymous", status: http.StatusUnauthorized},
		{name: "different owner", userID: "intruder", status: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/download?token="+token, nil)
			request.Header.Set("Mattermost-User-Id", test.userID)
			response := httptest.NewRecorder()

			plugin.ServeHTTP(nil, response, request)

			if response.Code != test.status {
				t.Errorf("status = %d, want %d", response.Code, test.status)
			}
		})
	}

	if _, err := store.Claim("owner", token); err != nil {
		t.Fatalf("failed requests consumed the owner's token: %v", err)
	}
	store.Finish("owner", token, true)
}

func TestServeHTTPSetsSafeDownloadHeadersAndPreventsReplay(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 1, 1)
	token, err := store.Put("owner", "export.html", []byte("<!doctype html><title>Export</title>"))
	if err != nil {
		t.Fatalf("Put returned an error: %v", err)
	}
	plugin := &Plugin{exportStore: store}

	request := httptest.NewRequest(http.MethodGet, "/download?token="+token, nil)
	request.Header.Set("Mattermost-User-Id", "owner")
	response := httptest.NewRecorder()
	plugin.ServeHTTP(nil, response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Body.String(); got != "<!doctype html><title>Export</title>" {
		t.Errorf("body = %q, want stored export", got)
	}
	wantHeaders := map[string]string{
		"Content-Type":            "text/html; charset=utf-8",
		"Content-Disposition":     `attachment; filename="export.html"`,
		"Cache-Control":           "no-store, no-cache, must-revalidate",
		"Pragma":                  "no-cache",
		"Expires":                 "0",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "sandbox; default-src 'none'",
	}
	for name, want := range wantHeaders {
		if got := response.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}

	replay := httptest.NewRecorder()
	plugin.ServeHTTP(nil, replay, request)
	if replay.Code != http.StatusNotFound {
		t.Errorf("replay status = %d, want %d", replay.Code, http.StatusNotFound)
	}
}

func TestServeHTTPPreservesTokenWhenResponseWriteFails(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 1, 1)
	token, err := store.Put("owner", "export.html", []byte("private export"))
	if err != nil {
		t.Fatalf("Put returned an error: %v", err)
	}
	plugin := &Plugin{exportStore: store}
	request := httptest.NewRequest(http.MethodGet, "/download?token="+token, nil)
	request.Header.Set("Mattermost-User-Id", "owner")

	plugin.ServeHTTP(nil, &failingResponseWriter{header: make(http.Header)}, request)

	retry := httptest.NewRecorder()
	plugin.ServeHTTP(nil, retry, request)
	if retry.Code != http.StatusOK || retry.Body.String() != "private export" {
		t.Errorf("retry status/body = %d/%q, want %d/%q", retry.Code, retry.Body.String(), http.StatusOK, "private export")
	}
}

func TestServeHTTPRejectsExpiredAndMalformedRequests(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 2, 1)
	now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	token, err := store.Put("owner", "export.html", []byte("expired"))
	if err != nil {
		t.Fatalf("Put returned an error: %v", err)
	}
	now = now.Add(time.Minute)
	plugin := &Plugin{exportStore: store}

	for _, target := range []string{
		"/download?token=" + token,
		"/download",
		"/download?token=",
		"/download?token=one&token=two",
		"/other?token=" + token,
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Mattermost-User-Id", "owner")
		response := httptest.NewRecorder()
		plugin.ServeHTTP(nil, response, request)
		if response.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want %d", target, response.Code, http.StatusNotFound)
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/download?token="+token, nil)
	response := httptest.NewRecorder()
	plugin.ServeHTTP(nil, response, request)
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
		t.Errorf("POST status/Allow = %d/%q, want %d/%q", response.Code, response.Header().Get("Allow"), http.StatusMethodNotAllowed, http.MethodGet)
	}
}

func TestServeHTTPRejectsTokenMismatchWithoutConsumingExport(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 1, 1)
	token, err := store.Put("owner", "export.html", []byte("private export"))
	if err != nil {
		t.Fatalf("Put returned an error: %v", err)
	}
	plugin := &Plugin{exportStore: store}

	request := httptest.NewRequest(http.MethodGet, "/download?token="+token+"-wrong", nil)
	request.Header.Set("Mattermost-User-Id", "owner")
	response := httptest.NewRecorder()
	plugin.ServeHTTP(nil, response, request)
	if response.Code != http.StatusNotFound {
		t.Errorf("mismatched token status = %d, want %d", response.Code, http.StatusNotFound)
	}

	contents, err := store.Claim("owner", token)
	if err != nil {
		t.Fatalf("token mismatch consumed the valid export: %v", err)
	}
	if string(contents.contents) != "private export" {
		t.Errorf("claimed contents = %q, want %q", contents.contents, "private export")
	}
	store.Finish("owner", token, true)
}

func TestMemoryExportStoreRejectsConcurrentClaimAndAllowsRetryAfterFailure(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 1, 1)
	token, err := store.Put("owner", "export.html", []byte("private export"))
	if err != nil {
		t.Fatalf("Put returned an error: %v", err)
	}

	if _, err := store.Claim("owner", token); err != nil {
		t.Fatalf("first Claim returned an error: %v", err)
	}
	if _, err := store.Claim("owner", token); !errors.Is(err, errExportNotFound) {
		t.Fatalf("concurrent Claim returned %v, want errExportNotFound", err)
	}

	store.Finish("owner", token, false)
	if _, err := store.Claim("owner", token); err != nil {
		t.Fatalf("Claim after failed delivery returned an error: %v", err)
	}
	store.Finish("owner", token, true)
	if _, err := store.Claim("owner", token); !errors.Is(err, errExportNotFound) {
		t.Fatalf("replay Claim returned %v, want errExportNotFound", err)
	}
}

func TestMemoryExportStoreCreatesRandomOwnerBoundTokens(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 4, 2)
	contents := []byte("private export")
	token, err := store.Put("owner-one", "export.html", contents)
	if err != nil {
		t.Fatalf("Put returned an error: %v", err)
	}
	if token == "" {
		t.Fatal("Put returned an empty token")
	}
	if len(token) != 43 {
		t.Errorf("token length = %d, want 43 base64url characters", len(token))
	}

	contents[0] = 'X'
	if _, err := store.Claim("owner-two", token); !errors.Is(err, errExportNotFound) {
		t.Fatalf("Claim by another owner returned %v, want errExportNotFound", err)
	}
	got, err := store.Claim("owner-one", token)
	if err != nil {
		t.Fatalf("Consume by owner returned an error: %v", err)
	}
	if string(got.contents) != "private export" {
		t.Errorf("consumed contents = %q, want an isolated copy", got.contents)
	}
	store.Finish("owner-one", token, true)
}

func TestMemoryExportStoreEnforcesConfiguredLimits(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 2, 1)
	if _, err := store.Put("owner-one", "export.html", []byte("one")); err != nil {
		t.Fatalf("first Put returned an error: %v", err)
	}
	if _, err := store.Put("owner-one", "export.html", []byte("duplicate")); !errors.Is(err, errOwnerCapacity) {
		t.Errorf("second owner Put returned %v, want errOwnerCapacity", err)
	}
	if _, err := store.Put("owner-two", "export.html", []byte("two")); err != nil {
		t.Fatalf("second Put returned an error: %v", err)
	}
	if _, err := store.Put("owner-three", "export.html", []byte("three")); !errors.Is(err, errExportCapacity) {
		t.Errorf("Put beyond total limit returned %v, want errExportCapacity", err)
	}
}

func TestMemoryExportStoreRemovesExpiredEntries(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 1, 1)
	now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	token, err := store.Put("owner", "export.html", []byte("expired"))
	if err != nil {
		t.Fatalf("Put returned an error: %v", err)
	}
	now = now.Add(time.Minute)
	if _, err := store.Claim("owner", token); !errors.Is(err, errExportNotFound) {
		t.Errorf("Claim at expiry returned %v, want errExportNotFound", err)
	}
	if _, err := store.Put("replacement", "export.html", []byte("current")); err != nil {
		t.Fatalf("expired entry continued to consume capacity: %v", err)
	}
}

func TestMemoryExportStoreConsumesAtomically(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 4, 1)
	token, err := store.Put("owner", "export.html", []byte("one use"))
	if err != nil {
		t.Fatalf("Put returned an error: %v", err)
	}

	const callers = 20
	start := make(chan struct{})
	results := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			contents, consumeErr := store.Claim("owner", token)
			if consumeErr == nil && !bytes.Equal(contents.contents, []byte("one use")) {
				consumeErr = errors.New("unexpected contents")
			}
			if consumeErr == nil {
				store.Finish("owner", token, true)
			}
			results <- consumeErr
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for result := range results {
		if result == nil {
			successes++
		} else if !errors.Is(result, errExportNotFound) {
			t.Errorf("Consume returned an unexpected error: %v", result)
		}
	}
	if successes != 1 {
		t.Errorf("successful consumes = %d, want exactly 1", successes)
	}
}

func TestMemoryExportStorePropagatesRandomSourceFailure(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 1, 1)
	store.random = errorReader{}
	if _, err := store.Put("owner", "export.html", []byte("contents")); err == nil {
		t.Fatal("Put returned nil error for random source failure")
	}
}

type errorReader struct{}

type failingResponseWriter struct {
	header http.Header
}

func (w *failingResponseWriter) Header() http.Header { return w.header }

func (w *failingResponseWriter) WriteHeader(int) {}

func (w *failingResponseWriter) Write(contents []byte) (int, error) {
	return len(contents) / 2, errors.New("client disconnected")
}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("random source failed")
}
