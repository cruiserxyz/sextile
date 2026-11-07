package ui

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// specs/details-follows-its-tab.md, slice D3, C12-C14: what each cached surface
// remembers, the frame a remembered result is drawn in, and the first key of a
// run starting what it newly owes at once.

// withArgv makes the fixture's transit casts report the command behind them, so
// a test can see which argv lines the Log holds.
func withArgv(m Model) Model {
	orig := m.rows[surfaceText].spawn.cast.transit
	m.rows[surfaceText].spawn.cast.transit = func(ctx context.Context, p *lookup.Probers, when time.Time, place location.PlaceInfo, lens lookup.Lens) (lookup.Result, error) {
		r, err := orig(ctx, p, when, place, lens)
		r.Commands = []string{"astrolog-transit " + when.UTC().Format("2006-01-02T15:04")}
		return r, err
	}
	return m
}

// resized delivers a terminal resize and lets what it starts finish.
func resized(t *testing.T, m Model, w, h int) Model {
	t.Helper()
	tm, cmd := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return pump(t, tm.(Model), cmd)
}

// stepDay presses d (or D) and lets everything it starts finish.
func stepDay(t *testing.T, m Model, key string) Model {
	t.Helper()
	m, cmd := tpress(t, m, runes(key))
	return pump(t, m, cmd)
}

func TestMemoryKeyedAndBounded(t *testing.T) {
	// The structure: bounded at memoryBound, least recently used dropped first,
	// a use counts as recent, and a key is the whole key.
	t.Run("lru", func(t *testing.T) {
		var c lru[int, string]
		for i := 0; i < memoryBound+6; i++ {
			c = c.put(i, strconv.Itoa(i))
			if i == memoryBound-1 {
				// The oldest entry is used once, so it is the newest by the time the rest arrive.
				_, c, _ = c.use(0)
			}
		}
		if c.len() != memoryBound {
			t.Fatalf("%d entries, want the bound %d", c.len(), memoryBound)
		}
		if _, ok := c.peek(0); !ok {
			t.Error("the entry used just before the overflow was dropped: a use does not count as recent")
		}
		for _, gone := range []int{1, 2, 3, 4, 5, 6} {
			if _, ok := c.peek(gone); ok {
				t.Errorf("entry %d outlived the six that arrived after the bound", gone)
			}
		}
		if _, ok := c.peek(memoryBound + 5); !ok {
			t.Error("the newest entry is missing")
		}
		before := c
		_ = c.put(1000, "x")
		if before.len() != memoryBound || before.items[0].key != c.items[0].key {
			t.Error("put changed the value it was called on: a Model copied before an Update would see what the Update remembered")
		}

		keyA := detailsKey{panel: PanelBodies, object: 4, slot: 1}
		keyB := keyA
		keyB.slotWhen++
		var r lru[detailsKey, string]
		r = r.put(keyA, "a").put(keyB, "b")
		if a, _ := r.peek(keyA); a != "a" || r.len() != 2 {
			t.Errorf("two keys that differ in one field share an entry: %d entries", r.len())
		}
	})

	// A real run: 70 days of stepping keeps the newest 64 frames and drops the
	// first six, and the reports keep the same bound.
	t.Run("frames_and_reports", func(t *testing.T) {
		m, _ := followFixture(t, Config{Accessible: true}, 132, 40)
		first := mustTextKey(t, m)
		for i := 0; i < memoryBound+6; i++ {
			m = stepDay(t, m, "d")
		}
		if got := m.mem.text.len(); got != memoryBound {
			t.Errorf("%d text frames remembered after %d steps, want %d", got, memoryBound+6, memoryBound)
		}
		if _, ok := m.mem.text.peek(first); ok {
			t.Error("the first frame is still remembered after 70 later ones")
		}
		if _, ok := m.mem.text.peek(mustTextKey(t, m)); !ok {
			t.Error("the frame on screen is not remembered")
		}
		if got := m.mem.report.len(); got == 0 || got > memoryBound {
			t.Errorf("%d reports remembered, want 1 to %d", got, memoryBound)
		}
	})

	// A failure is not a result to show again.
	t.Run("failures_are_not_kept", func(t *testing.T) {
		m, stub := followFixture(t, Config{Accessible: true}, 132, 40)
		stub.failBodies = true
		m = pump(t, m, nil)
		m, cmd := pressOnly(t, m, runes("2")) // Bodies: the Sun's report, which fails
		m = pump(t, m, cmd)
		if m.detailsReportState() != paneFailed {
			t.Fatalf("fixture: the Sun's report is %v, want failed", m.detailsReportState())
		}
		key, _ := m.detailsKeyFor()
		if _, ok := m.mem.report.peek(key); ok {
			t.Error("the failed report was remembered")
		}
	})

	// RAM only: the file that holds the memory names no package that could write.
	t.Run("ram_only", func(t *testing.T) {
		f, err := parser.ParseFile(token.NewFileSet(), "memory.go", nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			switch imp.Path.Value {
			case `"slices"`, `"github.com/charmbracelet/bubbletea"`:
			default:
				t.Errorf("memory.go imports %s: the memory is RAM only and needs nothing but slices", imp.Path.Value)
			}
		}
	})
}

