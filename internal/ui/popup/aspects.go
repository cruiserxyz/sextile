// Package popup holds the popups that choose what a chart draws: the body
// selector and the aspect chooser. Each holds its state, its rows and its
// rendering, and takes its keys as Actions the Model resolves from its bindings
// table, so it reads neither the table nor the Model.
package popup

import (
	"fmt"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

//
// It binds the shared chassis: menu.FuzzyList holds the search buffer, the cursor
// and the filtered rows, and menu.RenderPinnedBody puts the five family rows in the
// block pinned above the list. The families are the fast path and never scroll;
// the list below them does.
//
// One cursor walks both blocks — rows 0..4 are the families, everything after
// is the filtered list — which is what makes the group rows feel like part of
// the list rather than a separate widget to tab into.

// Aspects is the chooser's state. The list machinery is embedded, so
// FilterText, cursor, matches and truncated all come from the chassis.
type Aspects struct {
	menu.FuzzyList[aspect.RegistryAspect]

	// registry is every aspect the user's binary carries, read once at
	// startup. Nil until it has been read; the modal refuses to open.
	Registry []aspect.RegistryAspect

	// selected holds Astrolog registry indices the operator has ticked.
	//
	// INVARIANT: every key with a true value is an index this registry can
	// actually report — SetTick is the only writer and refuses the rest. The
	// invariant is what lets policy() hand `selected` straight to the
	// compiler without re-filtering it, and it is C14's second half:
	// selecting a zero-orb aspect must not report success, and the only way
	// to be sure of that everywhere is to have one place where a tick can
	// be written at all.
	Selected map[int]bool

	// entry is the policy in force when the modal opened. Only the switched-off
	// rule (chooserCloseKeeps = false) restores it, on Esc; by default every
	// closing key keeps the ticks.
	Entry recipe.AspectPolicy
}

// FamilyRows is how many pinned rows sit above the list.
func FamilyRows() int { return len(aspect.Families()) }

// Refilter recomputes the visible aspects, best match first by
// location.MatchTier over the registry's NAME and ABBREVIATION (the better of
// the two), ties in registry order. The abbreviation is what the aspect
// waterfall prints, so an operator who saw "Ses" there can type it here.
func (a *Aspects) Refilter() {
	if a.Registry == nil {
		a.Clear()
		return
	}
	q := strings.ToLower(strings.TrimSpace(a.FilterText))
	if q == "" {
		a.SetMatches(append([]aspect.RegistryAspect(nil), a.Registry...), false)
		return
	}
	var hits []aspect.RegistryAspect
	var tiers []int
	for _, r := range a.Registry {
		t := max(location.MatchTier(strings.ToLower(r.Name), q), location.MatchTier(strings.ToLower(r.Abbrev), q))
		if t > 0 {
			hits = append(hits, r)
			tiers = append(tiers, t)
		}
	}
	a.SetMatches(menu.RankByTier(hits, tiers), false)
}

// OpenAspects seeds the modal from the set the chart is ACTUALLY counting
// (C3a). Those are two different sources depending on whether the operator has
// overridden anything, and getting the second one wrong is not a display bug:
// the first toggle confirms whatever was seeded as an explicit policy, so a
// seed that overstates the host's set silently widens the chart.
func OpenAspects(registry []aspect.RegistryAspect, policy recipe.AspectPolicy) Aspects {
	st := Aspects{Registry: registry, Selected: map[int]bool{}, Entry: policy}
	if policy.Explicit {
		for _, n := range policy.Indices {
			st.SetTick(n, true)
		}
	} else {
		// No override in force: the host's astrolog.as is what the operator
		// is looking at, and `-HA`'s unrestricted section is where it says
		// so. Seeding from the ORB instead — every aspect with a positive
		// one — reads the wrong fact entirely: a default host counts five of
		// twenty-four and the other nineteen all print positive orbs.
		for _, r := range registry {
			if r.InForce {
				st.SetTick(r.Index, true)
			}
		}
	}
	st.Refilter()
	return st
}

// Selectable reports whether an index can be ticked at all. An aspect whose
// EFFECTIVE orb is zero cannot produce a row however the restriction band is
// set, so a tick on it would report a selection the chart will not honour
// (C14). Restriction and orb are orthogonal switches, and this is the one the
// operator cannot change from here.
func (a Aspects) Selectable(idx int) bool {
	for _, r := range a.Registry {
		if r.Index == idx {
			return r.CountsAnything()
		}
	}
	// An index that is not in this registry at all — a stale explicit policy
	// carried over from a binary that had it — is not Selectable either.
	return false
}

// SetTick is the ONLY writer of `selected`. Every seed and every toggle goes
// through it, which is what makes C14 a property of the type rather than a
// check four callers have to remember. A refused tick is silent: the row
// already renders `-` at every width, which is C14's first half.
func (a *Aspects) SetTick(idx int, on bool) {
	if !a.Selectable(idx) {
		return
	}
	a.Selected[idx] = on
}

// policy turns the ticked set into an AspectPolicy. It is always Explicit —
// the operator confirmed a set, even an empty one.
//
// It does not filter: `selected` cannot contain an unreportable index, because
// SetTick is its only writer.
func (a Aspects) Policy() recipe.AspectPolicy {
	idx := make([]int, 0, len(a.Selected))
	for n, on := range a.Selected {
		if on {
			idx = append(idx, n)
		}
	}
	sort.Ints(idx)
	return recipe.AspectPolicy{Explicit: true, Indices: idx}
}

// FamilyMembers are the registry indices a family row speaks for. Three things
// narrow it, and the row's count, its glyph and its toggle all read this one
// function so they cannot disagree with each other.
//
//   - The angle's harmonic (C5), against the LIVE registry rather than a
//     constant.
//   - Selectability (C14): a zero-orb aspect is left out because the row is a
//     control, and a family holding a member the row cannot tick could never
//     read all-selected — toggling it would fill forever and never empty.
//   - The FILTER. Decided by the operator, 2026-09-14 (§ Taste lane V4): with
//     `squ` typed, a family's count is how many of its members match, "and its
//     tri-state glyph reads against the filtered membership, so an empty group
//     is visibly empty rather than silently stale".
//
// The filter clause is why this reads `matches` and not `registry`. It also
// settles what a filtered toggle DOES: it acts on the members the operator can
// see, because a control whose label describes the visible members and whose
// effect reaches invisible ones is the incoherence the count decision was
// avoiding. With no filter the two sets are identical, which is every ordinary
// use of the row.
func (a Aspects) FamilyMembers(f aspect.Family) []int {
	var out []int
	for _, r := range a.Matches {
		if f.Contains(r.Angle) && r.CountsAnything() {
			out = append(out, r.Index)
		}
	}
	return out
}

// FamilyState is how a family row reads: all of its members ticked, some, or
// none. It is derived every render, never stored, so it cannot go stale.
type FamilyState int

const (
	FamilyNone FamilyState = iota
	FamilyPartial
	FamilyAll
)

func (a Aspects) FamilyState(f aspect.Family) FamilyState {
	members := a.FamilyMembers(f)
	if len(members) == 0 {
		return FamilyNone
	}
	on := 0
	for _, n := range members {
		if a.Selected[n] {
			on++
		}
	}
	switch {
	case on == 0:
		return FamilyNone
	case on == len(members):
		return FamilyAll
	default:
		return FamilyPartial
	}
}

// ToggleFamily selects every member when not all are selected, and deselects
// them all when they are.
func (a *Aspects) ToggleFamily(f aspect.Family) {
	members := a.FamilyMembers(f)
	if len(members) == 0 {
		return
	}
	want := a.FamilyState(f) != FamilyAll
	for _, n := range members {
		a.SetTick(n, want)
	}
}

// ToggleAt flips whatever the combined cursor is on.
func (a *Aspects) ToggleAt(cursor int) {
	fams := aspect.Families()
	if cursor < len(fams) {
		a.ToggleFamily(fams[cursor])
		return
	}
	i := cursor - len(fams)
	if i < 0 || i >= len(a.Matches) {
		return
	}
	n := a.Matches[i].Index
	a.SetTick(n, !a.Selected[n])
}

// AspectPinnedRows is how many lines sit pinned above the list: the family rows
// and the rule under them (the specification 3, 4). Only the family
// rows are cursor rows; the rule is a line, so it is not in TotalRows.
func AspectPinnedRows() int { return FamilyRows() + 1 }

// AspectListHeight is how many rows the LIST gets: the box's list area minus
// the pinned family block and its rule, which never scroll.
func AspectListHeight(termH int) int {
	h := dialog.ListHeight(termH) - AspectPinnedRows()
	if h < 1 {
		h = 1
	}
	return h
}

// TotalRows is the family block plus the filtered list.
func (a Aspects) TotalRows() int { return FamilyRows() + len(a.Matches) }

// Render draws the chooser: the family block pinned above a filtered list of
// every aspect the binary carries. The frame's top edge, border and dimming
// arrive as chrome, and counter writes `N of M` for the cursor and the row
// total it is given.
func (state Aspects) Render(cfg term.Style, termW, termH int, chrome dialog.Chrome, counter func(cursor, total int) *dialog.Counter) string {
	// One cursor walks the pinned families and the filtered aspects, so
	// both count as positions; the families never scroll, so the track is
	// the filtered block below them.
	return dialog.Render(dialog.Box{
		Title:    dialog.TitleAspects,
		Fit:      aspectFit(state, cfg),
		Position: counter(state.Cursor, FamilyRows()+len(state.Matches)),
		Scroll: func(height int) (pane.Viewport, int) {
			h := height - AspectPinnedRows()
			_, offset := state.WindowAt(state.Cursor-FamilyRows(), h)
			return pane.Viewport{Total: len(state.Matches), Visible: h, Offset: offset}, AspectPinnedRows()
		},
		FilterText: state.FilterText,
	}, cfg, termW, termH, chrome, func(width, height int) string {
		return menu.RenderPinnedBody(menu.PinnedBody{
			Above: append(AspectFamilyRows(state, cfg, width), menu.Rule(cfg, width)),
			Rows:  AspectListRows(state, cfg, width, height-AspectPinnedRows()),
		}, width, height)
	})
}

// aspectFit is the chooser's size, decided when it opens for everything it
// could show: wide enough for the widest aspect row of the registry, orb and
// all; as tall as the family block, its rule and every aspect. A filter leaves
// rows blank; it does not resize the box.
func aspectFit(state Aspects, cfg term.Style) dialog.Fit {
	fit := dialog.Fit{Width: 1, Rows: AspectPinnedRows() + max(len(state.Registry), 1)}
	for _, f := range aspect.Families() {
		fit.Width = max(fit.Width, menu.RowWidth(aspectFamilyRow(state, f, false), cfg))
	}
	fit.Width = max(fit.Width, aspectFullWidth(state, cfg))
	return fit
}

// aspectFullWidth is the width at which every aspect row keeps its orb: the
// widest row of the registry with all three right cells. Below it the orb is
// left off of every row, so the name keeps its room and the columns stay one
// column.
func aspectFullWidth(state Aspects, cfg term.Style) int {
	w := 0
	for _, r := range state.Registry {
		w = max(w, menu.RowWidth(aspectRowWith(state, r, false, true), cfg))
	}
	return w
}

// AspectFamilyRows renders the pinned block. The count and the glyph both come
// from FamilyMembers, which already follows the filter — deriving the count
// here separately is how they came to disagree, showing `(1)` beside `◐` for a
// family whose one visible member was selected.
//
// The family rows and the list are one column to the cursor, so they are one
// column to the stripe too: a row's stripe index is its position in that walk.
func AspectFamilyRows(state Aspects, cfg term.Style, width int) []string {
	rows := make([]string, 0, FamilyRows())
	for i, f := range aspect.Families() {
		rows = append(rows, menu.Row(aspectFamilyRow(state, f, state.Cursor == i), cfg, width))
	}
	return rows
}

// aspectFamilyRow is one family of the pinned block.
func aspectFamilyRow(state Aspects, f aspect.Family, cursor bool) menu.RowSpec {
	mark := "○"
	switch state.FamilyState(f) {
	case FamilyAll:
		mark = "●"
	case FamilyPartial:
		mark = "◐"
	}
	return menu.RowSpec{
		Left:   fmt.Sprintf("[%s ] %-10s (%d)", mark, f.Name(), len(state.FamilyMembers(f))),
		Cursor: cursor,
	}
}

// AspectListRows renders the filtered list.
//
// Drop order (the specification): the orb goes first, then the
// name truncates. The marker, the abbreviation and the angle never drop — the
// abbreviation because it is the bridge to the aspect waterfall, and the angle
// because among the thirteen harmonics the enum cannot name it is the only
// thing a practitioner can scan.
func AspectListRows(state Aspects, cfg term.Style, width, height int) []string {
	if height < 1 {
		return nil
	}
	if state.Registry == nil {
		return []string{menu.Note("(no aspect registry)", width, cfg)}
	}
	if len(state.Matches) == 0 {
		return []string{menu.Note("(no matching aspects)", width, cfg)}
	}

	// The cursor walks the family block and the list as one column, so the
	// list's own index is the combined cursor minus the pinned rows. A
	// negative value means the cursor is still up in the family block, which
	// is a perfectly good reason to show the list from the top.
	base := FamilyRows()
	listCursor := state.Cursor - base
	visible, offset := state.WindowAt(listCursor, height)
	rows := make([]string, 0, len(visible))
	withOrb := width >= aspectFullWidth(state, cfg)
	for i, r := range visible {
		onCursor := listCursor >= 0 && offset+i == listCursor
		rows = append(rows, menu.Row(aspectRowWith(state, r, onCursor, withOrb), cfg, width))
	}
	return rows
}

// AspectRow is one aspect: the tick box and name on the left, and the
// abbreviation, the angle and the orb pinned to the right (the specification
// C9a). The row is pinned: only the name ever shortens (C4b).
//
// The marker carries three states, not two: an aspect whose EFFECTIVE orb is
// zero cannot produce a row however it is restricted, so it reads as
// unavailable at every width — including the widths where the orb column itself
// has been dropped, which is exactly where the operator would otherwise have no
// way to tell (C14).
func AspectRow(state Aspects, cfg term.Style, r aspect.RegistryAspect, onCursor bool, width int) menu.RowSpec {
	return aspectRowWith(state, r, onCursor, width >= aspectFullWidth(state, cfg))
}

// aspectRowWith is AspectRow once the list has decided whether its rows keep
// the orb: the decision is the list's, so every row of it has the same columns.
func aspectRowWith(state Aspects, r aspect.RegistryAspect, onCursor, withOrb bool) menu.RowSpec {
	mark := " "
	switch {
	case !r.CountsAnything():
		mark = "-"
	case state.Selected[r.Index]:
		mark = "✓"
	}
	right := []string{fmt.Sprintf("%-3s", r.Abbrev), fmt.Sprintf("%6.2f°", r.Angle)}
	if withOrb {
		right = append(right, fmt.Sprintf("orb %4.1f", r.Orb))
	}
	return menu.RowSpec{
		Left:   fmt.Sprintf("[%s] %s", mark, r.Name),
		Right:  right,
		Cursor: onCursor,
		Pinned: true,
	}
}

// AspectOutcome is what one keystroke decided. Both Confirmed and
// Cancelled carry the policy the caller should put in force, so the caller
// never has to know which of the two restores and which applies.
type AspectOutcome struct {
	Confirmed bool
	// Dismissed is a Confirmed that came from Esc rather than Enter: the
	// ticks stay in force, and with none made there is nothing to apply.
	Dismissed bool
	Cancelled bool
	Policy    recipe.AspectPolicy
}

// Apply runs one action on the chooser. msg is the key that caused it, read only
// by Type for the filter text; listHeight is the rows the list is drawn with.
// The actions are the bodies popup's, and mean here what they mean there.
//
// Nothing here starts an Astrolog process: every branch returns a nil Cmd, and
// only closing hands a policy back to the caller (C9). Enter and Esc both
// close and keep the ticks; the switched-off closing rule makes Esc cancel.
func (a *Aspects) Apply(act Action, msg tea.KeyMsg, listHeight int) AspectOutcome {
	switch act {
	case Confirm, Keep:
		return AspectOutcome{Confirmed: true, Policy: a.Policy()}
	case Dismiss:
		return AspectOutcome{Confirmed: true, Dismissed: true, Policy: a.Policy()}
	case Discard:
		return AspectOutcome{Cancelled: true, Policy: a.Entry}
	case Up:
		if a.Cursor > 0 {
			a.Cursor--
		}
		a.ScrollTo(a.Cursor-FamilyRows(), listHeight)
	case Down:
		if a.Cursor+1 < a.TotalRows() {
			a.Cursor++
		}
		a.ScrollTo(a.Cursor-FamilyRows(), listHeight)
	case PageUp:
		a.PageBelow(FamilyRows(), menu.PageUp, listHeight)
	case PageDown:
		a.PageBelow(FamilyRows(), menu.PageDown, listHeight)
	case PageHome:
		a.PageBelow(FamilyRows(), menu.PageHome, listHeight)
	case PageEnd:
		a.PageBelow(FamilyRows(), menu.PageEnd, listHeight)
	case Toggle:
		a.ToggleAt(a.Cursor)
	default:
		if a.TypeKey(msg) {
			cur := a.Cursor // Refilter resets it
			a.Refilter()
			a.Cursor = menu.CursorAfterTyping(cur, FamilyRows(), len(a.Matches))
			a.ScrollTo(a.Cursor-FamilyRows(), listHeight)
		}
	}
	return AspectOutcome{}
}
