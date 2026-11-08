package auth

import (
	"context"

	"google.golang.org/api/idtoken"
)

type GoogleIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
}

type GoogleVerifier interface {
	Verify(ctx context.Context, idToken string) (*GoogleIdentity, error)
}

type googleVerifier struct{ clientID string }

// NewGoogleVerifier verifies Google ID tokens (signature, expiry, audience) against clientID.
func NewGoogleVerifier(clientID string) GoogleVerifier {
	return &googleVerifier{clientID: clientID}
}

func (g *googleVerifier) Verify(ctx context.Context, token string) (*GoogleIdentity, error) {
	p, err := idtoken.Validate(ctx, token, g.clientID)
	if err != nil {
		return nil, err
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
	}, nil
}
