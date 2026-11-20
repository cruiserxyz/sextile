package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// borderFocused reads the focus fact the way the operator does: the title of
// the panel's top border carries the focus mark.
func borderFocused(t *testing.T, m Model, id PanelID) bool {
	t.Helper()
	r, ok := m.geometry()[id]
	if !ok {
		t.Fatalf("no %v panel in this layout", id)
	}
	if m.cfg.UseColor() {
		s := parseDesignScreen(m.View())
		return s[r.y][r.x].fg == styleFG(t, pane.BorderColour(true, m.cfg))
	}
	top := ansi.Strip(ansi.Cut(frameRows(m.View())[r.y], r.x, r.x+r.w))
	return strings.Contains(top, focusMarkUnicode) || strings.Contains(top, focusMarkASCII)
}

// cursorRowLook reads the cursor row of a list in a panel by a text only that
// row holds: whether it carries the band (colours on) or reverse (colours
// off), and whether it is bold.
func cursorRowLook(t *testing.T, m Model, id PanelID, text string) (lit, bold bool) {
	t.Helper()
	s := parseDesignScreen(m.View())
	y := rowWith(s, m.geometry()[id], text)
	if y < 0 {
		t.Fatalf("the cursor row %q is not on screen", text)
	}
	cells := cellsOf(s, y, text)
	if len(cells) == 0 {
		t.Fatalf("no cells for %q", text)
	}
	c := cells[0]
	lit = c.bg != "" || c.reverse
	return lit, c.bold
}

func attachedTimelineText(m Model) string {
	return prefixOf(panel.AlignRow(m.timelineRows()[m.details.cursor]), 24)
}

// Details attached on a timeline, then `,L` takes the keyboard: the border
// loses focus and the cursor row goes bold alone. Back, and the band returns.
func TestDetailsCursorFollowsItsBorder(t *testing.T) {
	forceColor(t)
	for _, cfg := range []Config{{Color: ColorAlways}, {Accessible: true, Color: ColorNever}} {
		for _, owner := range []string{"console", "focused Log"} {
			t.Run(owner, func(t *testing.T) {
				m := onTimeline(t, cfg)
				m = atSize(t, m, 200, 60)
				text := attachedTimelineText(m)
				if !borderFocused(t, m, PanelDetails) {
					t.Fatal("premise: Details' border shows focus")
				}
				if lit, bold := cursorRowLook(t, m, PanelDetails, text); !lit || !bold {
					t.Fatalf("attached: lit %v bold %v, want the band", lit, bold)
				}
				keys := []string{",", "L"}
				if owner == "console" {
					keys = append(keys, "l")
				}
				m2, _ := press(t, m, keys...)
				if owner == "console" {
					if !m2.console.open {
						t.Fatal("premise: the console is open")
					}
					// The console covers Details: nothing of its cursor is drawn.
					return
				}
				if !m2.logFocus.active {
					t.Fatalf("premise: the Log has the focus (on screen %v, console %v, %dx%d)", m2.logOnScreen(), m2.console.open, m2.width, m2.height)
				}
				if borderFocused(t, m2, PanelDetails) {
					t.Error("Details' border still shows focus while the Log has the keyboard")
				}
				if lit, bold := cursorRowLook(t, m2, PanelDetails, text); lit || !bold {
					t.Errorf("under the Log: lit %v bold %v, want bold alone", lit, bold)
				}
				m3, _ := press(t, m2, "esc")
				if !borderFocused(t, m3, PanelDetails) {
					t.Fatal("premise: Esc gives Details its focus back")
				}
				if lit, _ := cursorRowLook(t, m3, PanelDetails, text); !lit {
					t.Error("the band did not come back")
				}
			})
		}
	}
}

