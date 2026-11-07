package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/popup"

	tea "github.com/charmbracelet/bubbletea"
)

// atlas_modal_model.go is the Model's half of the City picker (⌃l; the popup
// itself is internal/ui/popup/atlas.go): the key-to-action mapping, the phase
// Esc puts back and the pick's effect. The atlas load and the resolve stay with
// the startup handlers.

// atlasModalState is the City picker: the popup's state, plus the phase Esc
// puts back.
type atlasModalState struct {
	popup.Atlas

	// prior is the phase Esc puts back: what the app was doing when ⌃l
	// opened the picker. Empty for the cold-start picker, which Esc leaves
	// in startupBootstrapping.
	prior startupState
}

// atlasTypeHint is the empty chooser's line (the popup's).
const atlasTypeHint = popup.AtlasTypeHint

// cityBindings is the City popup's table.
func cityBindings() []keys.BindingEntry {
	return keys.Titled(append([]keys.BindingEntry{listUp().ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 1 }), listDown(), confirm("Choose").ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 0 }), closeOn("Cancel")}, pageEntries()...))
}

// openCityPicker is Ctrl+L: the universal re-open of the atlas modal. Honors
// [PARITY-HARD] Ctrl+L. The atlas index is already loaded (loadAtlasCmd fired at
// Init); the modal opens with an empty filter buffer. The IANA preseed
// (system-derived city) only ever pre-fills on first cold-start without a
// stored location -- re-opens after that show an empty buffer so the user types
// fresh, not against leftover text from a prior pick.
//
// The phase it opened from is kept, so Esc puts it back: re-opening the picker
// from a running app must not drop the app into its startup phase (the
// specification C7, C8). Nothing stops it, so blocked has no reason for it.
func (m Model) openCityPicker() (Model, tea.Cmd) {
	m.atlas.prior = m.startup
	m.startup = startupAtlasOpen
	m.atlas.FilterText = ""
	m.atlas.Cursor = 0
	m.atlas.Refilter()
	return m, nil
}

// updateAtlasModalKey handles a single keystroke targeted at the atlas
// modal: the key resolves through the bindings table to the popup's Action.
// Returns the (possibly updated) Model + tea.Cmd. Caller in model.go has
// already gated on m.startup == startupAtlasOpen.
func updateAtlasModalKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	a := popup.Type
	switch keys.MatchBinding(msg, cityBindings()) {
	case keys.BindingConfirm:
		a = popup.Confirm
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
	case keys.BindingClose:
		a = popup.Dismiss
	}
	out := m.atlas.Apply(a, msg, dialog.ListHeight(m.height))
	switch {
	case out.Picked:
		var cmd tea.Cmd
		m, cmd = applyAtlasPicked(m, AtlasPickedMsg{Place: out.Place})
		return m, cmd
	case out.Dismissed:
		// Esc closes the picker and puts back the phase it was opened from:
		// a running app stays running (the specification C7,
		// C8), and the cold-start picker lands in startupBootstrapping —
		// empty panels, no modal — from which ⌃l re-opens it.
		m.startup = m.atlas.prior
		if m.startup == "" {
			m.startup = startupBootstrapping
		}
	}
	return m, nil
}

// renderAtlasModal renders the centered modal with the chrome and the position
// counter the popups share.
func renderAtlasModal(state atlasModalState, cfg Config, termW, termH int) string {
	return state.Render(cfg, termW, termH, popupChrome(cfg), func(cursor, total int) (*dialog.Counter, *string) {
		pos := &listPosition{Cursor: cursor, Total: total}
		return popupCounter(pos, cfg), &pos.Text
	}, atlasModalHint)
}

// renderResultList draws the place picker's list.
func renderResultList(state atlasModalState, cfg Config, width, height int) string {
	return state.ResultList(cfg, width, height, atlasModalHint)
}
