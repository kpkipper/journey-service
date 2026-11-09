package app_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/app"
	"github.com/kpkipper/journey-service/internal/auth"
	"github.com/kpkipper/journey-service/internal/storage"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMomentHTTPEnv is newEnv with a LocalStore (temp dir) so signed URLs are served by the app itself.
func newMomentHTTPEnv(t *testing.T) *env {
	t.Helper()
	mail := &testutil.CaptureMailer{}
	now := new(time.Time)
	*now = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	db := testutil.NewTestDB(t)
	ls, err := storage.NewLocalStore(t.TempDir(), "http://localhost/api/v1/media/local", []byte("test-secret"))
	require.NoError(t, err)
	a := app.New(app.Deps{
		DB:         db,
		Google:     &testutil.FakeGoogle{Identities: map[string]*auth.GoogleIdentity{}},
		Mailer:     mail,
		WebURL:     "http://web.test/",
		Now:        func() time.Time { return *now },
		Store:      ls,
		LocalStore: ls,
		TempDir:    t.TempDir(),
	})
	return &env{app: a, mail: mail, db: db, now: now}
}

func jpegWithExif(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for x := 0; x < 64; x++ {
		for y := 0; y < 48; y++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 5), 100, 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	b := buf.Bytes()
	payload := append([]byte("Exif\x00\x00"), []byte("GPS-SECRET-LOCATION")...)
	seg := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte((len(payload) + 2) & 0xFF)}
	seg = append(seg, payload...)
	out := append([]byte{}, b[:2]...)
	out = append(out, seg...)
	return append(out, b[2:]...)
}

// rawReq sends an arbitrary request (absolute URLs allowed) and returns status, headers' content type and body.
func (e *env) rawReq(t *testing.T, method, target string, hdr map[string]string, body []byte) (int, string, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := e.app.Test(req, 10000)
	require.NoError(t, err)
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Content-Type"), b
}

func (e *env) startUpload(t *testing.T, tok, slug, dayID string, size int) (status int, mid string, upload map[string]any, raw string) {
	t.Helper()
	body := fmt.Sprintf(`{"kind":"photo","content_type":"image/jpeg","size_bytes":%d,"day_id":%q,"caption":"hi"}`, size, dayID)
	status, resp, raw := e.do(t, "POST", api+"/journeys/"+slug+"/moments/uploads", tok, body)
	if status == 201 {
		d := resp["data"].(map[string]any)
		mid = d["moment"].(map[string]any)["id"].(string)
		upload = d["upload"].(map[string]any)
	}
	return
}

func headersOf(u map[string]any) map[string]string {
	h := map[string]string{}
	for k, v := range u["headers"].(map[string]any) {
		h[k] = v.(string)
	}
	return h
}

func TestMomentsHTTP_PhotoFlow(t *testing.T) {
	e := newMomentHTTPEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	slug := e.createJourney(t, alice)
	day := e.addDay(t, alice, slug)

	status, mid, up, raw := e.startUpload(t, alice, slug, day, 1000)
	require.Equal(t, 201, status, raw)
	assert.Equal(t, "PUT", up["method"])

	st, _, _ := e.rawReq(t, "PUT", up["url"].(string), headersOf(up), jpegWithExif(t))
	require.Equal(t, 200, st)

	status, resp, raw := e.do(t, "POST", api+"/journeys/"+slug+"/moments/"+mid+"/complete", alice, "")
	require.Equal(t, 200, status, raw)
	assert.Equal(t, "ready", resp["data"].(map[string]any)["status"])

	status, resp, raw = e.do(t, "GET", api+"/journeys/"+slug+"/moments", alice, "")
	require.Equal(t, 200, status, raw)
	ms := resp["data"].(map[string]any)["moments"].([]any)
	require.Len(t, ms, 1)
	u := ms[0].(map[string]any)["url"].(string)

	st, ct, body := e.rawReq(t, "GET", u, nil, nil)
	require.Equal(t, 200, st)
	assert.Equal(t, "image/jpeg", ct)
	assert.NotContains(t, string(body), "Exif")
	assert.NotContains(t, string(body), "GPS-SECRET")

	pu, err := url.Parse(u)
	require.NoError(t, err)
	q := pu.Query()
	q.Set("sig", "00"+q.Get("sig")[2:])
	pu.RawQuery = q.Encode()
	st, _, _ = e.rawReq(t, "GET", pu.String(), nil, nil)
	assert.Equal(t, 403, st)

	// /me/moments lists it with its trip; caption edit then delete.
	status, resp, raw = e.do(t, "GET", api+"/me/moments", alice, "")
	require.Equal(t, 200, status, raw)
	assert.Len(t, resp["data"].(map[string]any)["moments"], 1)
	status, resp, raw = e.do(t, "PATCH", api+"/moments/"+mid, alice, `{"caption":"new"}`)
	require.Equal(t, 200, status, raw)
	assert.Equal(t, "new", resp["data"].(map[string]any)["caption"])
	status, _, raw = e.do(t, "DELETE", api+"/moments/"+mid, alice, "")
	assert.Equal(t, 204, status, raw)
}

