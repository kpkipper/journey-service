package mailer

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func listen(t *testing.T, serve func(net.Conn)) (host, port string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serve(c)
		}
	}()
	h, p, _ := net.SplitHostPort(ln.Addr().String())
	return h, p
}

func TestSMTPMailer_TimesOutOnSilentServer(t *testing.T) {
	host, port := listen(t, func(c net.Conn) {
		time.Sleep(5 * time.Second) // accepts, never greets
		c.Close()
	})
	m := &smtpMailer{cfg: config.SMTP{Host: host, Port: port, From: "noreply@test"}, timeout: 200 * time.Millisecond}
	start := time.Now()
	err := m.Send(context.Background(), "a@gmail.com", "hi", "body")
	assert.Error(t, err)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestSMTPMailer_StopsWhenContextEnds(t *testing.T) {
	host, port := listen(t, func(c net.Conn) {
		time.Sleep(5 * time.Second)
		c.Close()
	})
	m := &smtpMailer{cfg: config.SMTP{Host: host, Port: port, From: "noreply@test"}, timeout: time.Minute}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	assert.Error(t, m.Send(ctx, "a@gmail.com", "hi", "body"))
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestSMTPMailer_SendsToWorkingServer(t *testing.T) {
	got := make(chan string, 1)
	host, port := listen(t, func(c net.Conn) {
		defer c.Close()
		r := bufio.NewReader(c)
		say := func(s string) { c.Write([]byte(s + "\r\n")) }
		say("220 fake ESMTP")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					got <- data.String()
					say("250 queued")
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"):
				say("250 fake")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				say("250 ok")
			case cmd == "DATA":
				inData = true
				say("354 go")
			case cmd == "QUIT":
				say("221 bye")
				return
			default:
				say("502 no")
			}
		}
	})
	m := &smtpMailer{cfg: config.SMTP{Host: host, Port: port, From: "noreply@test"}, timeout: 5 * time.Second}
	require.NoError(t, m.Send(context.Background(), "a@gmail.com", "เข้าสู่ระบบ", "hello"))
	select {
	case msg := <-got:
		assert.Contains(t, msg, "To: a@gmail.com")
		assert.Contains(t, msg, "hello")
	case <-time.After(2 * time.Second):
		t.Fatal("server got no message")
	}
}

func TestSMTPMailer_RejectsHeaderInjection(t *testing.T) {
	m := &smtpMailer{cfg: config.SMTP{Host: "127.0.0.1", Port: "1", From: "noreply@test"}, timeout: time.Second}
	assert.Error(t, m.Send(context.Background(), "a@gmail.com\r\nBcc: x@y", "hi", "body"))
}

func TestSMTPMailer_TimeoutReportsContextError(t *testing.T) {
	host, port := listen(t, func(c net.Conn) {
		time.Sleep(5 * time.Second)
		c.Close()
	})
	m := &smtpMailer{cfg: config.SMTP{Host: host, Port: port, From: "noreply@test"}, timeout: 200 * time.Millisecond}
	err := m.Send(context.Background(), "a@gmail.com", "hi", "body")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestIsLoopback(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost": true, "127.0.0.1": true, "::1": true,
		"smtp.resend.com": false, "10.0.0.5": false, "": false,
	} {
		assert.Equal(t, want, isLoopback(host), host)
	}
}
