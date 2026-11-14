package app_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uploadCover runs start + PUT + complete and returns the complete response.
func (e *env) uploadCover(t *testing.T, tok, slug string, file []byte) (int, map[string]any, string) {
	t.Helper()
	body := fmt.Sprintf(`{"content_type":"image/jpeg","size_bytes":%d}`, len(file))
	status, resp, raw := e.do(t, "POST", api+"/journeys/"+slug+"/cover/uploads", tok, body)
	require.Equal(t, 201, status, raw)
	d := resp["data"].(map[string]any)
	id := d["upload_id"].(string)
	require.Len(t, id, 12)
	up := d["upload"].(map[string]any)
	st, _, _ := e.rawReq(t, "PUT", up["url"].(string), headersOf(up), file)
	require.Equal(t, 200, st)
	return e.do(t, "POST", api+"/journeys/"+slug+"/cover/complete", tok, `{"upload_id":"`+id+`"}`)
}

func (e *env) tripCover(t *testing.T, tok, slug string) string {
	t.Helper()
	status, resp, raw := e.do(t, "GET", api+"/journeys/"+slug, tok, "")
	require.Equal(t, 200, status, raw)
	u, _ := resp["data"].(map[string]any)["cover_url"].(string)
	return u
}

func TestCoverHTTP_UploadReplaceRemove(t *testing.T) {
	e := newMomentHTTPEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	slug := e.trip(t, alice, "Tokyo", "public", pastDates)
	assert.Empty(t, e.tripCover(t, alice, slug), "no cover yet: the web app draws the gradient")

	status, resp, raw := e.uploadCover(t, alice, slug, jpegWithExif(t))
	require.Equal(t, 200, status, raw)
	first := resp["data"].(map[string]any)["cover_url"].(string)
	require.NotEmpty(t, first)

	// The stored cover is re-encoded: no EXIF/GPS.
	st, ct, body := e.rawReq(t, "GET", first, nil, nil)
	require.Equal(t, 200, st)
	assert.Equal(t, "image/jpeg", ct)
	assert.NotContains(t, string(body), "GPS-SECRET")

	// Everyone who can see the trip gets the cover: anonymous trip page, public list, my trips.
	assert.Equal(t, first, e.tripCover(t, "", slug))
	status, list, raw := e.do(t, "GET", api+"/journeys", "", "")
	require.Equal(t, 200, status, raw)
	item := list["data"].([]any)[0].(map[string]any)["plan"].([]any)[0].(map[string]any)
	assert.Equal(t, first, item["cover_url"])
	status, mine, raw := e.do(t, "GET", api+"/me/journeys", alice, "")
	require.Equal(t, 200, status, raw)
	assert.Equal(t, first, mine["data"].([]any)[0].(map[string]any)["cover_url"])

	// A linked cover_url can't silently hide behind the uploaded one.
	status, _, raw = e.do(t, "PATCH", api+"/journeys/"+slug, alice, `{"cover_url":"https://img.example/x.jpg"}`)
	assert.Equal(t, 409, status, raw)

	// Replacing deletes the old file.
	status, resp, raw = e.uploadCover(t, alice, slug, jpegWithExif(t))
	require.Equal(t, 200, status, raw)
	second := resp["data"].(map[string]any)["cover_url"].(string)
	assert.NotEqual(t, first, second)
	st, _, _ = e.rawReq(t, "GET", first, nil, nil)
	assert.Equal(t, 404, st)

	// Removing goes back to no cover and deletes the file.
	status, _, raw = e.do(t, "DELETE", api+"/journeys/"+slug+"/cover", alice, "")
	require.Equal(t, 200, status, raw)
	assert.Empty(t, e.tripCover(t, alice, slug))
	st, _, _ = e.rawReq(t, "GET", second, nil, nil)
	assert.Equal(t, 404, st)
}

func TestCoverHTTP_OnlyTheOwner(t *testing.T) {
	e := newMomentHTTPEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	carol := e.named(t, "carol@gmail.com", "carol")
	slug := e.trip(t, alice, "Tokyo", "public", pastDates)
	e.inviteAndAccept(t, alice, bob, slug, "bob", "editor")

	body := `{"content_type":"image/jpeg","size_bytes":1000}`
	for name, tok := range map[string]string{"editor": bob, "stranger": carol, "anonymous": ""} {
		status, _, _ := e.do(t, "POST", api+"/journeys/"+slug+"/cover/uploads", tok, body)
		assert.Contains(t, []int{401, 403, 404}, status, name)
		status, _, _ = e.do(t, "DELETE", api+"/journeys/"+slug+"/cover", tok, "")
		assert.Contains(t, []int{401, 403, 404}, status, name)
	}
}

func TestCoverHTTP_RejectsBadInput(t *testing.T) {
	e := newMomentHTTPEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	slug := e.trip(t, alice, "Tokyo", "public", pastDates)

	for _, body := range []string{
		`{"content_type":"image/gif","size_bytes":1000}`,
		`{"content_type":"image/jpeg","size_bytes":0}`,
		`{"content_type":"image/jpeg","size_bytes":10485761}`,
	} {
		status, _, raw := e.do(t, "POST", api+"/journeys/"+slug+"/cover/uploads", alice, body)
		assert.Equal(t, 400, status, raw)
	}

	// A 10-character ID is a moment's raw upload: a cover can never claim it.
	status, _, raw := e.do(t, "POST", api+"/journeys/"+slug+"/cover/complete", alice, `{"upload_id":"abcdefghij"}`)
	assert.Equal(t, 400, status, raw)

	// Not an image.
	status, resp, raw := e.uploadCover(t, alice, slug, []byte("definitely not a picture"))
	assert.Equal(t, 400, status, raw)
	assert.Contains(t, raw, "unsupported file format")
	_ = resp
	assert.Empty(t, e.tripCover(t, alice, slug))
}

func TestCoverHTTP_CompletingTwiceKeepsALiveCover(t *testing.T) {
	e := newMomentHTTPEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	slug := e.trip(t, alice, "Tokyo", "public", pastDates)
	file := jpegWithExif(t)

	status, resp, raw := e.do(t, "POST", api+"/journeys/"+slug+"/cover/uploads", alice,
		fmt.Sprintf(`{"content_type":"image/jpeg","size_bytes":%d}`, len(file)))
	require.Equal(t, 201, status, raw)
	d := resp["data"].(map[string]any)
	up := d["upload"].(map[string]any)
	st, _, _ := e.rawReq(t, "PUT", up["url"].(string), headersOf(up), file)
	require.Equal(t, 200, st)
	body := `{"upload_id":"` + d["upload_id"].(string) + `"}`
	status, _, raw = e.do(t, "POST", api+"/journeys/"+slug+"/cover/complete", alice, body)
	require.Equal(t, 200, status, raw)
	// A retry of the same complete finds no raw file any more and changes nothing.
	status, _, raw = e.do(t, "POST", api+"/journeys/"+slug+"/cover/complete", alice, body)
	assert.Equal(t, 400, status, raw)

	st, _, _ = e.rawReq(t, "GET", e.tripCover(t, alice, slug), nil, nil)
	assert.Equal(t, 200, st, "the cover file the trip points at still exists")
}
