package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
)

// nodes_finder_model.go is the Model's half of the Nodes and Lilith chooser
// (`, t`, Ctrl+T): the four pairs Astrolog casts, in the zodiac finder's chassis.
// Enter on a pair other than the one cast goes to chooseNodes, the one door a
// choice of form comes through; Enter on the cast pair closes and writes nothing.

// nodesFinderState is the chooser's state. The list machinery is the chassis's.
type nodesFinderState = popup.Finder[recipe.NodeForm]

// nodesFinderFor is the chooser for slot: the cursor and the mark on the pair
// that is cast, the note on the pair the chart's home form is.
func (m Model) nodesFinderFor(slot *store.Slot) nodesFinderState {
	cast, _ := m.ownNodes(slot)
	home, fromFile := m.homeNodes(slot)
	note := "your default"
	if fromFile {
		note = "from its file"
	}
	return nodesFinderState{
		Kind: popup.NodesKind(home, note), Name: houseFinderChartName(slot.Chart.Name, slot.Floating),
		Current: cast, SlotIdx: m.slotIndexOf(slot),
	}
}

// openNodesFinder is the `, t` and Ctrl+T handler: it opens the chooser for the
// focused chart, from the main screen only and once the first frame is in.
func openNodesFinder(m Model) (tea.Model, tea.Cmd) {
	if m.blocked(keys.BindingNodes) != "" {
		return m, nil
	}
	st := m.nodesFinderFor(m.focusedSlot())
	st.Open(dialog.ListHeight(m.height))
	m.nodesFinder = st
	m.startup = startupNodesOpen
	return m, nil
}

// nodesBindings is the chooser's table: the finders'.
func nodesBindings() []keys.BindingEntry { return finderBindings() }

// updateNodesFinderKey handles one keystroke while the chooser has the keyboard.
func updateNodesFinderKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.nodesFinder
	id := keys.MatchBinding(msg, nodesBindings())
	out := f.Apply(finderAction(id), msg, dialog.ListHeight(m.height))
	switch {
	case out.Dismissed:
		m.startup = startupReady
		return m, nil
	case out.Confirmed:
		m.startup = startupReady
		return m.applyNodesChoice(f.SlotIdx, f.Current, out.Chosen)
	}
	m.nodesFinder = f
	return m, nil
}

// renderNodesFinder draws the chooser.
func renderNodesFinder(f nodesFinderState, cfg Config, termW, termH int) string {
	return f.Render(cfg, termW, termH, popupChrome(cfg), func(cursor, total int) *dialog.Counter {
		pos := listPosition{Cursor: cursor, Total: total}
		return popupCounter(&pos, cfg)
	})
}
