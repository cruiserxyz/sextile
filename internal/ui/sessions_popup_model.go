package ui

import (
	"errors"
	"github.com/charmbracelet/bubbles/key"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// sessions_popup_model.go is the Model's half of the Sessions list (`, O`; the popup
// itself is internal/ui/popup/sessions.go, and this file keeps the store calls): the first row saves the
// open workspace under a name, the second starts a new, unnamed session, and
// the rows below are the named sessions saved in this charts folder, newest first. It moves like the chart list: ↑↓, Enter,
// Ctrl+R to rename, Esc to close; Ctrl+D moves a session to the trash.
//
// A window runs exactly ONE session and keeps updating only that one: the
// general session (session.go, as before), or the named session it saved or
// opened here. Every write that went to the general session goes to the named
// one instead (saveRunningSession), and the window holds that session's lock,
// so a session open in another window is marked as such and is not opened,
// renamed or trashed from this one.
//
//	main ──, O──▶ startupSessionsOpen ──Enter on row 1──▶ startupSessionNameOpen (Save as)
//	                   │                ──Enter on row 2──▶ a new session (an unnamed one holding
//	                   │                   charts asks first: Yes opens Save as, saved then new)
//	                   │                ──Ctrl+R on a session──▶ startupSessionNameOpen (rename)
//	                   │                ──Enter on a session──▶ the session opens (NamedSessionLoadedMsg)
//	                   └─Esc──▶ prior state
//	startupSessionNameOpen ──Esc──▶ the list; Enter──▶ saved (main) / renamed (the list) / refused (stays)

// namedRun is the named session a window runs. Its zero value is the general
// session.
type namedRun struct {
	file string // base name in store.SessionsDir()
	name string // the name it was saved under
	lock *store.SessionLock
	// pending is set from the moment a chosen session's lock is taken until its
	// charts replace the open ones: nothing is written meanwhile, or the old
	// workspace would be saved into the new session's file.
	pending bool
	// kept is set when the launch's read of the session --session opened was
	// refused: the file is left exactly as it is, and this window writes no
	// session until it saves or opens another, as with a refused general
	// session it could not move aside.
	kept bool
}

func (r namedRun) running() bool { return r.lock != nil }

// sessionsPopupState is the Sessions list and its name field: the popup's own
// state, plus where to return to.
type sessionsPopupState struct {
	popup.Sessions
	prior startupState
}

// openSessionsList is `, O`.
func openSessionsList(m Model) (tea.Model, tea.Cmd) {
	if m.blocked(keys.BindingSessions) != "" {
		return m, nil
	}
	m.sessions = sessionsPopupState{prior: m.startup}
	m = m.reloadSessions("")
	m.sessions.Fit = sessionsFit(m)
	m.startup = startupSessionsOpen
	return m, nil
}

// reloadSessions reads the list again and puts the cursor on file, when it is
// listed; else it keeps the cursor's row, clamped.
func (m Model) reloadSessions(file string) Model {
	rows, err := store.ListNamedSessions(store.FolderID(store.ChartsDir()), m.named.file)
	m.sessions.Rows, m.sessions.ListErr = rows, err
	if file != "" {
		for i, r := range rows {
			if r.File == file {
				m.sessions.Cursor = i + 2
			}
		}
	}
	m.sessions.Cursor = min(max(m.sessions.Cursor, 0), len(rows)+1)
	return m
}

// sessionRow is the saved session under the cursor, or false on an action row.
func (m Model) sessionRow() (store.NamedSession, bool) { return m.sessions.RowUnder() }

// The conditions the Sessions list's bar shows its keys under.
func (m Model) sessionsOnSaveRow() bool { return m.sessions.Cursor == 0 }

func (m Model) sessionsOnNewRow() bool { return m.sessions.OnNewRow() }

func (m Model) sessionsOnFreeRow() bool {
	r, ok := m.sessionRow()
	return ok && !r.OpenElsewhere
}

func (m Model) sessionsCanOpen() bool {
	r, ok := m.sessionRow()
	return ok && !r.OpenElsewhere && r.File != m.named.file
}

func (m Model) sessionsCanTrash() bool {
	r, ok := m.sessionRow()
	return ok && !r.OpenElsewhere && r.File != m.named.file
}

func (m Model) sessionNameTyped() bool { return m.sessions.NameTyped() }

// sessionsVisible is how many rows of the list one page holds.
func (m Model) sessionsVisible() int { return dialog.ListHeight(m.height) + 2 }

// sessionsBindings is the Sessions list's table: the chart list's moves. Enter
// saves on the first row, starts a new session on the second and opens a
// session on the others; Ctrl+R renames and
// Ctrl+D trashes the highlighted session. Nothing in it types.
func sessionsBindings() []keys.BindingEntry {
	return keys.TitledLocal(append([]keys.BindingEntry{
		listUp().ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 1 }), listDown(),
		confirm("Save as").ShownWhen(func(c keys.Context) bool { return c.Sessions.OnSaveRow }),
		confirm("New").ShownWhen(func(c keys.Context) bool { return c.Sessions.OnNewRow }),
		confirm("Open").ShownWhen(func(c keys.Context) bool { return c.Sessions.CanOpen }),
		{ID: keys.BindingRename, Binding: key.NewBinding(key.WithKeys("ctrl+r")), Title: "Rename", BarKey: "ctrl+r", Bar: true, When: func(c keys.Context) bool { return c.Sessions.OnFreeRow }},
		{ID: keys.BindingSessionDelete, Binding: key.NewBinding(key.WithKeys("ctrl+d")), Title: "Delete", BarKey: "ctrl+d", Bar: true, When: func(c keys.Context) bool { return c.Sessions.CanTrash }},
		closeOn("Close"),
	}, pageEntries()...))
}

