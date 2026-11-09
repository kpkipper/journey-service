package services

import (
	"context"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ---- full PUT: owner only, only its own columns ----

func TestFullUpdateOwnerOnly(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	body := func() *models.Journey {
		return &models.Journey{
			ID: "evil", OwnerID: "edtr", Title: "Edited", Destination: "Nagoya",
			DepartureDate: j.DepartureDate, ReturnDate: j.ReturnDate,
			CoverURL: "https://evil.test/c.jpg", EditorsCanInvite: true,
			DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true},
		}
	}

	assert.ErrorIs(t, e.journeys.Update(e.ctx, "edtr", j.Slug, body()), ErrForbidden)
	assert.ErrorIs(t, e.journeys.Update(e.ctx, "trav", j.Slug, body()), ErrForbidden)
	assert.ErrorIs(t, e.journeys.Update(e.ctx, "out", j.Slug, body()), ErrNotFound)
	assert.Equal(t, "Trip", e.reload(j).Title)

	// Owner: hostile body cannot change id/owner/deleted_at, nor the PATCH-only fields.
	require.NoError(t, e.journeys.Update(e.ctx, "alice", j.Slug, body()))
	got := e.reload(j)
	assert.Equal(t, "Edited", got.Title)
	assert.Equal(t, j.ID, got.ID)
	assert.Equal(t, "alice", got.OwnerID)
	assert.False(t, got.DeletedAt.Valid)
	assert.False(t, got.EditorsCanInvite, "PUT does not touch editors_can_invite")
	assert.Equal(t, "https://x.test/c.jpg", got.CoverURL, "PUT does not touch cover_url")

	assert.ErrorIs(t, e.journeys.Delete(e.ctx, "edtr", j.Slug), ErrForbidden)
	assert.ErrorIs(t, e.journeys.Delete(e.ctx, "out", j.Slug), ErrNotFound)
	require.NoError(t, e.journeys.Delete(e.ctx, "alice", j.Slug))
	assert.ErrorIs(t, e.journeys.Delete(e.ctx, "alice", j.Slug), ErrNotFound)
}

func TestFullUpdateKeepsPatchOnlyFields(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	_, err := e.svc.PatchJourney(e.ctx, "alice", j.Slug, JourneyPatch{CoverURL: sp("https://x.test/patched.jpg"), EditorsCanInvite: bp(true)})
	require.NoError(t, err)

	in := &models.Journey{Title: "T2", Destination: "Nagoya", DepartureDate: j.DepartureDate, ReturnDate: j.ReturnDate}
	require.NoError(t, e.journeys.Update(e.ctx, "alice", j.Slug, in))
	got := e.reload(j)
	assert.Equal(t, "T2", got.Title)
	assert.Equal(t, "https://x.test/patched.jpg", got.CoverURL, "PUT without cover_url keeps the PATCHed cover")
	assert.True(t, got.EditorsCanInvite)

	in = &models.Journey{Title: "T3", Destination: "Nagoya", DepartureDate: j.DepartureDate, ReturnDate: j.ReturnDate, EditorsCanInvite: false, CoverURL: "bad"}
	require.NoError(t, e.journeys.Update(e.ctx, "alice", j.Slug, in))
	got = e.reload(j)
	assert.True(t, got.EditorsCanInvite)
	assert.Equal(t, "https://x.test/patched.jpg", got.CoverURL)
}

func TestFullUpdateOnSoftDeletedTrip(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	require.NoError(t, e.db.Delete(&models.Journey{}, "id = ?", j.ID).Error)
	in := &models.Journey{Title: "Zombie", Destination: "Nagoya", DepartureDate: j.DepartureDate, ReturnDate: j.ReturnDate}
	assert.ErrorIs(t, e.journeys.Update(e.ctx, "alice", j.Slug, in), ErrNotFound)

	// The repository itself also refuses to touch (or resurrect) the deleted row.
	in.ID = j.ID
	assert.ErrorIs(t, e.jrepo.Update(e.ctx, in), gorm.ErrRecordNotFound)
	var raw models.Journey
	require.NoError(t, e.db.Unscoped().First(&raw, "id = ?", j.ID).Error)
	assert.True(t, raw.DeletedAt.Valid, "trip stays deleted")
	assert.Equal(t, "Trip", raw.Title)
}

