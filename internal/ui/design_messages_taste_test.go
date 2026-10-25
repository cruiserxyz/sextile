package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// the specification V1: the Log panel and the console with two notes
// and two errors, at 144x40, colours on and off. Under -design both reds are
// written for the operator to choose between (A: terminal red 1, the build's;
// B: bright red 9); otherwise the shipped pair is compared with its golden.
func TestDesignMessagesV1(t *testing.T) {
	forceColor(t)
	for _, mode := range []struct {
		name string
		cfg  Config
	}{{"color", Config{Color: ColorAlways}}, {"plain", Config{Accessible: true, Color: ColorNever}}} {
		m := messagesModel(t, mode.cfg)
		frames := map[string]string{"log": m.View()}
		opened, _ := press(t, m, ",", "l")
		frames["console"] = opened.View()
		for _, view := range []string{"log", "console"} {
			frame := frames[view]
			if *designFrames == "" {
				writeDesignFrame(t, "design-messages-"+view+"-144x40-"+mode.name, frame)
				continue
			}
			if mode.name == "plain" {
				writeDesignFrame(t, "messages-"+view+"-plain", frame)
				continue
			}
			writeDesignFrame(t, "messages-"+view+"-red1", frame)
			writeDesignFrame(t, "messages-"+view+"-red9", swapForeground(t, frame, errorColour, lipgloss.Color("9")))
		}
	}
}

// swapForeground redraws every cell drawn in colour from in colour to. It is
// for the contact sheet only: the alternative is shown without a second
// build.
func swapForeground(t *testing.T, frame string, from, to lipgloss.Color) string {
	t.Helper()
	sgr := func(c lipgloss.Color) string {
		r := lipgloss.NewStyle().Foreground(c).Render("x")
		return r[:strings.Index(r, "x")]
	}
	if !strings.Contains(frame, sgr(from)) {
		t.Fatal("the frame draws nothing in the colour being swapped")
	}
	return strings.ReplaceAll(frame, sgr(from), sgr(to))
}

// V2: the settled empty states, 80x24, colours on.
func TestDesignMessagesV2(t *testing.T) {
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	own := readyModelWithSlots(t, berlinP)
	own.cfg = cfg
	own.width, own.height = 80, 24
	own.limitedViewShown = true // the first-frame notice is not what this fixture is about
	own, _ = applyTextCommit(own, FirstFrameReadyMsg{Positions: fullChartPositions()})
	own = withSceneColumns(own)
	own, _ = press(t, own, "3")

	cross := tabsPairModel(t, cfg)
	cross.width, cross.height = 80, 24
	cross.limitedViewShown = true
	cross, _ = applyTextCommit(cross, FirstFrameReadyMsg{Positions: fullChartPositions(), Aspects: tasteOwnAspects(), Overlay: true})
	cross = withSceneColumns(cross)
	cross, _ = press(t, cross, "3")

	console := tabsPairModel(t, cfg)
	console.width, console.height = 80, 24
	console.logLines = nil
	console, _ = press(t, console, ",", "l")

	for name, m := range map[string]Model{"aspects": own, "cross": cross, "console": console} {
		writeDesignFrame(t, "design-empty-"+name+"-80x24-color", m.View())
	}
}

// V3: the main bar where piece 5 hides an entry: the anchored slot focused
// (no Step day) and Aspects focused (no Select, no Move), at 144 and 80
// columns. Each is the bar row alone.
func TestDesignMessagesV3(t *testing.T) {
	forceColor(t)
	for _, size := range []int{144, 80} {
		for name, focus := range map[string]PanelID{"anchored": PanelChart, "aspects": PanelAspects} {
			m := escBase(t)
			m.cfg = Config{Color: ColorAlways}
			m.width = size
			m.focused = focus
			m.cursorIdx = 0
			lines := strings.Split(m.View(), "\n")
			writeDesignFrame(t, "design-quiet-bar-"+name+"-"+strconv.Itoa(size), lines[len(lines)-1])
		}
	}
}
