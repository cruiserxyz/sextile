package popup

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// house_display.go is the House display popup (`, a`): one row per form with a
// radio mark, and under a dotted line the focused form's description and an
// example row. Moving the cursor only changes what the lower area says; Space
// chooses the focused form and puts it live on the Bodies panel at once, with
// the popup still open. Enter and Esc close and keep what is chosen, as the
// Aspects and Bodies choosers do.
//
// The choice is the session's, for every slot. The popup writes nothing: it
// reports the form a Space chose and ui puts it in force. The forms, their
// words and the note are passed in; ui owns them (the preferences file reads
// the same table).

// TitleDisplay is the popup's name in its frame.
const TitleDisplay = dialog.TitleHouseDisplay

// houseDisplayVisible is how many rows the paging rule counts as one page: the
// four forms.
const houseDisplayVisible = 4

// HouseFormRow is one form as the popup lists it.
type HouseFormRow struct {
	Form  panel.HouseForm
	Title string
	// Description is one or two sentences; the popup follows every one, on its
	// own line, by the note.
	Description string
}

// HouseDisplay is the popup's list. The chosen form is the Model's; the list's
// own selection is that, restated, and the two are moved together by ui.
type HouseDisplay struct {
	List  menu.RadioList
	Forms []HouseFormRow
	// Note ends every form's description, on its own line.
	Note string
}

// NewHouseDisplay is the popup with the cursor and the mark on the form chosen.
func NewHouseDisplay(forms []HouseFormRow, note string, chosen panel.HouseForm) HouseDisplay {
	idx := 0
	for i, info := range forms {
		if info.Form == chosen {
			idx = i
			break
		}
	}
	return HouseDisplay{List: menu.NewRadioList(len(forms), idx), Forms: forms, Note: note}
}

// HouseDisplayOutcome is what one key decided: Closed when Enter, Esc or q
// left the popup; Chose when Space chose Form.
type HouseDisplayOutcome struct {
	Closed bool
	Chose  bool
	Form   panel.HouseForm
}

// CanChoose is where Space acts: the focus is on a form that is not the chosen
// one.
func (h HouseDisplay) CanChoose() bool {
	return h.List.Cursor != h.List.Selected
}

// Apply runs one action: Dismiss closes keeping the choice, Toggle chooses the
// focused form, the moving keys move the cursor.
func (h *HouseDisplay) Apply(a Action) HouseDisplayOutcome {
	l := h.List
	var out HouseDisplayOutcome
	switch a {
	case Dismiss:
		out.Closed = true
	case Up:
		l.MoveUp()
	case Down:
		l.MoveDown()
	case Toggle:
		l.SelectCursor()
		out.Chose, out.Form = true, h.Forms[l.Selected].Form
	case PageUp:
		l.Page(menu.PageUp, houseDisplayVisible)
	case PageDown:
		l.Page(menu.PageDown, houseDisplayVisible)
	case PageHome:
		l.Page(menu.PageHome, houseDisplayVisible)
	case PageEnd:
		l.Page(menu.PageEnd, houseDisplayVisible)
	}
	h.List = l
	return out
}

// Render draws the popup.
func (h HouseDisplay) Render(cfg term.Style, termW, termH int, chrome dialog.Chrome) string {
	return dialog.Render(dialog.Box{
		Title:   TitleDisplay,
		NoInput: true,
		Fit:     h.Fit(cfg),
	}, cfg, termW, termH, chrome, func(width, height int) string {
		return menu.Block(h.lines(cfg, width, height), width, height)
	})
}

