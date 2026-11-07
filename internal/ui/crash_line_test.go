package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// A call Astrolog dies on says so, in the two places that run it for the
// operator: the chart cast and the Details report. The stand-in ends itself on
// a signal, so the refusal travels the real spawn, lookup and commit seams.
func TestCrashLine(t *testing.T) {
	const kill = "kill -SEGV $$"
	crashed := func(t *testing.T, m Model, subject string) string {
		t.Helper()
		var line string
		for _, l := range errorLines(m) {
			if strings.HasPrefix(l, subject) {
				line = l
			}
		}
		want := subject + " Astrolog crashed."
		if line != want {
			t.Fatalf("Log error line = %q, want %q (all error lines: %q)", line, want, errorLines(m))
		}
		// The line fits the Log: a real frame holds the WHOLE sentence.
		if frame := errLaneModel(t).testError("%s", line).View(); !strings.Contains(frame, line) {
			t.Errorf("the Log truncated the line; it renders as:\n%s", logRegion(frame))
		}
		return line
	}
	t.Run("cast", func(t *testing.T) {
		astrologWarning(t, "", kill)
		m := textModel(t)
		m, cmd := m.advanceTime(unitDay, +1)
		m = pump(t, m, cmd)
		crashed(t, m, "Couldn't cast the chart.")
	})
	t.Run("details", func(t *testing.T) {
		astrologWarning(t, "", kill)
		m := detailsTestModel(t)
		msg := runDetailsCmd(t, m)
		m, _ = m.commitRefresh(surfaceDetails, msg)
		crashed(t, m, "Couldn't read the full report.")
	})
}

// Every site that calls failureReason gets the case by construction, and none
// adds a pointer: the cast, the Details report, the template tab and the
// timeline (console pointer sites) and the wheel render, wheel publish, chart
// file, save and title sites all read "Astrolog crashed.".
func TestCrashClauseAtEverySite(t *testing.T) {
	m := errLaneModel(t)
	err := fmt.Errorf("astrolog: ComputeRawReport: %w", &lookupCrash{})
	if !errors.Is(err, lookup.ErrAstrologCrashed) {
		t.Fatal("the stand-in error does not match lookup.ErrAstrologCrashed")
	}
	for name, fallback := range map[string]string{
		"pick":          pickFallback,
		"wheel render":  wheelRenderFallback,
		"wheel publish": wheelPublishFallback,
		"save":          "some file error",
	} {
		if got := m.failureReason(err, fallback); got != "Astrolog crashed." {
			t.Errorf("%s: %q", name, got)
		}
	}
	// A site that would point at the console still names the cause alone.
	if got := m.failureReason(err, m.consolePointer(true)); got != "Astrolog crashed." {
		t.Errorf("a site with a console pointer: %q", got)
	}
}

type lookupCrash struct{}

func (*lookupCrash) Error() string        { return "astrolog crashed: segmentation fault" }
func (*lookupCrash) Is(target error) bool { return target == lookup.ErrAstrologCrashed }
