package storage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	gcs "cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

// FirebaseStore uses a private Firebase Storage (GCS) bucket.
type FirebaseStore struct {
	client *gcs.Client
	bucket string
	email  string
	key    []byte
}

var _ Store = (*FirebaseStore)(nil)

// NewFirebaseStore accepts service-account JSON either raw or base64-encoded.
func NewFirebaseStore(ctx context.Context, bucket string, credentialsJSON []byte) (*FirebaseStore, error) {
	if bucket == "" {
		return nil, errors.New("storage: firebase bucket is required")
	}
	raw := bytesTrim(credentialsJSON)
	if len(raw) > 0 && raw[0] != '{' {
		dec, err := base64.StdEncoding.DecodeString(string(raw))
		if err != nil {
			return nil, fmt.Errorf("storage: credentials are neither JSON nor base64: %w", err)
		}
		raw = dec
	}
	var sa struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal(raw, &sa); err != nil {
		return nil, fmt.Errorf("storage: invalid credentials JSON: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("storage: credentials need client_email and private_key")
	}
	client, err := gcs.NewClient(ctx, option.WithCredentialsJSON(raw))
	if err != nil {
		return nil, fmt.Errorf("storage: new client: %w", err)
	}
	s := &FirebaseStore{client: client, bucket: bucket, email: sa.ClientEmail, key: []byte(sa.PrivateKey)}
	// Fail fast on an unusable private key.
	if _, err := s.SignedGet(ctx, "moments/x/x/x.jpg", time.Minute); err != nil {
		client.Close()
		return nil, fmt.Errorf("storage: unusable private key: %w", err)
	}
	return s, nil
}

func bytesTrim(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && (b[i] == ' ' || b[i] == '\n' || b[i] == '\r' || b[i] == '\t') {
		i++
	}
	for j > i && (b[j-1] == ' ' || b[j-1] == '\n' || b[j-1] == '\r' || b[j-1] == '\t') {
		j--
	}
	return b[i:j]
}

func (s *FirebaseStore) SignedPut(_ context.Context, key, contentType string, maxBytes int64, ttl time.Duration) (*UploadTarget, error) {
	rng := fmt.Sprintf("0,%d", maxBytes)
	exp := time.Now().Add(ttl)
	u, err := s.client.Bucket(s.bucket).SignedURL(key, &gcs.SignedURLOptions{
		GoogleAccessID: s.email,
		PrivateKey:     s.key,
		Method:         "PUT",
		Scheme:         gcs.SigningSchemeV4,
		Expires:        exp,
		ContentType:    contentType,
		Headers:        []string{"x-goog-content-length-range:" + rng},
	})
	if err != nil {
		return nil, err
	}
	return &UploadTarget{
		URL:       u,
		Method:    "PUT",
		Headers:   map[string]string{"Content-Type": contentType, "x-goog-content-length-range": rng},
		ExpiresAt: exp,
	}, nil
}

func (s *FirebaseStore) SignedGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	return s.client.Bucket(s.bucket).SignedURL(key, &gcs.SignedURLOptions{
		GoogleAccessID: s.email,
		PrivateKey:     s.key,
		Method:         "GET",
		Scheme:         gcs.SigningSchemeV4,
		Expires:        time.Now().Add(ttl),
	})
}

func (s *FirebaseStore) Open(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	r, err := s.client.Bucket(s.bucket).Object(key).NewReader(ctx)
	if errors.Is(err, gcs.ErrObjectNotExist) {
		return nil, 0, ErrObjectNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	return r, r.Attrs.Size, nil
}

func (s *FirebaseStore) Put(ctx context.Context, key, contentType string, r io.Reader) error {
	w := s.client.Bucket(s.bucket).Object(key).NewWriter(ctx)
	w.ContentType = contentType
	if _, err := io.Copy(w, r); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func (s *FirebaseStore) Delete(ctx context.Context, key string) error {
	err := s.client.Bucket(s.bucket).Object(key).Delete(ctx)
	if err != nil && !errors.Is(err, gcs.ErrObjectNotExist) {
		return err
	}
	return nil
}
