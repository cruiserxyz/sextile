package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// aspect_modal.go is the chooser for which aspects a chart counts.
//
// It binds the shared chassis: fuzzyList holds the search buffer, the cursor
// and the filtered rows, and renderPinnedBody puts the five family rows in the
// block pinned above the list. The families are the fast path and never scroll;
// the list below them does.
//
// One cursor walks both blocks — rows 0..4 are the families, everything after
// is the filtered list — which is what makes the group rows feel like part of
// the list rather than a separate widget to tab into.

// aspectModalState is the chooser's state. The list machinery is embedded, so
// filterText, cursor, matches and truncated all come from the chassis.
type aspectModalState struct {
	fuzzyList[aspect.RegistryAspect]

	// registry is every aspect the user's binary carries, read once at
	// startup. Nil until it has been read; the modal refuses to open.
	registry []aspect.RegistryAspect

	// selected holds Astrolog registry indices the operator has ticked.
	//
	// INVARIANT: every key with a true value is an index this registry can
	// actually report — setTick is the only writer and refuses the rest. The
	// invariant is what lets policy() hand `selected` straight to the
	// compiler without re-filtering it, and it is C14's second half:
	// selecting a zero-orb aspect must not report success, and the only way
	// to be sure of that everywhere is to have one place where a tick can
	// be written at all.
	selected map[int]bool

	// entry is the policy in force when the modal opened. Only the switched-off
	// rule (chooserCloseKeeps = false) restores it, on Esc; by default every
	// closing key keeps the ticks.
	entry recipe.AspectPolicy
}

// familyRows is how many pinned rows sit above the list.
func familyRows() int { return len(aspect.Families()) }

// refilter recomputes the visible aspects, best match first by
// location.MatchTier over the registry's NAME and ABBREVIATION (the better of
// the two), ties in registry order. The abbreviation is what the aspect
// waterfall prints, so an operator who saw "Ses" there can type it here.
func (a *aspectModalState) refilter() {
	if a.registry == nil {
		a.clear()
		return
	}
	q := strings.ToLower(strings.TrimSpace(a.filterText))
	if q == "" {
		a.setMatches(append([]aspect.RegistryAspect(nil), a.registry...), false)
		return
	}
	var hits []aspect.RegistryAspect
	var tiers []int
	for _, r := range a.registry {
		t := max(location.MatchTier(strings.ToLower(r.Name), q), location.MatchTier(strings.ToLower(r.Abbrev), q))
		if t > 0 {
			hits = append(hits, r)
			tiers = append(tiers, t)
		}
	}
	a.setMatches(rankByTier(hits, tiers), false)
}

// openAspectModal seeds the modal from the set the chart is ACTUALLY counting
// (C3a). Those are two different sources depending on whether the operator has
// overridden anything, and getting the second one wrong is not a display bug:
// the first toggle confirms whatever was seeded as an explicit policy, so a
// seed that overstates the host's set silently widens the chart.
func openAspectModal(registry []aspect.RegistryAspect, policy recipe.AspectPolicy) aspectModalState {
	st := aspectModalState{registry: registry, selected: map[int]bool{}, entry: policy}
	if policy.Explicit {
		for _, n := range policy.Indices {
			st.setTick(n, true)
		}
	} else {
		// No override in force: the host's astrolog.as is what the operator
		// is looking at, and `-HA`'s unrestricted section is where it says
		// so. Seeding from the ORB instead — every aspect with a positive
		// one — reads the wrong fact entirely: a default host counts five of
		// twenty-four and the other nineteen all print positive orbs.
		for _, r := range registry {
			if r.InForce {
				st.setTick(r.Index, true)
			}
		}
	}
	st.refilter()
	return st
}

// selectable reports whether an index can be ticked at all. An aspect whose
// EFFECTIVE orb is zero cannot produce a row however the restriction band is
// set, so a tick on it would report a selection the chart will not honour
// (C14). Restriction and orb are orthogonal switches, and this is the one the
// operator cannot change from here.
func (a aspectModalState) selectable(idx int) bool {
	for _, r := range a.registry {
		if r.Index == idx {
			return r.CountsAnything()
		}
	}
	// An index that is not in this registry at all — a stale explicit policy
	// carried over from a binary that had it — is not selectable either.
	return false
}

// setTick is the ONLY writer of `selected`. Every seed and every toggle goes
// through it, which is what makes C14 a property of the type rather than a
// check four callers have to remember. A refused tick is silent: the row
// already renders `-` at every width, which is C14's first half.
func (a *aspectModalState) setTick(idx int, on bool) {
	if !a.selectable(idx) {
		return
	}
	a.selected[idx] = on
}

