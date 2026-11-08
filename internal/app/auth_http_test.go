package app_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/app"
	"github.com/kpkipper/journey-service/internal/auth"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var tokenRe = regexp.MustCompile(`token=([0-9a-f]{64})`)

type env struct {
	app  *fiber.App
	mail *testutil.CaptureMailer
	db   *gorm.DB
	now  *time.Time // the app clock; tests may advance it
}

func newEnv(t *testing.T) *env {
	t.Helper()
	mail := &testutil.CaptureMailer{}
	now := new(time.Time)
	*now = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	db := testutil.NewTestDB(t)
	a := app.New(app.Deps{
		DB:     db,
		Google: &testutil.FakeGoogle{Identities: map[string]*auth.GoogleIdentity{}},
		Mailer: mail,
		WebURL: "http://web.test/",
		Now:    func() time.Time { return *now },
	})
	return &env{app: a, mail: mail, db: db, now: now}
}

func (e *env) do(t *testing.T, method, path, token, body string) (int, map[string]any, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.app.Test(req)
	require.NoError(t, err)
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return resp.StatusCode, m, string(raw)
}

func (e *env) login(t *testing.T, email string) string {
	t.Helper()
	status, _, _ := e.do(t, "POST", "/api/v1/auth/email/start", "", `{"email":"`+email+`"}`)
	require.Equal(t, 200, status)
	m := tokenRe.FindStringSubmatch(e.mail.Sent[len(e.mail.Sent)-1].Body)
	require.Len(t, m, 2)
	status, body, raw := e.do(t, "POST", "/api/v1/auth/email/verify", "", `{"token":"`+m[1]+`"}`)
	require.Equal(t, 200, status, raw)
	return body["data"].(map[string]any)["token"].(string)
}

func TestHTTP_EmailLoginFlow(t *testing.T) {
	e := newEnv(t)
	tok := e.login(t, "kip@gmail.com")
	assert.Contains(t, e.mail.Sent[0].Body, "http://web.test/")
	assert.NotContains(t, e.mail.Sent[0].Body, "web.test//")
	status, body, _ := e.do(t, "GET", "/api/v1/me", tok, "")
	require.Equal(t, 200, status)
	assert.Equal(t, "kip@gmail.com", body["data"].(map[string]any)["email"])
}

func TestHTTP_MeWithoutToken(t *testing.T) {
	e := newEnv(t)
	status, body, _ := e.do(t, "GET", "/api/v1/me", "", "")
	assert.Equal(t, 401, status)
	assert.Equal(t, "unauthorized", body["message"])

	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Basic abc")
	resp, err := e.app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestHTTP_AuthBoundary(t *testing.T) {
	e := newEnv(t)
	tok := e.login(t, "a@gmail.com")

	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "bearer "+tok)
	resp, err := e.app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	unknown := strings.Repeat("ab", 32)
	status, body, _ := e.do(t, "GET", "/api/v1/me", unknown, "")
	assert.Equal(t, 401, status)
	assert.Equal(t, "unauthorized", body["message"])

	status, _, _ = e.do(t, "POST", "/api/v1/auth/logout", "", "")
	assert.Equal(t, 401, status)
	status, _, _ = e.do(t, "PATCH", "/api/v1/me", "", `{"username":"kipper"}`)
	assert.Equal(t, 401, status)
}

func TestHTTP_PatchMe(t *testing.T) {
	e := newEnv(t)
	a := e.login(t, "a@gmail.com")
	b := e.login(t, "b@gmail.com")
	status, _, raw := e.do(t, "PATCH", "/api/v1/me", a, `{"username":"kipper"}`)
	assert.Equal(t, 200, status, raw)
	status, _, _ = e.do(t, "PATCH", "/api/v1/me", b, `{"username":"kipper"}`)
	assert.Equal(t, 409, status)
	status, _, _ = e.do(t, "PATCH", "/api/v1/me", b, `{"username":"x"}`)
	assert.Equal(t, 400, status)
}

func TestHTTP_PublicProfileHidesEmail(t *testing.T) {
	e := newEnv(t)
	a := e.login(t, "a@gmail.com")
	status, _, _ := e.do(t, "PATCH", "/api/v1/me", a, `{"username":"kipper"}`)
	require.Equal(t, 200, status)
	status, _, raw := e.do(t, "GET", "/api/v1/users/kipper", "", "")
	assert.Equal(t, 200, status)
	assert.NotContains(t, raw, `"email"`)
	status, _, _ = e.do(t, "GET", "/api/v1/users/nobody", "", "")
	assert.Equal(t, 404, status)
}

func TestHTTP_Logout(t *testing.T) {
	e := newEnv(t)
	tok := e.login(t, "a@gmail.com")
	status, _, _ := e.do(t, "POST", "/api/v1/auth/logout", tok, "")
	assert.Equal(t, 200, status)
	status, _, _ = e.do(t, "GET", "/api/v1/me", tok, "")
	assert.Equal(t, 401, status)
}

func TestHTTP_RateLimit(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 5; i++ {
		status, _, _ := e.do(t, "POST", "/api/v1/auth/email/start", "", `{"email":"a@gmail.com"}`)
		require.Equal(t, 200, status)
	}
	status, _, _ := e.do(t, "POST", "/api/v1/auth/email/start", "", `{"email":"a@gmail.com"}`)
	assert.Equal(t, 429, status)
}

func TestHTTP_CORSPreflight(t *testing.T) {
	e := newEnv(t)
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/me", nil)
	req.Header.Set("Origin", "http://web.test")
	req.Header.Set("Access-Control-Request-Method", "PATCH")
	req.Header.Set("Access-Control-Request-Headers", "authorization")
	resp, err := e.app.Test(req)
	require.NoError(t, err)
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Headers"), "Authorization")
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Methods"), "PATCH")
}
