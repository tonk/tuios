package terminal

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// captureResetTerminal runs ResetTerminal with stdout redirected to a pipe and
// returns what it wrote.
//
// The pipe is drained while ResetTerminal runs, not after. ResetTerminal syncs
// stdout, and on Windows that is FlushFileBuffers, which on a pipe blocks until
// the other end has read everything written to it. Reading only afterwards
// leaves both sides waiting on each other until the test binary times out.
func captureResetTerminal(t *testing.T) []byte {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	defer func() { _ = r.Close() }()

	read := make(chan []byte, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		read <- buf.Bytes()
	}()

	oldStdout := os.Stdout
	os.Stdout = w
	ResetTerminal()
	os.Stdout = oldStdout
	_ = w.Close()

	return <-read
}

// TestResetTerminal verifies that ResetTerminal doesn't panic and produces output.
// Since it writes escape sequences to stdout, we capture output to verify behavior.
func TestResetTerminal(t *testing.T) {
	output := captureResetTerminal(t)

	// Verify some escape sequences are present
	if len(output) == 0 {
		t.Error("Expected ResetTerminal to produce output")
	}

	// Check for ESC character (0x1b)
	if !bytes.Contains(output, []byte{0x1b}) {
		t.Error("Expected output to contain escape sequences")
	}

	// Check for reset sequence (ESC c)
	if !bytes.Contains(output, []byte{0x1b, 'c'}) {
		t.Error("Expected output to contain terminal reset sequence (ESC c)")
	}

	// Check for cursor show sequence (ESC [?25h)
	if !bytes.Contains(output, []byte{0x1b, '[', '?', '2', '5', 'h'}) {
		t.Error("Expected output to contain cursor show sequence")
	}

	// Check for attribute reset (ESC [0m)
	if !bytes.Contains(output, []byte{0x1b, '[', '0', 'm'}) {
		t.Error("Expected output to contain attribute reset sequence")
	}

	// Check for line ending
	if !bytes.Contains(output, []byte{'\r', '\n'}) {
		t.Error("Expected output to contain CRLF line ending")
	}
}

// TestResetTerminalSequences verifies specific escape sequences are in correct order.
func TestResetTerminalSequences(t *testing.T) {
	output := captureResetTerminal(t)

	// Expected sequences in order
	sequences := []struct {
		name string
		seq  []byte
	}{
		{"terminal reset", []byte{0x1b, 'c'}},
		{"disable normal tracking", []byte("\033[?1000l")},
		{"disable button event tracking", []byte("\033[?1002l")},
		{"disable all motion tracking", []byte("\033[?1003l")},
		{"disable focus tracking", []byte("\033[?1004l")},
		{"disable SGR extended mouse", []byte("\033[?1006l")},
		{"show cursor", []byte("\033[?25h")},
		{"exit alternate screen", []byte("\033[?47l")},
		{"reset attributes", []byte("\033[0m")},
	}

	lastIndex := 0
	for _, seq := range sequences {
		idx := bytes.Index(output[lastIndex:], seq.seq)
		if idx == -1 {
			t.Errorf("Expected to find %s sequence after position %d", seq.name, lastIndex)
			continue
		}
		lastIndex += idx + len(seq.seq)
	}
}
