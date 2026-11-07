package popup

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// sessions.go is the Sessions list (`, o`) and its name field as popups: the
// rows, the cursor, the draw. The store calls (save, open, rename, trash) and
// the run the window holds stay in ui; the popup reports what a key chose and
// ui acts on it.

// The popups' names.
const (
	titleSessions      = dialog.TitleSessions
	titleSaveSession   = "Save session"
	titleRenameSession = "Rename session"
)

// Sessions is the list and its name field.
type Sessions struct {
	Rows    []store.NamedSession
	ListErr error
	Cursor  int // 0 is the Save row; i+1 is Rows[i]
	Offset  int

	Form     dialog.Form
	Renaming string // the file the name field renames; "" for Save as
	Refusal  string // the last submit was refused; the field stays open

	// Fit is the list's size, decided when it opens for the sessions it holds
	// then. A session saved from the list joins it without resizing the box:
	// the list scrolls, and the next opening sizes for it. Zero for a state
	// that was never opened (FitFor then measures what it holds).
	Fit dialog.Fit
}

// Two actions only the Sessions list has: Ctrl+R on a session, and Ctrl+D.
const (
	// Rename asks to rename the session under the cursor.
	Rename Action = Toggle + 1 + iota
	// Trash asks to move the session under the cursor to the trash.
	Trash
)

// SessionsOutcome is what one key decided. Action is Confirm, Rename, Trash or
// Dismiss when the key asks ui to act, else Type; Row and OnRow name the saved
// session under the cursor (OnRow is false on the Save row).
type SessionsOutcome struct {
	Action Action
	Row    store.NamedSession
	OnRow  bool
}

// SaveRow is the first row's text.
func SaveRow(cfg term.Style) string {
	if cfg.UseASCII() {
		return "+ Save this session as..."
	}
	return "+ Save this session as…"
}

// RowUnder is the saved session under the cursor, or false on the Save row.
func (s Sessions) RowUnder() (store.NamedSession, bool) {
	i := s.Cursor - 1
	if i < 0 || i >= len(s.Rows) {
		return store.NamedSession{}, false
	}
	return s.Rows[i], true
}

// Apply runs one action. visible is how many rows one page holds. Moving keys
// are handled here; Confirm, Rename, Trash and Dismiss come back as an outcome.
func (s *Sessions) Apply(a Action, visible int) SessionsOutcome {
	total := len(s.Rows) + 1
	page := func(op menu.PageOp) {
		s.Cursor, s.Offset = menu.PageCursor(s.Cursor, s.Offset, total, visible, op)
	}
	switch a {
	case Up:
		if s.Cursor > 0 {
			s.Cursor--
		}
	case Down:
		if s.Cursor < total-1 {
			s.Cursor++
		}
	case PageUp:
		page(menu.PageUp)
	case PageDown:
		page(menu.PageDown)
	case PageHome:
		page(menu.PageHome)
	case PageEnd:
		page(menu.PageEnd)
	case Confirm, Rename, Trash, Dismiss:
		r, ok := s.RowUnder()
		return SessionsOutcome{Action: a, Row: r, OnRow: ok}
	}
	return SessionsOutcome{}
}

// RowSpecs are the list's rows: the Save row, then a row per session. running
// is the file this window runs ("" for the general session); when is a
// session's last-updated text.
func (s Sessions) RowSpecs(cfg term.Style, running string, when func(store.NamedSession) string) []menu.RowSpec {
	rows := []menu.RowSpec{{Left: SaveRow(cfg), Cursor: s.Cursor == 0}}
	for i, r := range s.Rows {
		state := ""
		switch {
		case r.File == running:
			state = "this window"
		case r.OpenElsewhere:
			state = "in another window"
		}
		rows = append(rows, menu.RowSpec{Left: r.Name, Right: []string{state, when(r)}, Cursor: s.Cursor == i+1})
	}
	return rows
}

// Note is the line under the rows when the list has something to say about
// itself: why it is empty, or why it could not be read.
func (s Sessions) Note() string {
	switch {
	case s.ListErr != nil:
		return fmt.Sprintf("(sessions unavailable: %v)", s.ListErr)
	case len(s.Rows) == 0:
		return "(no saved sessions in this charts folder)"
	}
	return ""
}

// FitFor is the list's size for the sessions it holds: wide enough for the
// widest row and as tall as every row and the note under them, if there is one.
func (s Sessions) FitFor(cfg term.Style, running string, when func(store.NamedSession) string) dialog.Fit {
	rows := s.RowSpecs(cfg, running, when)
	fit := dialog.Fit{Width: menu.WidestRow(cfg, rows...), Rows: len(rows)}
	if note := s.Note(); note != "" {
		fit.Width = max(fit.Width, min(ansi.StringWidth(note), dialog.ProseWidth))
		fit.Rows++
	}
	return fit
}

// Render draws the list.
func (s Sessions) Render(cfg term.Style, termW, termH int, chrome dialog.Chrome, counter func(cursor, total int) *dialog.Counter, running string, when func(store.NamedSession) string) string {
	total := len(s.Rows) + 1
	offsetFor := func(height int) int {
		off := s.Offset
		if s.Cursor < off {
			off = s.Cursor
		}
		if s.Cursor >= off+height {
			off = s.Cursor - height + 1
		}
		return max(off, 0)
	}
	rows := s.RowSpecs(cfg, running, when)
	note := s.Note()
	fit := s.Fit
	if fit.Width == 0 {
		fit = s.FitFor(cfg, running, when)
	}
	return dialog.Render(dialog.Box{
		Title:    titleSessions,
		NoInput:  true,
		Fit:      fit,
		Position: counter(s.Cursor, total),
		Scroll: func(height int) (pane.Viewport, int) {
			return pane.Viewport{Total: total, Visible: height, Offset: offsetFor(height)}, 0
		},
	}, cfg, termW, termH, chrome, func(width, height int) string {
		var lines []string
		off := offsetFor(height)
		for i := off; i < len(rows) && len(lines) < height; i++ {
			lines = append(lines, menu.Row(rows[i], cfg, width))
		}
		if note != "" && len(lines) < height {
			lines = append(lines, menu.Note(note, width, cfg))
		}
		return menu.Block(lines, width, height)
	})
}

// NameTyped reports whether the name field holds anything but blanks.
func (s Sessions) NameTyped() bool { return strings.TrimSpace(s.Form.Value(0)) != "" }

// RenderName draws the name field: Save as or a rename.
func (s Sessions) RenderName(cfg term.Style, termW, termH int, chrome dialog.Chrome) string {
	title := titleSaveSession
	if s.Renaming != "" {
		title = titleRenameSession
	}
	lead := "…/"
	if cfg.UseASCII() {
		lead = ".../"
	}
	// The folder the name will be saved in, with the file once there is a name.
	info := lead + "sessions/" + store.NamedSessionFile(s.Form.Value(0))
	return dialog.RenderForm(s.Form, dialog.FormView{
		Title:   title,
		Info:    []string{info},
		Refusal: s.Refusal,
	}, cfg, termW, termH, chrome)
}