// ---- map_url normalization and error positions ----

func TestMapURLNormalizationAndPositions(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	var ve *ValidationError
	plans := func(urls ...string) *models.Journey {
		day := func(ps ...models.ActivityPlan) models.ItineraryDay {
			return models.ItineraryDay{Date: "d", DateISO: j.DepartureDate, Plans: ps}
		}
		var ps []models.ActivityPlan
		for _, u := range urls {
			ps = append(ps, models.ActivityPlan{Description: "x", MapURL: u})
		}
		return &models.Journey{Title: "T", Destination: "N", DepartureDate: j.DepartureDate, ReturnDate: j.ReturnDate,
			ItineraryDays: []models.ItineraryDay{day(models.ActivityPlan{Description: "ok"}), day(ps...)}}
	}

	// Scheme-less gets https:// on PUT, create and the plan endpoints.
	require.NoError(t, e.journeys.Update(e.ctx, "alice", j.Slug, plans("maps.app.goo.gl/x")))
	assert.Equal(t, "https://maps.app.goo.gl/x", e.reload(j).ItineraryDays[1].Plans[0].MapURL)
	created := plans("maps.app.goo.gl/y")
	require.NoError(t, e.journeys.Create(e.ctx, "alice", created))
	assert.Equal(t, "https://maps.app.goo.gl/y", created.ItineraryDays[1].Plans[0].MapURL)
	d := e.reload(j).ItineraryDays[1]
	p, err := e.svc.AddPlan(e.ctx, "edtr", j.Slug, d.ID, PlanInput{Description: sp("z"), MapURL: sp("maps.app.goo.gl/z")})
	require.NoError(t, err)
	assert.Equal(t, "https://maps.app.goo.gl/z", p.MapURL)
	p, err = e.svc.UpdatePlan(e.ctx, "edtr", j.Slug, p.ID, PlanInput{MapURL: sp("maps.app.goo.gl/w")})
	require.NoError(t, err)
	assert.Equal(t, "https://maps.app.goo.gl/w", p.MapURL)

	// Other schemes are still rejected, and the message names the position.
	for _, u := range []string{"javascript:alert(1)", "mailto:a@b.c", "ftp://x.test/a", "data:text/html,x"} {
		err := e.journeys.Update(e.ctx, "alice", j.Slug, plans("ok.test/a", "ok.test/b", u))
		require.ErrorAs(t, err, &ve, u)
		assert.Equal(t, "day 2, plan 3: invalid map_url", ve.Message, u)
		_, err = e.svc.AddPlan(e.ctx, "edtr", j.Slug, d.ID, PlanInput{Description: sp("z"), MapURL: sp(u)})
		assert.ErrorAs(t, err, &ve, u)
	}
	err = e.journeys.Create(e.ctx, "alice", plans("javascript:x"))
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "day 2, plan 1: invalid map_url", ve.Message)
}

// ---- reorder race ----

type vanishingItinRepo struct {
	repository.ItineraryRepository
	db *gorm.DB
}

// SetPositions deletes a plan first, as a concurrent DeletePlan would.
func (r vanishingItinRepo) SetPositions(ctx context.Context, dayID string, ids []string) error {
	if err := r.db.Delete(&models.ActivityPlan{}, "id = ?", ids[0]).Error; err != nil {
		return err
	}
	return r.ItineraryRepository.SetPositions(ctx, dayID, ids)
}

