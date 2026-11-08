package testutil

import "context"

type SentMail struct{ To, Subject, Body string }

// CaptureMailer records mails instead of sending them; it implements mailer.Mailer.
type CaptureMailer struct{ Sent []SentMail }

func (m *CaptureMailer) Send(_ context.Context, to, subject, body string) error {
	m.Sent = append(m.Sent, SentMail{To: to, Subject: subject, Body: body})
	return nil
}
