package services

import (
	"net/url"

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

// validMapURL reports whether s is an http(s) URL with a host and at most 2048 bytes.
func validMapURL(s string) bool {
	if len(s) > maxURLBytes {
		return false
	}
	pu, err := url.Parse(s)
	return err == nil && (pu.Scheme == "http" || pu.Scheme == "https") && pu.Host != ""
}

var errBadMapURL = &ValidationError{Message: "map_url must be an http(s) URL of at most 2048 bytes"}

// validateJourney applies the cover_url, date-order and map_url rules shared by every write path.
func validateJourney(j *models.Journey) error {
	if j.CoverURL != "" && !validHTTPSURL(j.CoverURL) {
		return &ValidationError{Message: "cover_url must be an https URL of at most 2048 bytes"}
	}
	if j.ReturnDate.Before(j.DepartureDate) {
		return &ValidationError{Message: "return_date must not be before departure_date"}
	}
	for _, d := range j.ItineraryDays {
		for _, p := range d.Plans {
			if p.MapURL != "" && !validMapURL(p.MapURL) {
				return errBadMapURL
			}
		}
	}
	return nil
}
