package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kpkipper/journey-service/internal/media"
	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/storage"
	"github.com/kpkipper/journey-service/pkg/logger"
	"gorm.io/gorm"
)

var (
	ErrMomentNotFound  = errors.New("moment not found")
	ErrTripMomentLimit = errors.New("trip has too many moments")
	ErrDayHasMoments   = errors.New("day has moments")
	ErrTripHasMoments  = errors.New("trip has moments")
)

const (
	maxPhotoBytes       int64 = 10485760
	maxVideoBytes       int64 = 104857600
	maxCaptionRunes           = 300
	maxMomentsPerTrip         = 500
	maxStartsPerHour          = 100
	maxPendingPerUser         = 40
	uploadURLTTL              = 15 * time.Minute
	minViewURLRemaining       = 30 * time.Minute
	maxSignedURLCache         = 5000
	maxConcurrentImage        = 2
	maxConcurrentVideo        = 1
)

var photoTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}
var videoTypes = map[string]bool{"video/mp4": true, "video/quicktime": true}

type MomentDeps struct {
	Access    *AccessService
	Privacy   *PrivacyService
	Moments   repository.MomentRepository
	Itinerary repository.ItineraryRepository
	Users     repository.UserRepository
	Blocks    repository.BlockRepository // optional (nil = no blocks)
	Store     storage.Store
	Video     media.VideoProcessor
	TempDir   string
	Now       func() time.Time
}

type MomentService struct {
	MomentDeps
	imageSem chan struct{}
	videoSem chan struct{}

	urlMu     sync.Mutex
	urlExpiry time.Time // expiry the cache currently holds URLs for
	urlCache  map[signedURLKey]string
}

type signedURLKey struct {
	key    string
	expiry time.Time
}

// signedExpiry is the next whole-hour boundary with at least 30 minutes left,
// so a view URL lives 30-90 minutes and is identical for everyone in the bucket.
func signedExpiry(now time.Time) time.Time {
	exp := now.Truncate(time.Hour).Add(time.Hour)
	if exp.Sub(now) < minViewURLRemaining {
		exp = exp.Add(time.Hour)
	}
	return exp
}

// signedGet returns a signed GET URL for a moments/ object, cached per
// (key, expiry). The cache is bounded and dropped when the bucket rolls over.
func (s *MomentService) signedGet(ctx context.Context, key string) (string, error) {
	now := s.Now()
	exp := signedExpiry(now)
	ck := signedURLKey{key, exp}
	s.urlMu.Lock()
	if !s.urlExpiry.Equal(exp) || len(s.urlCache) >= maxSignedURLCache {
		s.urlExpiry = exp
		s.urlCache = map[signedURLKey]string{}
	}
	if u, ok := s.urlCache[ck]; ok {
		s.urlMu.Unlock()
		return u, nil
	}
	s.urlMu.Unlock()
	u, err := s.Store.SignedGet(ctx, key, exp.Sub(now))
	if err != nil {
		return "", err
	}
	s.urlMu.Lock()
	if s.urlExpiry.Equal(exp) {
		s.urlCache[ck] = u
	}
	s.urlMu.Unlock()
	return u, nil
}

func NewMomentService(d MomentDeps) *MomentService {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &MomentService{
		MomentDeps: d,
		imageSem:   make(chan struct{}, maxConcurrentImage),
		videoSem:   make(chan struct{}, maxConcurrentVideo),
	}
}

