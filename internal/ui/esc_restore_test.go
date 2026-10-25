package ui

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification C7, C7a and C8: Esc closes whatever is open
// and the screen goes back to what it was when it opened.
//
// This file uses only what the tree had at the fork point (ced53ad2), so it
// can be dropped onto that tree for C8's red run.

// escBase is a ready two-slot workspace at 132x40 with a computed frame, both
// registries read, Bodies focused and its cursor off the first row, so every
// field Esc must restore has a value worth restoring.
func escBase(t *testing.T) Model {
	t.Helper()
	m := tabsPairModel(t, Config{Accessible: true})
	m.width, m.height = 132, 40
	m.limitedViewShown = true // the first-frame notice is not what these tests ask about
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{
		Positions:    fullChartPositions(),
		Aspects:      tasteOwnAspects(),
		CrossAspects: tasteCrossAspects(),
		Overlay:      true,
	})
	m.startup = startupReady
	m.aspectRegistry = testRegistry()
	m.objectRegistry = testObjectRegistry()
	m.focused = PanelBodies
	m.bodyCursorIdx = 3
	return m
}

// escSnapshot is what C7 names: focus, the slot cursor, the Bodies cursor,
// selections, the slots and the anchor, the startup phase, the Details tab and
// the chart frame. The Log is not part of it.
type escSnapshot struct {
	Focused    PanelID
	Cursor     int
	BodyCursor int
	Startup    startupState
	DetailsTab string
	Chart      chartViewState
	Anchor     store.SessionChart
	Slots      string
}

func snapshotForEsc(m Model) escSnapshot {
	slots := ""
	if m.slots != nil {
		for i, s := range m.slots.All() {
			// The slot's chart record is the Anchor field's business (it is
			// what a rename changes); the slots string is moment, place and marks.
			c := *s
			c.Chart = store.SessionChart{}
			slots += fmt.Sprintf("%d %+v %v|", i, c, m.slots.SelectedBodies(i))
		}
		slots += fmt.Sprintf("cmp=%v", m.slots.ComparisonSelected())
	}
	return escSnapshot{
		Focused:    m.focused,
		Cursor:     m.cursorIdx,
		BodyCursor: m.bodyCursorIdx,
		Startup:    m.startup,
		DetailsTab: fmt.Sprint(m.activeTabID(PanelDetails)),
		Chart:      m.chart,
		Anchor:     anchorRecord(m),
		Slots:      slots,
	}
}

func escKey(k tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: k} }
func escRunes(s string) tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// escSend delivers each message through the whole Update path.
func escSend(t *testing.T, m Model, msgs ...tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	var tm tea.Model = m
	var cmd tea.Cmd
	for _, msg := range msgs {
		tm, cmd = tm.Update(msg)
	}
	return tm.(Model), cmd
}

func requireSameScreen(t *testing.T, when string, want, got escSnapshot) {
	t.Helper()
	if reflect.DeepEqual(want, got) {
		return
	}
	wv, gv := reflect.ValueOf(want), reflect.ValueOf(got)
	for i := 0; i < wv.NumField(); i++ {
		if !reflect.DeepEqual(wv.Field(i).Interface(), gv.Field(i).Interface()) {
			t.Errorf("%s: %s is %v, want %v", when, wv.Type().Field(i).Name, gv.Field(i).Interface(), wv.Field(i).Interface())
		}
	}
}

// escPickable is a library row that opens without asking for a title.
func escPickable() store.NamedChart {
	return store.NamedChart{
		ID: "esc-fixture", Name: "Esc Fixture", Year: 1990, Month: 6, Day: 15,
		PlaceLabel: "Berlin",
		Place:      location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"},
		Source:     store.ChartSource{Kind: store.ChartKindFile, File: "esc-fixture.as"},
	}
}

