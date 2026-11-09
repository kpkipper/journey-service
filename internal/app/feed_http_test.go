package app_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dataOf(body map[string]any) map[string]any {
	d, _ := body["data"].(map[string]any)
	return d
}

func TestHTTP_BlockFlow(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")

	status, _, raw := e.do(t, "POST", api+"/users/bob/block", "", "")
	assert.Equal(t, 401, status, raw)
	status, _, raw = e.do(t, "POST", api+"/users/alice/block", alice, "")
	assert.Equal(t, 400, status, raw)
	status, _, raw = e.do(t, "POST", api+"/users/bob/block", alice, "")
	assert.Equal(t, 204, status, raw)
	status, _, raw = e.do(t, "POST", api+"/users/bob/block", alice, "")
	assert.Equal(t, 204, status, raw)

	status, body, raw := e.do(t, "POST", api+"/users/alice/follow", bob, "")
	assert.Equal(t, 404, status, raw)
	assert.Equal(t, "user not found", body["message"])
	status, _, raw = e.do(t, "GET", api+"/users/alice", bob, "")
	assert.Equal(t, 404, status, raw)
	status, _, raw = e.do(t, "GET", api+"/users/alice/profile", bob, "")
	assert.Equal(t, 404, status, raw)

	status, body, raw = e.do(t, "GET", api+"/me/blocks", alice, "")
	require.Equal(t, 200, status, raw)
	assert.Contains(t, raw, `"bob"`)
	assert.Len(t, dataOf(body)["users"], 1)

	status, _, raw = e.do(t, "DELETE", api+"/users/bob/block", alice, "")
	assert.Equal(t, 204, status, raw)
	status, _, raw = e.do(t, "DELETE", api+"/users/bob/block", alice, "")
	assert.Equal(t, 204, status, raw)
	status, _, raw = e.do(t, "POST", api+"/users/alice/follow", bob, "")
	assert.Equal(t, 200, status, raw)
}

func TestHTTP_Feed(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	status, _, raw := e.do(t, "GET", api+"/feed", "", "")
	assert.Equal(t, 401, status, raw)
	status, _, raw = e.do(t, "GET", api+"/feed/now", "", "")
	assert.Equal(t, 401, status, raw)

	slug := e.trip(t, alice, "Live", "public", liveDates)
	e.follow(t, bob, alice)

	status, body, raw := e.do(t, "GET", api+"/feed", bob, "")
	require.Equal(t, 200, status, raw)
	items := dataOf(body)["items"].([]any)
	require.NotEmpty(t, items, raw)
	assert.Contains(t, raw, slug)

	status, body, raw = e.do(t, "GET", api+"/feed?limit=1", bob, "")
	require.Equal(t, 200, status, raw)
	assert.Len(t, dataOf(body)["items"], 1)
	require.NotNil(t, dataOf(body)["next_before"])

	for _, q := range []string{"limit=abc", "limit=1.5", "before=!!!"} {
		status, _, raw = e.do(t, "GET", api+"/feed?"+q, bob, "")
		assert.Equal(t, 400, status, q+raw)
	}

	status, body, raw = e.do(t, "GET", api+"/feed/now", bob, "")
	require.Equal(t, 200, status, raw)
	tr := dataOf(body)["travelers"].([]any)
	require.Len(t, tr, 1, raw)
	assert.Equal(t, "alice", tr[0].(map[string]any)["user"].(map[string]any)["username"])
}

func TestHTTP_Report(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	open := e.trip(t, alice, "Open", "public", pastDates)
	secret := e.trip(t, alice, "Secret", "private", pastDates)

	req := `{"target_type":"journey","target":"` + open + `","reason":"spam"}`
	status, _, raw := e.do(t, "POST", api+"/reports", "", req)
	assert.Equal(t, 401, status, raw)
	status, body, raw := e.do(t, "POST", api+"/reports", bob, req)
	require.Equal(t, 201, status, raw)
	d := dataOf(body)
	assert.ElementsMatch(t, []string{"id", "target_type", "reason", "status", "created_at"}, keysOf(d))
	assert.Equal(t, "open", d["status"])
	status, body, raw = e.do(t, "POST", api+"/reports", bob, req)
	assert.Equal(t, 200, status, raw)
	assert.Equal(t, d["id"], dataOf(body)["id"])

	status, _, raw = e.do(t, "POST", api+"/reports", bob, `{"target_type":"journey","target":"`+secret+`","reason":"spam"}`)
	assert.Equal(t, 404, status, raw)
	status, _, raw = e.do(t, "POST", api+"/reports", bob, `{"target_type":"journey","target":"`+open+`","reason":"nope"}`)
	assert.Equal(t, 400, status, raw)
	status, _, raw = e.do(t, "POST", api+"/reports", bob, `{bad`)
	assert.Equal(t, 400, status, raw)
}

// Empty lists must serialise as [] (never null): the contract is an array.
func TestHTTP_EmptyListsAreArrays(t *testing.T) {
	e := newEnv(t)
	fresh := e.named(t, "fresh@gmail.com", "fresh")
	owner := e.named(t, "owner@gmail.com", "owner")
	slug := e.trip(t, owner, "Quiet", "public", liveDates)

	for _, c := range []struct{ path, want string }{
		{"/feed", `"items":[]`},
		{"/feed/now", `"travelers":[]`},
		{"/me/blocks", `"users":[]`},
		{"/me/moments", `"moments":[]`},
		{"/journeys/" + slug + "/moments", `"moments":[]`},
	} {
		status, _, raw := e.do(t, "GET", api+c.path, fresh, "")
		require.Equal(t, 200, status, c.path+raw)
		assert.Contains(t, raw, c.want, c.path)
		assert.NotContains(t, raw, "null,", c.path) // next_before is the only null, and it trails
	}
}
