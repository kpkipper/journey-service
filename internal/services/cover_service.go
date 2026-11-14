package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"regexp"
	"sync"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"github.com/kpkipper/journey-service/internal/media"
	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/storage"
	"github.com/kpkipper/journey-service/pkg/logger"
)

const (
	// Cover upload IDs are 12 characters, moment IDs 10, so a cover upload can
	// never name (and steal) another member's raw moment upload.
	coverIDLen          = 12
	coverIDAlphabet     = "0123456789abcdefghijklmnopqrstuvwxyz"
	maxCoverStartsPerHr = 20
)

var coverIDRe = regexp.MustCompile(`^[0-9a-z]{12}$`)

var (
	// ErrCoverChanged means another cover change finished first; the client can retry.
	ErrCoverChanged = errors.New("the cover was changed at the same time, please try again")
	// ErrCoverUploaded refuses a linked cover_url while an uploaded cover exists.
	ErrCoverUploaded = errors.New("remove the uploaded cover before setting cover_url")
)

// CoverResolver turns a journey into the cover URL clients should load.
type CoverResolver interface {
	URL(ctx context.Context, j *models.Journey) string
}

type CoverDeps struct {
	Access   *AccessService
	Journeys repository.JourneyRepository
	Store    storage.Store
	Now      func() time.Time // nil = time.Now
}

// CoverService lets a trip owner upload or remove the trip's cover image and
// signs cover URLs for everyone who can see the trip.
type CoverService struct {
	CoverDeps
	sem chan struct{}

	mu        sync.Mutex
	starts    map[string][]time.Time // userID -> recent upload starts
	urlExpiry time.Time
	urlCache  map[string]string // key -> signed URL for urlExpiry
}

func NewCoverService(d CoverDeps) *CoverService {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &CoverService{CoverDeps: d, sem: make(chan struct{}, 2), starts: map[string][]time.Time{}}
}

