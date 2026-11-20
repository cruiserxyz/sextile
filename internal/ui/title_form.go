package ui

import (
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/bubbles/key"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
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
//	    startupTitleOpen ──Enter with a title──▶ the folder's position decides
//	        protected ──▶ the form, with the folder's refusal
//	        ask, add  ──▶ the change question (nothing written yet)
//	            Yes ──▶ (write runs off-loop)
//	            No ──▶ the form, typed values intact
//	        allow     ──▶ (write runs off-loop)
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
//
// The typed fields, the refusal (the last submit was refused; the form stays
// open), the write in flight (only Esc acts until it lands) and the change
// question (Confirming) are popup.Form's.
type titleFormState struct {
	popup.Form
	chart  store.NamedChart // the row being titled, as the picker listed it
	seen   string           // the listing's fingerprint of its file
	rename bool             // opened by Ctrl+R: a write returns to the picker
	batch  bool             // asked after other ticked charts opened: Esc skips this one
	// proposed is the nearest atlas city the place name was filled with, for
	// a file that states no place.
	proposed string

	dir   string    // the charts directory captured when the form was submitted
	stamp time.Time // the dated copy's UTC stamp, fixed when the change question opens
	Lit   int       // the lit button of the change question
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
func newRenameForm(c store.NamedChart) dialog.Form {
	f := newTitleForm(c)
	f.Fields[0].Value = currentTitle(c)
	return f
}

// openRenameForm is Ctrl+R on c, whatever else its file holds: Astrolog
// writes back only the record, every change keeps a dated copy of the file
// whole, and Sextile's own settings travel on the argv, not in the file
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
	m.title = titleFormState{Form: popup.Form{Fields: newRenameForm(c)}, chart: c, seen: m.picker.seen[c.Source.File], rename: true}
	m.startup = startupTitleOpen
	return m.proposePlace()
}

// proposePlace fills the place name of a file that states none with the
// nearest atlas city to its coordinates. Whatever the operator then types is
// taken as it is.
func (m Model) proposePlace() Model {
	c := m.title.chart
	if c.PlaceLabel == "" && m.atlas.Index != nil {
		if p, ok := location.NearestIn(m.atlas.Index, c.Place.Lon, c.Place.Lat); ok {
			m.title.proposed = p.Display()
			m.title.Fields.Fields[titlePlaceField].Value = m.title.proposed
		}
	}
	return m
}

// noTitleRefusal is said when Enter has a place name typed and no title: a
// place name is written only with a title, so the typing is kept, not lost.
const noTitleRefusal = "Add a title to save the place name."

// emptyPlaceRefusal is C3.6b's sentence, as a required-field error: the chart
// states no place and none was typed, and Astrolog cannot be asked to write an
// empty one. It names no key: an error is not the place to explain the way
// out (operator, 2026-10-02), and an empty title already opens the chart
// untitled.
const emptyPlaceRefusal = "Add a place to save this title."

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

// The form's labels, padded so the values start in one column.
const (
	titleLabel  = "Title:       "
	placeLabel  = "Place name:  "
	coordsLabel = "Coordinates: "
)

// newTitleForm is the form for c: the Title field empty with the row's
// current name as its hint (no autofill, by the operator's decision), and the
// Place name field holding the file's own place label. The coordinates are a
// line of the form the operator reads and does not change (titleFixed).
func newTitleForm(c store.NamedChart) dialog.Form {
	return dialog.NewForm(dialog.Field{Label: titleLabel, Placeholder: c.Name}, dialog.Field{Label: placeLabel, Value: c.PlaceLabel})
}

// titleFixed is the form's coordinates line: the file's, which the form
// shows and never changes.
func titleFixed(c store.NamedChart) []dialog.FixedLine {
	return []dialog.FixedLine{{Label: coordsLabel, Value: lookup.CoordinateWords(c.Place.Lon, c.Place.Lat), Note: "from the file, not changed here"}}
}

// openTitleForm replaces the pick with the form. The picker's own state is
// left as it was, so a skip resumes the pick from exactly there.
//
// Opening it is a pick, so it bumps the pick generation: a resolve still in
// flight from an earlier pick must not land and anchor behind the form.
func openTitleForm(m Model, c store.NamedChart) Model {
	m.pickGen++
	m.title = titleFormState{Form: popup.Form{Fields: newTitleForm(c)}, chart: c, seen: m.picker.seen[c.Source.File]}
	m.startup = startupTitleOpen
	return m.proposePlace()
}

// chartTitleBindings is the Chart title popup's table. Esc goes back to the
// chart list, or, for a chart of a batch of ticked charts, opens it under the
// name the chart list shows;
// Enter with an empty title opens the chart without one.
func chartTitleBindings() []keys.BindingEntry {
	back := closeOn("Back")
	back.TitleFn = func(c keys.Context) string {
		if c.Picker.InBatch {
			return "Use the file name"
		}
		return "Back"
	}
	return keys.TitledLocal([]keys.BindingEntry{
		{ID: keys.BindingNextField, Binding: key.NewBinding(key.WithKeys("tab")), Title: "Next field", BarKey: "tab", Bar: true},
		{ID: keys.BindingPrevField, Binding: key.NewBinding(key.WithKeys("shift+tab")), Title: "Previous field", BarKey: "shift+tab"},
		confirm("Save title"),
		back,
	})
}

// titleConfirmBindings is the Chart title popup's change question: the arrows
// choose Yes or No, y changes the file, n and Esc go back to the form with
// nothing written.
func titleConfirmBindings() []keys.BindingEntry { return keys.TitledLocal(questionChange.bindings()) }

// updateTitleFormKey handles one keystroke. The caller in model.go has gated
// on m.startup == startupTitleOpen.
func updateTitleFormKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.title.Confirming {
		// The change question: Yes changes the file, No is back to the form
		// with the typed values, and nothing was written. A question asked
		// again gets a new stamp.
		var took string
		m.title.Lit, took = questionChange.key(msg, m.title.Lit)
		switch took {
		case "":
			return m, nil
		case buttonYes:
			m.title.Confirming = false
			// A folder protected since the question opened gets the refusal,
			// and no Astrolog process starts.
			if store.ReadPosition(m.title.dir).Effective == store.PositionProtected {
				m.title.stamp = time.Time{}
				m.title.Refusal = m.protectedRefusal(refusalChanges, m.titleDir())
				return m, nil
			}
			return startTitleWrite(m)
		}
		m.title.Confirming = false
		m.title.stamp = time.Time{}
		return m, nil
	}
	shut := m.protectedRefusal(refusalChanges, m.titleDir())
	id := keys.MatchBinding(msg, chartTitleBindings())
	if id == keys.BindingClose {
		return titleBack(m)
	}
	if m.title.Saving {
		// A write is in flight. Only Esc acts: the name on screen must stay
		// the name being written.
		return m, nil
	}
	switch id {
	case keys.BindingNextField:
		m.title.Apply(popup.NextField, msg)
		return m, nil
	case keys.BindingPrevField:
		m.title.Apply(popup.PrevField, msg)
		return m, nil
	case keys.BindingConfirm:
		return confirmTitle(m)
	}
	before := formValues(m.title.Form)
	m.title.Apply(popup.Type, msg)
	typedClears(&m.title.Form, shut, before)
	return m, nil
}

