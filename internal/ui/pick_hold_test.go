package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// A chart pick keeps the old chart on screen (the specification).

var pickMoment = time.Date(2001, 3, 14, 11, 30, 0, 0, time.UTC)

// pickModel is a settled comparing scene, nothing open, and the lookup counter
// its stubs feed.
func pickModel(t *testing.T) (Model, *swapSpawns) {
	t.Helper()
	spawns := stubSwapLookups(t)
	m := settledComparing(t, 1)
	m.width, m.height = 132, 40
	return m, spawns
}

// libraryPick walks pick -> resolve for a chart and returns the model the
// moment the resolve lands, before any recompute has run.
func libraryPick(t *testing.T, m Model, c store.NamedChart, when time.Time) (Model, tea.Cmd) {
	t.Helper()
	m.startup = startupChartOpen
	m.picker = chartPickerState{loaded: true, prior: startupReady}
	picked, resolve := applyChartPicked(m, ChartPickedMsg{Chart: c})
	if resolve == nil {
		t.Fatal("fixture: the pick dispatched no resolve")
	}
	return applyChartResolved(picked, ChartResolvedMsg{Chart: c, When: when, Gen: picked.pickGen})
}

// inked drops the panel rows that hold nothing. An added chart takes a row of
// the chart list from the panels below it, so the held frame is the same
// content on one fewer blank row.
func inked(s string) string {
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Trim(l, "│ ") != "" {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

// Rules 1 and 2: Bodies and Aspects keep painting the previous committed frame
// after a library pick (an added chart, a chart already open) and an atlas
// pick, then switch in whole; the chart list already shows the picked chart.
func TestPickHoldsTheOldChart(t *testing.T) {
	munich := location.PlaceInfo{Name: "Munich", Country: "DE", Lat: 48.1374, Lon: 11.5761, TZ: "Europe/Berlin"}
	for _, tc := range []struct {
		name     string
		listName string // what the chart list must show in the pick's own frame; "" = not asserted
		sequence bool   // the pick's command is a tea.Sequence the pump cannot walk: land its frame by hand
		pick     func(*testing.T, Model) (Model, tea.Cmd)
	}{
		{"library add", "Picked Client", false, func(t *testing.T, m Model) (Model, tea.Cmd) {
			return libraryPick(t, m, testNamedChart("Picked Client", fixtureOneP), pickMoment)
		}},
		{"library replace", "", false, func(t *testing.T, m Model) (Model, tea.Cmd) {
			open := m.slots.All()[0]
			c := store.NamedChart{ID: open.Chart.ID, Name: open.Chart.Name, Place: open.Location, PlaceLabel: open.Location.Name, Source: open.Chart.Source}
			return libraryPick(t, m, c, open.Time.Add(48*time.Hour))
		}},
		{"atlas", "", true, func(t *testing.T, m Model) (Model, tea.Cmd) {
			return applyAtlasPicked(m, AtlasPickedMsg{Place: munich})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := pickModel(t)
			list0, before := leftColumn(t, m.View())
			m1, cmd := tc.pick(t, m)
			if cmd == nil {
				t.Fatal("the pick asked for no recompute")
			}
			if !m1.holding() {
				t.Fatal("the pick left nothing held")
			}
			if len(m1.chart.positions) != 0 {
				t.Fatalf("the live chart was not cleared: %d positions", len(m1.chart.positions))
			}
			list1, mid := leftColumn(t, m1.View())
			if inked(mid) != inked(before) {
				t.Fatalf("Bodies and Aspects changed before the picked chart's frame landed:\n--- before\n%s\n--- mid-pick\n%s", before, mid)
			}
			if tc.listName != "" {
				if list1 == list0 || !strings.Contains(list1, tc.listName) {
					t.Fatalf("the chart list does not show %q in the pick's frame:\n%s", tc.listName, list1)
				}
			}
			var m2 Model
			var views []string
			if tc.sequence {
				tm, _ := m1.Update(FirstFrameReadyMsg{Positions: fullChartPositions(), RunID: m1.refresh[surfaceText].runningRunID, WantID: m1.refresh[surfaceText].runningWantID})
				m2 = tm.(Model)
				views = []string{m2.View()}
			} else {
				m2, views = pumpViews(t, m1, cmd)
			}
			if m2.holding() || m2.shown.held {
				t.Fatal("the hold outlived the picked chart's frame")
			}
			_, after := leftColumn(t, m2.View())
			if inked(after) == inked(before) {
				t.Fatalf("the pick never reached Bodies and Aspects:\n%s", after)
			}
			for i, v := range views {
				if _, got := leftColumn(t, v); inked(got) != inked(before) && inked(got) != inked(after) {
					t.Fatalf("view %d of %d shows a mixture of the two charts:\n--- before\n%s\n--- after\n%s\n--- got\n%s", i+1, len(views), before, after, got)
				}
			}
		})
	}
}

// A pick on a session with nothing committed has nothing to hold.
func TestFirstPickHoldsNothing(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m1, _ := libraryPick(t, m, testNamedChart("First Client", fixtureOneP), pickMoment)
	if m1.holding() || m1.shown.held || m1.chart.epoch != 0 {
		t.Fatalf("held=%v epoch=%d: a first pick has nothing to hold", m1.shown.held, m1.chart.epoch)
	}
}

// Rules 3 and 4: a pick that fails swaps in with its error and never outlives
// it; a pick superseded before it lands never swaps in.
func TestFailedOrSupersededPick(t *testing.T) {
	ids := func(m Model) (run, want uint64) {
		return m.refresh[surfaceText].runningRunID, m.refresh[surfaceText].runningWantID
	}
	send := func(t *testing.T, m Model, msg FirstFrameReadyMsg) Model {
		t.Helper()
		tm, _ := m.Update(msg)
		return tm.(Model)
	}
	chartA := testNamedChart("Picked A", fixtureOneP)
	chartB := testNamedChart("Picked B", fixtureOneP)

	t.Run("unreadable file changes nothing", func(t *testing.T) {
		m, _ := pickModel(t)
		_, before := leftColumn(t, m.View())
		picked, _ := applyChartPicked(m, ChartPickedMsg{Chart: chartA})
		got, _ := applyChartResolved(picked, ChartResolvedMsg{Chart: chartA, Err: errors.New("read: no such file"), Gen: picked.pickGen})
		if got.holding() || got.shown.held {
			t.Fatal("a failed resolve left a frame held")
		}
		if _, now := leftColumn(t, got.View()); now != before {
			t.Fatalf("a failed resolve changed Bodies or Aspects:\n%s", now)
		}
		if log := strings.Join(got.logPanelRows(), "\n"); !strings.Contains(log, "Couldn't open that chart") {
			t.Fatalf("no error line in the Log:\n%s", log)
		}
	})

	t.Run("refused cast swaps in with its error", func(t *testing.T) {
		m, _ := pickModel(t)
		_, before := leftColumn(t, m.View())
		m1, _ := libraryPick(t, m, chartA, pickMoment)
		if !m1.holding() {
			t.Fatal("fixture: the pick held nothing")
		}
		run, want := ids(m1)
		m2 := send(t, m1, FirstFrameReadyMsg{Err: errors.New("astrolog: exit status 1"), RunID: run, WantID: want})
		if m2.shown.held || m2.holding() {
			t.Fatal("the failed frame left the old chart held")
		}
		if _, after := leftColumn(t, m2.View()); after == before {
			t.Fatalf("Bodies and Aspects still show the old chart after the failed frame:\n%s", after)
		}
		if log := strings.Join(m2.logPanelRows(), "\n"); !strings.Contains(log, "Couldn't cast the chart") {
			t.Fatalf("no error line in the Log:\n%s", log)
		}
	})

	t.Run("stale resolve never lands", func(t *testing.T) {
		m, _ := pickModel(t)
		_, before := leftColumn(t, m.View())
		pickedA, _ := applyChartPicked(m, ChartPickedMsg{Chart: chartA})
		pickedB, _ := applyChartPicked(pickedA, ChartPickedMsg{Chart: chartB})
		slots := len(pickedB.slots.All())
		got, cmd := applyChartResolved(pickedB, ChartResolvedMsg{Chart: chartA, When: pickMoment, Gen: pickedA.pickGen})
		if cmd != nil || len(got.slots.All()) != slots || got.shown.held {
			t.Fatalf("the superseded pick landed: cmd=%v slots %d->%d held=%v", cmd != nil, slots, len(got.slots.All()), got.shown.held)
		}
		if _, now := leftColumn(t, got.View()); now != before {
			t.Fatalf("the superseded pick changed Bodies or Aspects:\n%s", now)
		}
	})

	t.Run("superseded frame never swaps in", func(t *testing.T) {
		m, _ := pickModel(t)
		mA, _ := libraryPick(t, m, chartA, pickMoment)
		runA, wantA := ids(mA)
		mB, _ := libraryPick(t, mA, chartB, pickMoment.Add(24*time.Hour))
		runB, wantB := ids(mB)
		if runA == runB || wantA == wantB || !mB.holding() {
			t.Fatalf("fixture: A=(%d,%d) B=(%d,%d) holding=%v", runA, wantA, runB, wantB, mB.holding())
		}
		_, held := leftColumn(t, mB.View())
		m2 := send(t, mB, FirstFrameReadyMsg{Positions: fullChartPositions(), RunID: runA, WantID: wantA})
		if !m2.holding() {
			t.Fatal("A's superseded frame released the hold")
		}
		if _, got := leftColumn(t, m2.View()); got != held {
			t.Fatalf("A's superseded frame changed Bodies or Aspects:\n%s", got)
		}
		if m3 := send(t, m2, FirstFrameReadyMsg{Positions: fullChartPositions(), RunID: runB, WantID: wantB}); m3.shown.held {
			t.Fatal("B's frame did not release the hold")
		}
	})
}

// Rule 5: holding the old chart adds no Astrolog call. The same pick on a model
// that holds nothing makes the same number of lookups.
func TestPickAddsNoSpawn(t *testing.T) {
	run := func(hold bool) int {
		m, spawns := pickModel(t)
		if !hold {
			m.shown = shownFrame{}
		}
		before := spawns.total()
		m1, cmd := libraryPick(t, m, testNamedChart("Counted", fixtureOneP), pickMoment)
		if m1.shown.held != hold {
			t.Fatalf("held = %v, want %v", m1.shown.held, hold)
		}
		pump(t, m1, cmd)
		return spawns.total() - before
	}
	held, unheld := run(true), run(false)
	if unheld == 0 {
		t.Fatal("the pick made no lookup at all: the count proves nothing")
	}
	if held != unheld {
		t.Fatalf("holding the old chart made %d lookups, the same pick without it %d", held, unheld)
	}
}

// The frame the spec freezes: mid-pick, colours off.
func TestPickHeldFrame(t *testing.T) {
	m, _ := pickModel(t)
	m.cfg = Config{Color: ColorNever}
	m1, _ := libraryPick(t, m, testNamedChart("Picked Client", fixtureOneP), pickMoment)
	checkGolden(t, "pick-held-132x40-never", m1.View())
}