// newSessionAskBindings is the table of the question asked before a new
// session starts.
func newSessionAskBindings() []keys.BindingEntry {
	return keys.TitledLocal(questionNewSession.bindings())
}

// deleteAskBindings is the table of the question asked before a session goes
// to the trash.
func deleteAskBindings() []keys.BindingEntry { return keys.TitledLocal(questionDelete.bindings()) }

// sessionAskBindings is the table of the question the list is asking.
func (m Model) sessionAskBindings() []keys.BindingEntry {
	if m.sessions.Deleting != "" {
		return deleteAskBindings()
	}
	return newSessionAskBindings()
}

// newSessionChoice is the question "Save this session first?" in front of a way
// out of a session that is not written when left: the unnamed one, or a named
// one whose file was left as it was.
func newSessionChoice(m Model) dialog.Choice {
	n := m.slots.AnchoredCount()
	title := map[popup.Exit]string{popup.ExitNew: dialog.TitleNewSession, popup.ExitOpen: dialog.TitleOpenSession, popup.ExitQuit: dialog.TitleQuit}[m.sessions.Exit]
	verb, noun := "are", "charts"
	if n == 1 {
		verb, noun = "is", "chart"
	}
	var second string
	if m.named.running() {
		second = "Sextile left " + m.named.name + "'s file as it was, so its " + strconv.Itoa(n) + " " + noun + " " + verb + " in no file."
	} else {
		rest := "No forgets which were open. Their files stay."
		if n == 1 {
			rest = "No forgets which one was open. Its file stays."
		}
		second = "Its " + chartCount(n) + " " + verb + " open under no name. " + rest
	}
	return dialog.Choice{
		Title:   title,
		Body:    []string{"Save this session first?", second},
		Buttons: questionNewSession.Buttons,
		Lit:     m.sessions.Lit,
	}
}

// deleteChoice is the question "Move <session> to the trash?" for the session
// the delete key was pressed on.
func deleteChoice(m Model) dialog.Choice {
	name := m.sessions.Deleting
	for _, r := range m.sessions.Rows {
		if r.File == m.sessions.Deleting {
			name = r.Name
		}
	}
	return dialog.Choice{
		Title:   dialog.TitleDeleteSession,
		Body:    []string{"Move " + name + " to the trash?", "Sextile cannot bring it back."},
		Buttons: questionDelete.Buttons,
		Lit:     m.sessions.Lit,
	}
}

