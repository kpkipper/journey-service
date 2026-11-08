package app_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The test clock is 2026-11-09 12:00 UTC (see newEnv).
const (
	liveDates   = `"departure_date":"2026-11-08T00:00:00Z","return_date":"2026-11-12T00:00:00Z"`
	pastDates   = `"departure_date":"2026-06-01T00:00:00Z","return_date":"2026-06-07T00:00:00Z"`
	futureDates = `"departure_date":"2027-03-01T00:00:00Z","return_date":"2027-03-05T00:00:00Z"`
)

// trip creates a trip with the given visibility and dates and returns its slug.
func (e *env) trip(t *testing.T, tok, title, visibility, dates string) string {
	t.Helper()
	body := fmt.Sprintf(`{"title":%q,"destination":"Tokyo","country":"Japan","visibility":%q,%s}`, title, visibility, dates)
	status, resp, raw := e.do(t, "POST", api+"/journeys/create", tok, body)
	require.Equal(t, 201, status, raw)
	return resp["data"].(map[string]any)["slug"].(string)
}

func (e *env) follow(t *testing.T, follower, followee string) {
	t.Helper()
	require.NoError(t, repository.NewFollowRepository(e.db).Follow(context.Background(), e.userID(t, follower), e.userID(t, followee)))
}

// listSlugs returns every slug in GET /journeys for the token.
func (e *env) listSlugs(t *testing.T, tok string) []string {
	t.Helper()
	status, body, raw := e.do(t, "GET", api+"/journeys", tok, "")
	require.Equal(t, 200, status, raw)
	var out []string
	for _, g := range body["data"].([]any) {
		for _, it := range g.(map[string]any)["plan"].([]any) {
			out = append(out, it.(map[string]any)["slug"].(string))
		}
	}
	return out
}

func TestHTTP_StrangerSweep(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	carol := e.named(t, "carol@gmail.com", "carol")
	followersSlug := e.trip(t, alice, "Followers Live", "followers", liveDates)
	publicSlug := e.trip(t, alice, "Public Live", "public", liveDates)

	for name, tok := range map[string]string{"anonymous": "", "stranger": carol} {
		t.Run(name, func(t *testing.T) {
			// GET /journeys
			_, _, list := e.do(t, "GET", api+"/journeys", tok, "")
			assert.NotContains(t, list, followersSlug)
			assert.Contains(t, list, publicSlug)

			// GET /journeys/:slug of the followers trip: nothing about it leaks.
			status, _, raw := e.do(t, "GET", api+"/journeys/"+followersSlug, tok, "")
			assert.Equal(t, 404, status)
			assert.Contains(t, raw, "journey not found")

			// GET /journeys/:slug/members (anonymous is stopped by login, stranger by 404).
			status, _, rawM := e.do(t, "GET", api+"/journeys/"+followersSlug+"/members", tok, "")
			if tok == "" {
				assert.Equal(t, 401, status)
			} else {
				assert.Equal(t, 404, status)
			}

			// The redacted public trip: no dates, no "ongoing", no days or members.
			_, _, pub := e.do(t, "GET", api+"/journeys/"+publicSlug, tok, "")

			// The profile: the live public trip is only a redacted "upcoming" item.
			profile, prof := e.profile(t, tok, "alice")
			assert.Nil(t, profile["now"])
			assert.Equal(t, []string{publicSlug}, slugsIn(profile["upcoming"]))
			assert.Equal(t, []string{publicSlug}, slugsIn(profile["calendar"]))
			assert.Empty(t, profile["past"])
			assert.Equal(t, map[string]any{"countries": 0.0, "cities": 0.0, "trips": 0.0}, profile["stats"])

			for _, body := range []string{list, raw, rawM, pub, prof} {
				assert.NotContains(t, body, followersSlug)
				assert.NotContains(t, body, "ongoing")
				assert.NotContains(t, body, "2026-11")
				assert.NotContains(t, body, "departure_date")
				assert.NotContains(t, body, "return_date")
				assert.NotContains(t, body, "month_start")
			}
			assert.Contains(t, pub, `"redacted":true`)
			assert.Contains(t, pub, `"upcoming"`)
		})
	}
}

