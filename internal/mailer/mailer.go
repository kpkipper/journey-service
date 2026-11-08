package mailer

import (
	"context"
	"fmt"
	"mime"
	"net/smtp"
	"strings"

	"github.com/kpkipper/journey-service/config"
	"github.com/kpkipper/journey-service/pkg/logger"
)

type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

type logMailer struct{}

// NewLogMailer prints mails to the log instead of sending them (dev only).
func NewLogMailer() Mailer { return logMailer{} }

func (logMailer) Send(_ context.Context, to, subject, body string) error {
	logger.Get().Info().Str("to", to).Str("subject", subject).Str("body", body).Msg("mail (not sent)")
	return nil
}

type smtpMailer struct{ cfg config.SMTP }

func NewSMTPMailer(cfg config.SMTP) Mailer { return &smtpMailer{cfg: cfg} }

func (m *smtpMailer) Send(_ context.Context, to, subject, body string) error {
	if strings.ContainsAny(to, "\r\n") || strings.ContainsAny(m.cfg.From, "\r\n") {
		return fmt.Errorf("invalid address")
	}
	msg := "From: " + m.cfg.From + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" + body
	auth := smtp.PlainAuth("", m.cfg.User, m.cfg.Password, m.cfg.Host)
	return smtp.SendMail(m.cfg.Host+":"+m.cfg.Port, auth, m.cfg.From, []string{to}, []byte(msg))
}
