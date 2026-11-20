package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
)

// originals_ui.go is the screen half of the charts folder's door
// (store/originals.go): what the save prompt and the title form ask before a
// write, the refusal at the protected position, and the folder row of Keybindings. The door itself decides nothing here: it refuses at protected, keeps
// the dated copy and writes the journal.

// What the refusal says it would allow.
const (
	refusalSaving  = "saving"
	refusalChanges = "changes"
)

// protectedRefusal is the refusal of a protected folder, as the save prompt
// (saving) or the title form (changes) shows it under its own lines. In a run
// the launcher started read-only, the folder row cannot open the folder for
// this run, so the refusal says what a new start needs instead.
func (m Model) protectedRefusal(what, dir string) string {
	r := store.ReadPosition(dir)
	switch {
	case r.ReadonlyEnv && r.Stored == store.PositionProtected:
		return "This charts folder is protected.\nNothing was written.\n" +
			"This run only reads it. To save here, press ?\nand run Protect the charts folder, then start Sextile again."
	case r.ReadonlyEnv:
		return "This charts folder is read-only for this run.\nNothing was written.\n" +
			"A new start is needed to save here."
	}
	return "This charts folder is protected.\nNothing was written.\n" +
		"To allow " + what + " here, press ? and run\nProtect the charts folder."
}

// originalsLine is one line of the folder row or of the launch: on the Log, and in the
// key bar for a few seconds where the screen has no Log panel. text is the
// message, as the Folder group says it; the key bar names the folder before it.
func (m Model) originalsLine(text string) (Model, tea.Cmd) {
	m = m.logNote("charts folder", "%s", text)
	if m.logOnScreen() {
		return m, nil
	}
	return m.showNotice(noticeFitting("charts folder: "+text, m.width))
}

// folderWords is a refusal of the store as a Folder message: lower case at its
// start, its sentences joined by commas and no closing full stop.
func folderWords(s string) string {
	parts := strings.Split(strings.TrimSuffix(s, "."), ". ")
	for i, p := range parts {
		if r, n := utf8.DecodeRuneInString(p); n > 0 {
			parts[i] = string(unicode.ToLower(r)) + p[n:]
		}
	}
	return strings.Join(parts, ", ")
}

// noticeFitting is the line as the key bar can show it whole, which leaves the
// bar three cells. A line wider than that gives up its later sentences, and the
// Log holds the whole line: a notice is never cut in the middle of a sentence.
func noticeFitting(text string, width int) string {
	room := width - 3
	if ansi.StringWidth(text) <= room {
		return text
	}
	for _, sep := range []string{". ", " · "} {
		if first, _, ok := strings.Cut(text, sep); ok {
			if sep == ". " {
				first += "."
			}
			return first
		}
	}
	return text
}

// isOriginalsNotice reports whether a footer indicator is a line of the folder row or
// of the launch.
func isOriginalsNotice(s string) bool {
	return strings.HasPrefix(s, "charts folder")
}

// pressOriginals is the folder row of Keybindings: it protects the charts folder or opens it
// again, once. dir is the charts folder the press is about.
func (m Model) pressOriginals(dir string) (Model, tea.Cmd) {
	if m.blocked(keys.BindingOriginals) != "" {
		return m, nil
	}
	r := store.ReadPosition(dir)
	if r.Fault == "" && !(r.HasLine && r.Stored == store.PositionAllow) {
		// The press changes the line: the folder is claimed before its
		// position is read again, so the line changed is the current one.
		var refused string
		if m, refused = m.claimFolder(dir); refused != "" {
			return m.originalsLine(folderWords(refused))
		}
		r = store.ReadPosition(dir)
	}
	name := store.PositionFileName()
	setTo := func(p store.Position, line string) (Model, tea.Cmd) {
		if err := store.SetPosition(dir, p); err != nil {
			if errors.Is(err, store.ErrFolderBusy) {
				return m.originalsLine(folderWords(err.Error()))
			}
			m = m.logTrace("charts folder position", "%v", err)
			return m.originalsLine("whether Sextile may write here was not changed")
		}
		return m.originalsLine(line)
	}
	switch {
	case r.Fault != "":
		m = m.logTrace("charts folder position", "%s", store.PositionFile())
		return m.originalsLine("protected, " + name + " " + r.Fault)
	case r.HasLine && r.Stored == store.PositionProtected:
		if r.ReadonlyEnv {
			return setTo(store.PositionAsk, "asks first from the next start, writes nothing now")
		}
		return setTo(store.PositionAsk, "Sextile asks before each write")
	case r.HasLine && r.Stored == store.PositionAllow:
		m = m.logTrace("charts folder position", "%s", store.PositionFile())
		return m.originalsLine("no question before a write, set in " + name)
	}
	return setTo(store.PositionProtected, "protected, Sextile writes nothing here")
}