type StartUploadInput struct {
	Kind        string `json:"kind"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	DayID       string `json:"day_id"`
	PlanID      string `json:"plan_id"`
	Caption     string `json:"caption"`
}

type MomentJourney struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

type MomentView struct {
	ID         string                `json:"id"`
	DayID      string                `json:"day_id"`
	PlanID     string                `json:"plan_id"`
	Kind       string                `json:"kind"`
	Status     string                `json:"status"`
	Caption    string                `json:"caption"`
	Width      int                   `json:"width"`
	Height     int                   `json:"height"`
	DurationMs int64                 `json:"duration_ms"`
	URL        string                `json:"url"`
	ThumbURL   string                `json:"thumb_url"`
	Uploader   *models.PublicProfile `json:"uploader"`
	CreatedAt  time.Time             `json:"created_at"`
	CanDelete  bool                  `json:"can_delete"`
	Journey    *MomentJourney        `json:"journey,omitempty"`
}

type UploadStart struct {
	Moment *MomentView           `json:"moment"`
	Upload *storage.UploadTarget `json:"upload"`
}

func maxBytesFor(kind string) int64 {
	if kind == models.MomentVideo {
		return maxVideoBytes
	}
	return maxPhotoBytes
}

// toView builds the client view of a moment, looking up its uploader. URLs
// are signed only for ready moments and only ever point at moments/ objects,
// never raw uploads.
func (s *MomentService) toView(ctx context.Context, m *models.Moment, canDelete bool) (*MomentView, error) {
	u, err := s.Users.GetByID(ctx, m.UploaderID)
	if err != nil {
		return nil, err
	}
	return s.viewWith(ctx, m, canDelete, u)
}

// viewWith is toView with the uploader already loaded (nil = unknown user).
func (s *MomentService) viewWith(ctx context.Context, m *models.Moment, canDelete bool, u *models.User) (*MomentView, error) {
	v := &MomentView{
		ID: m.ID, DayID: m.DayID, PlanID: m.PlanID, Kind: m.Kind, Status: m.Status,
		Caption: m.Caption, Width: m.Width, Height: m.Height, DurationMs: m.DurationMs,
		CreatedAt: m.CreatedAt, CanDelete: canDelete,
	}
	if u != nil {
		v.Uploader = &models.PublicProfile{Username: u.Username, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL, Bio: u.Bio}
	}
	if m.Status == models.MomentReady {
		var err error
		if v.URL, err = s.signedGet(ctx, m.ObjectKey); err != nil {
			return nil, err
		}
		if v.ThumbURL, err = s.signedGet(ctx, m.ThumbKey); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// uploaders loads the uploaders of ms in a single query, keyed by user id.
func (s *MomentService) uploaders(ctx context.Context, ms []models.Moment) (map[string]*models.User, error) {
	seen := map[string]bool{}
	var ids []string
	for i := range ms {
		if id := ms[i].UploaderID; id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	out := make(map[string]*models.User, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	users, err := s.Users.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range users {
		out[users[i].ID] = &users[i]
	}
	return out, nil
}

// checkCanPost: owner always; joined editor/traveler only when the trip lets
// members post; legacy trips nobody. Non-members get Access.Deny.
func (s *MomentService) checkCanPost(ctx context.Context, a *Access, userID string) error {
	if !a.IsMember() {
		return s.Access.Deny(ctx, a, userID)
	}
	if a.Journey.OwnerID == "" || (!a.IsOwner() && !a.Journey.MembersCanPost) {
		return ErrForbidden
	}
	return nil
}

func (s *MomentService) StartUpload(ctx context.Context, userID, slug string, in StartUploadInput) (*UploadStart, error) {
	a, err := s.Access.Load(ctx, slug, userID)
	if err != nil {
		return nil, err
	}
	if err := s.checkCanPost(ctx, a, userID); err != nil {
		return nil, err
	}

	switch in.Kind {
	case models.MomentPhoto:
		if !photoTypes[in.ContentType] {
			return nil, &ValidationError{Message: "unsupported content type"}
		}
	case models.MomentVideo:
		if !videoTypes[in.ContentType] {
			return nil, &ValidationError{Message: "unsupported content type"}
		}
	default:
		return nil, &ValidationError{Message: "kind must be photo or video"}
	}
	if in.Kind == models.MomentVideo && s.Video == nil {
		return nil, &ValidationError{Message: "video processing is unavailable"}
	}
	if in.SizeBytes <= 0 {
		return nil, &ValidationError{Message: "size_bytes must be positive"}
	}
	if in.SizeBytes > maxBytesFor(in.Kind) {
		return nil, &ValidationError{Message: "file too large"}
	}
	caption := strings.TrimSpace(in.Caption)
	if utf8.RuneCountInString(caption) > maxCaptionRunes {
		return nil, &ValidationError{Message: "caption must be at most 300 characters"}
	}

	day, err := s.Itinerary.GetDay(ctx, a.Journey.ID, in.DayID)
	if err != nil {
		return nil, err
	}
	if day == nil {
		return nil, ErrDayNotFound
	}
	if in.PlanID != "" {
		p, err := s.Itinerary.GetPlan(ctx, a.Journey.ID, in.PlanID)
		if err != nil {
			return nil, err
		}
		if p == nil || p.ItineraryDayID != day.ID {
			return nil, ErrPlanNotFound
		}
	}

	n, err := s.Moments.CountByJourney(ctx, a.Journey.ID)
	if err != nil {
		return nil, err
	}
	if n >= maxMomentsPerTrip {
		return nil, ErrTripMomentLimit
	}
	started, err := s.Moments.CountStartedSince(ctx, userID, s.Now().Add(-time.Hour))
	if err != nil {
		return nil, err
	}
	if started >= maxStartsPerHour {
		return nil, ErrTooManyRequests
	}
	pending, err := s.Moments.CountPendingByUploader(ctx, userID)
	if err != nil {
		return nil, err
	}
	if pending >= maxPendingPerUser {
		return nil, ErrTooManyRequests
	}

	m := &models.Moment{
		JourneyID: a.Journey.ID, DayID: day.ID, PlanID: in.PlanID, UploaderID: userID,
		Kind: in.Kind, Status: models.MomentPending, Caption: caption,
		ContentType: in.ContentType, SizeBytes: in.SizeBytes, CreatedAt: s.Now(),
	}
	if err := s.Moments.Create(ctx, m); err != nil {
		return nil, err
	}
	target, err := s.Store.SignedPut(ctx, storage.RawKey(m.JourneyID, m.ID), in.ContentType, maxBytesFor(in.Kind), uploadURLTTL)
	if err != nil {
		_ = s.Moments.Delete(ctx, m.ID)
		return nil, err
	}
	v, err := s.toView(ctx, m, true)
	if err != nil {
		_ = s.Moments.Delete(context.WithoutCancel(ctx), m.ID)
		return nil, err
	}
	return &UploadStart{Moment: v, Upload: target}, nil
}

func (s *MomentService) acquire(ctx context.Context, sem chan struct{}) error {
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// reject removes the raw upload and marks the row rejected after the file was refused.
func (s *MomentService) reject(ctx context.Context, m *models.Moment, msg string) error {
	_ = s.Store.Delete(ctx, storage.RawKey(m.JourneyID, m.ID))
	// Keep the row (status rejected) so the attempt still counts toward the
	// per-user start rate limit.
	if err := s.Moments.MarkRejected(ctx, m.ID); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return &ValidationError{Message: msg}
}

// mediaRejection maps a processing error to the sentinel's own message (never
// the wrapped detail, which may leak paths or ffmpeg output).
func mediaRejection(err error) (string, bool) {
	for _, sentinel := range []error{media.ErrUnsupportedFormat, media.ErrImageTooLarge, media.ErrVideoTooLong} {
		if errors.Is(err, sentinel) {
			return sentinel.Error(), true
		}
	}
	return "", false
}

func (s *MomentService) Complete(ctx context.Context, userID, slug, momentID string) (*MomentView, error) {
	a, err := s.Access.Load(ctx, slug, userID)
	if err != nil {
		return nil, err
	}
	if err := s.checkCanPost(ctx, a, userID); err != nil {
		return nil, err
	}
	m, err := s.Moments.Get(ctx, momentID)
	if err != nil {
		return nil, err
	}
	if m == nil || m.JourneyID != a.Journey.ID || m.Status != models.MomentPending || m.UploaderID != userID {
		return nil, ErrMomentNotFound
	}

	// Take the processing slot before opening the raw object so queued
	// requests do not hold storage readers.
	sem := s.imageSem
	if m.Kind == models.MomentVideo {
		if s.Video == nil {
			return nil, &ValidationError{Message: "video processing is unavailable"}
		}
		sem = s.videoSem
	}
	if err := s.acquire(ctx, sem); err != nil {
		return nil, err
	}
	defer func() { <-sem }()

	rawKey := storage.RawKey(m.JourneyID, m.ID)
	rc, size, err := s.Store.Open(ctx, rawKey)
	if errors.Is(err, storage.ErrObjectNotFound) {
		return nil, &ValidationError{Message: "upload not found"}
	}
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	max := maxBytesFor(m.Kind)
	if size > max {
		return nil, s.reject(ctx, m, "file too large")
	}

	var (
		full, thumb    []byte
		videoPath      string
		width, height  int
		durationMs     int64
		fullName, ctyp string
		fullSize       int64
	)
	if m.Kind == models.MomentVideo {
		in, err := os.CreateTemp(s.TempDir, "moment-in-*")
		if err != nil {
			return nil, err
		}
		defer os.Remove(in.Name())
		n, err := io.Copy(in, io.LimitReader(rc, max+1))
		if cerr := in.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
		if n > max {
			return nil, s.reject(ctx, m, "file too large")
		}
		outDir, err := os.MkdirTemp(s.TempDir, "moment-out-*")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(outDir)
		res, err := s.Video.Process(ctx, in.Name(), outDir)
		if err != nil {
			if msg, ok := mediaRejection(err); ok {
				return nil, s.reject(ctx, m, msg)
			}
			return nil, err
		}
		videoPath, thumb = res.Path, res.Thumb
		width, height, durationMs = res.Width, res.Height, res.DurationMs
		fullName, ctyp = "video.mp4", "video/mp4"
	} else {
		res, err := media.ProcessImage(io.LimitReader(rc, max+1))
		if err != nil {
			if msg, ok := mediaRejection(err); ok {
				return nil, s.reject(ctx, m, msg)
			}
			return nil, err
		}
		full, thumb = res.Full, res.Thumb
		width, height = res.Width, res.Height
		fullName, ctyp = "full.jpg", "image/jpeg"
		fullSize = int64(len(full))
	}

	fullKey := storage.MomentKey(m.JourneyID, m.ID, fullName)
	thumbKey := storage.MomentKey(m.JourneyID, m.ID, "thumb.jpg")
	// cleanup removes the processed objects unless a concurrent Complete has
	// already made this moment ready (the keys are identical and now in use).
	cleanup := func() {
		bg := context.WithoutCancel(ctx)
		if cur, err := s.Moments.Get(bg, m.ID); err != nil || (cur != nil && cur.Status == models.MomentReady) {
			return
		}
		_ = s.Store.Delete(bg, fullKey)
		_ = s.Store.Delete(bg, thumbKey)
	}

	if m.Kind == models.MomentVideo {
		f, err := os.Open(videoPath)
		if err != nil {
			return nil, err
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		fullSize = st.Size()
		err = s.Store.Put(ctx, fullKey, ctyp, f)
		f.Close()
		if err != nil {
			cleanup()
			return nil, err
		}
	} else if err := s.Store.Put(ctx, fullKey, ctyp, bytes.NewReader(full)); err != nil {
		cleanup()
		return nil, err
	}
	if err := s.Store.Put(ctx, thumbKey, "image/jpeg", bytes.NewReader(thumb)); err != nil {
		cleanup()
		return nil, err
	}

	m.Width, m.Height, m.DurationMs = width, height, durationMs
	m.ObjectKey, m.ThumbKey, m.SizeBytes = fullKey, thumbKey, fullSize
	if err := s.Moments.MarkReady(ctx, m); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			cleanup()
			return nil, ErrMomentNotFound
		}
		cleanup()
		return nil, err
	}
	m.Status = models.MomentReady
	_ = s.Store.Delete(ctx, rawKey)
	return s.toView(ctx, m, true)
}

// List returns the ready moments of a trip the viewer may see. A viewer who
// cannot see the trip gets ErrNotFound; a redacted view (stranger during the
// trip) gets nothing; moments on days or plans the view hides are dropped.
func (s *MomentService) List(ctx context.Context, viewerID, slug string) ([]MomentView, error) {
	v, err := s.Privacy.View(ctx, viewerID, slug)
	if err != nil {
		return nil, err
	}
	if v.Redacted {
		return []MomentView{}, nil
	}
	ms, err := s.Moments.ListReadyByJourney(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	return s.visibleMoments(ctx, viewerID, v, ms)
}

// visibleMoments applies the one rule set for which of a trip's moments a
// viewer may see (shared by List and the feed): ready only, on a day the view
// shows, plans hidden from non-members dropped, members-only dropped for
// non-members, uploaders block-related to the viewer dropped. Views are built
// with the uploaders loaded in one query. A redacted view yields nothing.
func (s *MomentService) visibleMoments(ctx context.Context, viewerID string, v *JourneyView, ms []models.Moment) ([]MomentView, error) {
	out := []MomentView{}
	if v.Redacted {
		return out, nil
	}
	visibleDay := map[string]bool{}
	visiblePlan := map[string]bool{}
	for _, d := range v.Days {
		visibleDay[d.ID] = true
		for _, p := range d.Plans {
			visiblePlan[p.ID] = true
		}
	}
	ms, err := s.WithoutBlockedUploaders(ctx, viewerID, ms)
	if err != nil {
		return nil, err
	}
	isOwner := v.Role == models.RoleOwner
	type kept struct {
		m         models.Moment
		planID    string
		canDelete bool
	}
	var keep []kept
	for i := range ms {
		m := &ms[i]
		if m.Status != models.MomentReady || !visibleDay[m.DayID] {
			continue
		}
		if m.MembersOnly && v.Role == "" {
			continue // came from a hidden plan that was later deleted
		}
		planID := m.PlanID
		if planID != "" && !visiblePlan[planID] {
			if v.Role == "" {
				continue // non-members: the plan may be hidden from them
			}
			// Members see every plan, so a plan missing here is dangling
			// (e.g. after a failed DetachPlan): show it as a day-level moment.
			planID = ""
		}
		canDelete := viewerID != "" && (m.UploaderID == viewerID || isOwner)
		keep = append(keep, kept{*m, planID, canDelete})
	}
	shown := make([]models.Moment, len(keep))
	for i := range keep {
		shown[i] = keep[i].m
	}
	users, err := s.uploaders(ctx, shown)
	if err != nil {
		return nil, err
	}
	for i := range keep {
		mv, err := s.viewWith(ctx, &keep[i].m, keep[i].canDelete, users[keep[i].m.UploaderID])
		if err != nil {
			return nil, err
		}
		mv.PlanID = keep[i].planID
		out = append(out, *mv)
	}
	return out, nil
}

// WithoutBlockedUploaders drops moments whose uploader is block-related to the
// viewer (either direction). One RelatedIDs query per call. Shared with the
// feed so every moment listing applies the same filter.
func (s *MomentService) WithoutBlockedUploaders(ctx context.Context, viewerID string, ms []models.Moment) ([]models.Moment, error) {
	if s.Blocks == nil || viewerID == "" || len(ms) == 0 {
		return ms, nil
	}
	related, err := s.Blocks.RelatedIDs(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	if len(related) == 0 {
		return ms, nil
	}
	out := make([]models.Moment, 0, len(ms))
	for _, m := range ms {
		if !related[m.UploaderID] {
			out = append(out, m)
		}
	}
	return out, nil
}

// UpdateCaption lets the uploader, while still a member, edit a ready moment's caption.
func (s *MomentService) UpdateCaption(ctx context.Context, userID, momentID, caption string) (*MomentView, error) {
	caption = strings.TrimSpace(caption)
	if utf8.RuneCountInString(caption) > maxCaptionRunes {
		return nil, &ValidationError{Message: "caption must be at most 300 characters"}
	}
	m, err := s.Moments.Get(ctx, momentID)
	if err != nil {
		return nil, err
	}
	if m == nil || userID == "" || m.UploaderID != userID || m.Status != models.MomentReady {
		return nil, ErrMomentNotFound
	}
	j, err := s.Access.journeys.GetByID(ctx, m.JourneyID)
	if err != nil {
		return nil, err
	}
	if j == nil {
		return nil, ErrMomentNotFound
	}
	a, err := s.Access.ForJourney(ctx, j, userID)
	if err != nil {
		return nil, err
	}
	if !a.IsMember() {
		return nil, ErrMomentNotFound
	}
	if err := s.Moments.UpdateCaption(ctx, m.ID, caption); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMomentNotFound
		}
		return nil, err
	}
	m.Caption = caption
	return s.toView(ctx, m, true)
}

// Delete removes a moment: the uploader (even after leaving the trip) or the trip owner.
func (s *MomentService) Delete(ctx context.Context, userID, momentID string) error {
	m, err := s.Moments.Get(ctx, momentID)
	if err != nil {
		return err
	}
	if m == nil || userID == "" {
		return ErrMomentNotFound
	}
	if m.UploaderID != userID {
		j, err := s.Access.journeys.GetByID(ctx, m.JourneyID)
		if err != nil {
			return err
		}
		if j == nil || j.OwnerID == "" || j.OwnerID != userID {
			return ErrMomentNotFound
		}
	}
	if err := s.Moments.Delete(ctx, m.ID); err != nil {
		return err
	}
	s.deleteFiles(ctx, m)
	return nil
}

// JourneySlugOf returns the slug of the trip a moment belongs to
// (ErrMomentNotFound when the moment or its trip is missing).
func (s *MomentService) JourneySlugOf(ctx context.Context, momentID string) (string, error) {
	m, err := s.Moments.Get(ctx, momentID)
	if err != nil {
		return "", err
	}
	if m == nil {
		return "", ErrMomentNotFound
	}
	j, err := s.Access.journeys.GetByID(ctx, m.JourneyID)
	if err != nil {
		return "", err
	}
	if j == nil {
		return "", ErrMomentNotFound
	}
	return j.Slug, nil
}

// AdminDelete removes a moment and its files without any permission check.
// It is only for the operator CLI.
func (s *MomentService) AdminDelete(ctx context.Context, momentID string) error {
	m, err := s.Moments.Get(ctx, momentID)
	if err != nil {
		return err
	}
	if m == nil {
		return ErrMomentNotFound
	}
	if err := s.Moments.Delete(ctx, m.ID); err != nil {
		return err
	}
	s.deleteFiles(ctx, m)
	return nil
}

// deleteFiles removes every object that could exist for the moment, best effort.
func (s *MomentService) deleteFiles(ctx context.Context, m *models.Moment) {
	full := "full.jpg"
	if m.Kind == models.MomentVideo {
		full = "video.mp4"
	}
	for _, key := range []string{
		storage.RawKey(m.JourneyID, m.ID),
		storage.MomentKey(m.JourneyID, m.ID, full),
		storage.MomentKey(m.JourneyID, m.ID, "thumb.jpg"),
	} {
		if err := s.Store.Delete(ctx, key); err != nil {
			logger.Get().Error().Err(err).Str("key", key).Msg("moment file delete failed")
		}
	}
}

// ListMine returns the caller's ready moments across all existing trips, newest first.
func (s *MomentService) ListMine(ctx context.Context, userID string) ([]MomentView, error) {
	ms, err := s.Moments.ListByUploader(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := []MomentView{}
	trips := map[string]*models.Journey{}
	var shown []models.Moment
	for i := range ms {
		m := &ms[i]
		if m.Status != models.MomentReady {
			continue
		}
		j, seen := trips[m.JourneyID]
		if !seen {
			if j, err = s.Access.journeys.GetByID(ctx, m.JourneyID); err != nil {
				return nil, err
			}
			trips[m.JourneyID] = j
		}
		if j == nil {
			continue
		}
		shown = append(shown, *m)
	}
	users, err := s.uploaders(ctx, shown)
	if err != nil {
		return nil, err
	}
	for i := range shown {
		m := &shown[i]
		mv, err := s.viewWith(ctx, m, true, users[m.UploaderID])
		if err != nil {
			return nil, err
		}
		j := trips[m.JourneyID]
		mv.Journey = &MomentJourney{Slug: j.Slug, Title: j.Title}
		out = append(out, *mv)
	}
	return out, nil
}

// DeleteForJourney removes all moments of a trip (any status) and their files.
func (s *MomentService) DeleteForJourney(ctx context.Context, journeyID string) error {
	ms, err := s.Moments.ListByJourney(ctx, journeyID)
	if err != nil {
		return err
	}
	for i := range ms {
		m := &ms[i]
		if err := s.Moments.Delete(ctx, m.ID); err != nil {
			return err
		}
		s.deleteFiles(ctx, m)
	}
	return nil
}

// CleanupPending removes uploads still pending after an hour (row and raw
// object) and old rejected rows. It returns the number of rows removed.
func (s *MomentService) CleanupPending(ctx context.Context) (int, error) {
	cutoff := s.Now().Add(-time.Hour)
	ms, err := s.Moments.ListPendingBefore(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range ms {
		m := &ms[i]
		if m.Status != models.MomentPending {
			continue
		}
		if err := s.Moments.Delete(ctx, m.ID); err != nil {
			return n, err
		}
		n++
		if err := s.Store.Delete(ctx, storage.RawKey(m.JourneyID, m.ID)); err != nil {
			logger.Get().Error().Err(err).Str("moment", m.ID).Msg("raw upload delete failed")
		}
	}
	r, err := s.Moments.DeleteRejectedBefore(ctx, cutoff)
	return n + int(r), err
}