// sessionNameBindings is the Sessions name field's table: Enter saves (or
// renames), Esc goes back to the list.
func sessionNameBindings() []keys.BindingEntry {
	return keys.TitledLocal([]keys.BindingEntry{confirm("Save").ShownWhen(func(c keys.Context) bool { return c.Sessions.NameTyped }), closeOn("Back")})
}

// updateSessionsKey handles one keystroke while the list has the keyboard: it
// resolves the key through the bindings table to the popup's Action, applies it,
// and acts on what the popup reports.
func updateSessionsKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.sessions.Asking {
		if m.sessions.Deleting != "" {
			return answerDelete(m, msg)
		}
		return answerNewSession(m, msg)
	}
	a := popup.Type
	switch keys.MatchBinding(msg, sessionsBindings()) {
	case keys.BindingClose:
		a = popup.Dismiss
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
	case keys.BindingConfirm:
		a = popup.Confirm
	case keys.BindingRename:
		a = popup.Rename
	case keys.BindingSessionDelete:
		a = popup.Trash
	}
	out := m.sessions.Apply(a, m.sessionsVisible())
	r := out.Row
	switch out.Action {
	case popup.Dismiss:
		m.startup = m.sessions.prior
		m.sessions = sessionsPopupState{}
		return m, nil
	case popup.Confirm:
		if m.sessions.OnNewRow() {
			return m.newSessionRow()
		}
		if !out.OnRow {
			return m.openSaveAs(false), nil
		}
		return m.openNamedSession(r)
	case popup.Rename:
		if !out.OnRow {
			return m, nil
		}
		if r.OpenElsewhere {
			return m.logNote("session", "%s is open in another window", logIdent(r.Name, m.cfg)), nil
		}
		m.sessions.Form = dialog.NewForm(dialog.Field{Label: "Name: ", Value: r.Name})
		m.sessions.Renaming, m.sessions.Refusal = r.File, ""
		m.startup = startupSessionNameOpen
		return m, nil
	case popup.Trash:
		if !out.OnRow {
			return m, nil
		}
		return m.askTrash(r), nil
	}
	return m, nil
}

// openSaveAs opens the list's name field to save the session this window runs.
// thenExit says the way out the question stood in front of goes on once the
// save succeeds.
func (m Model) openSaveAs(thenExit bool) Model {
	m.sessions.Form = dialog.NewForm(dialog.Field{Label: "Name: ", Value: m.sessionPrefill()})
	m.sessions.Renaming, m.sessions.Refusal = "", ""
	m.sessions.ThenExit = thenExit
	m.startup = startupSessionNameOpen
	return m
}

// newSessionRow is Enter on the New row. An unnamed session holding a chart
// is asked about first; every other session is left at once (a named one is
// saved already).
func (m Model) newSessionRow() (tea.Model, tea.Cmd) {
	if m.named.pending {
		return m, nil
	}
	if m.asksBeforeLeaving(popup.ExitNew) {
		return m.askLeave(popup.ExitNew, ""), nil
	}
	return m.leaveForNewSession()
}

// leaveForNewSession closes the list and starts the new, unnamed session. A
// session with nothing in it only says so in the Log.
func (m Model) leaveForNewSession() (tea.Model, tea.Cmd) {
	m.startup = m.sessions.prior
	m.sessions = sessionsPopupState{}
	switch m.blocked(keys.BindingNewSession) {
	case "":
		return startNewSession(m)
	case whySessionEmpty:
		return m.logNote("session", "new session with no name"), nil
	}
	return m, nil
}

// answerNewSession is a key pressed while the question shows. Yes opens the
// name field, No starts the new session and closes the open charts, Cancel and
// Esc go back to the list.
func answerNewSession(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var took string
	m.sessions.Lit, took = questionNewSession.key(msg, m.sessions.Lit)
	switch took {
	case buttonYes:
		m.sessions.Asking = false
		return m.openSaveAs(true), nil
	case buttonNo:
		return m.goOn(m.sessions.Exit, m.sessions.Target, false)
	case buttonCancel:
		m.sessions.Asking = false
		if m.sessions.Exit == popup.ExitQuit {
			m.startup = m.sessions.prior
			m.sessions = sessionsPopupState{}
		}
	}
	return m, nil
}

