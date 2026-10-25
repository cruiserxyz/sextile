package ui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/rules"
	"github.com/cruiserxyz/sextile/internal/templates"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// A body template's Details tab (the specification): while
// Bodies is focused and its cursor row's body is listed by a shipped
// template, Details declares a second tab titled by the template, whose
// lines come from facts lookup obtains from Astrolog, the cited rules and the
// template. The tab's facts are fetched like the timeline's: a strict
// latest-only surface keyed by its inputs, the span's cycle search kept so a
// time step inside the span reruns only the Sun pair.

// tabTemplatePrefix starts a template tab's ID: tpl:<template id>.
const tabTemplatePrefix = "tpl:"

// shippedTemplates and shippedRules are loaded once. A template that does not
// load fails `go test ./templates`, so loadErr is never set in a build that
// passed ./check; if it is, no template tab is declared and the reason is in
// the Log at startup.
var shippedTemplates, shippedRules, templatesLoadErr = loadShippedTemplates()

func loadShippedTemplates() ([]templates.Template, rules.Set, error) {
	set, err := rules.Load()
	if err != nil {
		return nil, set, err
	}
	ts, err := templates.Shipped(factRegistry{}, set)
	return ts, set, err
}

// factRegistry is lookup's fact registry as the template loader reads it.
type factRegistry struct{}

func (factRegistry) FactKind(key string) (templates.Kind, bool) {
	spec, ok := lookup.FactSpecFor(key)
	if !ok {
		return 0, false
	}
	k := templateKind(spec.Kind)
	return k, k != 0
}

func (factRegistry) FactMax(key string) int {
	spec, _ := lookup.FactSpecFor(key)
	return spec.Max
}

// templateKind is a lookup fact kind as the renderer knows it. A switch, not
// a map: the shipped templates load during package initialisation, before a
// package-level map reached only through an interface method would be set.
func templateKind(k lookup.FactKind) templates.Kind {
	switch k {
	case lookup.FactValue:
		return templates.KindValue
	case lookup.FactInstant:
		return templates.KindInstant
	case lookup.FactSpan:
		return templates.KindSpan
	case lookup.FactList:
		return templates.KindList
	case lookup.FactCategory:
		return templates.KindCategory
	}
	return 0
}

// factSection adapts lookup's facts for one section to the renderer.
type factSection map[string]lookup.BodyFact

func (s factSection) Fact(key string) (templates.Fact, bool) {
	f, ok := s[key]
	if !ok {
		return templates.Fact{}, false
	}
	return templates.Fact{Kind: templateKind(f.Kind), Absent: f.Absent, Text: f.Text, Term: f.Term, Items: f.Items, Num: f.Num}, true
}

// cursorTemplate is the template for the Bodies cursor row's body, while
// Bodies is the focused panel (Details attached from it keeps it focused).
func (m Model) cursorTemplate() (templates.Template, aspect.Body, bool) {
	if m.focused != PanelBodies || templatesLoadErr != nil {
		return templates.Template{}, 0, false
	}
	rows := m.shownChart()
	body, ok := bodyAtCursor(rows.positions, rows.cusps, m.bodyCursorIdx)
	if !ok {
		return templates.Template{}, 0, false
	}
	t, ok := templates.ForBody(shippedTemplates, body.String())
	return t, body, ok
}

func templateTabID(t templates.Template) TabID { return TabID(tabTemplatePrefix + t.ID) }

func isTemplateTab(id TabID) bool { return strings.HasPrefix(string(id), tabTemplatePrefix) }

// templateKey is everything the tab's lines depend on.
type templateKey struct {
	template string
	body     aspect.Body
	when     int64
	lon, lat float64
	tz       string
	houses   recipe.HouseSystem
	zodiac   recipe.Zodiac
}

