package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// twoTabFixtures says the timeline fixtures build their Models with a registry
// that names no Moon, so Details declares the report and the Timeline and
// nothing else. A test about how `0`, `[` and `]` move between those two tabs
// asks for it with twoTabs; the Moon tab's own tests keep the full registry.
var twoTabFixtures bool

// twoTabs makes the timeline fixtures of this test declare no Moon tab.
func twoTabs(t *testing.T) {
	t.Helper()
	twoTabFixtures = true
	t.Cleanup(func() { twoTabFixtures = false })
}

// fixtureRegistry is the object list the timeline fixtures carry.
func fixtureRegistry() aspect.Registry {
	if twoTabFixtures {
		return registryWithoutMoon()
	}
	return lookup.Hellenistic.Registry
}

// forEachList runs f once per Details list as a "timeline" and a "moon"
// subtest. A test that holds for a list holds for both instances (M6).
func forEachList(t *testing.T, f func(t *testing.T, lk timelineList)) {
	t.Helper()
	for _, lk := range allLists {
		t.Run(lk.scope(), func(t *testing.T) { f(t, lk) })
	}
}

// openList is the `0` key for list k: it makes k's tab the active one the way
// that key would, and syncs. For the Timeline it is the key itself; the Moon
// tab is the next one along, and pressing through the Timeline would search
// the Timeline too, so the tab is chosen as cycleTab chooses it.
func openList(t *testing.T, m Model, k timelineList) (Model, tea.Cmd) {
	t.Helper()
	if k == listTimeline {
		return tpress(t, m, runes("0"))
	}
	m = m.withDetailsTab(m.focused, k.tab())
	m = m.clampTimelineCursor()
	return m.syncTimeline()
}

// listKeys are the `0` presses that make list k's tab the active one from the
// report: one for the Timeline, two for the Moon tab behind it.
func listKeys(k timelineList) []tea.KeyMsg {
	keys := make([]tea.KeyMsg, 0, 2)
	for range allLists[:k+1] {
		keys = append(keys, runes("0"))
	}
	return keys
}

// reportKey is the tab key that goes from list k's tab to the report: the
// strip is the report, the Timeline, the Moon list, and it wraps.
func reportKey(k timelineList) string {
	if k == listMoon {
		return "]"
	}
	return "["
}

// reattachList opens Details again on list k from the panel, as a user does
// after Enter on a row closed it.
func reattachList(t *testing.T, m Model, k timelineList) Model {
	t.Helper()
	if m.details.attached {
		t.Fatal("fixture: Details is still open")
	}
	if m.activeTabID(PanelDetails) != k.tab() {
		m, _ = openList(t, m, k)
	}
	m, cmd := tpress(t, m, keyEnter)
	m = pump(t, m, cmd)
	if !m.details.attached || m.activeTabID(PanelDetails) != k.tab() {
		t.Fatalf("fixture: reattach left attached=%v tab=%s", m.details.attached, m.activeTabID(PanelDetails))
	}
	return m
}
