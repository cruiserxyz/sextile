package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// TabID is the typed identity of one named content choice inside a panel
// . Like PanelID it is a string so it reads in logs and goldens.
type TabID string

const (
	TabChart   TabID = "chart"
	TabBodies  TabID = "bodies"
	TabAspects TabID = "aspects"
	// TabAspectsCross is REUSED, not retired: it is the ID of the FIRST declared
	// pair tab, which every comparing selected set has (`N in ①` with the
	// floating slot selected, `k₁ in c` without it). The other pairs are
	// "aspects-pair-<left>-<right>" (the specification),
	// so a reader of the old ID still finds the first pair.
	TabAspectsCross TabID = "aspects-cross"
	// Details declares two tabs on every subject (specs/details-timeline-
	// tab.md C11): Astrolog's report for the subject, and the dated-event
	// timeline for the floating slot's month.
	TabDetailsReport   TabID = "report"
	TabDetailsTimeline TabID = "timeline"
	TabLog             TabID = "log"
)

// Tab is one entry of a panel's derived tab list: the typed identity and the
// title the strip prints.
type Tab struct {
	ID    TabID
	Title string
}

// tabStrip is the resolved title-border content of one panel: the Jump-digit
// prefix (`[3] `, empty for Log), the derived tab list and the effective
// active index. A one-tab strip renders byte-identical to the pre-tab title
// (prefix + title in one styled run).
type tabStrip struct {
	prefix string
	tabs   []Tab
	active int
}

// tabTitleFor is the tab title a one-tab panel prints; the compose-time
// prefix (`[1] `, `[2] `, …) is kept separate so the strip can style the
// prefix and each tab title independently.
const (
	tabTitleChart   = "Chart"
	tabTitleBodies  = "Bodies"
	tabTitleAspects = "Aspects"
	// The comparing Aspects panel has no fixed title: each pair tab is titled
	// `② in ①` … (pairTabTitle, C1). The old "Transit aspects" title is gone
	// with the single cross tab it named.
	tabTitleTimeline = "Timeline"
	tabTitleLog      = "Log"
)

// aspectsStripPrefix is the Aspects strip's Jump-digit prefix.
const aspectsStripPrefix = "[3] "

// detailsPendingMark ends the active Details tab's title, colours off, while
// that tab's content for the current scene is still to come.
const detailsPendingMark = " …"

// detailsPrefix is Details' strip prefix and the head of every report title:
// `0` is Details' digit as `1`/`2`/`3` are the ring's (D14, D16), and the Log
// keeps none.
const detailsPrefix = "[0] "

// panelTabs derives a panel's tab list from state on every call. It is the
// only place that knows which tabs a panel has; the list is never stored
// . Chart, Bodies and Log each declare exactly one tab. Aspects declares
// `Aspects` (the cursor's own chart) and, while comparing, one tab per declared
// pair (the specification rules 1-2). The
// id drives the renderer (aspects_tabs.go) and the Details report
// (details.go) together, so the panel body, its title, and its report can
// never disagree.
//
// Details declares `report` and `timeline` for every subject and in every
// focus and attachment state (C11, C32). The report tab's title is the
// report's own context title — `Details: Chart`, `Details: Moon` — so the
// strip still says what the report is about.
func (m Model) panelTabs(p PanelID) []Tab {
	switch p {
	case PanelChart:
		return []Tab{{ID: TabChart, Title: tabTitleChart}}
	case PanelBodies:
		return []Tab{{ID: TabBodies, Title: m.bodiesTitle()}}
	case PanelAspects:
		// `Aspects` (the cursor's own chart) always comes first; while
		// comparing, one tab per declared pair follows in numeral order: the
		// cursor's chart visited by each other member (`k in c`, the pair with the
		// floating slot last), or on the floating slot each anchored member
		// visited by now (`N in k`) (the specification).
		tabs := []Tab{{ID: TabAspects, Title: tabTitleAspects}}
		sc := m.deriveScene()
		for i := range sc.pairs {
			tabs = append(tabs, Tab{ID: sc.pairTabID(i), Title: sc.pairTitle(i, m.cfg)})
		}
		return tabs
	case PanelDetails:
		// The caption names the report's subject, never "Details" itself
		// (the specification); before any report has
		// arrived it names the focused panel.
		report := strings.TrimPrefix(m.details.title, detailsPrefix)
		if report == "" {
			report = focusedPanelTitle(m.focused)
		}
		tabs := []Tab{{ID: TabDetailsReport, Title: report}}
		// The timeline belongs to the Chart panel and to the floating chart's
		// month: see timelineTabOffered for whose cursor it is offered on.
		if m.timelineTabOffered() {
			tabs = append(tabs, Tab{ID: TabDetailsTimeline, Title: tabTitleTimeline})
		}
		// A body a shipped template lists gains the template's tab while its
		// Bodies row has the cursor (the specification P1).
		if t, _, ok := m.cursorTemplate(); ok {
			tabs = append(tabs, Tab{ID: templateTabID(t), Title: t.Tab})
		}
		return tabs
	case PanelLog:
		return []Tab{{ID: TabLog, Title: tabTitleLog}}
	}
	return nil
}

