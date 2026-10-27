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

// title_form.go asks for a title when an untitled chart is picked
// (the specification item 3). It draws through the form chassis
// (form.go) and keeps its own state machine, as the save prompt does:
//
//	chart picker ──Enter on an untitled chart──▶ startupTitleOpen
//	    startupTitleOpen ──Esc──▶ the chart picker as it was; nothing opens
//	    startupTitleOpen ──Enter with no title──▶ the pick as today
//	    startupTitleOpen ──Enter with a title──▶ the confirm step (nothing written)
//	        confirm ──Cancel──▶ the form, typed values intact
//	        confirm ──Save title──▶ (write runs off-loop)
//	        TitleWrittenMsg{Err: nil} ──▶ the pick, under the new title
//	        TitleWrittenMsg{Err: …}   ──▶ stays open, with the refusal
//
// Skipping is the pick exactly as it was before this form existed: nothing is
// written, and the next pick of the same chart asks again (C3.6). Only a pick
// asks; a restore never reaches this file (C3.16). Esc is not a skip: it goes
// back to the list (the specification C9).
//
// Ctrl+R in the picker opens the same form as a rename (item 4), prefilled
// with the chart's title and place, and it differs only at the ends:
//
//	chart picker ──Ctrl+R──▶ startupTitleOpen (rename)
//	    ──Esc, Enter with no title, or nothing changed──▶ the picker as it was
//	    TitleWrittenMsg{Err: nil} ──▶ the picker, reloaded, cursor on the chart;
//	                                  nothing anchors (C4.3)
type titleFormState struct {
	form    form
	chart   store.NamedChart // the row being titled, as the picker listed it
	seen    string           // the listing's fingerprint of its file
	refusal string           // the last submit was refused; the form stays open
	saving  bool             // a write is in flight; only Esc acts until it lands
	rename  bool             // opened by Ctrl+R: a write returns to the picker

	confirming bool   // Enter on a filled form showed the second step; nothing is written yet
	dir        string // the charts directory captured when the second step opened
	backupHeld bool   // the chart's backup was already kept when the second step opened
}

// currentTitle is the title c's file states, or "" for an untitled chart,
// whose Name is only its file stem standing in.
func currentTitle(c store.NamedChart) string {
	if c.Untitled {
		return ""
	}
	return c.Name
}

// newRenameForm is the form for renaming c (C4.1): both fields hold what the
// file states now, as values the operator edits. An untitled chart's Title is
// empty and hints its current name, as the pick's form does.
func newRenameForm(c store.NamedChart) form {
	f := newTitleForm(c)
	f.fields[0].value = currentTitle(c)
	return f
}

// openRenameForm is Ctrl+R on c, whatever else its file holds: Astrolog
// writes back only the record, the first backup keeps the original whole, and
// Sextile's own settings travel on the argv, not in the file
// (the specification Part C). Every row the library lists is a chart file
// since YAML charts were retired (D28); anything else has no file Astrolog
// could rewrite, and Ctrl+R does nothing on it.
func openRenameForm(m Model, c store.NamedChart) Model {
	if c.Source.Kind != store.ChartKindFile {
		return m
	}
	// As a pick's form does, it supersedes any resolve still in flight.
	m.pickGen++
	m.picker.resolving = false
	m.title = titleFormState{form: newRenameForm(c), chart: c, seen: m.picker.seen[c.Source.File], rename: true}
	m.startup = startupTitleOpen
	return m
}

// emptyPlaceRefusal is C3.6b's sentence, as a required-field error: the chart
// states no place and none was typed, and Astrolog cannot be asked to write an
// empty one. It names no key: an error is not the place to explain the way
// out (operator, 2026-10-02), and an empty title already opens the chart
// untitled.
const emptyPlaceRefusal = "Add a place to save this title."

// backupRefusal is the second step's refusal when the folder keeps the chart's
// original somewhere that cannot hold it. One line of the narrowest form.
const backupRefusal = "This file can't keep its original, so nothing was saved."

// titlePlaceField is the form's Place field.
const titlePlaceField = 1

// needsTitle reports whether picking c asks for a title first (C3.4): a chart file
// with no title or no place label, whatever else the file holds (Part C).
func needsTitle(c store.NamedChart) bool {
	if c.Source.Kind != store.ChartKindFile {
		return false
	}
	return c.Untitled || c.PlaceLabel == ""
}

