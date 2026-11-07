package ui

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// The Experiments popup (specs/experiments-popup.md): a hidden door behind the
// focused Log, switches remembered in preferences.yaml, and the first switch,
// enter-colour.

// experimentsHome points the preferences file (and the session folder) at a
// temp folder and returns the preferences path.
func experimentsHome(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	return store.PreferencesPath()
}

func writePrefs(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readPrefs(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// experimentsRow is the popup's enter-colour row as the frame shows it.
func experimentsRow(t *testing.T, m Model) string {
	t.Helper()
	for _, ln := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(ln, "enter-colour") {
			return ln
		}
	}
	t.Fatalf("no enter-colour row in the frame:\n%s", ansi.Strip(m.View()))
	return ""
}

func tickOn(row string) bool { return strings.Contains(row, "[✓]") }

func openExperimentsFromLog(t *testing.T) Model {
	t.Helper()
	m := logModel(t, 4)
	m, _ = press(t, m, ",", "l", "p")
	if m.currentOwner() != ownerExperiments {
		t.Fatalf("owner = %v after ,l p, want the Experiments popup", m.currentOwner())
	}
	return m
}

func TestLogPOpensExperiments(t *testing.T) {
	experimentsHome(t)
	m := openExperimentsFromLog(t)
	plain := ansi.Strip(m.View())
	for _, want := range []string{"Experiments", "enter-colour", "Enter on a Timeline aspect: its orb and the led body's name and degrees in the aspect's colour"} {
		if !strings.Contains(plain, want) {
			t.Errorf("the popup does not show %q:\n%s", want, plain)
		}
	}
	// Enter closes it, back to the focused Log.
	after, _ := press(t, m, "enter")
	if after.currentOwner() != ownerLog {
		t.Errorf("owner after Enter = %v, want the focused Log", after.currentOwner())
	}

	// From the console, where ,l lands with no Log panel on screen.
	c := logModel(t, 4)
	c.width, c.height = 80, 24
	c, _ = press(t, c, ",", "l")
	if !c.console.open {
		t.Fatal("fixture: the console did not open")
	}
	c, _ = press(t, c, "p")
	if c.currentOwner() != ownerExperiments || !strings.Contains(ansi.Strip(c.View()), "enter-colour") {
		t.Errorf("console p: owner %v, want the Experiments popup", c.currentOwner())
	}
	c, _ = press(t, c, "esc")
	if c.currentOwner() != ownerConsole {
		t.Errorf("Esc from the popup returned to %v, want the console", c.currentOwner())
	}
}

func TestExperimentsDoorIsHidden(t *testing.T) {
	experimentsHome(t)
	m := logModel(t, 4)
	for name, table := range map[string][]keys.BindingEntry{"focused Log": logBindings(), "console": consoleBindings()} {
		for _, e := range table {
			for _, k := range e.Binding.Keys() {
				if k == "p" {
					t.Errorf("the %s table binds p (%q): the door is hidden", name, e.Title)
				}
			}
			if strings.Contains(strings.ToLower(e.Title), "experiment") {
				t.Errorf("the %s table names %q", name, e.Title)
			}
		}
	}
	logged, _ := press(t, m, ",", "l")
	if bar := ansi.Strip(logged.View()); strings.Contains(bar, "Experiments") || strings.Contains(bar, "xperiment") {
		t.Errorf("the focused Log's frame names the popup:\n%s", bar)
	}
	help := ansi.Strip(keys.RenderHelp(m.keysContextFor(ownerHelp), popupChrome(m.cfg), 132, 200))
	if strings.Contains(help, "xperiment") {
		t.Errorf("Help names the popup:\n%s", help)
	}
	palette := ansi.Strip(renderActions(m.refilterActions(actionsState{}), m.blocked, m.cfg, 132, 200))
	if strings.Contains(palette, "xperiment") {
		t.Errorf("the Actions list names the popup:\n%s", palette)
	}
}

func TestExperimentsSpaceWritesThenFlips(t *testing.T) {
	path := experimentsHome(t)
	m := openExperimentsFromLog(t)
	if tickOn(experimentsRow(t, m)) {
		t.Fatal("fixture: the switch starts on")
	}
	m, cmd := press(t, m, " ")
	if cmd == nil {
		t.Fatal("Space returned no command: the write must be a command")
	}
	if tickOn(experimentsRow(t, m)) || m.experiment("enter-colour") {
		t.Fatal("the Model flipped before the write's result")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("the file exists before the command ran: %q", readPrefs(t, path))
	}
	// While that write is out, Space on the same row does nothing.
	if _, again := press(t, m, " "); again != nil {
		t.Error("a second Space while the write is out started another write")
	}
	tm, _ := m.Update(cmd())
	m = tm.(Model)
	if got := readPrefs(t, path); !strings.Contains(got, "enter-colour: on") {
		t.Errorf("preferences.yaml = %q, want enter-colour: on", got)
	}
	if !tickOn(experimentsRow(t, m)) || !m.experiment("enter-colour") {
		t.Errorf("after the result: tick %v, experiment %v, want both on", tickOn(experimentsRow(t, m)), m.experiment("enter-colour"))
	}
	m, cmd = press(t, m, " ")
	if cmd == nil {
		t.Fatal("the second Space returned no command")
	}
	tm, _ = m.Update(cmd())
	m = tm.(Model)
	if got := readPrefs(t, path); !strings.Contains(got, "enter-colour: off") {
		t.Errorf("preferences.yaml = %q, want enter-colour: off", got)
	}
	if tickOn(experimentsRow(t, m)) || m.experiment("enter-colour") {
		t.Error("the switch is still on after the off write")
	}
}

func TestExperimentsWriteFailureKeepsTheRow(t *testing.T) {
	path := experimentsHome(t)
	writePrefs(t, path, "experimental: on\n")
	m := openExperimentsFromLog(t)
	lines := len(m.logLines)
	m, cmd := press(t, m, " ")
	if cmd == nil {
		t.Fatal("Space returned no command")
	}
	tm, _ := m.Update(cmd())
	m = tm.(Model)
	plain := ansi.Strip(m.View())
	if tickOn(experimentsRow(t, m)) || m.experiment("enter-colour") {
		t.Error("a failed write changed the row")
	}
	if !strings.Contains(plain, "Not saved:") || !strings.Contains(plain, "not a block") {
		t.Errorf("the popup's foot does not say why:\n%s", plain)
	}
	if len(m.logLines) != lines {
		t.Errorf("a failed write added %d line(s) to the logs", len(m.logLines)-lines)
	}
	// The next key clears the foot.
	m, _ = press(t, m, "down")
	if strings.Contains(ansi.Strip(m.View()), "Not saved:") {
		t.Error("the foot outlived the next key")
	}
	if got := readPrefs(t, path); got != "experimental: on\n" {
		t.Errorf("the file changed: %q", got)
	}
}

func TestExperimentsRememberedAtStart(t *testing.T) {
	path := experimentsHome(t)
	writePrefs(t, path, "experimental:\n  enter-colour: on\n  gone-switch: on\n")
	msg, ok := ingest.ReadStartupPerimeter()().(ingest.StartupPerimeterReadMsg)
	if !ok {
		t.Fatal("the startup read did not answer a StartupPerimeterReadMsg")
	}
	m := logModel(t, 2)
	tm, _ := m.Update(msg)
	m = tm.(Model)
	if !m.experiment("enter-colour") {
		t.Fatal("the switch the file holds is not on after the startup read")
	}
	if m.experiment("gone-switch") {
		t.Error("an id the table does not have is on")
	}
	m, _ = press(t, m, ",", "l", "p")
	row := experimentsRow(t, m)
	if !tickOn(row) {
		t.Errorf("the tick is off at start: %q", row)
	}
	if strings.Contains(ansi.Strip(m.View()), "gone-switch") {
		t.Error("the popup shows an id the table does not have")
	}
}

// TestEveryExperimentIsRead holds R9: a row whose id no code asks for is dead,
// and graduating a switch deletes the row with its fork.
func TestEveryExperimentIsRead(t *testing.T) {
	files := sourceFiles(t)
	consts := map[string]string{}
	for _, f := range files {
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, s := range g.Specs {
				vs := s.(*ast.ValueSpec)
				for i, n := range vs.Names {
					if i < len(vs.Values) {
						if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
							if v, err := strconv.Unquote(lit.Value); err == nil {
								consts[n.Name] = v
							}
						}
					}
				}
			}
		}
	}
	read := map[string]bool{}
	for name, f := range files {
		if filepath.Base(name) == "experiments.go" {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "experiment" {
				return true
			}
			switch a := call.Args[0].(type) {
			case *ast.BasicLit:
				if v, err := strconv.Unquote(a.Value); err == nil {
					read[v] = true
				}
			case *ast.Ident:
				if v, ok := consts[a.Name]; ok {
					read[v] = true
				}
			}
			return true
		})
	}
	var dead []string
	for _, row := range experiments {
		if !read[row.id] {
			dead = append(dead, row.id)
		}
	}
	sort.Strings(dead)
	if len(dead) > 0 {
		t.Errorf("experiment id(s) %v are in the table and no code asks experiment(...) for them", dead)
	}
	if len(experiments) == 0 {
		t.Error("the table is empty")
	}
}

