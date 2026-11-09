package app_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// profile fetches GET /users/:username/profile as tok and returns its data.
func (e *env) profile(t *testing.T, tok, username string) (map[string]any, string) {
	t.Helper()
	status, body, raw := e.do(t, "GET", api+"/users/"+username+"/profile", tok, "")
	require.Equal(t, 200, status, raw)
	return body["data"].(map[string]any), raw
}

func TestHTTP_Follow(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	status, _, raw := e.do(t, "PATCH", api+"/me", alice, `{"follow_approval":false}`)
	require.Equal(t, 200, status, raw) // instant follows, as before requests existed

	for i := 0; i < 2; i++ { // idempotent
		status, body, raw := e.do(t, "POST", api+"/users/alice/follow", bob, "")
		require.Equal(t, 200, status, raw)
		assert.Equal(t, map[string]any{"is_following": true, "requested": false, "followers": 1.0}, body["data"])
	}
	p, _ := e.profile(t, bob, "alice")
	assert.Equal(t, 1.0, p["followers"])
	assert.Equal(t, true, p["is_following"])
	p, _ = e.profile(t, "", "alice")
	assert.Equal(t, false, p["is_following"], "anonymous")
	p, _ = e.profile(t, bob, "bob")
	assert.Equal(t, 1.0, p["following"])

	status, body, raw := e.do(t, "POST", api+"/users/alice/follow", alice, "")
	assert.Equal(t, 400, status, raw)
	assert.Equal(t, "cannot follow yourself", body["message"])
	status, body, _ = e.do(t, "POST", api+"/users/nobody/follow", bob, "")
	assert.Equal(t, 404, status)
	assert.Equal(t, "user not found", body["message"])
	for _, m := range []string{"POST", "DELETE"} {
		status, _, _ = e.do(t, m, api+"/users/alice/follow", "", "")
		assert.Equal(t, 401, status, m)
	}

	for i := 0; i < 2; i++ { // idempotent
		status, body, raw = e.do(t, "DELETE", api+"/users/alice/follow", bob, "")
		require.Equal(t, 200, status, raw)
		assert.Equal(t, map[string]any{"is_following": false, "requested": false, "followers": 0.0}, body["data"])
	}
	p, _ = e.profile(t, bob, "alice")
	assert.Equal(t, 0.0, p["followers"])
	assert.Equal(t, false, p["is_following"])
}

func TestHTTP_ProfileUnknownUser(t *testing.T) {
	e := newEnv(t)
	status, body, _ := e.do(t, "GET", api+"/users/nobody/profile", "", "")
	assert.Equal(t, 404, status)
	assert.Equal(t, "user not found", body["message"])
}

func TestHTTP_ProfileSections(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	carol := e.named(t, "carol@gmail.com", "carol")
	live := e.trip(t, alice, "Live", "public", liveDates)
	past := e.trip(t, alice, "Past", "public", pastDates)
	future := e.trip(t, alice, "Future", "followers", futureDates)
	e.inviteAndAccept(t, alice, carol, past, "carol", "traveler")
	e.follow(t, bob, alice)

	// Follower: now = the live trip with city and day X of Y; future in upcoming.
	p, raw := e.profile(t, bob, "alice")
	now := p["now"].(map[string]any)
	assert.Equal(t, live, now["slug"])
	liveInfo := now["live"].(map[string]any)
	assert.Equal(t, "Tokyo", liveInfo["city"])
	assert.Equal(t, 2.0, liveInfo["day_number"])
	assert.Equal(t, 5.0, liveInfo["total_days"])
	assert.NotContains(t, liveInfo, "today_plans")
	assert.Equal(t, []string{future}, slugsIn(p["upcoming"]))
	assert.Equal(t, []string{past}, slugsIn(p["past"]))
	assert.Len(t, p["calendar"], 3)
	assert.Equal(t, map[string]any{"countries": 1.0, "cities": 1.0, "trips": 1.0}, p["stats"])
	assert.Contains(t, raw, `"ongoing"`)

	// Calendar of the past trip names carol as a companion, not alice.
	for _, it := range p["calendar"].([]any) {
		item := it.(map[string]any)
		if item["slug"] == past {
			assert.Equal(t, "2026-06-01T00:00:00Z", item["departure_date"])
			assert.Equal(t, "2026-06", item["month_start"])
			companions := item["companions"].([]any)
			require.Len(t, companions, 1)
			assert.Equal(t, "carol", companions[0].(map[string]any)["username"])
		}
	}

	// carol's profile lists the joined trip.
	p, _ = e.profile(t, "", "carol")
	assert.Equal(t, []string{past}, slugsIn(p["past"]))
}

func slugsIn(list any) []string {
	out := []string{}
	for _, it := range list.([]any) {
		out = append(out, it.(map[string]any)["slug"].(string))
	}
	return out
}

func TestHTTP_ProfileHidesPrivateTripFromPendingInvitee(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	secret := e.trip(t, alice, "Secret", "private", pastDates)
	open := e.trip(t, alice, "Open", "public", pastDates)

	status, _, raw := e.do(t, "POST", api+"/journeys/"+secret+"/members", alice, `{"username":"bob","role":"traveler"}`)
	require.Equal(t, 201, status, raw)

	p, raw := e.profile(t, bob, "alice")
	assert.Equal(t, []string{open}, slugsIn(p["past"]), raw)
	assert.NotContains(t, raw, secret)
}
