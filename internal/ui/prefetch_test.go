package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/templates"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// specs/details-follows-its-tab.md, slice D4, C15-C17: what is computed ahead
// while the screen is idle, that it speaks nowhere until it is drawn, and when
// it never runs.

// callBook records every Astrolog call the stand-in answers, one string each,
// in the order they began, and how many were in flight at once.
type callBook struct {
	mu       sync.Mutex
	calls    []string
	inflight int
	peak     int
	timeline int
	// hold, when set, is run by the call it names (by prefix) before it answers;
	// it returns the error the call then fails with.
	hold func(ctx context.Context, what string) error
}

func (b *callBook) enter(ctx context.Context, what string) error {
	b.mu.Lock()
	b.calls = append(b.calls, what)
	b.inflight++
	b.peak = max(b.peak, b.inflight)
	hold := b.hold
	b.mu.Unlock()
	if hold != nil {
		return hold(ctx, what)
	}
	return nil
}

func (b *callBook) leave() {
	b.mu.Lock()
	b.inflight--
	b.mu.Unlock()
}

func (b *callBook) list() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.calls)
}

func (b *callBook) count() int { return len(b.list()) }

// recorded wraps the fixture's stand-in so that every cast, report and fact set
// is written in the book, reports the command behind it (so the Log's argv
// lines can be compared), and the Timeline's search is counted.
func recorded(m Model, b *callBook) Model {
	return applySpawns(m, func(s *rowSpawns) {
		if f := s.cast.transit; f != nil {
			s.cast.transit = func(ctx context.Context, p *lookup.Probers, now time.Time, place location.PlaceInfo, lens lookup.Lens) (lookup.Result, error) {
				if err := b.enter(ctx, "frame floating "+now.Format("Jan 2")); err != nil {
					b.leave()
					return lookup.Result{}, err
				}
				defer b.leave()
				r, err := f(ctx, p, now, place, lens)
				r.Commands = []string{"astrolog frame floating " + now.UTC().Format("2006-01-02T15:04")}
				return r, err
			}
		}
		if f := s.cast.natal; f != nil {
			s.cast.natal = func(ctx context.Context, p *lookup.Probers, c store.AnchorChart, lens lookup.Lens) (lookup.Result, error) {
				if err := b.enter(ctx, fmt.Sprintf("frame natal %d", c.When.Year())); err != nil {
					b.leave()
					return lookup.Result{}, err
				}
				defer b.leave()
				r, err := f(ctx, p, c, lens)
				r.Commands = []string{fmt.Sprintf("astrolog frame natal %d", c.When.Year())}
				return r, err
			}
		}
		if f := s.reports.raw; f != nil {
			s.reports.raw = func(ctx context.Context, when time.Time, place location.PlaceInfo, lens lookup.Lens, req lookup.RawReportRequest) (string, string, error) {
				what := "report chart " + when.Format("2006 Jan 2")
				if req.Kind != lookup.RawReportChart {
					what = "report body " + panel.BodyName(req.Object) + " " + when.Format("2006 Jan 2")
				}
				if err := b.enter(ctx, what); err != nil {
					b.leave()
					return "", "", err
				}
				defer b.leave()
				raw, _, err := f(ctx, when, place, lens, req)
				return raw, "astrolog " + what, err
			}
		}
		if s.bodyFacts != nil {
			s.bodyFacts = func(ctx context.Context, req lookup.BodyFactsRequest) (lookup.BodyFactsResult, error) {
				what := "facts " + panel.BodyName(req.Object) + " " + req.When.Format("2006 Jan 2")
				if err := b.enter(ctx, what); err != nil {
					b.leave()
					return lookup.BodyFactsResult{}, err
				}
				defer b.leave()
				return lookup.BodyFactsResult{Commands: []string{"astrolog " + what}, Now: researchSkyFacts()}, nil
			}
		}
		if s.timeline != nil {
			s.timeline = func(context.Context, lookup.TimelineRequest) (lookup.TimelineResult, error) {
				b.mu.Lock()
				b.timeline++
				b.mu.Unlock()
				return lookup.TimelineResult{}, nil
			}
		}
	})
}