// bodiesTitle is the Bodies panel's title: the active Aspects pair tab's own
// string while a pair is on screen ("② in ①", the cursor's bodies read in the
// base's houses), else "Bodies". It reads the strip Aspects is drawing, the
// held one included, so the two titles always name the view the panels show.
func (m Model) bodiesTitle() string {
	strip := m.shown.aspects
	if !m.holding() {
		tabs := m.panelTabs(PanelAspects)
		strip = tabStrip{tabs: tabs, active: activeTabIndex(tabs, m.tabChoice(PanelAspects))}
	}
	if id := strip.activeID(); isAspectsPairTab(id) {
		for _, tab := range strip.tabs {
			if tab.ID == id {
				return tab.Title
			}
		}
	}
	return tabTitleBodies
}

// timelineTabOffered is whether Details declares the Timeline tab right now:
// the Chart panel is focused and the cursor is on the floating slot (alone,
// the reduced sky-only list, or in a group, the full one) or on a member of a
// selected group that contains the floating slot (the full list). An
// anchored chart outside the group, or a group without the floating slot,
// has no Timeline tab at all (timeline-tab-rules T1 rule 1). It reads the
// cursor and the selection only, so it is re-evaluated at every call, like
// the rest of the tab list.
func (m Model) timelineTabOffered() bool {
	if m.focused != PanelChart || m.slots == nil {
		return false
	}
	sc := m.deriveScene()
	if sc.cursor == nil {
		return false
	}
	if sc.cursor.Floating {
		return true
	}
	if sc.numeral(sc.cursor) == 0 {
		return false
	}
	for _, member := range sc.set {
		if member.Floating {
			return true
		}
	}
	return false
}

// focusedPanelTitle is the ring panel's own title, which the Details caption
// shows until a report names a subject. Details is never the focused panel
// (C15), so the Chart's title covers every other case.
func focusedPanelTitle(p PanelID) string {
	switch p {
	case PanelBodies:
		return tabTitleBodies
	case PanelAspects:
		return tabTitleAspects
	}
	return tabTitleChart
}

// detailsTabWaiting is THE answer to "is this Details tab waiting for new
// content?" (the specification). Everything that looks
// different or ignores keys while a tab waits reads it: the dimmed body
// (renderDetailsPanel), the ` …` title mark (tabStrips) and the row-key gate
// (updateDetailsKey). A future Details tab joins by adding a case here that
// says when its content is owed or in flight, and, if it has a row cursor, by
// being named in detailsTabHasRowCursor; no other site needs to change.
func (m Model) detailsTabWaiting(id TabID) bool {
	switch {
	case id == TabDetailsReport:
		return m.details.pending
	case id == TabDetailsTimeline:
		return m.timelineWaiting()
	case isTemplateTab(id):
		return m.templateWaiting()
	}
	return false
}

// detailsTabDimmed is whether the tab draws faint: waiting for content, or
// held while a navigation key repeats (followKeyRun). A wait a tap started
// is not drawn faint: the previous content stays as drawn until the new
// content paints. Only the look follows the hold; keys follow
// detailsTabWaiting alone, so a timeline Enter does not lock the rows for
// the hold's length.
func (m Model) detailsTabDimmed(id TabID) bool {
	return m.detailsHold.on || (m.detailsTabWaiting(id) && !m.detailsHold.tapped)
}

// detailsTabHasRowCursor is the one declaration of which Details tabs carry a
// row cursor. A waiting tab of this kind ignores its row keys (↑/↓/j/k, Enter,
// Space, `/` and the scroll keys): the rows on screen are the previous inputs'
// and must not be acted on. A future tab with a row cursor is added here.
func detailsTabHasRowCursor(id TabID) bool {
	return id == TabDetailsTimeline
}

// detailsRowKeysBlocked is whether the active Details tab has a row cursor
// and is waiting.
func (m Model) detailsRowKeysBlocked() bool {
	id := m.activeTabID(PanelDetails)
	return detailsTabHasRowCursor(id) && m.detailsTabWaiting(id)
}