// Fit is the popup's size: as wide as the widest of the four rows, the
// descriptions wrapped at dialog.ProseWidth, the comparing note and the
// example, over all four forms, and as tall as the fullest form needs at that
// width, so the box holds its size while the cursor moves (operator,
// 2026-10-27: a popup's size is decided when it opens). A form with less to say
// leaves rows blank.
func (h HouseDisplay) Fit(cfg term.Style) dialog.Fit {
	widest := func(s string) int {
		w := 0
		for _, l := range strings.Split(ansi.Wordwrap(s, dialog.ProseWidth, ""), "\n") {
			w = max(w, ansi.StringWidth(l))
		}
		return w
	}
	w := widest(h.Note)
	for _, info := range h.Forms {
		w = max(w, menu.RowWidth(menu.RowSpec{Left: radioMark(true, cfg) + " " + info.Title}, cfg),
			widest(info.Description), ansi.StringWidth(HouseExampleRow(info.Form, cfg)))
	}
	rows := 0
	for _, info := range h.Forms {
		rows = max(rows, h.rows(info, w))
	}
	return dialog.Fit{Width: w, Rows: rows}
}

// rows is the rows the popup needs for this form: the four forms, the dotted
// line, and the form's description, the comparing note, a blank line and the
// example.
func (h HouseDisplay) rows(info HouseFormRow, width int) int {
	desc := strings.Split(ansi.Wordwrap(info.Description, width, ""), "\n")
	note := strings.Split(ansi.Wordwrap(h.Note, width, ""), "\n")
	return len(h.Forms) + 1 + len(desc) + len(note) + 2
}

// radioMark is the mark of a row: the filled one for the chosen form, in
// ASCII under --accessible.
func radioMark(chosen bool, cfg term.Style) string {
	switch {
	case !chosen:
		return "( )"
	case cfg.UseASCII():
		return "(*)"
	}
	return "(•)"
}

// lines is the popup's body: the four rows, the dotted line, then as much of
// the focused form's description and example as the height holds.
func (h HouseDisplay) lines(cfg term.Style, width, height int) []string {
	var lines []string
	for i, info := range h.Forms {
		lines = append(lines, menu.Row(menu.RowSpec{
			Left:   radioMark(h.List.IsSelected(i), cfg) + " " + info.Title,
			Cursor: i == h.List.Cursor,
		}, cfg, width))
	}
	lines = append(lines, menu.Rule(cfg, width))

	focus := h.Forms[min(max(h.List.Cursor, 0), len(h.Forms)-1)]
	return append(lines, h.description(focus, cfg, width, height-len(lines))...)
}

// description is the area under the dotted line, in room rows: the
// description, the comparing note, a blank line, and the example. When the room
// is short the example is kept before anything else, then as much of the
// description as fits (cut with an ellipsis), and the comparing note goes
// first: it reads the same under every form.
func (h HouseDisplay) description(info HouseFormRow, cfg term.Style, width, room int) []string {
	if room <= 0 {
		return nil
	}
	wrap := func(s string) []string {
		return strings.Split(ansi.Wordwrap(s, width, ""), "\n")
	}
	desc, note := wrap(info.Description), wrap(h.Note)
	example := HouseExampleRow(info.Form, cfg)

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
			out[keep-1] = term.CutRight(out[keep-1], width-1, cfg) + "…"
		}
		out = append(out, example)
	}
	for i := range out {
		out[i] = term.FitLine(out[i], width, cfg)
	}
	return out
}

// houseExampleBody is the example's Moon, in the 10th house of the compared
// chart and the 5th of its own.
var houseExampleBody = aspect.Position{Object: aspect.Object{Number: 2, Name: "Moon"}, Longitude: 285.5, Speed: 13.2, House: 5}

// HouseExampleRow is the Moon row as the Bodies panel draws it in form: the
// real row renderer, handed a frame in which the Moon's own house is the 5th
// and the house in the compared chart the 10th. Nothing here draws a house
// column by hand.
func HouseExampleRow(form panel.HouseForm, cfg term.Style) string {
	var cusps aspect.Cusps
	for i := range cusps {
		cusps[i] = float64(30 * i) // whole signs from Aries: 285°30' is the 10th
	}
	ov := panel.HouseOverlay{Cusps: cusps, Positions: []aspect.Position{houseExampleBody}, Form: form}
	return panel.FormatPositionRowStyled(houseExampleBody, astrologstyle.BodyRow{}, cfg, ov)
}
