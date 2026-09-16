package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"sync"
	"time"
)

const (
	defaultExportTTL             = 10 * time.Minute
	defaultMaxActiveExports      = 4
	defaultMaxActiveExportsOwner = 1
	exportTokenBytes             = 32
)

var (
	errExportCapacity = errors.New("temporary export capacity reached")
	errOwnerCapacity  = errors.New("temporary export owner capacity reached")
	errExportNotFound = errors.New("temporary export not found")
)

// temporaryExportStore is the narrow interface used by the command and HTTP
// delivery paths. Consume combines authorization and deletion so a successful
// token can be used only once, even by concurrent callers.
type temporaryExportStore interface {
	Put(ownerID string, contents []byte) (string, error)
	Consume(ownerID, token string) ([]byte, error)
}

type storedExport struct {
	ownerID  string
	contents []byte
	expires  time.Time
}

// memoryExportStore keeps short-lived exports in one plugin process. All
// entry inspection and mutation is protected by mu.
type memoryExportStore struct {
	mu             sync.Mutex
	entries        map[string]storedExport
	ttl            time.Duration
	maxActive      int
	maxActiveOwner int
	now            func() time.Time
	random         io.Reader
}

func newMemoryExportStore(ttl time.Duration, maxActive, maxActiveOwner int) *memoryExportStore {
	if ttl <= 0 || maxActive <= 0 || maxActiveOwner <= 0 {
		panic("temporary export store limits must be positive")
	}

	return &memoryExportStore{
		entries:        make(map[string]storedExport),
		ttl:            ttl,
		maxActive:      maxActive,
		maxActiveOwner: maxActiveOwner,
		now:            time.Now,
		random:         rand.Reader,
	}
}

func newDefaultMemoryExportStore() *memoryExportStore {
	return newMemoryExportStore(defaultExportTTL, defaultMaxActiveExports, defaultMaxActiveExportsOwner)
}

func (s *memoryExportStore) Put(ownerID string, contents []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.removeExpired(now)

	ownerEntries := 0
	for _, entry := range s.entries {
		if entry.ownerID == ownerID {
			ownerEntries++
		}
	}
	if ownerEntries >= s.maxActiveOwner {
		return "", errOwnerCapacity
	}
	if len(s.entries) >= s.maxActive {
		return "", errExportCapacity
	}

	for {
		randomBytes := make([]byte, exportTokenBytes)
		if _, err := io.ReadFull(s.random, randomBytes); err != nil {
			return "", err
		}
		token := base64.RawURLEncoding.EncodeToString(randomBytes)
		if _, exists := s.entries[token]; exists {
			continue
		}

		s.entries[token] = storedExport{
			ownerID:  ownerID,
			contents: append([]byte(nil), contents...),
			expires:  now.Add(s.ttl),
		}
		return token, nil
	}
}

func (s *memoryExportStore) Consume(ownerID, token string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.removeExpired(now)
	entry, exists := s.entries[token]
	if !exists || entry.ownerID != ownerID {
		return nil, errExportNotFound
	}

	delete(s.entries, token)
	return append([]byte(nil), entry.contents...), nil
}

// removeExpired is called with mu held. An entry expires exactly at its
// deadline, avoiding an extra usable instant at the TTL boundary.
func (s *memoryExportStore) removeExpired(now time.Time) {
	for token, entry := range s.entries {
		if !now.Before(entry.expires) {
			delete(s.entries, token)
		}
	}
}
