package ui

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// the specification Part B: a key that cannot act here does nothing and
// writes nothing. The key bar shows only what works now; that is the whole
// explanation, and the Log is never used to explain a key press.

// quietState is one state the keyboard can be in, named for a failure message.
type quietState struct {
	name string
	m    Model
}

// withoutLog is the model with its Log taken out, printed: two presses that
// differ only in what they wrote to the Log print the same.
func withoutLog(m Model) string {
	m.logLines = nil
	return fmt.Sprintf("%+v", m)
}

// mainQuietStates are the main screen's states from § Key states.
func mainQuietStates(t *testing.T) []quietState {
	t.Helper()
	cfg := Config{Accessible: true, Color: ColorNever}

	preLocation := NewModelWithClock(cfg, FrozenClock(time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)))
	preLocation.width, preLocation.height = 144, 40

	noAnchor := readyModelWithSlots(t, berlinP)
	noAnchor.cfg = cfg
	noAnchor.width, noAnchor.height = 144, 40
	noAnchor.focused = PanelChart

	base := escBase(t)
	base.cfg = cfg
	onAnchored := base
	onAnchored.focused = PanelChart
	onAnchored.cursorIdx = 0
	onFloating := base
	onFloating.focused = PanelChart
	onFloating.cursorIdx = len(base.slots.All()) - 1
	onBodies := base
	onBodies.focused = PanelBodies
	onAspects := base
	onAspects.focused = PanelAspects
	computing := base
	computing.startup = startupComputing
	noRegistries := base
	noRegistries.aspectRegistry, noRegistries.aspectRegErr = nil, errors.New("no aspect rows")
	noRegistries.objectRegistry, noRegistries.objectRegErr = nil, errors.New("no object rows")
	leader, _ := escSend(t, onFloating, escRunes(","))

	return []quietState{
		{"before a location", preLocation},
		{"no anchor", noAnchor},
		{"anchored slot", onAnchored},
		{"floating slot", onFloating},
		{"bodies focused", onBodies},
		{"aspects focused", onAspects},
		{"computing", computing},
		{"registries failed", noRegistries},
		{"leader pending", leader},
	}
}

// ownerQuietStates are every owner's states from § Key states.
func ownerQuietStates(t *testing.T) []quietState {
	t.Helper()
	press := func(m Model, k string) Model { m, _ = escSend(t, m, keyMsgFor(t, k)); return m }
	states := mainQuietStates(t)
	for _, c := range keyOwnerCases() {
		if c.owner == ownerMain {
			continue
		}
		base := reachOwner(t, c, Config{Accessible: true, Color: ColorNever}, 144, 40)
		states = append(states, quietState{c.name, base})
		switch c.owner {
		case ownerCity, ownerOpenChart, ownerBodies, ownerAspects:
			states = append(states, quietState{c.name + ", no match", press(press(press(base, "q"), "x"), "z")})
		case ownerChartTitle, ownerSaveChart:
			states = append(states, quietState{c.name + ", typed", press(base, "x")})
		case ownerDetails:
			states = append(states, quietState{"details, timeline", onTimeline(t, Config{Accessible: true, Color: ColorNever})})
		}
	}
	return states
}

// B1: a press that acts on nothing writes nothing. Every entry of the owner's
// table, and two keys no table binds, are pressed in every state; a press that
// returns no command and changes nothing but the Log must not change the Log.
func TestQuietKeys(t *testing.T) {
	old := yankLine
	yankLine = func(string) error { return nil }
	t.Cleanup(func() { yankLine = old })
	stubTextCompute(t)
	stubRawReports(t)

	t.Run("removed notes", quietKeysRemovedNotes)
	for _, s := range ownerQuietStates(t) {
		t.Run(s.name, func(t *testing.T) {
			keys := []string{"z", "f9"}
			for _, e := range s.m.ownerBindings(s.m.currentOwner()) {
				keys = append(keys, e.Binding.Keys()...)
			}
			for _, k := range keys {
				msg, ok := quietKeyMsg(t, k)
				if !ok {
					continue
				}
				before := withoutLog(s.m)
				tm, cmd := s.m.Update(msg)
				after := tm.(Model)
				if cmd != nil || withoutLog(after) != before {
					continue
				}
				if !reflect.DeepEqual(after.logLines, s.m.logLines) {
					t.Errorf("%q did nothing and wrote to the Log: %q", k, after.logPanelLines()[len(s.m.logPanelLines()):])
				}
			}
		})
	}
}

