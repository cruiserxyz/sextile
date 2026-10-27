package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// house_display_popup.go is the House display popup (`, a`): one row per form
// with a radio mark, and under a dotted line the focused form's description and
// an example row. Moving the cursor only changes what the lower area says;
// Space chooses the focused form and puts it live on the Bodies panel at once,
// with the popup still open. Enter and Esc close and keep what is chosen, as
// the Aspects and Bodies choosers do.
//
// The choice is the session's, for every slot. It is written by `, c d` (with
// the default city) and read at start (house_display.go); the popup itself
// writes nothing.

// popupTitleDisplay is the popup's name in its frame.
const popupTitleDisplay = "House display"

// houseDisplayVisible is how many rows the paging rule counts as one page: the
// four forms.
const houseDisplayVisible = 4

// houseDisplayState is the popup's list. The chosen form is the Model's
// houseDisplay; the list's own selection is that, restated, and the two are
// moved together by one door (chooseHouseForm).
type houseDisplayState struct {
	list radioList
}

// formIndex is the form's row in houseForms.
func formIndex(f houseForm) int {
	for i, info := range houseForms {
		if info.form == f {
			return i
		}
	}
	return 0
}

// openHouseDisplay is the `, a` handler: it opens the popup on the main screen
// with the cursor on the form in force.
func openHouseDisplay(m Model) (tea.Model, tea.Cmd) {
	if m.startup != startupReady {
		return m, nil
	}
	m.houseDisplayPopup = houseDisplayState{list: newRadioList(len(houseForms), formIndex(m.houseDisplay))}
	m.startup = startupDisplayOpen
	return m, nil
}

// updateHouseDisplayKey handles one keystroke while the popup has the keyboard.
func updateHouseDisplayKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	l := m.houseDisplayPopup.list
	switch matchBinding(msg, displayBindings()) {
	case BindingClose:
		m.startup = startupReady
		return m, nil
	case BindingListUp:
		l.moveUp()
	case BindingListDown:
		l.moveDown()
	case BindingToggle:
		l.selectCursor()
		m.houseDisplay = houseForms[l.selected].form
	case BindingPageUp:
		l.page(pageUp, houseDisplayVisible)
	case BindingPageDown:
		l.page(pageDown, houseDisplayVisible)
	case BindingPageHome:
		l.page(pageHome, houseDisplayVisible)
	case BindingPageEnd:
		l.page(pageEnd, houseDisplayVisible)
	}
	m.houseDisplayPopup.list = l
	return m, nil
}

// displayCanChoose is where Space acts: the focus is on a form that is not
// the chosen one.
func (m Model) displayCanChoose() bool {
	l := m.houseDisplayPopup.list
	return l.cursor != l.selected
}

// renderHouseDisplay draws the popup.
func renderHouseDisplay(st houseDisplayState, cfg Config, termW, termH int) string {
	return renderModalBox(modalBox{
		Title:   popupTitleDisplay,
		NoInput: true,
		Fit:     houseDisplayFit(cfg),
	}, cfg, termW, termH, func(width, height int) string {
		return listBlock(houseDisplayLines(st, cfg, width, height), width, height)
	})
}

// houseDisplayFit is the popup's size: as wide as the widest of the four rows,
// the descriptions wrapped at modalProseWidth, the comparing note and the
// example, over all four forms, and as tall as the fullest form needs at that
// width, so the box holds its size while the cursor moves (operator,
// 2026-10-27: a popup's size is decided when it opens). A form with less to say
// leaves rows blank.
func houseDisplayFit(cfg Config) modalFit {
	widest := func(s string) int {
		w := 0
		for _, l := range strings.Split(ansi.Wordwrap(s, modalProseWidth, ""), "\n") {
			w = max(w, ansi.StringWidth(l))
		}
		return w
	}
	w := widest(houseComparingNote)
	for _, info := range houseForms {
		w = max(w, listRowWidth(listRowSpec{Left: radioMark(true, cfg) + " " + info.title}, cfg),
			widest(info.description), ansi.StringWidth(houseExampleRow(info.form, cfg)))
	}
	rows := 0
	for _, info := range houseForms {
		rows = max(rows, houseDisplayRows(info, w))
	}
	return modalFit{Width: w, Rows: rows}
}