// cursorStateSites finds every function of the screen packages that decides
// how a cursor row is drawn, by the call of CursorRowState or RenderRowState:
// a new one fails the test below until cursorRowSources says where its lit
// state comes from.
func cursorStateSites(t *testing.T) []string {
	t.Helper()
	found := map[string]bool{}
	for _, dir := range []string{".", "panel", "menu"} {
		paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range paths {
			if strings.HasSuffix(p, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range f.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Body == nil || fn.Name.Name == "CursorRowState" || fn.Name.Name == "RenderRow" {
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					c, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					name := ""
					switch f := c.Fun.(type) {
					case *ast.SelectorExpr:
						name = f.Sel.Name
					case *ast.Ident:
						name = f.Name
					}
					if name == "CursorRowState" || name == "RenderRowState" {
						found[f.Name.Name+"."+fn.Name.Name] = true
					}
					return true
				})
			}
		}
	}
	var out []string
	for k := range found {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// cursorRowSources says, for each function that draws a cursor row, where its
// lit state comes from: the border focus of its own panel (a parameter the
// panel's compose call fills from the one fact that colours the border), or a
// popup's own list, which has the keyboard while the popup is open.
var cursorRowSources = map[string]string{
	"ui.chartSlotsBodyLayout": "border",
	"ui.consoleRow":           "border",
	"panel.RenderTimeline":    "border",
	"panel.layoutBodyRows":    "border",
	"menu.Row":                "popup",
}

func TestEveryCursorRowTakesItsLitStateFromTheBorder(t *testing.T) {
	forceColor(t)
	sites := cursorStateSites(t)
	for _, s := range sites {
		if _, ok := cursorRowSources[s]; !ok {
			t.Errorf("%s draws a cursor row and is not in cursorRowSources", s)
		}
	}
	for s := range cursorRowSources {
		if !slices.Contains(sites, s) {
			t.Errorf("cursorRowSources names %s, which draws no cursor row any more", s)
		}
	}

	type entry struct {
		name  string
		panel PanelID
		model func(t *testing.T, cfg Config, focus bool) Model
		text  func(m Model) string
	}
	entries := []entry{
		{"chart slots", PanelChart, func(t *testing.T, cfg Config, focus bool) Model {
			m := designBase(t, cfg)
			m.focused = PanelChart
			// Details attached takes the keyboard: the panel it was entered from stays drawn, its border unfocused.
			m.details.attached = !focus
			return m
		}, func(m Model) string { return strconv.Itoa(m.slots.All()[m.cursorIdx].Time.Year()) }},
		{"bodies", PanelBodies, func(t *testing.T, cfg Config, focus bool) Model {
			m := designBase(t, cfg)
			m.focused = PanelBodies
			m.details.attached = !focus
			return m
		}, func(m Model) string {
			planets, _ := panel.BodyRows(m.chart.positions, nil, plainColour, panel.HouseOverlay{})
			return prefixOf(planets[m.bodyCursorIdx].Text, 10)
		}},
		{"timeline", PanelDetails, func(t *testing.T, cfg Config, focus bool) Model {
			m := onTimeline(t, cfg)
			m = atSize(t, m, 200, 60)
			if !focus {
				m, _ = press(t, m, ",", "L")
			}
			return m
		}, attachedTimelineText},
	}
	for _, cfg := range []Config{{Color: ColorAlways}, {Accessible: true, Color: ColorNever}} {
		for _, e := range entries {
			for _, focus := range []bool{true, false} {
				t.Run(e.name, func(t *testing.T) {
					m := e.model(t, cfg, focus)
					border := borderFocused(t, m, e.panel)
					if border != focus {
						t.Fatalf("premise: border focus %v, want %v", border, focus)
					}
					lit, bold := cursorRowLook(t, m, e.panel, e.text(m))
					if lit != border {
						t.Errorf("border focus %v but the cursor row's band is %v", border, lit)
					}
					if !border && !bold {
						t.Errorf("an unfocused cursor row is not bold")
					}
				})
			}
		}
	}
}

func atSize(t *testing.T, m Model, w, h int) Model {
	t.Helper()
	tm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return tm.(Model)
}

// A popup's own list has the keyboard while the popup is open: its cursor row
// is lit, whatever the panels under it show.
func TestPopupListIsLitWhileOpen(t *testing.T) {
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	band := styleBG(t, term.DefaultCursorRowColour)
	seen := 0
	for _, l := range cursorLists() {
		if !l.popup {
			continue
		}
		seen++
		t.Run(l.name, func(t *testing.T) {
			m := l.model(t, cfg)
			for _, id := range []PanelID{PanelChart, PanelBodies} {
				if borderFocused(t, m, id) {
					t.Fatalf("premise: the %v border shows focus under a popup", id)
				}
			}
			rows := parseDesignScreen(m.View()).rowsWhere(func(c designCell) bool { return c.bg == band })
			if len(rows) != 1 {
				t.Errorf("the popup's cursor row is lit on %d rows, want 1", len(rows))
			}
		})
	}
	if seen == 0 {
		t.Fatal("no popup list in cursorLists")
	}
}

// The console's cursor row is lit exactly when its border shows focus: both
// read the one focused argument of renderConsole.
func TestConsoleCursorFollowsItsBorder(t *testing.T) {
	forceColor(t)
	band := styleBG(t, term.DefaultCursorRowColour)
	m, _ := press(t, designBase(t, Config{Color: ColorAlways}), ",", "L", "l")
	r, ok := m.consoleRect()
	if !ok {
		t.Fatal("no console")
	}
	for _, focused := range []bool{true, false} {
		box := renderConsole(m.console.snapshot, m.console, m.tabStrips()[PanelLog], r.w, r.h, focused, m.cfg)
		s := parseDesignScreen(box)
		border := s[0][0].fg == styleFG(t, pane.BorderColour(true, m.cfg))
		lit := len(s.rowsWhere(func(c designCell) bool { return c.bg == band })) == 1
		if border != focused || lit != focused {
			t.Errorf("focused %v: border shows focus %v, cursor row lit %v", focused, border, lit)
		}
	}
}
