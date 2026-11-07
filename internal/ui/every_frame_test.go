package ui

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// specs/details-follows-its-tab.md C10, the every-frame test.
//
// A deterministic, seeded corpus runs the Model through a VIRTUAL scheduler
// against a STAND-IN Astrolog, and holds every frame to the view rule.
//
//   - The stand-in answers every cast, report, search and fact set at once, but
//     says in the text itself which subject it answered for (`SUBJECT <kind>
//     <object> <chart>`, and a different Sun for each chart), so a frame can be
//     read for whose content it draws. Whether a call fails is a pure function
//     of the seed, the call and the phase the world is in, so a retry of the
//     same call in the same phase fails the same way.
//   - The scheduler holds the answer of every command for a seeded number of
//     steps and delivers the held messages in a seeded order, so a result
//     arrives late, early or after the next key. A timer is a zero-length
//     debounce in this world (its rows are set to 0); a long timer is dropped.
//   - Inputs are the entry points the spec lists, each with its own coverage
//     counters; the test fails when a counter is zero, and it fails when a
//     frame draws a caption over another subject's content, when any pane
//     breaks C3-C5, when a Details report breaks C6, and when the world is not
//     settled 100 delivered messages after input stops.

// chaosSeed and the corpus size: 14 sequences of 36 entry points each (504),
// about 3,000 delivered messages, a few seconds.
const (
	chaosSeed      = 20261105
	chaosSequences = 10
	chaosPerSeq    = 30
	chaosSettle    = 100
	// idleRestSteps is how many steps of the virtual clock the idle rest takes:
	// a stand-in answers within four.
	idleRestSteps = 5
	// idleBound is the most messages of its own the idle prefetch may deliver
	// after input stopped: one period, a call and a result each, and the polls
	// of a rest that waited for a busy surface.
	idleBound = 200
)

// chaosRequired are the entry points C10 names, each with its own counters.
var chaosRequired = []string{
	"focus digits", "left/right", "tab", "up/down", "pgup/pgdn", "d/D", "n/N",
	"space on the Chart", "space on Bodies", "0", "enter/esc", ",l", "l",
	"digit from the Log", "digit from the large log",
	"body chooser preview", "body chooser confirm", "aspect chooser preview", "aspect chooser confirm",
	"resize", "minute tick",
}

// chaosStates are the pane states (C3 current, C4 updating, C5 empty, C6
// failed) the corpus must draw at least once. Bodies and Aspects have no C4: the
// main panels never draw a faint version (operator, 2026-11-06).
var chaosStates = []string{
	"details/current", "details/updating", "details/empty", "details/failed",
	"bodies/current", "bodies/empty",
	"aspects/current", "aspects/empty",
	"timeline/current", "timeline/empty", "template/current", "template/empty",
}

// ---- the stand-in Astrolog ------------------------------------------------

type standIn struct {
	seed  uint64
	phase atomic.Uint64
	calls atomic.Int64
	fails atomic.Int64
	week  []lookup.TimelineRow
}

// failsFor says whether the call tagged tag fails in the phase the world is in:
// a hash of the seed, the tag and the phase, per cent (a storm phase raises it).
func (s *standIn) failsFor(tag string, percent uint32) bool {
	h := fnv.New32a()
	fmt.Fprintf(h, "%d|%d|%s", s.seed, s.phase.Load(), tag)
	s.calls.Add(1)
	if s.phase.Load()%3 == 2 {
		percent = 60 // every third phase is a storm
	}
	if h.Sum32()%100 < percent {
		s.fails.Add(1)
		return true
	}
	return false
}

var errStandIn = errors.New("astrolog stopped")

// whoYear names a chart by the year of its moment: the floating chart is the
// sky of 2026, an anchored one a natal of the 1960s.
func whoYear(year int) string {
	if year >= 2000 {
		return "floating"
	}
	return fmt.Sprintf("natal%d", year)
}

func whoSlot(s *store.Slot) string {
	if s == nil {
		return ""
	}
	if s.Floating {
		return "floating"
	}
	return whoYear(s.Time.Year())
}

// sunOf is the Sun's longitude for a chart: a different sign for each.
func sunOf(year int) float64 {
	if year >= 2000 {
		return 35.5
	}
	return float64((year-1960)*95%360) + 5.5
}

func standInPositions(year int) []aspect.Position {
	sun := sunOf(year)
	return []aspect.Position{
		{Object: objSun, Longitude: sun},
		{Object: objMoon, Longitude: sun + 80},
		{Object: objMercury, Longitude: sun + 15},
		{Object: objVenus, Longitude: sun + 40},
		{Object: objMars, Longitude: sun + 200},
	}
}

