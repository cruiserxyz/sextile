package ui

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// ephemeris_probe_gate_test.go: slice 8c. Which objects a chart draws is the
// host's own default set, read by the startup check of the ephemeris files, so
// a check that fails or does not answer in time is a can't-start in its own
// words, never a start on a guess (it used to log a line and carry on with a
// typed list of thirteen).

// startedAfterProbe drives a cold start the way Init orders it: the check's
// answer first, then the atlas, the perimeter and the object list.
func startedAfterProbe(t *testing.T, probe EphemerisProbedMsg) Model {
	t.Helper()
	sessionSandbox(t)
	var tm tea.Model = withoutObjectList(newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen)))
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	tm, _ = tm.Update(probe)
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(perimeter())
	tm, _ = tm.Update(ObjectRegistryLoadedMsg{Registry: drawnRegistry()})
	return tm.(Model)
}

// requireProbeCantStart asserts the can't-start screen names want, nothing was
// cast and a quit writes no session.
func requireProbeCantStart(t *testing.T, m Model, want string) {
	t.Helper()
	if got := csText(m.View()); !strings.Contains(got, want) {
		t.Errorf("the screen lacks %q:\n%s", want, got)
	}
	if strings.Contains(csText(m.View()), "every body came back at 0° Aries") {
		t.Errorf("the screen gives the 0-Aries reason for a check that failed:\n%s", csText(m.View()))
	}
	if m.slots != nil {
		t.Error("a workspace was built without the check of the ephemeris files")
	}
	if got := argvTexts(m); len(got) != 0 {
		t.Errorf("Astrolog was called without the check of the ephemeris files:\n%s", strings.Join(got, "\n"))
	}
	for _, line := range logTexts(m) {
		if strings.Contains(line, "ephemeris check failed") {
			t.Errorf("the failure was left as a Log line the start went on past: %q", line)
		}
	}
	m.quit()
	if _, err := os.Stat(store.SessionPath()); err == nil {
		t.Error("a quit on the can't-start screen wrote a session")
	}
}

// A check that fails stops the start on the can't-start screen, in the error's
// words; nothing is cast and a quit writes no session.
func TestFailedEphemerisProbeCannotStart(t *testing.T) {
	m := startedAfterProbe(t, EphemerisProbedMsg{Err: errors.New("astrolog: availability: main set: exit status 2")})
	requireProbeCantStart(t, m,
		"Sextile can't start: Astrolog's files for the planets could not be checked: astrolog: availability: main set: exit status 2")
}

// A check that does not answer within its deadline is the same screen. The
// stub ignores its context altogether, so the deadline is the start's own, not
// the probe's good manners.
func TestLateEphemerisProbeCannotStart(t *testing.T) {
	oldProbe, oldDeadline := ephemerisProbe, ephemerisProbeDeadline
	release := make(chan struct{})
	t.Cleanup(func() {
		close(release)
		ephemerisProbe, ephemerisProbeDeadline = oldProbe, oldDeadline
	})
	ephemerisProbe = func(context.Context) (lookup.EphemerisAvailability, error) {
		<-release
		return lookup.EphemerisAvailability{Default: []int{1}}, nil
	}
	ephemerisProbeDeadline = 40 * time.Millisecond

	start := time.Now()
	msg, ok := loadEphemerisCmd()().(EphemerisProbedMsg)
	if !ok {
		t.Fatal("the check did not answer with an EphemerisProbedMsg")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("the check was waited for %v past a 40ms deadline", took)
	}
	if msg.Err == nil {
		t.Fatalf("a check that never answered came back without an error: %+v", msg)
	}
	m := startedAfterProbe(t, msg)
	requireProbeCantStart(t, m,
		"Sextile can't start: Astrolog's files for the planets could not be checked: no answer within 40ms")
}

// The deadline is the measured one: the larger of 30 s and ten times the slower
// measurement (2.45 s).
func TestEphemerisProbeDeadlineIsThirtySeconds(t *testing.T) {
	if ephemerisProbeLimit != 30*time.Second || ephemerisProbeDeadline != ephemerisProbeLimit {
		t.Errorf("the deadline is %v (limit %v), want 30s", ephemerisProbeDeadline, ephemerisProbeLimit)
	}
}

// A check that answers is a start, as before; and when the binary itself is
// missing the screen keeps its one reason, not the check's as well.
func TestEphemerisProbeReasonYieldsToAMissingBinary(t *testing.T) {
	s := cantStartState{binary: true, ephemerisProbe: "exec: \"astrolog\": executable file not found in $PATH"}
	got := s.reasons()
	if len(got) != 1 || got[0] != cantStartBinary {
		t.Errorf("reasons = %q, want only the missing binary", got)
	}
	s.binary = false
	if got := s.reasons(); len(got) != 1 || !strings.HasPrefix(got[0], "Astrolog's files for the planets could not be checked: ") {
		t.Errorf("reasons = %q, want the check's own words", got)
	}
}