// tabChoice is the stored CHOICE within a panel's derived list: only the choice is stored,
// never the list. Details' choice lives beside the rest of its presentation
// state.
//
// Aspects' choice is derived from the stored base (Model.base), spelled as the
// active pair's tab ID. No active pair is the `Aspects` tab.
func (m Model) tabChoice(p PanelID) TabID {
	switch p {
	case PanelDetails:
		return m.details.tabs[m.focused]
	case PanelAspects:
		if sc := m.deriveScene(); sc.pair != nil {
			return sc.pairTabID(sc.pairIndex())
		}
		return TabAspects
	}
	return ""
}

// withDetailsTab stores Details' tab choice for panel p, copy on write.
func (m Model) withDetailsTab(p PanelID, id TabID) Model {
	tabs := make(map[PanelID]TabID, len(m.details.tabs)+1)
	for k, v := range m.details.tabs {
		tabs[k] = v
	}
	tabs[p] = id
	m.details.tabs = tabs
	return m
}

// activeTabIndex resolves a stored choice against the currently declared
// list. A choice that is not declared right now — or no choice at all —
// makes the first tab active WITHOUT touching the stored choice, so a tab
// that is declared again becomes active again.
func activeTabIndex(tabs []Tab, choice TabID) int {
	for i, t := range tabs {
		if t.ID == choice {
			return i
		}
	}
	return 0
}

// activeTabID is the identity of the panel's active tab; "" for a panel with
// no tabs. The strip, the renderer and the key handlers all read it, so the
// id a caller gets is always the one the strip is printing.
func (m Model) activeTabID(p PanelID) TabID {
	tabs := m.panelTabs(p)
	if len(tabs) == 0 {
		return ""
	}
	return tabs[activeTabIndex(tabs, m.tabChoice(p))].ID
}

// focusedView is the view the tab keys act on (C34): Details while Details
// is attached, the Aspects panel while the Chart panel is focused, the focused
// ring panel otherwise. One definition, because m.focused can never be Details
// (C15, C40) while `[`/`]` must still reach Details' tabs when it is attached
// (D15). The Chart panel has one tab, and the Aspects tab picks the base the
// cursor's chart is read against, so from the rows `[`/`]` change the base
// without moving focus (operator, 2026-10-24: "Approve, but ] from Chart too").
func (m Model) focusedView() PanelID {
	if m.details.attached {
		return PanelDetails
	}
	if m.focused == PanelChart {
		return PanelAspects
	}
	return m.focused
}

// cycleTab makes the view's next (step +1) or previous (step -1) declared
// tab active, wrapping. A view with one tab is left exactly as it was, with
// no command (C36). A Details switch is presentation only: it starts no
// compute and bumps no refresh surface (C38). An Aspects pair switch changes
// WHICH pair is computed, so it also starts the recompute (C1b). Neither ever
// touches focus, attachment or the slot cursor.
//
// Before the workspace is ready there is nothing on either tab to choose
// between, so the keys wait for it.
func (m Model) cycleTab(p PanelID, step int) (Model, tea.Cmd) {
	tabs := m.panelTabs(p)
	if len(tabs) <= 1 || m.slots == nil || m.startup != startupReady {
		return m, nil
	}
	next := (activeTabIndex(tabs, m.tabChoice(p)) + step + len(tabs)) % len(tabs)
	switch p {
	case PanelDetails:
		m = m.withDetailsTab(m.focused, tabs[next].ID)
		m = m.clampTimelineCursor()
		// Choosing a tab makes it the visible one, and the visible tab
		// catches up (the specification rules 2 and 3).
		switch id := tabs[next].ID; {
		case id == TabDetailsReport:
			return m.catchUpDetails()
		case id == TabDetailsTimeline:
			return m.syncTimeline()
		}
	case PanelAspects:
		// Index 0 is `Aspects` (no pair); i > 0 is the pair of pairs[i-1].
		pairs := m.pairSlots()
		// The choice is a deliberate act and writes the base: the tab's left
		// chart, nil for the own tab.
		if next == 0 {
			m = m.withBase(nil)
		} else if next <= len(pairs) {
			m = m.withBase(pairs[next-1].left)
		} else {
			return m, nil
		}
		return m.afterPairChange()
	}
	return m, nil
}

