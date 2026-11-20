package ui

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/muesli/termenv"
)

// A scope literal passed to a Log writer must be in the table that gives it a
// group. A scope the table lacks is this failure, never a silent fallback.
func TestEveryLogScopeHasAGroup(t *testing.T) {
	seen := map[string]bool{}
	for _, method := range []string{"logNote", "logBare", "logError", "logErrorLine", "logTrace"} {
		for _, site := range callSites(t, method) {
			if site.literal == "" {
				continue // a scope the call receives, checked below by its source
			}
			seen[site.literal] = true
			if _, ok := scopeGroups[site.literal]; !ok {
				t.Errorf("%s in %s (%s): scope %q has no group in scopeGroups", method, site.enclosing, site.file, site.literal)
			}
		}
	}
	// Scopes that reach a writer as a value: the copy plans, the lists, the
	// frame choices and the Details copy.
	for _, scope := range []string{"log", "chart", "bodies", "aspects", "details", "moon", "timeline", "houses", "zodiac", "wheel"} {
		if _, ok := scopeGroups[scope]; !ok {
			t.Errorf("scope %q reaches a writer as a value and has no group", scope)
		}
	}
	if len(seen) < 15 {
		t.Fatalf("only %d scope literals found: the writer names no longer match internal/ui", len(seen))
	}
	// The set is closed: every one of the 15 groups takes at least one scope.
	taken := map[logGroup]bool{}
	for _, g := range scopeGroups {
		taken[g] = true
	}
	for g := groupChart; g <= groupSextile; g++ {
		if !taken[g] {
			t.Errorf("group %s takes no scope", g)
		}
	}
	if got := len(logGroupNames) - 1; got != 15 {
		t.Errorf("%d groups, want 15", got)
	}
}

// The Log panel: every line is its group padded to eight cells, one space, and
// the message from column 10. A long line is cut with … at the edge.
func TestLogPanelGroupColumn(t *testing.T) {
	cfg := Config{Accessible: true, Color: ColorNever}
	scopes := map[logGroup]string{}
	for scope, g := range scopeGroups {
		if _, ok := scopes[g]; !ok || scope < scopes[g] {
			scopes[g] = scope
		}
	}
	m := newTestModel(t, cfg, FrozenClock(testFrozenTime()))
	for g := groupChart; g <= groupSextile; g++ {
		m = m.logNote(scopes[g], "message of %s", g)
	}
	m = m.logError("wheel", "the picture did not open in an image viewer")
	lines := m.logPanelLines()
	if len(lines) != 16 {
		t.Fatalf("%d panel lines, want 16: %q", len(lines), lines)
	}
	for i, row := range m.logPanelRows() {
		cells := []rune(ansi.Strip(row))
		if len(cells) < 10 {
			t.Fatalf("row %d is %q", i, row)
		}
		group := strings.TrimRight(string(cells[:8]), " ")
		if !strings.Contains(strings.Join(logGroupNames[:], " "), group) || group == "" {
			t.Errorf("row %d starts with %q, which is no group", i, group)
		}
		if cells[8] != ' ' || cells[9] == ' ' {
			t.Errorf("row %d: the message does not start in column 10: %q", i, string(cells))
		}
	}

	t.Run("a long line is cut at the edge", func(t *testing.T) {
		m := modelWithSlot(t)
		m.cfg = Config{Color: ColorNever}
		m.startup = startupReady
		m.width, m.height = 132, 40
		m, _ = applyTextCommit(m, FirstFrameReadyMsg{Positions: fullChartPositions()})
		m.logLines = nil
		long := "a very long message " + strings.Repeat("word ", 40) + "end"
		m = m.logNote("chart", "%s", long)
		rows := logPanelRows(t, m)
		var shown []string
		for _, r := range rows {
			if strings.TrimSpace(r) != "" {
				shown = append(shown, r)
			}
		}
		if len(shown) != 1 {
			t.Fatalf("the long line takes %d rows, want 1: %q", len(shown), shown)
		}
		if !strings.HasPrefix(shown[0], "Chart    a very long message") || !strings.HasSuffix(shown[0], "…") {
			t.Errorf("the long line reads %q, want group, message and a closing …", shown[0])
		}
	})
}

func sgr(code, text string) string { return "\x1b[" + code + "m" + text + "\x1b[0m" }