func (s *standIn) install(t *testing.T) {
	stubSpawns(t, func(r *rowSpawns) {
		r.cast.transit = func(_ context.Context, _ *lookup.Probers, now time.Time, _ location.PlaceInfo, lens lookup.Lens) (lookup.Result, error) {
			if s.failsFor(fmt.Sprintf("transit|%d|%v", now.UnixNano(), lens.Houses), 12) {
				return lookup.Result{}, errStandIn
			}
			return lookup.Result{Positions: standInPositions(now.Year()), AspectsReport: "SUBJECT aspects floating\n"}, nil
		}
		r.cast.natal = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, lens lookup.Lens) (lookup.Result, error) {
			if s.failsFor(fmt.Sprintf("natal|%d|%v", c.When.UnixNano(), lens.Houses), 12) {
				return lookup.Result{}, errStandIn
			}
			return lookup.Result{Positions: standInPositions(c.When.Year()), AspectsReport: "SUBJECT aspects " + whoYear(c.When.Year()) + "\n"}, nil
		}
		r.cast.overlay = func(_ context.Context, _ lookup.Lens, natal store.AnchorChart, when time.Time, _ location.PlaceInfo) (lookup.OverlayAspects, error) {
			if s.failsFor(fmt.Sprintf("overlay|%d|%d", natal.When.UnixNano(), when.UnixNano()), 9) {
				return lookup.OverlayAspects{}, errStandIn
			}
			return lookup.OverlayAspects{Report: "SUBJECT cross " + whoYear(natal.When.Year()) + ">" + whoYear(when.Year()) + "\n"}, nil
		}
		r.cast.synastry = func(_ context.Context, _ lookup.Lens, natal store.AnchorChart, when time.Time, _ location.PlaceInfo, _ store.ReportLabel) (lookup.OverlayAspects, error) {
			if s.failsFor(fmt.Sprintf("synastry|%d|%d", natal.When.UnixNano(), when.UnixNano()), 9) {
				return lookup.OverlayAspects{}, errStandIn
			}
			return lookup.OverlayAspects{Report: "SUBJECT cross " + whoYear(natal.When.Year()) + ">" + whoYear(when.Year()) + "\n"}, nil
		}
		r.reports.raw = func(_ context.Context, when time.Time, _ location.PlaceInfo, lens lookup.Lens, req lookup.RawReportRequest) (string, string, error) {
			if s.failsFor(fmt.Sprintf("raw|%d|%d|%d|%v|%v", when.UnixNano(), req.Kind, req.Object.Number, lens.Houses, lens.Aspects.Indices), 22) {
				return "", "", errStandIn
			}
			who := whoYear(when.Year())
			if req.Kind == lookup.RawReportChart {
				return influenceHead + "\nSUBJECT chart - " + who + "\n", "", nil
			}
			return fmt.Sprintf("SUBJECT body %s %s\nreport at %s\n", panel.BodyName(req.Object), who, when.Format("15:04 Jan 2 2006")), "", nil
		}
		r.reports.cross = func(_ context.Context, lens lookup.Lens, natal store.AnchorChart, when time.Time, _ location.PlaceInfo, o aspect.Object) (string, string, error) {
			if s.failsFor(fmt.Sprintf("cross|%d|%d|%d|%v", natal.When.UnixNano(), when.UnixNano(), o.Number, lens.Houses), 22) {
				return "", "", errStandIn
			}
			return fmt.Sprintf("SUBJECT body %s %s>%s\n", panel.BodyName(o), whoYear(natal.When.Year()), whoYear(when.Year())), "", nil
		}
		r.reports.synastry = func(_ context.Context, lens lookup.Lens, natal store.AnchorChart, when time.Time, _ location.PlaceInfo, _ store.ReportLabel, o aspect.Object) (string, string, error) {
			if s.failsFor(fmt.Sprintf("syn|%d|%d|%d|%v", natal.When.UnixNano(), when.UnixNano(), o.Number, lens.Houses), 22) {
				return "", "", errStandIn
			}
			return fmt.Sprintf("SUBJECT body %s %s>%s\n", panel.BodyName(o), whoYear(natal.When.Year()), whoYear(when.Year())), "", nil
		}
		r.timeline = func(_ context.Context, req lookup.TimelineRequest) (lookup.TimelineResult, error) {
			if s.failsFor(fmt.Sprintf("timeline|%d|%d|%d", req.When.Year(), req.When.Month(), len(req.Natals)), 18) {
				return lookup.TimelineResult{Commands: []string{"astrolog -dm"}}, errStandIn
			}
			return lookup.TimelineResult{Rows: s.week}, nil
		}
		r.bodyFacts = func(_ context.Context, req lookup.BodyFactsRequest) (lookup.BodyFactsResult, error) {
			if s.failsFor(fmt.Sprintf("facts|%d|%d", req.When.UnixNano(), req.Object.Number), 18) {
				return lookup.BodyFactsResult{}, errStandIn
			}
			return lookup.BodyFactsResult{Now: researchSkyFacts()}, nil
		}
	})
}

// ---- the virtual scheduler ------------------------------------------------

type held struct {
	msg tea.Msg
	due int
}

// resultErr is the error a surface result carries, and whether msg is a result.
func resultErr(msg tea.Msg) (err error, isResult bool) {
	switch r := msg.(type) {
	case refreshResultMsg:
		return r.err, true
	case FirstFrameReadyMsg:
		return r.Err, true
	}
	return nil, false
}

type cover struct{ applied, changed, outstanding, failing int }