// asksBeforeLeaving says a way out is stood in front of by the question "Save
// this session first?": the session has a chart and is not written when left
// (a named one whose file was left as it was), or is the unnamed one and the
// way out is a new session (the unnamed session is saved by the other two).
func (m Model) asksBeforeLeaving(exit popup.Exit) bool {
	if m.slots == nil || m.slots.AnchoredCount() == 0 || m.named.pending {
		return false
	}
	if m.named.running() {
		return !m.savedWhenLeft()
	}
	return exit == popup.ExitNew
}

// askLeave opens the question in front of a way out. Quit has no list under it:
// the question stands over the screen the operator was on.
func (m Model) askLeave(exit popup.Exit, target string) Model {
	if exit == popup.ExitQuit {
		m.sessions = sessionsPopupState{prior: m.startup}
		m.startup = startupSessionsOpen
	}
	m.sessions.Asking, m.sessions.Lit = true, questionNewSession.Open
	m.sessions.Exit, m.sessions.Target = exit, target
	return m
}

// goOn is the way out itself, once the question was answered. saved says the
// running session was written a moment ago, so leaving does not write it again.
func (m Model) goOn(exit popup.Exit, target string, saved bool) (tea.Model, tea.Cmd) {
	m.sessions.Asking = false
	switch exit {
	case popup.ExitOpen:
		for _, r := range m.sessions.Rows {
			if r.File == target {
				return m.openNamedSessionAfter(r, true, saved)
			}
		}
		return m, nil
	case popup.ExitQuit:
		m.startup = m.sessions.prior
		m.sessions = sessionsPopupState{}
		return m.quit()
	}
	if saved {
		m.startup = m.sessions.prior
		m.sessions = sessionsPopupState{}
		return startNewSessionAfter(m, true)
	}
	return m.leaveForNewSession()
}

// quitOrAsk is the quit key: a session that is not written when left is asked
// about first; any other quits as it always did. With the Sessions list or one
// of its questions on screen the key keeps its plain meaning.
func (m Model) quitOrAsk() (tea.Model, tea.Cmd) {
	inSessions := m.sessions.Asking || m.startup == startupSessionsOpen || m.startup == startupSessionNameOpen
	if !inSessions && m.asksBeforeLeaving(popup.ExitQuit) {
		return m.askLeave(popup.ExitQuit, ""), nil
	}
	return m.quit()
}

// askTrash is Ctrl+D: a session that cannot be trashed is refused as before;
// any other is asked about first, with No lit.
func (m Model) askTrash(r store.NamedSession) Model {
	switch {
	case r.File == m.named.file:
		return m.logNote("session", "%s runs in this window. Switch to another session first", logIdent(r.Name, m.cfg))
	case r.OpenElsewhere:
		return m.logNote("session", "%s is open in another window", logIdent(r.Name, m.cfg))
	}
	m.sessions.Asking, m.sessions.Deleting, m.sessions.Lit = true, r.File, questionDelete.Open
	return m
}

// answerDelete is a key pressed while the delete question shows: Yes moves the
// session to the trash, No and Esc leave the list as it was.
func answerDelete(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var took string
	m.sessions.Lit, took = questionDelete.key(msg, m.sessions.Lit)
	if took == "" {
		return m, nil
	}
	file := m.sessions.Deleting
	m.sessions.Asking, m.sessions.Deleting = false, ""
	if took != buttonYes {
		return m, nil
	}
	for _, r := range m.sessions.Rows {
		if r.File == file {
			return m.trashNamedSession(r), nil
		}
	}
	return m, nil
}