// prefetchBed is the follow fixture, recording, with no rest timer: the idle
// period begins when the test says so (idleNow).
func prefetchBed(t *testing.T) (Model, *followStub, *callBook) {
	t.Helper()
	m, stub := followFixture(t, Config{Accessible: true}, 132, 40)
	b := &callBook{}
	return recorded(m, b), stub, b
}

// idleNow ends the rest the last key started: the idle period begins.
func idleNow(m Model) (Model, tea.Cmd) {
	tm, cmd := m.Update(prefetchIdleMsg{seq: m.pre.seq})
	return tm.(Model), cmd
}

// runChain runs a command and every command its message starts, one at a time
// and in order, until none is left; a prefetch is a chain of single calls.
func runChain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 2000 {
			t.Fatal("runChain: the model never went quiet")
		}
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		default:
			tm, next := m.Update(msg)
			m = tm.(Model)
			queue = append(queue, next)
		}
	}
	return m
}

// idlePeriod is a whole idle period from its start to its end.
func idlePeriod(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := idleNow(m)
	return runChain(t, m, cmd)
}

// expectedOrder is C15's order for the fixture, written from the spec and the
// screen, not from the snapshot: the focused chart `Now`, Chart focused,
// Details on its report tab, one anchored chart `Synthetic 1`.
func expectedOrder(m Model) []string {
	day := func(t time.Time) string { return t.Format("2006 Jan 2") }
	floating := m.slots.Floating()
	var want []string
	// (1) every reportable Bodies row: its report, then its template's facts.
	ch := m.shownChart()
	for row := range panel.BodyRowCount(ch.positions, ch.cusps) {
		obj, ok := panel.ObjectAtCursor(ch.positions, ch.cusps, row)
		if !ok {
			continue
		}
		want = append(want, "report body "+panel.BodyName(obj)+" "+day(floating.Time))
		if _, _, ok := templates.ForObject(shippedTemplates, obj); ok {
			want = append(want, "facts "+panel.BodyName(obj)+" "+day(floating.Time))
		}
	}
	// (2) every chart of the list but the one on screen: its frame, its Chart report.
	for _, s := range m.slots.All() {
		if s.Floating {
			continue // Now: its frame and Chart report are on screen, hence in memory
		}
		want = append(want, fmt.Sprintf("frame natal %d", s.Time.Year()), "report chart "+day(s.Time))
	}
	// (3) the floating chart a day on, then a day back: frame, then the report tab's result.
	for _, d := range []int{+1, -1} {
		t := floating.Time.AddDate(0, 0, d)
		want = append(want, "frame floating "+t.Format("Jan 2"), "report chart "+day(t))
	}
	// (4) the current Bodies row is (1)'s: nothing new. The Timeline is never asked.
	return want
}

