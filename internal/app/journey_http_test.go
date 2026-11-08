package app_test

import (
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const journeyBody = `{"title":"Japan","destination":"Tokyo","country":"Japan","departure_date":"2026-06-01T00:00:00Z","return_date":"2026-06-07T00:00:00Z"}`

func (e *env) createJourney(t *testing.T, token string) string {
	t.Helper()
	status, body, raw := e.do(t, "POST", "/api/v1/journeys/create", token, journeyBody)
	require.Equal(t, 201, status, raw)
	return body["data"].(map[string]any)["slug"].(string)
}

func (e *env) ownerOf(t *testing.T, slug string) string {
	t.Helper()
	status, body, raw := e.do(t, "GET", "/api/v1/journeys/"+slug, "", "")
	require.Equal(t, 200, status, raw)
	return body["data"].(map[string]any)["owner_id"].(string)
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
		`{"owner_id":"someoneelse","title":"T","destination":"Tokyo","departure_date":"2026-06-01T00:00:00Z","return_date":"2026-06-07T00:00:00Z"}`)
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