type CoverUploadInput struct {
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

type CoverUploadStart struct {
	UploadID string                `json:"upload_id"`
	Upload   *storage.UploadTarget `json:"upload"`
}

type CoverResult struct {
	CoverURL string `json:"cover_url"`
}

func (s *CoverService) loadOwned(ctx context.Context, userID, slug string) (*Access, error) {
	a, err := s.Access.Load(ctx, slug, userID)
	if err != nil {
		return nil, err
	}
	if !a.IsOwner() {
		return nil, s.Access.Deny(ctx, a, userID)
	}
	return a, nil
}

// allowStart records an upload start, refusing more than maxCoverStartsPerHr per user.
func (s *CoverService) allowStart(userID string) bool {
	now := s.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	recent := s.starts[userID][:0]
	for _, t := range s.starts[userID] {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	if len(recent) == 0 {
		delete(s.starts, userID) // keep the map to active users only
	}
	if len(recent) >= maxCoverStartsPerHr {
		s.starts[userID] = recent
		return false
	}
	s.starts[userID] = append(recent, now)
	return true
}

// StartUpload gives the owner a signed URL to upload the new cover's raw file.
func (s *CoverService) StartUpload(ctx context.Context, userID, slug string, in CoverUploadInput) (*CoverUploadStart, error) {
	a, err := s.loadOwned(ctx, userID, slug)
	if err != nil {
		return nil, err
	}
	if !photoTypes[in.ContentType] {
		return nil, &ValidationError{Message: "unsupported content type"}
	}
	if in.SizeBytes <= 0 {
		return nil, &ValidationError{Message: "size_bytes must be positive"}
	}
	if in.SizeBytes > maxPhotoBytes {
		return nil, &ValidationError{Message: "file too large"}
	}
	if !s.allowStart(userID) {
		return nil, ErrTooManyRequests
	}
	id, err := gonanoid.Generate(coverIDAlphabet, coverIDLen)
	if err != nil {
		return nil, err
	}
	target, err := s.Store.SignedPut(ctx, storage.RawKey(a.Journey.ID, id), in.ContentType, maxPhotoBytes, uploadURLTTL)
	if err != nil {
		return nil, err
	}
	return &CoverUploadStart{UploadID: id, Upload: target}, nil
}

// Complete re-encodes the uploaded file (no EXIF/GPS survives), stores it as
// the trip's cover and deletes the previous cover.
func (s *CoverService) Complete(ctx context.Context, userID, slug, uploadID string) (*CoverResult, error) {
	a, err := s.loadOwned(ctx, userID, slug)
	if err != nil {
		return nil, err
	}
	if !coverIDRe.MatchString(uploadID) {
		return nil, &ValidationError{Message: "invalid upload id"}
	}
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.sem }()

	j := a.Journey
	rawKey := storage.RawKey(j.ID, uploadID)
	rc, size, err := s.Store.Open(ctx, rawKey)
	if errors.Is(err, storage.ErrObjectNotFound) {
		return nil, &ValidationError{Message: "upload not found"}
	}
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	// The raw file is never needed again, whatever happens below.
	defer func() { _ = s.Store.Delete(context.WithoutCancel(ctx), rawKey) }()
	if size > maxPhotoBytes {
		return nil, &ValidationError{Message: "file too large"}
	}
	res, err := media.ProcessImage(io.LimitReader(rc, maxPhotoBytes+1))
	if err != nil {
		if msg, ok := mediaRejection(err); ok {
			return nil, &ValidationError{Message: msg}
		}
		return nil, err
	}

	// A fresh name per completion: a retried or doubled complete for the same
	// upload can then never delete the other one's live file.
	objID, err := gonanoid.Generate(coverIDAlphabet, coverIDLen)
	if err != nil {
		return nil, err
	}
	key := storage.CoverKey(j.ID, objID)
	if err := s.Store.Put(ctx, key, "image/jpeg", bytes.NewReader(res.Full)); err != nil {
		return nil, err
	}
	ok, err := s.Journeys.SwapCoverKey(ctx, j.ID, j.CoverKey, key)
	if err != nil || !ok {
		s.deleteFile(ctx, j.ID, key)
		if err == nil {
			err = s.swapLost(ctx, j.ID)
		}
		return nil, err
	}
	if j.CoverKey != "" {
		s.deleteFile(ctx, j.ID, j.CoverKey)
	}
	j.CoverKey, j.CoverURL = key, ""
	return &CoverResult{CoverURL: s.URL(ctx, j)}, nil
}

// Remove clears the trip's cover (uploaded or linked); the trip falls back to its gradient.
func (s *CoverService) Remove(ctx context.Context, userID, slug string) error {
	a, err := s.loadOwned(ctx, userID, slug)
	if err != nil {
		return err
	}
	j := a.Journey
	ok, err := s.Journeys.SwapCoverKey(ctx, j.ID, j.CoverKey, "")
	if err != nil {
		return err
	}
	if !ok {
		return s.swapLost(ctx, j.ID)
	}
	if j.CoverKey != "" {
		s.deleteFile(ctx, j.ID, j.CoverKey)
	}
	return nil
}

// swapLost explains a failed cover swap: the trip was deleted meanwhile, or
// another cover change won.
func (s *CoverService) swapLost(ctx context.Context, journeyID string) error {
	if j, err := s.Journeys.GetByID(ctx, journeyID); err == nil && j == nil {
		return ErrNotFound
	}
	return ErrCoverChanged
}

// deleteFile removes a cover file; covers/ has no lifecycle rule, so a failure is logged.
func (s *CoverService) deleteFile(ctx context.Context, journeyID, key string) {
	if err := s.Store.Delete(context.WithoutCancel(ctx), key); err != nil {
		logger.Get().Error().Err(err).Str("journey", journeyID).Str("key", key).Msg("delete cover file")
	}
}

// DeleteForJourney removes a deleted trip's cover file.
func (s *CoverService) DeleteForJourney(ctx context.Context, journeyID string) error {
	key, err := s.Journeys.CoverKeyOf(ctx, journeyID)
	if err != nil || key == "" {
		return err
	}
	return s.Store.Delete(ctx, key)
}

// URL is the cover to show: a signed URL for an uploaded cover (shared by
// everyone within the same hour bucket, like moment URLs), else the linked
// cover_url, else "" (the client draws the gradient). Callers only pass
// journeys the viewer may see.
func (s *CoverService) URL(ctx context.Context, j *models.Journey) string {
	if j.CoverKey == "" {
		return j.CoverURL
	}
	now := s.Now()
	exp := signedExpiry(now)
	s.mu.Lock()
	if !s.urlExpiry.Equal(exp) || len(s.urlCache) >= maxSignedURLCache {
		s.urlExpiry, s.urlCache = exp, map[string]string{}
	}
	if u, ok := s.urlCache[j.CoverKey]; ok {
		s.mu.Unlock()
		return u
	}
	s.mu.Unlock()
	u, err := s.Store.SignedGet(ctx, j.CoverKey, exp.Sub(now))
	if err != nil {
		logger.Get().Error().Err(err).Str("journey", j.ID).Msg("sign cover url")
		return ""
	}
	s.mu.Lock()
	if s.urlExpiry.Equal(exp) {
		s.urlCache[j.CoverKey] = u
	}
	s.mu.Unlock()
	return u
}

// coverURL resolves a journey's cover with r, or falls back to the linked URL.
func coverURL(ctx context.Context, r CoverResolver, j *models.Journey) string {
	if r == nil {
		if j.CoverKey != "" {
			return ""
		}
		return j.CoverURL
	}
	return r.URL(ctx, j)
}
