package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// detailsKeyStates are attached-Details states worth pressing a main key in:
// the timeline from Chart (with and without a jump to go back from), and the
// report from Aspects, which has no row cursor and lets more keys fall through.
func detailsKeyStates(t *testing.T) []Model {
	t.Helper()
	timeline := onTimeline(t, Config{Accessible: true})
	timeline.aspectRegistry, timeline.objectRegistry = testRegistry(), testObjectRegistry()
	jumped, _ := tpress(t, timeline, runes("n"))
	report := attachedReport(t, false, "Aspects", strings.Repeat("a report line\n", 60))
	states := []Model{timeline, jumped, report}
	// Each again with its cursor or window moved off the top, so a key that
	// moves back up has somewhere to go.
	for _, s := range append([]Model(nil), states...) {
		down, _ := tpress(t, s, keyDown)
		down, _ = tpress(t, down, keyDown)
		paged, _ := tpress(t, s, keyPgDn)
		scrolled, _ := tpress(t, s, tea.KeyMsg{Type: tea.KeyCtrlPgDown})
		states = append(states, down, paged, scrolled)
	}
	for _, s := range states {
		if s.currentOwner() != ownerDetails {
			t.Fatalf("fixture: owner %v, want attached Details", s.currentOwner())
		}
	}
	return states
}

// mainKeyName is how a failure names a main binding: its help words and key.
func mainKeyName(e BindingEntry) string {
	h := e.Binding.Help()
	return fmt.Sprintf("%q (%s, binding %d)", h.Desc, h.Key, e.ID)
}

// The operator's rule (Saturday try, 2026-10-24): Details answers its own keys
// first, and every other main key works exactly as on the main screen, except
// the keys on detailsRefused. A main binding added later that is neither served
// in Details nor on that list fails here by name.
func TestEveryMainKeyWorksInDetailsOrIsRefused(t *testing.T) {
	stubTextCompute(t)
	stubRawReports(t)
	states := detailsKeyStates(t)
	changed := func(before, after Model, cmd tea.Cmd) bool {
		return cmd != nil || before.View() != after.View() || fmt.Sprintf("%+v", before) != fmt.Sprintf("%+v", after)
	}
	for _, e := range defaultKeys().mainBindings() {
		if _, refused := detailsRefused[e.ID]; refused {
			continue
		}
		k := e.Binding.Keys()[0]
		// The leader's own entries are typed after the leader. A sequence the
		// leader does not know only clears it, so that is the baseline they must
		// beat; Esc's whole job is clearing it.
		leaderEntry := e.When != nil && e.When(Model{leader: leaderState{pending: true}}) && !e.When(Model{})
		acted := false
		for _, s := range states {
			before := s
			if leaderEntry {
				s, _ = tpress(t, s, runes(","))
				before = s
				if e.ID != BindingCancelLeader {
					before = s.clearLeader()
				}
			} else if e.When != nil && !e.When(s) {
				continue
			}
			tm, cmd := s.Update(keyMsgFor(t, k))
			if changed(before, tm.(Model), cmd) {
				acted = true
				break
			}
		}
		if !acted {
			t.Errorf("main key %s does nothing in Details and is not on detailsRefused", mainKeyName(e))
		}
	}
}

// Every refused key, pressed where Details does not answer it itself, leaves
// the main panels exactly as they were: focus, cursors, selection, the slots.
func TestRefusedKeysDoNothingInDetails(t *testing.T) {
	stubTextCompute(t)
	stubRawReports(t)
	pressed := 0
	for _, e := range defaultKeys().mainBindings() {
		if _, refused := detailsRefused[e.ID]; !refused {
			continue
		}
		for _, k := range e.Binding.Keys() {
			for i, s := range detailsKeyStates(t) {
				msg := keyMsgFor(t, k)
				if matchBinding(msg, s.available(s.ownerBindings(ownerDetails))) != BindingNone {
					continue // Details answers this key here; it is not the refused one
				}
				want := snapshotForEsc(s)
				got, _ := tpress(t, s, msg)
				pressed++
				requireSameScreen(t, fmt.Sprintf("%s key %q in state %d", mainKeyName(e), k, i), want, snapshotForEsc(got))
				if !got.details.attached || got.details.preEnter != s.details.preEnter {
					t.Errorf("%s key %q in state %d: Details attached=%v, what leaving restores changed", mainKeyName(e), k, i, got.details.attached)
				}
			}
		}
	}
	if pressed == 0 {
		t.Fatal("no refused key reached the main screen's dispatch: the test proves nothing")
	}
}

// `, a` from attached Details opens the House display popup, and Esc puts the
// operator back in Details, attached; `, c d` saves the defaults from there.
func TestLeaderWorksFromDetails(t *testing.T) {
	t.Run("house display", func(t *testing.T) {
		m := onTimeline(t, Config{Accessible: true})
		m, _ = tpress(t, m, keyDown)
		row := m.details.cursor
		m, _ = tpress(t, m, runes(","))
		m, _ = tpress(t, m, runes("a"))
		if got := m.currentOwner(); got != ownerDisplay {
			t.Fatalf("`, a` in Details: owner %v, want the House display popup", got)
		}
		m, _ = tpress(t, m, keyEsc)
		if got := m.currentOwner(); got != ownerDetails || !m.details.attached || m.details.cursor != row {
			t.Errorf("Esc in House display: owner %v attached=%v row %d, want Details attached on row %d", got, m.details.attached, m.details.cursor, row)
		}
	})
	t.Run("save defaults", func(t *testing.T) {
		stubTextCompute(t)
		sessionSandbox(t)
		m := onTimeline(t, Config{Accessible: true})
		m.place, m.placeGuessed = amsterdamNL, true
		var cmd tea.Cmd
		for _, k := range []string{",", "c", "d"} {
			m, cmd = tpress(t, m, runes(k))
		}
		if cmd == nil {
			t.Fatal("`, c d` in Details scheduled no save")
		}
		m = deliver(m, cmd)
		if got := ansi.Strip(m.View()); !strings.Contains(got, noticeDefaultsSaved) {
			t.Errorf("the footer does not show %q after `, c d` in Details:\n%s", noticeDefaultsSaved, got)
		}
		if !m.details.attached {
			t.Error("`, c d` detached Details")
		}
	})
}
