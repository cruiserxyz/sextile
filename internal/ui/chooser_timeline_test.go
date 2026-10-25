package ui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// bandAwareSearch is weekSearch that honours the band it is handed the way
// Astrolog does: a main `-R0` list without the Moon (object 2) leaves no row
// naming the Moon. A search with no such band returns the week unchanged, so a
// timeline that ignored the choosers still shows the Moon and fails below.
func bandAwareSearch(ctx context.Context, call lookup.BodyCall) (string, string, error) {
	out, cmd, err := weekSearch(ctx, call)
	i := slices.Index(call.Argv(), "-R0")
	if err != nil || i < 0 {
		return out, cmd, err
	}
	var nums []string
	for _, a := range call.Argv()[i+1:] {
		if strings.HasPrefix(a, "-") {
			break
		}
		nums = append(nums, a)
	}
	if slices.Contains(nums, "2") {
		return out, cmd, nil
	}
	var kept []string
	for _, l := range strings.Split(out, "\n") {
		if !strings.Contains(l, "Moon") {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n"), cmd, nil
}

// bandAwareTimeline installs the timeline seam as the real lookup path over
// bandAwareSearch, counting every call. Rows therefore depend on the request's
// lens exactly as far as lookup and the argv carry it.
func bandAwareTimeline(t *testing.T) *[]timelineCall {
	defaultClock(t)
	t.Helper()
	return stubTimeline(t, func(req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		return lookup.TimelineWith(context.Background(), req, bandAwareSearch, stubDefaults)
	})
}

// wholeTimeline renders every row through the real panel renderer: a panel
// tall enough for the whole list, so no row can hide below the viewport.
func wholeTimeline(m Model) []string {
	return detailsBodyRows(renderDetailsPanel(80, len(m.timeline.rows)+2, (m.detailsFrame()).state.attached, m.cfg, m.detailsFrame()))
}

func countContaining(rows []string, needle string) int {
	n := 0
	for _, r := range rows {
		if strings.Contains(r, needle) {
			n++
		}
	}
	return n
}

// untickMoonInBodies drives ⌃b the way an operator does: filter to the Moon,
// Space to untick it. It stops before Enter.
func untickMoonInBodies(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if !m.modalOpen() {
		t.Fatal("⌃b did not open the body selector")
	}
	m, _ = tpress(t, m, runes("moon"))
	var cmd tea.Cmd
	m, cmd = tpress(t, m, tea.KeyMsg{Type: tea.KeySpace})
	return pump(t, m, cmd)
}

// C13, C14: unticking the Moon in ⌃b and confirming, through real key
// dispatch, removes the Moon from the RENDERED timeline while the Sun stays, and
// re-runs the searches once. A confirmed ⌃a change does the same.
func TestConfirmedChooserChangeReSearchesTimeline(t *testing.T) {
	calls := bandAwareTimeline(t)
	stubTextCompute(t)
	m := loadedTimeline(t, Config{Accessible: true})
	m.aspectRegistry = testRegistry()
	m.objectRegistry = testObjectRegistry()
	m, _ = tpress(t, m, runes("0"))

	before := wholeTimeline(m)
	if countContaining(before, "Moon") == 0 || countContaining(before, "Sun") == 0 {
		t.Fatalf("fixture: the unfiltered timeline must name both the Moon and the Sun:\n%s", strings.Join(before, "\n"))
	}
	spawned := len(*calls)
	keyBefore := m.timeline.key

	m = untickMoonInBodies(t, m)
	var cmd tea.Cmd
	m, cmd = tpress(t, m, keyEnter)
	m = pump(t, m, cmd)
	if m.modalOpen() {
		t.Fatal("Enter left ⌃b open")
	}
	if got := len(*calls) - spawned; got != 1 {
		t.Errorf("confirming ⌃b ran the timeline %d times, want exactly once", got)
	}
	if m.timeline.key == keyBefore {
		t.Error("the timeline key did not change with the confirmed body policy")
	}
	after := wholeTimeline(m)
	if n := countContaining(after, "Moon"); n != 0 {
		t.Errorf("%d rendered rows still name the Moon after it was unticked:\n%s", n, strings.Join(after, "\n"))
	}
	if countContaining(after, "Sun") == 0 {
		t.Errorf("no rendered row names the Sun: an empty timeline must not pass:\n%s", strings.Join(after, "\n"))
	}
	if len(after) == 0 || len(after) >= len(before) {
		t.Errorf("rendered %d rows after, %d before; want fewer but some", len(after), len(before))
	}

	// ⌃a: the chosen aspects reach the timeline's request, and a confirm
	// re-runs it once. Space ticks the first row, which changes the policy.
	spawned, keyBefore = len(*calls), m.timeline.key
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyCtrlA})
	if !m.modalOpen() {
		t.Fatal("⌃a did not open the aspect chooser")
	}
	m, cmd = tpress(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = pump(t, m, cmd)
	m, cmd = tpress(t, m, keyEnter)
	m = pump(t, m, cmd)
	if got := len(*calls) - spawned; got != 1 {
		t.Errorf("confirming ⌃a ran the timeline %d times, want exactly once", got)
	}
	if m.timeline.key == keyBefore {
		t.Error("the timeline key did not change with the confirmed aspect policy")
	}
	// The scripted searcher ignores aspects, so the list must be exactly what
	// the body choice left. A UI that filtered on its own after lookup handed
	// the rows over — below the first viewport, where a one-screen check cannot
	// see it — would differ here.
	if got := wholeTimeline(m); !slices.Equal(got, after) {
		t.Errorf("the aspect confirm changed the rendered rows although the searcher ignores aspects:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(after, "\n"))
	}
	last := (*calls)[len(*calls)-1].req
	if !last.Lens.Aspects.Explicit {
		t.Errorf("the re-run request carries aspects %+v, want the confirmed explicit set", last.Lens.Aspects)
	}
}

// C15: while ⌃b or ⌃a is open, toggling previews the chart but spawns no
// timeline search, and Esc leaves the key, rows and spawn count as they were.
func TestChooserPreviewSpawnsNoTimelineSearch(t *testing.T) {
	oldChooserRule(t)
	for _, open := range []struct {
		name  string
		key   tea.KeyMsg
		typed string
	}{
		{"body selector", tea.KeyMsg{Type: tea.KeyCtrlB}, "moon"},
		{"aspect chooser", tea.KeyMsg{Type: tea.KeyCtrlA}, ""},
	} {
		t.Run(open.name, func(t *testing.T) {
			calls := bandAwareTimeline(t)
			text := stubTextCompute(t)
			m := loadedTimeline(t, Config{Accessible: true})
			m.aspectRegistry = testRegistry()
			m.objectRegistry = testObjectRegistry()
			m, _ = tpress(t, m, runes("0"))
			rowsBefore, keyBefore, spawned := wholeTimeline(m), m.timeline.key, len(*calls)
			textBefore := len(*text)

			m, _ = tpress(t, m, open.key)
			if !m.modalOpen() {
				t.Fatalf("%s did not open", open.name)
			}
			if open.typed != "" {
				m, _ = tpress(t, m, runes(open.typed))
			}
			lensOpen := m.lens
			var cmd tea.Cmd
			m, cmd = tpress(t, m, tea.KeyMsg{Type: tea.KeySpace})
			m = pump(t, m, cmd)
			if len(*text) == textBefore {
				t.Fatal("fixture: the toggle did not preview the chart, so a silent timeline proves nothing")
			}
			if lensOpen.Objects.Selected() != nil && slices.Equal(lensOpen.Objects.Selected(), m.lens.Objects.Selected()) && lensOpen.Aspects.Explicit == m.lens.Aspects.Explicit {
				t.Fatal("fixture: the toggle did not change the live lens")
			}
			if len(*calls) != spawned {
				t.Errorf("the preview ran the timeline %d times", len(*calls)-spawned)
			}
			if m.timeline.key != keyBefore {
				t.Error("the preview changed the timeline's key")
			}

			m, cmd = tpress(t, m, keyEsc)
			m = pump(t, m, cmd)
			if m.modalOpen() {
				t.Fatal("Esc left the modal open")
			}
			if len(*calls) != spawned {
				t.Errorf("Cancel ran the timeline %d times", len(*calls)-spawned)
			}
			if m.timeline.key != keyBefore {
				t.Error("Cancel left the timeline on another key")
			}
			if got := wholeTimeline(m); !slices.Equal(got, rowsBefore) {
				t.Errorf("Cancel changed the rendered rows:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(rowsBefore, "\n"))
			}
		})
	}
}

// C12a: when the default sets cannot be read, no search runs, the tab holds no
// rows, and the refusal is the line the operator reads.
func TestTimelineShowsDefaultSetRefusal(t *testing.T) {
	defaultClock(t)
	searches := 0
	counting := func(ctx context.Context, call lookup.BodyCall) (string, string, error) {
		searches++
		return bandAwareSearch(ctx, call)
	}
	stubTimeline(t, func(req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		return lookup.TimelineWith(context.Background(), req, counting, func(context.Context) (lookup.DefaultSets, error) {
			return lookup.DefaultSets{}, errors.New("astrolog: -R1 -HO: no object rows found")
		})
	})
	m := timelineModel(t, Config{Accessible: true})
	m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
	m, cmd := syncTimelineVisited(m)
	m = pump(t, m, cmd)
	m, _ = tpress(t, m, runes("0"))

	if searches != 0 {
		t.Errorf("%d searches ran with no default sets", searches)
	}
	shown := strings.Join(detailsBodyRows(renderDetails(m)), "\n")
	if len(m.timeline.rows) != 0 || strings.TrimSpace(shown) != "" {
		t.Errorf("the tab holds %d rows after a refusal, and renders:\n%s", len(m.timeline.rows), shown)
	}
	var lane []string
	for _, l := range m.logLines {
		if l.lane == laneError {
			lane = append(lane, l.text)
		}
	}
	const want = "Couldn't read the timeline. Astrolog's default body lists could not be read"
	if len(lane) != 1 || !strings.HasPrefix(lane[0], want) {
		t.Fatalf("error lane = %q, want one line starting %q", lane, want)
	}
	// The Log panel cuts a long line at its edge (Enter copies the whole), so
	// the screen shows the line's opening words.
	if !strings.Contains(ansi.Strip(m.View()), "Couldn't read the timeline. Astrolog's default body lists") {
		t.Error("the refusal is not on the rendered screen")
	}
}
