package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// specs/details-follows-its-tab.md C3-C5 for the Timeline and template tabs of
// Details, which are panes (Definitions) and follow the view rule with their own
// keys, timelineKey and templateKey: the content under the caption of its own
// subject is bright, the same subject at another version is faint with ` …`
// after the caption, and another subject, or nothing shown, is one faint `…`.

func TestTimelineFollowsTheViewRule(t *testing.T) {
	forceColor(t)
	m := loadedTimeline(t, Config{Color: ColorAlways})
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)

	// C3. The list for the key it asks for.
	if got := m.timelinePaneState(); got != paneCurrent {
		t.Fatalf("a loaded timeline: %v, want current", got)
	}
	if isDimmed(m) || strings.HasSuffix(stripTitle(m), detailsPendingMark) || !strings.Contains(detailsPanelText(m), "/2026") {
		t.Errorf("a current timeline is faint, marked or empty:\n%s", detailsPanelText(m))
	}

	// C4. The same compared charts in another month: the list stays, faint, and
	// the caption ends in ` …`.
	next, _ := nextMonth(m)
	if got := next.timelinePaneState(); got != paneUpdating {
		t.Fatalf("another month of the same charts: %v, want updating", got)
	}
	if !isDimmed(next) || !strings.HasSuffix(stripTitle(next), detailsPendingMark) || !strings.Contains(detailsPanelText(next), "/2026") {
		t.Errorf("an updating timeline is not the old list, faint, under a marked caption (%q):\n%s", stripTitle(next), detailsPanelText(next))
	}

	// C5. Another subject: nothing of the old list, one faint `…`, an unmarked caption.
	other := m
	other.timeline.content.natals = "another chart"
	if got := other.timelinePaneState(); got != paneEmpty {
		t.Fatalf("the list of other charts: %v, want empty", got)
	}
	text := detailsPanelText(other)
	if strings.Contains(text, "/2026") || !detailsBodyBlank(text) || strings.HasSuffix(stripTitle(other), detailsPendingMark) {
		t.Errorf("another subject's timeline is not blank under an unmarked caption (%q):\n%s", stripTitle(other), text)
	}
	// Nothing shown at all.
	blank := m
	blank.timeline.hasContent = false
	if got := blank.timelinePaneState(); got != paneEmpty {
		t.Errorf("nothing shown: %v, want empty", got)
	}
}

func TestTemplateTabFollowsTheViewRule(t *testing.T) {
	forceColor(t)
	freshModels(t)
	stubBodyFacts(t, nil)
	m := templateTestModel(t)
	m.cfg = Config{Color: ColorAlways}
	m = runTemplate(t, activeOnTemplate(m))
	id := m.activeTabID(PanelDetails)

	if got := m.templatePaneState(); got != paneCurrent || m.detailsTabDimmed(id) || isDimmed(m) {
		t.Fatalf("a run template: %v dimmed %v, want current and bright", got, isDimmed(m))
	}
	// C4. A new moment of the same body: the lines stay, faint, caption marked.
	moved := m
	moved.slots.SetFloatingTime(m.slots.Floating().Time.Add(time.Hour))
	moved, _ = moved.syncTemplate()
	if got := moved.templatePaneState(); got != paneUpdating || !isDimmed(moved) || !strings.HasSuffix(stripTitle(moved), detailsPendingMark) {
		t.Errorf("another moment: %v dimmed %v caption %q, want updating, faint, marked", got, isDimmed(moved), stripTitle(moved))
	}
	// C5. Another body: its first run, nothing of the other body's lines.
	m.tpl.key.object = objMercury.Number
	m, _ = m.syncTemplate()
	text := detailsPanelText(m)
	if got := m.templatePaneState(); got != paneEmpty || !detailsBodyBlank(text) || strings.HasSuffix(stripTitle(m), detailsPendingMark) {
		t.Errorf("another subject: %v, caption %q:\n%s", got, stripTitle(m), text)
	}
}

// C5, "or nothing is shown": before the first text frame has ever committed,
// Bodies and Aspects read the one faint `…` and nothing else, and the frame's
// commit replaces it.
func TestPanesReadOneEllipsisBeforeTheFirstFrame(t *testing.T) {
	forceColor(t)
	m, _ := followFixture(t, Config{Color: ColorAlways}, 132, 40)
	committed := m
	m.shown, m.chart = shownFrame{}, chartViewState{}
	for _, p := range []PanelID{PanelBodies, PanelAspects} {
		if got := m.textPaneState(p); got != paneEmpty {
			t.Errorf("%v before any frame: %v, want empty", p, got)
		}
	}
	view := m.View()
	if n := strings.Count(view, term.SGRFaint+emptyPaneLine); n < 2 {
		t.Errorf("the panels do not each read a faint %q before the first frame:\n%s", emptyPaneLine, view)
	}
	if plain := strings.Join(strings.Fields(ansi.Strip(view)), " "); strings.Contains(plain, "☉ Sun") {
		t.Errorf("rows before any frame:\n%s", view)
	}
	// And once the frame has committed they are the frame's.
	for _, p := range []PanelID{PanelBodies, PanelAspects} {
		if got := committed.textPaneState(p); got != paneCurrent {
			t.Errorf("%v after the frame: %v, want current", p, got)
		}
	}
}
