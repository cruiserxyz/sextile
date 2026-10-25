package ui

// ColorMode is the resolved colour policy for a run. The zero value
// (ColorAuto) preserves the historical behaviour: colour follows Accessible,
// so an accessible run is colourless and a normal run is coloured.
type ColorMode int

const (
	// ColorAuto emits colour unless Accessible is set. Default.
	ColorAuto ColorMode = iota
	// ColorAlways forces explicit colour even in accessible (ASCII) mode.
	ColorAlways
	// ColorNever suppresses all explicit colour, independent of glyph mode.
	ColorNever
)

// Config holds cross-cutting presentation concerns that affect every render.
// Constructed at program start (from CLI flags and environment) and carried
// by the outer Model. Never mutated, with one exception: the two display
// colours are filled once from preferences.yaml when the startup perimeter
// read lands, because that is when the file is read (applyDisplayConfig).
type Config struct {
	// Accessible substitutes ASCII glyphs for Unicode box-drawing chars.
	// Triggered by `--accessible`. Colour is a separate axis (Color) so a
	// run can be ASCII-with-colour or Unicode-without-colour.
	Accessible bool

	// Color is the resolved colour policy. Constructed at startup from the
	// `--color` flag, the SEXTILE_COLOR / NO_COLOR environment variables.
	Color ColorMode

	// FocusColour and CursorRowColour override the two colours the operator
	// may change (display.focus_color, display.cursor_row_color): a
	// 256-colour number or #rrggbb, already validated. Empty means the
	// theme's default (the specification C8, C9).
	FocusColour     string
	CursorRowColour string

	// InstanceLockUnsupported is true when the session folder's filesystem
	// cannot flock, so a second copy of Sextile cannot be detected. The model
	// says so once, on the note lane, at startup (the specification C6).
	InstanceLockUnsupported bool

	// SecondWindow is true when another Sextile holds the instance lock. This
	// window restores the session that window keeps and never writes it: not
	// at quit, not for a house choice, not before a rename.
	SecondWindow bool

	// Launch is what the launcher says about how it started this copy: the
	// value of SEXTILE_LAUNCH, `docker <image>` or `no-docker <describe>`.
	// The Log's first line names it. Any other shape, and empty, says nothing.
	Launch string
}

// useColor reports whether explicit colours should be emitted for this render.
// ColorAuto defers to Accessible so existing callers (and golden fixtures that
// build Config{Accessible: ...} directly) keep their current behaviour.
func (c Config) useColor() bool {
	switch c.Color {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	default: // ColorAuto
		return !c.Accessible
	}
}

// useASCII reports whether ASCII glyphs should replace Unicode box-drawing.
func (c Config) useASCII() bool {
	return c.Accessible
}
