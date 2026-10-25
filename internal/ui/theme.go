package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// theme.go is the only place internal/ui writes a colour or a raw terminal
// escape (the specification; rulebook the design notes).
// Each role is named for what it is used for, never for the colour it happens
// to be, so changing how Sextile looks is an edit to this file alone.
// Kept by convention: no colour or escape belongs anywhere else in the
// package, and no two roles merge into one (the invariant that enforced this
// was retired 2026-10-03).
//
// Whether a colour is drawn at all stays where it is decided today
// (cfg.useColor()); this file holds values, not branches.

// Colour roles.
const (
	// focusColour marks the one place with the keyboard: the frame of the
	// panel, Details, the console or the popup that owns it, its title, the
	// active tab, a selection marker. The default; display.focus_color
	// overrides it (focusColourFor).
	focusColour = lipgloss.Color("10")
	// cursorRowBackground is the band behind the cursor row: the terminal's
	// own blue, so it follows the operator's theme. The default;
	// display.cursor_row_color overrides it (cursorRowColourFor).
	cursorRowBackground = lipgloss.Color("4")
	// frameColour is an unfocused frame, the dim tab chrome and an unfocused
	// scroll thumb.
	frameColour = lipgloss.Color("240")
	// titleColour is an unfocused title, an inactive tab and a key label.
	titleColour = lipgloss.Color("252")
	// mutedColour is the key bar's text and the chart picker's unreadable rows.
	mutedColour = lipgloss.Color("244")
	// tooSmallFrameColour frames the terminal-too-small notice.
	tooSmallFrameColour = lipgloss.Color("208")
	// errorColour is an error line's text, in the Log and the console: the
	// terminal's own red, so it follows the operator's theme
	// (the specification A1, V1).
	errorColour = lipgloss.Color("1")
	// wheelOpenColour is the Log line that says where the wheel is served. It
	// was asked for as a light green, but green means "you are here" on exactly
	// one frame (focusColour), so a second green would be a second owner of the
	// keyboard. Cyan is the nearest hue that is not that green, and like
	// errorColour it is the terminal's own, so it follows the operator's theme.
	wheelOpenColour = lipgloss.Color("6")
	// wheelClosedColour is the Log line that says the wheel was closed: the
	// error red, the terminal's own, because the wheel has gone away. It is
	// not an error (the operator asked for it), so the line keeps the error
	// lane's colour and nothing else of it.
	wheelClosedColour = errorColour
)

// errorLinePrefix marks an error line when colours are off (A2). It is drawn,
// never stored: the console copies the line without it.
const errorLinePrefix = "! "

// renderErrorLine draws one error line: in the error colour with colours on,
// behind errorLinePrefix with them off.
func renderErrorLine(text string, cfg Config) string {
	if !cfg.useColor() {
		return errorLinePrefix + text
	}
	return lipgloss.NewStyle().Foreground(errorColour).Render(text)
}

// renderLaneLine draws one stored line the way its lane is drawn: an error line
// in the error colour (or behind errorLinePrefix with colours off), the wheel
// lines in wheelOpenColour and wheelClosedColour. Colours off, a wheel line is
// its plain text: the words carry the meaning, so it needs no mark. Every other lane is its
// text as stored.
//
// w is the cells the text may take; an argv line is fitted to it by
// argvDisplay, and w <= 0 shows it whole.
func renderLaneLine(l logLine, w int, cfg Config) string {
	switch l.lane {
	case laneError:
		return renderErrorLine(l.text, cfg)
	case laneWheelOpen:
		if cfg.useColor() {
			return lipgloss.NewStyle().Foreground(wheelOpenColour).Render(l.text)
		}
	case laneWheelClosed:
		if cfg.useColor() {
			return lipgloss.NewStyle().Foreground(wheelClosedColour).Render(l.text)
		}
	case laneArgv:
		return argvDisplay(l.text, w, cfg)
	case laneNow:
		return nowLinePrefix + argvDisplay(l.text, w-ansi.StringWidth(nowLinePrefix), cfg)
	}
	return l.text
}

// Tab-strip colours, read from the roles above.
const (
	// dimTabChromeColor is the fixed gray the tabChromeDim variant paints the
	// strip chrome in, in both focus states (the unfocused border gray).
	dimTabChromeColor = frameColour
	// inactiveTabColor is the title gray every non-active tab keeps in every
	// state, so the active tab is the only green title text in the strip.
	inactiveTabColor = titleColour
)

// speedDirectGradient and speedRetroGradient are the velocity bar's 256-colour
// stops (7 each): white→blue for direct motion, white→red for retrograde.
var (
	speedDirectGradient = [7]int{255, 153, 111, 75, 69, 33, 27}
	speedRetroGradient  = [7]int{255, 217, 210, 203, 196, 160, 124}
)

// speedGradientColour turns one gradient stop into a colour.
func speedGradientColour(stop int) lipgloss.Color {
	return lipgloss.Color(strconv.Itoa(stop))
}

// focusColourFor is the focus colour this run draws: the operator's
// display.focus_color, or the default.
func focusColourFor(cfg Config) lipgloss.Color {
	if cfg.FocusColour != "" {
		return lipgloss.Color(cfg.FocusColour)
	}
	return focusColour
}

// cursorRowColourFor is the cursor-row background this run draws: the
// operator's display.cursor_row_color, or the default.
func cursorRowColourFor(cfg Config) lipgloss.Color {
	if cfg.CursorRowColour != "" {
		return lipgloss.Color(cfg.CursorRowColour)
	}
	return cursorRowBackground
}

// cursorBandStart opens the cursor row with colours on: bold over the
// cursor-row background, and no foreground, so every cell keeps its own.
// lipgloss writes the sequence, so it follows the terminal's colour profile.
func cursorBandStart(cfg Config) string {
	const probe = "X"
	r := lipgloss.NewStyle().Bold(true).Background(cursorRowColourFor(cfg)).Render(probe)
	if i := strings.Index(r, probe); i > 0 {
		return r[:i]
	}
	return sgrBold
}

// Raw escapes, for text that is assembled by hand rather than by lipgloss.
const (
	sgrBold    = "\x1b[1m"
	sgrFaint   = "\x1b[2m"
	sgrReverse = "\x1b[7m"
	// sgrReset is the one reset escape internal/ui writes.
	sgrReset = "\x1b[0m"
	// sgrIntro opens every SGR sequence; normalWeightSGR rebuilds Astrolog's
	// sequences with it.
	sgrIntro = "\x1b["
	// escapeByte is ESC itself, for telling whether a line carries any
	// escape at all.
	escapeByte = '\x1b'

	// cursorRowOff is the cursor row with colours off: bold and reverse
	// video, no colour.
	cursorRowOff = sgrBold + sgrReverse
)