// policy turns the ticked set into an AspectPolicy. It is always Explicit —
// the operator confirmed a set, even an empty one.
//
// It does not filter: `selected` cannot contain an unreportable index, because
// setTick is its only writer.
func (a aspectModalState) policy() recipe.AspectPolicy {
	idx := make([]int, 0, len(a.selected))
	for n, on := range a.selected {
		if on {
			idx = append(idx, n)
		}
	}
	sort.Ints(idx)
	return recipe.AspectPolicy{Explicit: true, Indices: idx}
}

// familyMembers are the registry indices a family row speaks for. Three things
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
func (a aspectModalState) familyMembers(f aspect.Family) []int {
	var out []int
	for _, r := range a.matches {
		if f.Contains(r.Angle) && r.CountsAnything() {
			out = append(out, r.Index)
		}
	}
	return out
}

// familyState is how a family row reads: all of its members ticked, some, or
// none. It is derived every render, never stored, so it cannot go stale.
type familyState int

const (
	familyNone familyState = iota
	familyPartial
	familyAll
)

func (a aspectModalState) familyState(f aspect.Family) familyState {
	members := a.familyMembers(f)
	if len(members) == 0 {
		return familyNone
	}
	on := 0
	for _, n := range members {
		if a.selected[n] {
			on++
		}
	}
	switch {
	case on == 0:
		return familyNone
	case on == len(members):
		return familyAll
	default:
		return familyPartial
	}
}

// toggleFamily selects every member when not all are selected, and deselects
// them all when they are.
func (a *aspectModalState) toggleFamily(f aspect.Family) {
	members := a.familyMembers(f)
	if len(members) == 0 {
		return
	}
	want := a.familyState(f) != familyAll
	for _, n := range members {
		a.setTick(n, want)
	}
}

// toggleAt flips whatever the combined cursor is on.
func (a *aspectModalState) toggleAt(cursor int) {
	fams := aspect.Families()
	if cursor < len(fams) {
		a.toggleFamily(fams[cursor])
		return
	}
	i := cursor - len(fams)
	if i < 0 || i >= len(a.matches) {
		return
	}
	n := a.matches[i].Index
	a.setTick(n, !a.selected[n])
}

// aspectPinnedRows is how many lines sit pinned above the list: the family rows
// and the rule under them (the specification 3, 4). Only the family
// rows are cursor rows; the rule is a line, so it is not in totalRows.
func aspectPinnedRows() int { return familyRows() + 1 }

// aspectListHeight is how many rows the LIST gets: the box's list area minus
// the pinned family block and its rule, which never scroll.
func aspectListHeight(termH int) int {
	h := modalListHeight(termH) - aspectPinnedRows()
	if h < 1 {
		h = 1
	}
	return h
}

// totalRows is the family block plus the filtered list.
func (a aspectModalState) totalRows() int { return familyRows() + len(a.matches) }

// renderAspectModal draws the chooser: the family block pinned above a filtered
// list of every aspect the binary carries.
func renderAspectModal(state aspectModalState, cfg Config, termW, termH int) string {
	// One cursor walks the pinned families and the filtered aspects, so
	// both count as positions; the families never scroll, so the track is
	// the filtered block below them.
	pos := listPosition{Cursor: state.cursor, Total: familyRows() + len(state.matches)}
	return renderModalBox(modalBox{
		Title:    popupTitleAspects,
		Position: &pos,
		Scroll: func(height int) (viewport, int) {
			h := height - aspectPinnedRows()
			_, offset := state.windowAt(state.cursor-familyRows(), h)
			return viewport{Total: len(state.matches), Visible: h, Offset: offset}, aspectPinnedRows()
		},
		FilterText: state.filterText,
	}, cfg, termW, termH, func(width, height int) string {
		return renderPinnedBody(pinnedBody{
			Above: append(aspectFamilyRows(state, cfg, width), listRule(cfg, width)),
			Rows:  aspectListRows(state, cfg, width, height-aspectPinnedRows()),
		}, width, height)
	})
}

