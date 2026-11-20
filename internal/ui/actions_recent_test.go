package ui

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// The commands run last are kept in the session file and are on top of the
// Keybindings popup again after a quit and a start, newest first.
func TestRecentCommandsSurviveARestart(t *testing.T) {
	sessionSandbox(t)
	m := paletteModel(t)
	m = runViaCommands(t, m, "choose houses")
	m, _ = tpress(t, m, keyEsc)
	m = runViaCommands(t, m, "choose zodiac")
	m, _ = tpress(t, m, keyEsc)

	if _, cmd := m.quit(); cmd == nil {
		t.Fatal("q did not quit")
	}
	b, err := os.ReadFile(store.SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "\nactions:\n  - Choose zodiac\n  - Choose houses\n") {
		t.Fatalf("the file does not keep the recent commands:\n%s", b)
	}
	sess, ok, err := store.LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession: ok %v err %v", ok, err)
	}
	m2 := drawnRestart(t, m, m.ephemeris, sess)
	m2 = openCommandsPopup(t, m2)
	got := commandTitles(m2)
	recent := 0
	for _, r := range m2.keybindings.Matches {
		if r.Recent {
			recent++
		}
	}
	if len(got) < 2 || got[0] != "Choose zodiac" || got[1] != "Choose houses" || recent != 2 {
		t.Errorf("after the restart Global starts %v with %d recent, want Choose zodiac, Choose houses and 2", got[:min(3, len(got))], recent)
	}
}

// The popup keeps five, newest first, each once.
func TestRecentCommandsKeepFive(t *testing.T) {
	var recent []keys.BindingID
	ids := []keys.BindingID{keys.BindingZodiac, keys.BindingJumpNow, keys.BindingSessions, keys.BindingCopy, keys.BindingQuit, keys.BindingOpenCharts, keys.BindingJumpNow}
	for _, id := range ids {
		recent = promoteAction(recent, id)
	}
	want := []keys.BindingID{keys.BindingJumpNow, keys.BindingOpenCharts, keys.BindingQuit, keys.BindingCopy, keys.BindingSessions}
	if !slices.Equal(recent, want) {
		t.Errorf("recent = %v, want %v", recent, want)
	}
}

// Reading the titles back drops one no command has, counts a repeat once and
// keeps five; writing gives each command's own title in the same order.
func TestRecentCommandsReadFromTitles(t *testing.T) {
	m := paletteModel(t)
	got := m.recentActionsFromTitles([]string{"Gone command", "Choose zodiac", "Now", "Choose zodiac", "Copy", "Quit", "Sessions", "Open chart"})
	want := []keys.BindingID{keys.BindingZodiac, keys.BindingJumpNow, keys.BindingCopy, keys.BindingQuit, keys.BindingSessions}
	if !slices.Equal(got, want) {
		t.Fatalf("read %v, want %v", got, want)
	}
	m.recentActions = got
	if titles := m.recentActionTitles(); !slices.Equal(titles, []string{"Choose zodiac", "Now", "Copy", "Quit", "Sessions"}) {
		t.Errorf("titles = %q", titles)
	}
	m.recentActions = nil
	if titles := m.recentActionTitles(); titles != nil {
		t.Errorf("no recent commands wrote %#v, want nil (no block)", titles)
	}
}

// A title names one command: two commands of the list never share one, so a
// kept title always reads back as the command that wrote it.
func TestEveryActionTitleIsOnce(t *testing.T) {
	m := paletteModel(t)
	seen := map[string]keys.BindingID{}
	for _, e := range m.paletteEntries() {
		if e.Title == "" {
			t.Errorf("command %v has no title", e.ID)
		}
		if id, ok := seen[e.Title]; ok {
			t.Errorf("%q names both %v and %v", e.Title, id, e.ID)
		}
		seen[e.Title] = e.ID
	}
}

// A command whose title follows the screen (Enter reads Timeline or Report) is
// kept by its own title, so it reads back whatever the screen showed.
func TestRecentCommandIsKeptByItsOwnTitle(t *testing.T) {
	m := paletteModel(t)
	if m.attachTitle() == "Timeline" {
		t.Fatal("premise: Enter reads Timeline here, so the two titles cannot be told apart")
	}
	m.recentActions = []keys.BindingID{keys.BindingAttach}
	titles := m.recentActionTitles()
	if !slices.Equal(titles, []string{"Timeline"}) {
		t.Fatalf("titles = %q, want the command's own title Timeline", titles)
	}
	if got := m.recentActionsFromTitles(titles); !slices.Equal(got, m.recentActions) {
		t.Errorf("read back %v, want %v", got, m.recentActions)
	}
}

// The New row of the Sessions list starts a session with no recent commands,
// from a session with charts and from one that holds only recent commands.
func TestNewSessionHasNoRecentCommands(t *testing.T) {
	for _, n := range []int{2, 0} {
		freshModels(t)
		stubHouseLookups(t)
		m := sessionsModel(t, n)
		m.recentActions = []keys.BindingID{keys.BindingZodiac}
		m, cmd := pressNewSession(t, m)
		m = pump(t, m, cmd)
		store.ReleaseInstanceLock()
		if m.recentActions != nil {
			t.Errorf("with %d charts the new session keeps %v", n, m.recentActions)
		}
	}
}