func TestPrefetchOneKeyAway(t *testing.T) {
	t.Run("the_order_and_each_key_once", func(t *testing.T) {
		m, _, b := prefetchBed(t)
		before := b.count()
		m = idlePeriod(t, m)
		got := b.list()[before:]
		want := expectedOrder(m)
		if !slices.Equal(got, want) {
			t.Errorf("the calls of the idle period, in order:\n got  %s\n want %s", strings.Join(got, "\n      "), strings.Join(want, "\n      "))
		}
		seen := map[string]bool{}
		for _, c := range got {
			if seen[c] {
				t.Errorf("%q was computed twice in one period", c)
			}
			seen[c] = true
		}
		if b.timeline != 0 {
			t.Errorf("the Timeline was searched %d times ahead, never", b.timeline)
		}
		if m.pre.cancel != nil || len(m.pre.queue) != 0 {
			t.Errorf("the period did not end: a call is out %v, %d keys left", m.pre.cancel != nil, len(m.pre.queue))
		}
		t.Logf("one idle period on the follow fixture: %d Astrolog calls", len(got))
	})

	t.Run("one_call_at_a_time", func(t *testing.T) {
		m, _, b := prefetchBed(t)
		m = idlePeriod(t, m)
		if b.count() == 0 {
			t.Fatal("fixture: the period made no call")
		}
		if b.peak != 1 {
			t.Errorf("%d Astrolog calls were in flight at once, want 1", b.peak)
		}
	})

	// With a template tab the active Details tab, it is the template's result
	// that is asked at a day on and a day back, and not the report's.
	t.Run("the_active_template_tab_is_asked_a_day_on_and_back", func(t *testing.T) {
		m, _, b := prefetchBed(t)
		m, cmd := pressOnly(t, m, runes("2")) // Bodies
		m = pump(t, m, cmd)
		for i := 0; i < 8; i++ { // down to a row that offers a template tab
			if _, ok := m.templateKeyFor(); ok {
				break
			}
			m, cmd = pressOnly(t, m, keyDown)
			m = pump(t, m, cmd)
		}
		if _, ok := m.templateKeyFor(); !ok {
			t.Fatal("fixture: no Bodies row offers a template tab")
		}
		for i := 0; i < 4 && !isTemplateTab(m.activeTabID(PanelDetails)); i++ {
			m, cmd = pressOnly(t, m, runes("0"))
			m = pump(t, m, cmd)
		}
		if !isTemplateTab(m.activeTabID(PanelDetails)) {
			t.Fatal("fixture: the template tab did not become the active one")
		}
		obj, _ := panel.ObjectAtCursor(m.shownChart().positions, m.shownChart().cusps, m.bodyCursorIdx)
		floating := m.slots.Floating().Time
		before := b.count()
		m = idlePeriod(t, m)
		got := strings.Join(b.list()[before:], "\n")
		for _, d := range []int{+1, -1} {
			day := floating.AddDate(0, 0, d)
			if want := "facts " + panel.BodyName(obj) + " " + day.Format("2006 Jan 2"); !strings.Contains(got, want) {
				t.Errorf("no %q among the calls:\n%s", want, got)
			}
			if bad := "report body " + panel.BodyName(obj) + " " + day.Format("2006 Jan 2"); strings.Contains(got, bad) {
				t.Errorf("%q asked ahead while the template tab is the active one", bad)
			}
		}
		if b.timeline != 0 {
			t.Errorf("the Timeline was searched ahead %d times", b.timeline)
		}
	})

	t.Run("a_key_stops_the_call_in_flight", func(t *testing.T) {
		m, _, b := prefetchBed(t)
		entered := make(chan string, 1)
		stopped := make(chan struct{})
		var once sync.Once
		b.hold = func(ctx context.Context, what string) error {
			var first bool
			once.Do(func() { first = true })
			if !first {
				return nil
			}
			entered <- what
			select {
			case <-ctx.Done():
				close(stopped)
				return ctx.Err()
			case <-time.After(3 * time.Second):
				return nil
			}
		}
		m, cmd := idleNow(m)
		if cmd == nil {
			t.Fatal("the idle period started no call")
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- cmd() }()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("the first call never began")
		}
		calls := b.count()
		m, _ = pressOnly(t, m, keyDown)
		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
			t.Fatal("a key arrived and the call in flight was not stopped")
		}
		// The stopped call's message arrives late: it files nothing and starts nothing.
		tm, next := m.Update(<-done)
		m = tm.(Model)
		if next != nil || b.count() != calls {
			t.Errorf("after the stop, the call's message started more work: %d calls, was %d", b.count(), calls)
		}
		if m.pre.cancel != nil || len(m.pre.queue) != 0 {
			t.Error("the idle period goes on after a key")
		}
		if len(m.pre.failed) != 0 {
			t.Errorf("a call that a key stopped counts as failed: %v", m.pre.failed)
		}
	})
}

// A quiet screen starts nothing a second time, and a key whose period found
// everything attempted starts no call (C17: the period ends after every key of
// its snapshot was attempted once).
func TestPrefetchIdlePeriodEnds(t *testing.T) {
	m, stub, b := prefetchBed(t)
	stub.failBodies = true // some keys fail: they are attempted once and left alone
	m = idlePeriod(t, m)
	first := b.count()
	if first == 0 {
		t.Fatal("fixture: the first period made no call")
	}
	if len(m.pre.failed) == 0 {
		t.Fatal("fixture: no key failed")
	}
	// Another key that changes nothing, then its rest: an unchanged Model.
	m, _ = pressOnly(t, m, keyEsc)
	m = idlePeriod(t, m)
	if got := b.count() - first; got != 0 {
		t.Errorf("a second idle period on an unchanged Model made %d calls: %v", got, b.list()[first:])
	}
	// Nor does a rest that fires twice start a second period of the same key.
	m, cmd := idleNow(m)
	if cmd != nil || b.count() != first {
		t.Errorf("the same rest ended twice and started a call")
	}
}