func TestHTTP_HiddenIs404(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	slug := e.trip(t, alice, "Secret", "private", pastDates)

	for _, tok := range []string{"", bob} {
		status, body, raw := e.do(t, "GET", api+"/journeys/"+slug, tok, "")
		assert.Equal(t, 404, status, raw)
		assert.Equal(t, "journey not found", body["message"])
	}
	status, body, raw := e.do(t, "PATCH", api+"/journeys/"+slug, bob, `{"title":"x"}`)
	assert.Equal(t, 404, status, raw)
	assert.Equal(t, "journey not found", body["message"])

	// Every other route answers the same 404 "journey not found" to a stranger.
	day := e.addDay(t, alice, slug)
	plan := e.addPlan(t, alice, slug, day, "p")
	for _, c := range [][3]string{
		{"PUT", "/update", journeyBody},
		{"DELETE", "", ""},
		{"POST", "/days", `{"date":"1 Jun","date_iso":"2026-06-01T00:00:00Z"}`},
		{"PATCH", "/days/" + day, `{"title":"x"}`},
		{"DELETE", "/days/" + day, ""},
		{"POST", "/days/" + day + "/plans", `{"description":"x"}`},
		{"PUT", "/days/" + day + "/plans/order", `{"plan_ids":["` + plan + `"]}`},
		{"PATCH", "/plans/" + plan, `{"description":"x"}`},
		{"DELETE", "/plans/" + plan, ""},
		{"POST", "/members", `{"username":"alice","role":"traveler"}`},
		{"PATCH", "/members/alice", `{"role":"traveler"}`},
		{"DELETE", "/members/alice", ""},
		{"GET", "/members", ""},
		{"POST", "/invite-links", `{"role":"traveler"}`},
		{"GET", "/invite-links", ""},
		{"DELETE", "/invite-links/abc", ""},
		{"POST", "/leave", ""},
	} {
		status, body, raw := e.do(t, c[0], api+"/journeys/"+slug+c[1], bob, c[2])
		assert.Equal(t, 404, status, "%s %s: %s", c[0], c[1], raw)
		assert.Equal(t, "journey not found", body["message"], "%s %s", c[0], c[1])
	}

	// The owner still sees and edits it.
	status, _, raw = e.do(t, "PATCH", api+"/journeys/"+slug, alice, `{"title":"Renamed"}`)
	assert.Equal(t, 200, status, raw)
}

func TestHTTP_VisibleButNotMemberIs403(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	pub := e.trip(t, alice, "Open", "public", pastDates)
	fol := e.trip(t, alice, "Followers", "followers", pastDates)
	e.follow(t, bob, alice)

	for _, slug := range []string{pub, fol} {
		status, _, raw := e.do(t, "PATCH", api+"/journeys/"+slug, bob, `{"title":"x"}`)
		assert.Equal(t, 403, status, raw)
		status, _, raw = e.do(t, "GET", api+"/journeys/"+slug+"/members", bob, "")
		assert.Equal(t, 403, status, raw)
		status, _, raw = e.do(t, "DELETE", api+"/journeys/"+slug, bob, "")
		assert.Equal(t, 403, status, raw)
	}
}

func TestHTTP_FollowerSeesFullTrip(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	slug := e.trip(t, alice, "Fol", "followers", liveDates)

	status, _, _ := e.do(t, "GET", api+"/journeys/"+slug, bob, "")
	require.Equal(t, 404, status, "not following yet")

	e.follow(t, bob, alice)
	status, body, raw := e.do(t, "GET", api+"/journeys/"+slug, bob, "")
	require.Equal(t, 200, status, raw)
	data := body["data"].(map[string]any)
	assert.Equal(t, false, data["redacted"])
	assert.Equal(t, "follower", e.relationOf(data))
	assert.NotEmpty(t, data["departure_date"])
	assert.NotEmpty(t, data["return_date"])
	assert.Equal(t, "2026-11", data["month_start"])
	assert.NotEmpty(t, data["members"])
	assert.Contains(t, e.listSlugs(t, bob), slug)
}