// trashNamedSession moves the session to the trash, once its question was
// answered Yes, and one Log line says where it went.
func (m Model) trashNamedSession(r store.NamedSession) Model {
	switch {
	case r.File == m.named.file:
		return m.logNote("session", "%s runs in this window", logIdent(r.Name, m.cfg))
	case r.OpenElsewhere:
		return m.logNote("session", "%s is open in another window", logIdent(r.Name, m.cfg))
	}
	path, err := store.TrashNamedSession(r.File, m.named.lock, m.clock.Now())
	switch {
	case errors.Is(err, store.ErrSessionOpenElsewhere):
		m = m.logNote("session", "%s is open in another window", logIdent(r.Name, m.cfg))
	case err != nil:
		m = m.logTrace("session", "trash %s: %v", r.Name, err).logNote("session", "%s was not moved to the trash", logIdent(r.Name, m.cfg))
	default:
		m = m.logNote("session", "%s moved to %s", logIdent(r.Name, m.cfg), store.NamedSessionDisplay(path))
		m = m.moveLastSession(r.File, "")
	}
	return m.reloadSessions("")
}

// updateSessionNameKey handles one keystroke in the name field.
func updateSessionNameKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keys.MatchBinding(msg, sessionNameBindings()) {
	case keys.BindingClose:
		m.startup = startupSessionsOpen
		m.sessions.Refusal = ""
		if m.sessions.ThenExit && m.sessions.Exit == popup.ExitQuit {
			m.startup = m.sessions.prior
			m.sessions = sessionsPopupState{}
		}
		m.sessions.ThenExit = false
		return m, nil
	case keys.BindingConfirm:
		name := strings.TrimSpace(m.sessions.Form.Value(0))
		if name == "" {
			return m, nil
		}
		if m.sessions.Renaming != "" {
			return m.renameNamedSession(name), nil
		}
		thenExit, exit, target, rows := m.sessions.ThenExit, m.sessions.Exit, m.sessions.Target, m.sessions.Rows
		m = m.saveSessionAs(name)
		if thenExit && m.startup != startupSessionNameOpen {
			m.sessions.Rows, m.sessions.prior = rows, m.startup
			return m.goOn(exit, target, true)
		}
		return m, nil
	}
	m.sessions.Form, _ = m.sessions.Form.UpdateKey(msg)
	return m, nil
}

// nameRefusal is a refused name in the field's words.
func nameRefusal(err error) string {
	switch {
	case errors.Is(err, store.ErrSessionNameTaken):
		return "That name is taken. Try another."
	case errors.Is(err, store.ErrSessionNameEmpty):
		return "That name has no letters or digits to save under."
	case errors.Is(err, store.ErrSessionOpenElsewhere):
		return "It is open in another Sextile window."
	}
	return firstLine(err.Error())
}

// saveSessionAs is Enter in the Save as field. The new file is published
// first, so a taken name writes nothing at all; then the session this window
// ran is saved as it is at this moment, its lock is let go, and from here on
// the window runs the new one.
func (m Model) saveSessionAs(name string) Model {
	snap := m.sessionSnapshot()
	lock, err := store.SaveNamedSessionAs(name, snap)
	if err != nil {
		m.sessions.Refusal = nameRefusal(err)
		return m
	}
	if err := m.saveRunningSession(); err != nil {
		var outside bool
		if m, outside = m.sessionChangedOutside(err); !outside {
			m = m.logError("session", "the session this window ran was not saved, its file could not be written")
		}
	}
	m.releaseRunningLock()
	m.named = namedRun{file: lock.File(), name: name, lock: lock}
	m.sessionKept = false
	m = m.recordLastSession()
	m = m.logNote("session", "saved as %s in %s", logIdent(name, m.cfg), store.NamedSessionDisplay(store.NamedSessionPath(lock.File())))
	m.startup = m.sessions.prior
	m.sessions = sessionsPopupState{}
	return m
}

// renameNamedSession is Enter in the rename field.
func (m Model) renameNamedSession(name string) Model {
	old := m.sessions.Renaming
	var oldName string
	for _, r := range m.sessions.Rows {
		if r.File == old {
			oldName = r.Name
		}
	}
	if name == oldName {
		m.startup = startupSessionsOpen
		return m
	}
	var running *store.SessionLock
	if m.named.file == old {
		running = m.named.lock
	}
	file, lock, err := store.RenameNamedSession(old, name, running)
	if err != nil {
		m.sessions.Refusal = nameRefusal(err)
		return m
	}
	if running != nil {
		m.named = namedRun{file: file, name: name, lock: lock}
	}
	m = m.moveLastSession(old, file)
	m = m.logNote("session", "%s renamed to %s", logIdent(oldName, m.cfg), logIdent(name, m.cfg))
	m.startup = startupSessionsOpen
	m.sessions.Renaming, m.sessions.Refusal = "", ""
	return m.reloadSessions(file)
}