// newTitleForm is the form for c: the Title field empty with the row's
// current name as its hint (no autofill, by the operator's decision), and the
// Place field holding the file's own place label, or hinting the atlas place
// when the file states none.
func newTitleForm(c store.NamedChart) form {
	place := formField{label: "Place: ", value: c.PlaceLabel}
	if c.PlaceLabel == "" {
		place.placeholder = c.Place.Display()
	}
	return newForm(formField{label: "Title: ", placeholder: c.Name}, place)
}

// openTitleForm replaces the pick with the form. The picker's own state is
// left as it was, so a skip resumes the pick from exactly there.
//
// Opening it is a pick, so it bumps the pick generation: a resolve still in
// flight from an earlier pick must not land and anchor behind the form.
func openTitleForm(m Model, c store.NamedChart) Model {
	m.pickGen++
	m.title = titleFormState{form: newTitleForm(c), chart: c, seen: m.picker.seen[c.Source.File]}
	m.startup = startupTitleOpen
	return m
}

// updateTitleFormKey handles one keystroke. The caller in model.go has gated
// on m.startup == startupTitleOpen.
func updateTitleFormKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.title.confirming {
		// The second step: only Save title and Cancel act. Cancel is back to
		// the form with the typed values, and nothing was written.
		switch matchBinding(msg, titleConfirmBindings()) {
		case BindingClose:
			m.title.confirming = false
			return m, nil
		case BindingConfirm:
			return startTitleWrite(m)
		}
		return m, nil
	}
	id := matchBinding(msg, chartTitleBindings())
	if id == BindingClose {
		return titleBack(m)
	}
	if m.title.saving {
		// A write is in flight. Only Esc acts: the name on screen must stay
		// the name being written.
		return m, nil
	}
	switch id {
	case BindingNextField:
		m.title.form = m.title.form.nextField()
		return m, nil
	case BindingPrevField:
		m.title.form = m.title.form.prevField()
		return m, nil
	case BindingConfirm:
		return confirmTitle(m)
	}
	m.title.form, _ = m.title.form.updateKey(msg)
	return m, nil
}

// confirmTitle is Enter on the form. An empty title skips; a refusal stays in
// the form; a filled form of a pick opens the second step, which names the
// file and its backup and writes nothing. A rename keeps its direct write.
func confirmTitle(m Model) (tea.Model, tea.Cmd) {
	title := strings.TrimSpace(m.title.form.value(0))
	if title == "" {
		return skipTitle(m)
	}
	place := strings.TrimSpace(m.title.form.value(1))
	if place == "" {
		place = m.title.chart.PlaceLabel
	}
	if m.title.rename && title == currentTitle(m.title.chart) && place == m.title.chart.PlaceLabel {
		// Nothing to rename: write nothing (C4.4).
		return titleBack(m)
	}
	if place == "" {
		m.title.refusal = emptyPlaceRefusal
		m.title.form.focus = titlePlaceField
		return m, nil
	}
	for _, l := range []struct {
		what  string
		s     string
		field int
	}{{"title", title, 0}, {"place", place, titlePlaceField}} {
		if err := lookup.CheckLabelLength(l.what, l.s); err != nil {
			m.title.refusal = err.Error()
			m.title.form.focus = l.field
			return m, nil
		}
	}
	m.title.refusal = ""
	if m.title.rename {
		m.title.dir = store.ChartsDir()
		return startTitleWrite(m)
	}
	// The directory is fixed here, once: Save title writes into the
	// directory the popup named, whatever the resolver answers later.
	dir := store.ChartsDir()
	held, err := store.ChartBackupState(dir, m.title.chart.Source.File)
	if err != nil {
		m.title.refusal = backupRefusal
		return m, nil
	}
	m.title.dir, m.title.backupHeld, m.title.confirming = dir, held, true
	return m, nil
}

// startTitleWrite hands the typed values to the write, in the directory
// captured when the form was submitted. The checks ran before the second
// step; the values are read again from the same unchanged form.
func startTitleWrite(m Model) (tea.Model, tea.Cmd) {
	title := strings.TrimSpace(m.title.form.value(0))
	place := strings.TrimSpace(m.title.form.value(1))
	if place == "" {
		place = m.title.chart.PlaceLabel
	}
	m.title.confirming = false
	m.title.saving = true
	m.titleGen++
	// Astrolog rewrites the file, and the rewrite can change the house system
	// its line names. What the file names now is read first, so a chart with
	// no house choice keeps its houses through the rename (C9).
	return m, m.renameCmd(m.titleGen, m.title.dir, m.title.chart, m.title.seen, title, place)
}

