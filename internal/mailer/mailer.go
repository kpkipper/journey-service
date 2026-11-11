package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"

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

// sendTimeout bounds one whole send (connect, TLS, auth, data), so a stuck SMTP server
// can't hold a login request open.
const sendTimeout = 15 * time.Second

type smtpMailer struct {
	cfg     config.SMTP
	timeout time.Duration
}

func NewSMTPMailer(cfg config.SMTP) Mailer { return &smtpMailer{cfg: cfg, timeout: sendTimeout} }

func (m *smtpMailer) Send(ctx context.Context, to, subject, body string) error {
	if strings.ContainsAny(to, "\r\n") || strings.ContainsAny(m.cfg.From, "\r\n") {
		return fmt.Errorf("invalid address")
	}
	msg := "From: " + m.cfg.From + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" + body

	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	if err := m.send(ctx, to, msg); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("smtp: send stopped: %w", ctxErr)
		}
		return err
	}
	return nil
}

func (m *smtpMailer) send(ctx context.Context, to, msg string) error {
	deadline, _ := ctx.Deadline()

	addr := net.JoinHostPort(m.cfg.Host, m.cfg.Port)
	tlsCfg := &tls.Config{ServerName: m.cfg.Host}
	var conn net.Conn
	var err error
	if m.cfg.Port == "465" { // implicit TLS (SMTPS)
		conn, err = (&tls.Dialer{Config: tlsCfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	// Closing the connection unblocks any read/write when the caller's context ends first.
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	c, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Hello("localhost"); err != nil {
		return err
	}
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(tlsCfg); err != nil {
			return err
		}
	}
	// Login links are credentials: never send them unencrypted except to a local relay.
	if _, isTLS := c.TLSConnectionState(); !isTLS && !isLoopback(m.cfg.Host) {
		return fmt.Errorf("smtp: %s offers no TLS; refusing to send in plaintext", m.cfg.Host)
	}
	if m.cfg.User != "" {
		if ok, _ := c.Extension("AUTH"); !ok {
			return fmt.Errorf("smtp: server doesn't support AUTH")
		}
		if err := c.Auth(smtp.PlainAuth("", m.cfg.User, m.cfg.Password, m.cfg.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(m.cfg.From); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// isLoopback reports whether host names this machine.
func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
