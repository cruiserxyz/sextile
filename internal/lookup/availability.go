package lookup

import (
	"errors"

	"github.com/cruiserxyz/sextile/internal/astrolog"
)

// availability.go carries one fact across the boundary the perimeter is allowed
// to cross: whether Astrolog is installed at all.
//
// internal/ui may import this package and may not import internal/astrolog
// (invariants.TestPerimeterDoesNotReachActive), so the cause and the probe are
// re-exported here rather than reached for directly. Nothing is interpreted on
// the way through: the perimeter still decides what to say about it.

// ErrAstrologNotInstalled is the cause a spawn returns when the astrolog binary
// is not on PATH. Match it with errors.Is to tell "Astrolog is missing" apart
// from "Astrolog ran and refused", which are different problems with different
// fixes.
var ErrAstrologNotInstalled = astrolog.ErrNotInstalled

// ErrEphemerisMissing is the cause a compute returns when Astrolog ran but
// every body came back at 0 degrees Aries: its ephemeris files are not being
// read. Match it with errors.Is; the perimeter names the fix.
var ErrEphemerisMissing = astrolog.ErrEphemerisDisabled

// ProbeAstrolog reports whether Astrolog can be found, without running it.
// It returns ErrAstrologNotInstalled when it cannot.
//
// Sextile casts every chart with Astrolog, so a machine without it produces a
// failure on the very first frame and one on every keystroke after. Asking once
// at startup turns that into a single line that names the missing program.
func ProbeAstrolog() error { return astrolog.Available() }

// AstrologVersion reports the version Astrolog gives for `-Hc` and the PATH
// entry it was found at, for `sextile --version`. It returns
// ErrAstrologNotInstalled when Astrolog is absent, and astrolog.ErrNoVersion
// (with the path) when it answers without a version.
func AstrologVersion() (version, path string, err error) { return astrolog.Version() }

// ErrAstrologWarned is the cause a chart call returns when Astrolog wrote a
// warning to stderr (the specification C7): the chart is refused
// rather than shown with the warning hidden behind it. Match it with
// errors.Is; AstrologWarnings has the lines.
var ErrAstrologWarned = astrolog.ErrAstrologWarned

// AstrologWarnings are the warning lines of a refused chart call, in the order
// Astrolog wrote them, or nil when err is not such a refusal. Wheel progress
// lines are not warnings and are not in it.
func AstrologWarnings(err error) []string {
	var w *astrolog.WarnedError
	if errors.As(err, &w) {
		return w.Warnings()
	}
	return nil
}