// aspectFamilyRows renders the pinned block. The count and the glyph both come
// from familyMembers, which already follows the filter — deriving the count
// here separately is how they came to disagree, showing `(1)` beside `◐` for a
// family whose one visible member was selected.
//
// The family rows and the list are one column to the cursor, so they are one
// column to the stripe too: a row's stripe index is its position in that walk.
func aspectFamilyRows(state aspectModalState, cfg Config, width int) []string {
	rows := make([]string, 0, familyRows())
	for i, f := range aspect.Families() {
		mark := "○"
		switch state.familyState(f) {
		case familyAll:
			mark = "●"
		case familyPartial:
			mark = "◐"
		}
		n := len(state.familyMembers(f))
		rows = append(rows, listRow(listRowSpec{
			Left:   fmt.Sprintf("[%s] %-10s (%d)", mark, f.Name(), n),
			Cursor: state.cursor == i,
		}, cfg, width))
	}
	return rows
}

// aspectListRows renders the filtered list.
//
// Drop order (the specification): the orb goes first, then the
// name truncates. The marker, the abbreviation and the angle never drop — the
// abbreviation because it is the bridge to the aspect waterfall, and the angle
// because among the thirteen harmonics the enum cannot name it is the only
// thing a practitioner can scan.
func aspectListRows(state aspectModalState, cfg Config, width, height int) []string {
	if height < 1 {
		return nil
	}
	if state.registry == nil {
		return []string{listNote("(no aspect registry)", width, cfg)}
	}
	if len(state.matches) == 0 {
		return []string{listNote("(no matching aspects)", width, cfg)}
	}

	// The cursor walks the family block and the list as one column, so the
	// list's own index is the combined cursor minus the pinned rows. A
	// negative value means the cursor is still up in the family block, which
	// is a perfectly good reason to show the list from the top.
	base := familyRows()
	listCursor := state.cursor - base
	visible, offset := state.windowAt(listCursor, height)
	rows := make([]string, 0, len(visible))
	for i, r := range visible {
		onCursor := listCursor >= 0 && offset+i == listCursor
		rows = append(rows, listRow(aspectRow(state, r, onCursor, width), cfg, width))
	}
	return rows
}

// aspectRow is one aspect: the tick box and name on the left, and the
// abbreviation, the angle and the orb pinned to the right (the specification
// C9a). The row is pinned: only the name ever shortens (C4b).
//
// The marker carries three states, not two: an aspect whose EFFECTIVE orb is
// zero cannot produce a row however it is restricted, so it reads as
// unavailable at every width — including the widths where the orb column itself
// has been dropped, which is exactly where the operator would otherwise have no
// way to tell (C14).
func aspectRow(state aspectModalState, r aspect.RegistryAspect, onCursor bool, width int) listRowSpec {
	mark := " "
	switch {
	case !r.CountsAnything():
		mark = "-"
	case state.selected[r.Index]:
		mark = "✓"
	}
	right := []string{fmt.Sprintf("%-3s", r.Abbrev), fmt.Sprintf("%6.2f°", r.Angle)}
	if width >= aspectRowFullWidth {
		right = append(right, fmt.Sprintf("orb %4.1f", r.Orb))
	}
	return listRowSpec{
		Left:   fmt.Sprintf("[%s] %s", mark, r.Name),
		Right:  right,
		Cursor: onCursor,
		Pinned: true,
	}
}

// aspectRowFullWidth is the width at which the orb still fits beside the
// widest name in Astrolog's registry (14 cells): the cursor column, the tick
// box, the name, and the three right cells with their gaps. Below it the orb
// is left off, so the name keeps its room. Every cell is fixed-width by its
// format, which is why this is arithmetic and not a measurement.
const aspectRowFullWidth = 2 + 4 + 14 + listRowGap + 3 + listRowGap + 7 + listRowGap + 8

// updateAspectModalKey handles one keystroke. The caller in model.go has
// already gated on the modal being open.
//
// Nothing here starts an Astrolog process: every branch returns a nil Cmd, and
// only closing hands a policy back to the caller (C9). Enter and Esc both
// close and keep the ticks; chooserCloseKeeps = false makes Esc cancel again.
func updateAspectModalKey(state aspectModalState, msg tea.KeyMsg, listHeight int) (aspectModalState, aspectModalOutcome) {
	switch matchBinding(msg, aspectsBindings()) {
	case BindingConfirm:
		return state, aspectModalOutcome{Confirmed: true, Policy: state.policy()}
	case BindingClose:
		if chooserCloseKeeps {
			return state, aspectModalOutcome{Confirmed: true, Dismissed: msg.Type == tea.KeyEsc, Policy: state.policy()}
		}
		return state, aspectModalOutcome{Cancelled: true, Policy: state.entry}
	case BindingListUp:
		if state.cursor > 0 {
			state.cursor--
		}
		state.scrollTo(state.cursor-familyRows(), listHeight)
	case BindingListDown:
		if state.cursor+1 < state.totalRows() {
			state.cursor++
		}
		state.scrollTo(state.cursor-familyRows(), listHeight)
	case BindingPageUp:
		state.pageBelow(familyRows(), pageUp, listHeight)
	case BindingPageDown:
		state.pageBelow(familyRows(), pageDown, listHeight)
	case BindingPageHome:
		state.pageBelow(familyRows(), pageHome, listHeight)
	case BindingPageEnd:
		state.pageBelow(familyRows(), pageEnd, listHeight)
	case BindingToggle:
		state.toggleAt(state.cursor)
	default:
		if state.typeKey(msg) {
			cur := state.cursor // refilter resets it
			state.refilter()
			state.cursor = cursorAfterTyping(cur, familyRows(), len(state.matches))
			state.scrollTo(state.cursor-familyRows(), listHeight)
		}
	}
	return state, aspectModalOutcome{}
}

