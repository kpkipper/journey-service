package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kpkipper/journey-service/internal/media"
	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/storage"
	"gorm.io/gorm"
)

var (
	ErrMomentNotFound  = errors.New("moment not found")
	ErrTripMomentLimit = errors.New("trip has too many moments")
)

const (
	maxPhotoBytes      int64 = 10485760
	maxVideoBytes      int64 = 104857600
	maxCaptionRunes          = 300
	maxMomentsPerTrip        = 500
	maxStartsPerHour         = 100
	maxPendingPerUser        = 40
	uploadURLTTL             = 15 * time.Minute
	momentViewURLTTL         = time.Hour
	maxConcurrentImage       = 2
	maxConcurrentVideo       = 1
)

var photoTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}
var videoTypes = map[string]bool{"video/mp4": true, "video/quicktime": true}

type MomentDeps struct {
	Access    *AccessService
	Privacy   *PrivacyService
	Moments   repository.MomentRepository
	Itinerary repository.ItineraryRepository
	Users     repository.UserRepository
	Store     storage.Store
	Video     media.VideoProcessor
	TempDir   string
	Now       func() time.Time
}

type MomentService struct {
	MomentDeps
	imageSem chan struct{}
	videoSem chan struct{}
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

// toView builds the client view of a moment. URLs are signed only for ready
// moments and only ever point at moments/... objects, never raw uploads.
func (s *MomentService) toView(ctx context.Context, m *models.Moment, canDelete bool) (*MomentView, error) {
	v := &MomentView{
		ID: m.ID, DayID: m.DayID, PlanID: m.PlanID, Kind: m.Kind, Status: m.Status,
		Caption: m.Caption, Width: m.Width, Height: m.Height, DurationMs: m.DurationMs,
		CreatedAt: m.CreatedAt, CanDelete: canDelete,
	}
	u, err := s.Users.GetByID(ctx, m.UploaderID)
	if err != nil {
		return nil, err
	}
	if u != nil {
		v.Uploader = &models.PublicProfile{Username: u.Username, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL, Bio: u.Bio}
	}
	if m.Status == models.MomentReady {
		if v.URL, err = s.Store.SignedGet(ctx, m.ObjectKey, momentViewURLTTL); err != nil {
			return nil, err
		}
		if v.ThumbURL, err = s.Store.SignedGet(ctx, m.ThumbKey, momentViewURLTTL); err != nil {
			return nil, err
		}
	}
	return v, nil
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
		if s.Video == nil {
			return nil, &ValidationError{Message: "video processing is unavailable"}
		}
		if err := s.acquire(ctx, s.videoSem); err != nil {
			return nil, err
		}
		defer func() { <-s.videoSem }()

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
		if err := s.acquire(ctx, s.imageSem); err != nil {
			return nil, err
		}
		defer func() { <-s.imageSem }()
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
