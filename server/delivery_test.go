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
	token, err := store.Put("owner", []byte("private export"))
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

	if _, err := store.Consume("owner", token); err != nil {
		t.Fatalf("failed requests consumed the owner's token: %v", err)
	}
}

func TestServeHTTPSetsSafeDownloadHeadersAndPreventsReplay(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 1, 1)
	token, err := store.Put("owner", []byte("<!doctype html><title>Export</title>"))
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
		"Content-Disposition":     `attachment; filename="direct-messages.html"`,
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

func TestServeHTTPRejectsExpiredAndMalformedRequests(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 2, 1)
	now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	token, err := store.Put("owner", []byte("expired"))
	if err != nil {
		t.Fatalf("Put returned an error: %v", err)
	}
	now = now.Add(time.Minute)
	plugin := &Plugin{exportStore: store}

	for _, target := range []string{
		"/download?token=" + token,
		"/download",
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

func TestMemoryExportStoreCreatesRandomOwnerBoundTokens(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 4, 2)
	contents := []byte("private export")
	token, err := store.Put("owner-one", contents)
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
	if _, err := store.Consume("owner-two", token); !errors.Is(err, errExportNotFound) {
		t.Fatalf("Consume by another owner returned %v, want errExportNotFound", err)
	}
	got, err := store.Consume("owner-one", token)
	if err != nil {
		t.Fatalf("Consume by owner returned an error: %v", err)
	}
	if string(got) != "private export" {
		t.Errorf("consumed contents = %q, want an isolated copy", got)
	}
}

func TestMemoryExportStoreEnforcesConfiguredLimits(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 2, 1)
	if _, err := store.Put("owner-one", []byte("one")); err != nil {
		t.Fatalf("first Put returned an error: %v", err)
	}
	if _, err := store.Put("owner-one", []byte("duplicate")); !errors.Is(err, errOwnerCapacity) {
		t.Errorf("second owner Put returned %v, want errOwnerCapacity", err)
	}
	if _, err := store.Put("owner-two", []byte("two")); err != nil {
		t.Fatalf("second Put returned an error: %v", err)
	}
	if _, err := store.Put("owner-three", []byte("three")); !errors.Is(err, errExportCapacity) {
		t.Errorf("Put beyond total limit returned %v, want errExportCapacity", err)
	}
}

func TestMemoryExportStoreRemovesExpiredEntries(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 1, 1)
	now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	token, err := store.Put("owner", []byte("expired"))
	if err != nil {
		t.Fatalf("Put returned an error: %v", err)
	}
	now = now.Add(time.Minute)
	if _, err := store.Consume("owner", token); !errors.Is(err, errExportNotFound) {
		t.Errorf("Consume at expiry returned %v, want errExportNotFound", err)
	}
	if _, err := store.Put("replacement", []byte("current")); err != nil {
		t.Fatalf("expired entry continued to consume capacity: %v", err)
	}
}

func TestMemoryExportStoreConsumesAtomically(t *testing.T) {
	store := newMemoryExportStore(time.Minute, 4, 1)
	token, err := store.Put("owner", []byte("one use"))
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
			contents, consumeErr := store.Consume("owner", token)
			if consumeErr == nil && !bytes.Equal(contents, []byte("one use")) {
				consumeErr = errors.New("unexpected contents")
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
	if _, err := store.Put("owner", []byte("contents")); err == nil {
		t.Fatal("Put returned nil error for random source failure")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("random source failed")
}