// tabStrips resolves every titled panel's strip for one render.
func (m Model) tabStrips() map[PanelID]tabStrip {
	strips := make(map[PanelID]tabStrip, 5)
	for p, prefix := range map[PanelID]string{
		PanelChart:   "[1] ",
		PanelBodies:  "[2] ",
		PanelAspects: aspectsStripPrefix,
		PanelDetails: detailsPrefix,
		PanelLog:     "",
	} {
		if p == PanelAspects && m.holding() {
			// The previous scene's strip, tabs and active one, until its frame
			// swaps in with the body it names.
			strips[p] = m.shown.aspects
			continue
		}
		tabs := m.panelTabs(p)
		active := activeTabIndex(tabs, m.tabChoice(p))
		if p == PanelDetails && len(tabs) > 0 && !m.cfg.useColor() && m.detailsTabDimmed(tabs[active].ID) {
			// A waiting active tab is told apart without colour by a trailing
			// ellipsis (the specification rule 6); with
			// colour its body dims instead (renderDetailsPanel).
			tabs = append([]Tab(nil), tabs...)
			tabs[active].Title += detailsPendingMark
		}
		strips[p] = tabStrip{prefix: prefix, tabs: tabs, active: active}
	}
	return strips
}

// stripRole tags one run of the rendered strip so the border renderer can
// style prefix, separators, the active title and inactive titles apart.
type stripRole int

const (
	roleChrome   stripRole = iota // the `[N] ` prefix, ` - ` separators, ` +N`
	roleActive                    // the active tab's title (with marker in accessible)
	roleInactive                  // any other included tab's title
)

type stripRun struct {
	text string
	role stripRole
}

const (
	tabSeparator = " - "
	// tabMarkerOpen/Close wrap the active title when color is unavailable so
	// the choice is still visible without bold.
	tabMarkerOpen  = "["
	tabMarkerClose = "]"
)

// layoutTabStrip fits a multi-tab strip into maxCells. It keeps included tabs
// in list order and always retains the active tab; when the strip overflows
// it omits non-active tabs after the active one from the end first, then
// non-active tabs before it from nearest to farthest, appending ` +N` for the
// N omitted. If even the active title plus ` +N` cannot fit, the marker goes
// and the active title itself is truncated to the remaining cells.
func layoutTabStrip(prefix string, tabs []Tab, active int, marker bool, maxCells int, cfg Config) []stripRun {
	if active < 0 || active >= len(tabs) {
		active = 0
	}
	included := make([]int, len(tabs))
	for i := range tabs {
		included[i] = i
	}
	omitted := 0
	build := func(incl []int, omit int, title string) []stripRun {
		runs := []stripRun{{text: prefix, role: roleChrome}}
		for k, i := range incl {
			if k > 0 {
				runs = append(runs, stripRun{text: tabSeparator, role: roleChrome})
			}
			t := tabs[i].Title
			if i == active {
				t = title
				if marker {
					t = tabMarkerOpen + t + tabMarkerClose
				}
				runs = append(runs, stripRun{text: t, role: roleActive})
				continue
			}
			runs = append(runs, stripRun{text: t, role: roleInactive})
		}
		if omit > 0 {
			runs = append(runs, stripRun{text: fmt.Sprintf(" +%d", omit), role: roleChrome})
		}
		return runs
	}
	activeTitle := tabs[active].Title
	for {
		runs := build(included, omitted, activeTitle)
		if stripWidth(runs) <= maxCells {
			return runs
		}
		if len(included) == 1 {
			break
		}
		// Drop from the tail while the tail is not the active tab; once the
		// active tab is last, drop its nearest predecessor.
		if last := included[len(included)-1]; last != active {
			included = included[:len(included)-1]
		} else {
			included = append(included[:len(included)-2], included[len(included)-1])
		}
		omitted++
	}
	// Only the active tab is left and it still overflows with ` +N`: drop the
	// marker and truncate the title to what remains after the prefix (and the
	// accessible brackets).
	budget := maxCells - ansi.StringWidth(prefix)
	if marker {
		budget -= ansi.StringWidth(tabMarkerOpen) + ansi.StringWidth(tabMarkerClose)
	}
	if budget < 0 {
		budget = 0
	}
	return build(included, 0, cutRight(activeTitle, budget, cfg))
}

func stripWidth(runs []stripRun) int {
	w := 0
	for _, r := range runs {
		w += ansi.StringWidth(r.text)
	}
	return w
}

// activeID is the effective active tab's identity; "" for an empty strip.
func (s tabStrip) activeID() TabID {
	if len(s.tabs) == 0 {
		return ""
	}
	if s.active < 0 || s.active >= len(s.tabs) {
		return s.tabs[0].ID
	}
	return s.tabs[s.active].ID
}
