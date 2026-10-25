package ui

// PanelID is the typed identity of a UI panel. Was provisional string
// literals scattered across the package.
type PanelID string

const (
	PanelNone    PanelID = ""
	PanelChart   PanelID = "chart"
	PanelBodies  PanelID = "bodies"
	PanelAspects PanelID = "aspects"
	PanelDetails PanelID = "details"
	PanelLog     PanelID = "log"
)
