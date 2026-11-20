package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/popup"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

// aspect_modal_model.go is the Model's half of the aspect chooser (the popup
// itself is internal/ui/popup/aspects.go): the key-to-action mapping, the
// handlers that open it and the registry read.

// aspectsBindings is the Aspects popup's table.
func aspectsBindings() []keys.BindingEntry {
	return keys.TitledLocal(append(append([]keys.BindingEntry{listUp().ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 1 }), listDown(), toggle().ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 0 })}, chooserCloseBindings()...), pageEntries()...))
}

// updateAspectModalKey resolves one keystroke through the aspects bindings table
// to the popup's Action and applies it. The popup never reads the table.
func updateAspectModalKey(state popup.Aspects, msg tea.KeyMsg, listHeight int) (popup.Aspects, popup.AspectOutcome) {
	a := popup.Type
	switch keys.MatchBinding(msg, aspectsBindings()) {
	case keys.BindingConfirm:
		a = popup.Confirm
	case keys.BindingClose:
		switch {
		case !chooserCloseKeeps:
			a = popup.Discard
		case msg.Type == tea.KeyEsc:
			a = popup.Dismiss
		default:
			a = popup.Keep
		}
	case keys.BindingListUp:
		a = popup.Up
	case keys.BindingListDown:
		a = popup.Down
	case keys.BindingPageUp:
		a = popup.PageUp
	case keys.BindingPageDown:
		a = popup.PageDown
	case keys.BindingPageHome:
		a = popup.PageHome
	case keys.BindingPageEnd:
		a = popup.PageEnd
	case keys.BindingToggle:
		a = popup.Toggle
	}
	out := state.Apply(a, msg, listHeight)
	return state, out
}

// renderAspectModal draws the chooser with the chrome and the position counter
// the popups share.
func renderAspectModal(state popup.Aspects, cfg Config, termW, termH int) string {
	return state.Render(cfg, termW, termH, popupChrome(cfg), func(cursor, total int) *dialog.Counter {
		pos := listPosition{Cursor: cursor, Total: total}
		return popupCounter(&pos, cfg)
	})
}

// applyAspectModalKey is the Model-level half of the chooser: it runs one
// keystroke through the modal and, when the operator finishes, puts the
// resulting policy in force and recomputes.
//
// Nothing here starts a process except the confirm branch — opening, filtering,
// moving and toggling all return a nil Cmd (C9).
func applyAspectModalKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// SNAPSHOT, not a struct copy. popup.Aspects holds `selected` as a map,
	// so a copied struct shares that map with the original and any comparison
	// afterwards reports "unchanged" no matter what the keystroke did.
	beforeTicks := ticksOn(m.aspectModal.Selected)
	next, out := updateAspectModalKey(m.aspectModal, msg, popup.AspectListHeight(m.height))
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
	if !sameTickSets(beforeTicks, ticksOn(next.Selected)) {
		m.lens.Aspects = next.Policy()
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
	// Not before the first frame (a modal opened then is overwritten by its
	// commit), and not after a failed read (C10): blocked says so.
	if m.blocked(keys.BindingAspects) != "" {
		return m, nil
	}
	if m.aspectRegistry == nil {
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
	m.aspectModal = popup.OpenAspects(m.aspectRegistry, m.lens.Aspects)
	m.startup = startupAspectOpen
	m.chooserEntry = m.captureChooserEntry()
	return m, nil
}

// applyAspectRegistryLoaded records the startup read. A failure is kept rather
// than discarded so the refusal can name it.
func applyAspectRegistryLoaded(m Model, msg AspectRegistryLoadedMsg) (Model, tea.Cmd) {
	pending := m.aspectOpenPending
	m.aspectOpenPending = false
	awaiting := m.awaitingAspects
	m.awaitingAspects = false

	if msg.Err != nil {
		m.aspectRegErr = msg.Err
		m.cantStart.aspectList = oneLine(msg.Err.Error())
		return m.logTrace("aspects", "registry unreadable (%v)", msg.Err), nil
	}
	m.aspectRegistry = msg.Registry
	m.lens.AspectRegistry = lookup.NewAspectRegistry(msg.Registry)
	if awaiting {
		return maybeResolveStartup(m)
	}

	// Someone pressed the key before the read landed. Honour it now rather
	// than making them press again — the whole point of taking the request
	// was that the key should not depend on timing.
	// Only open on arrival if the app is still in a state that can hold a
	// modal. The same double duty as above: opening mid-startup would be
	// overwritten by the frame commit.
	if pending && m.blocked(keys.BindingAspects) == "" {
		m.aspectModal = popup.OpenAspects(m.aspectRegistry, m.lens.Aspects)
		m.startup = startupAspectOpen
		m.chooserEntry = m.captureChooserEntry()
	}
	return m, nil
}
