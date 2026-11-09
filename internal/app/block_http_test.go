package app_test

import (
	"context"
	"testing"

	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// block makes blocker block blocked directly in the DB.
func (e *env) block(t *testing.T, blocker, blocked string) {
	t.Helper()
	require.NoError(t, repository.NewBlockRepository(e.db).Block(context.Background(), e.userID(t, blocker), e.userID(t, blocked)))
}

func TestBlock_TripInvisibleEverywhere(t *testing.T) {
	for _, dir := range []string{"owner blocks viewer", "viewer blocks owner"} {
		t.Run(dir, func(t *testing.T) {
			e := newEnv(t)
			alice := e.named(t, "alice@gmail.com", "alice") // owner
			bob := e.named(t, "bob@gmail.com", "bob")       // block-related to alice
			carol := e.named(t, "carol@gmail.com", "carol") // member, bystander
			slug := e.trip(t, alice, "Past", "public", pastDates)
			e.inviteAndAccept(t, alice, carol, slug, "carol", "traveler")
			if dir == "owner blocks viewer" {
				e.block(t, alice, bob)
			} else {
				e.block(t, bob, alice)
			}

			status, body, raw := e.do(t, "GET", api+"/journeys/"+slug, bob, "")
			assert.Equal(t, 404, status, raw)
			assert.Equal(t, "journey not found", body["message"])
			assert.NotContains(t, e.listSlugs(t, bob), slug)
			assert.Contains(t, e.listSlugs(t, carol), slug)

			// carol's profile: the trip is hidden from bob in every bucket, visible to others.
			p, praw := e.profile(t, bob, "carol")
			assert.NotContains(t, praw, slug)
			assert.Empty(t, p["past"])
			assert.Empty(t, p["calendar"])
			assert.Empty(t, p["upcoming"])
			_, praw = e.profile(t, alice, "carol")
			assert.Contains(t, praw, slug)

			status, _, raw = e.do(t, "PATCH", api+"/journeys/"+slug, bob, `{"title":"x"}`)
			assert.Equal(t, 404, status, raw)
			status, _, raw = e.do(t, "GET", api+"/journeys/"+slug+"/moments", bob, "")
			assert.Equal(t, 404, status, raw)
		})
	}
}

func TestBlock_LiveTripAbsentFromProfileNow(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	carol := e.named(t, "carol@gmail.com", "carol")
	slug := e.trip(t, alice, "Live", "public", liveDates)
	e.inviteAndAccept(t, alice, carol, slug, "carol", "traveler")
	e.follow(t, bob, alice) // followers see a public live trip as "now"
	p, _ := e.profile(t, bob, "carol")
	require.NotNil(t, p["now"])
	e.block(t, alice, bob)
	p, raw := e.profile(t, bob, "carol")
	assert.Nil(t, p["now"], raw)
	assert.NotContains(t, raw, slug)
}

func TestBlock_MemberKeepsAccess(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	slug := e.trip(t, alice, "Trip", "followers", futureDates)
	e.inviteAndAccept(t, alice, bob, slug, "bob", "traveler")
	e.block(t, alice, bob)

	status, _, raw := e.do(t, "GET", api+"/journeys/"+slug, bob, "")
	assert.Equal(t, 200, status, raw)
	assert.Contains(t, e.listSlugs(t, bob), slug)
	// a member who lacks the right still gets 403, not 404
	status, _, raw = e.do(t, "PATCH", api+"/journeys/"+slug, bob, `{"title":"x"}`)
	assert.Equal(t, 403, status, raw)
}

func TestBlock_InviteAndJoinLink(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	carol := e.named(t, "carol@gmail.com", "carol")
	dave := e.named(t, "dave@gmail.com", "dave")
	slug := e.trip(t, alice, "Trip", "public", futureDates)
	e.inviteAndAccept(t, alice, carol, slug, "carol", "editor")

	status, body, raw := e.do(t, "POST", api+"/journeys/"+slug+"/invite-links", alice, `{"role":"traveler"}`)
	require.Equal(t, 201, status, raw)
	url := body["data"].(map[string]any)["url"].(string)
	token := url[len("http://web.test/join/"):]

	// owner blocks bob: invite -> user not found; join link -> invite not found
	e.block(t, alice, bob)
	status, body, raw = e.do(t, "POST", api+"/journeys/"+slug+"/members", alice, `{"username":"bob","role":"traveler"}`)
	assert.Equal(t, 404, status, raw)
	assert.Equal(t, "user not found", body["message"])
	status, body, raw = e.do(t, "POST", api+"/invites/join", bob, `{"token":"`+token+`"}`)
	assert.Equal(t, 404, status, raw)
	assert.Equal(t, "invite not found", body["message"])

	// reverse: bob blocks alice
	e2 := newEnv(t)
	alice2 := e2.named(t, "alice@gmail.com", "alice")
	e2.named(t, "bob@gmail.com", "bob")
	bob2 := e2.login(t, "bob@gmail.com")
	slug2 := e2.trip(t, alice2, "Trip", "public", futureDates)
	e2.block(t, bob2, alice2)
	status, body, raw = e2.do(t, "POST", api+"/journeys/"+slug2+"/members", alice2, `{"username":"bob","role":"traveler"}`)
	assert.Equal(t, 404, status, raw)
	assert.Equal(t, "user not found", body["message"])

	// editor (who may invite) invites someone block-related to the editor
	status, _, raw = e.do(t, "PATCH", api+"/journeys/"+slug, alice, `{"editors_can_invite":true}`)
	require.Equal(t, 200, status, raw)
	e.block(t, carol, dave)
	status, _, raw = e.do(t, "POST", api+"/journeys/"+slug+"/members", carol, `{"username":"dave","role":"traveler"}`)
	assert.Equal(t, 404, status, raw)
}

func TestBlock_HidesMomentsOfBlockedUploader(t *testing.T) {
	e := newMomentHTTPEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	carol := e.named(t, "carol@gmail.com", "carol")
	slug := e.createJourney(t, alice)
	day := e.addDay(t, alice, slug)
	e.inviteAndAccept(t, alice, bob, slug, "bob", "traveler")
	e.inviteAndAccept(t, alice, carol, slug, "carol", "traveler")

	upload := func(tok string) string {
		status, mid, up, raw := e.startUpload(t, tok, slug, day, 1000)
		require.Equal(t, 201, status, raw)
		st, _, _ := e.rawReq(t, "PUT", up["url"].(string), headersOf(up), jpegWithExif(t))
		require.Equal(t, 200, st)
		status, _, raw = e.do(t, "POST", api+"/journeys/"+slug+"/moments/"+mid+"/complete", tok, "")
		require.Equal(t, 200, status, raw)
		return mid
	}
	count := func(tok string) int {
		status, resp, raw := e.do(t, "GET", api+"/journeys/"+slug+"/moments", tok, "")
		require.Equal(t, 200, status, raw)
		return len(resp["data"].(map[string]any)["moments"].([]any))
	}
	upload(bob)
	upload(carol)
	assert.Equal(t, 2, count(carol))

	e.block(t, carol, bob)
	assert.Equal(t, 1, count(carol), "carol no longer sees bob's moment")
	assert.Equal(t, 2, count(alice), "alice unaffected")
	assert.Equal(t, 1, count(bob), "bob does not see carol's moment either (either direction)")
}
