package services

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type memberEnv struct {
	t   *testing.T
	db  *gorm.DB
	svc *MemberService
	now time.Time
}

func newMemberEnv(t *testing.T) *memberEnv {
	db := testutil.NewTestDB(t)
	e := &memberEnv{t: t, db: db, now: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}
	journeys := repository.NewJourneyRepository(db)
	members := repository.NewMemberRepository(db)
	e.svc = NewMemberService(MemberDeps{
		Access:   NewAccessService(journeys, members),
		Journeys: journeys,
		Members:  members,
		Users:    repository.NewUserRepository(db),
		WebURL:   "http://web.test/",
		Now:      func() time.Time { return e.now },
	})
	return e
}

func (e *memberEnv) user(name string) {
	require.NoError(e.t, e.db.Create(&models.User{ID: name, Email: name + "@x.com", Username: name, DisplayName: name}).Error)
}

func (e *memberEnv) journey(ownerID string, editorsCanInvite bool) *models.Journey {
	j := &models.Journey{
		OwnerID: ownerID, Title: "Trip", Destination: "Nagoya",
		DepartureDate: e.now, ReturnDate: e.now.Add(24 * time.Hour), EditorsCanInvite: editorsCanInvite,
	}
	require.NoError(e.t, e.db.Create(j).Error)
	return j
}

func (e *memberEnv) member(j *models.Journey, userID, role, status string) {
	require.NoError(e.t, e.db.Create(&models.JourneyMember{JourneyID: j.ID, UserID: userID, Role: role, Status: status}).Error)
}

func usernames(vs []models.MemberView) []string {
	out := []string{}
	for _, v := range vs {
		out = append(out, v.Username+":"+v.Role+":"+v.Status)
	}
	return out
}

func TestInvite_ByOwner(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	e.user("alice")
	e.user("bob")
	j := e.journey("alice", false)

	m, err := e.svc.Invite(ctx, "alice", j.Slug, "  Bob ", models.RoleEditor)
	require.NoError(t, err)
	assert.Equal(t, models.StatusInvited, m.Status)
	assert.Equal(t, "alice", m.InvitedBy)

	pub, err := e.svc.PublicMembers(ctx, j.Slug)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice:owner:joined"}, usernames(pub))

	list, err := e.svc.ListMembers(ctx, "alice", j.Slug)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice:owner:joined", "bob:editor:invited"}, usernames(list))

	invites, err := e.svc.MyInvites(ctx, "bob")
	require.NoError(t, err)
	require.Len(t, invites, 1)
	assert.Equal(t, InviteView{ID: m.ID, JourneySlug: j.Slug, JourneyTitle: "Trip", Role: "editor", InvitedBy: "alice"}, invites[0])

	require.NoError(t, e.svc.RespondInvite(ctx, "bob", m.ID, true))
	pub, err = e.svc.PublicMembers(ctx, j.Slug)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice:owner:joined", "bob:editor:joined"}, usernames(pub))

	// Already joined: cannot respond again.
	assert.ErrorIs(t, e.svc.RespondInvite(ctx, "bob", m.ID, true), ErrInviteNotFound)
}

func TestRespondInvite_Guards(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	e.user("alice")
	e.user("bob")
	e.user("eve")
	j := e.journey("alice", false)
	m, err := e.svc.Invite(ctx, "alice", j.Slug, "bob", models.RoleTraveler)
	require.NoError(t, err)

	assert.ErrorIs(t, e.svc.RespondInvite(ctx, "eve", m.ID, true), ErrInviteNotFound)
	assert.ErrorIs(t, e.svc.RespondInvite(ctx, "bob", "nope", true), ErrInviteNotFound)

	// Trip deleted before accepting.
	require.NoError(t, e.db.Delete(&models.Journey{}, "id = ?", j.ID).Error)
	assert.ErrorIs(t, e.svc.RespondInvite(ctx, "bob", m.ID, true), ErrInviteNotFound)
}

func TestInvite_Decline(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	e.user("alice")
	e.user("bob")
	j := e.journey("alice", false)

	m, err := e.svc.Invite(ctx, "alice", j.Slug, "bob", models.RoleTraveler)
	require.NoError(t, err)
	require.NoError(t, e.svc.RespondInvite(ctx, "bob", m.ID, false))

	list, err := e.svc.ListMembers(ctx, "alice", j.Slug)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice:owner:joined"}, usernames(list))

	_, err = e.svc.Invite(ctx, "alice", j.Slug, "bob", models.RoleTraveler)
	require.NoError(t, err)
}