// houseDisplayRows is the rows the popup needs for this form: the four forms,
// the dotted line, and the form's description, the comparing note, a blank line
// and the example.
func houseDisplayRows(info houseFormInfo, width int) int {
	desc := strings.Split(ansi.Wordwrap(info.description, width, ""), "\n")
	note := strings.Split(ansi.Wordwrap(houseComparingNote, width, ""), "\n")
	return len(houseForms) + 1 + len(desc) + len(note) + 2
}

// radioMark is the mark of a row: the filled one for the chosen form, in
// ASCII under --accessible.
func radioMark(chosen bool, cfg Config) string {
	switch {
	case !chosen:
		return "( )"
	case cfg.useASCII():
		return "(*)"
	}
	return "(•)"
}

// houseDisplayLines is the popup's body: the four rows, the dotted line, then
// as much of the focused form's description and example as the height holds.
func houseDisplayLines(st houseDisplayState, cfg Config, width, height int) []string {
	var lines []string
	for i, info := range houseForms {
		lines = append(lines, listRow(listRowSpec{
			Left:   radioMark(st.list.isSelected(i), cfg) + " " + info.title,
			Cursor: i == st.list.cursor,
		}, cfg, width))
	}
	lines = append(lines, listRule(cfg, width))

	focus := houseForms[min(max(st.list.cursor, 0), len(houseForms)-1)]
	return append(lines, houseDisplayDescription(focus, cfg, width, height-len(lines))...)
}

// houseDisplayDescription is the area under the dotted line, in room rows: the
// description, the comparing note, a blank line, and the example. When the room
// is short the example is kept before anything else, then as much of the
// description as fits (cut with an ellipsis), and the comparing note goes
// first: it reads the same under every form.
func houseDisplayDescription(info houseFormInfo, cfg Config, width, room int) []string {
	if room <= 0 {
		return nil
	}
	wrap := func(s string) []string {
		return strings.Split(ansi.Wordwrap(s, width, ""), "\n")
	}
	desc, note := wrap(info.description), wrap(houseComparingNote)
	example := houseExampleRow(info.form, cfg)

	var out []string
	switch {
	case room >= len(desc)+len(note)+2:
		out = append(append(append(out, desc...), note...), "", example)
	case room >= len(desc)+len(note)+1:
		out = append(append(out, desc...), note...)
		out = append(out, example)
	case room >= len(desc)+1:
		out = append(append(out, desc...), example)
	default:
		keep := room - 1
		if keep > 0 {
			out = append(out, desc[:keep]...)
			out[keep-1] = cutRight(out[keep-1], width-1, cfg) + "…"
		}
		out = append(out, example)
	}
	for i := range out {
		out[i] = fitLine(out[i], width, cfg)
	}
	return out
}

// houseExampleBody is the example's Moon, in the 10th house of the compared
// chart and the 5th of its own.
var houseExampleBody = aspect.Position{Body: aspect.Moon, Longitude: 285.5, Speed: 13.2, House: 5}

// houseExampleRow is the Moon row as the Bodies panel draws it in form: the
// real row renderer, handed a frame in which the Moon's own house is the 5th
// and the house in the compared chart the 10th. Nothing here draws a house
// column by hand.
func houseExampleRow(form houseForm, cfg Config) string {
	var cusps aspect.Cusps
	for i := range cusps {
		cusps[i] = float64(30 * i) // whole signs from Aries: 285°30' is the 10th
	}
	ov := houseOverlay{cusps: cusps, positions: []aspect.Position{houseExampleBody}, form: form}
	return formatPositionRowStyled(houseExampleBody, astrologstyle.BodyRow{}, cfg.useColor(), ov)
}
