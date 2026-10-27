package ui

import (
	"strings"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// wheel_format, the preferences.yaml key that says which image the wheel
// server hosts: `svg` (the default) or `png`. The compiler already renders
// both; this only chooses (the specification W1 part 5).

// readWheelFormat and setWheelFormat are the two doors, indirected like
// showWheel so a test can stand in for the file and for the render.
var (
	readWheelFormat = store.WheelFormat
	setWheelFormat  = lookup.SetWheelFormat
)

// applyWheelFormat takes the wheel_format key into the render: svg, png, or
// the default when the key is absent. Anything else is refused with one Log
// note naming the two words that are allowed, and svg stands. The reader
// reports; the decision and the Log line are made here, as for display:.
// Silence is the contract for the good cases.
func (m Model) applyWheelFormat(c store.WheelFormatConfig) Model {
	format := recipe.ImageSVG
	if c.Present {
		switch strings.ToLower(strings.TrimSpace(c.Value)) {
		case "svg":
			m.wheelFormatRefused = ""
		case "png":
			m.wheelFormatRefused = ""
			format = recipe.ImagePNG
		default:
			if m.wheelFormatRefused != c.Value {
				m.wheelFormatRefused = c.Value
				m = m.logNote("preferences", "wheel_format %q refused: expected svg or png — svg stands", c.Value)
			}
		}
	}
	setWheelFormat(format)
	return m
}