func TestEscRestoresTheScreen(t *testing.T) {
	oldChooserRule(t)
	cases := []struct {
		name string
		// open reaches the owner through keys; use works it; late, if set,
		// delivers what the work was still owed after Esc.
		open, use func(t *testing.T, m Model) Model
		late      func(t *testing.T, m Model) tea.Msg
	}{
		{
			// From a running app: Esc must not drop it back into its startup
			// phase (C8).
			name: "city",
			open: func(t *testing.T, m Model) Model { m, _ = escSend(t, m, escKey(tea.KeyCtrlL)); return m },
			use: func(t *testing.T, m Model) Model {
				m, _ = escSend(t, m, escRunes("b"), escRunes("e"), escKey(tea.KeyDown))
				return m
			},
		},
		{
			name: "open chart",
			open: func(t *testing.T, m Model) Model {
				m, _ = escSend(t, m, escKey(tea.KeyCtrlO), ChartsLoadedMsg{Charts: []store.NamedChart{escPickable()}, Gen: m.pickerGen + 1})
				return m
			},
			use: func(t *testing.T, m Model) Model {
				m, _ = escSend(t, m, escRunes("e"), escKey(tea.KeyDown), escKey(tea.KeyUp))
				return m
			},
		},
		{
			// Mid-work: the pick is resolving when Esc arrives; its late
			// result must change nothing.
			name: "open chart while resolving",
			open: func(t *testing.T, m Model) Model {
				m, _ = escSend(t, m, escKey(tea.KeyCtrlO), ChartsLoadedMsg{Charts: []store.NamedChart{escPickable()}, Gen: m.pickerGen + 1})
				return m
			},
			use: func(t *testing.T, m Model) Model {
				m, _ = escSend(t, m, escKey(tea.KeyEnter))
				if !m.picker.resolving {
					t.Fatal("Enter did not start resolving the pick")
				}
				return m
			},
			late: func(t *testing.T, m Model) tea.Msg {
				return ChartResolvedMsg{Chart: escPickable(), When: time.Date(1990, 6, 15, 12, 0, 0, 0, time.UTC), Gen: m.pickGen - 1}
			},
		},
		{
			name: "save chart",
			open: func(t *testing.T, m Model) Model {
				m.focused = PanelChart
				m.cursorIdx = len(m.slots.All()) - 1
				m, _ = escSend(t, m, escKey(tea.KeyCtrlS))
				return m
			},
			use: func(t *testing.T, m Model) Model { m, _ = escSend(t, m, escRunes("x")); return m },
		},
		{
			// Mid-work: a save in flight finishes, and its result changes
			// nothing on screen.
			name: "save chart while saving",
			open: func(t *testing.T, m Model) Model {
				m.focused = PanelChart
				m.cursorIdx = len(m.slots.All()) - 1
				m, _ = escSend(t, m, escKey(tea.KeyCtrlS))
				return m
			},
			use: func(t *testing.T, m Model) Model {
				m, _ = escSend(t, m, escRunes("x"), escKey(tea.KeyEnter))
				if !m.save.saving {
					t.Fatal("Enter did not start the save")
				}
				return m
			},
			late: func(t *testing.T, m Model) tea.Msg {
				return SaveChartDoneMsg{Gen: m.saveGen - 1, Saved: lookup.SavedChart{Name: "x", Path: "/nowhere/x.as", AtlasPlaced: true}}
			},
		},
		{
			name: "bodies",
			open: func(t *testing.T, m Model) Model { m, _ = escSend(t, m, escKey(tea.KeyCtrlB)); return m },
			use: func(t *testing.T, m Model) Model {
				m, _ = escSend(t, m, escKey(tea.KeyDown), escKey(tea.KeySpace), escRunes("m"))
				return m
			},
		},
		{
			name: "aspects",
			open: func(t *testing.T, m Model) Model { m, _ = escSend(t, m, escKey(tea.KeyCtrlA)); return m },
			use: func(t *testing.T, m Model) Model {
				m, _ = escSend(t, m, escKey(tea.KeyDown), escKey(tea.KeySpace), escRunes("s"))
				return m
			},
		},
		{
			name: "help",
			open: func(t *testing.T, m Model) Model { m, _ = escSend(t, m, escRunes("?")); return m },
			use:  func(t *testing.T, m Model) Model { m, _ = escSend(t, m, escKey(tea.KeyDown)); return m },
		},
		{
			name: "log console",
			open: func(t *testing.T, m Model) Model {
				m = m.logCommands([]string{"astrolog -one", "astrolog -two"})
				m, _ = escSend(t, m, escRunes(","), escRunes("l"))
				return m
			},
			use: func(t *testing.T, m Model) Model { m, _ = escSend(t, m, escKey(tea.KeyUp)); return m },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := escBase(t)
			if tc.name == "save chart" || tc.name == "save chart while saving" {
				base.focused = PanelChart
				base.cursorIdx = len(base.slots.All()) - 1
			}
			before := snapshotForEsc(base)
			m := tc.open(t, base)
			if m.startup == base.startup && !m.helpOpen && !m.console.open {
				t.Fatalf("%s did not open", tc.name)
			}
			m = tc.use(t, m)
			m, _ = escSend(t, m, escKey(tea.KeyEsc))
			requireSameScreen(t, "after Esc", before, snapshotForEsc(m))
			if m.helpOpen || m.console.open {
				t.Error("Esc left the owner open")
			}
			if tc.late != nil {
				m, _ = escSend(t, m, tc.late(t, m))
				requireSameScreen(t, "after the late result", before, snapshotForEsc(m))
			}
		})
	}

	// C7a: a chooser's preview has landed, a second one is in flight; Esc
	// puts the entry frame back at once, and the late preview changes
	// nothing.
	for _, chooser := range []struct {
		name string
		open tea.KeyType
	}{{"bodies preview", tea.KeyCtrlB}, {"aspects preview", tea.KeyCtrlA}} {
		t.Run(chooser.name, func(t *testing.T) {
			base := escBase(t)
			before := snapshotForEsc(base)
			beforeFrame := base.View()
			m, _ := escSend(t, base, escKey(chooser.open))
			preview := func(m Model, positions []aspect.Position) (Model, uint64, uint64) {
				m, _ = escSend(t, m, escKey(tea.KeySpace))
				m, _ = m.onRefreshTick(refreshTickMsg{surface: surfaceText, timerID: m.refresh[surfaceText].timerID})
				return m, m.refresh[surfaceText].runningRunID, m.refresh[surfaceText].runningWantID
			}
			landed := fullChartPositions()[:3]
			m, run1, want1 := preview(m, landed)
			if run1 == 0 {
				t.Fatal("the preview started no run")
			}
			m, _ = escSend(t, m, FirstFrameReadyMsg{Positions: landed, Aspects: tasteOwnAspects(), RunID: run1, WantID: want1})
			if reflect.DeepEqual(m.chart, before.Chart) {
				t.Fatal("the landed preview did not change the frame behind the chooser")
			}
			m, run2, want2 := preview(m, nil)
			m, _ = escSend(t, m, escKey(tea.KeyEsc))
			requireSameScreen(t, "right after Esc", before, snapshotForEsc(m))
			if got := m.View(); got != beforeFrame {
				t.Errorf("right after Esc the screen is not the one the chooser opened over:\n%s", got)
			}
			m, _ = escSend(t, m, FirstFrameReadyMsg{Positions: fullChartPositions()[:5], Aspects: tasteOwnAspects(), RunID: run2, WantID: want2})
			requireSameScreen(t, "after the late preview", before, snapshotForEsc(m))
			if got := m.View(); got != beforeFrame {
				t.Errorf("the late preview changed the screen:\n%s", got)
			}
		})
	}

	// Attached Details: Esc detaches and gives focus back to the panel that
	// had it, and keeps what was done while attached.
	t.Run("details attached", func(t *testing.T) {
		stubTextCompute(t)
		stubRawReports(t)
		m := loadedTimeline(t, Config{Accessible: true})
		m.focused = PanelChart
		m, _ = escSend(t, m, escRunes("0"), escKey(tea.KeyEnter))
		if !m.details.attached {
			t.Fatal("Enter did not attach Details")
		}
		tab := fmt.Sprint(m.activeTabID(PanelDetails))
		m, _ = escSend(t, m, escKey(tea.KeyDown), escKey(tea.KeyDown))
		before := m.targetFloatingTime
		m, _ = escSend(t, m, escKey(tea.KeyEnter))
		jumped := m.targetFloatingTime
		if jumped.Equal(before) {
			t.Fatal("Enter on a timeline row did not jump")
		}
		m, _ = escSend(t, m, escKey(tea.KeyEsc))
		if m.details.attached {
			t.Error("Esc left Details attached")
		}
		if m.focused != PanelChart {
			t.Errorf("focus is %v, want the Chart panel that had it", m.focused)
		}
		if got := fmt.Sprint(m.activeTabID(PanelDetails)); got != tab {
			t.Errorf("Details tab is %s, want %s kept", got, tab)
		}
		if !m.targetFloatingTime.Equal(jumped) {
			t.Error("Esc undid the timeline jump made while attached")
		}
	})
}