// titleBack is Esc: the chart picker comes back exactly as the pick left it,
// filter and cursor, and nothing is resolved. A write already handed to
// Astrolog cannot be called back, so it finishes; bumping the generation is
// what keeps its result from anchoring a chart the operator walked away from.
func titleBack(m Model) (tea.Model, tea.Cmd) {
	if m.title.saving {
		// Its result is still logged when it lands (A8).
		m.abandonedTitles = abandon(m.abandonedTitles, m.titleGen)
	}
	m.titleGen++
	m.title = titleFormState{}
	m.startup = startupChartOpen
	return m, nil
}

// skipTitle is the pick as it was before the form: back to the picker, and
// the resolve dispatched for the row as listed. A rename opened from the
// picker, not from a pick, so skipping it is going back (C4.4).
func skipTitle(m Model) (tea.Model, tea.Cmd) {
	if m.title.rename {
		return titleBack(m)
	}
	c := m.title.chart
	m.title = titleFormState{}
	m.startup = startupChartOpen
	var cmd tea.Cmd
	m, cmd = resolvePick(m, c)
	return m, cmd
}

// TitleWrittenMsg is one finished retitle.
type TitleWrittenMsg struct {
	Gen    int
	Chart  store.NamedChart // the row as it was picked
	Result lookup.Retitled
	Err    error
}

// retitleCmd runs the write off the Update loop: two Astrolog spawns to read
// and one to write, and file moves, none of which belongs in a key handler.
func retitleCmd(gen int, dir string, c store.NamedChart, seen, title, place string) tea.Cmd {
	return func() tea.Msg {
		r, err := lookup.RetitleChart(context.Background(), dir, c, seen, title, place)
		return TitleWrittenMsg{Gen: gen, Chart: c, Result: r, Err: err}
	}
}

// applyTitleWritten folds a finished write back in. A failure keeps the form
// open, typed values intact, with the reason as its refusal. A success logs
// one line (C3.14) and anchors the chart under the title Astrolog recorded
// (C3.13), through the same resolve a pick uses.
func applyTitleWritten(m Model, msg TitleWrittenMsg) (Model, tea.Cmd) {
	m = m.settlePreservation(msg)
	if msg.Gen != m.titleGen || m.startup != startupTitleOpen {
		// A write Esc walked away from is logged once, as it would have been
		// with the form open, and changes nothing else (A8). Any other stale
		// result is dropped.
		var late bool
		if m.abandonedTitles, late = reclaim(m.abandonedTitles, msg.Gen); !late {
			return m, nil
		}
		if msg.Err != nil {
			return m.noteEphemeris(msg.Err).logError("Couldn't title that chart. %s", m.failureReason(msg.Err, firstLine(msg.Err.Error()))), nil
		}
		return m.logTitled(msg.Result), nil
	}
	m.title.saving = false
	if msg.Err != nil {
		switch {
		case errors.Is(msg.Err, store.ErrChartChanged):
			m.title.refusal = fmt.Sprintf("%s changed since the list was read — reopen the picker", msg.Chart.Source.File)
		default:
			m.title.refusal = firstLine(msg.Err.Error())
		}
		return m.noteEphemeris(msg.Err).logError("Couldn't title that chart. %s", m.failureReason(msg.Err, firstLine(msg.Err.Error()))), nil
	}

	r := msg.Result
	m = m.logTitled(r)

	if m.title.rename {
		var recompute tea.Cmd
		m, recompute = applyRenamed(m, r)
		return m, tea.Batch(loadChartsCmd(m.pickerGen), recompute)
	}

	// Anchor the row as the library lists it now, so the anchor and the
	// session carry what the file holds. Without a fresh row -- the library
	// could not list it, e.g. the atlas went missing -- nothing is anchored
	// from the old pick: the write is done and logged, and the picker reloads
	// to show what the library holds now.
	written := m.title.dir
	m.title = titleFormState{}
	m.startup = startupChartOpen
	if r.Row.Source.File == "" {
		m.pickerGen++
		m.picker.loaded = false
		return m, loadChartsCmd(m.pickerGen)
	}
	var cmd tea.Cmd
	m, cmd = resolvePickIn(m, written, r.Row)
	return m, cmd
}

// logTitled is the one line a finished title write leaves (C3.14), open or
// late.
func (m Model) logTitled(r lookup.Retitled) Model {
	kept := ""
	if r.Backup != "" {
		kept = " (original kept as " + r.Backup + ")"
	}
	return m.logNote("chart", "titled %s as %q%s", r.File, r.Name, kept)
}

