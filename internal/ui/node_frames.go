package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
)

// node_frames.go is the per-chart form of the Nodes and Lilith: which of the
// four pairs Astrolog casts each chart shows, beside the house system and the
// zodiac of house_frames.go. The answer is resolved per slot, handed to lookup
// as lens.Objects.Nodes, and reaches Astrolog as the two explicit switches of
// every call.

// nodeSource says which rule produced a chart's own form.
type nodeSource int

const (
	nodeFallback nodeSource = iota // nothing named a form: the mean Node and Lilith
	nodeDefault                    // preferences.yaml `frame: node:` and `lilith:`
	nodeFile                       // the chart file's own switches
	nodeChosen                     // a session record
	nodeSession                    // chosen for a chart with no file: until quit
)

// homeNodes is the form a chart takes with no choice of this session: what its
// file states, else the user's default, else the mean Node and Lilith.
// fromFile says the file stated it.
func (m Model) homeNodes(s *store.Slot) (form recipe.NodeForm, fromFile bool) {
	if f := slotFile(s); f != "" {
		if ff, ok := m.frames.files[f]; ok && ff.nodesStated {
			return ff.nodes, true
		}
	}
	if m.frames.defNSet {
		return m.frames.defN, false
	}
	return recipe.NodesMean, false
}

// ownNodes resolves a chart's own form: its session record, else the form its
// file states, else the user's default, else the mean Node and Lilith.
func (m Model) ownNodes(s *store.Slot) (recipe.NodeForm, nodeSource) {
	if s != nil {
		if f, ok := m.frames.volatileN[volatileKey(s)]; ok && s.Chart.Source.File == "" {
			return f, nodeSession
		}
		var rec store.FrameRecord
		var found bool
		if s.Floating {
			rec, found = m.frames.find("", true)
		} else if f := slotFile(s); f != "" {
			rec, found = m.frames.find(f, false)
		}
		if found && rec.HasNodes && (s.Floating || sameRemembered(rec.When, m.chartMoment(s))) {
			return rec.Nodes, nodeChosen
		}
	}
	form, fromFile := m.homeNodes(s)
	switch {
	case fromFile:
		return form, nodeFile
	case m.frames.defNSet:
		return form, nodeDefault
	}
	return form, nodeFallback
}

// sceneNodes is the form every call of a comparison is cast in: the base's.
// Outside a comparison it is the cursor's own.
func (m Model) sceneNodes(sc scene) recipe.NodeForm {
	if sc.mode == sceneComparing && sc.frame != nil {
		f, _ := m.ownNodes(sc.frame)
		return f
	}
	f, _ := m.ownNodes(sc.cursor)
	return f
}

// applyNodesConfig reads the default form from the preferences: `node:` and
// `lilith:` together name one of the four pairs Astrolog casts. One line alone,
// a word that is not one of the three, or the pair Astrolog cannot cast leaves
// the mean Node and Lilith in force and is said once.
func (m Model) applyNodesConfig(cfg store.FrameConfig) Model {
	if !cfg.NodePresent && !cfg.LilithPresent {
		return m
	}
	if cfg.NodePresent && cfg.LilithPresent {
		if f, ok := recipe.NodeFormFromWords(strings.TrimSpace(cfg.Node), strings.TrimSpace(cfg.Lilith)); ok {
			m.frames.defN, m.frames.defNSet = f, true
			return m
		}
	}
	if !m.frames.logged["bad nodes default"] {
		m.frames = m.frames.withLogged("bad nodes default")
		m = m.logNote("settings", "the default Node and Lilith do not go together, Mean is used")
	}
	return m
}

// applyNodesChoice is what Enter does in the Nodes and Lilith chooser for the
// chart in slot slotIdx, which was cast in cast when the chooser opened. The
// pair that is cast writes nothing. The chart's home form removes the session's
// choice. Any other pair is recorded and saved at once, as a house choice is.
func (m Model) applyNodesChoice(slotIdx int, cast, chosen recipe.NodeForm) (Model, tea.Cmd) {
	if chosen == cast || m.slots == nil {
		return m, nil
	}
	all := m.slots.All()
	if slotIdx < 0 || slotIdx >= len(all) {
		return m, nil
	}
	if home, _ := m.homeNodes(all[slotIdx]); chosen == home {
		return m.forgetNodesChoice(slotIdx)
	}
	return m.chooseNodes(slotIdx, chosen)
}

// chooseNodes is the one door a choice of form comes through: the same path as
// a house choice, writing the record's `node:` and `lilith:`.
func (m Model) chooseNodes(slotIdx int, f recipe.NodeForm) (Model, tea.Cmd) {
	return m.chooseFrame(slotIdx, frameChoice{
		lane: "nodes", noun: "Nodes and Lilith", runOnly: "the form of the Nodes and Lilith is",
		set: func(rec *store.FrameRecord) { rec.Nodes, rec.HasNodes = f, true },
		setVolatile: func(fs *frameState, key string) {
			vol := make(map[string]recipe.NodeForm, len(fs.volatileN)+1)
			for k, v := range fs.volatileN {
				vol[k] = v
			}
			vol[key] = f
			fs.volatileN = vol
		},
	})
}

// forgetNodesChoice removes the session's choice of form for the chart, so it
// takes its home form again. A record left with no houses, zodiac or form goes.
func (m Model) forgetNodesChoice(slotIdx int) (Model, tea.Cmd) {
	return m.chooseFrame(slotIdx, frameChoice{
		lane: "nodes", noun: "Nodes and Lilith", runOnly: "the form of the Nodes and Lilith is", quiet: true,
		set: func(rec *store.FrameRecord) { rec.Nodes, rec.HasNodes = recipe.NodesMean, false },
		setVolatile: func(fs *frameState, key string) {
			vol := make(map[string]recipe.NodeForm, len(fs.volatileN))
			for k, v := range fs.volatileN {
				if k != key {
					vol[k] = v
				}
			}
			fs.volatileN = vol
		},
	})
}

// nodeFormWords is the form as the Log says it: `mean Node, mean Lilith`.
func nodeFormWords(f recipe.NodeForm) string {
	n := popup.NodeFormName(f)
	return strings.ToLower(n[:1]) + n[1:]
}
