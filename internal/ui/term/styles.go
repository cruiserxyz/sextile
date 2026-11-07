package term

import (
	"github.com/charmbracelet/lipgloss"
)

// Styles built from the theme's roles (theme.go).
var (
	KeyBarStyle = lipgloss.NewStyle().
		Foreground(mutedColour)
)

// ChartDropStyle greys the chart picker's unreadable-file rows: dimmer than a
// chart row because they cannot be picked, but present because a client file
// that failed to parse is exactly what the astrologer needs to know about.
var ChartDropStyle = lipgloss.NewStyle().Foreground(mutedColour)

// TemplateAsideStyle dims a body template line's aside — the criterion of a
// verdict or a qualifier — in the key bar's grey, the tab body's only second
// intensity (the specification R8).
var TemplateAsideStyle = lipgloss.NewStyle().Foreground(mutedColour)