// enterColourModel is the entered Moon-trine-Mars sky row of markFixture with
// the switch set, colours on.
func enterColourModel(t *testing.T, on bool, cfg Config) Model {
	t.Helper()
	m, _ := markFixture(t, cfg)
	if on {
		m.experimentsOn = map[string]bool{"enter-colour": true}
	}
	m.width, m.height = 132, 40
	return enter(t, m, timelineRowWith(t, m.timeline.rows, "Moon (Sag) Tri (Leo) Mars"))
}

const trineGreen = "\x1b[32m" // the Kind SGR markFixture gives Moon trine Mars

// orbGreen is the orb text right after the aspect's colour; bold may be
// re-applied between them (BoldCell).
var orbGreen = regexp.MustCompile("\x1b\\[32m(\x1b\\[1m)?\\(0°00'")

func TestEnterColourPaintsTheOrb(t *testing.T) {
	cfg := Config{Color: ColorAlways}
	cell := func(m Model) string {
		t.Helper()
		_, aspects := leftPanels(t, m.View())
		for _, ln := range aspects {
			if i := strings.Index(ln, term.SGRBold); i >= 0 {
				return ln[i:]
			}
		}
		t.Fatalf("no bold cell:\n%s", strings.Join(aspects, "\n"))
		return ""
	}
	on := cell(enterColourModel(t, true, cfg))
	off := cell(enterColourModel(t, false, cfg))

	// Switch on: the orb text carries the glyph's own Kind SGR, inside the bold.
	if !strings.HasPrefix(on, term.SGRBold) || !orbGreen.MatchString(on) {
		t.Errorf("switch on: the orb is not in the aspect's colour, bold kept:\n%q", on)
	}
	// The glyphs keep their own colours (red Moon, green trine, blue Mars).
	for _, sgr := range []string{"\x1b[31m", trineGreen, "\x1b[34m"} {
		if !strings.Contains(on, sgr) {
			t.Errorf("switch on: the cell lost the glyph colour %q:\n%q", sgr, on)
		}
	}
	// Switch off: today's bold cell, with no colour on the orb.
	if orbGreen.MatchString(off) || !strings.HasPrefix(off, term.SGRBold) {
		t.Errorf("switch off: the cell is not today's bold cell:\n%q", off)
	}
	if ansi.Strip(on) != ansi.Strip(off) {
		t.Errorf("the switch changed the cell's text: %q vs %q", ansi.Strip(on), ansi.Strip(off))
	}
	// Colours off: the switch changes nothing at all.
	plainOn := enterColourModel(t, true, Config{Color: ColorNever}).View()
	plainOff := enterColourModel(t, false, Config{Color: ColorNever}).View()
	if plainOn != plainOff {
		t.Error("with colours off the switch changed the frame")
	}
}