func TestPrefetchIsSilentUntilDrawn(t *testing.T) {
	prefetched, _, b := prefetchBed(t)
	freshModels(t)
	fresh, _, _ := prefetchBed(t)

	logBefore := slices.Clone(prefetched.logLines)
	viewBefore := prefetched.View()
	shownBefore, detailsBefore := prefetched.shown.key, prefetched.details.key
	prefetched = idlePeriod(t, prefetched)
	if b.count() == 0 {
		t.Fatal("fixture: nothing was computed ahead")
	}
	if !slices.Equal(prefetched.logLines, logBefore) {
		t.Errorf("a prefetch wrote to the Log or the large log:\n%v", prefetched.logLines[len(logBefore):])
	}
	if got := prefetched.View(); got != viewBefore {
		t.Errorf("a prefetch changed what is drawn:\nbefore\n%s\nafter\n%s", viewBefore, got)
	}
	if prefetched.shown.key != shownBefore || prefetched.details.key != detailsBefore {
		t.Error("a prefetched result replaced shown content")
	}
	if len(prefetched.mem.prefetched) == 0 {
		t.Fatal("fixture: nothing entered memory")
	}

	// Drawing it: the Chart cursor to Synthetic 1, one key away. The hit writes
	// the argv lines and notes a fresh run writes, in the same order.
	calls := b.count()
	freshBefore := len(fresh.logLines)
	fresh, cmd := tpress(t, fresh, keyUp)
	fresh = pump(t, fresh, cmd)
	prefetched, cmd = tpress(t, prefetched, keyUp)
	if prefetched.shown.key != mustTextKey(t, prefetched) || prefetched.detailsReportState() != paneCurrent {
		t.Errorf("the prefetched frame and report are not drawn at the key: Bodies %v, Details %v", prefetched.textPaneState(PanelBodies), prefetched.detailsReportState())
	}
	prefetched = pump(t, prefetched, cmd)
	if b.count() != calls {
		t.Errorf("the move spawned %d calls for a chart that was computed ahead", b.count()-calls)
	}
	gotLines, wantLines := prefetched.logLines[len(logBefore):], fresh.logLines[freshBefore:]
	if !slices.Equal(gotLines, wantLines) {
		t.Errorf("drawing the prefetched result writes\n%v\nbut a fresh run writes\n%v", gotLines, wantLines)
	}
	if !slices.ContainsFunc(gotLines, func(l logLine) bool { return l.lane == laneArgv }) {
		t.Error("fixture: the draw wrote no argv line, so nothing was compared")
	}
	if prefetched.mem.drawn == 0 {
		t.Error("the draw of a prefetched result was not counted as one")
	}
}

