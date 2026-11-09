package app_test

import (
	"context"
	"testing"

	"github.com/kpkipper/journey-service/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (e *env) items(t *testing.T, tok, path string) []any {
	t.Helper()
	status, body, raw := e.do(t, "GET", api+path, tok, "")
	require.Equal(t, 200, status, raw)
	return body["data"].(map[string]any)["items"].([]any)
}

func TestHTTP_FollowRequests(t *testing.T) {
	setup := func(t *testing.T) (e *env, alice, bob, slug string) {
		e = newEnv(t)
		alice = e.named(t, "alice@gmail.com", "alice")
		bob = e.named(t, "bob@gmail.com", "bob")
		slug = e.trip(t, alice, "Live", "followers", liveDates)
		return
	}
	// pending reports whether bob is treated as a non-follower.
	stranger := func(t *testing.T, e *env, bob, slug string) {
		status, _, raw := e.do(t, "GET", api+"/journeys/"+slug, bob, "")
		assert.Equal(t, 404, status, raw)
		status, _, raw = e.do(t, "GET", api+"/journeys/"+slug+"/moments", bob, "")
		assert.Equal(t, 404, status, raw)
	}
	follower := func(t *testing.T, e *env, bob, slug string) {
		status, body, raw := e.do(t, "GET", api+"/journeys/"+slug, bob, "")
		require.Equal(t, 200, status, raw)
		assert.Equal(t, false, body["data"].(map[string]any)["redacted"])
		status, _, raw = e.do(t, "GET", api+"/journeys/"+slug+"/moments", bob, "")
		assert.Equal(t, 200, status, raw)
	}

	t.Run("request, accept, remove", func(t *testing.T) {
		e, alice, bob, slug := setup(t)
		status, body, raw := e.do(t, "POST", api+"/users/alice/follow", bob, "")
		require.Equal(t, 200, status, raw)
		assert.Equal(t, map[string]any{"is_following": false, "requested": true, "followers": 0.0}, body["data"])
		stranger(t, e, bob, slug)

		status, body, raw = e.do(t, "GET", api+"/me/follow-requests", alice, "")
		require.Equal(t, 200, status, raw)
		d := body["data"].(map[string]any)
		assert.Equal(t, 1.0, d["count"])
		require.Len(t, d["items"], 1)
		assert.Equal(t, "bob", d["items"].([]any)[0].(map[string]any)["user"].(map[string]any)["username"])
		assert.Empty(t, e.items(t, alice, "/me/followers"))

		p, _ := e.profile(t, bob, "alice")
		assert.Equal(t, true, p["follow_requested"])
		assert.Equal(t, true, p["follow_approval"])
		assert.Equal(t, false, p["is_following"])

		status, body, raw = e.do(t, "POST", api+"/me/follow-requests/bob/accept", alice, "")
		require.Equal(t, 200, status, raw)
		assert.Equal(t, map[string]any{"accepted": true}, body["data"])
		follower(t, e, bob, slug)
		assert.Empty(t, e.items(t, alice, "/me/follow-requests"))
		assert.Len(t, e.items(t, alice, "/me/followers"), 1)
		p, _ = e.profile(t, bob, "alice")
		assert.Equal(t, false, p["follow_requested"])
		assert.Equal(t, true, p["is_following"])

		status, body, raw = e.do(t, "DELETE", api+"/me/followers/bob", alice, "")
		require.Equal(t, 200, status, raw)
		assert.Equal(t, map[string]any{"removed": true}, body["data"])
		stranger(t, e, bob, slug)
		assert.Empty(t, e.items(t, alice, "/me/followers"))
	})

	t.Run("pending requester sees no now, feed or traveler until accepted", func(t *testing.T) {
		e, alice, bob, slug := setup(t)
		e.do(t, "POST", api+"/users/alice/follow", bob, "")
		seen := func() (now any, feed, travelers string) {
			p, _ := e.profile(t, bob, "alice")
			status, _, rawFeed := e.do(t, "GET", api+"/feed", bob, "")
			require.Equal(t, 200, status, rawFeed)
			status, body, rawNow := e.do(t, "GET", api+"/feed/now", bob, "")
			require.Equal(t, 200, status, rawNow)
			tr := dataOf(body)["travelers"].([]any)
			names := ""
			for _, x := range tr {
				names += x.(map[string]any)["user"].(map[string]any)["username"].(string) + ","
			}
			return p["now"], rawFeed, names
		}
		now, feed, travelers := seen()
		assert.Nil(t, now)
		assert.NotContains(t, feed, slug)
		assert.NotContains(t, travelers, "alice")

		status, _, raw := e.do(t, "POST", api+"/me/follow-requests/bob/accept", alice, "")
		require.Equal(t, 200, status, raw)
		now, feed, travelers = seen()
		assert.NotNil(t, now)
		assert.Contains(t, feed, slug)
		assert.Contains(t, travelers, "alice")
	})

	t.Run("decline and cancel", func(t *testing.T) {
		e, alice, bob, slug := setup(t)
		e.do(t, "POST", api+"/users/alice/follow", bob, "")
		status, body, raw := e.do(t, "DELETE", api+"/me/follow-requests/bob", alice, "")
		require.Equal(t, 200, status, raw)
		assert.Equal(t, map[string]any{"declined": true}, body["data"])
		stranger(t, e, bob, slug)
		assert.Empty(t, e.items(t, alice, "/me/follow-requests"))
		assert.Empty(t, e.items(t, alice, "/me/followers"))

		e.do(t, "POST", api+"/users/alice/follow", bob, "")
		status, body, raw = e.do(t, "DELETE", api+"/users/alice/follow", bob, "")
		require.Equal(t, 200, status, raw)
		assert.Equal(t, map[string]any{"is_following": false, "requested": false, "followers": 0.0}, body["data"])
		assert.Empty(t, e.items(t, alice, "/me/follow-requests"))
	})

	t.Run("turning approval off accepts pending, later follows are instant", func(t *testing.T) {
		e, alice, bob, slug := setup(t)
		carol := e.named(t, "carol@gmail.com", "carol")
		e.do(t, "POST", api+"/users/alice/follow", bob, "")
		status, body, raw := e.do(t, "PATCH", api+"/me", alice, `{"follow_approval":false}`)
		require.Equal(t, 200, status, raw)
		assert.Equal(t, false, body["data"].(map[string]any)["follow_approval"])
		follower(t, e, bob, slug)
		assert.Empty(t, e.items(t, alice, "/me/follow-requests"))

		status, body, raw = e.do(t, "POST", api+"/users/alice/follow", carol, "")
		require.Equal(t, 200, status, raw)
		assert.Equal(t, true, body["data"].(map[string]any)["is_following"])
		follower(t, e, carol, slug)
	})

	t.Run("blocked users disappear and cannot be accepted", func(t *testing.T) {
		e, alice, bob, _ := setup(t)
		e.do(t, "POST", api+"/users/alice/follow", bob, "")
		carol := e.named(t, "carol@gmail.com", "carol")
		e.follow(t, carol, alice)
		e.block(t, alice, bob)
		assert.Empty(t, e.items(t, alice, "/me/follow-requests"))
		status, _, raw := e.do(t, "POST", api+"/me/follow-requests/bob/accept", alice, "")
		assert.Equal(t, 404, status, raw)
		e.block(t, alice, carol)
		assert.Empty(t, e.items(t, alice, "/me/followers"))
		status, _, raw = e.do(t, "DELETE", api+"/me/followers/carol", alice, "")
		assert.Equal(t, 404, status, raw)
		status, _, raw = e.do(t, "DELETE", api+"/me/follow-requests/bob", alice, "")
		assert.Equal(t, 404, status, raw)
		status, _, raw = e.do(t, "DELETE", api+"/users/bob/follow", alice, "")
		assert.Equal(t, 404, status, raw)
	})

	t.Run("count matches the block-filtered items", func(t *testing.T) {
		e, alice, bob, _ := setup(t)
		carol := e.named(t, "carol@gmail.com", "carol")
		e.do(t, "POST", api+"/users/alice/follow", bob, "")
		e.do(t, "POST", api+"/users/alice/follow", carol, "")
		// block row only (no cleanup), as if raced
		require.NoError(t, repository.NewBlockRepository(e.db).Block(context.Background(), e.userID(t, alice), e.userID(t, bob)))
		status, body, raw := e.do(t, "GET", api+"/me/follow-requests", alice, "")
		require.Equal(t, 200, status, raw)
		d := dataOf(body)
		assert.Len(t, d["items"], 1)
		assert.Equal(t, 1.0, d["count"])
	})

	t.Run("auth and unknown", func(t *testing.T) {
		e, alice, _, _ := setup(t)
		for _, r := range [][2]string{{"GET", "/me/follow-requests"}, {"GET", "/me/followers"}, {"POST", "/me/follow-requests/bob/accept"}, {"DELETE", "/me/follow-requests/bob"}, {"DELETE", "/me/followers/bob"}} {
			status, _, raw := e.do(t, r[0], api+r[1], "", "")
			assert.Equal(t, 401, status, r[0]+r[1]+raw)
		}
		status, _, raw := e.do(t, "POST", api+"/me/follow-requests/nobody/accept", alice, "")
		assert.Equal(t, 404, status, raw)
	})
}
