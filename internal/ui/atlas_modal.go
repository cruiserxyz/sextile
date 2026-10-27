package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
)

// atlasModalState carries the stateful atlas picker. cp1 had a render-only
// function; cp2 makes the modal interactive.
//
// Invariants:
//   - matches is always derived from filterText via refilter(); never set
//     directly. Keeps cursor + matches in sync.
//   - cursor is always in [0, len(matches)] — clamped on every refilter.
//   - index is nil until AtlasLoadedMsg arrives; modal renders a loading
//     line when nil.
//   - loadErr non-nil → modal renders an error line and Enter does nothing.
type atlasModalState struct {
	// The search buffer, cursor, matches and truncation flag live in the
	// shared chassis and are embedded, so every existing reference
	// -- m.atlas.filterText, .cursor, .matches, .truncated -- still reads
	// exactly as it did. What stays here is what only the place picker has.
	fuzzyList[location.PlaceInfo]

	index   []location.Indexed
	preseed string
	loadErr error
	// prior is the phase Esc puts back: what the app was doing when ⌃l
	// opened the picker. Empty for the cold-start picker, which Esc leaves
	// in startupBootstrapping.
	prior startupState
}

// maxAtlasResults caps the matched-list size per render. 200 keeps render
// + alloc cheap on broad queries (single-char) and is well above what a
// modal of height ~24 can show. The truncation hint surfaces to the user.
const maxAtlasResults = 200

// refilter recomputes matches from filterText against the loaded index.
// Cursor clamps into the new matches slice. No-op when the index has not
// been loaded.
func (a *atlasModalState) refilter() {
	if a.index == nil {
		a.clear()
		return
	}
	a.setMatches(location.Filter(a.index, a.filterText, maxAtlasResults))
}

// updateAtlasModalKey handles a single keystroke targeted at the atlas
// modal. Returns the (possibly updated) Model + tea.Cmd. Caller in
// model.go has already gated on m.startup == startupAtlasOpen.
func updateAtlasModalKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch matchBinding(msg, cityBindings()) {
	case BindingConfirm:
		if m.atlas.loadErr != nil || len(m.atlas.matches) == 0 {
			return m, nil
		}
		picked := m.atlas.matches[m.atlas.cursor]
		var cmd tea.Cmd
		m, cmd = applyAtlasPicked(m, AtlasPickedMsg{Place: picked})
		return m, cmd
	case BindingListUp:
		m.atlas.moveUp()
		m.atlas.scrollTo(m.atlas.cursor, modalListHeight(m.height))
		return m, nil
	case BindingListDown:
		m.atlas.moveDown()
		m.atlas.scrollTo(m.atlas.cursor, modalListHeight(m.height))
		return m, nil
	case BindingPageUp:
		m.atlas.page(pageUp, modalListHeight(m.height))
		return m, nil
	case BindingPageDown:
		m.atlas.page(pageDown, modalListHeight(m.height))
		return m, nil
	case BindingPageHome:
		m.atlas.page(pageHome, modalListHeight(m.height))
		return m, nil
	case BindingPageEnd:
		m.atlas.page(pageEnd, modalListHeight(m.height))
		return m, nil
	case BindingClose:
		// Esc closes the picker and puts back the phase it was opened from:
		// a running app stays running (the specification C7,
		// C8), and the cold-start picker lands in startupBootstrapping —
		// empty panels, no modal — from which ⌃l re-opens it.
		m.startup = m.atlas.prior
		if m.startup == "" {
			m.startup = startupBootstrapping
		}
		return m, nil
	}
	// Every typed character goes into the filter, `q` included: the quit
	// binding is the main screen's.
	if m.atlas.typeKey(msg) {
		m.atlas.refilter()
		m.atlas.cursor = 0
		m.atlas.offset = 0
	}
	return m, nil
}

// renderAtlasModal renders the centered modal: search line + result list +
// footer. The chrome is the shared modalBox; only the list below is
// the atlas picker's own.
func renderAtlasModal(state atlasModalState, cfg Config, termW, termH int) string {
	pos := listPosition{Cursor: state.cursor}
	if state.index != nil && state.loadErr == nil && state.filterText != "" {
		pos.Total = len(state.matches)
	}
	box := modalBox{
		Title:    popupTitleCity,
		Fit:      atlasFit(),
		Position: &pos,
		Scroll: func(height int) (viewport, int) {
			_, offset := state.windowAt(state.cursor, height)
			return viewport{Total: pos.Total, Visible: height, Offset: offset}, 0
		},
		FilterText: state.filterText,
	}
	// A name the row had to cut is shown whole in the bottom edge, where the
	// counter is, for as long as the cursor is on it.
	if pos.Total > 0 && state.cursor < len(state.matches) {
		p := state.matches[state.cursor]
		row := listRowSpec{Left: p.Name, Right: []string{p.Country}}
		if w := modalContentWidth(box, cfg, termW); listRowWidth(row, cfg) > w {
			pos.Text = cutRight(p.Name, w+modalChromeWidth-4, cfg)
		}
	}
	return renderModalBox(box, cfg, termW, termH, func(width, height int) string {
		return renderResultList(state, cfg, width, height)
	})
}

// atlasFit is the place picker's size, and it is always the same: as wide as
// its hint line, and eight rows tall (operator, 2026-10-27: a popup's size is
// decided once, when it opens, and never changes while it is open). The atlas
// has 33,627 places and one of 57 cells, so no size follows its content: a
// longer name is cut with the cut mark, and while its row is under the cursor
// the box's bottom edge shows the name.
func atlasFit() modalFit {
	return modalFit{Width: ansi.StringWidth(atlasTypeHint), Rows: atlasRows}
}

// atlasRows is the result rows the City list always has.
const atlasRows = 8

// atlasTypeHint is the empty chooser's line: what to type, with a query that
// ranks its example first. No comma: the search key is `name country`.
const atlasTypeHint = "Type a city, e.g. Amsterdam. Narrow it: Paris FR"

// renderResultList draws the place picker's list: the city on the left and its
// country code pinned to the right edge (the specification C8).
func renderResultList(state atlasModalState, cfg Config, width, height int) string {
	if state.loadErr != nil {
		return listMessage(atlasModalHint, width, height)
	}
	if state.index == nil {
		return listMessage("(loading atlas…)", width, height)
	}
	if state.filterText == "" {
		return listMessage(atlasTypeHint, width, height)
	}
	if len(state.matches) == 0 {
		return listMessage("(no matches)", width, height)
	}

	// The window scrolls to follow the cursor; without it the cursor walks
	// past the bottom of the list and simply vanishes.
	visible, offset := state.windowAt(state.cursor, height)
	lines := make([]string, 0, height)
	for i, p := range visible {
		lines = append(lines, listRow(listRowSpec{
			Left:   p.Name,
			Right:  []string{p.Country},
			Cursor: offset+i == state.cursor,
		}, cfg, width))
	}
	if state.truncated && len(lines) < height {
		lines = append(lines, listNote(fmt.Sprintf("…%d more", len(state.matches)-len(visible)), width, cfg))
	}
	return listBlock(lines, width, height)
}
