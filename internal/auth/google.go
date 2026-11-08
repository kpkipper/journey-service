package auth

import (
	"context"
	"errors"
	"time"

	"google.golang.org/api/idtoken"
)

type GoogleIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
	HostedDomain  string // "hd" claim: set for Google Workspace accounts
}

type GoogleVerifier interface {
	Verify(ctx context.Context, idToken string) (*GoogleIdentity, error)
}

type googleVerifier struct{ clientID string }

// NewGoogleVerifier verifies Google ID tokens (signature, expiry, audience, issuer) against clientID.
func NewGoogleVerifier(clientID string) GoogleVerifier {
	return &googleVerifier{clientID: clientID}
}

func (g *googleVerifier) Verify(ctx context.Context, token string) (*GoogleIdentity, error) {
	// idtoken.Validate skips the audience check when audience is empty, so fail closed.
	if g.clientID == "" {
		return nil, errors.New("google client id not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p, err := idtoken.Validate(ctx, token, g.clientID)
	if err != nil {
		return nil, err
	}
	return identityFromPayload(p, g.clientID)
}

func identityFromPayload(p *idtoken.Payload, clientID string) (*GoogleIdentity, error) {
	if p == nil || clientID == "" || p.Audience != clientID {
		return nil, errors.New("google token audience mismatch")
	}
	if p.Issuer != "accounts.google.com" && p.Issuer != "https://accounts.google.com" {
		return nil, errors.New("google token issuer mismatch")
	}
	str := func(k string) string { s, _ := p.Claims[k].(string); return s }
	verified := false
	switch v := p.Claims["email_verified"].(type) {
	case bool:
		verified = v
	case string:
		verified = v == "true"
	}
	return &GoogleIdentity{
		Subject:       p.Subject,
		Email:         str("email"),
		EmailVerified: verified,
		Name:          str("name"),
		Picture:       str("picture"),
		HostedDomain:  str("hd"),
	}, nil
}
