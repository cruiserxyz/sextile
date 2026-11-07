package popup

import (
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// experiments.go is the hidden Experiments popup (`,l p`): one row per
// experimental switch with a tick, its id and one line saying what it changes.
// Space flips the focused row, Enter, Esc and q close. The popup writes
// nothing and keeps no truth of its own: ui runs the write, and only a write
// that succeeded changes a row's tick (SetOn).

// TitleExperiments is the popup's name in its frame.
const TitleExperiments = dialog.TitleExperiments

// ExperimentRow is one switch as the popup lists it.
type ExperimentRow struct {
	ID   string
	Line string
	On   bool
}

// Experiments is the popup's list.
type Experiments struct {
	Rows   []ExperimentRow
	Cursor int
	// Pending is the id whose write is out; Space on that row does nothing.
	Pending string
	// Foot is the one line at the popup's foot, `Not saved: <cause>`, until
	// the next key.
	Foot string
}

// NewExperiments is the popup over rows with the cursor on the first.
func NewExperiments(rows []ExperimentRow) Experiments {
	return Experiments{Rows: rows}
}

// ExperimentsOutcome is what one key decided: Closed when Enter, Esc or q left
// the popup; Flip when Space asked for the focused row to be turned to To.
type ExperimentsOutcome struct {
	Closed bool
	Flip   bool
	ID     string
	To     bool
}

// Apply runs one action. Every key clears the foot.
func (e *Experiments) Apply(a Action) ExperimentsOutcome {
	e.Foot = ""
	var out ExperimentsOutcome
	switch a {
	case Dismiss:
		out.Closed = true
	case Up:
		e.Cursor = max(e.Cursor-1, 0)
	case Down:
		e.Cursor = min(e.Cursor+1, len(e.Rows)-1)
	case Toggle:
		if e.Cursor < 0 || e.Cursor >= len(e.Rows) {
			break
		}
		row := e.Rows[e.Cursor]
		if row.ID == e.Pending {
			break
		}
		out.Flip, out.ID, out.To = true, row.ID, !row.On
		e.Pending = row.ID
	}
	return out
}

// Settled records the result of the write for id: a success turns the tick to
// on, a failure keeps the row and says why at the foot.
func (e *Experiments) Settled(id string, on bool, cause error) {
	if e.Pending == id {
		e.Pending = ""
	}
	if cause != nil {
		e.Foot = "Not saved: " + cause.Error()
		return
	}
	for i := range e.Rows {
		if e.Rows[i].ID == id {
			e.Rows[i].On = on
		}
	}
}

func (e Experiments) rowSpec(i int) menu.RowSpec {
	row := e.Rows[i]
	mark := "[ ]"
	if row.On {
		mark = "[✓]"
	}
	return menu.RowSpec{Left: mark + " " + row.ID + "  " + row.Line, Cursor: i == e.Cursor}
}

// Fit is the popup's size: as wide as its widest row, one row each and the foot.
func (e Experiments) Fit(cfg term.Style) dialog.Fit {
	w := 1
	for i := range e.Rows {
		w = max(w, menu.RowWidth(e.rowSpec(i), cfg))
	}
	return dialog.Fit{Width: w, Rows: len(e.Rows) + 1}
}

// Render draws the popup.
func (e Experiments) Render(cfg term.Style, termW, termH int, chrome dialog.Chrome) string {
	return dialog.Render(dialog.Box{
		Title:   TitleExperiments,
		NoInput: true,
		Fit:     e.Fit(cfg),
	}, cfg, termW, termH, chrome, func(width, height int) string {
		var lines []string
		for i := range e.Rows {
			lines = append(lines, menu.Row(e.rowSpec(i), cfg, width))
		}
		lines = append(lines, menu.Note(e.Foot, width, cfg))
		return menu.Block(lines, width, height)
	})
}