type world struct {
	t         *testing.T
	rng       *rand.Rand
	astrolog  *standIn
	m         Model
	held      []held
	step      int
	delivered int
	checked   int
	// seen counts the frames each pane drew in each state.
	seen map[string]int
	cov  map[string]*cover
	// active is the entry point whose window is open: from its input until the
	// next input.
	active     string
	activeSeen bool
	activeFail bool
	// prefetchCalls counts the prefetch calls the model started.
	prefetchCalls int
	// prefetchDelivered counts the idle prefetch's own messages (the rest's end
	// and each call's result) delivered so far. They are held to a bound of their
	// own, idleBound: a period ends, and its calls are not what the panes wait for.
	prefetchDelivered int
	// worstSettle and worstIdle are the most messages of the panes' own, and of the
	// prefetch's, any sequence needed after input stopped.
	worstSettle, worstIdle int
}

func (w *world) outstanding() bool {
	for _, h := range w.held {
		if _, ok := resultErr(h.msg); ok {
			return true
		}
	}
	for s := range w.m.refresh {
		if w.m.refresh[s].cancel != nil {
			return true
		}
	}
	return false
}

func (w *world) failingHeld() bool {
	for _, h := range w.held {
		if err, ok := resultErr(h.msg); ok && err != nil {
			return true
		}
	}
	return false
}

// exec runs a command to completion, in order, and holds what it answers. A
// long timer (the leader's, the minute's) is dropped; a batch is its members.
func (w *world) exec(c tea.Cmd) {
	if c == nil {
		return
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- c() }()
	deadline := time.After(patientWait)
	for {
		select {
		case msg := <-ch:
			switch msg := msg.(type) {
			case nil:
			case tea.BatchMsg:
				for _, sub := range msg {
					w.exec(sub)
				}
			default:
				delay := 0
				if _, isResult := resultErr(msg); isResult {
					delay = w.rng.Intn(5)
				} else {
					delay = w.rng.Intn(3)
				}
				if _, isRest := msg.(prefetchIdleMsg); isRest {
					delay = idleRestSteps
				}
				w.held = append(w.held, held{msg: msg, due: w.step + delay})
			}
			return
		case <-time.After(5 * time.Millisecond):
			if longTimersSleeping.Load() > 0 {
				abandonLongTimers()
			}
		case <-deadline:
			w.t.Fatalf("a command never returned")
		}
	}
}

// deliver runs one held message that is due through Update and holds what that
// starts. It reports false when nothing is due; time then moves on by a step.
func (w *world) deliver() bool {
	var due []int
	for i, h := range w.held {
		if h.due <= w.step {
			due = append(due, i)
		}
	}
	if len(due) == 0 {
		w.step++
		return false
	}
	i := due[w.rng.Intn(len(due))]
	msg := w.held[i].msg
	w.held = append(w.held[:i], w.held[i+1:]...)
	if err, ok := resultErr(msg); ok && err != nil && w.active != "" && !w.activeFail {
		w.activeFail = true
		w.cov[w.active].failing++
	}
	switch msg.(type) {
	case prefetchIdleMsg, prefetchDoneMsg:
		w.prefetchDelivered++
	}
	w.update(msg)
	w.delivered++
	return true
}

func (w *world) update(msg tea.Msg) {
	tm, cmd := w.m.Update(msg)
	w.m = tm.(Model)
	w.prefetchCalls = w.m.pre.calls
	w.exec(cmd)
	w.checkFrame(fmt.Sprintf("%T", msg))
}

// ---- the entry points -----------------------------------------------------

type entry struct {
	name string
	// modal says the entry point acts while a chooser is open; the others act
	// only while none is.
	modal bool
	// inert says the entry point starts no work and changes no wanted key by
	// itself (the Log is a mode beside the ring, never m.focused): it is
	// covered by being applied while a run is outstanding and while one
	// fails, and the test asserts that it changed nothing.
	inert bool
	// log says the entry point works the Log or the console.
	log bool
	// act sets the scene (messages sent as the operator would, not counted)
	// and returns the message of the entry point itself.
	act func(w *world) tea.Msg
}

func choose[T any](w *world, xs ...T) T { return xs[w.rng.Intn(len(xs))] }

var chaosSizes = [][2]int{{170, 34}, {170, 34}, {150, 34}, {132, 34}, {100, 28}, {80, 24}}

// focus brings the focus to panel p with its digit.
func (w *world) focus(p PanelID, digit string) {
	if w.m.focused != p {
		w.send(runes(digit))
	}
}

// toFloating puts the Chart cursor on the floating chart.
func (w *world) toFloating() {
	w.focus(PanelChart, "1")
	for i := 0; i < 5 && !w.m.focusedIsFloating(); i++ {
		w.send(keyDown)
	}
}

// toTab makes id the active Details tab, pressing 0 as the operator would.
func (w *world) toTab(id TabID) {
	for i := 0; i < 4 && w.m.activeTabID(PanelDetails) != id; i++ {
		w.send(runes("0"))
	}
}

// toVenus puts the Bodies cursor on Venus, which has a template tab.
func (w *world) toVenus() {
	w.focus(PanelBodies, "2")
	for i := 0; i < 6; i++ {
		ch := w.m.shownChart()
		if o, ok := panel.ObjectAtCursor(ch.positions, ch.cusps, w.m.bodyCursorIdx); ok && o.Number == objVenus.Number {
			return
		}
		w.send(keyDown)
	}
}

