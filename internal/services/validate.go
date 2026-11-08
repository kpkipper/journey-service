package services

import "net/url"

const maxURLBytes = 2048

// validHTTPSURL reports whether s is an https URL with a host and at most 2048 bytes.
func validHTTPSURL(s string) bool {
	if len(s) > maxURLBytes {
		return false
	}
	pu, err := url.Parse(s)
	return err == nil && pu.Scheme == "https" && pu.Host != ""
}
