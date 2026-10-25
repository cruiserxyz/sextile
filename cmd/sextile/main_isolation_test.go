package main

import (
	"os"
	"testing"

	"github.com/cruiserxyz/sextile/internal/testenv"
)

// Isolation is the default for this whole test binary, not something each test
// remembers: main() starts the UI, so this package reaches the operator's own
// chart library, preferences and session through internal/ui, and a test that
// forgets its sandbox must cost a stray temp file rather than their data
// .
func TestMain(m *testing.M) {
	// The version tests build the real binary; capture go's own cache roots
	// before the sandbox moves HOME, or every build would start cold.
	captureGoEnv()
	code := testenv.Run(m.Run)
	cleanBuilt()
	os.Exit(code)
}