func TestInvite_Permissions(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	for _, u := range []string{"alice", "edtr", "trav", "inv", "target"} {
		e.user(u)
	}
	closed := e.journey("alice", false)
	e.member(closed, "edtr", models.RoleEditor, models.StatusJoined)
	e.member(closed, "trav", models.RoleTraveler, models.StatusJoined)
	e.member(closed, "inv", models.RoleEditor, models.StatusInvited)
	open := e.journey("alice", true)
	e.member(open, "edtr", models.RoleEditor, models.StatusJoined)

	_, err := e.svc.Invite(ctx, "trav", closed.Slug, "target", models.RoleTraveler)
	assert.ErrorIs(t, err, ErrForbidden)
	_, err = e.svc.Invite(ctx, "edtr", closed.Slug, "target", models.RoleTraveler)
	assert.ErrorIs(t, err, ErrForbidden)
	_, err = e.svc.Invite(ctx, "inv", closed.Slug, "target", models.RoleTraveler)
	assert.ErrorIs(t, err, ErrNotFound, "invited-only is not a member and cannot see a followers trip")
	_, err = e.svc.Invite(ctx, "target", closed.Slug, "target", models.RoleTraveler)
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = e.svc.Invite(ctx, "edtr", open.Slug, "target", models.RoleEditor)
	assert.ErrorIs(t, err, ErrForbidden)
	_, err = e.svc.Invite(ctx, "edtr", open.Slug, "target", models.RoleTraveler)
	require.NoError(t, err)

	_, err = e.svc.Invite(ctx, "alice", "no-such-slug", "target", models.RoleTraveler)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestInvite_Duplicates(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	e.user("alice")
	e.user("bob")
	j := e.journey("alice", false)

	_, err := e.svc.Invite(ctx, "alice", j.Slug, "alice", models.RoleTraveler)
	assert.ErrorIs(t, err, ErrAlreadyMember)

	_, err = e.svc.Invite(ctx, "alice", j.Slug, "bob", models.RoleTraveler)
	require.NoError(t, err)
	_, err = e.svc.Invite(ctx, "alice", j.Slug, "bob", models.RoleEditor)
	assert.ErrorIs(t, err, ErrAlreadyMember)

	_, err = e.svc.Invite(ctx, "alice", j.Slug, "ghost", models.RoleTraveler)
	assert.ErrorIs(t, err, ErrUserNotFound)

	_, err = e.svc.Invite(ctx, "alice", j.Slug, "bob", "admin")
	var ve *ValidationError
	assert.ErrorAs(t, err, &ve)
}

func TestInvite_EditorCannotInviteOwner(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	e.user("alice")
	e.user("edtr")
	j := e.journey("alice", true)
	e.member(j, "edtr", models.RoleEditor, models.StatusJoined)
	_, err := e.svc.Invite(ctx, "edtr", j.Slug, "alice", models.RoleTraveler)
	assert.ErrorIs(t, err, ErrAlreadyMember)
}

func TestTripFull(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	e.user("alice")
	e.user("late")
	e.user("linker")
	j := e.journey("alice", false)
	for i := 0; i < MaxTripPeople-1; i++ {
		id := fmt.Sprintf("m%02d", i)
		e.user(id)
		st := models.StatusJoined
		if i%2 == 0 {
			st = models.StatusInvited
		}
		e.member(j, id, models.RoleTraveler, st)
	}
	// owner + 19 = 20 people: full.
	_, err := e.svc.Invite(ctx, "alice", j.Slug, "late", models.RoleTraveler)
	assert.ErrorIs(t, err, ErrTripFull)

	url, _, err := e.svc.CreateInviteLink(ctx, "alice", j.Slug, models.RoleTraveler)
	require.NoError(t, err)
	_, err = e.svc.JoinByLink(ctx, "linker", url[len("http://web.test/join/"):])
	assert.ErrorIs(t, err, ErrTripFull)

	// One seat freed -> 19 people, invite works.
	require.NoError(t, e.svc.Leave(ctx, "m01", j.Slug))
	_, err = e.svc.Invite(ctx, "alice", j.Slug, "late", models.RoleTraveler)
	require.NoError(t, err)
}

func TestLeave(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	for _, u := range []string{"alice", "trav", "out", "inv"} {
		e.user(u)
	}
	j := e.journey("alice", false)
	e.member(j, "trav", models.RoleTraveler, models.StatusJoined)
	e.member(j, "inv", models.RoleTraveler, models.StatusInvited)

	var ve *ValidationError
	assert.ErrorAs(t, e.svc.Leave(ctx, "alice", j.Slug), &ve)
	assert.ErrorIs(t, e.svc.Leave(ctx, "out", j.Slug), ErrMemberNotFound)
	assert.ErrorIs(t, e.svc.Leave(ctx, "inv", j.Slug), ErrMemberNotFound)
	assert.ErrorIs(t, e.svc.Leave(ctx, "trav", "nope"), ErrNotFound)

	require.NoError(t, e.svc.Leave(ctx, "trav", j.Slug))
	pub, err := e.svc.PublicMembers(ctx, j.Slug)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice:owner:joined"}, usernames(pub))
}

func TestRemoveAndChangeRole(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	for _, u := range []string{"alice", "edtr", "trav", "out"} {
		e.user(u)
	}
	j := e.journey("alice", true)
	e.member(j, "edtr", models.RoleEditor, models.StatusJoined)
	e.member(j, "trav", models.RoleTraveler, models.StatusInvited)

	assert.ErrorIs(t, e.svc.Remove(ctx, "edtr", j.Slug, "trav"), ErrForbidden)
	assert.ErrorIs(t, e.svc.ChangeRole(ctx, "edtr", j.Slug, "edtr", models.RoleEditor), ErrForbidden)
	assert.ErrorIs(t, e.svc.ChangeRole(ctx, "edtr", j.Slug, "trav", models.RoleEditor), ErrForbidden)

	var ve *ValidationError
	assert.ErrorAs(t, e.svc.Remove(ctx, "alice", j.Slug, "alice"), &ve)
	assert.Equal(t, "owner cannot be removed", ve.Message)
	assert.ErrorAs(t, e.svc.ChangeRole(ctx, "alice", j.Slug, "alice", models.RoleEditor), &ve)
	assert.ErrorAs(t, e.svc.ChangeRole(ctx, "alice", j.Slug, "trav", "owner"), &ve)
	assert.ErrorIs(t, e.svc.Remove(ctx, "alice", j.Slug, "out"), ErrMemberNotFound)
	assert.ErrorIs(t, e.svc.Remove(ctx, "alice", j.Slug, "ghost"), ErrMemberNotFound)

	// Role change on an invited row.
	require.NoError(t, e.svc.ChangeRole(ctx, "alice", j.Slug, "TRAV", models.RoleEditor))
	list, err := e.svc.ListMembers(ctx, "alice", j.Slug)
	require.NoError(t, err)
	assert.Contains(t, usernames(list), "trav:editor:invited")

	require.NoError(t, e.svc.Remove(ctx, "alice", j.Slug, "edtr"))
	list, err = e.svc.ListMembers(ctx, "alice", j.Slug)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice:owner:joined", "trav:editor:invited"}, usernames(list))
}

func TestListMembers_OnlyMembers(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	for _, u := range []string{"alice", "inv", "out"} {
		e.user(u)
	}
	j := e.journey("alice", false)
	e.member(j, "inv", models.RoleTraveler, models.StatusInvited)
	for _, u := range []string{"inv", "out", ""} {
		_, err := e.svc.ListMembers(ctx, u, j.Slug)
		assert.ErrorIs(t, err, ErrNotFound, u)
	}
}

func TestInviteLink(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	for _, u := range []string{"alice", "bob", "carol", "dave", "edtr"} {
		e.user(u)
	}
	j := e.journey("alice", true)
	e.member(j, "edtr", models.RoleEditor, models.StatusJoined)

	url, link, err := e.svc.CreateInviteLink(ctx, "alice", j.Slug, models.RoleEditor)
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`^http://web\.test/join/[0-9a-f]{64}$`), url)
	assert.Equal(t, e.now.Add(InviteLinkTTL), link.ExpiresAt)
	token := url[len("http://web.test/join/"):]
	assert.NotEqual(t, token, link.TokenHash)

	slug, err := e.svc.JoinByLink(ctx, "bob", token)
	require.NoError(t, err)
	assert.Equal(t, j.Slug, slug)
	pub, _ := e.svc.PublicMembers(ctx, j.Slug)
	assert.Contains(t, usernames(pub), "bob:editor:joined")

	// Idempotent for joined members and owner.
	slug, err = e.svc.JoinByLink(ctx, "bob", token)
	require.NoError(t, err)
	assert.Equal(t, j.Slug, slug)
	slug, err = e.svc.JoinByLink(ctx, "alice", token)
	require.NoError(t, err)
	assert.Equal(t, j.Slug, slug)

	// Pending invite is upgraded to joined.
	e.member(j, "carol", models.RoleTraveler, models.StatusInvited)
	_, err = e.svc.JoinByLink(ctx, "carol", token)
	require.NoError(t, err)
	pub, _ = e.svc.PublicMembers(ctx, j.Slug)
	assert.Contains(t, usernames(pub), "carol:traveler:joined")

	// Unknown token, editor limits, listing.
	_, err = e.svc.JoinByLink(ctx, "dave", "deadbeef")
	assert.ErrorIs(t, err, ErrInviteNotFound)
	_, _, err = e.svc.CreateInviteLink(ctx, "edtr", j.Slug, models.RoleEditor)
	assert.ErrorIs(t, err, ErrForbidden)
	_, _, err = e.svc.CreateInviteLink(ctx, "edtr", j.Slug, models.RoleTraveler)
	require.NoError(t, err)
	_, _, err = e.svc.CreateInviteLink(ctx, "alice", j.Slug, "admin")
	var ve *ValidationError
	assert.ErrorAs(t, err, &ve)
	links, err := e.svc.ListInviteLinks(ctx, "edtr", j.Slug)
	require.NoError(t, err)
	assert.Len(t, links, 2)
	_, err = e.svc.ListInviteLinks(ctx, "dave", j.Slug)
	assert.ErrorIs(t, err, ErrNotFound)

	// Expiry: exactly at ExpiresAt is expired.
	e.now = e.now.Add(InviteLinkTTL)
	_, err = e.svc.JoinByLink(ctx, "dave", token)
	assert.ErrorIs(t, err, ErrInviteNotFound)
}

