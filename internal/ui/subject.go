package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// Every pane that shows a computed result (Details on each tab, Bodies,
// Aspects) shows it under the key it was computed for, and the screen draws a
// pane's content by one rule, evaluated at render time from the Model alone
// (specs/details-follows-its-tab.md C1-C7):
//
//	shown key == wanted key                  the content, bright
//	same subject, another version            the content, faint; caption ends ` …`
//	                                         (Details; Bodies and Aspects draw it bright)
//	another subject, or nothing is shown     an empty body with one faint `…`
//
// A key is everything a result was computed from. Its subject is the part of
// it that says WHAT the result is about (the panel, the chart, the object, the
// pair); everything else (a floating chart's moment and place, every lens
// field) is its version. Two equal subjects with different keys are the same
// subject at another version. The caption and the body are both read from the
// same keys, so no message order, timer or entry point can draw a caption and
// a content of different subjects.

// subject is what a result is about. It RETAINS the panel, the chart identity
// (a floating slot is the one subject `floating` whatever its moment and place),
// the reportable object, and the ordered pair identities; it DISCARDS the slot
// index, the order of the Chart list and every lens field.
type subject struct {
	frame  bool // a text-frame result (Bodies, Aspects) rather than a Details report
	panel  PanelID
	chart  string
	object int
	pair   string
	// tab names the Details tab of a Timeline or template result: `timeline`,
	// or the template's id. Empty for the report and the text frame.
	tab string
}

// paneState is where a pane stands against the key it wants.
type paneState int

const (
	paneCurrent  paneState = iota // the shown key is the wanted key: draw it bright
	paneUpdating                  // the same subject at another version: faint, caption ends ` …`
	paneEmpty                     // another subject, or nothing shown: one faint `…`
	paneFailed                    // the wanted Details report failed: one faint line saying so
)

// slotIdentity is a chart's identity as a subject sees it: the floating slot is
// always the one chart `floating`; an anchored chart is its name, label,
// moment and place.
func slotIdentity(s *store.Slot) string {
	switch {
	case s == nil:
		return ""
	case s.Floating:
		return "floating"
	}
	return fmt.Sprintf("%s|%s|%s|%d|%v", s.Chart.ID, s.Chart.Name, s.Chart.Label, s.Time.UnixNano(), s.Location)
}

// textKey is everything one committed text frame (Bodies and Aspects) was
// computed from: the scene (the cursor chart, the comparison, the active
// Aspects pair, the moment where it matters) and the lens.
type textKey struct {
	cursor    string
	pair      string // "<left>><right>" while an Aspects pair tab is active
	comparing bool
	set       string // the shown set, in numeral order, while comparing
	// when and place are the floating moment and place the frame is cast at;
	// zero where the frame does not read them (an anchored chart alone).
	when  int64
	place location.PlaceInfo
	// frames is the house system and zodiac of the cursor's chart and, while
	// comparing, of the frame chart and the sky.
	frames   string
	objects  string
	aspects  string
	trueNode bool
	only     string
}

func (k textKey) subject() subject {
	return subject{frame: true, chart: k.cursor, pair: k.pair}
}

// paneKey is the key one of the two text panes draws: the pane and the text
// frame's key. One committed frame feeds both, so each stamps its own.
type paneKey struct {
	pane PanelID
	text textKey
}

func (k paneKey) subject() subject {
	s := k.text.subject()
	s.panel = k.pane
	return s
}

// textKeyFor is the key of the text frame the Model asks for now; ok is false
// before a workspace exists. It reads the same state buildTextCmd casts from,
// so the key a run carries and the key the screen compares cannot disagree.
func (m Model) textKeyFor() (textKey, bool) {
	if m.slots == nil {
		return textKey{}, false
	}
	sc := m.deriveScene()
	if sc.cursor == nil {
		return textKey{}, false
	}
	k := textKey{cursor: slotIdentity(sc.cursor), comparing: sc.mode == sceneComparing}
	lens := m.viewLens()
	frames := []string{fmt.Sprintf("%d/%v", lens.Houses, lens.Zodiac)}
	if k.comparing {
		ids := make([]string, len(sc.shown))
		for i, s := range sc.shown {
			ids[i] = slotIdentity(s)
		}
		k.set = strings.Join(ids, ";")
		if sc.pair != nil {
			k.pair = slotIdentity(sc.pair.left) + ">" + slotIdentity(sc.pair.right)
		}
		sky := m.slots.Floating()
		for _, s := range []*store.Slot{sc.frame, &sky} {
			l := m.lensFor(s)
			frames = append(frames, fmt.Sprintf("%d/%v", l.Houses, l.Zodiac))
		}
	}
	k.frames = strings.Join(frames, ";")
	if sc.cursor.Floating || k.comparing {
		k.when = transitTimeFromWorkspace(m).UnixNano()
		k.place = m.place
	}
	show := m.showLens(sc.cursor)
	k.objects, k.aspects, k.trueNode = chooserKey(show)
	var only []string
	for _, n := range show.Objects.Only.Members() {
		only = append(only, strconv.Itoa(n))
	}
	k.only = strings.Join(only, ",")
	return k, true
}

