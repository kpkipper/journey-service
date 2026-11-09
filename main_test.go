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
