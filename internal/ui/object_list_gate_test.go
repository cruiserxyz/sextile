package ui

import (
	"errors"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// object_list_gate_test.go: objects slice 0b. Every printed row is named
// through Astrolog's object list, so the first chart is not cast before the
// list is in, and a list that cannot be read is a can't-start.

// A start with no saved session waits for the list too: the workspace, and so
// every Astrolog call, comes after it.
func TestStartWaitsForTheObjectList(t *testing.T) {
	var tm tea.Model = withoutObjectList(newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen)))
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, cmd := tm.Update(perimeter())
	m := tm.(Model)
	if m.slots != nil {
		t.Fatal("the workspace was built before the object list arrived")
	}
	if got := argvTexts(m); len(got) != 0 {
		t.Fatalf("Astrolog was called before the object list arrived:\n%s", strings.Join(got, "\n"))
	}
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, ok := msg.(refreshResultMsg); ok {
				t.Fatalf("a chart was cast before the object list arrived: %T", msg)
			}
		}
	}
	tm, _ = tm.Update(ObjectRegistryLoadedMsg{Registry: drawnRegistry()})
	m = tm.(Model)
	if m.slots == nil {
		t.Fatal("the workspace was not built when the object list arrived")
	}
	if len(m.lens.Registry) != len(drawnRegistry()) {
		t.Errorf("the lens carries %d objects, want the list's %d", len(m.lens.Registry), len(drawnRegistry()))
	}
}

// A list that cannot be read stops the start on the can't-start screen, in
// Astrolog's words; nothing is cast and a quit writes no session.
func TestUnreadableObjectListCannotStart(t *testing.T) {
	sessionSandbox(t)
	var tm tea.Model = withoutObjectList(newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen)))
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(perimeter())
	tm, _ = tm.Update(ObjectRegistryLoadedMsg{Err: errors.New("astrolog: no object rows found")})
	m := tm.(Model)
	want := "Sextile can't start: Astrolog's list of bodies could not be read: astrolog: no object rows found"
	if got := csText(m.View()); !strings.Contains(got, want) {
		t.Errorf("the screen lacks %q:\n%s", want, got)
	}
	if m.slots != nil {
		t.Error("a workspace was built without the object list")
	}
	if got := argvTexts(m); len(got) != 0 {
		t.Errorf("Astrolog was called without the object list:\n%s", strings.Join(got, "\n"))
	}
	m.quit()
	if _, err := os.Stat(store.SessionPath()); err == nil {
		t.Error("a quit on the can't-start screen wrote a session")
	}
}