func TestMemoryHitPaintsAtOnce(t *testing.T) {
	setup := func(t *testing.T) (Model, *followStub) {
		m, stub := followFixture(t, Config{Accessible: true}, 132, 40)
		m = withArgv(m)
		for i := 0; i < 3; i++ { // three days on: three more frames and reports
			m = stepDay(t, m, "d")
		}
		return m, stub
	}

	// Going back a day shows that day's frame and report in the frame of the key:
	// no command is returned that spawns, the Log holds the stored argv line, and
	// nothing earlier can land after it.
	t.Run("back_a_day", func(t *testing.T) {
		m, stub := setup(t)
		want := m.targetFloatingTime.AddDate(0, 0, -1)
		spawned := stub.casts + stub.reports
		wantBefore := m.refresh[surfaceText].wantID
		runBefore := m.refresh[surfaceText].runningRunID
		m, cmd := tpress(t, m, runes("D"))

		if !m.targetFloatingTime.Equal(want) {
			t.Fatalf("fixture: D moved the target to %v, want %v", m.targetFloatingTime, want)
		}
		key := mustTextKey(t, m)
		if m.shown.key != key || !bothPanesCurrent(m) {
			t.Errorf("after the key, before any command ran: the frame on screen is for %+v, Bodies %v Aspects %v", m.shown.key.when, m.textPaneState(PanelBodies), m.textPaneState(PanelAspects))
		}
		if !m.slots.Floating().Time.Equal(want) {
			t.Errorf("the Chart's moment is %v, want %v: the frame did not set the slot time", m.slots.Floating().Time, want)
		}
		if m.detailsReportState() != paneCurrent {
			t.Errorf("Details is %v after the key, want that day's report drawn with the frame", m.detailsReportState())
		}
		if got := m.refresh[surfaceText].wantID; got != wantBefore+1 {
			t.Errorf("the text want moved by %d, want the key's own registration only", got-wantBefore)
		}
		if r := m.refresh[surfaceText]; r.cancel != nil || r.lastCommitWantID != r.wantID || r.commitFloor != r.wantID || r.runningRunID <= runBefore {
			t.Errorf("the text surface is not fenced after the hit: %+v", r)
		}
		lines := argvLines(m)
		wantLine := "astrolog-transit " + want.UTC().Format("2006-01-02T15:04")
		if len(lines) == 0 || lines[len(lines)-1] != wantLine {
			t.Errorf("the Log's argv lines end %q, want the stored line %q", lines, wantLine)
		}
		m = pump(t, m, cmd)
		if stub.casts+stub.reports != spawned {
			t.Errorf("%d Astrolog calls for a day already seen", stub.casts+stub.reports-spawned)
		}
		if m.shown.key != key {
			t.Error("the frame on screen changed after the key's commands ran")
		}
	})

	// The Chart cursor moving to another chart and back is the other way a frame
	// is asked for (it fences the surface as an identity change does); the way
	// back is a hit, too.
	t.Run("cursor_back_to_a_chart", func(t *testing.T) {
		m, stub := followFixture(t, Config{Accessible: true}, 132, 40)
		m, cmd := tpress(t, m, keyUp) // Synthetic 1: not seen
		m = pump(t, m, cmd)
		if m.shown.key != mustTextKey(t, m) {
			t.Fatal("fixture: the other chart's frame did not arrive")
		}
		spawned := stub.casts + stub.reports
		m, cmd = tpress(t, m, keyDown) // Now again
		if m.shown.key != mustTextKey(t, m) || !bothPanesCurrent(m) || m.detailsReportState() != paneCurrent {
			t.Errorf("back on Now, before any command ran: Bodies %v, Aspects %v, Details %v", m.textPaneState(PanelBodies), m.textPaneState(PanelAspects), m.detailsReportState())
		}
		m = pump(t, m, cmd)
		if stub.casts+stub.reports != spawned {
			t.Errorf("%d Astrolog calls for a chart already seen", stub.casts+stub.reports-spawned)
		}
	})

	// A run for another key is still out when the hit lands; its result arrives
	// afterwards and must not commit (the text surface commits a stale success
	// as a burst's leading frame, so only the fence stops it).
	t.Run("a_stale_result_after_the_hit", func(t *testing.T) {
		m, _ := setup(t)
		m, held := tpress(t, m, runes("d")) // a day not seen: a run starts, and is held
		stale := mustTextKey(t, m)
		if m.refresh[surfaceText].cancel == nil {
			t.Fatal("fixture: no run is out for the day not seen")
		}
		m, cmd := tpress(t, m, runes("D")) // back to a day that was
		hit := mustTextKey(t, m)
		if hit == stale || m.shown.key != hit {
			t.Fatalf("fixture: the hit did not draw its frame (shown %+v)", m.shown.key.when)
		}
		committed := false
		m = pumpSeen(t, m, 400*time.Millisecond, func(m Model) { committed = committed || m.shown.key == stale }, held, cmd)
		if committed || m.shown.key != hit || mustTextKey(t, m) != hit {
			t.Errorf("the result of the run for the other day committed after the hit: committed %v, on screen %v", committed, m.shown.key.when)
		}
		if !m.slots.Floating().Time.Equal(m.targetFloatingTime) {
			t.Errorf("the Chart's moment is %v after the stale result, want the target %v", m.slots.Floating().Time, m.targetFloatingTime)
		}
	})

	// The Timeline and a template tab draw a remembered result in the frame of
	// the key, too.
	t.Run("timeline", func(t *testing.T) {
		calls := stubTimeline(t, nil)
		m := loadedTimeline(t, Config{Color: ColorNever})
		m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
		home := m.slots.Floating().Time
		m.slots.SetFloatingTime(home.AddDate(0, 1, 0))
		m, cmd := m.syncTimeline()
		m = pump(t, m, cmd)
		if len(m.timeline.rows) == 0 || m.mem.timeline.len() != 2 {
			t.Fatalf("fixture: %d rows, %d lists remembered, want a second month searched and remembered", len(m.timeline.rows), m.mem.timeline.len())
		}
		searched := len(*calls)
		m.slots.SetFloatingTime(home)
		m, cmd = m.syncTimeline()
		key, _ := m.timelineKeyFor()
		if cmd != nil || !m.timeline.complete || m.timeline.content != key || m.timelinePaneState() != paneCurrent || m.timelineWaiting() {
			t.Errorf("the month seen before is not drawn in the frame of the key: cmd %v, complete %v, state %v", cmd != nil, m.timeline.complete, m.timelinePaneState())
		}
		m = pump(t, m, cmd)
		if len(*calls) != searched {
			t.Errorf("%d searches for a month already searched", len(*calls)-searched)
		}
	})

	t.Run("template", func(t *testing.T) {
		calls := stubBodyFacts(t, nil)
		m := activeOnTemplate(templateTestModel(t))
		m = runTemplate(t, m)
		home := m.slots.Floating().Time
		m.slots.SetFloatingTime(home.Add(time.Hour))
		m = runTemplate(t, m)
		if m.mem.template.len() != 2 {
			t.Fatalf("fixture: %d results remembered, want 2", m.mem.template.len())
		}
		asked := len(*calls)
		m.slots.SetFloatingTime(home)
		m, cmd := m.syncTemplate()
		key, _ := m.templateKeyFor()
		if cmd != nil || m.tpl.content != key || m.templateWaiting() || m.templatePaneState() != paneCurrent {
			t.Errorf("the moment seen before is not drawn in the frame of the key: cmd %v, state %v", cmd != nil, m.templatePaneState())
		}
		if len(*calls) != asked {
			t.Errorf("%d fact runs for a moment already seen", len(*calls)-asked)
		}
	})
}