func TestReorderPlans_PlanVanishes(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	d := e.day(j, "D1")
	p1, p2 := e.plan(j, d.ID, "a"), e.plan(j, d.ID, "b")
	svc := NewItineraryService(NewAccessService(e.jrepo, repository.NewMemberRepository(e.db)),
		vanishingItinRepo{repository.NewItineraryRepository(e.db), e.db}, e.jrepo, nil)

	err := svc.ReorderPlans(e.ctx, "alice", j.Slug, d.ID, []string{p2.ID, p1.ID})
	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "plans changed, reload and try again", ve.Message)
}

// ---- membership races ----

// staleReadRepo deletes the row right after a read, simulating a concurrent removal
// between the service's read and its write.
type staleReadRepo struct {
	repository.MemberRepository
	db *gorm.DB
}

func (r staleReadRepo) GetByID(ctx context.Context, id string) (*models.JourneyMember, error) {
	m, err := r.MemberRepository.GetByID(ctx, id)
	if m != nil {
		r.db.Delete(&models.JourneyMember{}, "id = ?", m.ID)
	}
	return m, err
}

func (r staleReadRepo) Get(ctx context.Context, journeyID, userID string) (*models.JourneyMember, error) {
	m, err := r.MemberRepository.Get(ctx, journeyID, userID)
	if m != nil {
		r.db.Delete(&models.JourneyMember{}, "id = ?", m.ID)
	}
	return m, err
}

func (e *memberEnv) staleSvc() *MemberService {
	d := e.svc.MemberDeps
	d.Members = staleReadRepo{d.Members, e.db}
	return NewMemberService(d)
}

func (e *memberEnv) rowCount(j *models.Journey, userID string) int64 {
	var n int64
	require.NoError(e.t, e.db.Model(&models.JourneyMember{}).Where("journey_id = ? AND user_id = ?", j.ID, userID).Count(&n).Error)
	return n
}

func TestStaleWritesDoNotResurrectMembership(t *testing.T) {
	ctx := context.Background()

	t.Run("accept", func(t *testing.T) {
		e := newMemberEnv(t)
		e.user("alice")
		e.user("bob")
		j := e.journey("alice", false)
		m, err := e.svc.Invite(ctx, "alice", j.Slug, "bob", models.RoleTraveler)
		require.NoError(t, err)
		assert.ErrorIs(t, e.staleSvc().RespondInvite(ctx, "bob", m.ID, true), ErrInviteNotFound)
		assert.Zero(t, e.rowCount(j, "bob"))
	})
	t.Run("change role", func(t *testing.T) {
		e := newMemberEnv(t)
		e.user("alice")
		e.user("bob")
		j := e.journey("alice", false)
		e.member(j, "bob", models.RoleTraveler, models.StatusJoined)
		assert.ErrorIs(t, e.staleSvc().ChangeRole(ctx, "alice", j.Slug, "bob", models.RoleEditor), ErrMemberNotFound)
		assert.Zero(t, e.rowCount(j, "bob"))
	})
	t.Run("link upgrade", func(t *testing.T) {
		e := newMemberEnv(t)
		e.user("alice")
		e.user("bob")
		j := e.journey("alice", false)
		url, _, err := e.svc.CreateInviteLink(ctx, "alice", j.Slug, models.RoleTraveler)
		require.NoError(t, err)
		e.member(j, "bob", models.RoleTraveler, models.StatusInvited)
		_, err = e.staleSvc().JoinByLink(ctx, "bob", url[len("http://web.test/join/"):])
		assert.ErrorIs(t, err, ErrInviteNotFound)
		assert.Zero(t, e.rowCount(j, "bob"))
	})
}

func TestMemberCreate_DuplicateMapsToAlreadyMember(t *testing.T) {
	e := newMemberEnv(t)
	e.user("alice")
	e.user("bob")
	j := e.journey("alice", false)
	e.member(j, "bob", models.RoleTraveler, models.StatusInvited)
	dup := &models.JourneyMember{JourneyID: j.ID, UserID: "bob", Role: models.RoleTraveler, Status: models.StatusInvited}
	err := e.svc.Members.Create(context.Background(), dup)
	assert.ErrorIs(t, mapCreateErr(err), ErrAlreadyMember)
	assert.NoError(t, mapCreateErr(nil))
}