// confirmTitle is Enter on the form. An empty title skips; a refusal stays in
// the form; a filled form goes by the folder's position: protected refuses,
// ask and add open the change question, which names the file, what changes
// and the dated copy, and writes nothing, and allow writes at once.
func confirmTitle(m Model) (tea.Model, tea.Cmd) {
	title := strings.TrimSpace(m.title.Fields.Value(0))
	place := strings.TrimSpace(m.title.Fields.Value(1))
	if title == "" {
		if place != "" && place != m.title.proposed && place != m.title.chart.PlaceLabel {
			m.title.Refusal = noTitleRefusal
			m.title.Fields.Focus = 0
			return m, nil
		}
		return skipTitle(m)
	}
	if place == "" {
		place = m.title.chart.PlaceLabel
	}
	if m.title.rename && title == currentTitle(m.title.chart) && place == m.title.chart.PlaceLabel {
		// Nothing to rename: write nothing (C4.4).
		return titleBack(m)
	}
	if place == "" {
		m.title.Refusal = emptyPlaceRefusal
		m.title.Fields.Focus = titlePlaceField
		return m, nil
	}
	for _, l := range []struct {
		what  string
		s     string
		field int
	}{{"title", title, 0}, {"place", place, titlePlaceField}} {
		if err := lookup.CheckLabelLength(l.what, l.s); err != nil {
			m.title.Refusal = err.Error()
			m.title.Fields.Focus = l.field
			return m, nil
		}
	}
	m.title.Refusal = ""
	// The directory is the one the picker listed the chart from: the question
	// and the change are about that folder, whatever the resolver answers now.
	dir := m.titleDir()
	// Claimed first, read after: the position decided on is current.
	var refused string
	if m, refused = m.claimFolder(dir); refused != "" {
		m.title.Refusal = refused
		return m, nil
	}
	p := store.ReadPosition(dir).Effective
	if p == store.PositionProtected {
		m.title.Refusal = m.protectedRefusal(refusalChanges, m.titleDir())
		return m, nil
	}
	switch {
	case titleAsksFirst(p):
		m.title.dir, m.title.stamp, m.title.Confirming = dir, time.Now().UTC(), true
		m.title.Lit = questionChange.Open
		return m, nil
	}
	m.title.dir = dir
	return startTitleWrite(m)
}