func TestMemoryClearedOnIdentityBump(t *testing.T) {
	steps := func(t *testing.T) (Model, *followStub) {
		m, stub := followFixture(t, Config{Accessible: true}, 132, 40)
		for i := 0; i < 2; i++ {
			m = stepDay(t, m, "d")
		}
		return m, stub
	}

	// Control of the test: without a bump the day before is a hit.
	t.Run("no_bump_is_a_hit", func(t *testing.T) {
		m, stub := steps(t)
		casts := stub.casts
		m, cmd := tpress(t, m, runes("D"))
		if m.shown.key != mustTextKey(t, m) {
			t.Fatal("fixture: the day before was not drawn from memory")
		}
		m = pump(t, m, cmd)
		if stub.casts != casts {
			t.Fatal("fixture: the hit cast")
		}
	})

	// Details leaving the layout is where a workspace identity is given up (a
	// session reset, a chart or city pick and a chart closed pass through the
	// same suspendDetails): nothing computed before it is drawn after.
	t.Run("details_leaves_and_returns", func(t *testing.T) {
		m, stub := steps(t)
		if m.mem.text.len() == 0 || m.mem.report.len() == 0 {
			t.Fatal("fixture: nothing is remembered")
		}
		m = resized(t, m, 80, 24)
		if m.detailsVisible() {
			t.Fatal("fixture: Details is still in the layout at 80x24")
		}
		m = resized(t, m, 132, 40)
		casts := stub.casts
		m, cmd := tpress(t, m, runes("D"))
		if m.shown.key == mustTextKey(t, m) {
			t.Fatal("the day before was drawn from memory after the identity was given up")
		}
		if m.refresh[surfaceText].cancel == nil {
			t.Error("no run was started for a key whose memory was cleared")
		}
		m = pump(t, m, cmd)
		if stub.casts == casts {
			t.Error("no Astrolog call after the bump: the frame came from nowhere")
		}
		if m.shown.key != mustTextKey(t, m) {
			t.Error("the frame did not arrive")
		}
	})

	t.Run("suspendDetails", func(t *testing.T) {
		m, _ := steps(t)
		m = m.suspendDetails()
		if m.mem.text.len() != 0 || m.mem.report.len() != 0 || m.mem.timeline.len() != 0 || m.mem.template.len() != 0 {
			t.Errorf("memory after suspendDetails: %d frames, %d reports, %d lists, %d templates, want none", m.mem.text.len(), m.mem.report.len(), m.mem.timeline.len(), m.mem.template.len())
		}
	})
}

