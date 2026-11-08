package app_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const api = "/api/v1"

// named logs in email and sets a username, returning the token.
func (e *env) named(t *testing.T, email, username string) string {
	t.Helper()
	tok := e.login(t, email)
	status, _, raw := e.do(t, "PATCH", api+"/me", tok, `{"username":"`+username+`"}`)
	require.Equal(t, 200, status, raw)
	return tok
}

func (e *env) members(t *testing.T, slug string) []map[string]any {
	t.Helper()
	status, body, raw := e.do(t, "GET", api+"/journeys/"+slug, "", "")
	require.Equal(t, 200, status, raw)
	var out []map[string]any
	for _, m := range body["data"].(map[string]any)["members"].([]any) {
		out = append(out, m.(map[string]any))
	}
	return out
}

func memberByUsername(ms []map[string]any, username string) map[string]any {
	for _, m := range ms {
		if m["username"] == username {
			return m
		}
	}
	return nil
}

func (e *env) addDay(t *testing.T, tok, slug string) string {
	t.Helper()
	status, body, raw := e.do(t, "POST", api+"/journeys/"+slug+"/days", tok,
		`{"date":"1 Jun","date_iso":"2026-06-01T00:00:00Z","title":"Day 1"}`)
	require.Equal(t, 201, status, raw)
	return body["data"].(map[string]any)["id"].(string)
}

// invite invites username with role and has that user accept; returns the invite id via accept.
func (e *env) inviteAndAccept(t *testing.T, owner, invitee, slug, username, role string) {
	t.Helper()
	status, _, raw := e.do(t, "POST", api+"/journeys/"+slug+"/members", owner, `{"username":"`+username+`","role":"`+role+`"}`)
	require.Equal(t, 201, status, raw)
	status, body, raw := e.do(t, "GET", api+"/me/invites", invitee, "")
	require.Equal(t, 200, status, raw)
	invites := body["data"].([]any)
	require.NotEmpty(t, invites)
	id := invites[len(invites)-1].(map[string]any)["id"].(string)
	status, _, raw = e.do(t, "POST", api+"/me/invites/"+id+"/accept", invitee, "")
	require.Equal(t, 200, status, raw)
}

func TestHTTP_InviteFlow(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	slug := e.createJourney(t, alice)

	status, _, raw := e.do(t, "POST", api+"/journeys/"+slug+"/members", alice, `{"username":"bob","role":"editor"}`)
	require.Equal(t, 201, status, raw)
	assert.Nil(t, memberByUsername(e.members(t, slug), "bob"), "invited user is not public yet")

	status, body, raw := e.do(t, "GET", api+"/me/invites", bob, "")
	require.Equal(t, 200, status, raw)
	invites := body["data"].([]any)
	require.Len(t, invites, 1)
	inv := invites[0].(map[string]any)
	assert.Equal(t, slug, inv["journey_slug"])
	assert.Equal(t, "editor", inv["role"])

	status, _, raw = e.do(t, "POST", api+"/me/invites/"+inv["id"].(string)+"/accept", bob, "")
	require.Equal(t, 200, status, raw)
	bobView := memberByUsername(e.members(t, slug), "bob")
	require.NotNil(t, bobView)
	assert.Equal(t, "editor", bobView["role"])
	assert.NotNil(t, memberByUsername(e.members(t, slug), "alice"), "owner is listed")

	e.addDay(t, bob, slug)

	// duplicate invite and unknown user
	status, _, _ = e.do(t, "POST", api+"/journeys/"+slug+"/members", alice, `{"username":"bob","role":"traveler"}`)
	assert.Equal(t, 409, status)
	status, _, _ = e.do(t, "POST", api+"/journeys/"+slug+"/members", alice, `{"username":"nobody","role":"traveler"}`)
	assert.Equal(t, 404, status)
}

func TestHTTP_DeclineInvite(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	slug := e.createJourney(t, alice)
	status, _, _ := e.do(t, "POST", api+"/journeys/"+slug+"/members", alice, `{"username":"bob","role":"traveler"}`)
	require.Equal(t, 201, status)
	_, body, _ := e.do(t, "GET", api+"/me/invites", bob, "")
	id := body["data"].([]any)[0].(map[string]any)["id"].(string)

	status, _, _ = e.do(t, "POST", api+"/me/invites/"+id+"/decline", bob, "")
	assert.Equal(t, 200, status)
	status, _, _ = e.do(t, "POST", api+"/me/invites/"+id+"/accept", bob, "")
	assert.Equal(t, 404, status)
	_, body, _ = e.do(t, "GET", api+"/me/invites", bob, "")
	assert.Empty(t, body["data"])
}