func TestInviteLink_Revoke(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	for _, u := range []string{"alice", "bob", "edtr"} {
		e.user(u)
	}
	j := e.journey("alice", true)
	other := e.journey("alice", true)
	e.member(j, "edtr", models.RoleEditor, models.StatusJoined)

	url, link, err := e.svc.CreateInviteLink(ctx, "alice", j.Slug, models.RoleTraveler)
	require.NoError(t, err)
	token := url[len("http://web.test/join/"):]

	assert.ErrorIs(t, e.svc.RevokeInviteLink(ctx, "edtr", j.Slug, link.ID), ErrForbidden)
	// Link of another journey is not reachable via this slug.
	assert.ErrorIs(t, e.svc.RevokeInviteLink(ctx, "alice", other.Slug, link.ID), ErrInviteNotFound)
	_, err = e.svc.JoinByLink(ctx, "bob", token)
	require.NoError(t, err) // still valid

	require.NoError(t, e.svc.RevokeInviteLink(ctx, "alice", j.Slug, link.ID))
	e.user("zed")
	_, err = e.svc.JoinByLink(ctx, "zed", token)
	assert.ErrorIs(t, err, ErrInviteNotFound)
}

func TestInviteLink_TripDeleted(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	e.user("alice")
	e.user("bob")
	j := e.journey("alice", false)
	url, _, err := e.svc.CreateInviteLink(ctx, "alice", j.Slug, models.RoleTraveler)
	require.NoError(t, err)
	require.NoError(t, e.db.Delete(&models.Journey{}, "id = ?", j.ID).Error)
	_, err = e.svc.JoinByLink(ctx, "bob", url[len("http://web.test/join/"):])
	assert.ErrorIs(t, err, ErrInviteNotFound)
}

func TestLegacyTrip(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	e.user("alice")
	e.user("bob")
	j := e.journey("", true)

	_, err := e.svc.Invite(ctx, "alice", j.Slug, "bob", models.RoleTraveler)
	assert.ErrorIs(t, err, ErrForbidden)
	_, _, err = e.svc.CreateInviteLink(ctx, "alice", j.Slug, models.RoleTraveler)
	assert.ErrorIs(t, err, ErrForbidden)

	pub, err := e.svc.PublicMembers(ctx, j.Slug)
	require.NoError(t, err)
	assert.Empty(t, pub)
}
