package lookup

import (
	"os"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/testenv"
)

// Isolation is the default for this whole test binary, not something each test
// remembers: this package can resolve the operator's own chart library,
// preferences and session, and a test that forgets its sandbox must cost a
// stray temp file rather than their data.
func TestMain(m *testing.M) {
	// Every call carries the binary's object list (objects slice 0b): the
	// package's lens gets the list the astrolog fixtures were captured from.
	if b, err := os.ReadFile("../astrolog/testdata/ho-all.txt"); err == nil {
		if reg, err := astrolog.ParseObjectRegistry(string(b)); err == nil {
			Hellenistic.Registry = reg
		}
	}
	os.Exit(testenv.Run(func() int {
		// A started app holds the host's default object set before it casts
		// anything (the ephemeris probe answered): the compiler has no other
		// baseline, so the tests that compile a chart start from it too.
		testenv.InstallHostDefault()
		return m.Run()
	}))
}
