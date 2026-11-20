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
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// A body template's Details tab (the specification): while
// Bodies is focused and its cursor row's object is listed by a shipped
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

// cursorTemplate is the template for the Bodies cursor row's object, while
// Bodies is the focused panel (Details attached from it keeps it focused),
// with the object and the subject id the template lists it by. Any row with an
// Object finds a template that lists it, typed or not.
func (m Model) cursorTemplate() (templates.Template, aspect.Object, string, bool) {
	if m.focused != PanelBodies || templatesLoadErr != nil {
		return templates.Template{}, aspect.Object{}, "", false
	}
	rows := m.shownChart()
	obj, ok := panel.ObjectAtCursor(rows.positions, rows.cusps, m.bodyCursorIdx)
	if !ok {
		return templates.Template{}, aspect.Object{}, "", false
	}
	t, subject, ok := templates.ForObject(shippedTemplates, obj)
	return t, obj, subject, ok
}

func templateTabID(t templates.Template) TabID { return TabID(tabTemplatePrefix + t.ID) }

func isTemplateTab(id TabID) bool { return strings.HasPrefix(string(id), tabTemplatePrefix) }

// templateKey is everything the tab's lines depend on.
type templateKey struct {
	template string
	object   int    // the Astrolog number of the Bodies row's object
	listsBy  string // the id the template lists the object by
	when     int64
	lon, lat float64
	tz       string
	houses   recipe.HouseSystem
	zodiac   recipe.Zodiac
	nodes    recipe.NodeForm
}

