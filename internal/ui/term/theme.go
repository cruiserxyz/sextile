package term

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// theme.go is the only place the ui tree writes a colour or a raw terminal
// escape (the specification; rulebook the design notes).
// Each role is named for what it is used for, never for the colour it happens
// to be, so changing how Sextile looks is an edit to this file alone.
// Kept by convention: no colour or escape belongs anywhere else in the
// package, and no two roles merge into one (the invariant that enforced this
// was retired 2026-10-03).
//
// Whether a colour is drawn at all stays where it is decided today
// (cfg.UseColor()); this file holds values, not branches.

// Colour roles.
const (
	// DefaultFocusColour marks the one place with the keyboard: the frame of the
	// panel, Details, the console or the popup that owns it, its title, the
	// active tab, a selection marker. The default; display.focus_color
	// overrides it (focusColourFor).
	DefaultFocusColour = lipgloss.Color("10")
	// DefaultCursorRowColour is the band behind the cursor row: the terminal's
	// own blue, so it follows the operator's theme. The default;
	// display.cursor_row_color overrides it (cursorRowColourFor).
	DefaultCursorRowColour = lipgloss.Color("4")
	// FooterColour is the key bar's row, words, keys and separators alike: the
	// terminal's own blue (SGR 34 of the 16 colours, not a 256 number), so it
	// follows the user's theme as lazygit's does. NoticeColour is the notice at
	// the right end: the terminal's own cyan (SGR 36).
	FooterColour = lipgloss.Color("4")
	NoticeColour = lipgloss.Color("6")
	// LoadingColour is the Details loading label's text: a neon blue (256-colour
	// 39, #00afff) that stands out on a dark terminal (operator, 2026-11-06: "make
	// the blueish load-text colour neon-blueish for better visibility").
	LoadingColour = lipgloss.Color("39")
	// FrameColour is an unfocused frame, the dim tab chrome and an unfocused
	// scroll thumb.
	FrameColour = lipgloss.Color("240")
	// TitleColour is an unfocused title, an inactive tab and a key label.
	TitleColour = lipgloss.Color("252")
	// mutedColour is the key bar's text and the chart picker's unreadable rows.
	mutedColour = lipgloss.Color("244")
	// TooSmallFrameColour frames the terminal-too-small notice.
	TooSmallFrameColour = lipgloss.Color("208")
	// ErrorColour is an error line's text, in the Log and the console: the
	// terminal's own red, so it follows the operator's theme
	// (the specification A1, V1).
	ErrorColour = lipgloss.Color("1")
	// KeyColumnColour is a key in the Keybindings popup: the terminal's own
	// blue-green, so it follows the operator's theme.
	KeyColumnColour = lipgloss.Color("6")
	// LogFactColour is the group column of a Log panel line that states a fact:
	// the terminal's own cyan, so it follows the operator's theme.
	LogFactColour = lipgloss.Color("6")
	// LogSettingsColour is the group column of every Settings line: the
	// terminal's own yellow.
	LogSettingsColour = lipgloss.Color("3")
	// LogAstrologColour is the group column of every Astrolog line: the
	// terminal's own magenta.
	LogAstrologColour = lipgloss.Color("5")
)

// ErrorLinePrefix marks an error line when colours are off (A2). It is drawn,
// never stored: the console copies the line without it.
const ErrorLinePrefix = "! "

// RenderErrorLine draws one error line: in the error colour with colours on,
// behind ErrorLinePrefix with them off.
func RenderErrorLine(text string, cfg Style) string {
	if !cfg.UseColor() {
		return ErrorLinePrefix + text
	}
	return lipgloss.NewStyle().Foreground(ErrorColour).Render(text)
}

// RenderErrorMark draws a one-cell error mark (the bodies chooser's `!`) in the
// error colour with colours on, and leaves it plain with them off. Inside a
// faint row it steps out of faint for the mark and back in, and it ends with
// the default foreground rather than a full reset, so a bold or a band the row
// sits in goes on.
func RenderErrorMark(mark string, cfg Style) string {
	if !cfg.UseColor() {
		return mark
	}
	const probe = "X"
	r := lipgloss.NewStyle().Foreground(ErrorColour).Render(probe)
	i := strings.Index(r, probe)
	if i <= 0 {
		return mark
	}
	return SGRNormalIntensity + r[:i] + mark + SGRDefaultFg + SGRFaint
}

// Tab-strip colours, read from the roles above.
const (
	// DimTabChromeColor is the fixed gray the tabChromeDim variant paints the
	// strip chrome in, in both focus states (the unfocused border gray).
	DimTabChromeColor = FrameColour
	// InactiveTabColor is the title gray every non-active tab keeps in every
	// state, so the active tab is the only green title text in the strip.
	InactiveTabColor = TitleColour
)

// SpeedDirectGradient and SpeedRetroGradient are the velocity bar's 256-colour
// stops (7 each): white→blue for direct motion, white→red for retrograde.
var (
	SpeedDirectGradient = [7]int{255, 153, 111, 75, 69, 33, 27}
	SpeedRetroGradient  = [7]int{255, 217, 210, 203, 196, 160, 124}
)

// SpeedGradientColour turns one gradient stop into a colour.
func SpeedGradientColour(stop int) lipgloss.Color {
	return lipgloss.Color(strconv.Itoa(stop))
}

// Raw escapes, for text that is assembled by hand rather than by lipgloss.
const (
	SGRBold  = "\x1b[1m"
	SGRFaint = "\x1b[2m"
	// SGRNormalIntensity ends bold and faint; SGRDefaultFg ends a foreground.
	SGRNormalIntensity = "\x1b[22m"
	SGRDefaultFg       = "\x1b[39m"
	SGRReverse         = "\x1b[7m"
	SGRStrike          = "\x1b[9m"
	SGRNoStrike        = "\x1b[29m"
	// SGRReset is the one reset escape internal/ui writes.
	SGRReset = "\x1b[0m"
	// SGRIntro opens every SGR sequence; normalWeightSGR rebuilds Astrolog's
	// sequences with it.
	SGRIntro = "\x1b["
	// EscapeByte is ESC itself, for telling whether a line carries any
	// escape at all.
	EscapeByte = '\x1b'

	// CursorRowOff is the cursor row with colours off: bold and reverse
	// video, no colour.
	CursorRowOff = SGRBold + SGRReverse
)
