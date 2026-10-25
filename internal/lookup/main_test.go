package lookup

import (
	"os"
	"testing"

	"github.com/cruiserxyz/sextile/internal/testenv"
)

// Isolation is the default for this whole test binary, not something each test
// remembers: this package can resolve the operator's own chart library,
// preferences and session, and a test that forgets its sandbox must cost a
// stray temp file rather than their data.
func TestMain(m *testing.M) { os.Exit(testenv.Run(m.Run)) }
