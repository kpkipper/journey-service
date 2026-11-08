package app_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTP_FullUpdateIsOwnerOnlyAndKeepsPatchOnlyFields(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	slug := e.createJourney(t, alice)
	e.inviteAndAccept(t, alice, bob, slug, "bob", "editor")
	path := api + "/journeys/" + slug + "/update"

	status, _, _ := e.do(t, "PUT", path, bob, journeyBody)
	assert.Equal(t, 403, status, "editors cannot full-PUT")

	status, _, raw := e.do(t, "PATCH", api+"/journeys/"+slug, alice, `{"cover_url":"https://x.test/c.jpg"}`)
	require.Equal(t, 200, status, raw)
	status, _, raw = e.do(t, "PUT", path, alice, `{"title":"New","destination":"Tokyo","departure_date":"2026-06-01T00:00:00Z","return_date":"2026-06-07T00:00:00Z","editors_can_invite":true,"id":"evil","deleted_at":"2020-01-01T00:00:00Z"}`)
	require.Equal(t, 200, status, raw)
	data := e.getTrip(t, slug)
	assert.Equal(t, "New", data["title"])
	assert.Equal(t, "https://x.test/c.jpg", data["cover_url"])
	assert.False(t, e.dbJourney(t, slug).EditorsCanInvite)

	status, _, _ = e.do(t, "DELETE", api+"/journeys/"+slug, alice, "")
	require.Equal(t, 200, status)
	status, _, _ = e.do(t, "PUT", path, alice, journeyBody)
	assert.Equal(t, 404, status, "PUT on a deleted trip")
	status, _, _ = e.do(t, "GET", api+"/journeys/"+slug, "", "")
	assert.Equal(t, 404, status, "trip stays deleted")
}