func (w *world) entries() []entry {
	openLog := func(w *world) {
		if !w.m.logFocus.active {
			w.send(runes(","))
			w.send(runes("l"))
		}
	}
	return []entry{
		{name: "focus digits", act: func(w *world) tea.Msg { return runes(choose(w, "1", "2", "3")) }},
		{name: "left/right", act: func(w *world) tea.Msg { return choose[tea.Msg](w, keyLeft, keyRight) }},
		{name: "tab", act: func(w *world) tea.Msg { return keyTab }},
		{name: "up/down", act: func(w *world) tea.Msg { return choose[tea.Msg](w, keyUp, keyDown) }},
		{name: "pgup/pgdn", act: func(w *world) tea.Msg { return choose[tea.Msg](w, keyPgUp, keyPgDn) }},
		{name: "d/D", act: func(w *world) tea.Msg { return runes(choose(w, "d", "D")) }},
		{name: "n/N", act: func(w *world) tea.Msg { return runes(choose(w, "n", "N")) }},
		{name: "space on the Chart", act: func(w *world) tea.Msg {
			w.focus(PanelChart, "1")
			for k := w.rng.Intn(4); k > 0; k-- {
				w.send(choose[tea.Msg](w, keyUp, keyDown)) // any chart: the selection it joins or leaves differs
			}
			return keySpace
		}},
		{name: "space on Bodies", act: func(w *world) tea.Msg {
			w.focus(PanelBodies, "2")
			return keySpace
		}},
		{name: "0", act: func(w *world) tea.Msg {
			if w.rng.Intn(2) == 0 {
				w.toVenus()
			} else {
				w.toFloating()
			}
			return runes("0")
		}},
		{name: "enter/esc", act: func(w *world) tea.Msg {
			if !w.m.details.attached {
				w.toFloating()
				w.toTab(TabDetailsTimeline)
				w.send(keyEnter) // attach
			}
			for i := 0; i < 30 && w.m.timelineWaiting(); i++ {
				w.deliver() // the list the Enter would jump to
			}
			if len(w.m.timelineShown()) > 0 && w.rng.Intn(3) > 0 {
				for k := w.rng.Intn(8); k > 0; k-- {
					w.send(keyDown) // another row than the last jump's
				}
				w.send(runes("d")) // a frame and a report are out when the jump arrives
				return keyEnter    // a row: the floating moment jumps to it
			}
			return keyEsc
		}},
		{name: ",l", inert: true, log: true, act: func(w *world) tea.Msg {
			if w.m.logFocus.active || w.m.console.open {
				w.send(keyEsc)
			}
			w.send(runes(",")) // arms the leader; `l` completes it
			return runes("l")
		}},
		{name: "l", inert: true, log: true, act: func(w *world) tea.Msg {
			openLog(w)
			return runes("l")
		}},
		{name: "digit from the Log", log: true, act: func(w *world) tea.Msg {
			openLog(w)
			return runes(choose(w, "1", "2", "3"))
		}},
		{name: "digit from the large log", log: true, act: func(w *world) tea.Msg {
			openLog(w)
			w.send(runes("l"))
			return runes(choose(w, "1", "2", "3"))
		}},
		{name: "body chooser preview", modal: true, act: func(w *world) tea.Msg {
			if w.m.startup != startupBodyOpen {
				w.send(tea.KeyMsg{Type: tea.KeyCtrlB})
			}
			return keySpace
		}},
		{name: "body chooser confirm", modal: true, act: func(w *world) tea.Msg {
			if w.m.startup != startupBodyOpen {
				w.send(tea.KeyMsg{Type: tea.KeyCtrlB})
				w.send(keySpace)
			}
			return choose[tea.Msg](w, keyEnter, keyEsc)
		}},
		{name: "aspect chooser preview", modal: true, act: func(w *world) tea.Msg {
			if w.m.startup != startupAspectOpen {
				w.send(tea.KeyMsg{Type: tea.KeyCtrlA})
			}
			return keySpace
		}},
		{name: "aspect chooser confirm", modal: true, act: func(w *world) tea.Msg {
			if w.m.startup != startupAspectOpen {
				w.send(tea.KeyMsg{Type: tea.KeyCtrlA})
				w.send(keySpace)
			}
			return choose[tea.Msg](w, keyEnter, keyEsc)
		}},
		{name: "resize", modal: true, act: func(w *world) tea.Msg {
			s := choose(w, chaosSizes...)
			return tea.WindowSizeMsg{Width: s[0], Height: s[1]}
		}},
		{name: "minute tick", modal: true, act: func(w *world) tea.Msg {
			if w.m.floatingDetached {
				w.send(runes("n")) // the floating chart follows the clock again
			}
			clock := w.m.clock.(*fakeClock)
			clock.advanceTo(clock.t.Add(time.Minute))
			return nowTickMsg{gen: w.m.nowTickGen}
		}},
	}
}

