package main

import "testing"

func TestIsTrustedRemoteOrigin(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		trusted bool
	}{
		{name: "origin", value: "https://web.whatsapp.com", trusted: true},
		{name: "trailing slash", value: "https://web.whatsapp.com/", trusted: true},
		{name: "session route", value: "https://web.whatsapp.com/app", trusted: true},
		{name: "default https port", value: "https://web.whatsapp.com:443/", trusted: true},
		{name: "wrong scheme", value: "http://web.whatsapp.com", trusted: false},
		{name: "lookalike host", value: "https://web.whatsapp.com.evil.example", trusted: false},
		{name: "wrong port", value: "https://web.whatsapp.com:8443", trusted: false},
		{name: "userinfo", value: "https://web.whatsapp.com@evil.example", trusted: false},
		{name: "empty", value: "", trusted: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isTrustedRemoteOrigin(test.value); got != test.trusted {
				t.Fatalf("isTrustedRemoteOrigin(%q) = %v, want %v", test.value, got, test.trusted)
			}
		})
	}
}
