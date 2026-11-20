package ui

import (
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// empty_pick_test.go: operator, 2026-11-04 (interview): with nothing ticked in
// the body chooser, charts draw the lens's default set (Astrolog's own default
// objects plus the lens's additions; at first this was Astrolog's alone, and
// the same day the operator had it defined by the lens). Nothing ticked is "no
// choice of mine", not "draw no body": an empty set used to compile to a bare
// -R0, Astrolog printed no body and the screen said "Couldn't cast "
// at every start. An empty set is no longer saved, and the bare `drawn:` line
// that earlier 0.1.3 builds wrote reads the same way.

const noBodiesChosen = "no bodies chosen, the default bodies are shown"

// bareRestriction reports an argv that restricts a chart to no object at all:
// a -R0 or -RT0 with no object number after it.
func bareRestriction(argv []string) bool {
	for i, a := range argv {
		if a != "-R0" && a != "-RT0" {
			continue
		}
		if i+1 == len(argv) {
			return true
		}
		if _, err := strconv.Atoi(argv[i+1]); err != nil {
			return true
		}
	}
	return false
}

// hostDefault is the set the default policy draws on this host: Astrolog's own
// default objects and the five the lens adds (Chiron, the South Node, Lilith, the
// Nadir, the Descendant), less those with no ephemeris file.
func hostDefault(m Model) map[int]bool {
	out := map[int]bool{}
	for _, n := range append(slices.Clone(m.ephemeris.Default), 11, 17, 18, 25, 28) {
		if !m.ephemeris.IsAbsent(n) {
			out[n] = true
		}
	}
	return out
}

// emptyPickProof is what both tests hold the screen to once the empty choice
// has been made (or read): the default policy, no empty restriction in any
// argv since mark, the chart cast with the default set, the Log line once.
func emptyPickProof(t *testing.T, m Model, entry recipe.ObjectPolicy, read func() [][]string, mark int, want map[int]bool) {
	t.Helper()
	def := lookup.Hellenistic.Objects
	def.Nodes = entry.Nodes
	if !reflect.DeepEqual(m.lens.Objects, def) {
		t.Errorf("the lens holds %+v, want the default policy %+v", m.lens.Objects, def)
	}
	argvs := read()[mark:]
	for _, a := range argvs {
		if bareRestriction(a) {
			t.Errorf("a bare restriction went out: %v", band(a))
		}
	}
	argv := textArgv(argvs)
	if argv == nil {
		t.Fatalf("no text-surface cast; log:\n%s", said(m))
	}
	if only, extra := sameSet(want, drawnByArgv(argv, m.ephemeris.Default)); len(only) != 0 || len(extra) != 0 {
		t.Errorf("the chart does not draw the default set: without %v, extra %v (argv %v)", only, extra, band(argv))
	}
	if n := countLines(m, " not cast,"); n != 0 {
		t.Errorf("the empty choice failed the cast:\n%s", said(m))
	}
	if strings.Contains(ansi.Strip(m.View()), "Couldn't cast") {
		t.Error("the screen says the cast failed")
	}
	if n := countLines(m, noBodiesChosen); n != 1 {
		t.Errorf("the Log carries %q %d times, want once:\n%s", noBodiesChosen, n, said(m))
	}
}

// said is the Log without its argv lines, which would bury a failure.
func said(m Model) string {
	var out []string
	for _, l := range m.logLines {
		if l.lane != laneArgv {
			out = append(out, l.text)
		}
	}
	return strings.Join(out, "\n")
}

// noDrawnLine quits the model and requires a written session without a drawn:
// line, the bare one included.
func noDrawnLine(t *testing.T, m Model) {
	t.Helper()
	m.quit()
	b, err := os.ReadFile(store.SessionPath())
	if err != nil || !strings.Contains(string(b), "version: 4\n") {
		t.Fatalf("the quit wrote no session (%v):\n%s", err, b)
	}
	if line := fileLine(t, "drawn:"); line != "" {
		t.Errorf("the session holds the line %q, want none:\n%s", line, b)
	}
}

// Unticking every body and confirming draws the default set, says so once, and
// writes no drawn: line at the next quit.
func TestEmptyPickDrawsDefaults(t *testing.T) {
	sessionSandbox(t)
	m, read, _ := ticksStart(t)
	want, entry := hostDefault(m), m.lens.Objects
	if len(want) == 0 {
		t.Fatal("fixture: the host has no default objects")
	}

	cm := chooserOf(t, m)
	var ticked []int
	for n, on := range cm.bodyModal.Selected {
		if on {
			ticked = append(ticked, n)
		}
	}
	slices.Sort(ticked)
	if len(ticked) == 0 {
		t.Fatal("fixture: the chooser opens with nothing ticked")
	}
	mark := len(read())
	for _, n := range ticked {
		cm = tickObject(t, cm, n)
	}
	if left := ticksOn(cm.bodyModal.Selected); len(left) != 0 {
		t.Fatalf("fixture: still ticked after unticking all: %v", left)
	}
	// The default set's frame was cast when the app started and is remembered; the
	// proof reads the cast the empty choice asks for, so nothing is remembered.
	cm = cm.forgetMemory()
	cm = pressInChooser(t, cm, tea.KeyMsg{Type: tea.KeyEnter})
	emptyPickProof(t, cm, entry, read, mark, want)
	noDrawnLine(t, cm)
}

// A session file holding the bare `drawn:` line, as earlier 0.1.3 builds wrote
// it for an empty choice, reads as no choice: the default set is drawn, the Log
// says so once, and the next quit drops the line.
func TestBareDrawnLineDrawsDefaults(t *testing.T) {
	sessionSandbox(t)
	m, read, _ := ticksStart(t)
	want, entry := hostDefault(m), m.lens.Objects
	if len(want) == 0 {
		t.Fatal("fixture: the host has no default objects")
	}

	seedSession(t, hereText("version: 3\ndrawn:\nfloating:\n  selected: false\n"))
	sess := reloaded(t)
	if sess.Drawn == nil || len(sess.Drawn) != 0 {
		t.Fatalf("fixture: the file did not read as a bare drawn: line (%#v)", sess.Drawn)
	}
	mark := len(read())
	m2 := drawnRestart(t, m, m.ephemeris, sess)
	emptyPickProof(t, m2, entry, read, mark, want)
	noDrawnLine(t, m2)
}

// The session gets a name list or nothing, never an empty list: a bare line
// from a path that waited for the object list, an explicit policy that holds no
// object, and one whose only objects have a comma in their name (which no line
// could carry) are all nil.
func TestNoEmptyDrawnSetIsGiven(t *testing.T) {
	waiting := Model{drawnRaw: []string{}}
	if got := waiting.drawnNames(); got != nil {
		t.Errorf("a bare line waiting for the object list gives %#v, want nil", got)
	}
	reg := []aspect.RegistryObject{{Object: aspect.Object{Number: 99, Name: "Comma,Body"}}}
	for name, objects := range map[string]recipe.ObjectPolicy{
		"an explicit policy with no object":       {Baseline: recipe.Explicit},
		"an explicit policy of a comma-named one": {Baseline: recipe.Explicit, Selection: []int{99}},
	} {
		m := Model{objectRegistry: reg}
		m.lens.Objects = objects
		if got := m.drawnNames(); got != nil {
			t.Errorf("%s gives %#v, want nil", name, got)
		}
	}
}
