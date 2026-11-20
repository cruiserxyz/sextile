package ui

import (
	"context"
	"errors"
	"strings"
	"sync"
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

// Slice B of specs/moon-observatory.md: the Moon's lane in Bodies.

const speedBarRunes = "▏▎▍▌▋▊▉█"

// moonModel is a Model with a committed frame of the full chart, no selection
// (so Bodies lists one chart), the Hellenistic registry (it names a Moon), and
// the cursor on slot cursor.
func moonModel(t *testing.T, cursor int) Model {
	t.Helper()
	m := listModel(t, 2, cursor)
	m.cfg = Config{Color: ColorNever}
	m.lens.Registry = lookup.Hellenistic.Registry
	m.slots.ToggleSlotSelection(0)
	m.slots.ToggleSlotSelection(2)
	return withFrame(m)
}

// bodiesText is the Bodies body as the panel draws it for the Model's chart.
func bodiesText(m Model) string {
	bag := resolveBodiesBag(m.bodiesView(), m.paintedChart(), m.slots, m.cursorIdx, m.bodyCursorIdx, 70, 40, false, m.cfg)
	bag.Top = 0
	body, _, _ := panel.RenderBodies(bag)
	return ansi.Strip(body)
}

func bodyRowNamed(t *testing.T, text, name string) string {
	t.Helper()
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, name) {
			return l
		}
	}
	t.Fatalf("no %s row in:\n%s", name, text)
	return ""
}

// rememberVoid files the slot's window in memory with the slot's moment inside
// a void (inVoid) or between two.
func rememberVoid(t *testing.T, m Model, idx int, inVoid bool) Model {
	t.Helper()
	s := m.slots.All()[idx]
	_, key, ok := m.voidRequestForSlot(s)
	if !ok {
		t.Fatal("fixture: no void request for the slot")
	}
	start := s.Time.Add(24 * time.Hour)
	if inVoid {
		start = s.Time.Add(-time.Hour)
	}
	m.mem.voids = m.mem.voids.put(key, lookup.VoidResult{Voids: []lookup.VoidPeriod{{Start: start, End: start.Add(2 * time.Hour)}}})
	return m
}

func update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	tm, cmd := m.Update(msg)
	return tm.(Model), cmd
}

// nudge is the ask that follows a committed frame, with everything it starts run.
func nudge(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := m.askShownVoid()
	return runChain(t, m, cmd)
}

func hasBar(row string) bool { return strings.ContainsAny(row, speedBarRunes) }

// B1
func TestBodiesMoonVoid(t *testing.T) {
	m := rememberVoid(t, moonModel(t, 2), 2, true)
	row := bodyRowNamed(t, bodiesText(m), "Moon")
	if !strings.Contains(row, "v/c") || hasBar(row) {
		t.Errorf("Moon row = %q, want v/c and no bar", row)
	}
	if !hasBar(bodyRowNamed(t, bodiesText(m), "Sun")) {
		t.Error("the Sun row lost its bar: only the Moon has a lane")
	}
}

// B2
func TestBodiesMoonNotVoid(t *testing.T) {
	m := rememberVoid(t, moonModel(t, 2), 2, false)
	row := bodyRowNamed(t, bodiesText(m), "Moon")
	if hasBar(row) || strings.ContainsAny(row, "·?") || strings.Contains(row, "v/c") {
		t.Errorf("Moon row = %q, want blank speed cells", row)
	}
	// The lane keeps its three cells: the row is as long as it is with a mark.
	marked := bodyRowNamed(t, bodiesText(rememberVoid(t, moonModel(t, 2), 2, true)), "Moon")
	if len([]rune(row)) != len([]rune(marked)) {
		t.Errorf("blank row is %d cells, v/c row %d: the lane must keep its three cells", len([]rune(row)), len([]rune(marked)))
	}
}

func traceLinesAbout(m Model, word string) (trace, other []string) {
	for _, l := range m.logLines {
		if !strings.Contains(l.text, word) {
			continue
		}
		if l.lane == laneTrace {
			trace = append(trace, l.text)
		} else {
			other = append(other, l.text)
		}
	}
	return
}