// quietKeyMsg is keyMsgFor, plus the function keys no table binds.
func quietKeyMsg(t *testing.T, k string) (tea.KeyMsg, bool) {
	t.Helper()
	if k == "f9" {
		return tea.KeyMsg{Type: tea.KeyF9}, true
	}
	return keyMsgFor(t, k), true
}

// B1, by name: each line § Removed notes retires, pressed where it used to be
// written. The Log is unchanged. Run inside TestQuietKeys, B1's verifier.
func quietKeysRemovedNotes(t *testing.T) {
	states := map[string]Model{}
	for _, s := range mainQuietStates(t) {
		states[s.name] = s.m
	}
	for _, c := range []struct{ name, state, key string }{
		{"time step on the anchored slot", "anchored slot", "d"},
		{"n on the anchored slot", "anchored slot", "n"},
		{"N on the anchored slot", "anchored slot", "N"},
		{"N with nothing to go back to", "floating slot", "N"},
		{"t with no anchor", "no anchor", "t"},
		{"save on the anchored slot", "anchored slot", "ctrl+s"},
		{"bodies before the first frame", "computing", "ctrl+b"},
		{"aspects before the first frame", "computing", "ctrl+a"},
		{"bodies after the registry failed", "registries failed", "ctrl+b"},
		{"aspects after the registry failed", "registries failed", "ctrl+a"},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := states[c.state]
			tm, _ := m.Update(keyMsgFor(t, c.key))
			if got := tm.(Model).logPanelLines(); len(got) != len(m.logPanelLines()) {
				t.Errorf("%s wrote %q", c.key, got[len(m.logPanelLines()):])
			}
		})
	}
}

// partnersOf are the entries that share a bar pair with id: the bar names
// ↑↓ or 1/2/3 as one entry, so the pair acts if any of them does.
var partnersOf = map[BindingID][]BindingID{
	BindingSlotCycleUp: {BindingSlotCycleDown},
	BindingListUp:      {BindingListDown},
	BindingScrollUp:    {BindingScrollDown},
	BindingTabNext:     {BindingTabPrev},
	BindingFocusChart:  {BindingFocusBodies, BindingFocusAspects},
}

// B2, B3: every entry the bar shows acts where it is shown. Acting is a
// returned command or a changed model, the Log aside; ↑ at a list's first row
// and ↓ at its last are piece 4's C12, so a pair acts if either half does.
func TestKeyBarShowsWhatWorks(t *testing.T) {
	old := yankLine
	yankLine = func(string) error { return nil }
	t.Cleanup(func() { yankLine = old })
	stubTextCompute(t)
	stubRawReports(t)

	for _, s := range ownerQuietStates(t) {
		t.Run(s.name, func(t *testing.T) {
			all := s.m.ownerBindings(s.m.currentOwner())
			keysOf := func(id BindingID) []string {
				for _, e := range all {
					if e.ID == id {
						return e.Binding.Keys()
					}
				}
				return nil
			}
			for _, e := range s.m.keyBarEntries() {
				if !e.Bar {
					continue
				}
				keys := append([]string(nil), e.Binding.Keys()...)
				for _, p := range partnersOf[e.ID] {
					keys = append(keys, keysOf(p)...)
				}
				// A keyless entry is a leader sequence (Details' `,y`).
				start := s.m
				if len(keys) == 0 && e.ID == BindingCopy {
					keys = []string{"y"}
					tl, _ := start.Update(keyMsgFor(t, ","))
					start = tl.(Model)
				}
				acted := false
				for _, k := range keys {
					start.slots = cloneSlots(t, s.m.slots) // a list key changes the list
					start = withSceneColumns(start)        // columns name slots by identity
					before := withoutLog(start)
					beforeView := start.View()
					tm, cmd := start.Update(keyMsgFor(t, k))
					after := tm.(Model)
					if cmd != nil || withoutLog(after) != before || after.View() != beforeView {
						acted = true
						break
					}
				}
				if !acted {
					t.Errorf("the bar shows %q (%s), and it does nothing here", e.Action, e.BarKey)
				}
			}
		})
	}
}

