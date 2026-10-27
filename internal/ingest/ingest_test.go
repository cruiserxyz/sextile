package ingest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// All chart values in this file are constructed synthetic test inputs.

// perimeterSandbox isolates every file the cold-start read touches: the chart
// library on its top-precedence tier (the rule
// invariants.TestTestsIsolateChartsDirOnTopTier holds down), the session under
// XDG_STATE_HOME, and preferences.yaml under XDG_CONFIG_HOME. Without all three
// the test reads the operator's own files.
func perimeterSandbox(t *testing.T) (chartsDir, sessionDir string) {
	t.Helper()
	root := t.TempDir()
	chartsDir = filepath.Join(root, "charts")
	sessionDir = filepath.Join(root, "state", "sextile", "session")
	for _, d := range []string{chartsDir, sessionDir, filepath.Join(root, "config")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SEXTILE_CHARTS_DIR", chartsDir)
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HOME", root)
	return chartsDir, sessionDir
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readPerimeter(t *testing.T) StartupPerimeterReadMsg {
	t.Helper()
	msg := ReadStartupPerimeter()()
	got, ok := msg.(StartupPerimeterReadMsg)
	if !ok {
		t.Fatalf("ReadStartupPerimeter emitted %T, want a perimeter read", msg)
	}
	return got
}

// C5: the session rides along on the perimeter read, parsed and undecided.
func TestReadStartupPerimeterCarriesSession(t *testing.T) {
	t.Run("no session file", func(t *testing.T) {
		perimeterSandbox(t)
		got := readPerimeter(t)
		if got.HasSession || got.SessionErr != nil {
			t.Errorf("HasSession = %v, SessionErr = %v; want an absent session and no error", got.HasSession, got.SessionErr)
		}
	})

	t.Run("a session file is parsed and carried", func(t *testing.T) {
		_, sessionDir := perimeterSandbox(t)
		writeFile(t, filepath.Join(sessionDir, "session.yaml"), `version: 1
chart:
  id: fixture-one
  name: Constructed One
  when: "2001-02-03T04:05:00Z"
  place: Test Bay
  country: DE
  lat: 48.4
  lon: 10
  tz: Europe/Berlin
selection:
  comparison: false
  anchored_bodies: sun,moon
  floating_bodies: mars
`)
		got := readPerimeter(t)
		if !got.HasSession || got.SessionErr != nil {
			t.Fatalf("HasSession = %v, SessionErr = %v", got.HasSession, got.SessionErr)
		}
		if len(got.Session.Slots) != 1 || got.Session.Slots[0].Chart.Name != "Constructed One" {
			t.Fatalf("Session.Slots = %+v", got.Session.Slots)
		}
		if got.Session.Slots[0].Selected || got.Session.FloatingSelected {
			t.Error("comparison came back true; the file said false")
		}
		if len(got.Session.Slots[0].Bodies) != 2 || len(got.Session.FloatingBodies) != 1 {
			t.Errorf("selections = %v / %v", got.Session.Slots[0].Bodies, got.Session.FloatingBodies)
		}
	})

	t.Run("a session file this build must not act on is reported, not guessed at", func(t *testing.T) {
		_, sessionDir := perimeterSandbox(t)
		writeFile(t, filepath.Join(sessionDir, "session.yaml"), "version: 99\nselection:\n")
		got := readPerimeter(t)
		if got.HasSession {
			t.Error("a session from a later generation was accepted")
		}
		if got.SessionErr == nil {
			t.Error("a refused session file came back with no reason")
		}
	})
}

// C5, guarded from below: the perimeter's own body vocabulary is the enum's,
// so a session written by the UI reads back as the same bodies.
func TestReadStartupPerimeterBodyNamesAreTheEnums(t *testing.T) {
	_, sessionDir := perimeterSandbox(t)
	writeFile(t, filepath.Join(sessionDir, "session.yaml"),
		"version: 1\nselection:\n  floating_bodies: "+aspect.ImumCoeli.String()+"\n")
	got := readPerimeter(t)
	if len(got.Session.FloatingBodies) != 1 || got.Session.FloatingBodies[0] != aspect.ImumCoeli {
		t.Errorf("FloatingBodies = %v, want [%v]", got.Session.FloatingBodies, aspect.ImumCoeli)
	}
}

// the specification C9: the display: block rides along on the
// perimeter read, as read.
func TestReadStartupPerimeterCarriesDisplay(t *testing.T) {
	perimeterSandbox(t)
	if got := readPerimeter(t); got.Display.FocusColour != "" || got.Display.CursorRowColour != "" || got.Display.RetiredStripePresent {
		t.Errorf("no preferences file, yet Display = %+v", got.Display)
	}
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "sextile")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "preferences.yaml"), "display:\n  focus_color: 2\n")
	if got := readPerimeter(t); got.Display.FocusColour != "2" {
		t.Errorf("Display = %+v, want focus_color 2", got.Display)
	}
}