// aspectModalOutcome is what one keystroke decided. Both Confirmed and
// Cancelled carry the policy the caller should put in force, so the caller
// never has to know which of the two restores and which applies.
type aspectModalOutcome struct {
	Confirmed bool
	// Dismissed is a Confirmed that came from Esc rather than Enter: the
	// ticks stay in force, and with none made there is nothing to apply.
	Dismissed bool
	Cancelled bool
	Policy    recipe.AspectPolicy
}

// cursorAfterTyping is where a popup's combined cursor goes when its filter
// text changes: on the first matching item, below the pinned group rows, so
// Space ticks what was typed for (the specification).
// With nothing matching it stays where it was, clamped.
func cursorAfterTyping(cur, pinned, matches int) int {
	if matches > 0 {
		return pinned
	}
	return clampCursor(cur, pinned+matches)
}

// clampCursor keeps the combined cursor inside the rows that exist. The family
// block never shrinks, so a filter that empties the list parks the cursor on
// the last family row rather than off the end.
func clampCursor(cur, total int) int {
	if total < 1 {
		return 0
	}
	if cur >= total {
		return total - 1
	}
	if cur < 0 {
		return 0
	}
	return cur
}

// applyAspectModalKey is the Model-level half of the chooser: it runs one
// keystroke through the modal and, when the operator finishes, puts the
// resulting policy in force and recomputes.
//
// Nothing here starts a process except the confirm branch — opening, filtering,
// moving and toggling all return a nil Cmd (C9).
func applyAspectModalKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// SNAPSHOT, not a struct copy. aspectModalState holds `selected` as a map,
	// so a copied struct shares that map with the original and any comparison
	// afterwards reports "unchanged" no matter what the keystroke did.
	beforeTicks := ticksOn(m.aspectModal.selected)
	next, out := updateAspectModalKey(m.aspectModal, msg, aspectListHeight(m.height))
	m.aspectModal = next

	switch {
	case out.Cancelled:
		m.startup = startupReady
		restored := !sameSelection(m.lens.Aspects, out.Policy)
		m.lens.Aspects = out.Policy
		if !restored {
			return m, nil
		}
		// Reached only with chooserCloseKeeps = false. A preview is on screen,
		// or on its way. The frame from when the chooser opened comes back at
		// once and the preview is fenced, so the selection the operator just
		// cancelled neither stays in front of them nor lands a moment later
		// (C7a).
		return m.restoreChooserEntry()
	case out.Confirmed:
		m.startup = startupReady
		if out.Dismissed && sameSelection(m.lens.Aspects, m.chooserEntry.lens.Aspects) {
			// Esc with nothing ticked changes no lens and starts no work
			// (C9): the untouched policy is not rewritten into an explicit
			// copy of the host's set. Enter still does that.
			return m, nil
		}
		m.lens.Aspects = out.Policy
		// bumpIdentity (not noteWant): a new aspect set changes WHAT the surface
		// computes, so an in-flight old-set result must never paint. The
		// timeline's searches follow the committed lens too (confirmChooser).
		return m.confirmChooser()
	}

	// Live preview. A toggle repaints the panels BEHIND the modal, so the
	// operator judges the set they are building against the chart rather than
	// against their memory of it.
	//
	// noteWant with leading=false, not bumpIdentity: this is a burst, not an
	// identity change. The trailing 50ms debounce coalesces a run of toggles
	// into ONE compute, so holding Space costs one Astrolog process rather
	// than one per keystroke — which is why this is affordable at all, and why
	// legacy's per-keystroke preview was not the shape to copy.
	//
	// Only the text surface previews. The wheel is an external SVG handed to a
	// browser preview; repainting that under a modal that covers it would spend
	// a lot to show nobody anything.
	// The trigger is a changed SELECTION between before and after this key —
	// not a difference from the policy in force. Those are not the same thing:
	// opening with no override in force seeds every counting aspect as ticked,
	// so the modal's set differs from the in-force policy from the first frame,
	// and comparing against it would preview on every keystroke including plain
	// typing.
	if !sameTickSets(beforeTicks, ticksOn(next.selected)) {
		m.lens.Aspects = next.policy()
		return m.noteWant(surfaceText, false)
	}
	return m, nil
}

