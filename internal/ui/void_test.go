package ui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

// voidCounter stands in for the real void search: it counts the spawns and
// answers with an empty result.
type voidCounter struct {
	mu    sync.Mutex
	asked []lookup.VoidRequest
}

func (v *voidCounter) n() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.asked)
}

func stubVoids(t *testing.T) *voidCounter {
	t.Helper()
	v := &voidCounter{}
	stubVoidsAhead(t, func(_ context.Context, req lookup.VoidRequest) (lookup.VoidResult, error) {
		v.mu.Lock()
		v.asked = append(v.asked, req)
		v.mu.Unlock()
		return lookup.VoidResult{Command: "astrolog void"}, nil
	})
	return v
}

// moonRows is the lines of a canned sky output that name the Moon: what the
// void search prints.
func moonRows(out string) string {
	var moon []string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "Moon") {
			moon = append(moon, l)
		}
	}
	return strings.Join(moon, "\n") + "\n"
}

// L6: a void search that landed is remembered in the session, so a second
// timeline search for the same month spawns none; a changed zodiac asks again.
func TestVoidSearchIsRemembered(t *testing.T) {
	s := &mergeSearcher{}
	stubTimelineSearch(t, s)
	voids := stubVoids(t)
	m := loadTimeline(t, octoberSet(t, 0))
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
	first := voids.n()
	if first == 0 {
		t.Fatal("the first timeline search spawned no void search")
	}
	if len(m.mem.voids.items) != first {
		t.Fatalf("memory holds %d void results, want the %d that landed", len(m.mem.voids.items), first)
	}

	// Another timeline search for the same month (a mark changes the key).
	m.slots.ToggleObjectSelection(len(m.slots.All())-1, objMars)
	m = loadTimeline(t, m)
	if got := voids.n(); got != first {
		t.Errorf("a second search for the same month spawned %d more void searches, want none", got-first)
	}
	if len(m.timelineRows()) == 0 {
		t.Fatal("the second search showed no rows")
	}

	// A different zodiac is a different void search.
	m, cmd := m.chooseZodiac(0, recipe.Lahiri)
	m = pump(t, m, cmd)
	if got := voids.n(); got == first {
		t.Error("a changed zodiac spawned no void search")
	}
}

// L3b: a failed void search fails the sky search, and the Log names it exactly
// as it names a failed sky search.
func TestVoidFailureIsNamedAsSkyFailure(t *testing.T) {
	s := &mergeSearcher{}
	stubTimelineSearch(t, s)
	stubSpawns(t, func(sp *rowSpawns) {
		sp.voids = func(_ context.Context, req lookup.VoidRequest) (lookup.VoidResult, error) {
			if req.Offset != 7200 { // the CET segment of the month has nothing, as in the sky failure test
				return lookup.VoidResult{}, nil
			}
			return lookup.VoidResult{Command: "astrolog void"}, errors.New("astrolog exec: exit status 1")
		}
	})
	m := loadTimeline(t, octoberSet(t, 1))
	for _, r := range m.timelineRows() {
		if r.Numeral == 0 {
			t.Errorf("a sky row %q survived a failed void search", r.Line)
		}
	}
	lines := errorLines(m)
	if len(lines) != 1 || !strings.Contains(lines[0], "sky") || !strings.Contains(lines[0], "(-d)") {
		t.Errorf("error lane = %q, want one line naming the sky search (-d)", lines)
	}
}
