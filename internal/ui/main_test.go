package ui

import (
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/testenv"
)

// Isolation is the default for this whole test binary, not something each test
// remembers: this package can resolve the operator's own chart library,
// preferences and session, and a test that forgets its sandbox must cost a
// stray temp file rather than their data.
func TestMain(m *testing.M) {
	// No test waits for a wall-clock minute: the minute timer returns at once
	// with no message, and a test that wants the tick delivers it by hand with
	// the fake clock moved (live_now_test.go).
	minuteTimer = func(time.Duration, func(time.Time) tea.Msg) tea.Cmd {
		return func() tea.Msg { return nil }
	}
	// And none waits for the idle prefetch's rest: no timer is started, and a
	// test of the prefetch delivers the idle message by hand or puts a timer
	// that answers at once in its place (prefetch_test.go).
	idleTimer = func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }
	// Nor for the Details loading label's repaint: it only animates, and the
	// label's own tests put the real timer back (loading_overlay_test.go).
	loadingTimer = func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }
	os.Exit(testenv.Run(func() int {
		// Every call carries the binary's object list (objects slice 0b): the
		// package's lens has it, as a Model's has once the list lands.
		lookup.Hellenistic.Registry = startObjectList()
		lookup.Hellenistic.AspectRegistry = testAspects()
		// A started app holds the host's default object set before it casts
		// anything (the ephemeris probe answered): the compiler has no other
		// baseline, so the tests that compile a chart start from it too.
		testenv.InstallHostDefault()
		code := m.Run()
		writeGoldenDigest()
		return code
	}))
}
