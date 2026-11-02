package main

import (
	"os"
	"strings"
	"testing"
)

func TestPtsLine(t *testing.T) {
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("/dev/ptmx: %v", err)
	}
	defer ptmx.Close()

	line, err := ptsLine(ptmx)
	if err != nil {
		t.Fatalf("ptsLine: %v", err)
	}
	if !strings.HasPrefix(line, "pts/") {
		t.Fatalf("ptsLine = %q, want pts/N", line)
	}
	if line == "pts/" {
		t.Fatalf("ptsLine missing number: %q", line)
	}
}