func TestPrefetchStops(t *testing.T) {
	// idleStartsNothing delivers the rest to a Model in the given condition and
	// asserts no call began.
	idleStartsNothing := func(t *testing.T, m Model, b *callBook) {
		t.Helper()
		m, cmd := idleNow(m)
		if cmd != nil && m.pre.cancel != nil {
			t.Error("a call was started")
		}
		if m.pre.calls != 0 || b.count() != 0 {
			t.Errorf("a prefetch ran: %d calls started, %v", m.pre.calls, b.list())
		}
	}
	inFlight := func(s surface) func(*testing.T) {
		return func(t *testing.T) {
			m, _, b := prefetchBed(t)
			m.refresh[s].cancel = func() {}
			idleStartsNothing(t, m, b)
		}
	}
	t.Run("text_in_flight", inFlight(surfaceText))
	t.Run("report_in_flight", inFlight(surfaceDetails))
	t.Run("timeline_in_flight", inFlight(surfaceTimeline))
	t.Run("template_in_flight", inFlight(surfaceTemplate))
	t.Run("image_in_flight", inFlight(surfaceImage))
	t.Run("a_report_is_scheduled", func(t *testing.T) {
		m, _, b := prefetchBed(t)
		m.details.timerArmed = true
		idleStartsNothing(t, m, b)
	})
	t.Run("a_text_frame_is_owed", func(t *testing.T) {
		m, _, b := prefetchBed(t)
		m.refresh[surfaceText].wantID++
		idleStartsNothing(t, m, b)
	})
	t.Run("instance_lock_refused", func(t *testing.T) {
		m, _, b := prefetchBed(t)
		m.cantStart.lockReason, m.cantStart.lockRemedy = "Couldn't open the lock.", "Start again."
		idleStartsNothing(t, m, b)
	})
	t.Run("a_modal_owns_the_keyboard", func(t *testing.T) {
		m, _, b := prefetchBed(t)
		m.helpOpen = true
		idleStartsNothing(t, m, b)
	})
	t.Run("a_chooser_owns_the_keyboard", func(t *testing.T) {
		m, _, b := prefetchBed(t)
		m.startup = startupBodyOpen
		idleStartsNothing(t, m, b)
	})
	t.Run("astrolog_not_installed", func(t *testing.T) {
		m, _, b := prefetchBed(t)
		m.cantStart.binary = true
		idleStartsNothing(t, m, b)
	})

	// A call that is out when a surface starts its own run is stopped there: the
	// two never run side by side.
	t.Run("a_surface_starts_while_a_call_is_out", func(t *testing.T) {
		m, _, b := prefetchBed(t)
		stopped := make(chan struct{})
		entered := make(chan struct{})
		var once sync.Once
		b.hold = func(ctx context.Context, what string) error {
			first := false
			once.Do(func() { first = true })
			if !first {
				return nil
			}
			close(entered)
			select {
			case <-ctx.Done():
				close(stopped)
				return ctx.Err()
			case <-time.After(3 * time.Second):
				return nil
			}
		}
		m, cmd := idleNow(m)
		go cmd()
		<-entered
		// The debounce tick of a want the text surface holds: the run starts.
		m.refresh[surfaceText].wantID++
		m.refresh[surfaceText].timerID = m.refresh[surfaceText].wantID
		tm, _ := m.Update(refreshTickMsg{surface: surfaceText, timerID: m.refresh[surfaceText].timerID})
		m = tm.(Model)
		if m.refresh[surfaceText].cancel == nil {
			t.Fatal("fixture: the text surface did not start its run")
		}
		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
			t.Fatal("a surface started a run and the prefetch call was not stopped")
		}
		if m.pre.cancel != nil || len(m.pre.queue) != 0 {
			t.Error("the idle period goes on beside a surface's run")
		}
	})

	// The memory bound may drop some of what a period computed; the period still
	// ends after every key of its snapshot was attempted once.
	t.Run("the_period_ends_when_the_bound_evicts", func(t *testing.T) {
		m, _, b := prefetchBed(t)
		for n := 2; n <= memoryBound+8; n++ {
			openChart(t, m, n)
		}
		m, cmd := tpress(t, m, keyDown) // a key: the Model changes, the rest follows
		m = pump(t, m, cmd)
		before := b.count()
		m = idlePeriod(t, m)
		calls := b.count() - before
		if m.mem.text.len() != memoryBound {
			t.Fatalf("fixture: %d frames remembered, want the bound %d", m.mem.text.len(), memoryBound)
		}
		if m.pre.cancel != nil || len(m.pre.queue) != 0 {
			t.Fatalf("the period did not end: %d keys left", len(m.pre.queue))
		}
		// One call per key of the snapshot: each chart's frame and report, once.
		seen := map[string]int{}
		for _, c := range b.list()[before:] {
			seen[c]++
		}
		for c, n := range seen {
			if n != 1 {
				t.Errorf("%q was attempted %d times in one period", c, n)
			}
		}
		if calls < memoryBound {
			t.Errorf("only %d calls: the snapshot did not outgrow the bound, so no key was evicted", calls)
		}
	})
}

