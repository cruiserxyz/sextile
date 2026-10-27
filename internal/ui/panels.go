package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Panel interface {
	Name() PanelID
	Title() string
	View(width, height int, focused bool, cfg Config) string
}

type basicPanel struct {
	name  PanelID
	title string
}

func (p basicPanel) Name() PanelID { return p.name }
func (p basicPanel) Title() string { return p.title }

func (p basicPanel) View(width, height int, focused bool, cfg Config) string {
	return renderTitledBox(p.title, width, height, focused, cfg, "")
}

// asciiBorder is the lipgloss border style used in accessible mode.
var asciiBorder = lipgloss.Border{
	Top:         "-",
	Bottom:      "-",
	Left:        "|",
	Right:       "|",
	TopLeft:     "+",
	TopRight:    "+",
	BottomLeft:  "+",
	BottomRight: "+",
}

// renderTitledBox renders a bordered panel with a single-title strip: titled
// top border + 3-sided body, with `body` filling the interior (truncated to
// width, clipped to last height-2 lines, padded with blank lines if shorter).
func renderTitledBox(title string, width, height int, focused bool, cfg Config, body string) string {
	return renderTabbedBox(tabStrip{tabs: []Tab{{Title: title}}}, width, height, focused, cfg, body)
}

// renderTabbedBox is renderTitledBox with a tab strip in the title border. A
// one-tab strip renders exactly as the plain title did.
func renderTabbedBox(strip tabStrip, width, height int, focused bool, cfg Config, body string) string {
	if width < 4 || height < 3 {
		return ""
	}

	border := lipgloss.RoundedBorder()
	leftCorner, rightCorner, dash := "╭", "╮", "─"
	if cfg.useASCII() {
		border = asciiBorder
		leftCorner, rightCorner, dash = "+", "+", "-"
	}

	borderColor := panelBorderColor(focused, cfg)
	titleColor := titleColour
	bold := false
	if focused {
		titleColor = focusColourFor(cfg)
		bold = true
	}

	top := renderTopBorder(width, strip, borderColor, titleColor, bold, cfg, leftCorner, rightCorner, dash)

	bodyStyle := lipgloss.NewStyle().
		Border(border, false, true, true, true).
		Width(width - 2).
		Height(height - 2)
	if cfg.useColor() {
		bodyStyle = bodyStyle.BorderForeground(borderColor)
	}

	return lipgloss.JoinVertical(lipgloss.Left, top, bodyStyle.Render(padBody(body, width-2, height-2, cfg)))
}

