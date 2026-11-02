package main

import (
	"net/http"
	"testing"
)

func TestClientHostPrefersXForwardedFor(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "127.0.0.1:12345",
		Header:     http.Header{"X-Forwarded-For": {"203.0.113.9, 10.0.0.1"}},
	}
	if got := clientHost(r); got != "203.0.113.9" {
		t.Fatalf("clientHost = %q, want 203.0.113.9", got)
	}
}

func TestClientHostFallsBackToRemoteAddr(t *testing.T) {
	r := &http.Request{RemoteAddr: "198.51.100.7:443"}
	if got := clientHost(r); got != "198.51.100.7" {
		t.Fatalf("clientHost = %q, want 198.51.100.7", got)
	}
}
