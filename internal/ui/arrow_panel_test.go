package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// arrowBase is a laid-out, ready model with both registries seeded, so the
// modal states below render a real frame rather than a loading one.
func arrowBase(t *testing.T) Model {
	t.Helper()
	m := tabsPairModel(t, Config{Accessible: true, Color: ColorNever})
	m.width, m.height = 144, 40
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{
		Positions:    fullChartPositions(),
		Aspects:      tasteOwnAspects(),
		CrossAspects: tasteCrossAspects(),
		Overlay:      true,
	})
	m.startup = startupReady
	m.aspectRegistry = testRegistry()
	m.objectRegistry = testObjectRegistry()
	return m
}

// pressTyped sends a NAMED key (tab, arrows) rather than a rune; the
// package's pressKey covers the rune case.
func pressTyped(m Model, t tea.KeyType) Model {
	tm, _ := m.Update(tea.KeyMsg{Type: t})
	return tm.(Model)
}

// TestArrowsReachTheSamePanelAsTab is C1: → walks Chart → Bodies → Aspects →
// Chart and ← walks it back, which is the order Tab already walks.
func TestArrowsReachTheSamePanelAsTab(t *testing.T) {
	forward := []struct{ from, want PanelID }{
		{PanelChart, PanelBodies},
		{PanelBodies, PanelAspects},
		{PanelAspects, PanelChart},
	}
	for _, tc := range forward {
		m := arrowBase(t)
		m.focused = tc.from
		if got := pressTyped(m, tea.KeyRight).focused; got != tc.want {
			t.Errorf("→ from %v focused %v, want %v", tc.from, got, tc.want)
		}
		// The same press through Tab, so the two can never drift apart.
		if got := pressTyped(m, tea.KeyTab).focused; got != tc.want {
			t.Errorf("tab from %v focused %v, want %v (the arrows must ride this)", tc.from, got, tc.want)
		}
	}
	backward := []struct{ from, want PanelID }{
		{PanelChart, PanelAspects},
		{PanelBodies, PanelChart},
		{PanelAspects, PanelBodies},
	}
	for _, tc := range backward {
		m := arrowBase(t)
		m.focused = tc.from
		if got := pressTyped(m, tea.KeyLeft).focused; got != tc.want {
			t.Errorf("← from %v focused %v, want %v", tc.from, got, tc.want)
		}
		if got := pressTyped(m, tea.KeyShiftTab).focused; got != tc.want {
			t.Errorf("⇧tab from %v focused %v, want %v", tc.from, got, tc.want)
		}
	}
}

// TestArrowsRideTheExistingPanelBindings is C2. Without this a builder could
// satisfy C1 with two NEW bindings and leave the keymap carrying two ways to
// say one thing, which is how the help modal and the footer drift.
func TestArrowsRideTheExistingPanelBindings(t *testing.T) {
	keys := defaultKeys()
	for _, tc := range []struct {
		name string
		id   BindingID
		want []string
	}{
		{"next", BindingNextView, []string{"tab", "right"}},
		{"prev", BindingPrevView, []string{"shift+tab", "left"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var rows []BindingEntry
			for _, e := range keys.mainBindings() {
				if e.ID == tc.id {
					rows = append(rows, e)
				}
			}
			if len(rows) != 1 {
				t.Fatalf("binding %v has %d Entries() rows, want exactly 1", tc.id, len(rows))
			}
			got := rows[0].Binding.Keys()
			if len(got) != len(tc.want) {
				t.Fatalf("keys = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("keys = %v, want %v", got, tc.want)
					break
				}
			}
		})
	}
}