func TestTapStartsAtOnce(t *testing.T) {
	// The first key of a run starts the report with no rest.
	t.Run("first_key", func(t *testing.T) {
		m, stub := followFixture(t, Config{Accessible: true}, 132, 40)
		reports := stub.reports
		m, cmd := tpress(t, m, runes("2")) // Bodies, on the Sun: a report not seen
		if m.refresh[surfaceDetails].cancel == nil || m.details.timerArmed {
			t.Fatalf("the first key rested before the Sun's report: run started %v, rest timer armed %v", m.refresh[surfaceDetails].cancel != nil, m.details.timerArmed)
		}
		m = pump(t, m, cmd)
		if stub.reports != reports+1 || m.detailsReportState() != paneCurrent {
			t.Errorf("%d report spawns and Details %v after the first key", stub.reports-reports, m.detailsReportState())
		}

		// Another key is another run, and its first key starts at once as well.
		m, cmd = tpress(t, m, keyDown)
		if m.refresh[surfaceDetails].cancel == nil || m.details.timerArmed {
			t.Fatalf("a first key of another run rested: run started %v, timer armed %v", m.refresh[surfaceDetails].cancel != nil, m.details.timerArmed)
		}
		first := cmd

		// A repeat inside the run keeps the debounce: the run in flight is
		// replaced by one rest timer.
		m, cmd = tpress(t, m, keyDown)
		if m.refresh[surfaceDetails].cancel != nil || !m.details.timerArmed {
			t.Errorf("a repeat inside the run started at once: run started %v, timer armed %v", m.refresh[surfaceDetails].cancel != nil, m.details.timerArmed)
		}
		m = pump(t, m, first, cmd)
		if m.detailsReportState() != paneCurrent {
			t.Errorf("after the repeat settled Details is %v", m.detailsReportState())
		}
	})

	// A pause longer than the window makes the next key a first key again.
	t.Run("after_the_window", func(t *testing.T) {
		m, _ := followFixture(t, Config{Accessible: true}, 132, 40)
		clock := &fakeClock{t: testFrozenTime()}
		m.clock = clock
		m, cmd := tpress(t, m, runes("2"))
		m = pump(t, m, cmd)
		m, cmd = tpress(t, m, keyDown)
		m = pump(t, m, cmd)
		clock.advanceTo(clock.t.Add(holdStartWindow + time.Second))
		m, _ = tpress(t, m, keyDown)
		if m.refresh[surfaceDetails].cancel == nil || m.details.timerArmed {
			t.Errorf("a key after %v rested: run started %v, timer armed %v", holdStartWindow, m.refresh[surfaceDetails].cancel != nil, m.details.timerArmed)
		}
	})

	// Details waits for the frame it is cast over, and starts at once when the
	// frame commits.
	t.Run("prerequisite", func(t *testing.T) {
		m, stub := followFixture(t, Config{Accessible: true}, 132, 40)
		reports := stub.reports
		m, cmd := tpress(t, m, runes("d")) // a day not seen
		if m.refresh[surfaceDetails].cancel != nil || m.details.timerArmed {
			t.Fatal("Details started before the frame it is cast over")
		}
		startedAtCommit := false
		sawCommit := false
		m = pumpSeen(t, m, 400*time.Millisecond, func(m Model) {
			if !sawCommit && m.shown.key == mustTextKey(t, m) {
				sawCommit = true
				startedAtCommit = m.refresh[surfaceDetails].cancel != nil && !m.details.timerArmed
			}
		}, cmd)
		if !sawCommit || !startedAtCommit {
			t.Errorf("the frame committed (%v) and the report did not start with it (%v)", sawCommit, startedAtCommit)
		}
		if stub.reports != reports+1 {
			t.Errorf("%d report spawns for the day, want 1", stub.reports-reports)
		}
	})
}