// B3
func TestBodiesMoonVoidUnknown(t *testing.T) {
	var mu sync.Mutex
	asked := 0
	stubVoidsAhead(t, func(context.Context, lookup.VoidRequest) (lookup.VoidResult, error) {
		mu.Lock()
		asked++
		mu.Unlock()
		return lookup.VoidResult{}, errors.New("astrolog exec: exit status 1")
	})
	m := nudge(t, moonModel(t, 2))
	if row := bodyRowNamed(t, bodiesText(m), "Moon"); !strings.Contains(row, "?") || hasBar(row) {
		t.Errorf("Moon row = %q, want ? and no bar", row)
	}
	trace, other := traceLinesAbout(m, "void search")
	if len(trace) != 1 || len(other) != 0 {
		t.Errorf("trace lane %q, other lanes %q; want one trace line and nothing else", trace, other)
	}
	m = nudge(t, m)
	mu.Lock()
	defer mu.Unlock()
	if asked != 1 {
		t.Errorf("%d searches for a window that failed, want its one", asked)
	}
}

// leaves runs a command and everything a batch holds at once, delivering each
// leaf message on the channel.
func leaves(cmd tea.Cmd) <-chan tea.Msg {
	out := make(chan tea.Msg, 64)
	var run func(c tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		go func() {
			switch msg := c().(type) {
			case nil:
			case tea.BatchMsg:
				for _, sub := range msg {
					run(sub)
				}
			default:
				out <- msg
			}
		}()
	}
	run(cmd)
	return out
}