// typedClears ends the folder's refusal once the field has changed.
func typedClears(f *popup.Form, shut string, before string) {
	if f.Refusal == shut && formValues(*f) != before {
		f.Refusal = ""
	}
}

// formValues is the form's typed text, for typedClears to compare.
func formValues(f popup.Form) string { return f.Fields.Value(0) + "\x00" + f.Fields.Value(1) }

// logUncorrected adds the line O16a asks for when a write failed after its
// journal line and the line saying so could not be appended.
func (m Model) logUncorrected(err error, file string) Model {
	if !errors.Is(err, store.ErrJournalUncorrected) {
		return m
	}
	return m.logNote("journal", "its last line names a write to %s that did not happen", file)
}

// reportOriginalsAtStart is what the launch says about the charts folder,
// once: leftover staging files a crash left (the Log), and a position file the
// door cannot trust (the large log only).
func (m Model) reportOriginalsAtStart() (Model, tea.Cmd) {
	dir := store.ChartsDir()
	if r := store.ReadPosition(dir); r.Fault != "" {
		m = m.logTrace("charts folder position", "%s %s (%s)", store.PositionFileName(), r.Fault, store.PositionFile())
	}
	// The launcher's word, recorded before the first frame, may have been the
	// folder's first claim.
	if n := store.FolderLockNote(dir); n != "" {
		m = m.logTrace("charts folder", "%s", n)
	}
	names, err := store.LeftoverTemps(dir)
	if err != nil || len(names) == 0 {
		return m, nil
	}
	line := "a crash left " + names[0]
	if len(names) > 1 || ansi.StringWidth(line) > logLineCells {
		line = "a crash left " + strconv.Itoa(len(names)) + " .sextile-tmp files"
		if len(names) == 1 {
			line = "a crash left 1 file ending in .sextile-tmp"
		}
	}
	return m.originalsLine(line)
}

// logLineCells is the width a Log line may take.
const logLineCells = 76

// ---- the save prompt's first-write question -------------------------------

// The Charts folder's two questions' buttons, drawn in table order.
var (
	firstWriteButtons = questionFirstWrite.Buttons
	changeButtons     = questionChange.Buttons
)

// saveAskBindings is the first-write question's table (O7, O8).
func saveAskBindings() []keys.BindingEntry { return keys.TitledLocal(questionFirstWrite.bindings()) }

// slugFile is the file name the typed name would become, "" when it has none.
func slugFile(typed string) string {
	slug := store.Slugify(typed)
	if slug == "" {
		return ""
	}
	return slug + store.ChartFileExt
}

// firstWriteChoice is the question before a new chart in a folder at `ask`,
// asked before every new chart until Always. The folder is whole on its own line
// and wraps at a slash when it is long.
func firstWriteChoice(state saveModalState) dialog.Choice {
	return dialog.Choice{
		Body: []string{
			"Sextile asks before it writes a new chart in",
			state.dest,
			"Create " + slugFile(state.typedName()) + " there?",
		},
		Buttons: firstWriteButtons,
		Lit:     state.Lit,
	}
}

// changeChoice is the change question (V3) with its buttons.
func changeChoice(state titleFormState, cfg Config) dialog.Choice {
	return dialog.Choice{Body: changeInfo(state, cfg), Buttons: changeButtons, Lit: state.Lit}
}

// claimFolder takes the charts folder's writing lock before a question or a
// write: the first window to write a folder keeps it until it exits. It
// returns the refusal to show, "" when this window holds the folder.
func (m Model) claimFolder(dir string) (Model, string) {
	err := store.ClaimFolder(dir)
	if n := store.FolderLockNote(dir); n != "" {
		m = m.logTrace("charts folder", "%s", n)
	}
	if err != nil {
		return m, err.Error()
	}
	return m, ""
}