// templateState is the tab's lines (the last complete frame), the key they
// were asked for, and the cycle search kept for its span.
type templateState struct {
	key      templateKey
	keyed    bool
	complete bool
	lines    []templates.Line
	// content is the key the lines were computed for; hasContent says there
	// are lines. The view rule reads it (templatePaneState).
	content    templateKey
	hasContent bool
	// done is the key whose run last ended, in success or failure; hasDone says
	// there was one. A tab whose key is not done is waiting.
	done     templateKey
	hasDone  bool
	cycle    *lookup.CycleResult
	cycleFor templateKey // the object and frame of cycle (when and place unset)
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

// templateInputs is the request and key for the tab: the floating slot's
// committed moment and place (@now binds the floating moment), in the
// committed lens's frame.
func (m Model) templateInputs() (templates.Template, lookup.BodyFactsRequest, templateKey, bool) {
	t, obj, subject, ok := m.cursorTemplate()
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
	lens.Houses, lens.Zodiac, lens.Objects.Nodes = view.Houses, view.Zodiac, view.Objects.Nodes
	key := templateKey{template: t.ID, object: obj.Number, listsBy: subject, when: floating.Time.UnixNano(), lon: floating.Location.Lon, lat: floating.Location.Lat,
		tz: floating.Location.TZ, houses: lens.Houses, zodiac: lens.Zodiac, nodes: lens.Objects.Nodes}
	return t, lookup.BodyFactsRequest{Object: obj, When: floating.Time, Place: floating.Location, Lens: lens}, key, true
}

// templateKeyFor is the key of the run the Model would ask for now; ok is
// false when there is no request. The same key syncTemplate and
// buildTemplateCmd compare.
func (m Model) templateKeyFor() (templateKey, bool) {
	_, _, key, ok := m.templateInputs()
	return key, ok
}

// syncTemplate starts the tab's run when, and only when, its inputs differ
// from the ones it was last asked for, while the tab is active and Details
// is in the layout. A different template or object drops the lines on screen
// (another object's facts never stand under this one's tab); a new moment
// keeps them until the new frame commits (gate 5).
func (m Model) syncTemplate() (Model, tea.Cmd) {
	if !m.detailsVisible() || !isTemplateTab(m.activeTabID(PanelDetails)) {
		return m, nil
	}
	_, _, key, ok := m.templateInputs()
	if !ok || (m.tpl.keyed && m.tpl.key == key) {
		return m, nil
	}
	if m.tpl.key.template != key.template || m.tpl.key.object != key.object {
		m.tpl.lines, m.tpl.complete, m.tpl.hasContent = nil, false, false
	}
	m.tpl.key, m.tpl.keyed = key, true
	// A result already computed for these inputs is drawn now, from memory.
	if nm, ok := m.adoptTemplateFromMemory(key); ok {
		return nm, nil
	}
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

// newTemplateRow is the Template surface's refresh policy (specs/
// refresh-descriptor-template.md): the coordinator asks it through Model.rowFor.
// Its spawn is the body-facts seam the tests stub on their own Model.
// It has no rest timer: bumpIdentity starts the run at once and arms one
// defensive retry tick of textDebounce. The key is not in accept:
// commitTemplate logs the argv lines and keeps the cycle search first and
// then drops a result for a left key, so such a run still writes its argv line.
func newTemplateRow() surfaceRow {
	return surfaceRow{
		inputs: func(m Model) (refreshKey, bool) {
			k, ok := m.templateKeyFor()
			return k, ok
		},
		trigger:   triggerIdentity,
		supersede: supersedeHard,
		debounce:  textDebounce,
		build:     buildTemplateCmd,
		accept:    acceptExactWant,
		commit:    (Model).commitTemplate,
		suspend:   forgetIncompleteTemplate,
		spawn:     rowSpawns{bodyFacts: lookup.BodyNowFacts},
	}
}

// forgetIncompleteTemplate is the row's suspend: a tab's lines that never
// completed forget their key, so the run starts again when Details returns;
// lines still the previous moment's keep showing but forget the key too.
func forgetIncompleteTemplate(m Model) Model {
	if !m.tpl.complete {
		m.tpl = templateState{cycle: m.tpl.cycle, cycleFor: m.tpl.cycleFor}
	} else if m.tpl.done != m.tpl.key {
		// A run cancelled with the lines still the previous moment's: the key
		// is forgotten (the lines stay) so the run starts again on return.
		m.tpl.keyed = false
	}
	return m
}

func buildTemplateCmd(m Model, ctx context.Context, runID, wantID uint64) tea.Cmd {
	key, ok := m.templateKeyFor()
	if !ok || !m.tpl.keyed || key != m.tpl.key {
		return nil
	}
	return templateCmd(m, ctx, runID, wantID)
}

// templateCmd is the template compute for the key the Model asks for now,
// whether or not the tab has registered it: the surface's run reaches it
// through buildTemplateCmd, and the idle prefetch (prefetch.go) through a Model
// one key away, so both make the same call.
func templateCmd(m Model, ctx context.Context, runID, wantID uint64) tea.Cmd {
	t, req, key, ok := m.templateInputs()
	if !ok {
		return nil
	}
	// No guessed system: a frame whose chart file could not be read casts
	// nothing, as the text, the wheel and Details refuse (F1).
	if m.unresolvedFor(m.focusedSlot()) != nil {
		return nil
	}
	// The kept cycle search is handed on only for the same object and frame
	// and a moment inside its span: a step into another span searches again.
	if cf := cycleKeyOf(key); m.tpl.cycle != nil && m.tpl.cycleFor == cf {
		if zone, err := lookup.PlaceZone(req.When, req.Place); err == nil && m.tpl.cycle.Span == lookup.CycleSpanFor(req.When, zone) {
			req.Cycle = m.tpl.cycle
		}
	}
	set := shippedRules
	compute := m.rows[surfaceTemplate].spawn.bodyFacts
	return func() tea.Msg {
		p := templatePayload{key: key}
		res, err := compute(ctx, req)
		p.commands = res.Commands
		if res.Cycle.Span.Years > 0 {
			c := res.Cycle
			p.cycle = &c
		}
		if err == nil {
			var r templates.Rendered
			r, err = templates.Render(t, key.listsBy, map[string]templates.Section{"now": factSection(res.Now)}, set)
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
// span: the template, the object and the frame.
func cycleKeyOf(k templateKey) templateKey {
	return templateKey{template: k.template, object: k.object, houses: k.houses, zodiac: k.zodiac}
}

// commitTemplate adopts a completed run: its commands and verdict rows go to
// the Log (P10) at this gate, on success and failure alike; a failure keeps
// the last complete frame (P5, gate 5).
func (m Model) commitTemplate(msg refreshResultMsg) (Model, tea.Cmd) {
	p, _ := msg.payload.(templatePayload)
	quiet := m.isQuiet(surfaceTemplate, msg.wantID)
	m = m.logCommandsFor(surfaceTemplate, msg.wantID, p.commands, failedLast(msg.err, p.commands)...)
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
			reason = "Astrolog's list of events has no " + missing.Event
		}
		return m.logError("details", "the %s tab did not fill, %s", p.key.template, reason), nil
	}
	return m.landTemplate(p, msg.wantID, !quiet).rememberTemplate(p), nil
}

// landTemplate puts a computed template result on screen under its key. A fresh
// result and one adopted from memory land by it alike; notes says the notes of
// the run (its verdicts and the branch each slot took) are written too, which
// only a fresh result's are.
func (m Model) landTemplate(p templatePayload, wantID uint64, notes bool) Model {
	// A kept cycle search produced facts this frame shows too: its argv is
	// written again if the Log has evicted it since, so the Log carries every
	// argv behind what the tab shows (P10).
	if p.cycle != nil {
		for _, c := range p.cycle.Commands {
			if !m.logHasArgv(c) {
				m = m.logCommandsFor(surfaceTemplate, wantID, []string{c})
			}
		}
	}
	// A run the minute tick started leaves the notes out too: the tab shows
	// the frame, and the notes are for runs the operator asked for.
	if len(p.ruleRows) > 0 && notes {
		m = m.logTrace("template", "%s verdicts from rule rows %s", p.key.template, strings.Join(p.ruleRows, ", "))
	}
	// Each expression slot that ran says which branch it took: the template
	// line, the slot's place on it and the term or fact key it chose, never a
	// fact's value (T14, gate 17).
	for _, sl := range p.slots {
		if !notes {
			break
		}
		m = m.logTrace("template", "%s:%d/%d → %s", p.key.template, sl.Line, sl.Slot, sl.Chose)
	}
	m.tpl.lines, m.tpl.complete = p.lines, true
	m.tpl.content, m.tpl.hasContent = p.key, true
	return m
}