// closing is the entry point that closes the chooser the startup phase names.
func closing(entries []entry, startup startupState) entry {
	name := "aspect chooser confirm"
	if startup == startupBodyOpen {
		name = "body chooser confirm"
	}
	for _, e := range entries {
		if e.name == name {
			return e
		}
	}
	panic("no closing entry point")
}

// wanted is everything the Model asks for now, one string: a change of it is an
// entry point changing a wanted key.
func (w *world) wanted() string {
	m := w.m
	var b strings.Builder
	if k, ok := m.textKeyFor(); ok {
		fmt.Fprintf(&b, "%+v|", k)
	}
	d := m.detailsWanted()
	fmt.Fprintf(&b, "%s|%v|%+v|", d.caption, d.has, d.key)
	if k, ok := m.timelineKeyFor(); ok {
		fmt.Fprintf(&b, "%+v|", k)
	}
	if k, ok := m.templateKeyFor(); ok {
		fmt.Fprintf(&b, "%+v|", k)
	}
	fmt.Fprintf(&b, "%v|%s", m.detailsVisible(), m.activeTabID(PanelDetails))
	return b.String()
}

// input delivers one entry point's messages: its setup, then itself, and notes
// what it did for the coverage counters.
func (w *world) input(e entry) {
	// The Log and the console hold the keyboard until left: an entry point that
	// is not about them leaves first, as the operator would.
	if !e.log && (w.m.logFocus.active || w.m.console.open) {
		w.send(keyEsc)
	}
	// A small terminal is visited, not lived in.
	if w.m.width < 150 && w.rng.Intn(3) > 0 {
		w.send(tea.WindowSizeMsg{Width: 170, Height: 34})
	}
	in := e.act(w)
	w.cov[e.name].applied++
	before, outstanding, failing := w.wanted(), w.outstanding(), w.failingHeld()
	w.send(in)
	w.active, w.activeSeen, w.activeFail = e.name, false, false
	changed := w.wanted() != before
	if e.inert {
		if changed {
			w.t.Fatalf("%q changed a wanted key; the Log is a mode beside the ring and must not\nbefore %s\nafter  %s", e.name, before, w.wanted())
		}
	} else if !changed {
		w.active = ""
		return
	}
	c := w.cov[e.name]
	c.changed++
	if outstanding {
		c.outstanding++
	}
	if failing {
		c.failing++
		w.activeFail = true
	}
}

// send is one message from the operator: Update, then its commands. Keys that
// open a chooser drop the probe's rest timer, which is real time.
func (w *world) send(msg tea.Msg) {
	tm, cmd := w.m.Update(msg)
	w.m = tm.(Model)
	if k, ok := msg.(tea.KeyMsg); ok && k.Type == tea.KeyCtrlB {
		cmd = nil
	}
	w.exec(cmd)
	w.checkFrame(fmt.Sprintf("%T %v", msg, msg))
}

// ---- the frames -----------------------------------------------------------

var subjectMarker = regexp.MustCompile(`SUBJECT [a-z]+ \S+(?: \S+)?`)

// expectedMarker is the marker the Details report or the Aspects copy must
// carry for the subject the caption names, read from the focus and the scene
// alone (not from the view rule).
func expectedMarker(m Model) string {
	sc := m.deriveScene()
	switch m.focused {
	case PanelBodies:
		ch := m.shownChart()
		o, ok := panel.ObjectAtCursor(ch.positions, ch.cusps, m.bodyCursorIdx)
		if !ok {
			return ""
		}
		if sc.pair != nil {
			return "SUBJECT body " + panel.BodyName(o) + " " + whoSlot(sc.pair.left) + ">" + whoSlot(sc.pair.right)
		}
		return "SUBJECT body " + panel.BodyName(o) + " " + whoSlot(m.focusedSlot())
	case PanelAspects:
		if sc.pair != nil {
			return "SUBJECT cross " + whoSlot(sc.pair.left) + ">" + whoSlot(sc.pair.right)
		}
		return "SUBJECT aspects " + whoSlot(m.focusedSlot())
	}
	return "SUBJECT chart - " + whoSlot(m.focusedSlot())
}

// panelText is the text of the rendered panel p, cut out of the plain screen.
func panelText(m Model, plain []string, p PanelID) string {
	r, ok := m.geometry()[p]
	if !ok {
		return ""
	}
	var out []string
	for i := r.y; i < r.y+r.h && i < len(plain); i++ {
		l := []rune(plain[i])
		if r.x >= len(l) {
			continue
		}
		out = append(out, string(l[r.x:min(r.x+r.w, len(l))]))
	}
	return strings.Join(out, "\n")
}

// rendered says the panel is in the layout.
func rendered(m Model, p PanelID) bool { _, ok := m.geometry()[p]; return ok }