func TestPrefetchFailureIsQuiet(t *testing.T) {
	m, stub, b := prefetchBed(t)
	stub.failBodies = true // Astrolog dies on every body report
	logBefore := slices.Clone(m.logLines)
	m = idlePeriod(t, m)
	failed := 0
	for _, c := range b.list() {
		if strings.HasPrefix(c, "report body") {
			failed++
		}
	}
	if failed == 0 {
		t.Fatal("fixture: no body report was tried ahead")
	}
	if !slices.Equal(m.logLines, logBefore) {
		t.Errorf("a failed prefetch wrote to the Log or the large log:\n%v", m.logLines[len(logBefore):])
	}
	if m.details.failed.set || m.errorKeptView {
		t.Error("a failed prefetch marked a pane as failed")
	}
	// Not retried in the same idle period: every key of the snapshot, once.
	count := map[string]int{}
	for _, c := range b.list() {
		count[c]++
		if count[c] > 1 {
			t.Errorf("%q was attempted %d times in one idle period", c, count[c])
		}
	}

	// It becomes the wanted key: the normal run happens, and the failure is
	// drawn and logged as C6 says.
	calls := b.count()
	m, cmd := tpress(t, m, runes("2")) // Bodies: the Sun's report, which failed ahead
	m = pump(t, m, cmd)
	if got := b.count() - calls; got != 1 {
		t.Errorf("the failed key, once wanted, made %d calls, want its one normal run", got)
	}
	if m.detailsReportState() != paneFailed {
		t.Errorf("Details is %v, want the failure drawn", m.detailsReportState())
	}
	if !slices.ContainsFunc(m.logLines, func(l logLine) bool {
		return l.lane == laneError && strings.Contains(l.text, "Couldn't read the full report")
	}) {
		t.Errorf("the normal run's failure is not on the Log's error lane: %v", m.logLines[len(logBefore):])
	}
}

// The first idle period of a run begins when startup reaches ready, not only
// after a key: the operator's first key after launch may be the one that asks
// for what that period computes (`2`, the Sun's report).
func TestPrefetchStartsAfterStartup(t *testing.T) {
	m, _, b := prefetchBed(t)
	// Back to a launch: nothing shown, nothing remembered, startup computing.
	m.startup = startupComputing
	m.chart, m.shown, m.details, m.mem, m.pre = chartViewState{}, shownFrame{}, detailsViewState{}, memory{}, prefetch{}
	b.mu.Lock()
	b.calls = nil
	b.mu.Unlock()

	var rests []tea.Msg
	prev := idleTimer
	idleTimer = func(_ time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		rests = append(rests, fn(time.Time{}))
		return nil
	}
	t.Cleanup(func() { idleTimer = prev })

	m, cmd := m.bumpIdentity(surfaceText)
	m = pump(t, m, cmd) // the first frame commits and its report lands: ready, no key pressed
	if m.startup != startupReady {
		t.Fatalf("fixture: startup is %q, want ready", m.startup)
	}
	if len(rests) != 1 {
		t.Fatalf("reaching ready armed %d idle rests, want exactly 1", len(rests))
	}
	rest, ok := rests[0].(prefetchIdleMsg)
	if !ok {
		t.Fatalf("the rest is %T, want a prefetchIdleMsg", rests[0])
	}
	asked := b.count()
	tm, cmd := m.Update(rest)
	m = runChain(t, tm.(Model), cmd)
	got := b.list()[asked:]
	ch := m.shownChart()
	first, _ := panel.ObjectAtCursor(ch.positions, ch.cusps, 0)
	if len(got) == 0 || !strings.HasPrefix(got[0], "report body "+panel.BodyName(first)+" ") {
		t.Fatalf("the first call of the first period is %v, want the report of the first reportable Bodies row (%s)", got, panel.BodyName(first))
	}

	// `2`: the Sun's report is drawn from memory, no call.
	calls := b.count()
	m, cmd = tpress(t, m, runes("2"))
	if m.detailsReportState() != paneCurrent || !strings.Contains(m.details.body, "SUBJECT body Sun") {
		t.Errorf("after 2, Details is %v and holds %q, want the Sun's report drawn at the key", m.detailsReportState(), m.details.body)
	}
	m = pump(t, m, cmd)
	if b.count() != calls {
		t.Errorf("2 made %d Astrolog calls for a report computed ahead: %v", b.count()-calls, b.list()[calls:])
	}
}