// Colours B in the terminal's own 16: the group column by kind, the message
// plain, an error red in both. With colours off, the group column alone and
// `! ` before an error's message.
func TestLogPanelColours(t *testing.T) {
	m := newTestModel(t, Config{}, FrozenClock(testFrozenTime()))
	fact := m.logNote("chart", "3 rows copied").logPanelEntries()[0]
	settings := m.logNote("settings", "colour %q not valid, ignored", "neon").logPanelEntries()[0]
	astrolog := m.logNote("astrolog", "%q", "Too many transit events found").logPanelEntries()[0]
	failure := m.logError("wheel", "the picture did not open in an image viewer").logPanelEntries()[0]

	for _, profile := range []struct {
		name string
		p    termenv.Profile
	}{{"256 colours", termenv.ANSI256}, {"16 colours", termenv.ANSI}} {
		t.Run(profile.name, func(t *testing.T) {
			prev := lipgloss.ColorProfile()
			lipgloss.SetColorProfile(profile.p)
			t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
			cfg := Config{Color: ColorAlways}
			for _, c := range []struct {
				name string
				l    logLine
				want string
			}{
				{"fact", fact, sgr("36", "Chart   ") + " 3 rows copied"},
				{"settings", settings, sgr("33", "Settings") + ` colour "neon" not valid, ignored`},
				{"astrolog", astrolog, sgr("35", "Astrolog") + ` "Too many transit events found"`},
				{"error", failure, sgr("31", "Wheel   ") + " " + sgr("31", "the picture did not open in an image viewer")},
			} {
				if got := renderPanelLine(c.l, cfg); got != c.want {
					t.Errorf("%s: got %q, want %q", c.name, got, c.want)
				}
			}
		})
	}

	plain := Config{Accessible: true, Color: ColorNever}
	for _, c := range []struct {
		name string
		l    logLine
		want string
	}{
		{"fact", fact, "Chart    3 rows copied"},
		{"error", failure, "Wheel    ! the picture did not open in an image viewer"},
	} {
		if got := renderPanelLine(c.l, plain); got != c.want {
			t.Errorf("colours off, %s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// The words of every Log line in the source: lower case unless a name, no
// closing full stop, no "Couldn't", no chord or key, no colon, and the six
// rows of the page's before and after table appear as decided.
var (
	chordWords = regexp.MustCompile(`Ctrl\+|, c [a-z]\b|\bpress |\bPress |,L\b`)
	verbs      = regexp.MustCompile(`%(\[\d+\])?[-+# 0-9.]*[a-zA-Z%]`)
)

func TestLogMessagesAreFacts(t *testing.T) {
	seen := 0
	for _, method := range []string{"logNote", "logBare", "logError", "logErrorLine"} {
		for _, site := range callSites(t, method) {
			if site.format == "" {
				continue
			}
			seen++
			msg := verbs.ReplaceAllString(site.format, "")
			switch {
			case strings.HasSuffix(strings.TrimRight(site.format, " "), "."):
				t.Errorf("%s in %s: %q ends with a full stop", method, site.enclosing, site.format)
			case strings.Contains(site.format, "Couldn't"), strings.Contains(site.format, "couldn't"):
				t.Errorf("%s in %s: %q keeps the error voice", method, site.enclosing, site.format)
			case chordWords.MatchString(site.format):
				t.Errorf("%s in %s: %q names a key or chord", method, site.enclosing, site.format)
			case strings.Contains(msg, ":"):
				t.Errorf("%s in %s: %q has a colon", method, site.enclosing, site.format)
			}
		}
	}
	if seen < 60 {
		t.Fatalf("only %d messages found: the writer names no longer match internal/ui", seen)
	}
}

// The page's six before and after rows, as the lines are written now.
func TestLogLanguageRowsOfThePage(t *testing.T) {
	m := newTestModel(t, Config{}, FrozenClock(testFrozenTime()))
	rows := func(f func(Model) Model) []string { return f(m).logPanelLines() }

	got := rows(func(m Model) Model { return m.logNote("chart", "%d %s copied", 3, "rows") })
	if want := []string{"Chart    3 rows copied"}; !reflect.DeepEqual(got, want) {
		t.Errorf("copied rows: %q, want %q", got, want)
	}
	got = rows(func(m Model) Model { m, _ = m.originalsLine("protected, Sextile writes nothing here"); return m })
	if want := []string{"Folder   protected, Sextile writes nothing here"}; !reflect.DeepEqual(got, want) {
		t.Errorf("protected folder: %q, want %q", got, want)
	}
	got = rows(func(m Model) Model { return m.logNote("wheel", "the picture is at %s", "/tmp/sextile/p.png") })
	if want := []string{"Wheel    the picture is at /tmp/sextile/p.png"}; !reflect.DeepEqual(got, want) {
		t.Errorf("wheel by hand: %q, want %q", got, want)
	}
	got = rows(func(m Model) Model {
		return m.logError("wheel", "%s", m.failureReason(fmt.Errorf("x"), wheelPublishFallback))
	})
	if want := []string{"Wheel    the picture did not open in an image viewer"}; !reflect.DeepEqual(got, want) {
		t.Errorf("wheel failed: %q, want %q", got, want)
	}
	got = rows(func(m Model) Model { return m.applyDisplayConfig(store.DisplayConfig{FocusColour: "neon"}) })
	if want := []string{`Settings colour "neon" not valid, ignored`}; !reflect.DeepEqual(got, want) {
		t.Errorf("bad colour: %q, want %q", got, want)
	}
	got = rows(func(m Model) Model { return m.logNote("city", "%s, from the computer's time zone", "Amsterdam, NL") })
	if want := []string{"City     Amsterdam, NL, from the computer's time zone"}; !reflect.DeepEqual(got, want) {
		t.Errorf("city: %q, want %q", got, want)
	}
}

// The large Log: a note reads `scope: text`, an error the same behind `! `
// with colours off, a command line is bare.
func TestConsoleKeepsTheScope(t *testing.T) {
	m := newTestModel(t, Config{}, FrozenClock(testFrozenTime()))
	m = m.logNote("chart", "3 rows copied").logError("wheel", "the picture did not open in an image viewer").
		logCommands([]string{"astrolog -qj 2461156.000000"})
	var got []string
	for _, l := range m.consoleLines() {
		got = append(got, renderLaneLine(l, 0, Config{Accessible: true, Color: ColorNever}))
	}
	want := []string{"chart: 3 rows copied", "! wheel: the picture did not open in an image viewer"}
	if len(got) != 3 || !reflect.DeepEqual(got[:2], want) || strings.Contains(got[2], ": ") {
		t.Errorf("console lines = %q, want %q and a bare command", got, want)
	}
}
