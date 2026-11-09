package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
)

// MemoryStore is an in-memory Store for tests.
type MemoryStore struct {
	mu   sync.Mutex
	objs map[string][]byte
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{objs: map[string][]byte{}} }

func (m *MemoryStore) SignedPut(_ context.Context, key, contentType string, maxBytes int64, ttl time.Duration) (*UploadTarget, error) {
	exp := time.Now().Add(ttl)
	return &UploadTarget{
		URL:    fmt.Sprintf("memory://%s?exp=%d&max=%d", key, exp.Unix(), maxBytes),
		Method: "PUT",
		Headers: map[string]string{
			"Content-Type":                contentType,
			"x-goog-content-length-range": fmt.Sprintf("0,%d", maxBytes),
		},
		ExpiresAt: exp,
	}, nil
}

func (m *MemoryStore) SignedGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	return fmt.Sprintf("memory://%s?exp=%d", key, time.Now().Add(ttl).Unix()), nil
}

func (m *MemoryStore) Open(_ context.Context, key string) (io.ReadCloser, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.objs[key]
	if !ok {
		return nil, 0, ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}

func (m *MemoryStore) Put(_ context.Context, key, _ string, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.PutRaw(key, b)
	return nil
}

func (m *MemoryStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objs, key)
	return nil
}

// PutRaw simulates a browser having uploaded bytes to key.
func (m *MemoryStore) PutRaw(key string, b []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objs[key] = append([]byte(nil), b...)
}

// Keys returns all stored keys, sorted.
func (m *MemoryStore) Keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.objs))
	for k := range m.objs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
