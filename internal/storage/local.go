package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// LocalStore keeps objects on disk and serves them through HMAC-signed URLs
// handled by the API itself. Development use only.
type LocalStore struct {
	dir     string
	baseURL string
	secret  []byte
	now     func() time.Time
}

var _ Store = (*LocalStore)(nil)

// NewLocalStore: publicBaseURL is e.g. http://localhost:8080/api/v1/media/local.
func NewLocalStore(dir, publicBaseURL string, secret []byte) (*LocalStore, error) {
	if dir == "" {
		return nil, errors.New("storage: local dir is required")
	}
	if publicBaseURL == "" {
		return nil, errors.New("storage: public base URL is required")
	}
	if len(secret) == 0 {
		return nil, errors.New("storage: signing secret is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: create dir: %w", err)
	}
	return &LocalStore{dir: dir, baseURL: strings.TrimRight(publicBaseURL, "/"), secret: secret, now: time.Now}, nil
}

// sign covers method, key, expiry, size cap and (for PUT) the content type.
func (s *LocalStore) sign(method, key string, exp, max int64, contentType string) string {
	mac := hmac.New(sha256.New, s.secret)
	fmt.Fprintf(mac, "%s\n%s\n%d\n%d\n%s", method, key, exp, max, contentType)
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *LocalStore) signedURL(method, key string, ttl time.Duration, max int64, ct string) (string, time.Time) {
	expAt := s.now().Add(ttl)
	exp := expAt.Unix()
	return fmt.Sprintf("%s/%s?exp=%d&max=%d&sig=%s", s.baseURL, key, exp, max, s.sign(method, key, exp, max, ct)), expAt
}

func (s *LocalStore) SignedPut(_ context.Context, key, contentType string, maxBytes int64, ttl time.Duration) (*UploadTarget, error) {
	if err := checkPutKey(key); err != nil {
		return nil, err
	}
	u, exp := s.signedURL("PUT", key, ttl, maxBytes, contentType)
	return &UploadTarget{URL: u, Method: "PUT", Headers: map[string]string{"Content-Type": contentType}, ExpiresAt: exp}, nil
}

func (s *LocalStore) SignedGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	if err := checkGetKey(key); err != nil {
		return "", err
	}
	u, _ := s.signedURL("GET", key, ttl, 0, "")
	return u, nil
}

func (s *LocalStore) file(key string) (string, error) {
	if !validKey(key) {
		return "", ErrInvalidKey
	}
	return filepath.Join(s.dir, filepath.FromSlash(key)), nil
}

func (s *LocalStore) Open(_ context.Context, key string) (io.ReadCloser, int64, error) {
	p, err := s.file(key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, ErrObjectNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, st.Size(), nil
}

func (s *LocalStore) Put(_ context.Context, key, _ string, r io.Reader) error {
	p, err := s.file(key)
	if err != nil {
		return err
	}
	return writeAtomic(p, r)
}

func (s *LocalStore) Delete(_ context.Context, key string) error {
	p, err := s.file(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func writeAtomic(p string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// Register mounts GET and PUT /media/local/* on r.
func (s *LocalStore) Register(r fiber.Router) {
	r.Get("/media/local/*", s.handleGet)
	r.Put("/media/local/*", s.handlePut)
}

// authorize validates key shape, expiry and signature; returns the max param.
func (s *LocalStore) authorize(c *fiber.Ctx, method, ct string) (key string, max int64, err error) {
	raw := c.Params("*")
	key, uerr := url.PathUnescape(raw)
	if uerr != nil || path.Clean(key) != key || !validKey(key) {
		return "", 0, fiber.ErrNotFound
	}
	exp, e1 := strconv.ParseInt(c.Query("exp"), 10, 64)
	max, e2 := strconv.ParseInt(c.Query("max"), 10, 64)
	sig := c.Query("sig")
	if e1 != nil || e2 != nil || max < 0 || sig == "" {
		return "", 0, fiber.ErrForbidden
	}
	want := s.sign(method, key, exp, max, ct)
	if !hmac.Equal([]byte(want), []byte(sig)) || s.now().Unix() > exp {
		return "", 0, fiber.ErrForbidden
	}
	return key, max, nil
}

func (s *LocalStore) handleGet(c *fiber.Ctx) error {
	key, _, err := s.authorize(c, "GET", "")
	if err != nil {
		return err
	}
	// Raw uploads are never served; only processed moments and covers are.
	if checkGetKey(key) != nil {
		return fiber.ErrForbidden
	}
	p, _ := s.file(key)
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return fiber.ErrNotFound
	} else if err != nil {
		return err
	}
	c.Set(fiber.HeaderCacheControl, "private, max-age=3600")
	// SendFile supports Range requests (needed for <video>).
	return c.SendFile(p)
}

func (s *LocalStore) handlePut(c *fiber.Ctx) error {
	key, max, err := s.authorize(c, "PUT", c.Get(fiber.HeaderContentType))
	if err != nil {
		return err
	}
	if !strings.HasPrefix(key, "uploads/") {
		return fiber.ErrForbidden
	}
	body := c.Body()
	if int64(len(body)) > max {
		return fiber.ErrRequestEntityTooLarge
	}
	p, _ := s.file(key)
	if err := writeAtomic(p, bytes.NewReader(body)); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusOK)
}
