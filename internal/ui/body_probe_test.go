package ui

import (
	"context"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// body_probe_test.go proves the specification. All chart
// values are constructed test inputs.

// probeRegistry is a registry with the kinds of row the probe distinguishes: a
// planet, an asteroid, a Uranian, a body of a second family, and two angle/cusp
// rows that are never probed. Numbers follow Astrolog's own registry.
func probeRegistry() []aspect.RegistryObject {
	return []aspect.RegistryObject{
		{Number: 0, Name: "Earth"},
		{Number: 1, Name: "Sun"},
		{Number: 5, Name: "Mars"},
		{Number: 12, Name: "Ceres"},
		{Number: 22, Name: "Ascendant"},
		{Number: 23, Name: "2nd Cusp"},
		{Number: 36, Name: "Hades"},
		{Number: 43, Name: "Hygiea"},
	}
}

// probeCall is what the lookup seam was handed.
type probeCall struct {
	obj   int
	when  time.Time
	place location.PlaceInfo
}

// probeRecorder replaces the probe seam: it records every call, answers from
// reply, and blocks on gate (when set) until released or its ctx ends.
type probeRecorder struct {
	mu      sync.Mutex
	calls   []probeCall
	ctxs    []context.Context
	reply   func(obj int) (lookup.BodyProbe, error)
	gate    chan struct{}
	entered chan int
}

func (r *probeRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *probeRecorder) objects() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []int
	for _, c := range r.calls {
		out = append(out, c.obj)
	}
	return out
}

func stubProbe(t *testing.T, reply func(obj int) (lookup.BodyProbe, error)) *probeRecorder {
	t.Helper()
	r := &probeRecorder{reply: reply}
	old := probeBody
	t.Cleanup(func() { probeBody = old })
	probeBody = func(ctx context.Context, _ *lookup.Probers, when time.Time, place location.PlaceInfo, _ lookup.Lens, obj int) (lookup.BodyProbe, error) {
		r.mu.Lock()
		r.calls = append(r.calls, probeCall{obj: obj, when: when, place: place})
		r.ctxs = append(r.ctxs, ctx)
		gate, entered := r.gate, r.entered
		r.mu.Unlock()
		if entered != nil {
			entered <- obj
		}
		if gate != nil {
			select {
			case <-gate:
			case <-ctx.Done():
				return lookup.BodyProbe{}, ctx.Err()
			}
		}
		if r.reply == nil {
			return lookup.BodyProbe{Drawable: true}, nil
		}
		return r.reply(obj)
	}
	return r
}

// undrawable answers "no file" for the listed objects and "drawable" otherwise.
func undrawable(files []string, objs ...int) func(int) (lookup.BodyProbe, error) {
	return func(obj int) (lookup.BodyProbe, error) {
		for _, o := range objs {
			if o == obj {
				return lookup.BodyProbe{MissingFiles: files}, nil
			}
		}
		return lookup.BodyProbe{Drawable: true}, nil
	}
}

// probeModel is a ready model on the floating slot, with the probe registry.
func probeModel(t *testing.T) Model {
	t.Helper()
	m := readyModelWithSlots(t, testPlaceBerlin())
	m.width, m.height = 132, 40
	m.cfg = Config{Accessible: true, Color: ColorNever}
	m.objectRegistry = probeRegistry()
	return m
}

func probeKey(m Model, k tea.KeyMsg) (Model, tea.Cmd) {
	tm, cmd := m.Update(k)
	return tm.(Model), cmd
}

