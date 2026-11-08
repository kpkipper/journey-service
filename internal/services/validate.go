package services

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/kpkipper/journey-service/internal/models"
)

const maxURLBytes = 2048

// validHTTPSURL reports whether s is an https URL with a host and at most 2048 bytes.
func validHTTPSURL(s string) bool {
	if len(s) > maxURLBytes {
		return false
	}
	pu, err := url.Parse(s)
	return err == nil && pu.Scheme == "https" && pu.Host != ""
}

// normalizeMapURL prefixes "https://" to a scheme-less value (e.g. "maps.app.goo.gl/x")
// and reports whether the result is an http(s) URL with a host and at most 2048 bytes.
// Empty stays empty and valid. Other schemes (javascript:, mailto:, ...) are rejected.
func normalizeMapURL(s string) (string, bool) {
	if s == "" {
		return "", true
	}
	if !strings.Contains(s, "://") {
		head, _, _ := strings.Cut(s, "/")
		if !strings.Contains(head, ":") {
			s = "https://" + s
		}
	}
	if len(s) > maxURLBytes {
		return "", false
	}
	pu, err := url.Parse(s)
	if err != nil || (pu.Scheme != "http" && pu.Scheme != "https") || pu.Host == "" {
		return "", false
	}
	return s, true
}

var errBadMapURL = &ValidationError{Message: "map_url must be an http(s) URL of at most 2048 bytes"}

// validateCover checks cover_url (set on create and via PATCH; not part of a full PUT).
func validateCover(j *models.Journey) error {
	if j.CoverURL != "" && !validHTTPSURL(j.CoverURL) {
		return &ValidationError{Message: "cover_url must be an https URL of at most 2048 bytes"}
	}
	return nil
}

// validateJourney applies the date-order and map_url rules shared by Create and full update.
// It normalizes plan map_urls in place; errors name the offending day and plan (1-based).
func validateJourney(j *models.Journey) error {
	if j.ReturnDate.Before(j.DepartureDate) {
		return &ValidationError{Message: "return_date must not be before departure_date"}
	}
	for di := range j.ItineraryDays {
		plans := j.ItineraryDays[di].Plans
		for pi := range plans {
			u, ok := normalizeMapURL(plans[pi].MapURL)
			if !ok {
				return &ValidationError{Message: fmt.Sprintf("day %d, plan %d: invalid map_url", di+1, pi+1)}
			}
			plans[pi].MapURL = u
		}
	}
	return nil
}
