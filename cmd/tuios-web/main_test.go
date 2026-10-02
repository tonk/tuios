package main

import (
	"os"
	"testing"

	"github.com/tonk/tuios/internal/testutil"
)

// TestMain isolates the whole test binary from the developer's own XDG
// directories. See testutil.RunIsolated for why this cannot be a per-test
// helper.
//
// The fonts the tests spill to the temp directory go with it.
func TestMain(m *testing.M) {
	code := testutil.RunIsolated(m)
	removeSpilledFonts()
	os.Exit(code)
}
