package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/idtoken"
)

const testClientID = "client-123.apps.googleusercontent.com"

func payload(aud, iss string, claims map[string]interface{}) *idtoken.Payload {
	return &idtoken.Payload{Audience: aud, Issuer: iss, Subject: "sub-1", Claims: claims}
}

func TestGoogleVerifier_EmptyClientIDFailsClosed(t *testing.T) {
	_, err := NewGoogleVerifier("").Verify(context.Background(), "any.token.value")
	assert.Error(t, err)
}

func TestIdentityFromPayload_WrongAudience(t *testing.T) {
	_, err := identityFromPayload(payload("other-client", "accounts.google.com", nil), testClientID)
	assert.Error(t, err)
}

func TestIdentityFromPayload_EmptyClientID(t *testing.T) {
	_, err := identityFromPayload(payload("", "accounts.google.com", nil), "")
	assert.Error(t, err)
}

func TestIdentityFromPayload_Issuer(t *testing.T) {
	_, err := identityFromPayload(payload(testClientID, "https://evil.example", nil), testClientID)
	assert.Error(t, err)
	for _, iss := range []string{"accounts.google.com", "https://accounts.google.com"} {
		_, err := identityFromPayload(payload(testClientID, iss, nil), testClientID)
		assert.NoError(t, err, iss)
	}
}

func TestIdentityFromPayload_Claims(t *testing.T) {
	id, err := identityFromPayload(payload(testClientID, "accounts.google.com", map[string]interface{}{
		"email": "a@x.com", "email_verified": true, "name": "A", "picture": "http://p", "hd": "x.com",
	}), testClientID)
	require.NoError(t, err)
	assert.Equal(t, &GoogleIdentity{Subject: "sub-1", Email: "a@x.com", EmailVerified: true, Name: "A", Picture: "http://p", HostedDomain: "x.com"}, id)

	cases := []struct {
		name   string
		claims map[string]interface{}
		want   bool
	}{
		{"bool true", map[string]interface{}{"email_verified": true}, true},
		{"string true", map[string]interface{}{"email_verified": "true"}, true},
		{"bool false", map[string]interface{}{"email_verified": false}, false},
		{"string false", map[string]interface{}{"email_verified": "false"}, false},
		{"missing", map[string]interface{}{}, false},
	}
	for _, c := range cases {
		id, err := identityFromPayload(payload(testClientID, "accounts.google.com", c.claims), testClientID)
		require.NoError(t, err, c.name)
		assert.Equal(t, c.want, id.EmailVerified, c.name)
	}
}