// startTitleWrite hands the typed values to the write, in the directory
// captured when the form was submitted. The checks ran before the second
// step; the values are read again from the same unchanged form.
func startTitleWrite(m Model) (tea.Model, tea.Cmd) {
	title := strings.TrimSpace(m.title.Fields.Value(0))
	place := strings.TrimSpace(m.title.Fields.Value(1))
	if place == "" {
		place = m.title.chart.PlaceLabel
	}
	m.title.Confirming = false
	m.title.Saving = true
	m.titleGen++
	if m.title.stamp.IsZero() {
		m.title.stamp = time.Now().UTC()
	}
	return m, renameCmd(m.titleGen, m.title.dir, m.title.chart, m.title.seen, title, place, m.title.stamp)
}

// titleBack is Esc: the chart picker comes back exactly as the pick left it,
// filter and cursor, and nothing is resolved. A write already handed to
// Astrolog cannot be called back, so it finishes; bumping the generation is
// what keeps its result from anchoring a chart the operator walked away from.
// In a batch of ticked charts Esc opens the chart under its name in the chart
// list, as an empty title does, and the batch goes on.
func titleBack(m Model) (tea.Model, tea.Cmd) {
	if m.title.batch && !m.title.Saving {
		return skipTitle(m)
	}
	if m.title.Saving {
		// Its result is still logged when it lands (A8).
		m.abandonedTitles = abandon(m.abandonedTitles, m.titleGen)
	}
	m.titleGen++
	batch := m.title.batch
	m.title = titleFormState{}
	m.startup = startupChartOpen
	if batch {
		var next tea.Cmd
		var more bool
		if m, next, more = nextTicked(m); !more {
			m.startup = m.picker.prior
		}
		return m, next
	}
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
// titleDir is the charts folder of the chart in the title form: the folder
// the picker listed it from.
func (m Model) titleDir() string {
	if m.picker.dir != "" {
		return m.picker.dir
	}
	return store.ChartsDir()
}

func retitleCmd(gen int, dir string, c store.NamedChart, seen, title, place string, stamp time.Time) tea.Cmd {
	return func() tea.Msg {
		r, err := lookup.RetitleChart(context.Background(), dir, c, seen, title, place, stamp)
		return TitleWrittenMsg{Gen: gen, Chart: c, Result: r, Err: err}
	}
}

// applyTitleWritten folds a finished write back in. A failure keeps the form
// open, typed values intact, with the reason as its refusal. A success logs
// one line (C3.14) and anchors the chart under the title Astrolog recorded
// (C3.13), through the same resolve a pick uses.
func applyTitleWritten(m Model, msg TitleWrittenMsg) (Model, tea.Cmd) {
	if msg.Gen != m.titleGen || m.startup != startupTitleOpen {
		// A write Esc walked away from is logged once, as it would have been
		// with the form open, and changes nothing else (A8). Any other stale
		// result is dropped.
		var late bool
		if m.abandonedTitles, late = reclaim(m.abandonedTitles, msg.Gen); !late {
			return m, nil
		}
		if msg.Err != nil {
			return m.noteEphemeris(msg.Err).logError("chart", "chart not titled, %s", m.failureReason(msg.Err, firstLine(msg.Err.Error()))).logUncorrected(msg.Err, filepath.Base(msg.Chart.Source.File)), nil
		}
		return m.logTitled(msg.Result), nil
	}
	m.title.Saving = false
	if msg.Err != nil {
		switch {
		case errors.Is(msg.Err, store.ErrChartChanged):
			m.title.Refusal = fmt.Sprintf("%s changed since the list was read — reopen the picker", msg.Chart.Source.File)
		default:
			m.title.Refusal = firstLine(msg.Err.Error())
		}
		return m.noteEphemeris(msg.Err).logError("chart", "chart not titled, %s", m.failureReason(msg.Err, firstLine(msg.Err.Error()))).logUncorrected(msg.Err, filepath.Base(msg.Chart.Source.File)), nil
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
	return m.logNote("chart", "%s titled as %q%s", r.File, r.Name, kept)
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
				if r.Record != "" && r.Record != slot.Chart.Record {
					slot.Chart.Record, slot.Chart.Clock = r.Record, r.Clock
					relabelled = true
				}
			}
		}
	}
	m.title = titleFormState{}
	m.startup = startupChartOpen
	m.picker.ResetFilter()
	m.picker.loaded = false
	m.picker.Clear()
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
	if cfg.UseColor() {
		return []string{"Please enter " + what + "."}
	}
	return []string{"Please enter " + what + " for " + c.Source.File + "."}
}

// renderTitleForm draws the form.
func renderTitleForm(state titleFormState, cfg Config, termW, termH int) string {
	info := titleInfo(state.chart, state.rename, cfg)
	if place := strings.TrimSpace(state.Fields.Value(titlePlaceField)); state.proposed != "" && place == state.proposed {
		info = []string{"The nearest city is proposed as the place name."}
	}
	return state.Render(dialog.FormView{
		Title:    dialog.TitleNameChart,
		Fixed:    titleFixed(state.chart),
		Info:     info,
		WrapInfo: true,
	}, cfg, termW, termH, popupChrome(cfg))
}
