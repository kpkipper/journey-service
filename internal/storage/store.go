// Package storage abstracts the private object store that holds moment media.
//
// Raw browser uploads live at uploads/<journeyID>/<momentID> and are never
// served; processed files live at moments/<journeyID>/<momentID>/<name>.
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// UploadTarget tells a browser how to upload a raw file directly to the store.
// Headers must be sent verbatim.
type UploadTarget struct {
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// Store is the object storage used by MomentService.
type Store interface {
	SignedPut(ctx context.Context, key, contentType string, maxBytes int64, ttl time.Duration) (*UploadTarget, error)
	SignedGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	// Open returns ErrObjectNotFound when the object does not exist.
	Open(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Put(ctx context.Context, key, contentType string, r io.Reader) error
	// Delete succeeds when the object does not exist.
	Delete(ctx context.Context, key string) error
}

var (
	ErrObjectNotFound = errors.New("object not found")
	ErrInvalidKey     = errors.New("invalid object key")
)

// RawKey is where a browser uploads the original file.
func RawKey(journeyID, momentID string) string {
	return "uploads/" + journeyID + "/" + momentID
}

// MomentKey is where a processed file (full.jpg, thumb.jpg, video.mp4) lives.
func MomentKey(journeyID, momentID, name string) string {
	return "moments/" + journeyID + "/" + momentID + "/" + name
}
