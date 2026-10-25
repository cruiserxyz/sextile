package ui

import (
	"math"

	"github.com/charmbracelet/lipgloss"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Velocity-bar rendering ported from legacy gui/views/velocity_bar.go. The bar
// visualises a body's speed as a fraction of its own average daily motion (the
// astrolog -v0 ratio): 1.0 = average, <0.05 ≈ stationary, capped at 2.5. The
// sign of the speed encodes direction (negative = retrograde). Angles get no bar.
const (
	barCells            = 3
	barSubLevels        = 8
	barRatioCap         = 2.5
	stationaryThreshold = 0.05
)

// barRamp is the 9-level partial-block ramp (index 0..8 = empty..full).
var barRamp = [9]string{" ", "▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"}

// velocityBar renders the 4-cell velocity field: position 0 is the retrograde
// glyph (℞) or a space, followed by a 3-cell magnitude bar. Angles return four
// spaces (the -v0 ratio is meaningless for them). When useColor, the bar is
// tinted with the direction gradient. A slow direct body's S/R and S/D marks
// replace the bar elsewhere (stationMark, D38); this function only draws the
// bar and ℞.
func velocityBar(speed float64, angle, useColor bool) string {
	if angle {
		return "    "
	}
	retro := math.Signbit(speed) // Astrolog's R, including -0.000 (C12)
	ratio := math.Abs(speed)

	t := (ratio - stationaryThreshold) / (barRatioCap - stationaryThreshold)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	// sqrt scaling expands low speeds; legacy drives BOTH the fill AND the
	// gradient index off `normalized` (not raw t), which reaches the more
	// saturated gradient stops — matching legacy's colour intensity.
	normalized := math.Sqrt(t)
	fillUnits := 1 + int(math.Round(normalized*float64(barCells*barSubLevels-1)))

	pos0 := " "
	if retro {
		pos0 = "℞"
	}
	bar := renderBarCells(fillUnits)
	if useColor {
		bar = lipgloss.NewStyle().Foreground(barColor(retro, normalized)).Render(bar)
	}
	return pos0 + bar
}

// renderBarCells fills barCells cells (barSubLevels sublevels each) up to
// fillUnits, picking the partial-block glyph for each cell's remaining level.
func renderBarCells(fillUnits int) string {
	out := make([]rune, 0, barCells)
	for c := 0; c < barCells; c++ {
		level := fillUnits - c*barSubLevels
		if level < 0 {
			level = 0
		}
		if level > barSubLevels {
			level = barSubLevels
		}
		out = append(out, []rune(barRamp[level])[0])
	}
	return string(out)
}

// barColor picks the gradient stop for the already-sqrt-normalised magnitude
// (0..1), matching legacy pickColor(normalized, gradient).
func barColor(retro bool, normalized float64) lipgloss.Color {
	grad := speedDirectGradient
	if retro {
		grad = speedRetroGradient
	}
	idx := int(normalized * float64(len(grad)-1))
	if idx < 0 {
		idx = 0
	}
	if idx > len(grad)-1 {
		idx = len(grad) - 1
	}
	return speedGradientColour(grad[idx])
}

// stationMarkCells is a station mark's three cells in place of the bar: "S/R"
// or "S/D", red like the retrograde gradient and deepest at a speed of exactly
// zero (stop round((1 - speed/0.05) * 6), clamped to the gradient's seven
// stops). With colours off it is plain text (the specification C8).
func stationMarkCells(mark lookup.Station, speed float64, useColor bool) string {
	text := mark.String()
	if !useColor {
		return text
	}
	idx := int(math.Round((1 - speed/stationaryThreshold) * float64(len(speedRetroGradient)-1)))
	idx = max(0, min(idx, len(speedRetroGradient)-1))
	return lipgloss.NewStyle().Foreground(speedGradientColour(speedRetroGradient[idx])).Render(text)
}

// speedField is velocityBar for one Bodies row: the bar, or the station mark in
// its three cells when the row carries one (the ℞ cell stays blank). An entered
// station row's word (ov.forced) outranks the rule's mark and is drawn at the
// deepest red. Angles never carry a mark.
func speedField(p aspect.Position, ov houseOverlay, angle, useColor bool) string {
	if angle {
		return velocityBar(p.Speed, angle, useColor)
	}
	if word := ov.forced[p.Body]; word != lookup.StationNone {
		return " " + stationMarkCells(word, 0, useColor)
	}
	if mark := ov.stations[p.Body]; mark != lookup.StationNone {
		return " " + stationMarkCells(mark, p.Speed, useColor)
	}
	return velocityBar(p.Speed, angle, useColor)
}
