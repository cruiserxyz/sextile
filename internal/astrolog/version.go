package astrolog

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"time"
)

// ErrNoVersion means astrolog was found and ran, but its `-Hc` answer carried
// no `Astrolog version N.NN` line (or it did not finish in versionTimeout).
var ErrNoVersion = errors.New("astrolog: no version line in the -Hc answer")

// versionTimeout bounds the `-Hc` call so `sextile --version` cannot hang on a
// wedged binary.
const versionTimeout = 5 * time.Second

// versionLine anchors the number: `8.00.1` is not `8.00`, so a third component
// or a trailing digit refuses the line rather than truncating it.
var versionLine = regexp.MustCompile(`Astrolog version (\d+\.\d{2})(?:[^0-9.]|$)`)

// parseVersion extracts N.NN from astrolog's `-Hc` banner.
func parseVersion(out string) (string, bool) {
	m := versionLine.FindStringSubmatch(out)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// Version reports the version astrolog prints for `-Hc` and the PATH entry it
// resolved to. It returns ErrNotInstalled when astrolog is not on PATH, and
// ErrNoVersion (with path set) when it ran but gave no version line or ran past
// versionTimeout. Like every call here it starts the process only through spawn.
func Version() (version, path string, err error) {
	path, lerr := exec.LookPath(binary)
	if lerr != nil {
		return "", "", ErrNotInstalled
	}
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()
	out, _, serr := spawn(ctx, newExempt("version: -Hc prints the program banner with its version, no chart", "-Hc").bounded())
	if serr != nil {
		// A killed, timed-out or non-zero run is not an answer, whatever it
		// printed before it died.
		return "", path, ErrNoVersion
	}
	if v, ok := parseVersion(out); ok {
		return v, path, nil
	}
	return "", path, ErrNoVersion
}
