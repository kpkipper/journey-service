package storage

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func fakeCreds(t *testing.T) []byte {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(k)
	require.NoError(t, err)
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	b, err := json.Marshal(map[string]string{
		"type":         "service_account",
		"project_id":   "p",
		"client_email": "sa@p.iam.gserviceaccount.com",
		"private_key":  string(pemKey),
		"token_uri":    "https://oauth2.googleapis.com/token",
	})
	require.NoError(t, err)
	return b
}

func TestNewFirebaseStore_RejectsMissingBucket(t *testing.T) {
	_, err := NewFirebaseStore(context.Background(), "", fakeCreds(t))
	require.Error(t, err)
}

func TestNewFirebaseStore_RejectsBadCredentials(t *testing.T) {
	ctx := context.Background()
	for name, c := range map[string][]byte{
		"empty":         nil,
		"garbage":       []byte("not json !!"),
		"no email":      []byte(`{"private_key":"x"}`),
		"no key":        []byte(`{"client_email":"a@b"}`),
		"bad b64 json":  []byte(base64.StdEncoding.EncodeToString([]byte("nope"))),
		"unparseable k": []byte(`{"client_email":"a@b","private_key":"not pem"}`),
	} {
		_, err := NewFirebaseStore(ctx, "bucket", c)
		require.Error(t, err, name)
	}
}

func TestNewFirebaseStore_AcceptsBase64(t *testing.T) {
	ctx := context.Background()
	raw := fakeCreds(t)
	for name, c := range map[string][]byte{
		"json":   raw,
		"base64": []byte(base64.StdEncoding.EncodeToString(raw)),
	} {
		s, err := NewFirebaseStore(ctx, "my-bucket", c)
		require.NoError(t, err, name)
		var _ Store = s

		tgt, err := s.SignedPut(ctx, "uploads/j1/m1", "image/jpeg", 10485760, 15*time.Minute)
		require.NoError(t, err, name)
		require.Equal(t, "PUT", tgt.Method)
		require.Equal(t, "0,10485760", tgt.Headers["x-goog-content-length-range"])
		require.Equal(t, "image/jpeg", tgt.Headers["Content-Type"])
		require.WithinDuration(t, time.Now().Add(15*time.Minute), tgt.ExpiresAt, 5*time.Second)

		u, err := url.Parse(tgt.URL)
		require.NoError(t, err)
		require.Equal(t, "storage.googleapis.com", u.Host)
		require.NotEmpty(t, u.Query().Get("X-Goog-Signature"))
		require.Contains(t, []string{"899", "900"}, u.Query().Get("X-Goog-Expires"))
		signed := u.Query().Get("X-Goog-SignedHeaders")
		require.Contains(t, signed, "x-goog-content-length-range")
		require.Contains(t, signed, "content-type")

		g, err := s.SignedGet(ctx, "moments/j1/m1/thumb.jpg", time.Hour)
		require.NoError(t, err)
		gu, err := url.Parse(g)
		require.NoError(t, err)
		require.True(t, strings.HasSuffix(gu.Path, "/moments/j1/m1/thumb.jpg"))
		require.Contains(t, []string{"3599", "3600"}, gu.Query().Get("X-Goog-Expires"))
		require.NotEmpty(t, gu.Query().Get("X-Goog-Signature"))
	}
}
