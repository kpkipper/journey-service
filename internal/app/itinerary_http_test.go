package app_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (e *env) detail(t *testing.T, slug string) map[string]any {
	t.Helper()
	status, body, raw := e.do(t, "GET", api+"/journeys/"+slug, "", "")
	require.Equal(t, 200, status, raw)
	return body["data"].(map[string]any)
}

func (e *env) addPlan(t *testing.T, tok, slug, dayID, desc string) string {
	t.Helper()
	status, body, raw := e.do(t, "POST", api+"/journeys/"+slug+"/days/"+dayID+"/plans", tok, `{"description":"`+desc+`","time":"09:00"}`)
	require.Equal(t, 201, status, raw)
	return body["data"].(map[string]any)["id"].(string)
}

func planDescs(day any) []string {
	var out []string
	plans, _ := day.(map[string]any)["plans"].([]any)
	for _, p := range plans {
		out = append(out, p.(map[string]any)["description"].(string))
	}
	return out
}

func TestHTTP_PlanEditing(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	slug := e.createJourney(t, alice)
	day1 := e.addDay(t, alice, slug)
	status, body, raw := e.do(t, "POST", api+"/journeys/"+slug+"/days", alice,
		`{"date":"2 Jun","date_iso":"2026-06-02T00:00:00Z","title":"Day 2"}`)
	require.Equal(t, 201, status, raw)
	day2 := body["data"].(map[string]any)["id"].(string)

	a := e.addPlan(t, alice, slug, day1, "A")
	b := e.addPlan(t, alice, slug, day1, "B")
	c := e.addPlan(t, alice, slug, day1, "C")
	assert.Equal(t, []string{"A", "B", "C"}, planDescs(e.detail(t, slug)["itinerary_days"].([]any)[0]))

	// reorder
	status, _, raw = e.do(t, "PUT", api+"/journeys/"+slug+"/days/"+day1+"/plans/order", alice,
		`{"plan_ids":["`+c+`","`+a+`","`+b+`"]}`)
	require.Equal(t, 200, status, raw)
	assert.Equal(t, []string{"C", "A", "B"}, planDescs(e.detail(t, slug)["itinerary_days"].([]any)[0]))

	// bad reorder
	status, _, _ = e.do(t, "PUT", api+"/journeys/"+slug+"/days/"+day1+"/plans/order", alice, `{"plan_ids":["`+a+`"]}`)
	assert.Equal(t, 400, status)

	// edit + move to day 2
	status, body, raw = e.do(t, "PATCH", api+"/journeys/"+slug+"/plans/"+a, alice, `{"description":"A2","day_id":"`+day2+`"}`)
	require.Equal(t, 200, status, raw)
	assert.Equal(t, "A2", body["data"].(map[string]any)["description"])
	days := e.detail(t, slug)["itinerary_days"].([]any)
	assert.Equal(t, []string{"C", "B"}, planDescs(days[0]))
	assert.Equal(t, []string{"A2"}, planDescs(days[1]))

	// delete plan, update and delete day
	status, _, _ = e.do(t, "DELETE", api+"/journeys/"+slug+"/plans/"+b, alice, "")
	assert.Equal(t, 200, status)
	status, _, raw = e.do(t, "PATCH", api+"/journeys/"+slug+"/days/"+day1, alice, `{"title":"Renamed"}`)
	require.Equal(t, 200, status, raw)
	assert.Equal(t, "Renamed", e.detail(t, slug)["itinerary_days"].([]any)[0].(map[string]any)["title"])
	status, _, _ = e.do(t, "DELETE", api+"/journeys/"+slug+"/days/"+day2, alice, "")
	assert.Equal(t, 200, status)
	days = e.detail(t, slug)["itinerary_days"].([]any)
	require.Len(t, days, 1)
	assert.Equal(t, []string{"C"}, planDescs(days[0]))

	status, _, _ = e.do(t, "DELETE", api+"/journeys/"+slug+"/plans/nope", alice, "")
	assert.Equal(t, 404, status)
	status, _, _ = e.do(t, "DELETE", api+"/journeys/"+slug+"/days/nope", alice, "")
	assert.Equal(t, 404, status)
}

func TestHTTP_CrossTripPlan(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	slugA := e.createJourney(t, alice)
	slugB := e.createJourney(t, alice)
	dayB := e.addDay(t, alice, slugB)
	planB := e.addPlan(t, alice, slugB, dayB, "B plan")

	status, _, _ := e.do(t, "PATCH", api+"/journeys/"+slugA+"/plans/"+planB, alice, `{"description":"hijack"}`)
	assert.Equal(t, 404, status)
	status, _, _ = e.do(t, "DELETE", api+"/journeys/"+slugA+"/plans/"+planB, alice, "")
	assert.Equal(t, 404, status)
	status, _, _ = e.do(t, "PATCH", api+"/journeys/"+slugA+"/days/"+dayB, alice, `{"title":"x"}`)
	assert.Equal(t, 404, status)
	assert.Equal(t, []string{"B plan"}, planDescs(e.detail(t, slugB)["itinerary_days"].([]any)[0]))
}

func TestHTTP_PatchJourney(t *testing.T) {
	e := newEnv(t)
	alice := e.named(t, "alice@gmail.com", "alice")
	bob := e.named(t, "bob@gmail.com", "bob")
	slug := e.createJourney(t, alice)
	e.inviteAndAccept(t, alice, bob, slug, "bob", "editor")

	status, body, raw := e.do(t, "PATCH", api+"/journeys/"+slug, bob, `{"title":"New title"}`)
	require.Equal(t, 200, status, raw)
	data := body["data"].(map[string]any)
	assert.Equal(t, "New title", data["title"])
	assert.Equal(t, "Tokyo", data["destination"], "absent fields are untouched")

	status, _, _ = e.do(t, "PATCH", api+"/journeys/"+slug, bob, `{"editors_can_invite":true}`)
	assert.Equal(t, 403, status, "only the owner changes editors_can_invite")
	status, body, raw = e.do(t, "PATCH", api+"/journeys/"+slug, alice, `{"editors_can_invite":true,"cover_url":"https://img.test/a.jpg"}`)
	require.Equal(t, 200, status, raw)
	assert.Equal(t, true, body["data"].(map[string]any)["editors_can_invite"])
	status, _, _ = e.do(t, "PATCH", api+"/journeys/"+slug, bob, `{"members_can_post":false}`)
	assert.Equal(t, 403, status, "only the owner changes members_can_post")
	assert.True(t, e.dbJourney(t, slug).MembersCanPost, "new trip starts true")
	status, _, raw = e.do(t, "PATCH", api+"/journeys/"+slug, alice, `{"members_can_post":false}`)
	require.Equal(t, 200, status, raw)
	assert.False(t, e.dbJourney(t, slug).MembersCanPost, "false persists")
	status, body, raw = e.do(t, "GET", api+"/journeys/"+slug, bob, ``)
	require.Equal(t, 200, status, raw)
	settings := body["data"].(map[string]any)["settings"].(map[string]any)
	assert.Equal(t, false, settings["members_can_post"])
	status, _, _ = e.do(t, "PATCH", api+"/journeys/"+slug, alice, `{"cover_url":"http://insecure.test/a.jpg"}`)
	assert.Equal(t, 400, status)
	status, _, _ = e.do(t, "PATCH", api+"/journeys/"+slug, alice, `{bad`)
	assert.Equal(t, 400, status)
}
