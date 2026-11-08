package testutil

import (
	"context"
	"errors"

	"github.com/kpkipper/journey-service/internal/auth"
)

type SentMail struct{ To, Subject, Body string }

// CaptureMailer records mails instead of sending them; it implements mailer.Mailer.
type CaptureMailer struct{ Sent []SentMail }

func (m *CaptureMailer) Send(_ context.Context, to, subject, body string) error {
	m.Sent = append(m.Sent, SentMail{To: to, Subject: subject, Body: body})
	return nil
}

// FakeGoogle implements auth.GoogleVerifier; Identities is keyed by the id token string.
type FakeGoogle struct {
	Identities map[string]*auth.GoogleIdentity
}

func (f *FakeGoogle) Verify(_ context.Context, idToken string) (*auth.GoogleIdentity, error) {
	if id, ok := f.Identities[idToken]; ok {
		return id, nil
	}
	return nil, errors.New("unknown google token")
}
