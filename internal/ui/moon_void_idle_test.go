package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// Slice B, idle time and stepping: the windows searched ahead.

// leafMsgs runs a command and every command it holds, in order, without
// delivering anything.
func leafMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, leafMsgs(c)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

// B4c
func TestVoidWindowsAreSearchedAhead(t *testing.T) {
	stubColumnLookups(t, nil)
	voids := stubVoids(t)
	m := moonModel(t, 2)
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	m.slots.SetFloatingTime(now)
	m.targetFloatingTime = now
	m = rememberVoid(t, m, 2, false) // the frame's own window is known
	m, cmd0 := update(m, tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m = runChain(t, m, cmd0) // the new Now's frame commits
	if voids.n() != 0 {
		t.Fatalf("fixture: %d void searches before the period", voids.n())
	}
	for sf := range m.refresh { // the fixture's own arrival runs are not pumped: nothing is computing
		m.refresh[sf].cancel = nil
	}
	// Now is mid-May: its window starts in 2025, two months on is July (2026),
	// two months back is March (2025). The charts were born in 1961 and 1962.
	want := []int{2026, 1960, 1961}

	// Another surface computing: the idle period waits and asks nothing.
	busy := m
	busy.refresh[surfaceText].cancel = func() {}
	busy, cmd := update(busy, prefetchIdleMsg{seq: busy.pre.seq})
	_ = leafMsgs(cmd)
	if got := voids.n(); got != 0 {
		t.Fatalf("%d searches while another surface was computing, want none", got)
	}

	// Quiet: one void search per turn, in order. Everything else the period
	// would ask is stubbed above; only the void searches are counted here.
	m, cmd = update(m, prefetchIdleMsg{seq: m.pre.seq})
	var got []int
	for turns := 0; cmd != nil && turns < 40; turns++ {
		before := voids.n()
		msgs := leafMsgs(cmd)
		if d := voids.n() - before; d > 1 {
			t.Fatalf("turn %d asked %d void searches, want at most one", turns, d)
		} else if d == 1 {
			got = append(got, lookup.VoidFirstYear(voids.asked[before].Moment, 0))
		}
		cmd = nil
		for _, msg := range msgs {
			var next tea.Cmd
			m, next = update(m, msg)
			if next != nil {
				cmd = next
			}
		}
	}
	if len(got) != len(want) {
		t.Fatalf("idle turns searched windows starting %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("idle search %d began in %d, want %d (all: %v)", i+1, got[i], want[i], got)
		}
	}
	for _, y := range want {
		found := false
		for k := range m.mem.voids.items {
			if m.mem.voids.items[k].key.FirstYear == y {
				found = true
			}
		}
		if !found {
			t.Errorf("window %d is not in memory after the period", y)
		}
	}
}

// B4d
func TestSteppingInsideAKnownWindowSpawnsNothing(t *testing.T) {
	stubColumnLookups(t, nil)
	voids := stubVoids(t)
	m := moonModel(t, 2)
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	m.slots.SetFloatingTime(now)
	m.targetFloatingTime = now
	m = rememberVoid(t, m, 2, false)
	for i := 0; i < 5; i++ {
		var cmd tea.Cmd
		m, cmd = tpress(t, m, runes("h"))
		m = pump(t, m, cmd)
	}
	if m.slots.Floating().Time.Equal(now) {
		t.Fatalf("fixture: five steps left Now at %v", m.slots.Floating().Time)
	}
	if got := voids.n(); got != 0 {
		t.Errorf("stepping inside a remembered window spawned %d void searches, want none", got)
	}
}