func probeTyped(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// open presses ⌃b and walks the cursor past the group rows onto the first
// list row (Earth), returning the model with the cmd that last step returned —
// the rest timer for Earth. The cursor starts on the group rows
// (the specification 2), where no body is under it and nothing is
// probed, so this is where these tests' "the popup has just opened on Earth"
// begins.
func openProbePopup(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	m, cmd := probeKey(m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if m.startup != startupBodyOpen {
		t.Fatalf("⌃b did not open the popup (startup=%q)", m.startup)
	}
	for i := 0; i < groupRows(); i++ {
		m, cmd = probeKey(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	return m, cmd
}

// flatten runs a command tree to completion, returning every message.
func flatten(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, flatten(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

func popupText(m Model) string {
	return renderBodyModal(m.bodyModal, m.cfg, m.width, m.height)
}

func rowOf(frame, name string) string {
	for _, l := range strings.Split(frame, "\n") {
		if strings.Contains(l, name) {
			return l
		}
	}
	return ""
}

// C1 (N3, N4, N7, N9, N10): a probe runs once the cursor rests, for the body
// under it, in the focused slot's own moment and place.
func TestPopupProbesTheBodyUnderTheCursor(t *testing.T) {
	t.Run("opening, moving and filtering each probe the body they land on", func(t *testing.T) {
		rec := stubProbe(t, nil)
		m, cmd := openProbePopup(t, probeModel(t))
		// Nothing runs before the rest: the cmd is a timer, abandoned at 50 ms.
		m = pumpWithin(t, m, 50*time.Millisecond, cmd)
		if n := rec.count(); n != 0 {
			t.Fatalf("%d probe(s) ran before the cursor rested", n)
		}
		m, cmd = openProbePopup(t, probeModel(t))
		m = pump(t, m, cmd)
		if got := rec.objects(); !reflect.DeepEqual(got, []int{0}) {
			t.Fatalf("after opening, probed %v, want [0] (Earth under the cursor)", got)
		}
		m, cmd = probeKey(m, tea.KeyMsg{Type: tea.KeyDown})
		m = pump(t, m, cmd)
		if got := rec.objects(); !reflect.DeepEqual(got, []int{0, 1}) {
			t.Fatalf("after moving down, probed %v, want [0 1]", got)
		}
		// N7: the filter puts a different object under the SAME cursor index.
		m, cmd = probeKey(m, probeTyped("hades"))
		if cmd == nil {
			t.Fatal("typing in the filter scheduled nothing; the object under the cursor changed")
		}
		m = pump(t, m, cmd)
		if got := rec.objects(); !reflect.DeepEqual(got, []int{0, 1, 36}) {
			t.Fatalf("after filtering, probed %v, want [0 1 36]", got)
		}
		_ = m
	})
	t.Run("holding a key runs no probe until the cursor stops", func(t *testing.T) {
		rec := stubProbe(t, nil)
		m, open := openProbePopup(t, probeModel(t))
		m = pump(t, m, open)
		before := rec.count()
		// Three keys under 150 ms apart: three timers armed, only the last live.
		var cmds []tea.Cmd
		for range 3 {
			var c tea.Cmd
			m, c = probeKey(m, tea.KeyMsg{Type: tea.KeyDown})
			cmds = append(cmds, c)
		}
		pump(t, m, cmds...)
		if got := rec.objects()[before:]; !reflect.DeepEqual(got, []int{12}) {
			t.Fatalf("probed %v after a held key, want only the row the cursor stopped on ([12], Ceres)", got)
		}
		// Out and back onto the same unprobed row: two live-looking timers for
		// Mars, of which only the latest may fire.
		before = rec.count()
		cmds = nil
		for _, k := range []tea.KeyType{tea.KeyUp, tea.KeyUp, tea.KeyDown} {
			var c tea.Cmd
			m, c = probeKey(m, tea.KeyMsg{Type: k})
			cmds = append(cmds, c)
		}
		pump(t, m, cmds...)
		if got := rec.objects()[before:]; !reflect.DeepEqual(got, []int{5}) {
			t.Fatalf("probed %v after leaving Mars and coming back, want one probe of Mars ([5])", got)
		}
	})
	t.Run("a key on the same row restarts a pending rest", func(t *testing.T) {
		rec := stubProbe(t, nil)
		recordLensSeams(t) // the Space below previews; keep it off Astrolog
		m, open := openProbePopup(t, probeModel(t))
		m = pump(t, m, open)
		before := rec.count()
		// Onto an unprobed row, and 100 ms later a toggle on that same row.
		m, navCmd := probeKey(m, tea.KeyMsg{Type: tea.KeyDown})
		navMsgs := make(chan []tea.Msg, 1)
		go func() { navMsgs <- flatten(navCmd) }()
		time.Sleep(100 * time.Millisecond)
		m, spaceCmd := probeKey(m, tea.KeyMsg{Type: tea.KeySpace})
		spaceMsgs := make(chan []tea.Msg, 1)
		go func() { spaceMsgs <- flatten(spaceCmd) }()
		deliver := func(msgs []tea.Msg) {
			for _, msg := range msgs {
				if _, ok := msg.(bodyProbeRestMsg); !ok {
					continue
				}
				tm, c := m.Update(msg)
				m = tm.(Model)
				for _, r := range flatten(c) {
					tm, _ := m.Update(r)
					m = tm.(Model)
				}
			}
		}
		// 150 ms from the navigation: the cursor has not rested, a key came 50 ms ago.
		deliver(<-navMsgs)
		if n := rec.count() - before; n != 0 {
			t.Fatalf("%d probe(s) 150 ms after the navigation, but a key arrived 50 ms ago", n)
		}
		// 150 ms from the Space: now it has rested.
		deliver(<-spaceMsgs)
		if got := rec.objects()[before:]; !reflect.DeepEqual(got, []int{1}) {
			t.Fatalf("probed %v 150 ms after the last key, want exactly [1] (Sun)", got)
		}
	})
	t.Run("angles and cusps start zero calls", func(t *testing.T) {
		rec := stubProbe(t, nil)
		m, cmd := openProbePopup(t, probeModel(t))
		m = pump(t, m, cmd)
		before := rec.count()
		for _, name := range []string{"ascendant", "cusp"} {
			var c tea.Cmd
			for range 8 {
				m, c = probeKey(m, tea.KeyMsg{Type: tea.KeyBackspace})
				m = pump(t, m, c)
			}
			before = rec.count()
			m, c = probeKey(m, probeTyped(name))
			m = pump(t, m, c)
			if n := rec.count(); n != before {
				t.Fatalf("resting on %q started %d call(s), want 0", name, n-before)
			}
		}
	})
	t.Run("one body selected, in the slot's own moment and place", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			base func(t *testing.T) Model
		}{
			{"anchored slot", func(t *testing.T) Model { m := listModel(t, 2, 1); m.objectRegistry = probeRegistry(); return m }},
			{"time-stepped floating slot", func(t *testing.T) Model {
				m := probeModel(t)
				m.targetFloatingTime = m.targetFloatingTime.Add(37 * time.Hour)
				return m
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				rec := stubProbe(t, nil)
				m := tc.base(t)
				m.cfg = Config{Accessible: true, Color: ColorNever}
				m, cmd := openProbePopup(t, m)
				pump(t, m, cmd)
				if rec.count() != 1 {
					t.Fatalf("calls = %d, want 1", rec.count())
				}
				got := rec.calls[0]
				wantWhen, wantPlace := m.focusedSlot().Time, m.focusedSlot().Location
				if m.focusedIsFloating() {
					wantWhen, wantPlace = transitTimeFromWorkspace(m), m.place
				}
				if !got.when.Equal(wantWhen) || got.place != wantPlace {
					t.Fatalf("probed at %v in %v, want the slot's %v in %v", got.when, got.place, wantWhen, wantPlace)
				}
			})
		}
	})
	for _, tc := range []struct {
		name     string
		sidereal bool
		compare  bool
		trueNode bool
	}{
		{"true node, tropical", false, false, true},
		{"mean node, tropical", false, false, false},
		{"sidereal lens", true, false, true},
		{"focused slot of a selected comparison", false, true, true},
	} {
		t.Run("the compiled argv selects exactly that object and keeps the chart's frame: "+tc.name, func(t *testing.T) {
			spawns := fixtureAstrolog(t, nil)
			m := listModel(t, 2, 1)
			if tc.compare {
				m = selectedSet(t, 3, true, 1)
			}
			m.cfg = Config{Accessible: true, Color: ColorNever}
			m.objectRegistry = probeRegistry()
			if tc.sidereal {
				m.frames.defZ, m.frames.defZSet = recipe.Lahiri, true
			}
			// The active selection must not leak into the probe.
			m.lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 3, 16, 18}, TrueNodeMode: tc.trueNode}
			m, _ = openProbePopup(t, m)
			m, _ = probeKey(m, tea.KeyMsg{Type: tea.KeyDown})
			m, cmd := probeKey(m, tea.KeyMsg{Type: tea.KeyDown}) // Mars (5)
			pumpPatient(t, m, cmd)
			got := spawns()
			if len(got) != 1 {
				t.Fatalf("spawns = %d, want exactly the one probe", len(got))
			}
			argv := got[0]
			if band := numberBand(argv, "-R0"); !reflect.DeepEqual(band, []int{5}) {
				t.Errorf("object band = %v, want [5]; argv %v", band, argv)
			}
			has := func(tok string) bool {
				for _, a := range argv {
					if a == tok {
						return true
					}
				}
				return false
			}
			if has("-Yn") != tc.trueNode {
				t.Errorf("-Yn present = %v, want %v (the chart's node mode); argv %v", has("-Yn"), tc.trueNode, argv)
			}
			// The compiler spells "no label" as `-zi "" ""`; a name or place
			// would be client text in the argv.
			for i, a := range argv {
				if a == "-zi" && (i+2 >= len(argv) || argv[i+1] != "" || argv[i+2] != "") {
					t.Errorf("the probe carries a -zi name or place: %q", argv[i:min(i+3, len(argv))])
				}
			}
			// The focused slot's own moment, place and frame, as the chart
			// compute for that slot gives them.
			ref := fixtureAstrolog(t, nil)
			refMsg := computeNatalSingleCmd(context.Background(), 1, 1, m.probers, m.focusedChart(), m.viewLens())()
			if fm, ok := refMsg.(FirstFrameReadyMsg); ok && fm.Err != nil {
				t.Fatalf("reference chart compute failed: %v", fm.Err)
			}
			refSpawns := ref()
			if len(refSpawns) == 0 {
				t.Fatal("the reference chart compute started no process")
			}
			chartArgv := refSpawns[0]
			for _, flag := range []string{"-qj", "-zl"} {
				if a, b := flagValue(argv, flag), flagValue(chartArgv, flag); a == "" || a != b {
					t.Errorf("%s = %q, the chart compute has %q", flag, a, b)
				}
			}
			if a, b := frameBand(argv), frameBand(chartArgv); len(a) == 0 || !reflect.DeepEqual(a, b) {
				t.Errorf("frame band = %v, the chart compute has %v", a, b)
			}
			if tc.sidereal && !slices.Contains(frameBand(argv), "=s") {
				t.Errorf("the sidereal lens produced a tropical frame: %v", frameBand(argv))
			}
		})
	}
}

// frameBand is the tokens from `-c <houses>` up to the object band: the house
// system, the zodiac pin and the neutralisers the compiler issues together.
func frameBand(argv []string) []string {
	for i, a := range argv {
		if a != "-c" {
			continue
		}
		var out []string
		for _, t := range argv[i:] {
			if t == "-Yn" || t == "-R0" || t == "-v0" {
				break
			}
			out = append(out, t)
		}
		return out
	}
	return nil
}

// flagValue is the token after the first occurrence of flag, joined with the
// next one for the two-value -zl.
func flagValue(argv []string, flag string) string {
	for i, a := range argv {
		if a == flag && i+1 < len(argv) {
			if flag == "-zl" && i+2 < len(argv) {
				return argv[i+1] + " " + argv[i+2]
			}
			return argv[i+1]
		}
	}
	return ""
}

// C3: an undrawable body is marked, and the line below the pinned bar names the
// file. Three registry objects of different kinds stand in for the missing one.
func TestPopupMarksAnUndrawableBody(t *testing.T) {
	reg := probeRegistry()
	nameOf := func(n int) string {
		for _, o := range reg {
			if o.Number == n {
				return o.Name
			}
		}
		t.Fatalf("object %d is not in the registry", n)
		return ""
	}
	for _, n := range []int{5, 12, 36} { // a planet, an asteroid, a Uranian
		name := nameOf(n)
		t.Run(name, func(t *testing.T) {
			stubProbe(t, undrawable([]string{"a.se1", "b.se1"}, n))
			m, cmd := openProbePopup(t, probeModel(t))
			m = pump(t, m, cmd)
			m, cmd = probeKey(m, probeTyped(strings.ToLower(name)))
			m = pump(t, m, cmd)

			frame := popupText(m)
			if row := rowOf(frame, "! "+name); row == "" {
				t.Fatalf("no `! %s` row in:\n%s", name, frame)
			}
			want := name + " can't be drawn: no ephemeris file a.se1, b.se1"
			lines := strings.Split(frame, "\n")
			bar, status := -1, -1
			for i, l := range lines {
				if strings.Contains(l, " selected ") {
					bar = i
				}
				if strings.Contains(l, want) {
					status = i
				}
			}
			if bar < 0 || status != bar+2 || strings.TrimSpace(strings.Trim(lines[bar+1], "|│ ")) != "" {
				t.Fatalf("the status line %q must sit in the message zone: a blank row below the tag bar, then the note (bar %d, status %d):\n%s", want, bar, status, frame)
			}

			// Away from the cursor the row stays marked and the line goes.
			for range len(name) {
				m, _ = probeKey(m, tea.KeyMsg{Type: tea.KeyBackspace})
			}
			m, _ = probeKey(m, tea.KeyMsg{Type: tea.KeyDown})
			frame = popupText(m)
			if strings.Contains(frame, "can't be drawn") {
				t.Errorf("the status line stays after the cursor moved on:\n%s", frame)
			}
			if rowOf(frame, "! "+name) == "" {
				t.Errorf("the marked row lost its mark away from the cursor:\n%s", frame)
			}
			if r := rowOf(frame, "Sun"); strings.Contains(r, "!") {
				t.Errorf("an unprobed row is marked: %q", r)
			}
		})
	}
	t.Run("colours on dims the row", func(t *testing.T) {
		stubProbe(t, undrawable([]string{"se99999s.se1"}, 12))
		m := probeModel(t)
		m.cfg = Config{Color: ColorAlways}
		m, cmd := openProbePopup(t, m)
		m, cmd = probeKey(m, probeTyped("ceres"))
		m = pump(t, m, cmd)
		frame := popupText(m)
		row := rowOf(frame, "! Ceres")
		if !strings.Contains(row, sgrFaint) {
			t.Errorf("the marked row is not dim with colours on: %q", row)
		}
		m.cfg = Config{Accessible: true, Color: ColorNever}
		if r := rowOf(popupText(m), "! Ceres"); r == "" || strings.Contains(r, sgrFaint) {
			t.Errorf("with colours off the row must be `! Ceres` and plain: %q", r)
		}
	})
}

// C4: a result for one body marks that body's row and no other.
func TestSupersededProbeMarksOnlyItsOwnBody(t *testing.T) {
	stubProbe(t, undrawable([]string{"se99999s.se1"}, 0))
	m, cmd := openProbePopup(t, probeModel(t))
	// Rest on Earth: take the rest message, then the probe command, unrun.
	var probeCmd tea.Cmd
	for _, msg := range flatten(cmd) {
		var next tea.Cmd
		tm, c := m.Update(msg)
		m, next = tm.(Model), c
		probeCmd = next
	}
	if probeCmd == nil {
		t.Fatal("the rest started no probe")
	}
	// The cursor moves on to Sun before the probe lands.
	m, _ = probeKey(m, tea.KeyMsg{Type: tea.KeyDown})
	for _, msg := range flatten(probeCmd) {
		tm, _ := m.Update(msg)
		m = tm.(Model)
	}
	frame := popupText(m)
	if rowOf(frame, "! Earth") == "" {
		t.Errorf("the result for Earth did not mark Earth:\n%s", frame)
	}
	if r := rowOf(frame, "Sun"); strings.Contains(r, "!") {
		t.Errorf("Earth's result marked the row under the cursor: %q", r)
	}
	if strings.Contains(frame, "can't be drawn") {
		t.Errorf("Earth's result printed a status line while the cursor is on Sun:\n%s", frame)
	}
}

// C5: a probe in flight blocks neither the keys nor Esc, and Esc cancels it.
func TestProbeNeverBlocksThePopup(t *testing.T) {
	// start opens the popup, lets the cursor rest on Earth and leaves that
	// probe blocked in flight, running on its own goroutine.
	start := func(t *testing.T) (Model, *probeRecorder, chan []tea.Msg) {
		rec := stubProbe(t, nil)
		rec.gate = make(chan struct{})
		rec.entered = make(chan int, 4)
		t.Cleanup(func() { close(rec.gate) })
		m, cmd := openProbePopup(t, probeModel(t))
		var probeCmd tea.Cmd
		for _, msg := range flatten(cmd) {
			// Update runs bounded: a probe made inside the handler would block
			// it on the gate, and this must fail by name, not hang the suite.
			type step struct {
				m Model
				c tea.Cmd
			}
			got := make(chan step, 1)
			go func(m Model) { tm, c := m.Update(msg); got <- step{tm.(Model), c} }(m)
			select {
			case s := <-got:
				m, probeCmd = s.m, s.c
			case <-time.After(5 * time.Second):
				t.Fatal("the rest ran the probe inside the update: the popup waited on Astrolog")
			}
		}
		done := make(chan []tea.Msg, 1)
		go func() { done <- flatten(probeCmd) }()
		select {
		case <-rec.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("the probe never started")
		}
		return m, rec, done
	}
	pressQuickly := func(t *testing.T, m Model, k tea.KeyMsg) Model {
		t.Helper()
		start := time.Now()
		m, _ = probeKey(m, k)
		if time.Since(start) > 200*time.Millisecond {
			t.Fatalf("%v waited on the probe", k.Type)
		}
		return m
	}
	t.Run("cursor keys and toggles act in the key-press frame", func(t *testing.T) {
		m, rec, done := start(t)
		for _, k := range []tea.KeyMsg{{Type: tea.KeySpace}, {Type: tea.KeyDown}, {Type: tea.KeyUp}} {
			m = pressQuickly(t, m, k)
		}
		if m.startup != startupBodyOpen {
			t.Fatal("a key closed the popup")
		}
		select {
		case <-done: // moving on cancelled the probe that was in flight
		case <-time.After(5 * time.Second):
			t.Fatal("moving the cursor did not release the probe in flight")
		}
		if rec.ctxs[0].Err() == nil {
			t.Error("the probe left behind was not cancelled")
		}
	})
	t.Run("Esc closes at once and cancels the probe", func(t *testing.T) {
		m, rec, done := start(t)
		m = pressQuickly(t, m, tea.KeyMsg{Type: tea.KeyEsc})
		if m.startup == startupBodyOpen {
			t.Fatal("Esc did not close the popup while a probe was running")
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("closing the popup did not cancel the probe in flight")
		}
		if rec.ctxs[0].Err() == nil {
			t.Error("the probe's context was not cancelled")
		}
	})
}

// C6: a result is reused for the same body and inputs within one opening, and
// forgotten when the popup closes or an input changes.
func TestProbeResultIsReusedForTheSession(t *testing.T) {
	rec := stubProbe(t, undrawable([]string{"se99999s.se1"}, 0))
	m, cmd := openProbePopup(t, probeModel(t))
	m = pump(t, m, cmd)
	m, cmd = probeKey(m, tea.KeyMsg{Type: tea.KeyDown})
	m = pump(t, m, cmd)
	m, cmd = probeKey(m, tea.KeyMsg{Type: tea.KeyUp})
	m = pump(t, m, cmd)
	if got := rec.objects(); !reflect.DeepEqual(got, []int{0, 1}) {
		t.Fatalf("returning to Earth probed again: %v, want [0 1]", got)
	}
	// An input changes (the floating time was stepped): the same body probes again.
	m.targetFloatingTime = m.targetFloatingTime.Add(time.Hour)
	m, cmd = probeKey(m, tea.KeyMsg{Type: tea.KeyDown})
	m = pump(t, m, cmd)
	m, cmd = probeKey(m, tea.KeyMsg{Type: tea.KeyUp})
	m = pump(t, m, cmd)
	if got := rec.objects(); !reflect.DeepEqual(got, []int{0, 1, 1, 0}) {
		t.Fatalf("after the time changed, probed %v, want Sun and Earth again", got)
	}
	// Closing forgets everything.
	m, _ = probeKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, cmd = openProbePopup(t, m)
	m = pump(t, m, cmd)
	if got := rec.objects(); len(got) != 5 || got[4] != 0 {
		t.Fatalf("after close and reopen, probed %v, want Earth probed again", got)
	}
}

// C7 (N8, N9): a failed probe marks nothing and says one plain line.
func TestFailedProbeMarksNothing(t *testing.T) {
	t.Run("a refused call logs one plain line and probes again on the next rest", func(t *testing.T) {
		rec := stubProbe(t, func(int) (lookup.BodyProbe, error) {
			return lookup.BodyProbe{}, errors.New("astrolog warned: SECRET-STDERR argv -R0 0 -zi Client")
		})
		m, cmd := openProbePopup(t, probeModel(t))
		m = pump(t, m, cmd)
		if n := len(m.logLines); n != 1 || !strings.Contains(m.logLines[0].text, "Earth") {
			t.Fatalf("log = %v, want one line naming Earth", m.logLines)
		}
		for _, bad := range []string{"SECRET-STDERR", "-R0", "-zi", "Client", "astrolog warned"} {
			if logMentions(m, bad) {
				t.Errorf("the Log line leaks %q: %v", bad, m.logLines)
			}
		}
		if strings.Contains(popupText(m), "!") {
			t.Errorf("a failed probe marked a row:\n%s", popupText(m))
		}
		m, cmd = probeKey(m, tea.KeyMsg{Type: tea.KeyDown})
		m, cmd2 := probeKey(m, tea.KeyMsg{Type: tea.KeyUp})
		_ = cmd
		m = pump(t, m, cmd2)
		if got := rec.objects(); len(got) < 2 || got[len(got)-1] != 0 {
			t.Fatalf("resting on Earth again did not probe again: %v", got)
		}
	})
	t.Run("a cancelled probe is silent", func(t *testing.T) {
		stubProbe(t, func(int) (lookup.BodyProbe, error) { return lookup.BodyProbe{}, context.Canceled })
		m, cmd := openProbePopup(t, probeModel(t))
		m = pump(t, m, cmd)
		if len(m.logLines) != 0 {
			t.Errorf("a cancellation logged %v", m.logLines)
		}
	})
	t.Run("an unresolved place runs no call", func(t *testing.T) {
		rec := stubProbe(t, nil)
		m := probeModel(t)
		m.place.TZ = ""
		m, cmd := openProbePopup(t, m)
		m = pump(t, m, cmd)
		if rec.count() != 0 || len(m.logLines) != 0 {
			t.Errorf("calls %d, log %v; want none", rec.count(), m.logLines)
		}
	})
	t.Run("a zone that does not load runs no call", func(t *testing.T) {
		rec := stubProbe(t, nil)
		m := probeModel(t)
		m.place.TZ = "Invalid/Zone"
		m, cmd := openProbePopup(t, m)
		m = pump(t, m, cmd)
		if rec.count() != 0 || len(m.logLines) != 0 {
			t.Errorf("calls %d, log %v; want none", rec.count(), m.logLines)
		}
	})
	t.Run("an error that finished before the cursor moved on, delivered after, is silent", func(t *testing.T) {
		stubProbe(t, func(int) (lookup.BodyProbe, error) {
			return lookup.BodyProbe{}, errors.New("refused")
		})
		m, cmd := openProbePopup(t, probeModel(t))
		var probeCmd tea.Cmd
		for _, msg := range flatten(cmd) {
			tm, c := m.Update(msg)
			m, probeCmd = tm.(Model), c
		}
		late := flatten(probeCmd) // finished with an error; not yet delivered
		if len(late) == 0 {
			t.Fatal("the rest started no probe")
		}
		m, _ = probeKey(m, tea.KeyMsg{Type: tea.KeyDown})
		for _, msg := range late {
			tm, _ := m.Update(msg)
			m = tm.(Model)
		}
		if len(m.logLines) != 0 {
			t.Errorf("a probe the cursor had moved on from logged %v", m.logLines)
		}
	})
	t.Run("an unresolved house system runs no call", func(t *testing.T) {
		rec := stubProbe(t, nil)
		m := listModel(t, 2, 1)
		m.objectRegistry = probeRegistry()
		m.frames.failed = map[string]string{slotFile(m.focusedSlot()): "unreadable"}
		if m.unresolvedFor(m.focusedSlot()) == nil {
			t.Fatal("fixture: the focused slot's houses are not unresolved")
		}
		m, cmd := openProbePopup(t, m)
		m = pump(t, m, cmd)
		if rec.count() != 0 || len(m.logLines) != 0 {
			t.Errorf("calls %d, log %v; want none", rec.count(), m.logLines)
		}
	})
}

// C8: probe activity changes nothing but the marks.
func TestProbeChangesNothingElse(t *testing.T) {
	rec := stubProbe(t, undrawable([]string{"se99999s.se1"}, 0, 1))
	m := probeModel(t)
	m.lens.Objects.Selection = []int{1, 2}
	ticks := func(m Model) []int { return m.lens.Objects.Selected() }
	// The session file in this test's own HOME: probe activity must not write it.
	sessionBytes := func() string {
		b, err := os.ReadFile(store.SessionPath())
		if err != nil {
			return "absent:" + err.Error()
		}
		return string(b)
	}
	sessionBefore := sessionBytes()
	chartBefore := m.chart
	lensBefore := m.lens
	m, cmd := openProbePopup(t, m)
	entry := ticksOn(m.bodyModal.selected)
	m = pump(t, m, cmd)
	// A second probe is held back, to be delivered after Esc.
	m, cmd = probeKey(m, tea.KeyMsg{Type: tea.KeyDown})
	var probeCmd tea.Cmd
	for _, msg := range flatten(cmd) {
		tm, c := m.Update(msg)
		m, probeCmd = tm.(Model), c
	}
	late := flatten(probeCmd)
	if rec.count() != 2 || len(late) == 0 {
		t.Fatalf("probes run = %d, late results = %d; want Earth and Sun probed, Sun's held back", rec.count(), len(late))
	}
	if !reflect.DeepEqual(ticksOn(m.bodyModal.selected), entry) {
		t.Errorf("the selection changed under probe activity: %v -> %v", entry, ticksOn(m.bodyModal.selected))
	}
	m, _ = probeKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	for _, msg := range late { // delivered after Esc
		tm, c := m.Update(msg)
		m = tm.(Model)
		if c != nil {
			t.Error("a result after Esc scheduled work")
		}
	}
	if !reflect.DeepEqual(m.chart, chartBefore) {
		t.Error("the Bodies panel / chart changed")
	}
	if !reflect.DeepEqual(ticks(m), lensBefore.Objects.Selected()) {
		t.Errorf("the lens selection changed: %v", ticks(m))
	}
	if sessionBytes() != sessionBefore {
		t.Error("probe activity wrote the session file")
	}
	if len(m.logLines) != 0 {
		t.Errorf("probe activity logged %v", m.logLines)
	}
}

// N6: a result that arrives after Esc and a reopen touches nothing.
func TestLateProbeAfterReopenChangesNothing(t *testing.T) {
	rec := stubProbe(t, undrawable([]string{"se99999s.se1"}, 0))
	m, cmd := openProbePopup(t, probeModel(t))
	var probeCmd tea.Cmd
	for _, msg := range flatten(cmd) {
		tm, c := m.Update(msg)
		m, probeCmd = tm.(Model), c
	}
	late := flatten(probeCmd) // the result, held back
	if len(late) == 0 {
		t.Fatal("the rest started no probe, so there is no late result to deliver")
	}
	m, _ = probeKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	// A late result with the popup closed.
	for _, msg := range late {
		tm, _ := m.Update(msg)
		m = tm.(Model)
	}
	if len(m.logLines) != 0 {
		t.Errorf("a late result logged %v", m.logLines)
	}
	// Reopen, then deliver it again: it belongs to the previous opening.
	m, _ = openProbePopup(t, m)
	before := popupText(m)
	for _, msg := range late {
		tm, c := m.Update(msg)
		m = tm.(Model)
		if c != nil {
			t.Errorf("a stale result scheduled work")
		}
	}
	if after := popupText(m); after != before || len(m.logLines) != 0 {
		t.Errorf("a stale result changed the new popup:\n%s\nlog %v", after, m.logLines)
	}
	_ = rec
}
