package ui

import "github.com/cruiserxyz/sextile/internal/store"

// experiments.go is the table of experimental switches (specs/experiments-popup.md,
// the control repository). Each row is an id (store.SetExperimental's id rule), one
// line saying what it changes, and is off by default. A feature asks
// m.experiment(id) at the one place it forks; graduating or dropping a switch
// deletes its row and its fork, and TestEveryExperimentIsRead refuses a row
// no code asks for.
//
// Nothing outside the hidden popup (`,L p`) names a switch: not the key bar, Keybindings,
// the Keybindings popup, the Log, the docs or the changelog.

// expEnterColour is Enter on a Timeline aspect row, drawn in the aspect's colour.
const expEnterColour = "enter-colour"

// expAspectDot draws an aspect that has no glyph as a centered dot in the
// Aspects panel's cells, in place of its three letters.
const expAspectDot = "aspect-dot"

// expEnterStays keeps Details open after Enter on a timeline row.
const expEnterStays = "enter-stays"

// experimentRow is one switch of the table.
type experimentRow struct {
	id   string
	line string
}

// experiments is the table, in the order the popup lists it.
var experiments = []experimentRow{
	{expEnterColour, "Enter on a Timeline aspect: its orb and the led body's name and degrees in the aspect's colour"},
	{expAspectDot, "Aspects without a glyph show a centered dot instead of their letters"},
	{expEnterStays, "Enter on a timeline row keeps Details open instead of going back to the charts"},
}

// experiment reports whether the switch id is on. Off by default, and for an id
// the table does not have.
func (m Model) experiment(id string) bool { return m.experimentsOn[id] }

// applyExperimental takes the experimental: block into the Model: the ids the
// table lists that read on. Ids the file has and the table does not are ignored
// (the writer carries them through). It is silent.
func (m Model) applyExperimental(c store.ExperimentalConfig) Model {
	m.experimentsOn = nil
	for _, row := range experiments {
		if c.On[row.id] {
			if m.experimentsOn == nil {
				m.experimentsOn = map[string]bool{}
			}
			m.experimentsOn[row.id] = true
		}
	}
	return m
}
