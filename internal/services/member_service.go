package services

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/kpkipper/journey-service/internal/auth"
	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
)

const MaxTripPeople = 20
const InviteLinkTTL = 7 * 24 * time.Hour

var (
	ErrMemberNotFound = errors.New("member not found")
	ErrInviteNotFound = errors.New("invite not found")
	ErrAlreadyMember  = errors.New("already a member")
	ErrTripFull       = errors.New("trip is full")
)

type MemberDeps struct {
	Access   *AccessService
	Journeys repository.JourneyRepository
	Members  repository.MemberRepository
	Users    repository.UserRepository
	WebURL   string
	Now      func() time.Time
}

type MemberService struct {
	MemberDeps
}

func NewMemberService(d MemberDeps) *MemberService {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &MemberService{d}
}

// InviteView is a pending invitation as seen by the invited user.
type InviteView struct {
	ID           string `json:"id"`
	JourneySlug  string `json:"journey_slug"`
	JourneyTitle string `json:"journey_title"`
	Role         string `json:"role"`
	InvitedBy    string `json:"invited_by"`
}

// MyJourney is a trip the user owns or has joined, with the user's role in it.
type MyJourney struct {
	Slug          string    `json:"slug"`
	Title         string    `json:"title"`
	Destination   string    `json:"destination"`
	DepartureDate time.Time `json:"departure_date"`
	ReturnDate    time.Time `json:"return_date"`
	CoverURL      string    `json:"cover_url"`
	Role          string    `json:"role"`
}

func normalizeUsername(u string) string { return strings.ToLower(strings.TrimSpace(u)) }

func validMemberRole(role string) error {
	if role != models.RoleEditor && role != models.RoleTraveler {
		return &ValidationError{Message: "role must be editor or traveler"}
	}
	return nil
}

// checkCapacity returns ErrTripFull when no seat is left (owner + invited + joined).
func (s *MemberService) checkCapacity(ctx context.Context, journeyID string) error {
	n, err := s.Members.CountActive(ctx, journeyID)
	if err != nil {
		return err
	}
	if 1+n >= MaxTripPeople {
		return ErrTripFull
	}
	return nil
}

// authorizeInvite loads access and checks the actor may hand out the given role.
func (s *MemberService) authorizeInvite(ctx context.Context, actorID, slug, role string) (*Access, error) {
	a, err := s.Access.Load(ctx, slug, actorID)
	if err != nil {
		return nil, err
	}
	if !a.CanInvite() {
		return nil, ErrForbidden
	}
	if err := validMemberRole(role); err != nil {
		return nil, err
	}
	if role == models.RoleEditor && !a.IsOwner() {
		return nil, ErrForbidden
	}
	return a, nil
}

func (s *MemberService) Invite(ctx context.Context, actorID, slug, username, role string) (*models.JourneyMember, error) {
	a, err := s.authorizeInvite(ctx, actorID, slug, role)
	if err != nil {
		return nil, err
	}
	target, err := s.Users.GetByUsername(ctx, normalizeUsername(username))
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, ErrUserNotFound
	}
	if target.ID == a.Journey.OwnerID || target.ID == actorID {
		return nil, ErrAlreadyMember
	}
	existing, err := s.Members.Get(ctx, a.Journey.ID, target.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrAlreadyMember
	}
	if err := s.checkCapacity(ctx, a.Journey.ID); err != nil {
		return nil, err
	}
	m := &models.JourneyMember{
		JourneyID: a.Journey.ID, UserID: target.ID, Role: role,
		Status: models.StatusInvited, InvitedBy: actorID,
	}
	if err := s.Members.Create(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *MemberService) ownerView(ctx context.Context, j *models.Journey) ([]models.MemberView, error) {
	if j.OwnerID == "" {
		return nil, nil
	}
	u, err := s.Users.GetByID(ctx, j.OwnerID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}
	return []models.MemberView{{
		Username: u.Username, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL,
		Role: models.RoleOwner, Status: models.StatusJoined,
	}}, nil
}