// B5: routine reading writes nothing. Each case is the moment a dropped line
// used to be written (the specification).
func TestRoutineWorkIsQuiet(t *testing.T) {
	quiet := func(t *testing.T, what string, before, after Model) {
		t.Helper()
		if got := after.logPanelLines()[len(before.logPanelLines()):]; len(got) != 0 {
			t.Errorf("%s wrote %q", what, got)
		}
	}
	t.Run("bodies apply", func(t *testing.T) {
		m := escBase(t)
		m, _ = escSend(t, m, escKey(tea.KeyCtrlB))
		after, _ := escSend(t, m, escKey(tea.KeySpace), escKey(tea.KeyEnter))
		quiet(t, "applying the bodies popup", m, after)
	})
	t.Run("aspects apply", func(t *testing.T) {
		m := escBase(t)
		m, _ = escSend(t, m, escKey(tea.KeyCtrlA))
		after, _ := escSend(t, m, escKey(tea.KeySpace), escKey(tea.KeyEnter))
		quiet(t, "applying the aspects popup", m, after)
	})
	t.Run("a body comes back", func(t *testing.T) {
		m := escBase(t)
		m = m.reportUnavailable([]string{"Eris"})
		after := m.reportUnavailable(nil)
		quiet(t, "a body becoming available again", m, after)
		if got := m.logPanelLines(); len(got) == 0 || !strings.Contains(got[len(got)-1], "no ephemeris for Eris") {
			t.Errorf("a body with no ephemeris is no longer reported: %q", got)
		}
	})
	t.Run("wheel shown", func(t *testing.T) {
		m := escBase(t)
		m.previewActive = true
		// The first publish of a session names the address (W1 part 1); a
		// refresh after it, the routine work, says nothing.
		after, _ := m.onWheelPublished(wheelPublishedMsg{path: "http://127.0.0.1:1/wheel.png"})
		if got := after.logPanelLines()[len(m.logPanelLines()):]; len(got) != 1 || got[0] != "Wheel at http://127.0.0.1:1/wheel.png (Ctrl+X again closes it)" {
			t.Errorf("a wheel that opened wrote %q, want the one address line", got)
		}
		again, _ := after.onWheelPublished(wheelPublishedMsg{path: "http://127.0.0.1:1/wheel.png"})
		quiet(t, "a wheel that refreshed", after, again)
		failed, _ := m.onWheelPublished(wheelPublishedMsg{path: "http://127.0.0.1:1/wheel.png", err: errors.New("no browser")})
		lines := failed.logPanelLines()[len(m.logPanelLines()):]
		if len(lines) != 2 || !strings.Contains(lines[0], "http://127.0.0.1:1/wheel.png") {
			t.Errorf("a wheel whose browser did not open logged %q, want the address and the error", lines)
		}
	})
	t.Run("city resolved and session restored", func(t *testing.T) {
		for _, line := range launchLog(t) {
			for _, routine := range []string{"active city", "restored", "opening picker"} {
				if strings.Contains(line, routine) {
					t.Errorf("a launch wrote %q", line)
				}
			}
		}
	})
}

// launchLog is the Log after a launch that resolves its city and restores a
// session chart from its snapshot.
func launchLog(t *testing.T) []string {
	t.Helper()
	snap := snapshotChart()
	return startedModel(t, withSession(perimeter(), v1(&snap, true, false, nil, nil))).logPanelLines()
}

// U3 4: with a notice showing, the last row holds the notice (cut from the
// right if it must be) and still holds Help and Quit.
func TestNoticeKeepsHelpAndQuit(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {66, 15}} {
		m := consoleModel(t, 0)
		m.width, m.height = size[0], size[1]
		m, _ = m.showNotice(noticeOnlyNow)
		row := strings.TrimRight(ansi.Strip(lastRow(m.View())), " ")
		if !strings.Contains(row, "Help: ?") || !strings.Contains(row, "Quit: q") {
			t.Errorf("%dx%d: last row %q lost Help or Quit", size[0], size[1], row)
		}
		if !strings.Contains(row, "Only Now") {
			t.Errorf("%dx%d: last row %q lacks the notice", size[0], size[1], row)
		}
		if w := ansi.StringWidth(row); w > size[0] {
			t.Errorf("%dx%d: last row is %d wide", size[0], size[1], w)
		}
		if size[0] == 66 && !strings.Contains(row, "…") && !strings.Contains(row, "...") {
			t.Errorf("66x15: notice %q was not cut", row)
		}
	}
}