// confirmSave is Enter on a name that passed the checks of what was typed: the
// folder's position decides between a refusal, a question and the save.
func confirmSave(m Model) (tea.Model, tea.Cmd) {
	dir := m.save.dest
	// The folder is claimed first and its position read after, so the
	// position decided on is the one no other window can change meanwhile.
	var refused string
	if m, refused = m.claimFolder(dir); refused != "" {
		m.save.Refusal = refused
		return m, nil
	}
	r := store.ReadPosition(dir)
	if r.Effective == store.PositionProtected {
		m.save.Refusal = m.protectedRefusal(refusalSaving, m.save.dest)
		return m, nil
	}
	switch {
	case r.Effective == store.PositionAsk && slugFile(m.save.typedName()) != "" && dirExists(dir):
		m.save.Refusal = ""
		m.save.Confirming, m.save.Lit = true, questionFirstWrite.Open
		return m, nil
	}
	return startSave(m)
}

// dirExists reports whether dir is there to be written in. A folder that
// cannot be looked at counts as there: the door refuses what it must.
func dirExists(dir string) bool {
	_, err := os.Stat(dir)
	return !errors.Is(err, os.ErrNotExist)
}

// answerFirstWrite is a key pressed while the first-write question shows: Yes
// saves this once, Always saves and remembers the folder as add, No goes back
// to the name. Any other key does nothing.
//
// The position is read again at the answer: a folder protected since the
// question opened gets the refusal and no Astrolog process, and Always writes
// add only over the ask the question was asked at, never over a newer word.
func answerFirstWrite(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var took string
	m.save.Lit, took = questionFirstWrite.key(msg, m.save.Lit)
	if took == "" {
		return m, nil
	}
	m.save.Confirming = false
	if took == buttonNo {
		return m, nil
	}
	ans := keys.BindingConfirm
	if took == buttonAlways {
		ans = keys.BindingToggle
	}
	r := store.ReadPosition(m.save.dest)
	if r.Effective == store.PositionProtected {
		m.save.Refusal = m.protectedRefusal(refusalSaving, m.save.dest)
		return m, nil
	}
	switch {
	case ans == keys.BindingConfirm, r.Effective != store.PositionAsk:
		return startSave(m)
	default:
		if err := store.SetPosition(m.save.dest, store.PositionAdd); err != nil {
			if errors.Is(err, store.ErrFolderBusy) {
				m.save.Refusal = err.Error()
				return m, nil
			}
			m = m.logTrace("charts folder position", "%v", err)
			m, cmd := m.originalsLine("whether Sextile may write here was not changed")
			return m, cmd
		}
		return startSave(m)
	}
}

// ---- the title form's change question -------------------------------------

// titleAsksFirst is true when the folder wants the change question before a
// rename: ask and add. Protected has its refusal and allow asks nothing.
func titleAsksFirst(p store.Position) bool {
	return p == store.PositionAsk || p == store.PositionAdd
}

// changeInfo is the change question (V3): the file, what changes and the dated
// copy that is kept.
func changeInfo(state titleFormState, cfg Config) []string {
	arrow := "→"
	if cfg.UseASCII() {
		arrow = "->"
	}
	file := filepath.Base(state.chart.Source.File)
	title := strings.TrimSpace(state.Fields.Value(0))
	place := strings.TrimSpace(state.Fields.Value(1))
	if place == "" {
		place = state.chart.PlaceLabel
	}
	was := func(s, none string) string {
		if s == "" {
			return none
		}
		return s
	}
	out := []string{"Change " + file + "?", ""}
	field := func(label, old, now string) {
		if old == now {
			out = append(out, "  "+label+"   "+old+"  (unchanged)")
			return
		}
		out = append(out, "  "+label+"   "+old, "       "+arrow+"  "+now)
	}
	field("title", was(currentTitle(state.chart), "(no title)"), title)
	field("place", was(state.chart.PlaceLabel, "(no place)"), place)
	stamp := state.stamp
	if stamp.IsZero() {
		stamp = time.Now().UTC()
	}
	return append(out, "", "A copy of the file as it is now is kept:", "  "+store.BackupName(file, stamp))
}