// checkFrame holds the frame on screen to the view rule, C3-C5 for every
// rendered pane and C6 for a Details report, read from the keys and from the
// content the stand-in marked. It runs after every delivered message.
func (w *world) checkFrame(where string) {
	t, m := w.t, w.m
	t.Helper()
	w.checked++
	if m.width == 0 || m.height == 0 || m.geometry() == nil {
		return
	}
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf("frame %d after %s: %s\n%s", w.checked, where, fmt.Sprintf(format, args...), ansi.Strip(m.View()))
	}
	// The plain screen, drawn once for the frame and only if a check needs it.
	var plain []string
	screen := func() []string {
		if plain == nil {
			plain = strings.Split(ansi.Strip(m.View()), "\n")
		}
		return plain
	}

	// Bodies and Aspects: one text frame, each pane's key from it.
	for _, p := range []PanelID{PanelBodies, PanelAspects} {
		if !rendered(m, p) || !m.shown.stamped {
			continue
		}
		want, okW := m.wantedPaneKey(p)
		shown, okS := m.shownPaneKey(p)
		var expect paneState
		switch {
		case !okW:
			continue
		case !okS:
			expect = paneEmpty
		case shown.subject() == want.subject():
			expect = paneCurrent // the main panels never draw a faint version
		default:
			expect = paneEmpty
		}
		if got := m.textPaneState(p); got != expect {
			fail("%v draws state %v, its keys say %v (shown %+v, wanted %+v)", p, got, expect, shown, want)
		}
		w.seen[fmt.Sprintf("%v/%s", p, stateName(expect))]++
		if m.modalOpen() || m.console.open {
			continue // a popup or the console covers the panel's cells; the keys were held above
		}
		text := panelText(m, screen(), p)
		if expect == paneEmpty {
			if !strings.Contains(text, emptyPaneLine) || strings.Contains(text, "☉ Sun") {
				fail("%v is empty by the rule and draws:\n%s", p, text)
			}
		}
		if p == PanelBodies && expect != paneEmpty && len(m.deriveScene().shown) <= 1 && !m.shown.comparing {
			sc := m.deriveScene()
			sun := sunOf(2000)
			if !sc.cursor.Floating {
				sun = sunOf(sc.cursor.Time.Year())
			}
			for _, l := range strings.Split(text, "\n") {
				if strings.Contains(l, "☉ Sun") && !strings.Contains(l, signOf(sun)) {
					fail("Bodies draws a Sun that is not the cursor chart's (want sign %s):\n%s", signOf(sun), text)
				}
			}
		}
	}

	// Details: the report tab (C3-C6), the Timeline and the template tab.
	if !m.detailsVisible() {
		return
	}
	text := ansi.Strip(renderDetails(m))
	switch id := m.activeTabID(PanelDetails); {
	case id == TabDetailsReport:
		wnt := m.detailsWanted()
		var expect paneState
		switch {
		case !wnt.has:
			expect = paneEmpty
		case wnt.key.kind == reportRaw && m.details.failed.set && m.details.failed.key == wnt.key.raw:
			expect = paneFailed
		case m.details.keyed && m.details.key == wnt.key:
			expect = paneCurrent
		case m.details.keyed && m.details.key.subject() == wnt.key.subject():
			expect = paneUpdating
		default:
			expect = paneEmpty
		}
		if got := m.detailsReportState(); got != expect {
			fail("Details draws state %v, its keys say %v (shown %+v, wanted %+v)", got, expect, m.details.key, wnt.key)
		}
		w.seen["details/"+stateName(expect)]++
		marker := subjectMarker.FindString(text)
		switch expect {
		case paneEmpty:
			if !detailsBodyBlank(text) || marker != "" || strings.Contains(text, influenceHead) {
				fail("Details is empty by the rule and draws:\n%s", text)
			}
		case paneFailed:
			if !strings.Contains(text, "Couldn't read") || marker != "" || strings.Contains(text, influenceHead) {
				fail("Details is failed by the rule and draws:\n%s", text)
			}
		case paneCurrent, paneUpdating:
			want := expectedMarker(m)
			got := strings.TrimRight(strings.TrimRight(marker, "│| "), " ")
			if want == "" || got != want {
				fail("Details draws %q under the caption %q (want %q)", got, wnt.caption, want)
			}
			if updating := strings.HasSuffix(stripTitle(m), detailsPendingMark); updating != (expect == paneUpdating) {
				fail("caption %q is marked %v, state %v", stripTitle(m), updating, expect)
			}
		}
	case id == TabDetailsTimeline:
		st := m.timelinePaneState()
		w.seen["timeline/"+stateName(st)]++
		hasRows := strings.Contains(text, "/2026")
		if st == paneEmpty && (hasRows || !detailsBodyBlank(text)) {
			fail("the Timeline is empty by the rule and draws:\n%s", text)
		}
		if st == paneUpdating && !strings.HasSuffix(stripTitle(m), detailsPendingMark) {
			fail("the Timeline is another version of its subject and its caption %q is unmarked", stripTitle(m))
		}
	case isTemplateTab(id):
		st := m.templatePaneState()
		w.seen["template/"+stateName(st)]++
		if st == paneEmpty && !detailsBodyBlank(text) {
			fail("the template tab is empty by the rule and draws:\n%s", text)
		}
	}
}

func stateName(s paneState) string {
	return [...]string{"current", "updating", "empty", "failed"}[s]
}

var signNames = []string{"Ari", "Tau", "Gem", "Can", "Leo", "Vir", "Lib", "Sco", "Sag", "Cap", "Aqu", "Pis"}

