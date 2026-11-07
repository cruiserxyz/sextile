package ui

import (
	"errors"
	"github.com/charmbracelet/bubbles/key"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// sessions_popup_model.go is the Model's half of the Sessions list (`, o`; the popup
// itself is internal/ui/popup/sessions.go, and this file keeps the store calls): the first row saves the
// open workspace under a name, the rows below are the named sessions saved in
// this charts folder, newest first. It moves like the chart list: ↑↓, Enter,
// Ctrl+R to rename, Esc to close; Ctrl+D moves a session to the trash.
//
// A window runs exactly ONE session and keeps updating only that one: the
// general session (session.go, as before), or the named session it saved or
// opened here. Every write that went to the general session goes to the named
// one instead (saveRunningSession), and the window holds that session's lock,
// so a session open in another window is marked as such and is not opened,
// renamed or trashed from this one.
//
//	main ──, o──▶ startupSessionsOpen ──Enter on row 1──▶ startupSessionNameOpen (Save as)
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

// openSessionsList is `, o`.
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
				m.sessions.Cursor = i + 1
			}
		}
	}
	m.sessions.Cursor = min(max(m.sessions.Cursor, 0), len(rows))
	return m
}

// sessionRow is the saved session under the cursor, or false on the Save row.
func (m Model) sessionRow() (store.NamedSession, bool) { return m.sessions.RowUnder() }

// The conditions the Sessions list's bar shows its keys under.
func (m Model) sessionsOnSaveRow() bool { return m.sessions.Cursor == 0 }

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
// saves on the first row and opens a session on the others; Ctrl+R renames and
// Ctrl+D trashes the highlighted session. Nothing in it types.
func sessionsBindings() []keys.BindingEntry {
	return keys.Titled(append([]keys.BindingEntry{
		listUp().ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 1 }), listDown(),
		confirm("Save as").ShownWhen(func(c keys.Context) bool { return c.Sessions.OnSaveRow }),
		confirm("Open").ShownWhen(func(c keys.Context) bool { return c.Sessions.CanOpen }),
		{ID: keys.BindingRename, Binding: key.NewBinding(key.WithKeys("ctrl+r")), Title: "Rename", BarKey: "⌃r", Bar: true, When: func(c keys.Context) bool { return c.Sessions.OnFreeRow }},
		{ID: keys.BindingSessionDelete, Binding: key.NewBinding(key.WithKeys("ctrl+d")), Title: "Delete", BarKey: "⌃d", Bar: true, When: func(c keys.Context) bool { return c.Sessions.CanTrash }},
		closeOn("Close"),
	}, pageEntries()...))
}

// sessionNameBindings is the Sessions name field's table: Enter saves (or
// renames), Esc goes back to the list.
func sessionNameBindings() []keys.BindingEntry {
	return keys.Titled([]keys.BindingEntry{confirm("Save").ShownWhen(func(c keys.Context) bool { return c.Sessions.NameTyped }), closeOn("Back")})
}

// updateSessionsKey handles one keystroke while the list has the keyboard: it
// resolves the key through the bindings table to the popup's Action, applies it,
// and acts on what the popup reports.
func updateSessionsKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		if !out.OnRow {
			m.sessions.Form = dialog.NewForm(dialog.Field{Label: "Name: ", Value: m.sessionPrefill()})
			m.sessions.Renaming, m.sessions.Refusal = "", ""
			m.startup = startupSessionNameOpen
			return m, nil
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
		return m.trashNamedSession(r), nil
	}
	return m, nil
}

// trashNamedSession is Ctrl+D: the session moves to the trash, no confirm, and
// one Log line says where it went.
func (m Model) trashNamedSession(r store.NamedSession) Model {
	switch {
	case r.File == m.named.file:
		return m.logNote("session", "%s: this window runs it. Switch to another session first", logIdent(r.Name, m.cfg))
	case r.OpenElsewhere:
		return m.logNote("session", "%s is open in another window", logIdent(r.Name, m.cfg))
	}
	path, err := store.TrashNamedSession(r.File, m.named.lock, m.clock.Now())
	switch {
	case errors.Is(err, store.ErrSessionOpenElsewhere):
		m = m.logNote("session", "%s is open in another window", logIdent(r.Name, m.cfg))
	case err != nil:
		m = m.logNote("session", "%s was not moved to the trash: %s", logIdent(r.Name, m.cfg), firstLine(err.Error()))
	default:
		m = m.logNote("session", "%s moved to %s", logIdent(r.Name, m.cfg), store.NamedSessionDisplay(path))
	}
	return m.reloadSessions("")
}

// updateSessionNameKey handles one keystroke in the name field.
func updateSessionNameKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keys.MatchBinding(msg, sessionNameBindings()) {
	case keys.BindingClose:
		m.startup = startupSessionsOpen
		m.sessions.Refusal = ""
		return m, nil
	case keys.BindingConfirm:
		name := strings.TrimSpace(m.sessions.Form.Value(0))
		if name == "" {
			return m, nil
		}
		if m.sessions.Renaming != "" {
			return m.renameNamedSession(name), nil
		}
		return m.saveSessionAs(name), nil
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
			m = m.logError("Couldn't save the session this window ran. Its file could not be written.")
		}
	}
	m.releaseRunningLock()
	m.named = namedRun{file: lock.File(), name: name, lock: lock}
	m.sessionKept = false
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
	if r.File == m.named.file {
		m.startup = m.sessions.prior
		m.sessions = sessionsPopupState{}
		return m, nil
	}
	if r.OpenElsewhere {
		return m.logNote("session", "%s is open in another window", logIdent(r.Name, m.cfg)), nil
	}
	folder := store.FolderID(store.ChartsDir())
	sess, name, lock, err := store.OpenNamedSession(r.File, folder)
	switch {
	case errors.Is(err, store.ErrSessionOpenElsewhere):
		return m.logNote("session", "%s is open in another window", logIdent(r.Name, m.cfg)).reloadSessions(""), nil
	case errors.Is(err, store.ErrSessionUnreadable):
		return m.logNote("session", "%s can't be opened: its file cannot be used", logIdent(r.Name, m.cfg)), nil
	case err != nil:
		return m.logNote("session", "%s can't be opened: %s", logIdent(r.Name, m.cfg), firstLine(err.Error())), nil
	}
	if err := m.saveRunningSession(); err != nil {
		// A session whose file changed outside Sextile is left as it is, and
		// the window may still leave it for another.
		var outside bool
		if m, outside = m.sessionChangedOutside(err); !outside {
			lock.Release()
			return m.logError("Couldn't save the session this window ran. Its file could not be written."), nil
		}
	}
	m.releaseRunningLock()
	m.named = namedRun{file: r.File, name: name, lock: lock, pending: true}
	m.sessionKept = false
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
		return m.logNote("slots", "%v", err), nil
	}
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
	m, say := m.sayEphemeris()
	m.named.pending = false
	m = m.logNote("session", "opened %s", logIdent(msg.Name, m.cfg))

	m.startup = startupComputing
	var recompute, wheel, tick tea.Cmd
	m, recompute = m.bumpIdentity(surfaceText)
	m, wheel = m.refreshWheelIfActive()
	m, tick = m.armNowTick()
	return m, tea.Batch(recompute, recheck, wheel, tick, say)
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
		m = m.logNote("session", "%s: its file changed outside Sextile and was left as it is", logIdent(m.named.name, m.cfg))
	}
	return m, true
}
