package main

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"
)

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