func (s *MemberService) ListMembers(ctx context.Context, actorID, slug string) ([]models.MemberView, error) {
	a, err := s.Access.Load(ctx, slug, actorID)
	if err != nil {
		return nil, err
	}
	if !a.IsMember() {
		return nil, ErrForbidden
	}
	out, err := s.ownerView(ctx, a.Journey)
	if err != nil {
		return nil, err
	}
	rows, err := s.Members.ListByJourney(ctx, a.Journey.ID)
	if err != nil {
		return nil, err
	}
	return append(out, rows...), nil
}

func (s *MemberService) PublicMembers(ctx context.Context, slug string) ([]models.MemberView, error) {
	a, err := s.Access.Load(ctx, slug, "")
	if err != nil {
		return nil, err
	}
	out, err := s.ownerView(ctx, a.Journey)
	if err != nil {
		return nil, err
	}
	rows, err := s.Members.ListByJourney(ctx, a.Journey.ID)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.Status == models.StatusJoined {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *MemberService) MyInvites(ctx context.Context, userID string) ([]InviteView, error) {
	rows, err := s.Members.ListInvitesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]InviteView, 0, len(rows))
	for _, m := range rows {
		j, err := s.Journeys.GetByID(ctx, m.JourneyID)
		if err != nil {
			return nil, err
		}
		if j == nil {
			continue
		}
		v := InviteView{ID: m.ID, JourneySlug: j.Slug, JourneyTitle: j.Title, Role: m.Role}
		if m.InvitedBy != "" {
			u, err := s.Users.GetByID(ctx, m.InvitedBy)
			if err != nil {
				return nil, err
			}
			if u != nil {
				v.InvitedBy = u.Username
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// MyJourneys lists trips the user owns or has joined (not pending invites), newest departure first.
func (s *MemberService) MyJourneys(ctx context.Context, userID string) ([]MyJourney, error) {
	owned, err := s.Journeys.ListByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]MyJourney, 0, len(owned))
	seen := map[string]bool{}
	add := func(j models.Journey, role string) {
		seen[j.ID] = true
		out = append(out, MyJourney{
			Slug: j.Slug, Title: j.Title, Destination: j.Destination,
			DepartureDate: j.DepartureDate, ReturnDate: j.ReturnDate,
			CoverURL: j.CoverURL, Role: role,
		})
	}
	for _, j := range owned {
		add(j, models.RoleOwner)
	}
	ids, err := s.Members.ListJourneyIDsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	joined, err := s.Journeys.ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, j := range joined {
		if seen[j.ID] {
			continue
		}
		m, err := s.Members.Get(ctx, j.ID, userID)
		if err != nil {
			return nil, err
		}
		if m == nil || m.Status != models.StatusJoined {
			continue
		}
		add(j, m.Role)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].DepartureDate.After(out[b].DepartureDate) })
	return out, nil
}

func (s *MemberService) RespondInvite(ctx context.Context, userID, memberID string, accept bool) error {
	m, err := s.Members.GetByID(ctx, memberID)
	if err != nil {
		return err
	}
	if m == nil || m.UserID != userID || m.Status != models.StatusInvited {
		return ErrInviteNotFound
	}
	if !accept {
		return s.Members.Delete(ctx, m.ID)
	}
	j, err := s.Journeys.GetByID(ctx, m.JourneyID)
	if err != nil {
		return err
	}
	if j == nil {
		return ErrInviteNotFound
	}
	m.Status = models.StatusJoined
	return s.Members.Update(ctx, m)
}

func (s *MemberService) Leave(ctx context.Context, userID, slug string) error {
	a, err := s.Access.Load(ctx, slug, userID)
	if err != nil {
		return err
	}
	if a.IsOwner() {
		return &ValidationError{Message: "owner cannot leave the trip"}
	}
	if !a.IsMember() {
		return ErrMemberNotFound
	}
	m, err := s.Members.Get(ctx, a.Journey.ID, userID)
	if err != nil {
		return err
	}
	if m == nil {
		return ErrMemberNotFound
	}
	return s.Members.Delete(ctx, m.ID)
}

// ownerTarget authorizes an owner-only action and resolves the target member row by username.
// ownerMsg is the ValidationError message used when the target is the owner.
func (s *MemberService) ownerTarget(ctx context.Context, actorID, slug, username, ownerMsg string) (*models.JourneyMember, error) {
	a, err := s.Access.Load(ctx, slug, actorID)
	if err != nil {
		return nil, err
	}
	if !a.IsOwner() {
		return nil, ErrForbidden
	}
	u, err := s.Users.GetByUsername(ctx, normalizeUsername(username))
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrMemberNotFound
	}
	if u.ID == a.Journey.OwnerID {
		return nil, &ValidationError{Message: ownerMsg}
	}
	m, err := s.Members.Get(ctx, a.Journey.ID, u.ID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrMemberNotFound
	}
	return m, nil
}

