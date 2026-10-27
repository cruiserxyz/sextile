package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// save_modal.go is the prompt that saves the FLOATING slot into the chart
// library as an astrolog chart file. It follows the two pickers' shape — a centred
// bordered box over a modal state — but it is a prompt, not a picker: there is
// no list to filter and Enter acts on what was typed rather than on a row.
//
// State machine (driven from model.go Update):
//
//	main ──⌃s / <leader>s──▶ startupSaveOpen ──Enter──▶ (save runs off-loop)
//	                              │ ▲ SaveChartDoneMsg{Err: nil}  ──▶ prior state
//	                              │ └ SaveChartDoneMsg{Err: taken} ──▶ stays open
//	                              └─Esc──▶ prior state
//
// It opens ONLY when the floating slot is focused. Saving the anchored slot is
// out of scope: an anchored chart came FROM the library, so writing it back is
// a copy under a second name, and the one thing this feature was authorised to
// do is create a chart that did not exist.
type saveModalState struct {
	form    form   // one field, "Save as: "; its value is what the operator has typed
	dest    string // the charts directory this would write into, shown beneath
	refusal string // non-empty: the last attempt was refused and the prompt stays open
	saving  bool   // a save is in flight; Enter is inert until it lands
	prior   startupState
}

// newSaveForm is the prompt's one field. Always empty, and with no
// placeholder: a prompt that suggested a name would invite
// Enter-without-reading, and every save creates a NEW file.
func newSaveForm() form {
	return newForm(formField{label: "Save as: "})
}

// typedName is what the operator has typed so far.
func (s saveModalState) typedName() string {
	return s.form.value(0)
}

// openSavePrompt is ⌃s and the leader's `s`, from main mode.
//
// Both bindings land here rather than one delegating to the other, because a
// terminal holding ⌃s for flow control has to reach the same action by the
// leader — and a leader sequence that merely synthesised a key press would
// depend on the very key that is being swallowed.
func (m Model) openSavePrompt() (tea.Model, tea.Cmd) {
	// Before the workspace resolves there is no floating slot to save, the
	// same pre-resolution gate ⌃o carries.
	if m.slots == nil {
		return m, nil
	}
	all := m.slots.All()
	if m.cursorIdx < 0 || m.cursorIdx >= len(all) {
		return m, nil
	}
	if !all[m.cursorIdx].Floating {
		// Saving acts on the floating slot only. On the anchored slot the key
		// does nothing and the key bar says why.
		return m.showNotice(noticeOnlyNow)
	}
	m.save = saveModalState{
		form: newSaveForm(),
		dest: store.ChartsDir(),
		// Captured BEFORE the state moves, so Esc restores the view the
		// prompt was opened from.
		prior: m.startup,
	}
	m.startup = startupSaveOpen
	return m, nil
}

// updateSaveModalKey handles one keystroke for the save prompt. The caller in
// model.go has already gated on m.startup == startupSaveOpen.
func updateSaveModalKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch matchBinding(msg, saveChartBindings()) {
	case BindingConfirm:
		if m.save.saving || strings.TrimSpace(m.save.typedName()) == "" {
			return m, nil
		}
		// A name Astrolog could not read back is refused here, before any
		// spawn, with the typed text left in the field (the specification
		// C1, C3).
		if err := lookup.CheckLabelLength("name", m.save.typedName()); err != nil {
			m.save.refusal = err.Error()
			return m, nil
		}
		slot := m.slots.Floating()
		m.save.saving = true
		m.save.refusal = ""
		// The directory captured when the prompt opened is the one the
		// operator was SHOWN, and it is what the save uses -- resolving again
		// here could pick a different one (store.ChartsDir()'s second tier
		// turns on when the XDG default appears), which would write the chart
		// somewhere nobody displayed or checked.
		m.saveGen++
		return m, saveChartCmd(m.saveGen, m.save.dest, m.save.typedName(), slot)
	case BindingClose:
		// Bumping the generation is what makes Esc actually cancel. The
		// astrolog spawn is already running and is not interruptible, so a
		// save backed out of still COMPLETES -- and without this, its message
		// would arrive later and apply itself to whatever the operator had
		// moved on to, restoring a `prior` this line has already zeroed.
		// A save already handed to Astrolog is remembered, so its result is
		// still logged when it lands (A8).
		if m.save.saving {
			m.abandonedSaves = abandon(m.abandonedSaves, m.saveGen)
		}
		m.saveGen++
		m.startup = m.save.prior
		m.save = saveModalState{}
		return m, nil
	}
	// Typing is the form's. Nothing is typed while a save is in flight: the
	// name on screen must stay the name being saved.
	if m.save.saving {
		return m, nil
	}
	m.save.form, _ = m.save.form.updateKey(msg)
	return m, nil
}