// templateState is the tab's lines (the last complete frame), the key they
// were asked for, and the cycle search kept for its span.
type templateState struct {
	key      templateKey
	keyed    bool
	complete bool
	lines    []templates.Line
	// done is the key whose run last ended, in success or failure; hasDone says
	// there was one. A tab whose key is not done is waiting.
	done     templateKey
	hasDone  bool
	cycle    *lookup.CycleResult
	cycleFor templateKey // the body and frame of cycle (when and place unset)
}

type templatePayload struct {
	key      templateKey
	cycle    *lookup.CycleResult
	lines    []templates.Line
	ruleRows []string
	slots    []templates.SlotTrace
	commands []string
}

func (templatePayload) isRefreshPayload() {}

// computeBodyFacts is the seam the tests stub.
var computeBodyFacts = lookup.BodyNowFacts

// templateInputs is the request and key for the tab: the floating slot's
// committed moment and place (@now binds the floating moment), in the
// committed lens's frame.
func (m Model) templateInputs() (templates.Template, lookup.BodyFactsRequest, templateKey, bool) {
	t, body, ok := m.cursorTemplate()
	if !ok || m.slots == nil {
		return t, lookup.BodyFactsRequest{}, templateKey{}, false
	}
	floating := m.slots.Floating()
	if floating.Location.TZ == "" || floating.Time.IsZero() {
		return t, lookup.BodyFactsRequest{}, templateKey{}, false
	}
	if _, err := time.LoadLocation(floating.Location.TZ); err != nil {
		return t, lookup.BodyFactsRequest{}, templateKey{}, false
	}
	lens := m.committedLens()
	view := m.viewLens()
	lens.Houses, lens.Zodiac = view.Houses, view.Zodiac
	key := templateKey{template: t.ID, body: body, when: floating.Time.UnixNano(), lon: floating.Location.Lon, lat: floating.Location.Lat,
		tz: floating.Location.TZ, houses: lens.Houses, zodiac: lens.Zodiac}
	return t, lookup.BodyFactsRequest{Body: body, When: floating.Time, Place: floating.Location, Lens: lens}, key, true
}

// syncTemplate starts the tab's run when, and only when, its inputs differ
// from the ones it was last asked for, while the tab is active and Details
// is in the layout. A different template or body drops the lines on screen
// (another body's facts never stand under this one's tab); a new moment
// keeps them until the new frame commits (gate 5).
func (m Model) syncTemplate() (Model, tea.Cmd) {
	if !m.detailsVisible() || !isTemplateTab(m.activeTabID(PanelDetails)) {
		return m, nil
	}
	_, _, key, ok := m.templateInputs()
	if !ok || (m.tpl.keyed && m.tpl.key == key) {
		return m, nil
	}
	if m.tpl.key.template != key.template || m.tpl.key.body != key.body {
		m.tpl.lines, m.tpl.complete = nil, false
	}
	m.tpl.key, m.tpl.keyed = key, true
	return m.bumpIdentity(surfaceTemplate)
}

// templateWaiting is detailsTabWaiting's template case: a run is owed or in
// flight for the current inputs. A run that failed awaits nothing.
func (m Model) templateWaiting() bool {
	_, _, key, ok := m.templateInputs()
	if !ok {
		return false
	}
	return !m.tpl.keyed || m.tpl.key != key || !m.tpl.hasDone || m.tpl.done != key
}

