package services

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
)

var ErrReportNotFound = errors.New("report not found")

const (
	maxReportNoteRunes    = 500
	maxReportsPerReporter = 20
)

var reportReasons = map[string]bool{"spam": true, "harassment": true, "inappropriate": true, "privacy": true, "other": true}

type ReportInput struct {
	TargetType string `json:"target_type"`
	Target     string `json:"target"`
	Reason     string `json:"reason"`
	Note       string `json:"note"`
}

type ReportDeps struct {
	Reports repository.ReportRepository
	Users   repository.UserRepository
	Privacy *PrivacyService
	Moments *MomentService
	Blocks  repository.BlockRepository // optional
	Now     func() time.Time
}

type ReportService struct {
	ReportDeps
}

func NewReportService(d ReportDeps) *ReportService {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &ReportService{ReportDeps: d}
}

// Create files a report. The target must exist and be visible to the reporter,
// otherwise the matching not-found error is returned so reports cannot probe
// hidden things. An earlier open report by the same reporter on the same target
// is returned with created=false.
func (s *ReportService) Create(ctx context.Context, reporterID string, in ReportInput) (*models.Report, bool, error) {
	reason := strings.TrimSpace(in.Reason)
	if !reportReasons[reason] {
		return nil, false, &ValidationError{Message: "reason must be spam, harassment, inappropriate, privacy or other"}
	}
	note := strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(note) > maxReportNoteRunes {
		return nil, false, &ValidationError{Message: "note must be at most 500 characters"}
	}

	var targetID string
	var err error
	switch in.TargetType {
	case models.ReportTargetUser:
		targetID, err = s.resolveUser(ctx, reporterID, in.Target)
	case models.ReportTargetJourney:
		targetID, err = s.resolveJourney(ctx, reporterID, in.Target)
	case models.ReportTargetMoment:
		targetID, err = s.resolveMoment(ctx, reporterID, in.Target)
	default:
		return nil, false, &ValidationError{Message: "target_type must be user, journey or moment"}
	}
	if err != nil {
		return nil, false, err
	}

	existing, err := s.Reports.FindOpen(ctx, reporterID, in.TargetType, targetID)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, false, nil
	}

	n, err := s.Reports.CountSince(ctx, reporterID, s.Now().Add(-24*time.Hour))
	if err != nil {
		return nil, false, err
	}
	if n >= maxReportsPerReporter {
		return nil, false, ErrTooManyRequests
	}

	r := &models.Report{ReporterID: reporterID, TargetType: in.TargetType, TargetID: targetID,
		Reason: reason, Note: note, Status: models.ReportOpen, CreatedAt: s.Now()}
	if err := s.Reports.Create(ctx, r); err != nil {
		return nil, false, err
	}
	return r, true, nil
}

func (s *ReportService) resolveUser(ctx context.Context, reporterID, username string) (string, error) {
	u, err := s.Users.GetByUsername(ctx, normalizeUsername(username))
	if err != nil {
		return "", err
	}
	if u == nil {
		return "", ErrUserNotFound
	}
	if u.ID == reporterID {
		return "", &ValidationError{Message: "cannot report yourself"}
	}
	// A reporter blocked by the target sees no profile; the one who blocked may still report.
	if s.Blocks != nil {
		blocked, err := s.Blocks.IsBlocking(ctx, u.ID, reporterID)
		if err != nil {
			return "", err
		}
		if blocked {
			return "", ErrUserNotFound
		}
	}
	return u.ID, nil
}

func (s *ReportService) resolveJourney(ctx context.Context, reporterID, slug string) (string, error) {
	v, err := s.Privacy.View(ctx, reporterID, slug)
	if err != nil {
		return "", err
	}
	return v.ID, nil
}

func (s *ReportService) resolveMoment(ctx context.Context, reporterID, id string) (string, error) {
	slug, err := s.Moments.JourneySlugOf(ctx, id)
	if err != nil {
		return "", err
	}
	visible, err := s.Moments.List(ctx, reporterID, slug)
	if errors.Is(err, ErrNotFound) {
		return "", ErrMomentNotFound
	}
	if err != nil {
		return "", err
	}
	for i := range visible {
		if visible[i].ID == id {
			return id, nil
		}
	}
	return "", ErrMomentNotFound
}

func (s *ReportService) ListOpen(ctx context.Context) ([]models.Report, error) {
	return s.Reports.ListOpen(ctx)
}

func (s *ReportService) Resolve(ctx context.Context, id string) error {
	r, err := s.Reports.Get(ctx, id)
	if err != nil {
		return err
	}
	if r == nil {
		return ErrReportNotFound
	}
	return s.Reports.Resolve(ctx, id, s.Now())
}