func TestEnterColourPaintsTheLedBodiesRow(t *testing.T) {
	cfg := Config{Color: ColorAlways}
	moonRow := func(m Model) string {
		t.Helper()
		bodies, _ := leftPanels(t, m.View())
		for _, ln := range bodies {
			if strings.Contains(ansi.Strip(ln), "Moon") {
				return ln
			}
		}
		t.Fatalf("no Moon row:\n%s", strings.Join(bodies, "\n"))
		return ""
	}
	on, off := enterColourModel(t, true, cfg), enterColourModel(t, false, cfg)
	rowOn, rowOff := moonRow(on), moonRow(off)
	if rowOn == rowOff {
		t.Fatal("the switch changed nothing on the led row")
	}
	if ansi.Strip(rowOn) != ansi.Strip(rowOff) {
		t.Errorf("the switch changed the row's text: %q vs %q", ansi.Strip(rowOn), ansi.Strip(rowOff))
	}
	i := strings.Index(rowOn, trineGreen)
	if i < 0 {
		t.Fatalf("the led row carries no aspect colour: %q", rowOn)
	}
	coloured := rowOn[i:]
	if !strings.Contains(ansi.Strip(coloured[:strings.Index(coloured, term.SGRReset)+len(term.SGRReset)]), "Moon") {
		t.Errorf("the name is not in the aspect's colour: %q", rowOn)
	}
	// The glyph column before the colour is as today; so is everything after
	// the position part (speed lane and house column).
	if !strings.HasPrefix(rowOff, rowOn[:i]) {
		t.Errorf("the glyph part changed:\n on %q\noff %q", rowOn[:i], rowOff)
	}
	if !strings.Contains(ansi.Strip(coloured), "Sag") && !strings.Contains(ansi.Strip(coloured), "°") {
		t.Errorf("the degrees are outside the colour: %q", coloured)
	}
	// Every other Bodies row is as today.
	bOn, _ := leftPanels(t, on.View())
	bOff, _ := leftPanels(t, off.View())
	if len(bOn) != len(bOff) {
		t.Fatalf("the Bodies panels differ in height: %d vs %d", len(bOn), len(bOff))
	}
	for k := range bOn {
		if strings.Contains(ansi.Strip(bOn[k]), "Moon") {
			continue
		}
		if bOn[k] != bOff[k] {
			t.Errorf("a row other than the led one changed:\n on %q\noff %q", bOn[k], bOff[k])
		}
	}
	// Colours off: the row is today's.
	noColour := Config{Color: ColorNever}
	if a, b := moonRow(enterColourModel(t, true, noColour)), moonRow(enterColourModel(t, false, noColour)); a != b {
		t.Errorf("with colours off the led row changed: %q vs %q", a, b)
	}
}