func TestMomentsHTTP_StatusCodes(t *testing.T) {
	e := newMomentHTTPEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	stranger := e.named(t, "carol@gmail.com", "carol")
	slug := e.trip(t, alice, "Secret", "private", liveDates)
	day := e.addDay(t, alice, slug)

	status, _, _ := e.do(t, "GET", api+"/journeys/"+slug+"/moments", stranger, "")
	assert.Equal(t, 404, status, "private trip is hidden from strangers")
	status, _, _ = e.do(t, "POST", api+"/journeys/"+slug+"/moments/uploads", "", `{}`)
	assert.Equal(t, 401, status)

	status, mid, up, raw := e.startUpload(t, alice, slug, day, 1000)
	require.Equal(t, 201, status, raw)
	st, _, _ := e.rawReq(t, "PUT", up["url"].(string), headersOf(up), jpegWithExif(t))
	require.Equal(t, 200, st)
	status, _, raw = e.do(t, "POST", api+"/journeys/"+slug+"/moments/"+mid+"/complete", alice, "")
	require.Equal(t, 200, status, raw)

	status, _, _ = e.do(t, "DELETE", api+"/moments/"+mid, stranger, "")
	assert.Equal(t, 404, status, "someone else's moment")
	status, _, _ = e.do(t, "PATCH", api+"/moments/"+mid, stranger, `{"caption":"x"}`)
	assert.Equal(t, 404, status)

	status, body, raw := e.do(t, "DELETE", api+"/journeys/"+slug+"/days/"+day, alice, "")
	assert.Equal(t, 409, status, raw)
	assert.Equal(t, "day has moments", body["message"])

	status, body, raw = e.do(t, "PUT", api+"/journeys/"+slug+"/update", alice, journeyBody)
	assert.Equal(t, 409, status, raw)
	assert.Equal(t, "trip has moments", body["message"])

	// Deleting the trip removes the moments.
	jid := e.dbJourney(t, slug).ID
	status, _, raw = e.do(t, "DELETE", api+"/journeys/"+slug, alice, "")
	require.Equal(t, 200, status, raw)
	var n int64
	require.NoError(t, e.db.Table("moments").Where("journey_id = ?", jid).Count(&n).Error)
	assert.Zero(t, n)
}

func TestMomentsHTTP_LocalPutAcceptsLargeBody(t *testing.T) {
	e := newMomentHTTPEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	slug := e.createJourney(t, alice)
	day := e.addDay(t, alice, slug)
	status, _, up, raw := e.startUpload(t, alice, slug, day, 6<<20)
	require.Equal(t, 201, status, raw)
	big := bytes.Repeat([]byte{0xAB}, 5<<20)
	st, _, body := e.rawReq(t, "PUT", up["url"].(string), headersOf(up), big)
	assert.Equal(t, 200, st, strings.TrimSpace(string(body)))
}