// padBody cuts each line to innerWidth (text.go), clips to the last
// innerHeight lines (autoscroll) and pads with blank rows so the output is
// exactly innerHeight lines tall.
func padBody(body string, innerWidth, innerHeight int, cfg Config) string {
	if innerWidth <= 0 || innerHeight <= 0 {
		return ""
	}
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		lines[i] = cutRight(ln, innerWidth, cfg)
	}
	if len(lines) > innerHeight {
		lines = lines[len(lines)-innerHeight:]
	}
	for len(lines) < innerHeight {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// focusMarkUnicode and focusMarkASCII open the focused frame's title while
// colours are off.
const (
	focusMarkUnicode = "▸"
	focusMarkASCII   = ">"
)

// tabChromeDim is the V1 taste switch for multi-tab strips: false renders the
// `[N] ` prefix and ` - ` separators in the title color (the operator's pick,
// matching the one-tab `[1] Chart` look); true renders them in the dim border
// gray so the active tab is the only lit text in the strip.
var tabChromeDim = false

// renderTopBorder builds exactly `width` cells: ╭─<strip>──────╮ — the strip
// abuts the border dashes with no flanking spaces. A one-tab strip is the
// prefix + title in a single styled run, byte-identical to the plain title
// border; a multi-tab strip styles prefix, separators, active and inactive
// titles apart and fits itself into width-4 cells (layoutTabStrip).
func renderTopBorder(width int, strip tabStrip, borderColor, titleColor lipgloss.Color, bold bool, cfg Config, leftCorner, rightCorner, dash string) string {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()
	if cfg.useColor() {
		borderStyle = borderStyle.Foreground(borderColor)
		titleStyle = titleStyle.Foreground(titleColor)
	}
	if bold {
		titleStyle = titleStyle.Bold(true)
	}

	maxCells := width - 4
	if maxCells < 0 {
		maxCells = 0
	}

	// With colours off the focused title is marked, because there is no green
	// to say where the keyboard is (the specification C2).
	mark := ""
	if bold && !cfg.useColor() && maxCells > 0 {
		mark = focusMarkUnicode
		if cfg.useASCII() {
			mark = focusMarkASCII
		}
		maxCells--
	}

	var segment string
	if len(strip.tabs) <= 1 {
		title := strip.prefix
		if len(strip.tabs) == 1 {
			title += strip.tabs[0].Title
		}
		title = cutRight(title, maxCells, cfg)
		segment = titleStyle.Render(mark + title)
	} else {
		segment = renderTabStrip(strip, maxCells, titleStyle, bold, cfg)
		if mark != "" {
			segment = titleStyle.Render(mark) + segment
		}
	}
	// A title that ends with a glyph a terminal may draw wide is given its space
	// before the dashes ("② in ① ─"), or the glyph would spill onto the border.
	// The cell is a real one: trailing, below, is what is left of the width.
	if endsWithRoomyGlyph(segment) {
		segment += " "
	}
	segW := lipgloss.Width(segment)

	trailing := width - 2 - 1 - segW
	if trailing < 1 {
		trailing = 1
	}

	return borderStyle.Render(leftCorner+dash) +
		segment +
		borderStyle.Render(strings.Repeat(dash, trailing)+rightCorner)
}

// renderTabStrip styles a laid-out multi-tab strip. Without color the active
// title carries the `[`/`]` marker, and focus still lands as bold on the
// prefix and the active title — the same bold a one-tab title keeps without
// color — while separators and inactive titles stay plain. With
// color the active title is green (bold only while focused), inactive titles
// stay title gray, and the chrome follows tabChromeDim.
func renderTabStrip(strip tabStrip, maxCells int, titleStyle lipgloss.Style, bold bool, cfg Config) string {
	useColor := cfg.useColor()
	runs := layoutTabStrip(strip.prefix, strip.tabs, strip.active, !useColor, maxCells, cfg)
	if !useColor {
		var sb strings.Builder
		for i, r := range runs {
			if r.role == roleActive || i == 0 {
				sb.WriteString(titleStyle.Render(r.text))
				continue
			}
			sb.WriteString(r.text)
		}
		return sb.String()
	}
	chromeStyle := titleStyle.Bold(false)
	prefixStyle := titleStyle
	if tabChromeDim {
		dim := lipgloss.NewStyle().Foreground(dimTabChromeColor)
		chromeStyle, prefixStyle = dim, dim
	}
	activeStyle := lipgloss.NewStyle().Foreground(focusColourFor(cfg)).Bold(bold)
	inactiveStyle := lipgloss.NewStyle().Foreground(inactiveTabColor)
	var sb strings.Builder
	for i, r := range runs {
		switch {
		case r.role == roleActive:
			sb.WriteString(activeStyle.Render(r.text))
		case r.role == roleInactive:
			sb.WriteString(inactiveStyle.Render(r.text))
		case i == 0:
			sb.WriteString(prefixStyle.Render(r.text))
		default:
			sb.WriteString(chromeStyle.Render(r.text))
		}
	}
	return sb.String()
}

// defaultPanels returns the static-content panels. Log is rendered separately
// by renderLogPanel because its body is dynamic per-frame (from Model state).
func defaultPanels() []Panel {
	return []Panel{
		basicPanel{name: PanelChart, title: "[1] Chart"},
		basicPanel{name: PanelBodies, title: "[2] Bodies"},
		basicPanel{name: PanelAspects, title: "[3] Aspects"},
		basicPanel{name: PanelDetails, title: detailsPrefix + focusedPanelTitle(PanelChart)},
	}
}