// TestArrowsDoNotReachPanelsFromAModal is C3: the six states that dispatch
// ahead of main scope keep the arrows to themselves. Asserted on the whole
// frame, not just m.focused, because "changed nothing" is the claim.
func TestArrowsDoNotReachPanelsFromAModal(t *testing.T) {
	states := map[string]func(Model) Model{
		"atlas modal":  func(m Model) Model { m.startup = startupAtlasOpen; return m },
		"chart modal":  func(m Model) Model { m.startup = startupChartOpen; return m },
		"body modal":   func(m Model) Model { m.startup = startupBodyOpen; return m },
		"aspect modal": func(m Model) Model { m.startup = startupAspectOpen; return m },
		"help overlay": func(m Model) Model { m.helpOpen = true; return m },
		"console":      func(m Model) Model { m.console.open = true; return m },
	}
	for name, open := range states {
		for _, arrow := range []struct {
			label string
			key   tea.KeyType
		}{{"→", tea.KeyRight}, {"←", tea.KeyLeft}} {
			t.Run(name+" "+arrow.label, func(t *testing.T) {
				before := open(arrowBase(t))
				beforeFrame := before.View()
				after := pressTyped(before, arrow.key)
				if after.focused != before.focused {
					t.Errorf("%s moved focus from %v to %v while the %s was open",
						arrow.label, before.focused, after.focused, name)
				}
				if after.View() != beforeFrame {
					t.Errorf("%s changed the frame while the %s was open", arrow.label, name)
				}
			})
		}
	}
}

// TestPanelBindingsAdvertiseTheArrows is C6, restated for the sheet the
// operator approved on 2026-09-20. The FOOTER names the arrow alone, because
// naming both lands on 82 of 80 columns and KeyBarHeight reserves one row. The
// HELP MODAL carries the whole binding, so tab is documented, not dropped.
func TestPanelBindingsAdvertiseTheArrows(t *testing.T) {
	keys := defaultKeys()

	help := helpListText(keys, defaultLeaderKey, defaultLeaderMap())
	for _, want := range []string{"tab/→", "⇧tab/←"} {
		if !strings.Contains(help, want) {
			t.Errorf("the help modal does not carry %q, so that key is undocumented", want)
		}
	}

	bar := renderKeyBar(80, NewModel(Config{}).available(keys.mainBindings()), Config{Accessible: true}, "")
	for _, want := range []string{"Next panel: →"} {
		if !strings.Contains(bar, want) {
			t.Errorf("the key bar does not name %q", want)
		}
	}
	// The whole point of the footer entries: they must NOT spend the columns
	// the frame does not have.
	for _, unwanted := range []string{"tab/→", "⇧tab/←"} {
		if strings.Contains(bar, unwanted) {
			t.Errorf("the footer carries %q; that is the wording that overflows 80 columns", unwanted)
		}
	}
}

// TestFrameHeightSurvivesTheFooter is C7. A footer that wraps does not grow
// the frame — KeyBarHeight reserves exactly one row — so a too-long footer
// pushes the frame past the terminal instead of wrapping inside it.
//
// The scroll entry is only asserted from 132 columns up, and that is not a
// convenience: Details is laid out only at 132 and wider, so an 80-column
// frame never carries the scroll entry. `renderKeyBar(80, …, scroll=true)`
// does wrap onto two lines — it did so before this slice too, and the
// keybar golden's "80 accessible details=true" block records it — but no
// frame can reach that combination. Asserting it would pin a state the
// product cannot enter and would have failed against unchanged code.
func TestFrameHeightSurvivesTheFooter(t *testing.T) {
	for _, width := range []int{80, 132, 200} {
		bar := renderKeyBar(width, defaultKeys().mainBindings(), Config{Accessible: true}, "")
		if lines := strings.Count(bar, "\n") + 1; lines != 1 {
			t.Errorf("key bar at %d cols is %d lines; KeyBarHeight reserves 1, so it overflows the frame", width, lines)
		}
		m := arrowBase(t)
		m.width, m.height = width, 40
		if got := strings.Count(m.View(), "\n") + 1; got != m.height {
			t.Errorf("frame at %d cols is %d lines, want exactly %d", width, got, m.height)
		}
	}
}
