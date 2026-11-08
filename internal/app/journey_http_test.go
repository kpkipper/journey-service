package app_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const journeyBody = `{"visibility":"public","title":"Japan","destination":"Tokyo","country":"Japan","departure_date":"2026-06-01T00:00:00Z","return_date":"2026-06-07T00:00:00Z"}`

func (e *env) createJourney(t *testing.T, token string) string {
	t.Helper()
	status, body, raw := e.do(t, "POST", "/api/v1/journeys/create", token, journeyBody)
	require.Equal(t, 201, status, raw)
	return body["data"].(map[string]any)["slug"].(string)
}

func (e *env) ownerOf(t *testing.T, slug string) string {
	t.Helper()
	var j models.Journey
	require.NoError(t, e.db.First(&j, "slug = ?", slug).Error)
	return j.OwnerID
}

func (e *env) dbJourney(t *testing.T, slug string) models.Journey {
	t.Helper()
	var j models.Journey
	require.NoError(t, e.db.First(&j, "slug = ?", slug).Error)
	return j
}

func (e *env) userID(t *testing.T, token string) string {
	t.Helper()
	_, me, _ := e.do(t, "GET", "/api/v1/me", token, "")
	return me["data"].(map[string]any)["id"].(string)
}

func TestHTTP_CreateJourneyRequiresLogin(t *testing.T) {
	e := newEnv(t)
	status, _, _ := e.do(t, "POST", "/api/v1/journeys/create", "", journeyBody)
	assert.Equal(t, 401, status)

	tok := e.login(t, "a@gmail.com")
	slug := e.createJourney(t, tok)
	assert.Equal(t, e.userID(t, tok), e.ownerOf(t, slug))
}

func TestHTTP_CreateIgnoresClientOwnerID(t *testing.T) {
	e := newEnv(t)
	tok := e.login(t, "a@gmail.com")
	status, body, raw := e.do(t, "POST", "/api/v1/journeys/create", tok,
		`{"owner_id":"someoneelse","visibility":"public","title":"T","destination":"Tokyo","departure_date":"2026-06-01T00:00:00Z","return_date":"2026-06-07T00:00:00Z"}`)
	require.Equal(t, 201, status, raw)
	assert.Equal(t, e.userID(t, tok), e.ownerOf(t, body["data"].(map[string]any)["slug"].(string)))
}

func TestHTTP_OnlyOwnerCanUpdate(t *testing.T) {
	e := newEnv(t)
	a := e.login(t, "a@gmail.com")
	b := e.login(t, "b@gmail.com")
	slug := e.createJourney(t, a)
	path := "/api/v1/journeys/" + slug + "/update"

	status, _, _ := e.do(t, "PUT", path, "", journeyBody)
	assert.Equal(t, 401, status)
	status, _, _ = e.do(t, "PUT", path, b, journeyBody)
	assert.Equal(t, 403, status)
	status, _, raw := e.do(t, "PUT", path, a, journeyBody)
	assert.Equal(t, 200, status, raw)
	status, _, _ = e.do(t, "PUT", "/api/v1/journeys/nope/update", a, journeyBody)
	assert.Equal(t, 404, status)
}

func TestHTTP_UpdateKeepsOwner(t *testing.T) {
	e := newEnv(t)
	a := e.login(t, "a@gmail.com")
	slug := e.createJourney(t, a)
	owner := e.ownerOf(t, slug)
	path := "/api/v1/journeys/" + slug + "/update"
	for i := 0; i < 2; i++ {
		status, _, raw := e.do(t, "PUT", path, a, `{"owner_id":"evil","title":"Again","destination":"Tokyo"}`)
		require.Equal(t, 200, status, raw)
		assert.Equal(t, owner, e.ownerOf(t, slug))
	}
}