func signOf(lon float64) string { return signNames[int(lon/30)%12] }

// settled holds the world, after the input stopped and its held messages were
// all delivered, to what C9 and C10 promise: every rendered pane shows its
// wanted key, or settled on an accepted failure without retry.
func (w *world) settled(label string) {
	t, m := w.t, w.m
	t.Helper()
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf("%s: %s\n%s", label, fmt.Sprintf(format, args...), ansi.Strip(m.View()))
	}
	if len(w.held) > 0 {
		fail("%d messages still held", len(w.held))
	}
	for _, p := range []PanelID{PanelBodies, PanelAspects} {
		if rendered(m, p) && m.shown.stamped {
			if st := m.textPaneState(p); st != paneCurrent {
				fail("%v settled on %v, want the frame it asks for", p, st)
			}
		}
	}
	if m.detailsVisible() && !m.reportTabHidden() {
		if wnt := m.detailsWanted(); wnt.has {
			switch st := m.detailsReportState(); {
			case st == paneCurrent || st == paneFailed:
			case st == paneEmpty && !m.chart.settled && !m.textOutstanding():
				// The frame beside the report failed: no report is cast over it.
			default:
				fail("the Details report settled on %v (shown %+v, wanted %+v)", st, m.details.key, wnt.key)
			}
		}
	}
	switch id := m.activeTabID(PanelDetails); {
	case m.detailsVisible() && id == TabDetailsTimeline:
		if _, ok := m.timelineKeyFor(); ok && m.timelineWaiting() {
			fail("the Timeline still waits")
		}
	case m.detailsVisible() && isTemplateTab(id):
		if m.templateWaiting() {
			fail("the template tab still waits")
		}
	}
	// And nothing is owed any more: the reconciler starts nothing.
	_, cmd := m.reconcile()
	if cmd != nil {
		fail("the reconciler still has something to start on a settled Model")
	}
}

// ---- the world ------------------------------------------------------------

func newWorld(t *testing.T, seed int64) *world {
	t.Helper()
	freshModels(t)
	prev := minuteTimer
	minuteTimer = func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }
	t.Cleanup(func() { minuteTimer = prev })
	// The idle rest (C15) is a message the scheduler holds like any other, for
	// idleRestSteps: longer than a stand-in call takes, shorter than the world's
	// patience. A key before it arrives makes it stale, as in the app.
	prevIdle := idleTimer
	idleTimer = func(_ time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		return func() tea.Msg { return fn(time.Now()) }
	}
	t.Cleanup(func() { idleTimer = prevIdle })
	astrolog := &standIn{seed: uint64(seed), week: weekRows(t)}
	astrolog.install(t)
	m := newTestModel(t, Config{Accessible: true}, FrozenClock(testFrozenTime()))
	clock := &fakeClock{t: testFrozenTime()}
	m.clock = clock
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.place = testPlaceBerlin()
	m.targetFloatingTime = slots.Floating().Time
	m.startup = startupReady
	m.width, m.height = 170, 34
	openChart(t, m, 1)
	openChart(t, m, 2)
	m.cursorIdx = len(m.slots.All()) - 1
	m.focused = PanelChart
	m.limitedViewShown = true
	m.aspectRegistry = testRegistry()
	m.objectRegistry = testObjectRegistry()
	m.lens.Aspects = recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2, 3, 4, 5}}
	// A timer is a zero-length debounce here: the scheduler, not the clock,
	// decides when it fires.
	for s := range m.rows {
		m.rows[s].debounce = 0
	}
	w := &world{t: t, rng: rand.New(rand.NewSource(seed)), astrolog: astrolog, m: m, cov: map[string]*cover{}, seen: map[string]int{}}
	for _, e := range w.entries() {
		w.cov[e.name] = &cover{}
	}
	var cmd tea.Cmd
	w.m, cmd = w.m.bumpIdentity(surfaceText)
	w.exec(cmd)
	w.drain(chaosSettle * 3)
	return w
}

// drain delivers everything held, in the scheduler's order, and reports how many
// messages that took, and how many of them were the prefetch's; it fails when
// the others are more than limit, or the prefetch's more than idleBound.
func (w *world) drain(limit int) (n, idle int) {
	w.t.Helper()
	start := w.prefetchDelivered
	total := 0
	for len(w.held) > 0 {
		if w.deliver() {
			total++
			idle = w.prefetchDelivered - start
			n = total - idle
			if n > limit {
				w.t.Fatalf("the world is not quiet after %d delivered messages (%d still held)", n, len(w.held))
			}
			if idle > idleBound {
				w.t.Fatalf("the idle prefetch is not over after %d of its messages (%d still held): a period must end", idle, len(w.held))
			}
		}
	}
	return n, idle
}