func (s *MemberService) Remove(ctx context.Context, actorID, slug, username string) error {
	m, err := s.ownerTarget(ctx, actorID, slug, username, "owner cannot be removed")
	if err != nil {
		return err
	}
	return s.Members.Delete(ctx, m.ID)
}

func (s *MemberService) ChangeRole(ctx context.Context, actorID, slug, username, role string) error {
	if err := validMemberRole(role); err != nil {
		return err
	}
	m, err := s.ownerTarget(ctx, actorID, slug, username, "owner role cannot be changed")
	if err != nil {
		return err
	}
	m.Role = role
	return s.Members.Update(ctx, m)
}

func (s *MemberService) CreateInviteLink(ctx context.Context, actorID, slug, role string) (string, *models.InviteLink, error) {
	a, err := s.authorizeInvite(ctx, actorID, slug, role)
	if err != nil {
		return "", nil, err
	}
	token, err := auth.NewToken()
	if err != nil {
		return "", nil, err
	}
	link := &models.InviteLink{
		JourneyID: a.Journey.ID, TokenHash: auth.HashToken(token), Role: role,
		CreatedBy: actorID, ExpiresAt: s.Now().Add(InviteLinkTTL),
	}
	if err := s.Members.CreateInviteLink(ctx, link); err != nil {
		return "", nil, err
	}
	return strings.TrimRight(s.WebURL, "/") + "/join/" + token, link, nil
}

func (s *MemberService) RevokeInviteLink(ctx context.Context, actorID, slug, linkID string) error {
	a, err := s.Access.Load(ctx, slug, actorID)
	if err != nil {
		return err
	}
	if !a.IsOwner() {
		return ErrForbidden
	}
	l, err := s.Members.GetInviteLink(ctx, a.Journey.ID, linkID)
	if err != nil {
		return err
	}
	if l == nil {
		return ErrInviteNotFound
	}
	return s.Members.RevokeInviteLink(ctx, l.ID, s.Now())
}

func (s *MemberService) ListInviteLinks(ctx context.Context, actorID, slug string) ([]models.InviteLink, error) {
	a, err := s.Access.Load(ctx, slug, actorID)
	if err != nil {
		return nil, err
	}
	if !a.CanInvite() {
		return nil, ErrForbidden
	}
	return s.Members.ListInviteLinks(ctx, a.Journey.ID)
}

func (s *MemberService) JoinByLink(ctx context.Context, userID, token string) (string, error) {
	link, err := s.Members.GetInviteLinkByHash(ctx, auth.HashToken(token))
	if err != nil {
		return "", err
	}
	if link == nil || link.RevokedAt != nil || !s.Now().Before(link.ExpiresAt) {
		return "", ErrInviteNotFound
	}
	j, err := s.Journeys.GetByID(ctx, link.JourneyID)
	if err != nil {
		return "", err
	}
	if j == nil || j.OwnerID == "" {
		return "", ErrInviteNotFound
	}
	a, err := s.Access.Load(ctx, j.Slug, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", ErrInviteNotFound
		}
		return "", err
	}
	if a.IsMember() {
		return j.Slug, nil
	}
	existing, err := s.Members.Get(ctx, j.ID, userID)
	if err != nil {
		return "", err
	}
	if existing != nil {
		// Pending invite: clicking the link is consent. Keeps the invited role.
		existing.Status = models.StatusJoined
		if err := s.Members.Update(ctx, existing); err != nil {
			return "", err
		}
		return j.Slug, nil
	}
	if err := s.checkCapacity(ctx, j.ID); err != nil {
		return "", err
	}
	m := &models.JourneyMember{
		JourneyID: j.ID, UserID: userID, Role: link.Role,
		Status: models.StatusJoined, InvitedBy: link.CreatedBy,
	}
	if err := s.Members.Create(ctx, m); err != nil {
		return "", err
	}
	return j.Slug, nil
}