// openNamedSession is Enter on a saved session: its lock is taken (refused
// when another window holds it), this window's session is saved as it is
// continuously, its lock let go, and the chosen session's charts, selection
// and frames replace the open ones by the restore path a launch takes.
func (m Model) openNamedSession(r store.NamedSession) (tea.Model, tea.Cmd) {
	return m.openNamedSessionAfter(r, false, false)
}

// openNamedSessionAfter is openNamedSession; answered says the question in front
// of leaving was answered, and saved that the running session was written a
// moment ago (its Save as), so leaving does not write it again. A session still being opened is not left, and one that is not written
// when left is asked about first.
func (m Model) openNamedSessionAfter(r store.NamedSession, answered, saved bool) (tea.Model, tea.Cmd) {
	if m.named.pending {
		return m, nil
	}
	if r.File == m.named.file {
		m.startup = m.sessions.prior
		m.sessions = sessionsPopupState{}
		return m, nil
	}
	if r.OpenElsewhere {
		return m.logNote("session", "%s is open in another window", logIdent(r.Name, m.cfg)), nil
	}
	if !answered && m.asksBeforeLeaving(popup.ExitOpen) {
		return m.askLeave(popup.ExitOpen, r.File), nil
	}
	folder := store.FolderID(store.ChartsDir())
	sess, name, lock, err := store.OpenNamedSession(r.File, folder)
	switch {
	case errors.Is(err, store.ErrSessionOpenElsewhere):
		return m.logNote("session", "%s is open in another window", logIdent(r.Name, m.cfg)).reloadSessions(""), nil
	case errors.Is(err, store.ErrSessionUnreadable):
		return m.logNote("session", "%s did not open, its file cannot be read", logIdent(r.Name, m.cfg)), nil
	case err != nil:
		return m.logTrace("session", "open %s: %v", r.Name, err).logNote("session", "%s did not open", logIdent(r.Name, m.cfg)), nil
	}
	if !saved {
		var ok bool
		if m, ok = m.saveBeforeLeaving(); !ok {
			lock.Release()
			return m, nil
		}
	}
	m.releaseRunningLock()
	m.named = namedRun{file: r.File, name: name, lock: lock, pending: true}
	m.sessionKept = false
	m = m.recordLastSession()
	m.sessionGen++
	m.startup = m.sessions.prior
	m.sessions = sessionsPopupState{}
	return m, loadNamedSessionCmd(m.sessionGen, name, sess, folder)
}

// NamedSessionLoadedMsg is a chosen named session, read and with its chart
// files read as a launch reads them, ready to replace the open charts.
type NamedSessionLoadedMsg struct {
	Gen       int
	Name      string
	Perimeter ingest.StartupPerimeterReadMsg
}

// loadNamedSessionCmd reads the chart files a chosen session names, off the
// Update loop, as readPerimeterCmd does at launch.
func loadNamedSessionCmd(gen int, name string, s store.Session, folder string) tea.Cmd {
	return func() tea.Msg {
		p := ingest.StartupPerimeterReadMsg{Session: s, HasSession: true, FolderID: folder}
		p.FileReads, p.FileReadFailures = readSessionFiles(p)
		return NamedSessionLoadedMsg{Gen: gen, Name: name, Perimeter: p}
	}
}