func buildTemplateCmd(m Model, ctx context.Context, runID, wantID uint64) tea.Cmd {
	t, req, key, ok := m.templateInputs()
	if !ok || !m.tpl.keyed || key != m.tpl.key {
		return nil
	}
	// No guessed system: a frame whose chart file could not be read casts
	// nothing, as the text, the wheel and Details refuse (F1).
	if m.unresolvedFor(m.focusedSlot()) != nil {
		return nil
	}
	// The kept cycle search is handed on only for the same body and frame
	// and a moment inside its span: a step into another span searches again.
	if cf := cycleKeyOf(key); m.tpl.cycle != nil && m.tpl.cycleFor == cf {
		if zone, err := time.LoadLocation(req.Place.TZ); err == nil && m.tpl.cycle.Span == lookup.CycleSpanFor(req.When, zone) {
			req.Cycle = m.tpl.cycle
		}
	}
	set := shippedRules
	return func() tea.Msg {
		p := templatePayload{key: key}
		res, err := computeBodyFacts(ctx, req)
		p.commands = res.Commands
		if res.Cycle.Span.Years > 0 {
			c := res.Cycle
			p.cycle = &c
		}
		if err == nil {
			var r templates.Rendered
			r, err = templates.Render(t, req.Body.String(), map[string]templates.Section{"now": factSection(res.Now)}, set)
			p.lines, p.ruleRows, p.slots = r.Lines, r.RuleRows, r.Slots
		}
		return refreshResultMsg{surface: surfaceTemplate, runID: runID, wantID: wantID, payload: p, err: err}
	}
}

// logHasArgv reports whether the Log still holds the command on its argv
// lane.
func (m Model) logHasArgv(command string) bool {
	for _, l := range m.logLines {
		if l.lane == laneArgv && l.text == command {
			return true
		}
	}
	return false
}

// cycleKeyOf is the part of a key the cycle search depends on besides its
// span: the template, the body and the frame.
func cycleKeyOf(k templateKey) templateKey {
	return templateKey{template: k.template, body: k.body, houses: k.houses, zodiac: k.zodiac}
}

// commitTemplate adopts a completed run: its commands and verdict rows go to
// the Log (P10) at this gate, on success and failure alike; a failure keeps
// the last complete frame (P5, gate 5).
func (m Model) commitTemplate(msg refreshResultMsg) (Model, tea.Cmd) {
	p, _ := msg.payload.(templatePayload)
	quiet := m.isQuiet(surfaceTemplate, msg.wantID)
	m = m.logCommandsFor(surfaceTemplate, msg.wantID, p.commands)
	if p.cycle != nil && cycleKeyOf(p.key) == cycleKeyOf(m.tpl.key) {
		m.tpl.cycle, m.tpl.cycleFor = p.cycle, cycleKeyOf(p.key)
	}
	if p.key != m.tpl.key {
		return m, nil
	}
	m.tpl.done, m.tpl.hasDone = p.key, true
	if msg.err != nil {
		m = m.noteEphemeris(msg.err)
		reason := m.failureReason(msg.err, m.consolePointer(len(p.commands) > 0))
		var missing *lookup.MissingEventError
		if errors.As(msg.err, &missing) {
			reason = "Astrolog's event list lacks " + missing.Event + "."
		}
		return m.logError("Couldn't compute the %s tab. %s", p.key.template, reason), nil
	}
	// A kept cycle search produced facts this frame shows too: its argv is
	// written again if the Log has evicted it since, so the Log carries every
	// argv behind what the tab shows (P10).
	if p.cycle != nil {
		for _, c := range p.cycle.Commands {
			if !m.logHasArgv(c) {
				m = m.logCommandsFor(surfaceTemplate, msg.wantID, []string{c})
			}
		}
	}
	// A run the minute tick started leaves the notes out too: the tab shows
	// the frame, and the notes are for runs the operator asked for.
	if len(p.ruleRows) > 0 && !quiet {
		m = m.logNote("template", "%s verdicts from rule rows %s", p.key.template, strings.Join(p.ruleRows, ", "))
	}
	// Each expression slot that ran says which branch it took: the template
	// line, the slot's place on it and the term or fact key it chose, never a
	// fact's value (T14, gate 17).
	for _, sl := range p.slots {
		if quiet {
			break
		}
		m = m.logNote("template", "%s:%d/%d → %s", p.key.template, sl.Line, sl.Slot, sl.Chose)
	}
	m.tpl.lines, m.tpl.complete = p.lines, true
	return m, nil
}
