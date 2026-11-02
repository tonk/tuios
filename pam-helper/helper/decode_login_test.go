package main

import (
	"bytes"
	"testing"

	"github.com/tonk/tuios/pam-helper/internal/wire"
)

func TestDecodeLoginTwoFields(t *testing.T) {
	var buf bytes.Buffer
	wire.PutField(&buf, []byte("alice"))
	wire.PutField(&buf, []byte("secret"))

	user, pass, rhost, err := decodeLogin(buf.Bytes())
	if err != nil {
		t.Fatalf("decodeLogin: %v", err)
	}
	if user != "alice" || pass != "secret" || rhost != "" {
		t.Fatalf("got user=%q pass=%q rhost=%q", user, pass, rhost)
	}
}

func TestDecodeLoginWithRhost(t *testing.T) {
	var buf bytes.Buffer
	wire.PutField(&buf, []byte("alice"))
	wire.PutField(&buf, []byte("secret"))
	wire.PutField(&buf, []byte("203.0.113.9"))

	user, pass, rhost, err := decodeLogin(buf.Bytes())
	if err != nil {
		t.Fatalf("decodeLogin: %v", err)
	}
	if user != "alice" || pass != "secret" || rhost != "203.0.113.9" {
		t.Fatalf("got user=%q pass=%q rhost=%q", user, pass, rhost)
	}
}