func TestHTTP_InvitedCannotEdit(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	slug := e.createJourney(t, alice)
	status, _, _ := e.do(t, "POST", api+"/journeys/"+slug+"/members", alice, `{"username":"bob","role":"editor"}`)
	require.Equal(t, 201, status)

	status, _, _ = e.do(t, "POST", api+"/journeys/"+slug+"/days", bob,
		`{"date":"1 Jun","date_iso":"2026-06-01T00:00:00Z"}`)
	assert.Equal(t, 403, status)
}

func TestHTTP_InviteLinkFlow(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	carol := e.named(t, "carol@gmail.com", "carol")
	slug := e.createJourney(t, alice)
	dayID := e.addDay(t, alice, slug)
	status, body, raw := e.do(t, "POST", api+"/journeys/"+slug+"/days/"+dayID+"/plans", alice, `{"description":"Dinner"}`)
	require.Equal(t, 201, status, raw)
	planID := body["data"].(map[string]any)["id"].(string)

	status, body, raw = e.do(t, "POST", api+"/journeys/"+slug+"/invite-links", alice, `{"role":"traveler"}`)
	require.Equal(t, 201, status, raw)
	data := body["data"].(map[string]any)
	url := data["url"].(string)
	require.Contains(t, url, "http://web.test/join/")
	token := url[len("http://web.test/join/"):]
	linkID := data["link"].(map[string]any)["id"].(string)

	status, body, raw = e.do(t, "GET", api+"/journeys/"+slug+"/invite-links", alice, "")
	require.Equal(t, 200, status, raw)
	assert.Len(t, body["data"], 1)
	assert.NotContains(t, raw, token, "raw token is never listed")

	status, body, raw = e.do(t, "POST", api+"/invites/join", carol, `{"token":"`+token+`"}`)
	require.Equal(t, 200, status, raw)
	assert.Equal(t, slug, body["data"].(map[string]any)["slug"])
	c := memberByUsername(e.members(t, slug), "carol")
	require.NotNil(t, c)
	assert.Equal(t, "traveler", c["role"])

	status, _, _ = e.do(t, "PATCH", api+"/journeys/"+slug+"/plans/"+planID, carol, `{"description":"x"}`)
	assert.Equal(t, 403, status)

	// revoked link no longer works
	dave := e.named(t, "dave@gmail.com", "dave")
	status, _, _ = e.do(t, "DELETE", api+"/journeys/"+slug+"/invite-links/"+linkID, alice, "")
	require.Equal(t, 200, status)
	status, _, _ = e.do(t, "POST", api+"/invites/join", dave, `{"token":"`+token+`"}`)
	assert.Equal(t, 404, status)
}

func TestHTTP_MembersEndpointNeedsMembership(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	eve := e.named(t, "eve@gmail.com", "eve")
	slug := e.createJourney(t, alice)

	status, _, _ := e.do(t, "GET", api+"/journeys/"+slug+"/members", eve, "")
	assert.Equal(t, 403, status)
	status, body, raw := e.do(t, "GET", api+"/journeys/"+slug+"/members", alice, "")
	require.Equal(t, 200, status, raw)
	assert.Len(t, body["data"], 1)
}

func TestHTTP_ChangeRoleRemoveLeave(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	carol := e.named(t, "carol@gmail.com", "carol")
	slug := e.createJourney(t, alice)
	e.inviteAndAccept(t, alice, bob, slug, "bob", "traveler")
	e.inviteAndAccept(t, alice, carol, slug, "carol", "traveler")

	status, _, raw := e.do(t, "PATCH", api+"/journeys/"+slug+"/members/bob", alice, `{"role":"editor"}`)
	require.Equal(t, 200, status, raw)
	assert.Equal(t, "editor", memberByUsername(e.members(t, slug), "bob")["role"])
	status, _, _ = e.do(t, "PATCH", api+"/journeys/"+slug+"/members/bob", bob, `{"role":"traveler"}`)
	assert.Equal(t, 403, status)

	status, _, _ = e.do(t, "DELETE", api+"/journeys/"+slug+"/members/carol", alice, "")
	assert.Equal(t, 200, status)
	assert.Nil(t, memberByUsername(e.members(t, slug), "carol"))
	status, _, _ = e.do(t, "DELETE", api+"/journeys/"+slug+"/members/carol", alice, "")
	assert.Equal(t, 404, status)

	status, _, _ = e.do(t, "POST", api+"/journeys/"+slug+"/leave", bob, "")
	assert.Equal(t, 200, status)
	assert.Nil(t, memberByUsername(e.members(t, slug), "bob"))
	status, _, _ = e.do(t, "POST", api+"/journeys/"+slug+"/leave", alice, "")
	assert.Equal(t, 400, status, "owner cannot leave")
}