// SaveChartDoneMsg is one completed save attempt.
type SaveChartDoneMsg struct {
	// Gen is the save generation this message belongs to. A message whose
	// generation is not the model's current one is DROPPED: the save it
	// reports was abandoned, and applying it would move the operator's view
	// out from under them. Every other async completion in this package
	// carries the same kind of token (pickGen, leader gen, the refresh
	// coordinator's runID); this one did not, and a save cancelled with Esc
	// could land afterwards and close a modal the operator had since opened.
	Gen   int
	Name  string // the name the save was asked for, so a late failure can say it
	Saved lookup.SavedChart
	Err   error
}

// saveChartCmd runs the save off the Update loop: it spawns astrolog and
// writes a file, neither of which belongs in a key handler.
//
// The command it ran is deliberately NOT carried into the Log's argv lane
// (C10), for the same reason resolveChartCmd does not carry its own: the argv
// lane is what the console copies to a clipboard, and it is reserved for the
// spawns that produced the frame on screen. A save produces a file, not a
// frame.
func saveChartCmd(gen int, dir, name string, slot store.Slot) tea.Cmd {
	return func() tea.Msg {
		saved, err := lookup.SaveFloatingChart(context.Background(), dir, name, slot.Time, slot.Location)
		return SaveChartDoneMsg{Gen: gen, Name: name, Saved: saved, Err: err}
	}
}

// applySaveDone folds a finished save back into the model.
//
// A refusal keeps the prompt OPEN with the typed name intact (C6a): the
// operator's next move is to type a different name, and a prompt that closed
// would make them reopen it and retype what they had.
func applySaveDone(m Model, msg SaveChartDoneMsg) (Model, tea.Cmd) {
	if msg.Gen != m.saveGen {
		// A save the operator backed out of with Esc, arriving late. It
		// already wrote its file -- the spawn is not interruptible -- so its
		// result is logged once, and nothing else about the view is touched
		// (A8). A result from any other stale generation is dropped.
		var late bool
		if m.abandonedSaves, late = reclaim(m.abandonedSaves, msg.Gen); !late {
			return m, nil
		}
		if msg.Err != nil {
			var taken *lookup.ErrSlugTaken
			switch {
			case errors.As(msg.Err, &taken):
				return m.logError("Couldn't save %q. That name is taken.", msg.Name), nil
			case errors.Is(msg.Err, lookup.ErrEmptySlug):
				return m.logError("Couldn't save %q. That name has no letters or digits.", msg.Name), nil
			}
			return m.noteEphemeris(msg.Err).logError("Couldn't save %q. %s", msg.Name, m.failureReason(msg.Err, firstLine(msg.Err.Error()))), nil
		}
		return m.logSaved(msg.Saved), nil
	}
	m.save.saving = false
	if msg.Err != nil {
		var taken *lookup.ErrSlugTaken
		switch {
		case errors.As(msg.Err, &taken):
			m.save.refusal = fmt.Sprintf("%q is taken — try another name", taken.Slug)
		case errors.Is(msg.Err, lookup.ErrEmptySlug):
			m.save.refusal = "That name has no letters or digits to save under."
		default:
			// Not a refusal of what was typed: the save failed. The form
			// keeps the cause under its field, and the Log says it, red.
			m.save.refusal = firstLine(msg.Err.Error())
			return m.noteEphemeris(msg.Err).logError("Couldn't save %q. %s", msg.Name, m.failureReason(msg.Err, firstLine(msg.Err.Error()))), nil
		}
		return m, nil
	}
	// Saved. Close the prompt and say so in ONE line (C1a), on the note lane.
	//
	// The note lane, not the error lane: nothing failed. The operator asked
	// for a file and got one. The caveat below rides the same line rather
	// than taking a second, and it is not a logErrorf — TestErrorLaneMembership
	// holds the error lane to a closed set of "Couldn't …" sentences, and
	// widening it so a SUCCESS could appear there would cost the lane the
	// meaning that makes it worth reading.
	prior, dest := m.save.prior, m.save.dest
	m.startup = prior
	m.save = saveModalState{}
	m = m.logSaved(msg.Saved)
	// The new chart opens right away, by the path Ctrl+O takes: the file just
	// written is read back and anchored, the cursor rests on it, nothing is
	// selected. A file already open is jumped to, as a pick of it would.
	m.picker.prior = prior
	return resolvePickIn(m, dest, store.NamedChart{
		ID:     msg.Saved.ID,
		Name:   msg.Saved.Name,
		Source: store.ChartSource{Kind: store.ChartKindFile, File: msg.Saved.File},
	})
}

