package services

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
)

var (
	ErrUsernameTaken = errors.New("username already taken")
	ErrUserNotFound  = errors.New("user not found")
)

// ValidationError is returned for invalid input; Message is safe to show to the client.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

const (
	usernameAlphabet   = "0123456789abcdefghijklmnopqrstuvwxyz"
	maxDisplayNameLen  = 50
	maxBioLen          = 160
	maxGeneratedBase   = 15
	fallbackUsername   = "traveler"
	maxUsernameRetries = 5
)

var (
	usernameRe    = regexp.MustCompile(`^[a-z0-9_]{3,20}$`)
	nonUsernameRe = regexp.MustCompile(`[^a-z0-9_]`)
)

type UserService struct {
	repo    repository.UserRepository
	blocks  repository.BlockRepository  // optional
	follows repository.FollowRepository // optional
}

func NewUserService(repo repository.UserRepository) *UserService {
	return &UserService{repo: repo}
}

// WithBlocks lets PublicProfileFor hide users from those who blocked them.
func (s *UserService) WithBlocks(b repository.BlockRepository) *UserService {
	s.blocks = b
	return s
}

// WithFollows lets UpdateProfile accept pending requests when approval is turned off.
func (s *UserService) WithFollows(f repository.FollowRepository) *UserService {
	s.follows = f
	return s
}

func (s *UserService) GetByID(ctx context.Context, id string) (*models.User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrUserNotFound
	}
	return u, nil
}

// PublicProfileFor is GetPublicProfile as seen by viewerID ("" = anonymous):
// a viewer blocked by the user gets ErrUserNotFound.
func (s *UserService) PublicProfileFor(ctx context.Context, viewerID, username string) (*models.PublicProfile, error) {
	p, err := s.GetPublicProfile(ctx, username)
	if err != nil {
		return nil, err
	}
	if s.blocks == nil || viewerID == "" {
		return p, nil
	}
	u, err := s.repo.GetByUsername(ctx, strings.ToLower(username))
	if err != nil {
		return nil, err
	}
	if u == nil || u.ID == viewerID {
		return p, nil
	}
	blocked, err := s.blocks.IsBlocking(ctx, u.ID, viewerID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, ErrUserNotFound
	}
	return p, nil
}

func (s *UserService) GetPublicProfile(ctx context.Context, username string) (*models.PublicProfile, error) {
	u, err := s.repo.GetByUsername(ctx, strings.ToLower(username))
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrUserNotFound
	}
	return &models.PublicProfile{
		Username:    u.Username,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
		Bio:         u.Bio,
	}, nil
}

func (s *UserService) UpdateProfile(ctx context.Context, userID string, in models.ProfileUpdate) (*models.User, error) {
	u, err := s.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if in.Username != nil {
		name := strings.ToLower(*in.Username)
		if !usernameRe.MatchString(name) {
			return nil, &ValidationError{Message: "username must be 3-20 characters: a-z, 0-9, _"}
		}
		other, err := s.repo.GetByUsername(ctx, name)
		if err != nil {
			return nil, err
		}
		if other != nil && other.ID != u.ID {
			return nil, ErrUsernameTaken
		}
		u.Username = name
	}
	if in.DisplayName != nil {
		if utf8.RuneCountInString(*in.DisplayName) > maxDisplayNameLen {
			return nil, &ValidationError{Message: "display_name must be at most 50 characters"}
		}
		u.DisplayName = *in.DisplayName
	}
	if in.Bio != nil {
		if utf8.RuneCountInString(*in.Bio) > maxBioLen {
			return nil, &ValidationError{Message: "bio must be at most 160 characters"}
		}
		u.Bio = *in.Bio
	}
	if in.AvatarURL != nil {
		if *in.AvatarURL != "" && !validHTTPSURL(*in.AvatarURL) {
			return nil, &ValidationError{Message: "avatar_url must be an https URL of at most 2048 bytes"}
		}
		u.AvatarURL = *in.AvatarURL
	}

	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}
	if in.FollowApproval != nil {
		on := *in.FollowApproval
		if err := s.repo.SetFollowApproval(ctx, u.ID, on); err != nil {
			return nil, err
		}
		if u.FollowApproval && !on && s.follows != nil {
			var skip map[string]bool
			if s.blocks != nil {
				if skip, err = s.blocks.RelatedIDs(ctx, u.ID); err != nil {
					return nil, err
				}
			}
			if err := s.follows.AcceptAllRequests(ctx, u.ID, skip); err != nil {
				return nil, err
			}
		}
		u.FollowApproval = on
	}
	return u, nil
}

// GenerateUsername derives a unique username from the email's local part.
func (s *UserService) GenerateUsername(ctx context.Context, email string) (string, error) {
	local, _, _ := strings.Cut(email, "@")
	base := nonUsernameRe.ReplaceAllString(strings.ToLower(local), "")
	if len(base) > maxGeneratedBase {
		base = base[:maxGeneratedBase]
	}
	if len(base) < 3 {
		base = fallbackUsername
	}

	candidate := base
	for i := 0; i <= maxUsernameRetries; i++ {
		if i > 0 {
			suffix, err := gonanoid.Generate(usernameAlphabet, 4)
			if err != nil {
				return "", err
			}
			candidate = base + "_" + suffix
		}
		existing, err := s.repo.GetByUsername(ctx, candidate)
		if err != nil {
			return "", err
		}
		if existing == nil {
			return candidate, nil
		}
	}
	return "", errors.New("could not generate a unique username")
}