func TestHTTP_OnlyOwnerCanDelete(t *testing.T) {
	e := newEnv(t)
	a := e.login(t, "a@gmail.com")
	b := e.login(t, "b@gmail.com")
	slug := e.createJourney(t, a)

	status, _, _ := e.do(t, "DELETE", "/api/v1/journeys/"+slug, "", "")
	assert.Equal(t, 401, status)
	status, _, _ = e.do(t, "DELETE", "/api/v1/journeys/"+slug, b, "")
	assert.Equal(t, 403, status)
	status, _, _ = e.do(t, "DELETE", "/api/v1/journeys/"+slug, a, "")
	assert.Equal(t, 200, status)
	status, _, _ = e.do(t, "DELETE", "/api/v1/journeys/"+slug, a, "")
	assert.Equal(t, 404, status)
	status, _, _ = e.do(t, "GET", "/api/v1/journeys/"+slug, "", "")
	assert.Equal(t, 404, status)
}

func TestHTTP_LegacyJourneyReadOnly(t *testing.T) {
	e := newEnv(t)
	a := e.login(t, "a@gmail.com")
	j := &models.Journey{
		Title: "Old", Destination: "Kyoto",
		DepartureDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ReturnDate:    time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, e.db.Create(j).Error)

	status, _, _ := e.do(t, "GET", "/api/v1/journeys/"+j.Slug, "", "")
	assert.Equal(t, 200, status)
	status, _, _ = e.do(t, "PUT", "/api/v1/journeys/"+j.Slug+"/update", a, journeyBody)
	assert.Equal(t, 403, status)
	status, _, _ = e.do(t, "DELETE", "/api/v1/journeys/"+j.Slug, a, "")
	assert.Equal(t, 403, status)
}

func TestHTTP_DeleteAllRemoved(t *testing.T) {
	e := newEnv(t)
	a := e.login(t, "a@gmail.com")
	slug := e.createJourney(t, a)
	for _, tok := range []string{"", a} {
		status, _, _ := e.do(t, "DELETE", "/api/v1/journeys", tok, "")
		assert.Contains(t, []int{404, 405}, status)
	}
	status, _, _ := e.do(t, "GET", "/api/v1/journeys/"+slug, "", "")
	assert.Equal(t, 200, status)
}

const tripWithDays = `{"visibility":"public","title":"%s","destination":"Tokyo","departure_date":"2026-06-01T00:00:00Z","return_date":"2026-06-07T00:00:00Z","cover_url":"https://x.test/c.jpg","editors_can_invite":true,` +
	`"itinerary_days":[{"date":"1 Jun","date_iso":"2026-06-01T00:00:00Z","title":"D1","plans":[` +
	`{"time":"09:00","description":"p0"},{"time":"08:00","description":"p1"},{"time":"07:00","description":"p2"}]}]}`

func (e *env) createTrip(t *testing.T, token, title string) (string, map[string]any) {
	t.Helper()
	status, body, raw := e.do(t, "POST", "/api/v1/journeys/create", token, fmt.Sprintf(tripWithDays, title))
	require.Equal(t, 201, status, raw)
	slug := body["data"].(map[string]any)["slug"].(string)
	return slug, e.getTrip(t, slug)
}

func (e *env) getTrip(t *testing.T, slug string) map[string]any {
	t.Helper()
	status, body, raw := e.do(t, "GET", "/api/v1/journeys/"+slug, "", "")
	require.Equal(t, 200, status, raw)
	return body["data"].(map[string]any)
}

func TestHTTP_GetJourneyExposesIDs(t *testing.T) {
	e := newEnv(t)
	a := e.login(t, "a@gmail.com")
	slug, trip := e.createTrip(t, a, "A")
	assert.True(t, e.dbJourney(t, slug).EditorsCanInvite, "editors_can_invite is PATCH-only state, not in the public view")
	assert.Equal(t, "https://x.test/c.jpg", trip["cover_url"])
	days := trip["itinerary_days"].([]any)
	require.Len(t, days, 1)
	for _, d := range days {
		day := d.(map[string]any)
		require.Len(t, day["id"], 10)
		for _, p := range day["plans"].([]any) {
			plan := p.(map[string]any)
			require.Len(t, plan["id"], 10)
			assert.Equal(t, day["id"], plan["day_id"])
		}
	}
}

func TestHTTP_CreateSetsPositions(t *testing.T) {
	e := newEnv(t)
	a := e.login(t, "a@gmail.com")
	_, trip := e.createTrip(t, a, "A")
	plans := trip["itinerary_days"].([]any)[0].(map[string]any)["plans"].([]any)
	require.Len(t, plans, 3)
	for i, p := range plans {
		plan := p.(map[string]any)
		assert.EqualValues(t, i, plan["position"])
		assert.Equal(t, fmt.Sprintf("p%d", i), plan["description"])
	}
}