func TestHTTP_MyJourneys(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	own := e.createJourney(t, alice)
	joined := e.createJourney(t, bob)
	pending := e.createJourney(t, bob)
	e.inviteAndAccept(t, bob, alice, joined, "alice", "traveler")
	status, _, _ := e.do(t, "POST", api+"/journeys/"+pending+"/members", bob, `{"username":"alice","role":"traveler"}`)
	require.Equal(t, 201, status)

	status, body, raw := e.do(t, "GET", api+"/me/journeys", alice, "")
	require.Equal(t, 200, status, raw)
	items := body["data"].([]any)
	require.Len(t, items, 2, raw)
	roles := map[string]string{}
	for _, it := range items {
		m := it.(map[string]any)
		roles[m["slug"].(string)] = m["role"].(string)
		assert.Equal(t, "Japan", m["title"])
		assert.Contains(t, m, "cover_url")
		assert.Contains(t, m, "departure_date")
	}
	assert.Equal(t, map[string]string{own: "owner", joined: "traveler"}, roles)
}

func TestHTTP_MyJourneysSortedByDeparture(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	old := e.createJourney(t, alice)
	status, _, raw := e.do(t, "PATCH", api+"/journeys/"+old, alice, `{"departure_date":"2025-01-01T00:00:00Z","return_date":"2025-01-05T00:00:00Z"}`)
	require.Equal(t, 200, status, raw)
	recent := e.createJourney(t, alice)

	_, body, _ := e.do(t, "GET", api+"/me/journeys", alice, "")
	items := body["data"].([]any)
	require.Len(t, items, 2)
	assert.Equal(t, recent, items[0].(map[string]any)["slug"])
	assert.Equal(t, old, items[1].(map[string]any)["slug"])
}

func TestHTTP_ExistingRoutesStillWork(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	slug := e.createJourney(t, alice)
	status, _, raw := e.do(t, "PUT", api+"/journeys/"+slug+"/update", alice, journeyBody)
	assert.Equal(t, 200, status, raw)
	status, _, _ = e.do(t, "GET", api+"/journeys/"+slug, "", "")
	assert.Equal(t, 200, status)
	status, _, _ = e.do(t, "GET", api+"/journeys/nope", "", "")
	assert.Equal(t, 404, status)
}

func TestHTTP_NewEndpointsRequireAuth(t *testing.T) {
	e := newEnv(t)
	cases := []struct{ method, path, body string }{
		{"PATCH", "/journeys/s", `{}`},
		{"POST", "/journeys/s/days", `{}`},
		{"PATCH", "/journeys/s/days/d", `{}`},
		{"DELETE", "/journeys/s/days/d", ""},
		{"POST", "/journeys/s/days/d/plans", `{}`},
		{"PUT", "/journeys/s/days/d/plans/order", `{"plan_ids":[]}`},
		{"PATCH", "/journeys/s/plans/p", `{}`},
		{"DELETE", "/journeys/s/plans/p", ""},
		{"GET", "/journeys/s/members", ""},
		{"POST", "/journeys/s/members", `{}`},
		{"PATCH", "/journeys/s/members/u", `{}`},
		{"DELETE", "/journeys/s/members/u", ""},
		{"POST", "/journeys/s/leave", ""},
		{"GET", "/journeys/s/invite-links", ""},
		{"POST", "/journeys/s/invite-links", `{}`},
		{"DELETE", "/journeys/s/invite-links/i", ""},
		{"POST", "/invites/join", `{}`},
		{"GET", "/me/invites", ""},
		{"POST", "/me/invites/i/accept", ""},
		{"POST", "/me/invites/i/decline", ""},
		{"GET", "/me/journeys", ""},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			status, _, raw := e.do(t, c.method, api+c.path, "", c.body)
			assert.Equal(t, 401, status, raw)
		})
	}
}
