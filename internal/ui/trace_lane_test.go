package ui

import (
	"context"
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

func consoleLineTexts(m Model) []string {
	var out []string
	for _, l := range m.consoleLines() {
		out = append(out, l.text)
	}
	return out
}

// Sextile's own troubles reach the console and nothing else: no Log panel
// line, no unseen-error mark, and exactly one new console line.
func TestTraceOnlyInConsole(t *testing.T) {
	cases := []struct {
		name string
		want string
		run  func(t *testing.T) Model
	}{
		{"fixed stars left out", "details: no fixed stars in the influence chart, Astrolog 8.00 miscounts them", func(t *testing.T) Model {
			m := detailsTestModel(t)
			m.focused = PanelChart
			m.lens.Registry = lookup.Hellenistic.Registry
			m.lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 89, 96}}
			msg := runDetailsCmd(t, m)
			m, _ = m.commitRefresh(surfaceDetails, msg)
			return m
		}},
		{"kept the rows", "details: kept the rows where Venus is 2's: 3 of 11 rows, totals dropped", func(t *testing.T) Model {
			fixture, err := os.ReadFile("../lookup/testdata/f3-venus-cross.txt")
			if err != nil {
				t.Fatal(err)
			}
			stubSpawns(t, func(s *rowSpawns) {
				s.reports.cross = func(context.Context, lookup.Lens, store.AnchorChart, time.Time, location.PlaceInfo, aspect.Object) (string, string, error) {
					return string(fixture), "astrolog -fixture", nil
				}
			})
			m := pairVenusModel(t)
			m.cfg.Color = ColorNever
			msg := runDetailsCmd(t, m)
			m, _ = m.commitRefresh(surfaceDetails, msg)
			return m
		}},
		{"body probe failed", "bodies: couldn't check whether object 1 can be drawn", func(t *testing.T) Model {
			m := timelineModel(t, Config{Accessible: true})
			m.startup = startupBodyOpen
			m, _ = applyBodyProbeResult(m, bodyProbeResultMsg{seq: m.bodyModal.Seq, restID: m.bodyModal.RestID, obj: 1, err: errors.New("astrolog exited 1")})
			return m
		}},
		{"ephemeris account", "bodies: the ephemeris account cannot be made: ", func(t *testing.T) Model {
			t.Cleanup(resetEphemeris)
			lookup.ForgetEphemeris()
			m := timelineModel(t, Config{Accessible: true})
			m.ephemeris = lookup.EphemerisAvailability{Folder: "f", Absent: map[int]string{12: "seas_18.se1"}}
			m, _ = m.sayEphemeris()
			return m
		}},
		{"ephemeris account, saved bodies", "bodies: the ephemeris account cannot be made: ", func(t *testing.T) Model {
			t.Cleanup(resetEphemeris)
			lookup.ForgetEphemeris()
			m := timelineModel(t, Config{Accessible: true})
			m.lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}}
			m.ephemeris = lookup.EphemerisAvailability{Folder: "f", Absent: map[int]string{12: "seas_18.se1"}}
			m, _ = m.sayEphemeris()
			return m
		}},
		{"drained selection", "bodies: the drained selection cannot be told: ", func(t *testing.T) Model {
			t.Cleanup(resetEphemeris)
			lookup.ForgetEphemeris()
			m := timelineModel(t, Config{Accessible: true})
			m.lens.Objects.Only = recipe.NewObjectSet(12)
			return m.sayDrainedSelection()
		}},
		{"aspect registry", "aspects: registry unreadable (astrolog exited 1)", func(t *testing.T) Model {
			m := timelineModel(t, Config{Accessible: true})
			m, _ = applyAspectRegistryLoaded(m, AspectRegistryLoadedMsg{Err: errors.New("astrolog exited 1")})
			return m
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// The model before the event is not available from run, so count
			// against an equal fresh one is not possible; assert on the result.
			m := c.run(t)
			hits := 0
			for _, s := range consoleLineTexts(m) {
				if strings.HasPrefix(s, c.want) {
					hits++
				}
			}
			if hits != 1 {
				t.Fatalf("console = %q, want exactly one line starting %q", consoleLineTexts(m), c.want)
			}
			for _, s := range m.logPanelLines() {
				if strings.HasPrefix(s, c.want) {
					t.Errorf("the Log panel holds %q", s)
				}
			}
			for _, e := range m.logPanelEntries() {
				if strings.HasPrefix(e.text, c.want) {
					t.Errorf("the Log panel entries hold %q", e.text)
				}
			}
			if m.errorUnseen {
				t.Error("an unseen-error mark was set")
			}
		})
	}
}

// The cap counts argv and trace lines together, drops the oldest first, and
// never drops a note or an error.
func TestTraceEvictedWithArgv(t *testing.T) {
	m := newTestModel(t, Config{Accessible: true}, FrozenClock(testFrozenTime()))
	m = m.logNote("session", "kept note")
	m = m.logErrorLine("kept error")
	for i := 0; i < MaxLogLines; i++ {
		m = m.appendLog(laneArgv, "argv-"+strconv.Itoa(i))
		m = m.logTrace("t", "trace-%d", i)
	}
	n, notes, errs := 0, 0, 0
	var first string
	for _, l := range m.logLines {
		switch l.lane {
		case laneArgv, laneTrace:
			if n == 0 {
				first = l.text
			}
			n++
		case laneNote:
			notes++
		case laneError:
			errs++
		}
	}
	if n != MaxLogLines {
		t.Errorf("%d argv and trace lines kept, want %d", n, MaxLogLines)
	}
	if notes != 1 || errs != 1 {
		t.Errorf("notes=%d errors=%d, want both kept", notes, errs)
	}
	// 2*Max lines arrived, so the oldest Max are gone and the first kept is the Max-th.
	want := "argv-" + strconv.Itoa(MaxLogLines/2)
	if MaxLogLines%2 == 1 {
		want = "t: trace-" + strconv.Itoa(MaxLogLines/2)
	}
	if first != want {
		t.Errorf("oldest kept = %q, want %q", first, want)
	}
	if !slices.ContainsFunc(m.logLines, func(l logLine) bool { return l.text == "session: kept note" }) {
		t.Error("the note was dropped")
	}
}