func TestHTTP_UpdateIgnoresClientChildIDs(t *testing.T) {
	e := newEnv(t)
	a := e.login(t, "alice@gmail.com")
	b := e.login(t, "bob@gmail.com")
	slugA, _ := e.createTrip(t, a, "A")
	slugB, tripB := e.createTrip(t, b, "B")

	dayB := tripB["itinerary_days"].([]any)[0].(map[string]any)
	planB := dayB["plans"].([]any)[0].(map[string]any)
	body := fmt.Sprintf(`{"title":"A2","destination":"Tokyo","departure_date":"2026-06-01T00:00:00Z","return_date":"2026-06-07T00:00:00Z",`+
		`"itinerary_days":[{"id":%q,"journey_id":%q,"date":"HACK","date_iso":"2026-06-01T00:00:00Z","title":"HACK","plans":[`+
		`{"id":%q,"day_id":%q,"time":"01:00","description":"HACK"}]}]}`,
		dayB["id"], e.dbJourney(t, slugB).ID, planB["id"], dayB["id"])
	status, _, raw := e.do(t, "PUT", "/api/v1/journeys/"+slugA+"/update", a, body)
	require.Equal(t, 200, status, raw)

	assert.Equal(t, tripB, e.getTrip(t, slugB))
	newDay := e.getTrip(t, slugA)["itinerary_days"].([]any)[0].(map[string]any)
	assert.NotEqual(t, dayB["id"], newDay["id"])
	assert.NotEqual(t, planB["id"], newDay["plans"].([]any)[0].(map[string]any)["id"])
}

func TestHTTP_UpdateSetsPositions(t *testing.T) {
	e := newEnv(t)
	a := e.login(t, "a@gmail.com")
	slug := e.createJourney(t, a)
	body := fmt.Sprintf(tripWithDays, "Upd")
	status, _, raw := e.do(t, "PUT", "/api/v1/journeys/"+slug+"/update", a, body)
	require.Equal(t, 200, status, raw)

	plans := e.getTrip(t, slug)["itinerary_days"].([]any)[0].(map[string]any)["plans"].([]any)
	require.Len(t, plans, 3)
	for i, p := range plans {
		plan := p.(map[string]any)
		assert.EqualValues(t, i, plan["position"])
		assert.Equal(t, fmt.Sprintf("p%d", i), plan["description"])
	}
}

func TestHTTP_CreateIgnoresClientChildIDs(t *testing.T) {
	e := newEnv(t)
	a := e.login(t, "alice@gmail.com")
	b := e.login(t, "bob@gmail.com")
	slugB, tripB := e.createTrip(t, b, "B")

	dayB := tripB["itinerary_days"].([]any)[0].(map[string]any)
	planB := dayB["plans"].([]any)[0].(map[string]any)
	body := fmt.Sprintf(`{"visibility":"public","title":"A","destination":"Tokyo","departure_date":"2026-06-01T00:00:00Z","return_date":"2026-06-07T00:00:00Z",`+
		`"itinerary_days":[{"id":%q,"journey_id":%q,"date":"HACK","date_iso":"2026-06-01T00:00:00Z","title":"HACK","plans":[`+
		`{"id":%q,"day_id":%q,"time":"01:00","description":"HACK"}]}]}`,
		dayB["id"], e.dbJourney(t, slugB).ID, planB["id"], dayB["id"])
	status, resp, raw := e.do(t, "POST", "/api/v1/journeys/create", a, body)
	require.Equal(t, 201, status, raw)
	slugA := resp["data"].(map[string]any)["slug"].(string)

	assert.Equal(t, tripB, e.getTrip(t, slugB))
	newDay := e.getTrip(t, slugA)["itinerary_days"].([]any)[0].(map[string]any)
	assert.NotEqual(t, dayB["id"], newDay["id"])
	assert.NotEqual(t, planB["id"], newDay["plans"].([]any)[0].(map[string]any)["id"])
}