// reportKind says which kind of content a Details report tab holds.
type reportKind int

const (
	reportNone  reportKind = iota
	reportRaw              // an Astrolog report of its own: the Chart's, a body's
	reportFrame            // the Aspects panel's list, copied from the text frame
)

// contentKey is the key of what the Details report tab holds or wants: a raw
// report's detailsKey, or the text frame the Aspects copy was taken from.
type contentKey struct {
	kind  reportKind
	raw   detailsKey
	frame paneKey
}

func (k contentKey) subject() subject {
	switch k.kind {
	case reportRaw:
		return k.raw.subject()
	case reportFrame:
		return k.frame.subject()
	}
	return subject{}
}

// subject of a raw report: the panel, the chart (or, for a pair report, the
// ordered pair), and the object. The moment, place and lens are its version.
func (k detailsKey) subject() subject {
	s := subject{panel: k.panel, object: k.object, chart: k.chart}
	if k.cross {
		s.chart = k.natalChart
		s.pair = k.natalChart + ">" + k.otherChart
	}
	return s
}

// subject of a Timeline: the compared charts (the sky alone while single). The
// month, the zone and every lens field are its version.
func (k timelineKey) subject() subject {
	return subject{panel: PanelChart, tab: string(TabDetailsTimeline), chart: k.natals}
}

// subject of a template tab: the template and the object it lists. The moment,
// the place and the frame are its version.
func (k templateKey) subject() subject {
	return subject{panel: PanelBodies, tab: tabTemplatePrefix + k.template, object: k.object, chart: k.listsBy}
}

// timelinePaneState is the view rule for the Timeline tab (C3-C5), read from
// the key it asks for and the key of the list it can draw. With no search to
// ask for it draws as it always did.
func (m Model) timelinePaneState() paneState {
	want, ok := m.timelineKeyFor()
	if !ok {
		return paneCurrent
	}
	if !m.timeline.hasContent {
		return paneEmpty
	}
	switch {
	case m.timeline.complete && m.timeline.content == want:
		return paneCurrent
	case m.timeline.content.subject() == want.subject():
		return paneUpdating
	}
	return paneEmpty
}

// templatePaneState is the view rule for a template tab (C3-C5).
func (m Model) templatePaneState() paneState {
	want, ok := m.templateKeyFor()
	if !ok {
		return paneCurrent
	}
	if !m.tpl.hasContent {
		return paneEmpty
	}
	switch {
	case m.tpl.content == want:
		return paneCurrent
	case m.tpl.content.subject() == want.subject():
		return paneUpdating
	}
	return paneEmpty
}

// shownPaneKey is the key of the frame Bodies or Aspects draws, ok false when
// no frame is shown (nothing committed since the last reset).
func (m Model) shownPaneKey(p PanelID) (paneKey, bool) {
	if !m.shown.keyed || !m.shownFrameValid() {
		return paneKey{}, false
	}
	return paneKey{pane: p, text: m.shown.key}, true
}

// wantedPaneKey is the key Bodies or Aspects asks for now.
func (m Model) wantedPaneKey(p PanelID) (paneKey, bool) {
	k, ok := m.textKeyFor()
	return paneKey{pane: p, text: k}, ok
}

// shownFrameValid says a text commit is on screen: held, or the live chart
// itself. A hard reset zeroes the chart epoch and with it the frame.
func (m Model) shownFrameValid() bool {
	return m.shown.stamped && (m.holding() || m.chart.epoch != 0)
}

