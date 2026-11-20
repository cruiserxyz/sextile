// Package popup is the body selector (the bodies popup): which objects the chart
// draws. It holds the popup's state, its rows and its rendering, and takes its
// keys as Actions the Model resolves from its bindings table, so it reads
// neither the table nor the Model.
package popup

import (
	"context"
	"fmt"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// body.go is the body selector (⌃b): which objects the chart draws.
//
// It is the aspect chooser's sibling and deliberately the SMALLER of the two,
// because it binds the chassis that slice extracted rather than growing its
// own. Every piece of list machinery here comes from menu.FuzzyList — the search
// buffer, the cursor, the window and its scroll margin — and the box chrome
// from dialog.Render. This file declares no list, no cursor and no search
// editor of its own; that is C6, and it is the whole argument for the
// extraction having been worth doing.
//
// Two shape differences from the aspect chooser:
//
//   - GROUP ROWS above the list (the specification), the aspect
//     families' shape: seven tri-state toggles pinned over a dashed rule, and
//     one combined cursor that walks them first, so the list index is the
//     cursor minus GroupRows().
//   - A BOTTOM LINE, below the list and a blank buffer line: the count of ticked
//     bodies and the legend of the group marks. 134 rows do not fit on any
//     screen, so the operator cannot see their selection by scanning the list;
//     the count is where the size of the chosen set stays visible while they
//     filter.

// Body is the selector's state. The list machinery is embedded, so
// FilterText, cursor, matches and truncated all come from the chassis.
type Body struct {
	menu.FuzzyList[aspect.RegistryObject]

	// registry is every object the user's binary carries, read once at
	// startup from `astrolog -R1 -HO`. Nil until it has been read; the modal
	// refuses to open.
	Registry []aspect.RegistryObject

	// selected holds Astrolog object NUMBERS the operator has ticked. The
	// registry is 0-based and runs to 133.
	Selected map[int]bool

	// Defaults are the objects the lens draws when the operator has chosen
	// nothing, as Astrolog's own numbers: what the Restore row ticks. The model supplies
	// them (only it can ask the lens), and nil means it had none to give.
	Defaults []int

	// entry is the policy in force when the modal opened. Only the switched-off
	// rule (chooserCloseKeeps = false) restores it, on Esc; by default every
	// closing key keeps the ticks.
	Entry recipe.ObjectPolicy

	// The ephemeris probe (the specification). All of it
	// lives and dies with this opening of the popup, so closing forgets every
	// verdict (C6) and a result that outlives its opening is recognised by
	// seq and dropped (N6).
	//
	// seq numbers the opening. probes holds one verdict per probed body, each
	// tagged with the inputs it was asked under. inputs are the inputs now in
	// force. restID is the latest armed rest timer; an older one is stale.
	// cancel ends the probe in flight.
	Seq    uint64
	Probes map[int]ProbeEntry
	// absent is the startup probe's answer: object number to the file it lacks.
	Absent map[int]string
	// Said is what Astrolog wrote for each absent object (object number to its
	// first stderr line); no entry when it wrote nothing.
	Said   map[int]string
	Inputs ProbeInputs
	RestID uint64
	Cancel context.CancelFunc
	// inflight is the body whose probe is running now, if any.
	Inflight   int
	InflightOn bool
}

// ProbeInputs is everything a probe's answer depends on besides the body: the
// focused slot's moment and place, its effective frame and the node mode.
// Comparable, so "the same inputs" is ==.
type ProbeInputs struct {
	When     int64
	Lat, Lon float64
	TZ       string
	Houses   recipe.HouseSystem
	Zodiac   recipe.Zodiac
	Nodes    recipe.NodeForm
}

// ProbeEntry is one verdict: Drawable, or not with the files Astrolog named
// and the line it wrote ("" when it wrote nothing).
type ProbeEntry struct {
	Inputs   ProbeInputs
	Drawable bool
	Files    []string
	Said     string
}

// objectUnderCursor is the registry object on the cursor row, if any. The
// cursor walks the group rows first, so on one of them there is none.
func (b Body) ObjectUnderCursor() (aspect.RegistryObject, bool) {
	i := b.Cursor - GroupRows()
	if i < 0 || i >= len(b.Matches) {
		return aspect.RegistryObject{}, false
	}
	return b.Matches[i], true
}

// setProbe records a verdict for a body under the inputs it was asked with.
func (b *Body) SetProbe(obj int, in ProbeInputs, p lookup.BodyProbe) {
	if b.Probes == nil {
		b.Probes = map[int]ProbeEntry{}
	}
	b.Probes[obj] = ProbeEntry{Inputs: in, Drawable: p.Drawable, Files: p.MissingFiles, Said: p.Said}
}

// recordProbe stores a verdict under the inputs now in force.
func (b *Body) RecordProbe(obj int, drawable bool, files []string) {
	b.SetProbe(obj, b.Inputs, lookup.BodyProbe{Drawable: drawable, MissingFiles: files})
}

// undrawable is the verdict for obj when it is current and negative.
func (b Body) Undrawable(obj int) (ProbeEntry, bool) {
	// The startup probe already knows which objects have no file at all.
	if file, absent := b.Absent[obj]; absent {
		var files []string
		if file != "" {
			files = []string{file}
		}
		return ProbeEntry{Inputs: b.Inputs, Files: files, Said: b.Said[obj]}, true
	}
	e, ok := b.Probes[obj]
	if !ok || e.Drawable || e.Inputs != b.Inputs {
		return ProbeEntry{}, false
	}
	return e, true
}

// SetAbsent takes the startup probe's answer (the file each absent object
// lacks and the line Astrolog wrote for it) and lists again: the rows it names
// are ordered by it (Refilter).
func (b *Body) SetAbsent(absent, said map[int]string) {
	b.Absent = absent
	b.Said = said
	b.Refilter()
}

// stopProbe ends the probe in flight; its result, if one still arrives, is
// silent.
func (b *Body) StopProbe() {
	b.InflightOn = false
	if b.Cancel != nil {
		b.Cancel()
		b.Cancel = nil
	}
}

// The box keeps two lines at the bottom (dialog.Box.Zone): a blank buffer line
// and the bottom line, which is the count and the legend (TagBar) or, while the
// cursor is on an object Astrolog cannot draw, the message about its missing
// file in their place (operator, 2026-11-04). Neither wraps into the list
// height, so the window cannot resize under a moving cursor.

// refilter recomputes the visible objects. A body matches by its own name or by
// the name of its group (typing "angles" lists the four), both through
// location.MatchTier, the group's only by its exact or prefix tier, and its
// own-name matches rank before every group-name match, best tier first in each, ties in registry order. The name is the only
// text a row carries: the registry has no abbreviation field.
//
// A row Astrolog cannot draw comes after every row it can: with no filter the
// drawable objects in registry order, then all the others in registry order,
// across the groups; with a filter the drawable matches in their rank order,
// then the others in theirs (operator, 2026-11-05). The order is made when the
// list is made, so a verdict that lands later marks its row where it stands and
// never moves it under the cursor.
func (b *Body) Refilter() {
	if b.Registry == nil {
		b.Clear()
		return
	}
	q := strings.ToLower(strings.TrimSpace(b.FilterText))
	if q == "" {
		b.SetMatches(b.undrawableLast(), false)
		return
	}
	var hits, gone []aspect.RegistryObject
	var ranks, goneRanks []int
	for _, o := range b.Registry {
		rank := 0
		if t := location.MatchTier(strings.ToLower(o.Name), q); t > 0 {
			rank = ownNameRank + t
		} else if t := location.MatchTier(strings.ToLower(recipe.ObjectGroups[recipe.ObjectGroupOf(o.Number)].Name), q); t >= groupNameTier {
			rank = t
		}
		if rank == 0 {
			continue
		}
		if _, bad := b.Undrawable(o.Number); bad {
			gone = append(gone, o)
			goneRanks = append(goneRanks, rank)
			continue
		}
		hits = append(hits, o)
		ranks = append(ranks, rank)
	}
	b.SetMatches(append(menu.RankByTier(hits, ranks), menu.RankByTier(gone, goneRanks)...), false)
}

// ownNameRank lifts every own-name tier (1 to 4) above every group-name tier.
const ownNameRank = 4

// groupNameTier is the weakest match of a group's name that lists its members:
// the prefix (3) or the exact name (4). A group's name found inside, or as a
// scattering of letters (`e`, `an`), would list whole groups for a letter or
// two, so only an own-name match does that.
const groupNameTier = 3

// undrawableLast is the registry in its own order, except that every object
// Astrolog cannot draw comes after all the others, in registry order too.
func (b Body) undrawableLast() []aspect.RegistryObject {
	out := make([]aspect.RegistryObject, 0, len(b.Registry))
	var gone []aspect.RegistryObject
	for _, o := range b.Registry {
		if _, bad := b.Undrawable(o.Number); bad {
			gone = append(gone, o)
		} else {
			out = append(out, o)
		}
	}
	return append(out, gone...)
}

// Open seeds the selector from the policy in force.
//
// The seed when there is NO override is the lens's own drawn set, not every
// object the registry carries. The aspect chooser seeds all counting aspects
// there, and the specification § Open leaves the question open with an
// instruction not to diverge — but the two cases are not alike in size, and
// following it literally here would be a different behaviour, not the same one.
// Aspects: 24 rows, of which the host has most on. Objects: 134 rows, of which
// the shipped lens draws eighteen. Seeding everything would mean the operator
// opens the selector, unticks one fixed star, confirms, and gets a 133-object
// wheel they never asked for — the first keystroke silently widening the chart
// by two orders of magnitude rather than by a little.
//
// Seeding from what is on screen keeps the analogy where it actually holds: the
// aspect chooser's defect is that it seeds from the REGISTRY instead of from
// what the chart is doing, and this seeds from what the chart is doing. When
// the aspect side is fixed it will move toward this, not away from it.
func Open(registry []aspect.RegistryObject, policy recipe.ObjectPolicy, lensDefault []int) Body {
	st := Body{Registry: registry, Selected: map[int]bool{}, Entry: policy, Probes: map[int]ProbeEntry{}}
	st.Lead = RestoreRow
	if policy.Baseline == recipe.Explicit {
		for _, n := range policy.Selection {
			st.Selected[n] = true
		}
	} else {
		for _, n := range lensDefault {
			st.Selected[n] = true
		}
	}
	st.Refilter()
	return st
}

// policy turns the ticked set into an ObjectPolicy. It is always Explicit, an
// empty set included; the model reads an empty one as no choice and puts the
// default policy in force (chosenObjects).
//
// Nodes is carried over from the entry policy rather than dropped.
// It selects the variants of objects 16, 17 and 18 (mean, true or natural),
// which is orthogonal to which objects are drawn: losing it here would silently
// change the form of the Nodes and Lilith the first time the operator opened
// this modal, in a slice that is not about them at all.
func (b Body) Policy() recipe.ObjectPolicy {
	sel := make([]int, 0, len(b.Selected))
	for n, on := range b.Selected {
		if on {
			sel = append(sel, n)
		}
	}
	sort.Ints(sel)
	return recipe.ObjectPolicy{
		Baseline:  recipe.Explicit,
		Selection: sel,
		Nodes:     b.Entry.Nodes,
	}
}

// toggleAt flips the row under the cursor. Out-of-range is a no-op rather than
// a panic: a filter can empty the list while the cursor still holds a value.
//
// The cursor is combined, as in the aspect chooser: the first GroupRows()
// positions are the group rows, and the list index is the cursor minus them.
func (b *Body) ToggleAt(cursor int) {
	if cursor >= 0 && cursor < GroupRows() {
		b.ToggleGroup(cursor)
		return
	}
	i := cursor - GroupRows()
	if i < 0 || i >= len(b.Matches) {
		return
	}
	n := b.Matches[i].Number
	if b.cannotTick(n) {
		return
	}
	b.Selected[n] = !b.Selected[n]
}

// cannotTick is true for an object with no ephemeris file that is not ticked: it
// can be unticked, never ticked, because a chart asking only for such objects
// would be an empty one (the startup probe, ephemeris.go).
func (b Body) cannotTick(n int) bool {
	_, bad := b.Undrawable(n) // the startup answer or this opening's negative probe
	return bad && !b.Selected[n]
}

// groupRows is how many pinned group rows sit above the list.
func GroupRows() int { return len(recipe.ObjectGroups) }

// RestoreRow is the lead row of the list: Enter on it ticks the lens's default
// bodies.
const RestoreRow = "+ Restore the default bodies"

// PinnedRows is the lines pinned above the list: the lead row, the group rows
// and the rule under them. The lead row and the group rows are cursor rows.
func PinnedRows() int { return 1 + GroupRows() + 1 }

// totalRows is the group block plus the filtered list.
func (b Body) TotalRows() int { return GroupRows() + len(b.Matches) }

// groupMembers is the object numbers of group g that the filter leaves
// visible, less the ones the group never ticks (Earth, in Planets). Like the
// aspect families, a group follows the filter: its count, its state and its
// toggle all act on what the operator can see.
func (b Body) groupMembers(g int) []int {
	var out []int
	for _, o := range b.Matches {
		if recipe.ObjectGroupOf(o.Number) == g && !recipe.ObjectManual(o.Number) {
			out = append(out, o.Number)
		}
	}
	return out
}

// groupTicked is how many of group g's visible members are ticked, and how
// many could be: none, some or all of them is how a group row reads, derived
// every render. A member that cannot be ticked (no ephemeris file, not ticked)
// is left out of both counts: it would otherwise keep the group short of "all"
// for good, and Space on the row would try to tick it again forever (operator,
// 2026-11-04: "space on Others ... does not react").
func (b Body) groupTicked(g int) (on, total int) {
	for _, n := range b.groupMembers(g) {
		if b.cannotTick(n) {
			continue
		}
		total++
		if b.Selected[n] {
			on++
		}
	}
	return on, total
}

// toggleGroup ticks every visible member that can be ticked unless all of them
// are ticked, then unticks them all (Aspects.ToggleFamily, with the members that
// cannot be ticked left out of "all").
func (b *Body) ToggleGroup(g int) {
	members := b.groupMembers(g)
	if len(members) == 0 {
		return
	}
	on, total := b.groupTicked(g)
	want := on != total
	for _, n := range members {
		if want && b.cannotTick(n) {
			continue
		}
		b.Selected[n] = want
	}
}

// RestoreDefaults ticks the lens's default bodies and nothing else (Enter on the
// lead row). It
// is a tick change like any other, so the model previews it and a closing rule
// that puts the entry back undoes it. With no defaults given it does nothing.
func (b *Body) RestoreDefaults() {
	if len(b.Defaults) == 0 {
		return
	}
	b.Selected = make(map[int]bool, len(b.Defaults))
	for _, n := range b.Defaults {
		b.Selected[n] = true
	}
}

// groupBlockRows is how many group rows the box's list area was sized for. The
// block grew to seven (operator, 2026-11-04) and the box grows with it, by the
// rows beyond that (boxTaller), so the list keeps the rows it had at every
// terminal height, 80x24 included.
const groupBlockRows = 5

// boxTaller is the rows the Bodies box adds to the list area every popup gets:
// the group rows beyond that and the lead row.
func boxTaller() int { return max(GroupRows()-groupBlockRows, 0) + 1 }

// ListHeight is the rows the list gets: the box's list area, taller by the
// group rows it was not sized for, minus the pinned group block with its rule
// (the two bottom lines are the message zone, on top of what the terminal gives
// the content, dialog.Box.Zone). Navigation needs the same number drawing does,
// or the cursor lands outside the window that was painted. A long note takes
// more rows while it shows; the drawn window then keeps the cursor on screen by
// itself (WindowAt).
func ListHeight(termH int) int {
	h := dialog.ListHeight(termH) + boxTaller() - PinnedRows()
	if h < 1 {
		h = 1
	}
	return h
}

// Render draws the selector: a flat filtered list, a blank line and the bottom
// line (the count and legend, or the probe's note about a body Astrolog cannot
// draw in their place).
//
// The frame's top edge, border and dimming arrive as chrome, and counter writes
// `N of M` for the cursor and the row total it is given: both belong to the
// panels that share them and stay with the Model's package.
func (state Body) Render(cfg term.Style, termW, termH int, chrome dialog.Chrome, counter func(cursor, total int) *dialog.Counter) string {
	status, _ := bodyProbeStatus(state)
	return dialog.Render(dialog.Box{
		Title:    dialog.TitleBodies,
		Fit:      bodyFit(state, cfg),
		Position: counter(state.Cursor, state.TotalRows()),
		Scroll: func(height int) (pane.Viewport, int) {
			h := height - PinnedRows()
			_, offset := state.WindowAt(state.Cursor-GroupRows(), h)
			return pane.Viewport{Total: len(state.Matches), Visible: h, Offset: offset}, PinnedRows()
		},
		FilterText: state.FilterText,
		Prompt:     "Search bodies: ",
		Zone:       true,
		Taller:     boxTaller(),
		Message:    status,

		MessageLines: 2,
		Foot:         TagBar(state, termW, cfg),
		KeepRows:     PinnedRows() + 1,
	}, cfg, termW, termH, chrome, func(width, height int) string {
		return menu.RenderPinnedBody(menu.PinnedBody{
			Above: append(append([]string{menu.LeadRow(state.Lead, state.OnLead(), cfg, width)}, PinnedGroupRows(state, cfg, width)...), menu.Rule(cfg, width)),
			Rows:  ListRows(state, cfg, width, height-PinnedRows()),
		}, width, height)
	})
}

// bodyFit is the selector's size, decided when it opens for everything it could
// show: wide enough for the legend of the bottom line, the group rows and the widest
// body of the registry, and for a sentence in the message zone (the probe's
// answer arrives after the popup is open, and the cursor moves); as tall as the
// group block, its rule and every body. The zone's rows (the blank line and the
// bottom line) are the box's (dialog.Box.Zone).
func bodyFit(state Body, cfg term.Style) dialog.Fit {
	fit := dialog.Fit{Width: menu.RowWidth(menu.RowSpec{Left: RestoreRow}, cfg), Rows: PinnedRows() + max(len(state.Registry), 1)}
	for i := range recipe.ObjectGroups {
		// The widest the row can get: every member ticked, so the box does not
		// change width as bodies are ticked.
		all := 0
		for _, o := range state.Registry {
			if recipe.ObjectGroupOf(o.Number) == i && !recipe.ObjectManual(o.Number) {
				all++
			}
		}
		fit.Width = max(fit.Width, menu.RowWidth(menu.RowSpec{Left: groupRowText(GroupMarkNone, i, all, all)}, cfg))
	}
	for _, o := range state.Registry {
		fit.Width = max(fit.Width, menu.RowWidth(Row(state, o, false, cfg), cfg))
	}
	// The count is as wide as it can get, so the box does not change width as
	// bodies are ticked.
	fit.Width = max(fit.Width, ansi.StringWidth(bodyTagLegend(state, cfg, len(state.Registry))))
	// The line for the note about a body Astrolog cannot draw is as wide as a
	// sentence: which body, and which files it is missing.
	fit.Width = max(fit.Width, dialog.ProseWidth)
	return fit
}

// ListRows renders the filtered list.
func ListRows(state Body, cfg term.Style, width, height int) []string {
	if height < 1 {
		return nil
	}
	if state.Registry == nil {
		return []string{menu.Note("(Astrolog's list of bodies could not be read)", width, cfg)}
	}
	if len(state.Matches) == 0 {
		return []string{menu.Note("(no body of that name)", width, cfg)}
	}
	// The list's own index is the combined cursor minus the group rows; a
	// negative one means the cursor is up in the group block.
	listCursor := state.Cursor - GroupRows()
	visible, offset := state.WindowAt(listCursor, height)
	rows := make([]string, 0, len(visible))
	for i, o := range visible {
		rows = append(rows, menu.Row(Row(state, o, listCursor >= 0 && offset+i == listCursor, cfg), cfg, width))
	}
	return rows
}

// The three marks a group row draws, and the legend in the tag bar explains.
const (
	GroupMarkAll  = "●"
	GroupMarkSome = "◐"
	GroupMarkNone = "○"
)

// PinnedGroupRows renders the pinned group block: state glyph, name and how many
// of the members the filter leaves visible are ticked, in the aspect families'
// form.
func PinnedGroupRows(state Body, cfg term.Style, width int) []string {
	rows := make([]string, 0, GroupRows())
	for i := range recipe.ObjectGroups {
		rows = append(rows, menu.Row(bodyGroupRow(state, i, state.Cursor == i), cfg, width))
	}
	return rows
}

// bodyGroupRow is one group of the pinned block.
func bodyGroupRow(state Body, i int, cursor bool) menu.RowSpec {
	mark := GroupMarkNone
	on, total := state.groupTicked(i)
	switch {
	case on == 0:
	case on == total:
		mark = GroupMarkAll
	default:
		mark = GroupMarkSome
	}
	return menu.RowSpec{
		Left:   groupRowText(mark, i, on, len(state.groupMembers(i))),
		Cursor: cursor,
	}
}

// groupRowText is a group row: the mark, the name padded to the longest name
// and how many of the members the filter leaves visible are ticked.
func groupRowText(mark string, i, on, members int) string {
	width := 0
	for _, g := range recipe.ObjectGroups {
		width = max(width, len(g.Name))
	}
	return fmt.Sprintf("[%s ] %-*s (%d/%d)", mark, width, recipe.ObjectGroups[i].Name, on, members)
}

// bodyProbeStatus is the bottom line (in place of the count) while the cursor is on a
// body Astrolog cannot draw: what Astrolog itself said about it.
func bodyProbeStatus(state Body) (string, bool) {
	cur, ok := state.ObjectUnderCursor()
	if !ok {
		return "", false
	}
	return ProbeStatusFor(state, cur)
}

// ProbeStatusFor is that line for one body.
func ProbeStatusFor(state Body, o aspect.RegistryObject) (string, bool) {
	e, marked := state.Undrawable(o.Number)
	if !marked {
		return "", false
	}
	if e.Said == "" {
		return fmt.Sprintf("Astrolog cannot place %s here and did not say why", o.Name), true
	}
	return "Astrolog: " + e.Said, true
}

// Row is one object: tick box and the object's full name, on the left, with
// no right cell (the specification C9b).
//
// The form is legacy's, settled in the specification § Taste lane V4 and
// not re-litigated here. Bodies have no angle and no orb, so the aspect
// chooser's four-field row does not transfer and nothing replaces it. The
// registry number is deliberately NOT shown: the short form lives in the tag
// bar and nowhere else.
func Row(state Body, o aspect.RegistryObject, onCursor bool, cfg term.Style) menu.RowSpec {
	mark := " "
	if state.Selected[o.Number] {
		mark = "✓"
	}
	name := o.Name
	_, marked := state.Undrawable(o.Number)
	if marked {
		// Astrolog could not draw it: a `!` before the name (red with colours
		// on, also under the cursor band), the row faint around it.
		name = term.RenderErrorMark("!", cfg) + " " + name
	}
	return menu.RowSpec{
		Left:   fmt.Sprintf("[%s] %s", mark, name),
		Cursor: onCursor,
		Dim:    marked,
	}
}

// TagBar is the bottom line of the box: how many bodies are ticked and what
// the group marks mean. The count is of the whole selection, whatever the
// filter shows, because the list cannot show it (134 rows do not fit and a
// filter hides most of what is ticked). The names of the ticked bodies are not
// listed any more (operator, 2026-11-04). It never wraps; it is cut to width.
func TagBar(state Body, width int, cfg term.Style) string {
	n := 0
	for _, o := range state.Registry {
		if state.Selected[o.Number] {
			n++
		}
	}
	return term.CutRight(bodyTagLegend(state, cfg, n), width, cfg)
}

// bodyTagLegend is the bottom line: the count of ticked bodies and
// what the group marks mean.
func bodyTagLegend(state Body, cfg term.Style, n int) string {
	dot := "·"
	if cfg.UseASCII() {
		dot = "-"
	}
	return fmt.Sprintf("%d of %d selected %s %s all %s some %s none",
		n, len(state.Registry), dot, GroupMarkAll, GroupMarkSome, GroupMarkNone)
}

// Action is one thing the popup does for a key. The Model resolves the key
// through its bindings table and passes the action in, so the popup never
// reads the table.
type Action int

const (
	// Type is any key the table does not name: the filter text, or nothing.
	Type Action = iota
	// Confirm is Enter where Enter applies (the switched-off closing rule).
	Confirm
	// Keep closes with the ticks in force (Enter under the default rule).
	Keep
	// Dismiss is Keep by Esc: with nothing ticked there is nothing to apply.
	Dismiss
	// Discard closes and gives the entry policy back (the switched-off rule).
	Discard
	Up
	Down
	PageUp
	PageDown
	PageHome
	PageEnd
	// Toggle ticks or unticks the row under the cursor.
	Toggle
)

// Outcome is what one key decided. Both Confirmed and Cancelled carry the
// policy the caller should put in force, so the caller never has to know which
// of the two restores and which applies.
type Outcome struct {
	Confirmed bool
	// Dismissed is a Confirmed that came from Esc rather than Enter: the
	// ticks stay in force, and with none made there is nothing to apply.
	Dismissed bool
	Cancelled bool
	Policy    recipe.ObjectPolicy
}

// Apply runs one action. msg is the key that caused it, read only by Type for
// the filter text; listHeight is the rows the list is drawn with.
//
// Nothing here starts an Astrolog process: every branch returns state only, and
// only closing hands a policy back to the caller (C7).
func (b *Body) Apply(a Action, msg tea.KeyMsg, listHeight int) Outcome {
	switch a {
	case Confirm, Keep:
		if b.OnLead() {
			b.RestoreDefaults()
			return Outcome{}
		}
		return Outcome{Confirmed: true, Policy: b.Policy()}
	case Dismiss:
		return Outcome{Confirmed: true, Dismissed: true, Policy: b.Policy()}
	case Discard:
		return Outcome{Cancelled: true, Policy: b.Entry}
	case Up:
		if b.Cursor > b.HomeCursor() {
			b.Cursor--
		}
		b.ScrollTo(b.Cursor-GroupRows(), listHeight)
	case Down:
		if b.Cursor+1 < b.TotalRows() {
			b.Cursor++
		}
		b.ScrollTo(b.Cursor-GroupRows(), listHeight)
	case PageUp:
		b.pageBodies(menu.PageUp, listHeight)
	case PageDown:
		b.pageBodies(menu.PageDown, listHeight)
	case PageHome:
		b.pageBodies(menu.PageHome, listHeight)
	case PageEnd:
		b.pageBodies(menu.PageEnd, listHeight)
	case Toggle:
		b.ToggleAt(b.Cursor)
	default:
		if b.TypeKey(msg) {
			cur := b.Cursor // Refilter resets it
			b.Refilter()
			b.Cursor = menu.CursorAfterTyping(cur, GroupRows(), len(b.Matches))
			b.ScrollTo(b.Cursor-GroupRows(), listHeight)
		}
	}
	return Outcome{}
}

// pageBodies applies a paging key by the one rule, then holds the stored
// window where the render draws it for the cursor's new row.
func (b *Body) pageBodies(op menu.PageOp, listHeight int) {
	b.PageBelow(GroupRows(), op, listHeight)
	_, b.Offset = b.WindowAt(b.Cursor-GroupRows(), listHeight)
}
