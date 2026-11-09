package storage

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

const testBase = "http://localhost:8080/api/v1/media/local"

func newLocal(t *testing.T) (*LocalStore, *fiber.App, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := NewLocalStore(dir, testBase, []byte("secret"))
	require.NoError(t, err)
	app := fiber.New(fiber.Config{BodyLimit: 200 * 1024 * 1024})
	s.Register(app.Group("/api/v1"))
	return s, app, dir
}

// toPath turns a signed absolute URL into a request path+query for the test app.
func toPath(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u.RequestURI()
}

func do(t *testing.T, app *fiber.App, method, path, ct string, body []byte) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	return resp
}

func TestLocal_PutThenGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, app, dir := newLocal(t)

	// browser uploads raw
	tgt, err := s.SignedPut(ctx, "uploads/j1/m1", "image/jpeg", 1024, 15*time.Minute)
	require.NoError(t, err)
	require.Equal(t, "PUT", tgt.Method)
	require.Equal(t, map[string]string{"Content-Type": "image/jpeg"}, tgt.Headers)
	require.True(t, strings.HasPrefix(tgt.URL, testBase+"/uploads/j1/m1?exp="))
	resp := do(t, app, "PUT", toPath(t, tgt.URL), "image/jpeg", []byte("rawbytes"))
	require.Equal(t, 200, resp.StatusCode)
	b, err := os.ReadFile(filepath.Join(dir, "uploads/j1/m1"))
	require.NoError(t, err)
	require.Equal(t, "rawbytes", string(b))

	// processed file served via signed GET
	require.NoError(t, s.Put(ctx, "moments/j1/m1/full.jpg", "image/jpeg", strings.NewReader("jpegdata")))
	g, err := s.SignedGet(ctx, "moments/j1/m1/full.jpg", time.Hour)
	require.NoError(t, err)
	resp = do(t, app, "GET", toPath(t, g), "", nil)
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, "image/jpeg", resp.Header.Get("Content-Type"))
	require.Equal(t, "private, max-age=3600", resp.Header.Get("Cache-Control"))
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	require.Equal(t, "jpegdata", buf.String())

	// video content type
	require.NoError(t, s.Put(ctx, "moments/j1/m1/video.mp4", "video/mp4", strings.NewReader("v")))
	g, _ = s.SignedGet(ctx, "moments/j1/m1/video.mp4", time.Hour)
	resp = do(t, app, "GET", toPath(t, g), "", nil)
	require.Equal(t, "video/mp4", resp.Header.Get("Content-Type"))

	// Open
	rc, n, err := s.Open(ctx, "moments/j1/m1/full.jpg")
	require.NoError(t, err)
	rc.Close()
	require.EqualValues(t, 8, n)
}

func TestLocal_RejectsExpiredAndTamperedSignature(t *testing.T) {
	ctx := context.Background()
	s, app, _ := newLocal(t)

	expired, _ := s.SignedPut(ctx, "uploads/j1/m1", "image/jpeg", 1024, -time.Minute)
	require.Equal(t, 403, do(t, app, "PUT", toPath(t, expired.URL), "image/jpeg", []byte("x")).StatusCode)

	ok, _ := s.SignedPut(ctx, "uploads/j1/m1", "image/jpeg", 1024, time.Minute)
	p := toPath(t, ok.URL)

	// tampered sig
	u, _ := url.Parse(p)
	q := u.Query()
	q.Set("sig", strings.Repeat("0", 64))
	u.RawQuery = q.Encode()
	require.Equal(t, 403, do(t, app, "PUT", u.RequestURI(), "image/jpeg", []byte("x")).StatusCode)

	// tampered max
	u, _ = url.Parse(p)
	q = u.Query()
	q.Set("max", "999999")
	u.RawQuery = q.Encode()
	require.Equal(t, 403, do(t, app, "PUT", u.RequestURI(), "image/jpeg", []byte("x")).StatusCode)

	// different content type than signed
	require.Equal(t, 403, do(t, app, "PUT", p, "text/html", []byte("x")).StatusCode)

	// a PUT signature cannot be used for GET and vice versa
	require.Equal(t, 403, do(t, app, "GET", p, "", nil).StatusCode)

	// missing params
	require.Equal(t, 403, do(t, app, "PUT", "/api/v1/media/local/uploads/j1/m1", "image/jpeg", []byte("x")).StatusCode)

	// expired GET
	require.NoError(t, s.Put(ctx, "moments/j1/m1/full.jpg", "image/jpeg", strings.NewReader("a")))
	g, _ := s.SignedGet(ctx, "moments/j1/m1/full.jpg", -time.Second)
	require.Equal(t, 403, do(t, app, "GET", toPath(t, g), "", nil).StatusCode)
}

func TestLocal_RejectsOversizeBody(t *testing.T) {
	ctx := context.Background()
	s, app, dir := newLocal(t)
	tgt, _ := s.SignedPut(ctx, "uploads/j1/m1", "image/jpeg", 4, time.Minute)
	resp := do(t, app, "PUT", toPath(t, tgt.URL), "image/jpeg", []byte("12345"))
	require.Equal(t, 413, resp.StatusCode)
	_, err := os.Stat(filepath.Join(dir, "uploads/j1/m1"))
	require.True(t, os.IsNotExist(err))

	// exactly max is fine
	resp = do(t, app, "PUT", toPath(t, tgt.URL), "image/jpeg", []byte("1234"))
	require.Equal(t, 200, resp.StatusCode)
}

