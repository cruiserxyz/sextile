package panel

import (
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// BodyColumn is one shown chart's Bodies column: its positions as that chart
// shows them alone, or the error that left the column blank.
type BodyColumn struct {
	Label     string
	Positions []aspect.Position
	// Cusps is the chart's own house frame, from the same lookup.
	Cusps aspect.Cusps
	Err   error
}
