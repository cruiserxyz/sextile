package ui

import (
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// The Status panel is one line: the session this window runs and its charts
// folder. Its Details lists the saved sessions of that folder, the folder's
// position and the credits. Both only read: nothing here writes a file.

// unnamedSession is what the line calls the session no one has named.
const unnamedSession = "unnamed"

// sessionInfo is what the Status panel's Session tab reads from disk, kept so a
// frame draws without touching it. refreshSessionInfo fills it while the
// panel's Details is on screen.
type sessionInfo struct {
	read     bool
	rows     []store.NamedSession
	listErr  error
	position store.PositionRead
}

// homeShort writes the home folder of path as `~`.
func homeShort(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	switch {
	case path == home:
		return "~"
	case strings.HasPrefix(path, home+string(os.PathSeparator)):
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// sessionName is the name the running session was saved under, or `unnamed`.
func (m Model) sessionName() string {
	if m.named.running() && m.named.name != "" {
		return m.named.name
	}
	return unnamedSession
}

// sessionLine is the Status panel's line: the session's name, an arrow and
// the charts folder. It is derived from the model each frame, so it is true
// the moment a session is opened, saved under a name, renamed or left.
func (m Model) sessionLine() string {
	arrow := "→"
	if m.cfg.UseASCII() {
		arrow = "->"
	}
	return m.sessionName() + " " + arrow + " " + homeShort(store.ChartsDir())
}

// refreshSessionInfo reads the saved sessions of this charts folder and the
// folder's position, while the Status panel's Details is on screen.
func (m Model) refreshSessionInfo() Model {
	if m.focused != PanelStatus || !m.detailsVisible() {
		return m
	}
	dir := store.ChartsDir()
	info := sessionInfo{read: true, position: store.ReadPosition(dir)}
	info.rows, info.listErr = store.SavedNamedSessions(store.FolderID(dir))
	m.sessionInfo = info
	return m
}

// positionWords is the folder's position as the Log words it.
func (m Model) positionWords() string {
	r := m.sessionInfo.position
	switch {
	case r.Fault != "":
		return "Sextile writes nothing in it. " + store.PositionFileName() + " " + r.Fault + "."
	case r.Effective == store.PositionProtected:
		if r.ReadonlyEnv {
			return "Sextile writes nothing in it. This run is read-only."
		}
		return "Sextile writes nothing in it."
	case r.Effective == store.PositionAllow:
		return "Sextile writes without asking. It is set by hand in " + store.PositionFileName() + "."
	}
	return "Sextile asks before each write."
}

// sessionDetails is the text of the Status panel's Session tab.
func (m Model) sessionDetails() string {
	folder := homeShort(store.ChartsDir())
	var b []string
	b = append(b, "Sessions in "+folder)
	rows := m.sessionInfo.rows
	if m.sessionInfo.listErr != nil {
		b = append(b, "  The sessions could not be listed.")
	}
	if !m.named.running() {
		b = append(b, "  ● "+unnamedSession+" (this window)")
	}
	for _, r := range rows {
		mark := "  "
		if m.named.running() && r.File == m.named.file {
			mark = "● "
		}
		b = append(b, "  "+mark+sessionListName(r, m.cfg)+"  "+sessionWhen(r))
	}
	b = append(b, "", "Charts folder: "+folder, "  "+m.positionWords())
	if m.cfg.UseASCII() {
		for i, l := range b {
			b[i] = strings.ReplaceAll(l, "●", "*")
		}
	}
	return strings.Join(b, "\n")
}

// sessionListName is a saved session's name, cut to a width a column of dates
// can follow.
func sessionListName(r store.NamedSession, cfg Config) string {
	name := term.CutRight(r.Name, 30, cfg)
	return name + strings.Repeat(" ", max(30-ansi.StringWidth(name), 0))
}