// A hit writes to the large log what a fresh commit wrote: the stored argv lines,
// then the trace lines about the result itself (the rule applied after Astrolog),
// in the same order and never to the Log panel (gate 17, inspectable provenance).
// The fresh lines are cleared first, as the log's cap would have evicted them.
func TestMemoryHitReplaysTheTraceOfItsRule(t *testing.T) {
	logPanelLines := func(m Model) int {
		n := 0
		for _, l := range m.logLines {
			if l.lane == laneNote || l.lane == laneError {
				n++
			}
		}
		return n
	}
	evict := func(m Model) Model {
		m.logLines = nil
		return m
	}

	t.Run("kept_rows", func(t *testing.T) {
		fixture, err := os.ReadFile("../lookup/testdata/f3-venus-cross.txt")
		if err != nil {
			t.Fatal(err)
		}
		stubSpawns(t, func(s *rowSpawns) {
			s.reports.cross = func(context.Context, lookup.Lens, store.AnchorChart, time.Time, location.PlaceInfo, aspect.Object) (string, string, error) {
				return string(fixture), "astrolog -fixture", nil
			}
		})
		m := pairVenusModel(t)
		m.cfg.Color = ColorNever
		msg := runDetailsCmd(t, m)
		m, _ = m.commitRefresh(surfaceDetails, msg)
		want := []string{"astrolog -fixture", "details: kept the rows where Venus is 2's: 3 of 11 rows, totals dropped"}
		if got := consoleLineTexts(m); !slices.Equal(got, want) {
			t.Fatalf("fixture: the fresh commit wrote %q, want %q", got, want)
		}
		// The cursor leaves (the report goes) and comes back.
		m = evict(m)
		m.details.body, m.details.title, m.details.keyed = "", "", false
		panelBefore := logPanelLines(m)
		m, cmd := m.startDetailsNow()
		if cmd != nil || m.mem.hits[surfaceDetails] != 1 {
			t.Fatalf("fixture: the report was not drawn from memory (hits %d)", m.mem.hits[surfaceDetails])
		}
		if got := consoleLineTexts(m); !slices.Equal(got, want) {
			t.Errorf("the hit wrote %q, want the argv line then the kept-rows trace %q", got, want)
		}
		if logPanelLines(m) != panelBefore {
			t.Error("the hit wrote a note or an error to the Log panel")
		}
		if strings.Contains(m.details.body, "Uranus") {
			t.Error("the report drawn from memory is not the filtered one")
		}
	})

	t.Run("stars_left_out", func(t *testing.T) {
		m := detailsTestModel(t)
		m.focused = PanelChart
		m.lens.Registry = lookup.Hellenistic.Registry
		m.lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 89, 96}}
		msg := runDetailsCmd(t, m)
		m, _ = m.commitRefresh(surfaceDetails, msg)
		fresh := consoleLineTexts(m)
		if countStarsNote(m) != 1 {
			t.Fatalf("fixture: the fresh commit wrote %q", fresh)
		}
		m = evict(m)
		m.details.body, m.details.title, m.details.keyed = "", "", false
		m, _ = m.startDetailsNow()
		got := consoleLineTexts(m)
		if m.mem.hits[surfaceDetails] != 1 || !slices.Equal(got, fresh) {
			t.Errorf("hits %d, the hit wrote %q, want %q", m.mem.hits[surfaceDetails], got, fresh)
		}
	})

	templateTrace := func(t *testing.T) {
		stubBodyFacts(t, nil)
		m := activeOnTemplate(templateTestModel(t))
		m = runTemplate(t, m)
		fresh := consoleLineTexts(m)
		if templateNotes(m) == 0 {
			t.Fatalf("fixture: the fresh run wrote no template trace: %q", fresh)
		}
		m = evict(m)
		m.tpl.keyed, m.tpl.hasContent, m.tpl.lines = false, false, nil
		panelBefore := logPanelLines(m)
		m, cmd := m.syncTemplate()
		if cmd != nil || m.mem.hits[surfaceTemplate] != 1 {
			t.Fatalf("fixture: the result was not drawn from memory (hits %d)", m.mem.hits[surfaceTemplate])
		}
		if got := consoleLineTexts(m); !slices.Equal(got, fresh) {
			t.Errorf("the hit wrote %q, want the fresh run's %q", got, fresh)
		}
		if logPanelLines(m) != panelBefore {
			t.Error("the hit wrote a note or an error to the Log panel")
		}
	}
	t.Run("template_verdicts", templateTrace)
	t.Run("template_slot_branches", func(t *testing.T) {
		slotTemplate(t)
		templateTrace(t)
	})
}