// relationOf: a view carries role "" for non-members; a full (non-redacted) view
// for a non-member is a follower's.
func (e *env) relationOf(data map[string]any) string {
	if data["role"] == "" && data["redacted"] == false {
		return "follower"
	}
	return "other"
}

func TestHTTP_LinkNotListed(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	slug := e.trip(t, alice, "Linked", "link", futureDates)

	assert.NotContains(t, e.listSlugs(t, ""), slug)
	assert.NotContains(t, e.listSlugs(t, bob), slug)
	assert.Contains(t, e.listSlugs(t, alice), slug, "members still see it in the list")

	status, body, raw := e.do(t, "GET", api+"/journeys/"+slug, "", "")
	require.Equal(t, 200, status, raw)
	data := body["data"].(map[string]any)
	assert.Equal(t, true, data["redacted"], "unfinished trip is redacted for a stranger")
	assert.NotContains(t, raw, "departure_date")
}

func TestHTTP_OptionalAuthBadToken(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	slug := e.trip(t, alice, "Open", "public", pastDates)

	for _, tok := range []string{"garbage", strings.Repeat("a", 64)} {
		status, body, raw := e.do(t, "GET", api+"/journeys/"+slug, tok, "")
		assert.Equal(t, 200, status, raw)
		assert.Equal(t, "", body["data"].(map[string]any)["role"], "bad token is an anonymous viewer")
		status, _, raw = e.do(t, "GET", api+"/journeys", tok, "")
		assert.Equal(t, 200, status, raw)
	}
	// A valid token on the same route identifies the member.
	_, body, _ := e.do(t, "GET", api+"/journeys/"+slug, alice, "")
	assert.Equal(t, "owner", body["data"].(map[string]any)["role"])

	// Protected routes still reject a bad token.
	status, _, _ := e.do(t, "GET", api+"/me/journeys", "garbage", "")
	assert.Equal(t, 401, status)
}

func TestHTTP_ListOrderAndRedactedItemHasNoDates(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	oldest := e.trip(t, alice, "Oldest", "public", futureDates)
	middle := e.trip(t, alice, "Middle", "public", pastDates)
	newest := e.trip(t, alice, "Newest", "public", liveDates)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// created_at order is the reverse of departure order, so ordering by dates would fail.
	for i, slug := range []string{oldest, middle, newest} {
		require.NoError(t, e.db.Model(&models.Journey{}).Where("slug = ?", slug).
			Update("created_at", base.Add(time.Duration(i)*time.Hour)).Error)
	}

	assert.Equal(t, []string{newest, middle, oldest}, e.listSlugs(t, ""))

	_, body, raw := e.do(t, "GET", api+"/journeys", "", "")
	var items []map[string]any
	for _, g := range body["data"].([]any) {
		for _, it := range g.(map[string]any)["plan"].([]any) {
			items = append(items, it.(map[string]any))
		}
	}
	for _, it := range items {
		switch it["slug"] {
		case oldest, newest: // not completed: redacted
			assert.ElementsMatch(t, []string{"id", "slug", "title", "destination", "cover_url", "status"}, keysOf(it), raw)
			assert.NotRegexp(t, `\b20\d\d\b`, it["title"], "redacted title carries no year")
			assert.Equal(t, map[string]any{"state": "upcoming", "days_until": 0.0, "day_number": 0.0, "total_days": 0.0}, it["status"])
		case middle: // completed: full view
			assert.Equal(t, "completed", it["status"].(map[string]any)["state"])
		}
	}
	assert.NotContains(t, raw, "ongoing")
}

func keysOf(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestHTTP_MyJourneysHasStatus(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	e.trip(t, alice, "Live", "private", liveDates)

	status, body, raw := e.do(t, "GET", api+"/me/journeys", alice, "")
	require.Equal(t, 200, status, raw)
	item := body["data"].([]any)[0].(map[string]any)
	st := item["status"].(map[string]any)
	assert.Equal(t, "ongoing", st["state"], "members see the real status")
}
