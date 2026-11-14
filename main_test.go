package main

import "testing"

func TestAllowLogMailer(t *testing.T) {
	cases := map[string]bool{
		"": false, "development": true, "Development": true, "test": true, " TEST ": true,
		"production": false, "Production": false, "prod": false, "staging": false,
	}
	for env, want := range cases {
		if got := allowLogMailer(env); got != want {
			t.Errorf("allowLogMailer(%q) = %v, want %v", env, got, want)
		}
	}
}

func TestAllowLocalStorage(t *testing.T) {
	cases := map[string]bool{
		"development": true, "Development": true, "test": true, " TEST ": true,
		"production": false, "staging": false, "": false,
	}
	for env, want := range cases {
		if got := allowLocalStorage(env); got != want {
			t.Errorf("allowLocalStorage(%q) = %v, want %v", env, got, want)
		}
	}
}

func TestValidWebURL(t *testing.T) {
	cases := map[string]bool{
		"http://localhost:3000": true, "https://journey.example.com": true,
		"": false, "/path": false, "web.test": false, "ftp://x": false, "https://": false,
	}
	for in, want := range cases {
		if got := validWebURL(in); got != want {
			t.Errorf("validWebURL(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestEmailLoginMode(t *testing.T) {
	cases := []struct {
		name                string
		env, smtpHost       string
		disabled, hasGoogle bool
		want                string
		wantErr             bool
	}{
		{"smtp set", "production", "smtp.example.com", false, false, "smtp", false},
		{"smtp set but turned off", "production", "smtp.example.com", true, true, "off", false},
		{"dev without smtp prints links", "development", "", false, false, "log", false},
		{"dev turned off", "development", "", true, false, "off", false},
		{"prod without smtp, google only", "production", "", false, true, "off", false},
		{"prod with no way to log in", "production", "", false, false, "", true},
		{"blank smtp host counts as unset", "production", "  ", false, false, "", true},
		{"turned off with no google in prod", "production", "smtp.example.com", true, false, "", true},
	}
	for _, c := range cases {
		got, err := emailLoginMode(c.env, c.smtpHost, c.disabled, c.hasGoogle)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("%s: emailLoginMode = %q, %v; want %q, err=%v", c.name, got, err, c.want, c.wantErr)
		}
	}
}