// session runs one sequence of entry points, then closes what is open, drains
// the world and holds it to the settle rule.
func (w *world) session(label string, entries, modals []entry, inputs []entry) int {
	for i, e := range inputs {
		// While a chooser is open the keys belong to it: an entry point that is
		// not about it closes it first, the way the operator would.
		if w.m.startupShowsModal() && !e.modal {
			w.input(closing(entries, w.m.startup))
		}
		if i%4 == 3 {
			w.astrolog.phase.Add(1)
		}
		w.input(e)
		// Deliver a seeded number of held messages before the next input: the
		// next key arrives while older runs are still out.
		for k := w.rng.Intn(4); k > 0 && len(w.held) > 0; k-- {
			w.deliver()
		}
	}
	if w.m.startupShowsModal() {
		w.send(keyEsc)
	}
	w.active = ""
	n, idle := w.drain(chaosSettle)
	w.worstSettle, w.worstIdle = max(w.worstSettle, n), max(w.worstIdle, idle)
	w.settled(fmt.Sprintf("%s (settled in %d delivered messages, and %d of the idle prefetch's)", label, n, idle))
	return n
}

// lacking are the entry points a coverage counter has not reached yet.
func (w *world) lacking(entries []entry) []entry {
	var out []entry
	for _, e := range entries {
		if c := w.cov[e.name]; c.outstanding == 0 || c.failing == 0 {
			out = append(out, e)
		}
	}
	return out
}

// seededEveryFrame is the seeded half of TestEveryFrameShowsOneSubject.
func seededEveryFrame(t *testing.T) {
	start := time.Now()
	w := newWorld(t, chaosSeed)
	entries := w.entries()
	var modals []entry
	for _, e := range entries {
		if e.modal {
			modals = append(modals, e)
		}
	}
	for seq := 0; seq < chaosSequences; seq++ {
		w.astrolog.phase.Store(uint64(seq))
		order := w.rng.Perm(len(entries))
		inputs := make([]entry, chaosPerSeq)
		for i := range inputs {
			// The first inputs of every sequence visit every entry point in a
			// seeded order, so none depends on the dice; the rest are drawn.
			inputs[i] = entries[w.rng.Intn(len(entries))]
			if i < len(entries) {
				inputs[i] = entries[order[i]]
			}
		}
		w.session(fmt.Sprintf("sequence %d", seq), entries, modals, inputs)
	}
	// An entry point whose counters the dice left at zero is tried again, in
	// storms, a few times: the counters still fail the test when it cannot be
	// made to change a wanted key with a run outstanding and a run failing.
	for round := 0; round < 6; round++ {
		lacking := w.lacking(entries)
		if len(lacking) == 0 {
			break
		}
		w.astrolog.phase.Store(uint64(chaosSequences+round*3) + 2) // a storm
		var inputs []entry
		for k := 0; k < 4; k++ {
			inputs = append(inputs, lacking...)
		}
		w.session(fmt.Sprintf("top-up %d", round), entries, modals, inputs)
	}
	// The list is the spec's (C10), not the corpus's: an entry point dropped
	// from the corpus has no counter to read, and is a failure here.
	for _, n := range chaosRequired {
		c, ok := w.cov[n]
		if !ok {
			t.Errorf("coverage: the entry point %q is not in the corpus", n)
			continue
		}
		t.Logf("%-28s applied %3d, changed a wanted key %3d, with a run outstanding %3d, with a run failing %3d", n, c.applied, c.changed, c.outstanding, c.failing)
		if c.outstanding == 0 {
			t.Errorf("coverage: %q never changed a wanted key while an older run was outstanding", n)
		}
		if c.failing == 0 {
			t.Errorf("coverage: %q never changed a wanted key while a run failed", n)
		}
	}
	// Revisits are drawn from memory (C13): the corpus must have drawn some, or
	// its frames never held a remembered result to the rule.
	hits := w.m.mem.hits
	t.Logf("results drawn from memory: text frames %d, reports %d, timelines %d, templates %d", hits[surfaceText], hits[surfaceDetails], hits[surfaceTimeline], hits[surfaceTemplate])
	for s, name := range map[surface]string{surfaceText: "text frame", surfaceDetails: "Details report"} {
		if hits[s] == 0 {
			t.Errorf("coverage: no %s was ever drawn from memory", name)
		}
	}
	// Results computed ahead are drawn (C15): the corpus must have drawn some, or
	// the prefetch never met the rule.
	t.Logf("after input stopped, the world needed at most %d delivered messages of the panes' own to settle (bound %d) and %d of the idle prefetch's (bound %d)", w.worstSettle, chaosSettle, w.worstIdle, idleBound)
	t.Logf("results computed ahead and later drawn: %d (%d prefetch calls)", w.m.mem.drawn, w.prefetchCalls)
	if w.m.mem.drawn == 0 {
		t.Error("coverage: no result computed ahead was ever drawn")
	}
	var states []string
	for k, n := range w.seen {
		states = append(states, fmt.Sprintf("%s %d", k, n))
	}
	sort.Strings(states)
	t.Logf("frames by pane state: %s", strings.Join(states, ", "))
	for _, k := range chaosStates {
		if w.seen[k] == 0 {
			t.Errorf("coverage: no frame ever drew %s", k)
		}
	}
	t.Logf("%d sequences of %d entry points, seed %d: %d frames checked, %d stand-in calls, %d of them failed, %s", chaosSequences, chaosPerSeq, chaosSeed, w.checked, w.astrolog.calls.Load(), w.astrolog.fails.Load(), time.Since(start).Round(time.Millisecond))
}
