package main

import "testing"

func TestAllowLogMailer(t *testing.T) {
	cases := map[string]bool{
		"": true, "development": true, "Development": true, "test": true, " TEST ": true,
		"production": false, "Production": false, "prod": false, "staging": false,
	}
	for env, want := range cases {
		if got := allowLogMailer(env); got != want {
			t.Errorf("allowLogMailer(%q) = %v, want %v", env, got, want)
		}
	}
}