// applyNamedSessionLoaded replaces the open charts with the session's: the
// floating chart stays where it is, the anchored ones are the session's, and
// its selection and frames come back by the launch's own restore.
func applyNamedSessionLoaded(m Model, msg NamedSessionLoadedMsg) (Model, tea.Cmd) {
	if msg.Gen != m.sessionGen || m.slots == nil {
		return m, nil
	}
	slots, err := store.NewSlots(m.slots.Floating().Time, m.place)
	if err != nil {
		return m.logTrace("session", "%v", err).logNote("session", "the session did not open"), nil
	}
	m = m.clearLogPanel()
	m = m.clearChartKeepingHold()
	m = m.suspendDetails()
	m.bodyCursorIdx = 0
	m = m.resetBodyTop()
	m.slots = slots
	m.cursorIdx = 0
	m.pendingRechecks = nil
	m.frames.records = nil
	p := msg.Perimeter
	// A session without a drawn: line followed the defaults: they come back.
	if m.lens.Objects.Baseline == recipe.Explicit {
		m.lens.Objects = m.defaultObjects()
	}
	m, restored := m.restoreSession(p)
	recheck := m.sessionRecheckCmd(p, restored)
	m = m.applySessionSelections(p).forgetTabMemory()
	m = m.sayEphemeris()
	m.named.pending = false
	m = m.logNote("session", "opened %s", logIdent(msg.Name, m.cfg))

	m.startup = startupComputing
	var recompute, wheel, tick tea.Cmd
	m, recompute = m.bumpIdentity(surfaceText)
	m, wheel = m.refreshWheelIfActive()
	m, tick = m.armNowTick()
	return m, tea.Batch(recompute, recheck, wheel, tick)
}

// sessionsRows are the list's rows, as the popup draws them.
func sessionsRows(m Model) []menu.RowSpec {
	return m.sessions.RowSpecs(m.cfg, m.named.file, sessionWhen)
}

// sessionsFit is the list's size for the sessions it holds.
func sessionsFit(m Model) dialog.Fit {
	return m.sessions.FitFor(m.cfg, m.named.file, sessionWhen)
}

// renderSessions draws the list.
func renderSessions(m Model, termW, termH int) string {
	return m.sessions.Render(m.cfg, termW, termH, popupChrome(m.cfg), func(cursor, total int) *dialog.Counter {
		pos := listPosition{Cursor: cursor, Total: total}
		return popupCounter(&pos, m.cfg)
	}, m.named.file, sessionWhen)
}

// sessionWhen is a session's last-updated time, local, in the held date order.
func sessionWhen(r store.NamedSession) string {
	t := r.Modified.Local()
	return civilDate(t.Year(), int(t.Month()), t.Day()) + " " + t.Format("15:04")
}

// renderSessionName draws the name field: Save as or a rename.
func renderSessionName(m Model, termW, termH int) string {
	return m.sessions.RenderName(m.cfg, termW, termH, popupChrome(m.cfg))
}

// savedWhenLeft says the session this window runs is written when it is left: a
// named session this window owns and has finished opening. Any other is asked
// about before a new session replaces it.
func (m Model) savedWhenLeft() bool {
	return m.named.running() && m.ownsSession() && !m.named.pending
}

// saveBeforeLeaving writes the running session before the window leaves it for
// another. It reports false when the session must be kept: the write failed, or
// the file changed outside Sextile and was left as it is. The window then keeps
// the session, its lock and its charts.
func (m Model) saveBeforeLeaving() (Model, bool) {
	err := m.saveRunningSession()
	switch {
	case err == nil:
		return m, true
	case errors.Is(err, store.ErrSessionChangedOutside) && m.named.running():
		return m.logError("session", "%s changed outside Sextile and was left as it is, so this window stays in it", logIdent(m.named.name, m.cfg)), false
	}
	return m.logError("session", "the session this window ran was not saved, its file could not be written"), false
}

// sessionChangedOutside answers a save the store refused because the running
// named session's file changed outside Sextile since this window read or wrote
// it: the file is left as it is, the window keeps nothing more in it (as for a
// refused read), and one Log line says so. Save as still works. ok is false
// for any other error, which the caller reports as before.
func (m Model) sessionChangedOutside(err error) (Model, bool) {
	if !errors.Is(err, store.ErrSessionChangedOutside) || !m.named.running() {
		return m, false
	}
	if !m.named.kept {
		m.named.kept = true
		m = m.logNote("session", "%s changed in another program, Sextile left its file alone", logIdent(m.named.name, m.cfg))
	}
	return m, true
}