func TestExperimentsAreSilent(t *testing.T) {
	path := experimentsHome(t)
	m := logModel(t, 3)
	lines := len(m.logLines)
	before := ansi.Strip(m.View())

	m, _ = press(t, m, ",", "l", "p")
	m, cmd := press(t, m, " ")
	tm, _ := m.Update(cmd())
	m = tm.(Model)
	m, _ = press(t, m, "esc")
	m, _ = press(t, m, "esc") // out of the Log focus
	if len(m.logLines) != lines {
		t.Errorf("opening and flipping added %d log line(s)", len(m.logLines)-lines)
	}
	// With the switch on the main screen is what a switch itself changes: with
	// nothing entered, nothing.
	if after := ansi.Strip(m.View()); after != before {
		t.Errorf("flipping a switch changed the main screen:\n%s\nvs\n%s", before, after)
	}

	// A failed flip.
	writePrefs(t, path, "experimental: on\n")
	f := logModel(t, 3)
	n := len(f.logLines)
	f, _ = press(t, f, ",", "l", "p")
	f, cmd = press(t, f, " ")
	tm, _ = f.Update(cmd())
	f = tm.(Model)
	if len(f.logLines) != n {
		t.Errorf("a failed flip added %d log line(s)", len(f.logLines)-n)
	}

	// A start with the switch on.
	writePrefs(t, path, "experimental:\n  enter-colour: on\n")
	msg := ingest.ReadStartupPerimeter()().(ingest.StartupPerimeterReadMsg)
	s := logModel(t, 3)
	n = len(s.logLines)
	tm, _ = s.Update(msg)
	s = tm.(Model)
	if len(s.logLines) != n {
		var added []string
		for _, l := range s.logLines[n:] {
			added = append(added, l.text)
		}
		t.Errorf("a start with the switch on added %d log line(s): %s", len(added), fmt.Sprint(added))
	}
}
