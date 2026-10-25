package ui

import (
	"context"
	"errors"
	"github.com/charmbracelet/x/ansi"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the contract lane. Test names match
// the C-row verifiers verbatim.

// forceColor makes lipgloss emit SGR for the duration of one test. Off a TTY
// it otherwise downgrades to the ASCII profile and strips every style, which
// would make bold/foreground assertions vacuous.
func forceColor(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

// legacyTopBorders are the pre-an earlier change renderTopBorder outputs, captured from
// the live function verbatim (forced TrueColor profile for the color
// rows) before the strip landed. C1 holds every one-tab strip to them.
// The focused accessible rows carry the colours-off focus mark (`>`) that
// the specification C2 added inside the title's bold run.
var legacyTopBorders = []struct {
	mode    string
	width   int
	focused bool
	title   string
	want    string
}{
	{"color", 66, true, "[1] Chart", "\x1b[92m╭─\x1b[0m\x1b[1;92m[1] Chart\x1b[0m\x1b[92m──────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 66, true, "[2] Bodies", "\x1b[92m╭─\x1b[0m\x1b[1;92m[2] Bodies\x1b[0m\x1b[92m─────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 66, true, "[3] Aspects", "\x1b[92m╭─\x1b[0m\x1b[1;92m[3] Aspects\x1b[0m\x1b[92m────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 66, true, "[0] Details", "\x1b[92m╭─\x1b[0m\x1b[1;92m[0] Details\x1b[0m\x1b[92m────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 66, true, "Log", "\x1b[92m╭─\x1b[0m\x1b[1;92mLog\x1b[0m\x1b[92m────────────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 66, false, "[1] Chart", "\x1b[38;5;240m╭─\x1b[0m\x1b[38;5;252m[1] Chart\x1b[0m\x1b[38;5;240m──────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 66, false, "[2] Bodies", "\x1b[38;5;240m╭─\x1b[0m\x1b[38;5;252m[2] Bodies\x1b[0m\x1b[38;5;240m─────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 66, false, "[3] Aspects", "\x1b[38;5;240m╭─\x1b[0m\x1b[38;5;252m[3] Aspects\x1b[0m\x1b[38;5;240m────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 66, false, "[0] Details", "\x1b[38;5;240m╭─\x1b[0m\x1b[38;5;252m[0] Details\x1b[0m\x1b[38;5;240m────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 66, false, "Log", "\x1b[38;5;240m╭─\x1b[0m\x1b[38;5;252mLog\x1b[0m\x1b[38;5;240m────────────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 132, true, "[1] Chart", "\x1b[92m╭─\x1b[0m\x1b[1;92m[1] Chart\x1b[0m\x1b[92m────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 132, true, "[2] Bodies", "\x1b[92m╭─\x1b[0m\x1b[1;92m[2] Bodies\x1b[0m\x1b[92m───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 132, true, "[3] Aspects", "\x1b[92m╭─\x1b[0m\x1b[1;92m[3] Aspects\x1b[0m\x1b[92m──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 132, true, "[0] Details", "\x1b[92m╭─\x1b[0m\x1b[1;92m[0] Details\x1b[0m\x1b[92m──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 132, true, "Log", "\x1b[92m╭─\x1b[0m\x1b[1;92mLog\x1b[0m\x1b[92m──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 132, false, "[1] Chart", "\x1b[38;5;240m╭─\x1b[0m\x1b[38;5;252m[1] Chart\x1b[0m\x1b[38;5;240m────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 132, false, "[2] Bodies", "\x1b[38;5;240m╭─\x1b[0m\x1b[38;5;252m[2] Bodies\x1b[0m\x1b[38;5;240m───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 132, false, "[3] Aspects", "\x1b[38;5;240m╭─\x1b[0m\x1b[38;5;252m[3] Aspects\x1b[0m\x1b[38;5;240m──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 132, false, "[0] Details", "\x1b[38;5;240m╭─\x1b[0m\x1b[38;5;252m[0] Details\x1b[0m\x1b[38;5;240m──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 132, false, "Log", "\x1b[38;5;240m╭─\x1b[0m\x1b[38;5;252mLog\x1b[0m\x1b[38;5;240m──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮\x1b[0m"},
	{"accessible", 66, true, "[1] Chart", "+-\x1b[1m>[1] Chart\x1b[0m-----------------------------------------------------+"},
	{"accessible", 66, true, "[2] Bodies", "+-\x1b[1m>[2] Bodies\x1b[0m----------------------------------------------------+"},
	{"accessible", 66, true, "[3] Aspects", "+-\x1b[1m>[3] Aspects\x1b[0m---------------------------------------------------+"},
	{"accessible", 66, true, "[0] Details", "+-\x1b[1m>[0] Details\x1b[0m---------------------------------------------------+"},
	{"accessible", 66, true, "Log", "+-\x1b[1m>Log\x1b[0m-----------------------------------------------------------+"},
	{"accessible", 66, false, "[1] Chart", "+-[1] Chart------------------------------------------------------+"},
	{"accessible", 66, false, "[2] Bodies", "+-[2] Bodies-----------------------------------------------------+"},
	{"accessible", 66, false, "[3] Aspects", "+-[3] Aspects----------------------------------------------------+"},
	{"accessible", 66, false, "[0] Details", "+-[0] Details----------------------------------------------------+"},
	{"accessible", 66, false, "Log", "+-Log------------------------------------------------------------+"},
	{"accessible", 132, true, "[1] Chart", "+-\x1b[1m>[1] Chart\x1b[0m-----------------------------------------------------------------------------------------------------------------------+"},
	{"accessible", 132, true, "[2] Bodies", "+-\x1b[1m>[2] Bodies\x1b[0m----------------------------------------------------------------------------------------------------------------------+"},
	{"accessible", 132, true, "[3] Aspects", "+-\x1b[1m>[3] Aspects\x1b[0m---------------------------------------------------------------------------------------------------------------------+"},
	{"accessible", 132, true, "[0] Details", "+-\x1b[1m>[0] Details\x1b[0m---------------------------------------------------------------------------------------------------------------------+"},
	{"accessible", 132, true, "Log", "+-\x1b[1m>Log\x1b[0m-----------------------------------------------------------------------------------------------------------------------------+"},
	{"accessible", 132, false, "[1] Chart", "+-[1] Chart------------------------------------------------------------------------------------------------------------------------+"},
	{"accessible", 132, false, "[2] Bodies", "+-[2] Bodies-----------------------------------------------------------------------------------------------------------------------+"},
	{"accessible", 132, false, "[3] Aspects", "+-[3] Aspects----------------------------------------------------------------------------------------------------------------------+"},
	{"accessible", 132, false, "[0] Details", "+-[0] Details----------------------------------------------------------------------------------------------------------------------+"},
	{"accessible", 132, false, "Log", "+-Log------------------------------------------------------------------------------------------------------------------------------+"},
	// Dynamic Details titles (never focused), incl. the truncation path at
	// widths below the layout minimum.
	{"color", 66, false, "[0] Details: Mercury", "\x1b[38;5;240m╭─\x1b[0m\x1b[38;5;252m[0] Details: Mercury\x1b[0m\x1b[38;5;240m───────────────────────────────────────────╮\x1b[0m"},
	{"color", 132, false, "[0] Details: Mercury", "\x1b[38;5;240m╭─\x1b[0m\x1b[38;5;252m[0] Details: Mercury\x1b[0m\x1b[38;5;240m─────────────────────────────────────────────────────────────────────────────────────────────────────────────╮\x1b[0m"},
	{"color", 16, false, "[0] Details: Mercury", "\x1b[38;5;240m╭─\x1b[0m\x1b[38;5;252m[0] Details…\x1b[0m\x1b[38;5;240m─╮\x1b[0m"},
	{"color", 12, false, "[0] Details: Mercury", "\x1b[38;5;240m╭─\x1b[0m\x1b[38;5;252m[0] Det…\x1b[0m\x1b[38;5;240m─╮\x1b[0m"},
	{"accessible", 66, false, "[0] Details: Mercury", "+-[0] Details: Mercury-------------------------------------------+"},
	{"accessible", 132, false, "[0] Details: Mercury", "+-[0] Details: Mercury-------------------------------------------------------------------------------------------------------------+"},
	{"accessible", 16, false, "[0] Details: Mercury", "+-[0] Detai...-+"},
	{"accessible", 12, false, "[0] Details: Mercury", "+-[0] D...-+"},
}

// C1
func TestOneTabTitleBorderIdentity(t *testing.T) {
	forceColor(t)
	for _, tc := range legacyTopBorders {
		cfg := Config{}
		if tc.mode == "accessible" {
			cfg = Config{Accessible: true}
		}
		box := renderTitledBox(tc.title, tc.width, 3, tc.focused, cfg, "")
		got := strings.SplitN(box, "\n", 2)[0]
		if got != tc.want {
			t.Errorf("%s w=%d focused=%v %q:\n got %q\nwant %q", tc.mode, tc.width, tc.focused, tc.title, got, tc.want)
		}
		// The same title through the strip path with a one-tab list.
		var m Model
		m.cfg = cfg
		prefix, tab := "", tc.title
		if i := strings.Index(tc.title, "] "); i >= 0 {
			prefix, tab = tc.title[:i+2], tc.title[i+2:]
		}
		box = renderTabbedBox(tabStrip{prefix: prefix, tabs: []Tab{{Title: tab}}}, tc.width, 3, tc.focused, cfg, "")
		if got := strings.SplitN(box, "\n", 2)[0]; got != tc.want {
			t.Errorf("one-tab strip %s w=%d focused=%v %q:\n got %q\nwant %q", tc.mode, tc.width, tc.focused, tc.title, got, tc.want)
		}
		// Details titles arrive through renderDetailsPanel's dynamic title.
		// Details is no longer a one-tab panel (the specification
		// C11): its report title is the report tab of a two-tab strip, so
		// here it is checked for its prefix and its title, not for identity.
		if strings.HasPrefix(tc.title, "[0] Details:") && tc.width >= 40 {
			box = renderDetailsPanel(tc.width, 3, (detailsFrameOf(detailsViewState{title: tc.title})).state.attached, cfg, detailsFrameOf(detailsViewState{title: tc.title}))
			top := ansi.Strip(strings.SplitN(box, "\n", 2)[0])
			report := strings.TrimPrefix(tc.title, "[0] ")
			if !strings.Contains(top, "[0] "+report) && !strings.Contains(top, "[0] ["+report+"]") {
				t.Errorf("details panel %s w=%d: border %q does not carry %q", tc.mode, tc.width, top, tc.title)
			}
		}
	}
}

// tabsPairModel is a ready two-slot workspace with the comparison pair
// selected and Aspects focused; nothing committed yet.
func tabsPairModel(t *testing.T, cfg Config) Model {
	t.Helper()
	m := NewModelWithClock(cfg, FrozenClock(testFrozenTime()))
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	anchorWhen := time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC)
	if err := addAnchor(slots, anchorWhen, testPlaceAmsterdam()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	m.slots = slots
	m.cursorIdx = 1
	m.place = testPlaceBerlin()
	m.targetFloatingTime = slots.Floating().Time
	m.startup = startupReady
	m.focused = PanelAspects
	// The cursor sits on ② (the floating slot); its tab is pre-selected on
	// arrival (selected-charts-compare C1a).
	return m.noteArrival()
}

// tabsOnPair is tabsPairModel after a real `]` on the focused Aspects panel:
// the cursor's arrival at now lands on `Aspects` (the specification
// C1), so a test about the pair tab reaches it the way the operator does. The
// recompute the press starts is not pumped: these tests commit their own frames.
func tabsOnPair(t *testing.T, cfg Config) Model {
	t.Helper()
	m := tabsPairModel(t, cfg)
	if got := m.activeTabID(PanelAspects); got != TabAspects {
		t.Fatalf("arrival: active tab %q, want %q", got, TabAspects)
	}
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if got := m.activeTabID(PanelAspects); got != TabAspectsCross {
		t.Fatalf("after ]: active tab %q, want %q", got, TabAspectsCross)
	}
	return m
}

func tabsOwnAspects() []aspect.Aspect {
	mk := func(a, b aspect.Body, k aspect.AspectKind, orb, power float64) aspect.Aspect {
		asp := aspect.NewAspect(a, b, k, aspect.Orb(orb))
		asp.Power = power
		return asp
	}
	return []aspect.Aspect{
		mk(aspect.Sun, aspect.Moon, aspect.Square, 0.5, 22),
		mk(aspect.Venus, aspect.Jupiter, aspect.Sextile, 3.0, 8.4),
	}
}

func tabsCrossAspects() []lookup.OverlayAspect {
	return []lookup.OverlayAspect{
		{Transit: aspect.Saturn, Natal: aspect.Sun, Kind: aspect.Square, Orb: 0.5, Power: 9, Applying: true},
		{Transit: aspect.Jupiter, Natal: aspect.Moon, Kind: aspect.Trine, Orb: 1.4, Power: 6},
	}
}

func tabsOverlayFrame() FirstFrameReadyMsg {
	return FirstFrameReadyMsg{
		Overlay:      true,
		Positions:    []aspect.Position{{Body: aspect.Sun, Longitude: 35.5}, {Body: aspect.Moon, Longitude: 136.18}},
		Aspects:      tabsOwnAspects(),
		CrossAspects: tabsCrossAspects(),
	}
}

func pressKey(t *testing.T, m Model, r rune) (Model, tea.Cmd) {
	t.Helper()
	var tm tea.Model = m
	tm, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	return tm.(Model), cmd
}

func tabIDs(tabs []Tab) []TabID {
	ids := make([]TabID, len(tabs))
	for i, tab := range tabs {
		ids[i] = tab.ID
	}
	return ids
}

// C2, amended by selected-charts-compare C1 (operator 2026-10-04):
// the Aspects panel opens with `Aspects`, and while a pair is selected it
// carries the pair's tab after it.
func TestAspectsTabFollowsPairSelection(t *testing.T) {
	m := tabsPairModel(t, Config{Accessible: true})
	if got := tabIDs(m.panelTabs(PanelAspects)); !reflect.DeepEqual(got, []TabID{TabAspects, TabAspectsCross}) {
		t.Fatalf("pair selected: tabs = %v, want [aspects aspects-cross]", got)
	}
	m.slots.SetComparisonSelected(false)
	if got := tabIDs(m.panelTabs(PanelAspects)); !reflect.DeepEqual(got, []TabID{TabAspects}) {
		t.Fatalf("pair deselected: tabs = %v, want [aspects]", got)
	}
	m.slots.SetComparisonSelected(true)
	if got := tabIDs(m.panelTabs(PanelAspects)); !reflect.DeepEqual(got, []TabID{TabAspects, TabAspectsCross}) {
		t.Fatalf("pair reselected: tabs = %v, want [aspects aspects-cross]", got)
	}
	for _, p := range []PanelID{PanelChart, PanelBodies, PanelLog} {
		if n := len(m.panelTabs(p)); n != 1 {
			t.Errorf("%s declares %d tabs, want 1", p, n)
		}
	}
	// Details is the one exception, restored by the specification
	// C11: report and timeline, whatever the pair does.
	// the specification acceptance 1: the timeline
	// only while the Chart panel is focused.
	m.focused = PanelChart
	if got := tabIDs(m.panelTabs(PanelDetails)); !reflect.DeepEqual(got, []TabID{TabDetailsReport, TabDetailsTimeline}) {
		t.Errorf("details tabs = %v, want [report timeline]", got)
	}
	m.focused = PanelAspects
	if got := tabIDs(m.panelTabs(PanelDetails)); !reflect.DeepEqual(got, []TabID{TabDetailsReport}) {
		t.Errorf("details tabs on Aspects = %v, want [report]", got)
	}
}

// C3: the effective tab id is derived, never remembered, so it can
// never be stale — deselecting the pair changes it on the very next read.
func TestActiveTabIDIsDerived(t *testing.T) {
	m := tabsPairModel(t, Config{Accessible: true})
	// Arrival at now lands on `Aspects` (the specification C1);
	// the pair tab is one real `]` away.
	if got := m.activeTabID(PanelAspects); got != TabAspects {
		t.Errorf("arrival: id = %q, want %q", got, TabAspects)
	}
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if got := m.activeTabID(PanelAspects); got != TabAspectsCross {
		t.Errorf("pair selected: id = %q, want %q", got, TabAspectsCross)
	}
	m.slots.SetComparisonSelected(false)
	if got := m.activeTabID(PanelAspects); got != TabAspects {
		t.Errorf("pair deselected: id = %q, want %q", got, TabAspects)
	}
	for _, p := range []PanelID{PanelChart, PanelBodies, PanelDetails, PanelLog} {
		if got := m.activeTabID(p); got != m.panelTabs(p)[0].ID {
			t.Errorf("%s: id = %q, want its only tab", p, got)
		}
	}
}

// C4: committing an overlay frame needs no tab bookkeeping — the
// panel shows the cross-aspects because a pair is selected, and it keeps
// showing them across further frames of either kind.
func TestOverlayCommitShowsTransitAspects(t *testing.T) {
	m := tabsOnPair(t, Config{Accessible: true})
	m, _ = applyTextCommit(m, tabsOverlayFrame())
	if got := m.activeTabID(PanelAspects); got != TabAspectsCross {
		t.Errorf("after overlay commit: active = %q, want %q", got, TabAspectsCross)
	}
	// A single-chart frame does not change what the panel offers: selection
	// does, and the pair is still selected.
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{Aspects: tabsOwnAspects()})
	if got := m.activeTabID(PanelAspects); got != TabAspectsCross {
		t.Errorf("after single-chart frame: active = %q, want %q", got, TabAspectsCross)
	}
	// Changed by selected-charts-compare C1: the tab is titled by its pair, in
	// the numeral form of the colour mode (Accessible: colours off, so `2 in 1`),
	// no longer "Transit aspects".
	// Amended by C1: `Aspects` opens the list, the pair tab follows it.
	if got := m.panelTabs(PanelAspects)[1].Title; got != "2 in 1" {
		t.Errorf("title = %q, want %q", got, "2 in 1")
	}
}

// C9: a Jump digit only ever focuses. Re-pressing the focused
// panel's own digit does nothing at all.
func TestJumpDigitFocusesOnly(t *testing.T) {
	m := tabsOnPair(t, Config{Accessible: true})
	m.focused = PanelChart
	m, _ = pressKey(t, m, '3')
	if m.focused != PanelAspects {
		t.Fatalf("focus = %q, want aspects", m.focused)
	}
	if got := m.activeTabID(PanelAspects); got != TabAspectsCross {
		t.Errorf("focusing Aspects changed its content to %q", got)
	}
	// Amended by selected-charts-compare C1: Aspects now has two tabs while
	// comparing, so re-pressing its digit steps them (panelDigits) and starts
	// the pair's recompute; a one-tab panel's digit is still inert.
	m, cmd := pressKey(t, m, '3')
	if cmd == nil || m.activeTabID(PanelAspects) != TabAspects {
		t.Errorf("re-pressing 3 on the pair tab: cmd %v, active %q, want a recompute and Aspects", cmd != nil, m.activeTabID(PanelAspects))
	}
	one := tabsPairModel(t, Config{Accessible: true})
	one.slots.SetComparisonSelected(false)
	before := one
	m, cmd = pressKey(t, one, '3')
	if cmd != nil || !reflect.DeepEqual(m, before) {
		t.Error("re-pressing the digit of a one-tab panel changed something")
	}
}

// C10, narrowed by the specification: `[` and `]` act on
// the focused ring panel, which declares one tab, so they stay inert — no
// model change, no Cmd — whether or not a comparison pair is selected. `0`
// left this list: it is Details' digit again and cycles Details' tabs (C33).
func TestRetiredTabKeysAreInert(t *testing.T) {
	for _, paired := range []bool{true, false} {
		for _, focus := range []PanelID{PanelChart, PanelBodies, PanelAspects} {
			// Amended by selected-charts-compare C1: a pair makes Aspects a
			// two-tab panel, where `[` / `]` cycle (TestBracketsCycleAspectPairs).
			if paired && focus == PanelAspects {
				continue
			}
			// Amended by the view rule (a), 2026-10-24: from the Chart panel the
			// brackets step the Aspects tabs
			// (TestBracketKeysFromChartChangeTheAspectsTab).
			if paired && focus == PanelChart {
				continue
			}
			base := tabsPairModel(t, Config{Accessible: true})
			base.focused = focus
			base.slots.SetComparisonSelected(paired)
			for _, r := range []rune{'[', ']'} {
				m, cmd := pressKey(t, base, r)
				if cmd != nil {
					t.Errorf("paired=%v %s: %q returned a Cmd", paired, focus, r)
				}
				if !reflect.DeepEqual(m, base) {
					t.Errorf("paired=%v %s: %q changed the model", paired, focus, r)
				}
			}
		}
	}
}

// C11: the retired keys start no Astrolog work either.
func TestRetiredTabKeysStartNoCompute(t *testing.T) {
	m := tabsPairModel(t, Config{Accessible: true})
	m.width, m.height = 132, 40
	textWant := m.refresh[surfaceText].wantID
	imageWant := m.refresh[surfaceImage].wantID
	// Amended by selected-charts-compare C1: `3` and the brackets step
	// Aspects' tabs while a pair is selected and so do recompute; the inert
	// keys are the ones with no second tab to reach (focus on Bodies; from
	// Chart the brackets reach Aspects since the view rule's amendment (a)).
	m.focused = PanelBodies
	for _, r := range []rune{']', '[', '0', '2'} {
		m, _ = pressKey(t, m, r)
		if got := m.refresh[surfaceText].wantID; got != textWant {
			t.Errorf("%q bumped text wantID to %d", r, got)
		}
		if got := m.refresh[surfaceImage].wantID; got != imageWant {
			t.Errorf("%q bumped image wantID to %d", r, got)
		}
		if m.refresh[surfaceText].cancel != nil || m.refresh[surfaceImage].cancel != nil {
			t.Errorf("%q started a text/image run", r)
		}
	}
}

// C12a retired `[`, `]` and `0` for want of a second tab. The
// timeline spec restores them (D14, D15); TestNewGlobalBindingsAreRegisteredByName
// now pins which fields claim them.

// C13
func TestAspectsTabRendersSingleWaterfall(t *testing.T) {
	state := chartViewState{aspects: tabsOwnAspects(), crossAspects: tabsCrossAspects(), overlay: true}
	got := renderAspectsBody(state, TabAspects, 64, 4, Config{Accessible: true})
	want := renderAspectWaterfallStyled(state.aspects, nil, 64, 4, Config{Accessible: true})
	if got != want {
		t.Errorf("aspects tab body:\n got %q\nwant %q", got, want)
	}
	if strings.Contains(got, bodyGlyph(aspect.Saturn)) {
		t.Errorf("aspects tab leaked a cross-aspect body: %q", got)
	}
}

// C14
func TestCrossTabRendersFullHeightNoHeader(t *testing.T) {
	state := chartViewState{aspects: tabsOwnAspects(), crossAspects: tabsCrossAspects(), overlay: true}
	got := renderAspectsBody(state, TabAspectsCross, 64, 4, Config{Accessible: true})
	want := renderCrossAspectWaterfallStyled(state.crossAspects, nil, 64, 4, Config{Accessible: true})
	if got != want {
		t.Errorf("cross tab body:\n got %q\nwant %q", got, want)
	}
	if n := len(strings.Split(got, "\n")); n != 4 {
		t.Errorf("cross tab used %d rows, want the full inner height 4", n)
	}
	if strings.Contains(got, "Transit") {
		t.Errorf("cross tab still prints an in-body header: %q", got)
	}
}

// C15
func TestCrossTabEmptyLine(t *testing.T) {
	state := chartViewState{aspects: tabsOwnAspects(), overlay: true, settled: true}
	if got := renderAspectsBody(state, TabAspectsCross, 64, 4, Config{Accessible: true}); got != "(no cross-aspects in orb)" {
		t.Errorf("empty cross tab = %q", got)
	}
}

// C16
func TestOverlayFrameCarriesFocusedAspects(t *testing.T) {
	natalRows := []aspect.Aspect{aspect.NewAspect(aspect.Sun, aspect.Mars, aspect.Trine, 1)}
	transitRows := []aspect.Aspect{aspect.NewAspect(aspect.Moon, aspect.Venus, aspect.Square, 2)}
	natalStyles := astrologstyle.Aspects{{First: aspect.Sun, Second: aspect.Mars, Kind: aspect.Trine}: {Kind: "natal"}}
	transitStyles := astrologstyle.Aspects{{First: aspect.Moon, Second: aspect.Venus, Kind: aspect.Square}: {Kind: "transit"}}
	oldNatal, oldTransit, oldCross := firstCallNatalPositions, firstCallTransits, overlayCrossAspects
	t.Cleanup(func() {
		firstCallNatalPositions, firstCallTransits, overlayCrossAspects = oldNatal, oldTransit, oldCross
	})
	firstCallNatalPositions = func(context.Context, *lookup.Probers, store.AnchorChart, lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: []aspect.Position{{Body: aspect.Sun, Longitude: 250}}, Aspects: natalRows, AspectStyles: natalStyles}, nil
	}
	firstCallTransits = func(context.Context, *lookup.Probers, time.Time, location.PlaceInfo, lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: []aspect.Position{{Body: aspect.Sun, Longitude: 35}}, Aspects: transitRows, AspectStyles: transitStyles}, nil
	}
	overlayCrossAspects = func(context.Context, lookup.Lens, store.AnchorChart, time.Time, location.PlaceInfo) (lookup.OverlayAspects, error) {
		return lookup.OverlayAspects{Aspects: tabsCrossAspects()}, nil
	}
	chart := store.AnchorChart{When: testFrozenTime().AddDate(-30, 0, 0), Place: testPlaceAmsterdam()}
	for _, tc := range []struct {
		focusedNatal bool
		rows         []aspect.Aspect
		styles       astrologstyle.Aspects
	}{
		{false, transitRows, transitStyles},
		{true, natalRows, natalStyles},
	} {
		msg := computeOverlayCmd(context.Background(), 1, 1, nil, chart, store.AnchorChart{}, testFrozenTime(), testPlaceBerlin(), tc.focusedNatal, lookup.Hellenistic)().(FirstFrameReadyMsg)
		if !msg.Overlay || msg.Err != nil {
			t.Fatalf("focusedNatal=%v: overlay=%v err=%v", tc.focusedNatal, msg.Overlay, msg.Err)
		}
		if !reflect.DeepEqual(msg.Aspects, tc.rows) || !reflect.DeepEqual(msg.AspectStyles, tc.styles) {
			t.Errorf("focusedNatal=%v: frame aspects = %v / %v, want the focused slot's own", tc.focusedNatal, msg.Aspects, msg.AspectStyles)
		}
		if len(msg.CrossAspects) != 2 {
			t.Errorf("focusedNatal=%v: cross-aspects = %v", tc.focusedNatal, msg.CrossAspects)
		}
	}
}

// C17: while Aspects is focused, Details is the
// committed frame's own report for whatever the panel is showing — the -rt
// -aa list while a pair is selected, the focused slot's -aa list otherwise —
// and its title names the same thing the panel's border does.
func TestDetailsFollowsComparisonState(t *testing.T) {
	m := tabsPairModel(t, Config{Accessible: true})
	m.width, m.height = 132, 40
	m.chart.aspectsReport = "own -aa report"
	m.chart.crossAspectsReport = "-rt -aa report"
	// On arrival at now the panel shows `Aspects`, so Details is the own report
	// (the specification C1); the pair's report follows a `]`.
	if title, body, ok := m.aspectsDetailsFromFrame(); !ok || title != "[0] Aspects" || body != "own -aa report" {
		t.Errorf("arrival: %q / %q / %v, want the own -aa report", title, body, ok)
	}
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m.chart.crossAspectsReport = "-rt -aa report"
	if _, ok := m.detailsRequestFor(); ok {
		t.Error("Aspects focus must not produce a spawnable Details request")
	}
	title, body, ok := m.aspectsDetailsFromFrame()
	if !ok || title != "[0] 2 in 1" || body != "-rt -aa report" {
		t.Errorf("pair selected: %q / %q / %v, want the -rt -aa report", title, body, ok)
	}
	m.slots.SetComparisonSelected(false)
	title, body, ok = m.aspectsDetailsFromFrame()
	if !ok || title != "[0] Aspects" || body != "own -aa report" {
		t.Errorf("pair deselected: %q / %q / %v, want the own -aa report", title, body, ok)
	}
}

var overflowTabs = []Tab{
	{ID: "t0", Title: "aa"}, {ID: "t1", Title: "bb"}, {ID: "t2", Title: "cc"}, {ID: "t3", Title: "dd"}, {ID: "t4", Title: "ee"},
}

func stripText(runs []stripRun) string {
	var sb strings.Builder
	for _, r := range runs {
		sb.WriteString(r.text)
	}
	return sb.String()
}

// C19
func TestTabStripOverflowKeepsActive(t *testing.T) {
	full := stripText(layoutTabStrip("[9] ", overflowTabs, 2, true, 80, Config{Color: ColorNever}))
	if full != "[9] aa - bb - [cc] - dd - ee" {
		t.Fatalf("unconstrained strip = %q", full)
	}
	cases := []struct {
		maxCells int
		want     string
	}{
		{27, "[9] aa - bb - [cc] - dd +1"}, // after-active from the end first
		{25, "[9] aa - bb - [cc] +2"},
		{20, "[9] aa - [cc] +3"}, // then before-active, nearest first
		{15, "[9] [cc] +4"},
	}
	for _, tc := range cases {
		if got := stripText(layoutTabStrip("[9] ", overflowTabs, 2, true, tc.maxCells, Config{Color: ColorNever})); got != tc.want {
			t.Errorf("max %d: %q, want %q", tc.maxCells, got, tc.want)
		}
	}
	// Through the border at width maxCells+4.
	top := renderTopBorder(24, tabStrip{prefix: "[9] ", tabs: overflowTabs, active: 2}, "240", "252", false, Config{Accessible: true}, "+", "+", "-")
	if top != "+-[9] aa - [cc] +3-----+" {
		t.Errorf("border at 24 = %q", top)
	}
	// Active at the end: nothing after it, so predecessors go nearest-first.
	if got := stripText(layoutTabStrip("[9] ", overflowTabs, 4, true, 20, Config{Color: ColorNever})); got != "[9] aa - [ee] +3" {
		t.Errorf("active last, max 20: %q", got)
	}
}

// C20
func TestTabStripTruncatesActiveOnly(t *testing.T) {
	tabs := []Tab{{ID: "a", Title: "aa"}, {ID: "b", Title: "ccccc"}, {ID: "c", Title: "dd"}}
	got := stripText(layoutTabStrip("[9] ", tabs, 1, true, 9, Config{Color: ColorNever}))
	if got != "[9] [cc…]" {
		t.Errorf("collapsed strip = %q, want %q (a cut title is marked, text.go)", got, "[9] [cc…]")
	}
	if strings.Contains(got, "+") {
		t.Errorf("the +N marker survived where only the active tab fits: %q", got)
	}
	top := renderTopBorder(13, tabStrip{prefix: "[9] ", tabs: tabs, active: 1}, "240", "252", false, Config{Accessible: true}, "+", "+", "-")
	if top != "+-[9] [...]-+" {
		t.Errorf("border at 13 = %q", top)
	}
}

// C21: a production panel is a one-tab strip, so it renders its
// title plain — no bracket marker anywhere in the frame. The marker remains
// the multi-tab renderer's contract and is exercised directly below.
func TestOneTabStripCarriesNoMarker(t *testing.T) {
	for name, cfg := range map[string]Config{
		"accessible": {Accessible: true},
		"never":      {Color: ColorNever},
	} {
		m := tabsOnPair(t, cfg)
		m.width, m.height = 80, 24
		view := m.View()
		// Changed by selected-charts-compare C1: `Aspects` then the pair tab
		// `2 in 1` (colours off); the active tab carries the marker.
		if !strings.Contains(view, "[3] Aspects - [2 in 1]") {
			t.Errorf("%s: Aspects title is not the comparison title: %q", name, aspectsTopBorder(t, m))
		}
		for _, bad := range []string{"[Aspects]", "[Chart]", "[Bodies]", "Transit→Natal"} {
			if strings.Contains(view, bad) {
				t.Errorf("%s: frame still carries %q", name, bad)
			}
		}
		m.slots.SetComparisonSelected(false)
		if view := m.View(); !strings.Contains(view, "[3] Aspects") || strings.Contains(view, "2 in 1") {
			t.Errorf("%s: deselected title = %q", name, aspectsTopBorder(t, m))
		}
	}
}

func aspectsTopBorder(t *testing.T, m Model) string {
	t.Helper()
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "[3] ") {
			return line
		}
	}
	t.Fatal("no Aspects title row in View()")
	return ""
}

// C22: the comparison title is styled exactly like any one-tab
// title — bold green while focused, plain green while not.
func TestComparisonTitleStyledLikeAnyTitle(t *testing.T) {
	forceColor(t)
	m := tabsOnPair(t, Config{})
	m.width, m.height = 80, 24
	line := aspectsTopBorder(t, m)
	// Amended by selected-charts-compare C1: the strip is `Aspects - ② in ①`
	// with the pair tab active, so the active tab is the green run and the
	// other tab the plain title gray.
	if !strings.Contains(line, "\x1b[1;92m② in ①\x1b[0m") || !strings.Contains(line, "\x1b[38;5;252mAspects\x1b[0m") {
		t.Errorf("focused active tab not bold green beside a plain Aspects tab: %q", line)
	}
	m.focused = PanelChart
	line = aspectsTopBorder(t, m)
	if !strings.Contains(line, "\x1b[92m② in ①\x1b[0m") || !strings.Contains(line, "\x1b[38;5;252mAspects\x1b[0m") {
		t.Errorf("unfocused active tab not plain green beside a plain Aspects tab: %q", line)
	}
}

// C23: the multi-tab strip renderer keeps its styling contract for
// the day a panel declares a second tab. No production panel reaches it.
func TestMultiTabStripStyling(t *testing.T) {
	forceColor(t)
	tabs := []Tab{{ID: "a", Title: "Aspects"}, {ID: "b", Title: "Houses"}}
	focused := renderTopBorder(66, tabStrip{prefix: "[3] ", tabs: tabs, active: 0}, "92", "92", true, Config{}, "+", "+", "-")
	if !strings.Contains(focused, "\x1b[1;92mAspects\x1b[0m") {
		t.Errorf("focused active tab not bold green: %q", focused)
	}
	if !strings.Contains(focused, "\x1b[38;5;252mHouses\x1b[0m") {
		t.Errorf("inactive tab not plain title gray: %q", focused)
	}
	unfocused := renderTopBorder(66, tabStrip{prefix: "[3] ", tabs: tabs, active: 0}, "240", "252", false, Config{}, "+", "+", "-")
	if !strings.Contains(unfocused, "\x1b[92mAspects\x1b[0m") || strings.Contains(unfocused, "\x1b[1;92mAspects") {
		t.Errorf("unfocused active tab must keep green without bold: %q", unfocused)
	}
}

// C24: there is no tab state left to misplace. The Model carries no
// remembered tab, and neither the frame value nor the persistent store ever
// did.
func TestNoStoredTabState(t *testing.T) {
	fset := token.NewFileSet()
	for _, dir := range []string{".", "../store"} {
		pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", dir, err)
		}
		for _, pkg := range pkgs {
			for path, file := range pkg.Files {
				ast.Inspect(file, func(n ast.Node) bool {
					switch v := n.(type) {
					case *ast.TypeSpec:
						st, ok := v.Type.(*ast.StructType)
						if !ok {
							return true
						}
						// In ui only Model and the frame value are in scope;
						// tabStrip is a per-render value, not state.
						if dir == "." && v.Name.Name != "Model" && v.Name.Name != "chartViewState" {
							return true
						}
						for _, f := range st.Fields.List {
							typeStr := typeString(f.Type)
							for _, name := range f.Names {
								if mentionsTab(name.Name) || strings.Contains(typeStr, "TabID") {
									t.Errorf("%s: %s.%s (%s) stores tab state", filepath.Base(path), v.Name.Name, name.Name, typeStr)
								}
							}
						}
					case *ast.FuncDecl:
						if v.Recv == nil {
							return true
						}
						recv, _ := receiverName(v.Recv.List[0].Type)
						if dir == "." && recv != "chartViewState" {
							return true
						}
						if mentionsTab(v.Name.Name) {
							t.Errorf("%s: method %s.%s carries tab state", filepath.Base(path), recv, v.Name.Name)
						}
					}
					return true
				})
			}
		}
	}
}

func mentionsTab(ident string) bool {
	lower := strings.ToLower(ident)
	return strings.Contains(strings.ReplaceAll(lower, "table", ""), "tab")
}

func typeString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.MapType:
		return "map[" + typeString(v.Key) + "]" + typeString(v.Value)
	case *ast.ArrayType:
		return "[]" + typeString(v.Elt)
	case *ast.StarExpr:
		return "*" + typeString(v.X)
	case *ast.SelectorExpr:
		return typeString(v.X) + "." + v.Sel.Name
	case *ast.FuncType:
		return "func"
	default:
		return "?"
	}
}

func receiverName(e ast.Expr) (string, bool) {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name, true
	case *ast.StarExpr:
		if id, ok := v.X.(*ast.Ident); ok {
			return id.Name, true
		}
	}
	return "", false
}

// C26
func TestAspectsRenderersAreBagOnly(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "aspects_tabs.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	funcs := map[string]*ast.FuncDecl{}
	var resolveParams *ast.FieldList
	for _, d := range file.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil {
			funcs[fd.Name.Name] = fd
			if fd.Name.Name == "resolveAspectsBag" {
				resolveParams = fd.Type.Params
			}
		}
	}
	if resolveParams == nil {
		t.Fatal("resolveAspectsBag not declared in aspects_tabs.go")
	}
	wantSig := func(fd *ast.FuncDecl) bool {
		var params []string
		for _, p := range fd.Type.Params.List {
			ts := typeString(p.Type)
			for range p.Names {
				params = append(params, ts)
			}
			if len(p.Names) == 0 {
				params = append(params, ts)
			}
		}
		return reflect.DeepEqual(params, []string{"aspectsBag", "int", "int", "Config"}) &&
			fd.Type.Results != nil && len(fd.Type.Results.List) == 1 && typeString(fd.Type.Results.List[0].Type) == "string"
	}
	tableSeen := false
	ast.Inspect(file, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || vs.Names[0].Name != "aspectsRenderers" {
			return true
		}
		tableSeen = true
		lit, ok := vs.Values[0].(*ast.CompositeLit)
		if !ok {
			t.Fatal("aspectsRenderers is not a composite literal")
		}
		if typeString(lit.Type.(*ast.MapType).Key) != "TabID" {
			t.Errorf("aspectsRenderers keyed by %s, want TabID", typeString(lit.Type.(*ast.MapType).Key))
		}
		if len(lit.Elts) == 0 {
			t.Error("aspectsRenderers is empty")
		}
		for _, elt := range lit.Elts {
			kv := elt.(*ast.KeyValueExpr)
			id, ok := kv.Value.(*ast.Ident)
			if !ok {
				t.Errorf("renderer for %s is not a plain identifier", typeString(kv.Key))
				continue
			}
			fd, ok := funcs[id.Name]
			if !ok {
				t.Errorf("renderer %s is not declared in aspects_tabs.go", id.Name)
				continue
			}
			if !wantSig(fd) {
				t.Errorf("renderer %s does not have signature func(aspectsBag, int, int, Config) string", id.Name)
			}
		}
		return false
	})
	if !tableSeen {
		t.Fatal("aspectsRenderers table not found")
	}
	ast.Inspect(file, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || (id.Name != "Model" && id.Name != "chartViewState") {
			return true
		}
		if id.Pos() >= resolveParams.Pos() && id.End() <= resolveParams.End() {
			return true
		}
		t.Errorf("aspects_tabs.go references %s at %s outside resolveAspectsBag's parameters", id.Name, fset.Position(id.Pos()))
		return true
	})
}

// C30
func TestTabKeysNoopBeforeReady(t *testing.T) {
	noSlots := tabsPairModel(t, Config{Accessible: true})
	noSlots.slots = nil
	computing := tabsPairModel(t, Config{Accessible: true})
	computing.startup = startupComputing
	for name, base := range map[string]Model{"nil slots": noSlots, "computing": computing} {
		for _, r := range []rune{'[', ']', '0', '3'} {
			m, cmd := pressKey(t, base, r)
			if cmd != nil {
				t.Errorf("%s: %q returned a Cmd", name, r)
			}
			if !reflect.DeepEqual(m, base) {
				t.Errorf("%s: %q changed the model", name, r)
			}
		}
	}
}

// C31
func TestTabKeysIgnoredInModal(t *testing.T) {
	help := tabsPairModel(t, Config{Accessible: true})
	help.helpOpen = true
	atlas := tabsPairModel(t, Config{Accessible: true})
	atlas.startup = startupAtlasOpen
	charts := tabsPairModel(t, Config{Accessible: true})
	charts.startup = startupChartOpen
	for name, base := range map[string]Model{"help": help, "atlas": atlas, "charts": charts} {
		want := base.activeTabID(PanelAspects)
		for _, r := range []rune{'[', ']', '0', '3'} {
			m, _ := pressKey(t, base, r)
			if got := m.activeTabID(PanelAspects); got != want {
				t.Errorf("%s modal: %q changed the Aspects content to %q", name, r, got)
			}
		}
	}
}

// C32
func TestPairDeselectFallsBackWithoutBlank(t *testing.T) {
	m := tabsOnPair(t, Config{Accessible: true})
	m.width, m.height = 80, 24
	m, _ = applyTextCommit(m, tabsOverlayFrame())
	if m.activeTabID(PanelAspects) != TabAspectsCross {
		t.Fatal("precondition: cross tab active")
	}
	m.slots.SetComparisonSelected(false)
	if got := m.activeTabID(PanelAspects); got != TabAspects {
		t.Errorf("effective tab after deselect = %q, want aspects", got)
	}
	view := m.View()
	if !strings.Contains(view, "[3] Aspects----") {
		t.Errorf("Aspects did not fall back to its one-tab title: %q", aspectsTopBorder(t, m))
	}
	body := renderAspectsBody(m.chart, m.activeTabID(PanelAspects), 78, 4, m.cfg)
	if strings.TrimSpace(body) == "" {
		t.Error("Aspects body went blank after the pair was deselected")
	}
	if !strings.Contains(view, bodyGlyph(aspect.Venus)+" "+aspectGlyph(aspect.Sextile)) {
		t.Errorf("last frame's own aspects not rendered after deselect")
	}
}

// C33
func TestCrossTabOnErrorFrame(t *testing.T) {
	m := tabsOnPair(t, Config{Accessible: true})
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{Overlay: true, Err: errors.New("boom")})
	if got := m.activeTabID(PanelAspects); got != TabAspectsCross {
		t.Fatalf("active = %q", got)
	}
	// A failed frame is not a settled one: the tab stays blank, and the
	// failure is the Log's (the specification A6).
	if got := renderAspectsBody(m.chart, TabAspectsCross, 64, 4, m.cfg); got != "" {
		t.Errorf("error frame body = %q, want blank", got)
	}
	logged := false
	for _, line := range m.logLines {
		if strings.HasPrefix(line.text, "Couldn't cast the chart.") {
			logged = true
		}
	}
	if !logged {
		t.Errorf("compute error not logged: %v", m.logLines)
	}
}

// C34
func TestTooSmallUnchangedWithTabs(t *testing.T) {
	const wantAccessible = "    +------------------------------+    \n    |                              |    \n    |  Terminal too small          |    \n    |                              |    \n    |  current:   40x10            |    \n    |  required:  66x16 (minimum)  |    \n    |                              |    \n    |  resize and we'll pick up.   |    \n    |                              |    \n    +------------------------------+    "
	m := tabsPairModel(t, Config{Accessible: true})
	m.width, m.height = 40, 10
	m, _ = applyTextCommit(m, tabsOverlayFrame())
	got := m.View()
	if got != tooSmallView(40, 10, chartContentLines(m.slots), m.cfg) {
		t.Error("too-small View differs from tooSmallView")
	}
	if got != wantAccessible {
		t.Errorf("too-small render changed:\n%s", got)
	}
	if strings.Contains(got, "Transit") {
		t.Error("too-small render leaked a tab strip")
	}
}

// an earlier change (as amended by an earlier change): without color (accessible, or --color=never)
// focus still shows on the Aspects title as bold — the same bold every
// one-tab title keeps without color — and unfocused carries no styling at all.
// Rendered under a forced TrueColor profile so lipgloss emits the SGR.
func TestOneTabTitleAccessibleFocusBold(t *testing.T) {
	forceColor(t)
	for name, cfg := range map[string]Config{
		"accessible": {Accessible: true},
		"never":      {Color: ColorNever},
	} {
		// The focused title opens on the colours-off focus mark, inside the
		// same bold run (the specification C2).
		mark := focusMarkUnicode
		if cfg.Accessible {
			mark = focusMarkASCII
		}
		m := tabsOnPair(t, cfg)
		m.width, m.height = 80, 24
		// Amended by selected-charts-compare C1: the strip is `Aspects - [2 in 1]`;
		// the focus mark and the active tab are the bold runs.
		if line := aspectsTopBorder(t, m); !strings.Contains(line, "\x1b[1m"+mark+"\x1b[0m") || !strings.Contains(line, "\x1b[1m[2 in 1]\x1b[0m") {
			t.Errorf("%s focused: title not bold: %q", name, line)
		}
		m.focused = PanelChart
		var chart string
		for _, l := range strings.Split(m.View(), "\n") {
			if strings.Contains(l, "[1] ") {
				chart = l
				break
			}
		}
		if !strings.Contains(chart, "\x1b[1m"+mark+"[1] Chart\x1b[0m") {
			t.Errorf("%s: one-tab focused title not bold: %q", name, chart)
		}
		if line := aspectsTopBorder(t, m); strings.Contains(line, "\x1b[") {
			t.Errorf("%s unfocused: title carries styling without color: %q", name, line)
		}
	}
}