// ticksOn snapshots which indices are selected. The selection map carries
// explicit false entries for anything ever deselected, so it is the ON set that
// identifies a selection, not the map.
func ticksOn(m map[int]bool) map[int]struct{} {
	out := make(map[int]struct{}, len(m))
	for k, v := range m {
		if v {
			out[k] = struct{}{}
		}
	}
	return out
}

func sameTickSets(x, y map[int]struct{}) bool {
	if len(x) != len(y) {
		return false
	}
	for k := range x {
		if _, ok := y[k]; !ok {
			return false
		}
	}
	return true
}

// sameSelection compares two policies by what they would compute: the explicit
// flag and the normalised index set. Comparing the structs directly would call
// two policies different over index ORDER, which the compiler sorts away.
func sameSelection(a, b recipe.AspectPolicy) bool {
	if a.Explicit != b.Explicit {
		return false
	}
	x, y := a.Selected(), b.Selected()
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// openAspectChooser is the ⌃a handler. It refuses to open when the registry
// could not be read, and the frame keeps computing with whatever was already
// in force (C10). The failed read was logged once, when it landed; the key
// itself says nothing (the specification Part B). Offering a list Sextile
// invented would be worse than not opening: the operator would pick from a set
// their binary does not have.
func openAspectChooser(m Model) (tea.Model, tea.Cmd) {
	// `startup` does double duty in this model: it names the startup phase
	// AND which modal is open. So a modal opened before the first frame
	// commits is silently overwritten the moment it does. Rather than arm
	// something that will be clobbered, the key is inert until then.
	if m.startup != startupReady {
		return m, nil
	}
	if m.aspectRegistry == nil {
		// A read that FAILED is a refusal with a cause (C10): the chooser
		// will not offer a set Sextile invented.
		if m.aspectRegErr != nil {
			return m, nil
		}
		// A read that has not LANDED yet is not a refusal, and must not be
		// treated as one. The startup read is batched with everything else
		// and can arrive seconds in; telling the operator to "try again in
		// a moment" makes a hotkey that silently depends on how long they
		// waited, which is indistinguishable from a broken key. Instead:
		// take the request and open when the read lands.
		//
		// It does NOT start a second read. Init fires one unconditionally,
		// so a nil registry with no error means exactly one thing — the
		// startup read is still in flight — and a duplicate would spend a
		// second Astrolog process to learn the same answer. Worse, the two
		// results race: if the duplicate failed first it would clear this
		// pending flag, and the original's success would then arrive with
		// nothing to open (C4's once-only read).
		m.aspectOpenPending = true
		return m, nil
	}
	m.aspectModal = openAspectModal(m.aspectRegistry, m.lens.Aspects)
	m.startup = startupAspectOpen
	m.chooserEntry = m.captureChooserEntry()
	return m, nil
}

// applyAspectRegistryLoaded records the startup read. A failure is kept rather
// than discarded so the refusal can name it.
func applyAspectRegistryLoaded(m Model, msg AspectRegistryLoadedMsg) (Model, tea.Cmd) {
	pending := m.aspectOpenPending
	m.aspectOpenPending = false

	if msg.Err != nil {
		m.aspectRegErr = msg.Err
		return m.logNote("aspects", "registry unreadable (%v)", msg.Err), nil
	}
	m.aspectRegistry = msg.Registry

	// Someone pressed the key before the read landed. Honour it now rather
	// than making them press again — the whole point of taking the request
	// was that the key should not depend on timing.
	// Only open on arrival if the app is still in a state that can hold a
	// modal. The same double duty as above: opening mid-startup would be
	// overwritten by the frame commit.
	if pending && m.startup == startupReady {
		m.aspectModal = openAspectModal(m.aspectRegistry, m.lens.Aspects)
		m.startup = startupAspectOpen
		m.chooserEntry = m.captureChooserEntry()
	}
	return m, nil
}
