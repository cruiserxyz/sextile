package ui

import (
	"context"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/testenv"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Isolation is the default for this whole test binary, not something each test
// remembers: this package can resolve the operator's own chart library,
// preferences and session, and a test that forgets its sandbox must cost a
// stray temp file rather than their data.
func TestMain(m *testing.M) {
	// No test spawns the station probe unless it says so: every compute path
	// reaches Astrolog through a seam, and the probe is one more spawn that a
	// fixture with a slow, direct body (a zero speed is one) would otherwise
	// run for real. A test that wants marks installs its own (station_mark_test.go).
	stationMarks = func(context.Context, *lookup.Probers, []aspect.Position, time.Time, location.PlaceInfo, lookup.Lens) (map[aspect.Body]lookup.Station, []string, error) {
		return nil, nil, nil
	}
	// No test waits for a wall-clock minute: the minute timer returns at once
	// with no message, and a test that wants the tick delivers it by hand with
	// the fake clock moved (live_now_test.go).
	minuteTimer = func(time.Duration, func(time.Time) tea.Msg) tea.Cmd {
		return func() tea.Msg { return nil }
	}
	os.Exit(testenv.Run(func() int {
		code := m.Run()
		writeGoldenDigest()
		return code
	}))
}
