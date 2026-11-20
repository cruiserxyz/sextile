package ui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
)

// experiments_popup_model.go is the Model's half of the hidden Experiments popup
// (`,L p`; the popup itself is internal/ui/popup/experiments.go). The door is
// in no key bar, Keybindings popup or document.
//
// A flip writes first: Space runs store.SetExperimental as a command and only
// its successful result changes the Model and the tick, so the screen never
// shows a switch the file does not hold. Nothing here writes to the Log.

// experimentWrittenMsg is the result of one switch's write.
type experimentWrittenMsg struct {
	id  string
	on  bool
	err error
}

// openExperiments is `p` on the focused Log or in the console: the popup opens
// over whatever had the keyboard, and closing it returns there.
func (m Model) openExperiments() (tea.Model, tea.Cmd) {
	if m.startup != startupReady {
		return m, nil
	}
	rows := make([]popup.ExperimentRow, len(experiments))
	for i, row := range experiments {
		rows[i] = popup.ExperimentRow{ID: row.id, Line: row.line, On: m.experiment(row.id)}
	}
	m.experimentsPopup = popup.NewExperiments(rows)
	m.startup = startupExperimentsOpen
	return m, nil
}

// experimentsBindings is the popup's table: move, flip, close.
func experimentsBindings() []keys.BindingEntry {
	return keys.TitledLocal([]keys.BindingEntry{
		{ID: keys.BindingListUp, Binding: key.NewBinding(key.WithKeys("up", "k")), Title: "Move row up", BarKey: "up"},
		{ID: keys.BindingListDown, Binding: key.NewBinding(key.WithKeys("down", "j")), Title: "Move row down", BarKey: "down"},
		{ID: keys.BindingToggle, Binding: key.NewBinding(key.WithKeys(" ")), Title: "Switch", BarKey: "space", Bar: true},
		{ID: keys.BindingClose, Binding: key.NewBinding(key.WithKeys("enter", "esc", "q")), Title: "Close", BarKey: "enter/esc/q", Bar: true, Pinned: true},
	})
}

// updateExperimentsKey handles one keystroke while the popup has the keyboard.
func updateExperimentsKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	st := m.experimentsPopup
	a := popup.Type
	switch keys.MatchBinding(msg, experimentsBindings()) {
	case keys.BindingClose:
		a = popup.Dismiss
	case keys.BindingListUp:
		a = popup.Up
	case keys.BindingListDown:
		a = popup.Down
	case keys.BindingToggle:
		a = popup.Toggle
	}
	out := st.Apply(a)
	m.experimentsPopup = st
	if out.Closed {
		m.startup = startupReady
		return m, nil
	}
	if out.Flip {
		return m, writeExperiment(out.ID, out.To)
	}
	return m, nil
}

// writeExperiment is the flip's write, run as a command.
func writeExperiment(id string, on bool) tea.Cmd {
	return func() tea.Msg {
		return experimentWrittenMsg{id: id, on: on, err: store.SetExperimental(id, on)}
	}
}

// applyExperimentWritten folds a write's result in: a success changes the
// Model's switch and the popup's tick, a failure keeps both and the popup
// says why at its foot. Nothing goes to the Log either way.
func applyExperimentWritten(m Model, msg experimentWrittenMsg) Model {
	st := m.experimentsPopup
	st.Settled(msg.id, msg.on, msg.err)
	m.experimentsPopup = st
	if msg.err != nil {
		return m
	}
	next := make(map[string]bool, len(m.experimentsOn)+1)
	for id, on := range m.experimentsOn {
		next[id] = on
	}
	next[msg.id] = msg.on
	m.experimentsOn = next
	return m
}

// isPlainKey is whether msg is the single unmodified rune r.
func isPlainKey(msg tea.KeyMsg, r string) bool {
	return msg.Type == tea.KeyRunes && !msg.Alt && string(msg.Runes) == r
}

// renderExperiments draws the popup.
func renderExperiments(st popup.Experiments, cfg Config, termW, termH int) string {
	return st.Render(cfg, termW, termH, popupChrome(cfg))
}