func TestLocal_RejectsTraversalKey(t *testing.T) {
	ctx := context.Background()
	s, app, _ := newLocal(t)
	exp := time.Now().Add(time.Minute)

	// Even with a valid signature over the evil key, the key shape is refused.
	for _, key := range []string{
		"uploads/../../etc/passwd",
		"uploads/j1/m1/extra",
		"moments/j1/m1",
		"moments/j1/m1/full.png",
		"moments/j1/m1/../../x.jpg",
		"other/j1/m1",
		"uploads/J1/m1",
	} {
		sig := s.sign("PUT", key, exp.Unix(), 10, "image/jpeg")
		path := "/api/v1/media/local/" + key + "?exp=" + strconv.FormatInt(exp.Unix(), 10) + "&max=10&sig=" + sig
		require.Equal(t, 404, do(t, app, "PUT", path, "image/jpeg", []byte("x")).StatusCode, key)
		require.False(t, validKey(key), key)
	}
	// encoded traversal
	path := "/api/v1/media/local/uploads/%2e%2e/%2e%2e/etc/passwd?exp=1&max=1&sig=aa"
	require.Contains(t, []int{403, 404}, do(t, app, "PUT", path, "image/jpeg", []byte("x")).StatusCode)

	_, err := s.SignedPut(ctx, "uploads/../x", "image/jpeg", 1, time.Minute)
	require.ErrorIs(t, err, ErrInvalidKey)
	_, _, err = s.Open(ctx, "../etc/passwd")
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestLocal_CannotPutToMomentsPrefix(t *testing.T) {
	s, app, dir := newLocal(t)
	// Signed directly: the handler must refuse even a valid signature.
	exp := time.Now().Add(time.Minute).Unix()
	sig := s.sign("PUT", "moments/j1/m1/full.jpg", exp, 10, "image/jpeg")
	path := "/api/v1/media/local/moments/j1/m1/full.jpg?exp=" + strconv.FormatInt(exp, 10) + "&max=10&sig=" + sig
	require.Equal(t, 403, do(t, app, "PUT", path, "image/jpeg", []byte("x")).StatusCode)
	_, err := os.Stat(filepath.Join(dir, "moments/j1/m1/full.jpg"))
	require.True(t, os.IsNotExist(err))
}

func TestLocal_GetOfRawKeyIsRefused(t *testing.T) {
	ctx := context.Background()
	s, app, _ := newLocal(t)
	require.NoError(t, s.Put(ctx, "uploads/j1/m1", "image/jpeg", strings.NewReader("secret-raw")))
	// The public API refuses to sign it...
	_, err := s.SignedGet(ctx, "uploads/j1/m1", time.Hour)
	require.ErrorIs(t, err, ErrInvalidKey)
	// ...and the handler refuses even a validly-signed URL.
	exp := time.Now().Add(time.Hour).Unix()
	sig := s.sign("GET", "uploads/j1/m1", exp, 0, "")
	path := "/api/v1/media/local/uploads/j1/m1?exp=" + strconv.FormatInt(exp, 10) + "&max=0&sig=" + sig
	require.Equal(t, 403, do(t, app, "GET", path, "", nil).StatusCode)
}

func TestLocal_SignedKeyPrefixes(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newLocal(t)
	_, err := s.SignedPut(ctx, "moments/j1/m1/full.jpg", "image/jpeg", 1, time.Minute)
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestLocal_GetSupportsRange(t *testing.T) {
	ctx := context.Background()
	s, app, _ := newLocal(t)
	require.NoError(t, s.Put(ctx, "moments/j1/m1/video.mp4", "video/mp4", strings.NewReader("0123456789")))
	g, _ := s.SignedGet(ctx, "moments/j1/m1/video.mp4", time.Hour)
	req := httptest.NewRequest("GET", toPath(t, g), nil)
	req.Header.Set("Range", "bytes=0-3")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, 206, resp.StatusCode)
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	require.Equal(t, "0123", buf.String())
	require.Equal(t, "video/mp4", resp.Header.Get("Content-Type"))
	require.Equal(t, "private, max-age=3600", resp.Header.Get("Cache-Control"))
}

func TestLocal_GetMissingIs404(t *testing.T) {
	ctx := context.Background()
	s, app, _ := newLocal(t)
	g, _ := s.SignedGet(ctx, "moments/j1/m1/full.jpg", time.Hour)
	require.Equal(t, 404, do(t, app, "GET", toPath(t, g), "", nil).StatusCode)
}

func TestLocal_DeleteMissingIsOK(t *testing.T) {
	ctx := context.Background()
	s, _, dir := newLocal(t)
	require.NoError(t, s.Delete(ctx, "uploads/j1/nope"))
	require.NoError(t, s.Put(ctx, "uploads/j1/m1", "image/jpeg", strings.NewReader("a")))
	require.NoError(t, s.Delete(ctx, "uploads/j1/m1"))
	_, err := os.Stat(filepath.Join(dir, "uploads/j1/m1"))
	require.True(t, os.IsNotExist(err))
}

func TestLocal_OpenMissing(t *testing.T) {
	s, _, _ := newLocal(t)
	_, _, err := s.Open(context.Background(), "uploads/j1/nope")
	require.ErrorIs(t, err, ErrObjectNotFound)
}

func TestNewLocalStore_Validation(t *testing.T) {
	_, err := NewLocalStore("", testBase, []byte("s"))
	require.Error(t, err)
	_, err = NewLocalStore(t.TempDir(), testBase, nil)
	require.Error(t, err)
	_, err = NewLocalStore(t.TempDir(), "", []byte("s"))
	require.Error(t, err)
}