// blindGetRepo hides existing rows from Get, so Invite loses the race to the unique index.
type blindGetRepo struct{ repository.MemberRepository }

func (blindGetRepo) Get(context.Context, string, string) (*models.JourneyMember, error) {
	return nil, nil
}

func TestInvite_RaceDuplicateIsAlreadyMember(t *testing.T) {
	e := newMemberEnv(t)
	e.user("alice")
	e.user("bob")
	j := e.journey("alice", false)
	e.member(j, "bob", models.RoleTraveler, models.StatusInvited)
	d := e.svc.MemberDeps
	d.Members = blindGetRepo{d.Members}
	_, err := NewMemberService(d).Invite(context.Background(), "alice", j.Slug, "bob", models.RoleTraveler)
	assert.ErrorIs(t, err, ErrAlreadyMember)
}

// ---- invite links depend on the creator's current rights ----

func TestJoinByLink_CreatorMustStillBeAbleToInvite(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) (*memberEnv, *models.Journey, string) {
		e := newMemberEnv(t)
		for _, u := range []string{"alice", "edtr", "bob"} {
			e.user(u)
		}
		j := e.journey("alice", true)
		e.member(j, "edtr", models.RoleEditor, models.StatusJoined)
		url, _, err := e.svc.CreateInviteLink(ctx, "edtr", j.Slug, models.RoleTraveler)
		require.NoError(t, err)
		return e, j, url[len("http://web.test/join/"):]
	}
	flagOff := func(e *memberEnv, j *models.Journey) {
		require.NoError(t, e.db.Model(&models.Journey{}).Where("id = ?", j.ID).Update("editors_can_invite", false).Error)
	}

	t.Run("works while editor may invite", func(t *testing.T) {
		e, j, token := setup(t)
		slug, err := e.svc.JoinByLink(ctx, "bob", token)
		require.NoError(t, err)
		assert.Equal(t, j.Slug, slug)
	})
	t.Run("owner turns off editors_can_invite", func(t *testing.T) {
		e, j, token := setup(t)
		flagOff(e, j)
		_, err := e.svc.JoinByLink(ctx, "bob", token)
		assert.ErrorIs(t, err, ErrInviteNotFound)
		assert.Zero(t, e.rowCount(j, "bob"))
	})
	t.Run("editor removed", func(t *testing.T) {
		e, j, token := setup(t)
		require.NoError(t, e.svc.Remove(ctx, "alice", j.Slug, "edtr"))
		_, err := e.svc.JoinByLink(ctx, "bob", token)
		assert.ErrorIs(t, err, ErrInviteNotFound)
	})
	t.Run("editor demoted to traveler", func(t *testing.T) {
		e, j, token := setup(t)
		require.NoError(t, e.svc.ChangeRole(ctx, "alice", j.Slug, "edtr", models.RoleTraveler))
		_, err := e.svc.JoinByLink(ctx, "bob", token)
		assert.ErrorIs(t, err, ErrInviteNotFound)
	})
	t.Run("pending invite is not upgraded either", func(t *testing.T) {
		e, j, token := setup(t)
		e.member(j, "bob", models.RoleTraveler, models.StatusInvited)
		flagOff(e, j)
		_, err := e.svc.JoinByLink(ctx, "bob", token)
		assert.ErrorIs(t, err, ErrInviteNotFound)
		got, _ := e.svc.Members.Get(ctx, j.ID, "bob")
		assert.Equal(t, models.StatusInvited, got.Status)
	})
	t.Run("owner-created link keeps working", func(t *testing.T) {
		e, j, _ := setup(t)
		flagOff(e, j)
		url, _, err := e.svc.CreateInviteLink(ctx, "alice", j.Slug, models.RoleTraveler)
		require.NoError(t, err)
		_, err = e.svc.JoinByLink(ctx, "bob", url[len("http://web.test/join/"):])
		require.NoError(t, err)
	})
}