// textPaneState is the view rule for Bodies and Aspects (C3, C5). Before the
// first text frame has ever been committed nothing is shown, and the panels
// read one faint `…` (C5: "or nothing is shown"), unless the live chart holds
// rows nothing stamped (a Model built by hand), which are drawn as they stand.
// The main panels have no faint version: a frame of the subject they ask for is
// drawn as it is until the next one replaces it in one go, as the Chart panel
// names the moment it was cast at (operator, 2026-11-06: "flicker free has
// prority"). Only another subject empties them.
func (m Model) textPaneState(p PanelID) paneState {
	if !m.shown.stamped {
		if len(m.chart.positions) == 0 && m.chart.epoch == 0 {
			return paneEmpty
		}
		return paneCurrent
	}
	want, ok := m.wantedPaneKey(p)
	if !ok {
		return paneCurrent
	}
	shown, ok := m.shownPaneKey(p)
	if ok && shown.subject() == want.subject() {
		return paneCurrent
	}
	return paneEmpty
}

// chartCaption is how a chart's report is captioned: the floating chart is
// `Now`, an anchored one its name.
func chartCaption(s *store.Slot) string {
	switch {
	case s == nil:
		return tabTitleChart
	case s.Floating:
		return "Now"
	case s.Chart.Name != "":
		return s.Chart.Name
	}
	return tabTitleChart
}

// detailsWant is what the Details report tab asks for now: its caption in the
// frame of the key (always now), and the key of the content that belongs under
// it. has is false when nothing is owed (a plain cusp row, Details not in the
// layout, startup not ready).
type detailsWant struct {
	caption string
	key     contentKey
	has     bool
}

// detailsWanted derives the report tab's caption and key from the focused
// panel and its cursor alone.
func (m Model) detailsWanted() detailsWant {
	switch m.focused {
	case PanelBodies:
		if title, _, ok := m.detailsBodyTitle(); ok {
			w := detailsWant{caption: strings.TrimPrefix(title, detailsPrefix)}
			if k, ok := m.detailsKeyFor(); ok {
				w.key, w.has = contentKey{kind: reportRaw, raw: k}, true
			}
			return w
		}
		// A row that is no object (no row at all, or an index past them) owes no
		// report: the panel's own title, no key, one `…` (C18).
		return detailsWant{caption: tabTitleBodies}
	case PanelAspects:
		w := detailsWant{caption: m.activeAspectsTitle()}
		if !m.detailsVisible() {
			return w
		}
		if pk, ok := m.wantedPaneKey(PanelAspects); ok {
			w.key, w.has = contentKey{kind: reportFrame, frame: pk}, true
		}
		return w
	}
	w := detailsWant{caption: chartCaption(m.focusedSlot())}
	if k, ok := m.detailsKeyFor(); ok {
		w.key, w.has = contentKey{kind: reportRaw, raw: k}, true
	}
	return w
}

// activeAspectsTitle is the title of the Aspects tab the panel shows now.
func (m Model) activeAspectsTitle() string {
	tabs := m.panelTabs(PanelAspects)
	return tabs[activeTabIndex(tabs, m.tabChoice(PanelAspects))].Title
}

// detailsReportState is the view rule for the Details report tab.
func (m Model) detailsReportState() paneState {
	w := m.detailsWanted()
	if w.has && w.key.kind == reportRaw && m.details.failed.set && m.details.failed.key == w.key.raw {
		return paneFailed
	}
	if !m.details.keyed {
		// Content no commit stamped can only be a Model built by hand: it is
		// drawn as it stands. Every writer in the program stamps its key.
		if m.details.body != "" || m.details.title != "" {
			return paneCurrent
		}
		return paneEmpty
	}
	if !w.has {
		return paneEmpty
	}
	switch {
	case m.details.key == w.key:
		return paneCurrent
	case m.details.key.subject() == w.key.subject():
		return paneUpdating
	}
	return paneEmpty
}

// detailsBodyShown is the report text the pane draws and every action on it
// (scroll, `/`, copy) reads: the shown content while it is drawn, and nothing
// while the pane is empty or shows its failure line.
func (m Model) detailsBodyShown() string {
	switch m.detailsReportState() {
	case paneCurrent, paneUpdating:
		return m.details.body
	}
	return ""
}

// failedReportLine is the one faint line a failed report reads, the subject
// as the caption names it. Sun and Moon take English's article ("the Sun's
// report", the operator's own wording, 2026-11-05); every other name, an
// object's from Astrolog or a chart's, stands bare.
func failedReportLine(name string) string {
	if name == "Sun" || name == "Moon" {
		name = "the " + name
	}
	return "Couldn't read " + name + "'s report."
}

// emptyPaneLine is the one faint line an empty pane reads.
const emptyPaneLine = "…"