// applyRenamed returns a finished rename to the picker: the filter cleared,
// the library reloaded, and the cursor on the renamed chart once it lands
// (C4.3). Nothing is anchored. When the renamed chart is the anchored one, the
// name it is shown and remembered under changes. A rename rewrites the title
// and the place LABEL, never the moment or the coordinates (C3.8 verifies
// both), so the anchored slot and its chart stay where they are and nothing
// is re-anchored (C4.6).
//
// A slot whose REPORT name changes is recomputed, and only that slot's
// reports: Astrolog's own header names the chart, so the Aspects and Details
// text made under the old name is stale (the specification N3,
// N13). A place-only rename, or one of a chart no slot holds, changes no report
// name and recomputes nothing.
func applyRenamed(m Model, r lookup.Retitled) (Model, tea.Cmd) {
	relabelled := false
	if m.slots != nil {
		// Every slot holding the renamed chart takes the new name; none of them
		// moves, because a rename never touches a moment or a coordinate.
		for _, slot := range m.slots.All() {
			if slot.Chart.Source.Kind == store.ChartKindFile && slot.Chart.Source.File == r.File && slot.Chart.ID == r.ID {
				slot.Chart.Name = r.Name
				if l := renamedLabel(slot, r); l != slot.Chart.Label {
					slot.Chart.Label = l
					relabelled = true
				}
			}
		}
	}
	m.title = titleFormState{}
	m.startup = startupChartOpen
	m.picker.resetFilter()
	m.picker.loaded = false
	m.picker.clear()
	m.picker.land = r.File
	m.pickerGen++
	if !relabelled {
		return m, nil
	}
	// The new header is at the top of the report: read it from there.
	m.details.scrollOffset = 0
	m = m.waitDetailsForText()
	return m.bumpIdentity(surfaceText)
}

// renamedLabel is the report label a slot carries after its chart was
// retitled: the new title with the place text the slot's own line shows. A
// chart whose file now carries no title sends none.
func renamedLabel(slot *store.Slot, r lookup.Retitled) store.ReportLabel {
	if r.Name == "" {
		return store.ReportLabel{}
	}
	return store.ReportLabel{Name: r.Name, Place: slot.Location.Display()}
}

// titleInfo is the form's one line beneath the rule: what it asks for, and
// nothing about files (operator, 2026-10-02). What the write did to which
// file — the rewrite, the backup, a YAML becoming a chart file — is the Log's line
// (C3.14), written once it has happened.
//
// With colour the Title field's placeholder already shows the chart's current
// name, so the line stays general. Without colour no placeholder is drawn, so
// the line names the file instead.
func titleInfo(c store.NamedChart, rename bool, cfg Config) []string {
	what := "a chart title"
	switch {
	case rename:
		what = "a new title or place"
	case !c.Untitled:
		what = "a place"
	}
	if cfg.useColor() {
		return []string{"Please enter " + what + "."}
	}
	return []string{"Please enter " + what + " for " + c.Source.File + "."}
}

// renderTitleForm draws the form.
func renderTitleForm(state titleFormState, cfg Config, termW, termH int) string {
	info := titleInfo(state.chart, state.rename, cfg)
	if state.confirming {
		info = confirmInfo(state)
	}
	// The box holds for both steps, whichever is showing: the second step's
	// text is sized with the directory it will name.
	later := state
	later.confirming = true
	if later.dir == "" {
		later.dir = store.ChartsDir()
	}
	alt := [][]string{titleInfo(state.chart, state.rename, cfg)}
	for _, held := range []bool{false, true} {
		later.backupHeld = held
		alt = append(alt, confirmInfo(later))
	}
	return renderForm(state.form, formView{
		Title:    popupTitleChartTitle,
		Info:     info,
		Alt:      alt,
		WrapInfo: true,
		Refusal:  state.refusal,
	}, cfg, termW, termH)
}

// confirmInfo is the second step's text: the file it will rewrite, the backup
// it keeps or finds, and that nothing is written yet. The form's own Info
// stays file-free; the file belongs to this step.
func confirmInfo(state titleFormState) []string {
	file := filepath.Base(state.chart.Source.File)
	folder := store.ChartBackupFolderDisplay(state.dir)
	sentence := "Saving this title will update " + file + " and keep the previous file in " + folder + "."
	if state.backupHeld {
		sentence = "Saving this title will update " + file + ". Its first original is already in " + folder + " and stays there."
	}
	return []string{sentence, "Press Enter to save the title, or Esc to go back."}
}