// logSaved is the one line a finished save writes, open or late.
func (m Model) logSaved(saved lookup.SavedChart) Model {
	caveat := ""
	if !saved.AtlasPlaced {
		// The file is correct, but the library rebuilds a chart file's timezone
		// from the nearest atlas city and DROPS a row it cannot place. This
		// clause is the only chance the operator gets: the symptom
		// otherwise is a chart that is simply not in the picker.
		caveat = " — it will not list until the atlas can place its coordinates"
	}
	m = m.logNote("save", "%q saved to %s%s", saved.Name, saved.Path, caveat)
	if saved.PlaceCut != "" {
		// The slot's place was longer than Astrolog can read back; it is the
		// slot's, not typed here, so it is cut and said (the specification).
		m = m.logNote("save", "Place shortened to %d bytes for Astrolog: %s", placeBound, saved.PlaceCut)
	}
	return m
}

// placeBound is the byte bound the Log line names, read from the one rule.
const placeBound = lookup.MaxLabelBytes

// destLine is the line beneath the input: the charts folder with its path
// elided, then the file the typed name would become.
//
// Elided rather than full, at EVERY width (operator, 2026-09-20). The filename
// is the part the operator just typed and is deciding about, so it is the part
// that must never be cut; the folder is context and survives as its last
// component. At 80 columns the full path did not fit and was truncated
// mid-word, hiding the ".as" and, in the refusal state, the half of the
// sentence that says what to do next. The confirmation line applySaveDone
// writes to the Log carries the FULL path, which is where that information
// went rather than where it was lost — and it is worth saying once, because
// ChartsDir() resolves through four tiers and a dev-fixture library looks
// exactly like the real one from inside this prompt.
func destLine(state saveModalState, cfg Config) string {
	lead := "…/"
	if cfg.useASCII() {
		lead = ".../"
	}
	dir := lead + filepath.Base(state.dest)
	slug := store.Slugify(state.typedName())
	if slug == "" {
		return dir
	}
	return dir + "/" + slug + store.ChartFileExt
}

// firstLine trims a multi-line error down to the sentence a prompt can show.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// renderSaveModal draws the prompt through the form: the name field, the
// destination beneath it, and a refusal when there is one.
func renderSaveModal(state saveModalState, cfg Config, termW, termH int) string {
	return renderForm(state.form, formView{
		Title:   popupTitleSaveChart,
		Info:    []string{destLine(state, cfg)},
		Refusal: state.refusal,
	}, cfg, termW, termH)
}