// B4
func TestBodiesFrameNeverWaitsForTheVoidSearch(t *testing.T) {
	release := make(chan struct{})
	stubVoidsAhead(t, func(context.Context, lookup.VoidRequest) (lookup.VoidResult, error) {
		<-release
		return lookup.VoidResult{}, nil
	})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	t.Cleanup(free)

	m := moonModel(t, 2)
	done := make(chan struct{})
	var cmd tea.Cmd
	go func() {
		m, cmd = m.askShownVoid()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Update waited for the void search")
	}
	msgs := leaves(cmd)
	if row := bodyRowNamed(t, bodiesText(m), "Moon"); !strings.Contains(row, "·") || hasBar(row) {
		t.Errorf("Moon row while the search is held = %q, want ·", row)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Moon") {
		t.Error("the frame is not on screen while the search is held")
	}
	free()
	for {
		select {
		case msg := <-msgs:
			if landed, ok := msg.(voidLandedMsg); ok {
				m, _ = update(m, landed)
				if row := bodyRowNamed(t, bodiesText(m), "Moon"); strings.Contains(row, "·") {
					t.Errorf("Moon row after the landing = %q, want the lane settled", row)
				}
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the released search never landed")
		}
	}
}

// B4a
func TestVoidLandingRedrawsOnlyTheLane(t *testing.T) {
	m := moonModel(t, 2)
	before := strings.Split(ansi.Strip(m.View()), "\n")
	s := m.slots.All()[2]
	_, key, _ := m.voidRequestForSlot(s)
	res := lookup.VoidResult{Voids: []lookup.VoidPeriod{{Start: s.Time.Add(-time.Hour), End: s.Time.Add(time.Hour)}}}
	m, _ = update(m, voidLandedMsg{key: key, res: res})
	after := strings.Split(ansi.Strip(m.View()), "\n")
	if len(before) != len(after) {
		t.Fatalf("the screen went from %d to %d lines", len(before), len(after))
	}
	type cell struct{ line, col int }
	var diff []cell
	for i := range before {
		b, a := []rune(before[i]), []rune(after[i])
		for c := 0; c < max(len(a), len(b)); c++ {
			var x, y rune = ' ', ' '
			if c < len(b) {
				x = b[c]
			}
			if c < len(a) {
				y = a[c]
			}
			if x != y {
				diff = append(diff, cell{i, c})
			}
		}
	}
	if len(diff) == 0 || len(diff) > 3 {
		t.Fatalf("%d cells changed, want the Moon's lane (1 to 3 cells): %v", len(diff), diff)
	}
	for _, d := range diff {
		if d.line != diff[0].line || d.col-diff[0].col > 2 {
			t.Fatalf("changed cells %v are not one lane of three cells", diff)
		}
	}
	if !strings.Contains(after[diff[0].line], "Moon") || !strings.Contains(after[diff[0].line], "v/c") || !strings.Contains(before[diff[0].line], "·") {
		t.Errorf("the changed row went %q -> %q, want the Moon's · -> v/c", before[diff[0].line], after[diff[0].line])
	}
}

// B4b
func TestOneVoidSearchServesEveryChartInItsWindow(t *testing.T) {
	voids := stubVoids(t)
	m := moonModel(t, 0)
	place := location.PlaceInfo{Name: "Testford", Country: "TS", Lat: 41, Lon: -71, TZ: "America/New_York"}
	chart := store.SessionChart{ID: "second", Name: "Second", Source: store.ChartSource{Kind: store.ChartKindFile, File: "second.as"}}
	// Slot 0 was born on 11 January 1961 (window 1960): so was this one, in March.
	if err := m.slots.AddAnchored(chart, time.Date(1961, 3, 5, 9, 0, 0, 0, time.UTC), place); err != nil {
		t.Fatal(err)
	}
	m = nudge(t, m)
	if got := voids.n(); got != 1 {
		t.Fatalf("the first chart asked %d searches, want 1", got)
	}
	// The newly added chart is the one before Now: cursor there.
	second := len(m.slots.All()) - 2
	if m.slots.All()[second].Chart.ID != "second" {
		t.Fatalf("fixture: slot %d is %s", second, m.slots.All()[second].Chart.ID)
	}
	m.cursorIdx = second
	m = nudge(t, m)
	if got := voids.n(); got != 1 {
		t.Errorf("a second chart in the same window and zodiac made %d searches, want the one", got)
	}
	if req := voids.asked[0]; req.Offset != 0 {
		t.Errorf("the search was asked at offset %d, want universal time", req.Offset)
	}
	m, cmd := m.chooseZodiac(second, recipe.Lahiri)
	m = runChain(t, m, cmd)
	m = nudge(t, m)
	if got := voids.n(); got != 2 {
		t.Errorf("a sidereal chart in the same window made %d searches in all, want a second", got)
	}
}

// B5
func TestBodiesVoidFollowsTheCursorChart(t *testing.T) {
	m := moonModel(t, 0)
	m = rememberVoid(t, m, 0, true)
	m = rememberVoid(t, m, 2, false)
	if row := bodyRowNamed(t, bodiesText(m), "Moon"); !strings.Contains(row, "v/c") {
		t.Errorf("cursor on the anchored chart: Moon row = %q, want v/c", row)
	}
	m.cursorIdx = 2
	if row := bodyRowNamed(t, bodiesText(m), "Moon"); strings.Contains(row, "v/c") || hasBar(row) {
		t.Errorf("cursor on Now: Moon row = %q, want blank", row)
	}

	// An object the registry does not call Moon gets no lane, and neither does
	// any other row.
	reg := append(aspect.Registry(nil), lookup.Hellenistic.Registry...)
	for i := range reg {
		if reg[i].Name == "Moon" {
			reg[i].Name = "Luna"
		}
	}
	m.lens.Registry = reg
	text := bodiesText(m)
	if row := bodyRowNamed(t, text, "Moon"); strings.Contains(row, "v/c") || !hasBar(row) {
		t.Errorf("registry without a Moon: row = %q, want the ordinary speed bar", row)
	}
	if hasBar(bodyRowNamed(t, bodiesText(rememberVoid(t, moonModel(t, 0), 0, true)), "Sun")) == false {
		t.Error("the Sun row has no bar")
	}
}
