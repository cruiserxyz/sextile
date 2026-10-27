package ui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// All chart and place values in this file are constructed synthetic test
// inputs.

// Every popup fits its content (operator, 2026-10-27: "make the popup windows
// adjust their size to what they actually need. everywhere. as a rule").
//
// A popup's box is as wide as its widest line plus the border and padding, and
// as tall as its rows, and no wider or taller; only a search line or a form
// field may ask for more, because a field needs room to type. The guard below
// opens every popup at both supported sizes, in both looks, in several
// states, and holds each to that.
//
// "No wider than its content" cannot be read off the picture: a list row pads
// itself out to the width it is given, so a box that is too wide still has
// rows that reach its edge. The guard instead takes ONE cell off the box
// (modalFitProbe) and asks whether anything was cut or pushed out of its row.
// If nothing was, the cell was spare. Rows are asked the same way.

// fitSizes are the two terminal sizes the program supports.
var fitSizes = []struct{ w, h int }{{132, 40}, {80, 24}}

// fitModes are the two looks: colour, and colourless ASCII-safe.
var fitModes = []struct {
	name string
	cfg  Config
}{{"color", Config{Color: ColorAlways}}, {"accessible", Config{Accessible: true, Color: ColorNever}}}

// fitStep is something the operator does while a popup is open.
type fitStep struct {
	name string
	do   func(t *testing.T, m Model) Model
}

// fitCase is one popup in one state.
type fitCase struct {
	// popup is the renderer's own name: the function in the program that
	// calls renderModalBox or renderForm for it. The enumeration test holds
	// the table to the calls the program actually has.
	popup string
	state string

	// group names the states that must be one size: a popup whose content
	// changes with the cursor or with a step is decided once, for the fullest,
	// and holds it (operator, 2026-10-27). Empty: the state is its own group.
	group string

	// model is the popup open on a ready workspace; render draws its box.
	model  func(t *testing.T, cfg Config, w, h int) Model
	render func(m Model, cfg Config, w, h int) string

	// steps are what is done to it while it is open; its size must not move.
	steps []fitStep

	// title is the name in the box's top edge, as drawn.
	title string
	// typing is the cells a search line or a form's field asks for beyond
	// its content: zero for a popup with neither. It is the documented floor
	// (modalSearchRoom, modalFieldRoom, modalRefusalRoom), written out here in
	// cells so a change to the constants has to change this table too.
	typing int
	// inputs is how many input lines the popup has: 1 for a search line, the
	// field count for a form, 0 for none.
	inputs int
	// fill is a substring of a row that takes the width it is given by
	// design (the Bodies tag bar lists chosen names until it runs out of
	// room): the row is left out of the comparison, and the room it asks for is
	// part of typing, written out as the floor it is.
	fill string
	// blanks is how many blank rows the content has on purpose (the line
	// before the house display's example, the line kept for a refusal, the
	// rows the City list keeps for results); -1 when the state is a smaller
	// step of a popup sized for a larger one, or the content is a document with
	// blank lines of its own (Help).
	blanks int
	// zone is whether the popup keeps the message zone (modalBox.Zone): a
	// blank row and a row for a message, on top of the rows the terminal gives.
	zone bool
	// endsBlank is whether the last content row is the blank line kept for
	// something that is not there yet (a refusal, the Bodies note); otherwise
	// the content ends in a row with something in it.
	endsBlank bool
}

// open draws the popup as it is when it opens.
func (c fitCase) open(t *testing.T, cfg Config, w, h int) string {
	return c.render(c.model(t, cfg, w, h), cfg, w, h)
}

// fitBase is a ready workspace in a sandbox, at the given size and look.
func fitBase(t *testing.T, cfg Config, w, h int) Model {
	t.Helper()
	m := sessionsModel(t, 3)
	m.cfg = cfg
	m.width, m.height = w, h
	return m
}

func fitPlaces() []location.PlaceInfo {
	return []location.PlaceInfo{
		{Name: "Amsterdam", Country: "NL", TZ: "Europe/Amsterdam"},
		{Name: "Hamburg", Country: "DE", TZ: "Europe/Berlin"},
		{Name: "Hamilton", Country: "CA", TZ: "America/Toronto"},
		{Name: "Hamm", Country: "DE", TZ: "Europe/Berlin"},
		{Name: "Bremen", Country: "DE", TZ: "Europe/Berlin"},
		{Name: "Llanfairpwllgwyngyll", Country: "GB", TZ: "Europe/London"},
		{Name: "Karachi University Employees Co-operative Housing Society", Country: "PK", TZ: "Asia/Karachi"},
		{Name: "Ho Chi Minh City", Country: "VN", TZ: "Asia/Ho_Chi_Minh"},
	}
}

func fitCharts(n int) []store.NamedChart {
	out := make([]store.NamedChart, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("Client %02d", i)
		switch i {
		case 1:
			name = "A client with a rather long name indeed"
		case 2:
			name = "Femke Jansen"
		case 3:
			name = "Fem de Vries"
		}
		out = append(out, testNamedChart(name, location.PlaceInfo{Name: "Bremen", Country: "DE", Lat: 53.07, Lon: 8.8, TZ: "Europe/Berlin"}))
	}
	return out
}

func fitSessions() []store.NamedSession {
	return []store.NamedSession{
		{Name: "Monday clients", File: "monday-clients.yaml"},
		{Name: "Q", File: "q.yaml", OpenElsewhere: true},
		{Name: "A session with a long name", File: "a-session.yaml"},
	}
}

// fitEquinox is an untitled chart file with a place label, the title form's usual guest.
func fitEquinox() store.NamedChart {
	return store.NamedChart{ID: "equinox-2026-09", Name: "Equinox-2026-09", Untitled: true, PlaceLabel: "Bremen, Germany",
		Place:  location.PlaceInfo{Name: "Bremen", Country: "DE", Lat: 53.07, Lon: 8.8, TZ: "Europe/Berlin"},
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "Equinox-2026-09.as"}}
}

func fitBodyState(filter string) bodyModalState {
	st := listRowsBodyState()
	st.filterText = filter
	st.refilter()
	st.cursor = groupRows()
	return st
}

// fitTyped presses each letter of s in turn.
func fitTyped(s string) func(t *testing.T, m Model) Model {
	return func(t *testing.T, m Model) Model {
		for _, r := range s {
			m, _ = press(t, m, string(r))
		}
		return m
	}
}

// fitKeys presses each key in turn.
func fitKeys(names ...string) func(t *testing.T, m Model) Model {
	return func(t *testing.T, m Model) Model {
		for _, k := range names {
			m, _ = press(t, m, k)
		}
		return m
	}
}

// listSteps are what a person does in a popup with a search line: type, take
// it back, walk to the bottom and the top.
func listSteps(word string, extra ...fitStep) []fitStep {
	steps := []fitStep{
		{"type " + word, fitTyped(word)},
		{"backspace", fitKeys(slicesRepeat("backspace", len(word))...)},
		{"type " + word + " again", fitTyped(word)},
		{"cursor down", fitKeys("down", "down", "down")},
		{"cursor to the bottom", fitKeys("end", "pgdown")},
		{"cursor to the top", fitKeys("home", "pgup", "up")},
	}
	return append(steps, extra...)
}

func slicesRepeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

// fitCases is every popup the program has, as it is when it opens, with what a
// person does to it while it is open.
func fitCases() []fitCase {
	const searchPrompt, bodiesPrompt = "Search: ", "Search bodies: "
	search := len(searchPrompt) + 24 + 1
	// A form is the width of the line kept for a refusal.
	form := modalProseWidth

	atlas := func(state string, filter string) fitCase {
		return fitCase{popup: "renderAtlasModal", state: state, group: "atlas", title: "City", typing: search, inputs: 1, blanks: -1,
			model: func(t *testing.T, cfg Config, w, h int) Model {
				m := fitBase(t, cfg, w, h)
				m.startup = startupAtlasOpen
				m.atlas.index = location.NewIndex(append(fitPlaces(), syntheticPlaces(80)...))
				m.atlas.filterText = filter
				m.atlas.refilter()
				return m
			},
			render: func(m Model, cfg Config, w, h int) string { return renderAtlasModal(m.atlas, cfg, w, h) },
			steps:  listSteps("hamb", fitStep{"a name too long for its row", fitTyped("karachi")}),
		}
	}
	chart := func(state string, charts []store.NamedChart, drops []store.ChartDrop) fitCase {
		return fitCase{popup: "renderChartModal", state: state, title: "Open chart", typing: search, inputs: 1,
			model: func(t *testing.T, cfg Config, w, h int) Model {
				m := fitBase(t, cfg, w, h)
				m.startup = startupChartOpen
				m.picker = chartPickerState{prior: startupReady}
				m, _ = applyChartsLoaded(m, ChartsLoadedMsg{Gen: m.pickerGen, Charts: charts, Drops: drops})
				return m
			},
			render: func(m Model, cfg Config, w, h int) string { return renderChartModal(m.picker, cfg, w, h) },
			steps: listSteps("fem", fitStep{"a chart file appears", func(t *testing.T, m Model) Model {
				more := append(append([]store.NamedChart(nil), charts...), fitCharts(12)...)
				more = append(more, testNamedChart("A newly saved chart with a name longer than any before it", berlinP))
				m, _ = applyChartsLoaded(m, ChartsLoadedMsg{Gen: m.pickerGen, Charts: more, Drops: drops})
				return m
			}}),
		}
	}
	bodies := func(state string) fitCase {
		// The tag bar: its count at its widest, a space and room for a few names.
		n := len(fitBodyState("").registry)
		tagBar := ansi.StringWidth(fmt.Sprintf("%d of %d selected · ● all ◐ some ○ none", n, n)) + 1 + 12
		return fitCase{popup: "renderBodyModal", state: state, title: "Bodies", typing: max(len(bodiesPrompt)+24+1, tagBar, modalProseWidth), inputs: 1, fill: " selected ", blanks: 2, endsBlank: true, zone: true,
			model: func(t *testing.T, cfg Config, w, h int) Model {
				m := fitBase(t, cfg, w, h)
				m.startup = startupBodyOpen
				m.bodyModal = fitBodyState("")
				return m
			},
			render: func(m Model, cfg Config, w, h int) string { return renderBodyModal(m.bodyModal, cfg, w, h) },
			steps: listSteps("mo", fitStep{"tick", fitKeys(" ")}, fitStep{"a note of several rows", func(t *testing.T, m Model) Model {
				m.bodyModal.cursor = groupRows()
				if o, ok := m.bodyModal.objectUnderCursor(); ok {
					m.bodyModal.setProbe(o.Number, m.bodyModal.inputs, lookup.BodyProbe{MissingFiles: []string{"seas_18.se1", "sepl_18.se1", "semo_18.se1", "s136108s.se1", "s136199s.se1", "s136472s.se1"}})
				}
				return m
			}}),
		}
	}
	aspects := func(state string) fitCase {
		return fitCase{popup: "renderAspectModal", state: state, title: "Aspects", typing: search, inputs: 1,
			model: func(t *testing.T, cfg Config, w, h int) Model {
				m := fitBase(t, cfg, w, h)
				m.startup = startupAspectOpen
				m.aspectModal = listRowsAspectState("")
				return m
			},
			render: func(m Model, cfg Config, w, h int) string { return renderAspectModal(m.aspectModal, cfg, w, h) },
			steps:  listSteps("sq", fitStep{"tick", fitKeys(" ")}),
		}
	}
	houses := func(state string) fitCase {
		return fitCase{popup: "renderHouseFinder", state: state, title: "Houses", typing: search, inputs: 1,
			model: func(t *testing.T, cfg Config, w, h int) Model {
				next, _ := openHouseFinder(fitBase(t, cfg, w, h))
				return next.(Model)
			},
			render: func(m Model, cfg Config, w, h int) string { return renderHouseFinder(m.houseFinder, cfg, w, h) },
			steps:  listSteps("por"),
		}
	}
	zodiacs := func(state string) fitCase {
		return fitCase{popup: "renderZodiacFinder", state: state, title: "Zodiac", typing: search, inputs: 1,
			model: func(t *testing.T, cfg Config, w, h int) Model {
				next, _ := openZodiacFinder(fitBase(t, cfg, w, h))
				return next.(Model)
			},
			render: func(m Model, cfg Config, w, h int) string { return renderZodiacFinder(m.zodiacFinder, cfg, w, h) },
			steps:  listSteps("fa"),
		}
	}
	display := func(form int) fitCase {
		c := fitCase{popup: "renderHouseDisplay", state: fmt.Sprintf("form %d", form), group: "display", title: "House display", blanks: -1,
			model: func(t *testing.T, cfg Config, w, h int) Model {
				next, _ := openHouseDisplay(fitBase(t, cfg, w, h))
				m := next.(Model)
				m.houseDisplayPopup.list.cursor = form
				return m
			},
			render: func(m Model, cfg Config, w, h int) string { return renderHouseDisplay(m.houseDisplayPopup, cfg, w, h) },
			steps: []fitStep{{"cursor down", fitKeys("down")}, {"cursor down", fitKeys("down")}, {"cursor down", fitKeys("down")},
				{"cursor up", fitKeys("up")}, {"cursor to the top", fitKeys("up", "up", "up")}, {"choose", fitKeys(" ")}},
		}
		if form == 0 {
			c.blanks = 1 // the tallest form: the line before its example
		}
		return c
	}
	sessions := func(state string, rows []store.NamedSession) fitCase {
		return fitCase{popup: "renderSessions", state: state, title: "Sessions",
			model: func(t *testing.T, cfg Config, w, h int) Model {
				m := fitBase(t, cfg, w, h)
				m.startup = startupSessionsOpen
				m.sessions = sessionsPopupState{rows: rows, prior: startupReady}
				m.sessions.fit = sessionsFit(m)
				return m
			},
			render: func(m Model, cfg Config, w, h int) string { return renderSessions(m, w, h) },
			steps: []fitStep{{"cursor down", fitKeys("down", "down")}, {"cursor to the bottom", fitKeys("end")}, {"cursor to the top", fitKeys("home", "up")},
				{"a session is saved", func(t *testing.T, m Model) Model {
					m.sessions.rows = append(append([]store.NamedSession(nil), m.sessions.rows...), store.NamedSession{Name: "Saved just now, under a rather long name", File: "saved.yaml"})
					for i := 0; i < 20; i++ {
						m.sessions.rows = append(m.sessions.rows, store.NamedSession{Name: fmt.Sprintf("More %02d", i), File: fmt.Sprintf("more-%02d.yaml", i)})
					}
					return m
				}}},
		}
	}
	// A form: type, take it back, a message arrives, one that needs more rows
	// than its row, one that needs more than the form can give, it goes.
	formSteps := func(refusal string, after func(m Model, refusal string) Model) []fitStep {
		return []fitStep{
			{"type", fitTyped("Jane Doe")},
			{"backspace", fitKeys(slicesRepeat("backspace", 3)...)},
			{"a refusal", func(t *testing.T, m Model) Model { return after(m, refusal) }},
			{"a message of two rows", func(t *testing.T, m Model) Model {
				return after(m, refusal+" "+refusal)
			}},
			{"a message of many rows", func(t *testing.T, m Model) Model {
				return after(m, strings.Repeat(refusal+" ", 9))
			}},
			{"the message goes", func(t *testing.T, m Model) Model { return after(m, "") }},
		}
	}

	return []fitCase{
		atlas("hint", ""), atlas("results", "ha"),
		chart("few", fitCharts(5), []store.ChartDrop{{File: "broken.as", Reason: "no date"}}),
		chart("empty", nil, nil),
		chart("many", fitCharts(60), nil),
		bodies("all"), aspects("all"),
		{popup: "renderHelpModal", state: "help", title: "Help", blanks: -1,
			model: func(t *testing.T, cfg Config, w, h int) Model {
				m, _ := press(t, fitBase(t, cfg, w, h), "?")
				return m
			},
			render: func(m Model, cfg Config, w, h int) string { return renderHelpModal(m.helpLines(), cfg, m.help, w, h) },
			steps:  []fitStep{{"scroll down", fitKeys("down", "down", "pgdown")}, {"to the bottom", fitKeys("end")}, {"to the top", fitKeys("home")}},
		},
		houses("all"), zodiacs("all"),
		display(0), display(1), display(2), display(3),
		sessions("rows", fitSessions()), sessions("none", nil),
		{popup: "renderSaveModal", state: "empty", title: "Save chart", typing: form, inputs: 1, blanks: 2, endsBlank: true, zone: true,
			model: func(t *testing.T, cfg Config, w, h int) Model {
				m := fitBase(t, cfg, w, h)
				m.startup = startupSaveOpen
				m.save = saveModalState{form: newSaveForm(), dest: "/data/op/Clients", prior: startupReady}
				return m
			},
			render: func(m Model, cfg Config, w, h int) string { return renderSaveModal(m.save, cfg, w, h) },
			steps:  formSteps(`"jane-doe" is taken — try another name.`, func(m Model, r string) Model { m.save.refusal = r; return m }),
		},
		{popup: "renderTitleForm", state: "ask", group: "title", title: "Chart title", typing: max(form, len("Place: ")+40+1), inputs: 2, blanks: -1, zone: true,
			model: func(t *testing.T, cfg Config, w, h int) Model {
				m := fitBase(t, cfg, w, h)
				c := fitEquinox()
				m.startup = startupTitleOpen
				m.title = titleFormState{form: newTitleForm(c), chart: c}
				return m
			},
			render: func(m Model, cfg Config, w, h int) string { return renderTitleForm(m.title, cfg, w, h) },
			steps: append(formSteps("Add a place to save this title.", func(m Model, r string) Model { m.title.refusal = r; return m }),
				fitStep{"the second field", fitKeys("tab")},
				fitStep{"the second step", func(t *testing.T, m Model) Model {
					m.title.confirming, m.title.dir = true, store.ChartsDir()
					return m
				}},
				fitStep{"the second step, its backup already kept", func(t *testing.T, m Model) Model {
					m.title.backupHeld = true
					return m
				}}),
		},
		{popup: "renderTitleForm", state: "confirm", group: "title", title: "Chart title", typing: max(form, len("Place: ")+40+1), inputs: 2, blanks: -1, zone: true,
			model: func(t *testing.T, cfg Config, w, h int) Model {
				m := fitBase(t, cfg, w, h)
				c := fitEquinox()
				m.startup = startupTitleOpen
				m.title = titleFormState{form: newTitleForm(c), chart: c, confirming: true, dir: store.ChartsDir()}
				m.title.form.fields[0].value = "Constructed One"
				return m
			},
			render: func(m Model, cfg Config, w, h int) string { return renderTitleForm(m.title, cfg, w, h) },
		},
		{popup: "renderSessionName", state: "save as", title: "Save session", typing: form, inputs: 1, blanks: 2, endsBlank: true, zone: true,
			model: func(t *testing.T, cfg Config, w, h int) Model {
				m := fitBase(t, cfg, w, h)
				m.startup = startupSessionNameOpen
				m.sessions = sessionsPopupState{form: newForm(formField{label: "Save as: "}), prior: startupReady}
				return m
			},
			render: func(m Model, cfg Config, w, h int) string { return renderSessionName(m, w, h) },
			steps:  formSteps("That name is taken. Try another.", func(m Model, r string) Model { m.sessions.refusal = r; return m }),
		},
		{popup: "renderSessionName", state: "rename", title: "Rename session", typing: form, inputs: 1, blanks: 2, endsBlank: true, zone: true,
			model: func(t *testing.T, cfg Config, w, h int) Model {
				m := fitBase(t, cfg, w, h)
				m.startup = startupSessionNameOpen
				m.sessions = sessionsPopupState{form: newForm(formField{label: "Rename to: "}), prior: startupReady, renaming: "monday.yaml"}
				return m
			},
			render: func(m Model, cfg Config, w, h int) string { return renderSessionName(m, w, h) },
			steps:  formSteps("That name is taken. Try another.", func(m Model, r string) Model { m.sessions.refusal = r; return m }),
		},
	}
}

// --- reading a box -------------------------------------------------------

var (
	fitCounter = regexp.MustCompile(`(\d+) of (\d+)`)
	fitRuleRow = regexp.MustCompile(`^[─┈┄\-]+$`)
)

// fitBoxRows are the box's inner rows as plain text, the border and the
// padding taken off: rows[0] and rows[len-1] are the padding rows. Fails on a
// box that is not a rectangle.
func fitBoxRows(t *testing.T, box string) (rows []string, width int) {
	t.Helper()
	lines := strings.Split(ansi.Strip(box), "\n")
	if len(lines) < 4 {
		t.Fatalf("a popup of %d lines:\n%s", len(lines), box)
	}
	width = ansi.StringWidth(lines[0])
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			t.Fatalf("line %d is %d cells, the top edge is %d:\n%s", i, w, width, ansi.Strip(box))
		}
	}
	for _, l := range lines[1 : len(lines)-1] {
		rows = append(rows, strings.TrimRight(ansi.Cut(l, 3, width-3), " "))
	}
	return rows, width - 6
}

// fitContent is what the box says below its input lines, as one string: the
// rows with their spacing collapsed, the rules left out (they span whatever
// they are given), and blank rows dropped. If narrowing or
// shortening the box changes it, the box needed what it had.
func fitContent(rows []string, c fitCase) string {
	start := 1 + c.inputs
	if c.inputs > 0 {
		start++ // the rule under the inputs
	}
	var parts []string
	for i := start; i < len(rows)-1; i++ {
		if t := strings.TrimSpace(rows[i]); t == "" || fitRuleRow.MatchString(t) {
			continue // a blank row, or a rule: it spans whatever width it is given
		}
		if c.fill != "" && strings.Contains(rows[i], c.fill) {
			continue
		}
		parts = append(parts, strings.Join(strings.Fields(rows[i]), " "))
	}
	return strings.Join(parts, "\n")
}

func fitProbe(narrow int, uncapped bool) func() {
	prev := modalFitProbe
	modalFitProbe.narrowBy, modalFitProbe.uncapped = narrow, uncapped
	return func() { modalFitProbe = prev }
}

// fitResult is what one state of a group showed.
type fitResult struct {
	width int  // the box's content width
	rows  int  // and its height
	floor int  // the width the frame and the input lines ask for
	lost  bool // narrowing the box by one cell cut or pushed out something
}

// TestEveryPopupFitsItsContent is the rule at the moment a popup opens: it is
// as wide as the widest line it could show during this opening and as tall as
// all of its entries (and the line kept for a refusal), at 132x40 and 80x24,
// colour and colourless, in each state its table row lists.
func TestEveryPopupFitsItsContent(t *testing.T) {
	forceColor(t)
	groups := map[string][]fitResult{}
	for _, c := range fitCases() {
		for _, size := range fitSizes {
			for _, mode := range fitModes {
				name := fmt.Sprintf("%s/%s/%dx%d/%s", c.popup, c.state, size.w, size.h, mode.name)
				key := fmt.Sprintf("%s/%dx%d/%s", c.popup+"/"+c.state, size.w, size.h, mode.name)
				if c.group != "" {
					key = fmt.Sprintf("%s/%s/%dx%d/%s", c.popup, c.group, size.w, size.h, mode.name)
				}
				t.Run(name, func(t *testing.T) {
					box := c.open(t, mode.cfg, size.w, size.h)
					rows, cw := fitBoxRows(t, box)

					// The box sits inside the terminal.
					if cw+6 > size.w || len(rows)+2 > size.h {
						t.Fatalf("the box is %dx%d in a %dx%d terminal", cw+6, len(rows)+2, size.w, size.h)
					}

					// The floors that are not content: the typing room, the
					// title in the top edge and the widest counter in the
					// bottom one.
					floor := c.typing
					mark := 0
					if !mode.cfg.useColor() {
						mark = 1
					}
					floor = max(floor, ansi.StringWidth(c.title)+mark+4-6)
					edge := strings.Split(ansi.Strip(box), "\n")
					if m := fitCounter.FindStringSubmatch(edge[len(edge)-1]); m != nil {
						floor = max(floor, len(m[2])*2+len(" of ")+4-6)
					}
					if cw < floor {
						t.Errorf("the box is %d cells wide inside, below its floor %d", cw, floor)
					}

					// Width: with every row on show (the terminal's height
					// lifted), take one cell off. Something must be cut, or
					// no state of the group needs the cell.
					restore := fitProbe(0, true)
					all := c.open(t, mode.cfg, size.w, size.h)
					restore()
					allRows, _ := fitBoxRows(t, all)
					restore = fitProbe(1, true)
					narrow := c.open(t, mode.cfg, size.w, size.h)
					restore()
					nrows, ncw := fitBoxRows(t, narrow)
					if ncw != cw-1 {
						t.Fatalf("the probe did not narrow the box: %d then %d", cw, ncw)
					}
					groups[key] = append(groups[key], fitResult{
						width: cw, rows: len(rows), floor: floor,
						lost: fitContent(nrows, c) != fitContent(allRows, c) || len(nrows) != len(allRows),
					})

					// Height. The popup with every row on show (the terminal's
					// height lifted) is its content: it has no blank row but
					// the ones the content asks for (the line kept for a
					// refusal, the line before the House display's example). The popup
					// in the terminal is that, or as much of it as the
					// terminal gives.
					chrome := 2 + c.inputs // the padding rows and the input lines
					capRows := modalListHeight(size.h)
					if c.inputs > 0 {
						chrome++ // the rule under the inputs
					} else {
						capRows += 2 // nothing to type in: the rows are the list's
					}
					if c.inputs > 1 {
						capRows -= c.inputs - 1
					}
					if c.zone {
						capRows += modalZoneRows
					}
					if c.blanks >= 0 {
						blank := 0
						for _, r := range allRows[1 : len(allRows)-1] {
							if strings.TrimSpace(r) == "" {
								blank++
							}
						}
						if blank != c.blanks {
							t.Errorf("%d blank rows in the content, %d wanted: a row nothing needs, or content missing:\n%s", blank, c.blanks, ansi.Strip(all))
						}
					}
					if c.blanks >= 0 && !c.endsBlank && strings.TrimSpace(allRows[len(allRows)-2]) == "" {
						t.Errorf("the content ends in a blank row:\n%s", ansi.Strip(all))
					}
					listRows, needRows := len(rows)-chrome, len(allRows)-chrome
					if want := min(needRows, capRows); listRows != want {
						t.Errorf("the list is %d rows, want %d (its content is %d, the terminal gives %d):\n%s", listRows, want, needRows, capRows, ansi.Strip(box))
					}
					if listRows < capRows {
						for _, l := range strings.Split(ansi.Strip(box), "\n")[1 : len(rows)+1] {
							if !strings.HasSuffix(l, "│") && !strings.HasSuffix(l, "|") {
								t.Errorf("a scroll thumb on a box taller than its rows:\n%s", ansi.Strip(box))
								break
							}
						}
					}
				})
			}
		}
	}

	// A group is one width, and that width is needed by one of its states or
	// is the floor.
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rs := groups[k]
		lost, floor := false, 0
		for _, r := range rs {
			if r.width != rs[0].width || r.rows != rs[0].rows {
				t.Errorf("%s: the states are not one size (%dx%d and %dx%d): the box moves under the operator", k, rs[0].width, rs[0].rows, r.width, r.rows)
			}
			lost = lost || r.lost
			floor = max(floor, r.floor)
		}
		if !lost && rs[0].width != floor {
			t.Errorf("%s: the box is %d cells wide inside but no state of it is cut at %d: a spare cell (floor %d)", k, rs[0].width, rs[0].width-1, floor)
		}
	}
}

// fitStartups maps each popup to the startup state that shows it ("" for help,
// which has its own flag). startupShowsModal lists the states a modal can be
// in; the test below holds that list to this one.
var fitStartups = map[string]string{
	"renderAtlasModal":   "startupAtlasOpen",
	"renderChartModal":   "startupChartOpen",
	"renderBodyModal":    "startupBodyOpen",
	"renderAspectModal":  "startupAspectOpen",
	"renderHelpModal":    "",
	"renderHouseFinder":  "startupHouseOpen",
	"renderZodiacFinder": "startupZodiacOpen",
	"renderHouseDisplay": "startupDisplayOpen",
	"renderSessions":     "startupSessionsOpen",
	"renderSaveModal":    "startupSaveOpen",
	"renderTitleForm":    "startupTitleOpen",
	"renderSessionName":  "startupSessionNameOpen",
}

// sourceFiles parses the package's own (non-test) files.
func sourceFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{}
	for _, p := range pkgs {
		for name, f := range p.Files {
			files[name] = f
		}
	}
	return files
}

// callersOf is the set of functions that call the function name.
func callersOf(files map[string]*ast.File, name string) map[string]bool {
	out := map[string]bool{}
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if id, ok := call.Fun.(*ast.Ident); ok && id.Name == name {
						out[fn.Name.Name] = true
					}
				}
				return true
			})
		}
	}
	return out
}

// TestEveryPopupIsInTheFitGuard holds the table above to the program, by
// reading the source: a popup added without a row in fitCases fails here, so
// the rule cannot be skipped by forgetting it. Three ways to add a popup are
// closed: calling the box (renderModalBox, or renderForm which calls it),
// being shown by a startup state, and drawing a frame of its own.
func TestEveryPopupIsInTheFitGuard(t *testing.T) {
	files := sourceFiles(t)

	table := map[string]bool{}
	for _, c := range fitCases() {
		table[c.popup] = true
	}

	// 1. Whoever calls the box, or the form that calls the box, is a row.
	drawn := callersOf(files, "renderModalBox")
	delete(drawn, "renderForm")
	for f := range callersOf(files, "renderForm") {
		drawn[f] = true
	}
	if got, want := sortedKeys(drawn), sortedKeys(table); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("the popups the program draws and the popups the fit guard opens differ:\n program: %v\n guard:   %v\nadd a row to fitCases for a new popup (or remove its row with it)", got, want)
	}

	// 2. Every startup state that shows a modal belongs to a row.
	var shown []string
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "startupShowsModal" {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if cc, ok := n.(*ast.CaseClause); ok {
					for _, e := range cc.List {
						if id, ok := e.(*ast.Ident); ok {
							shown = append(shown, id.Name)
						}
					}
				}
				return true
			})
		}
	}
	var mapped []string
	for popup, st := range fitStartups {
		if !table[popup] {
			t.Errorf("fitStartups names %s, which has no row in fitCases", popup)
		}
		if st != "" {
			mapped = append(mapped, st)
		}
	}
	sort.Strings(shown)
	sort.Strings(mapped)
	if len(shown) == 0 || fmt.Sprint(shown) != fmt.Sprint(mapped) {
		t.Errorf("startupShowsModal lists %v, the fit guard knows %v: a new modal state needs a popup row", shown, mapped)
	}

	// 3. Only the box and the panels draw a border: a popup that drew its own
	// frame would not be held to the rule by either of the above.
	allowed := map[string]bool{"panels.go": true, "modal_box.go": true, "layout.go": true}
	for name, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && (sel.Sel.Name == "RoundedBorder" || sel.Sel.Name == "NormalBorder") && !allowed[filepath.Base(name)] {
				t.Errorf("%s draws a border of its own (%s): a popup's frame is renderModalBox", name, sel.Sel.Name)
			}
			return true
		})
	}
}

// sortedKeys is a set's members in order.
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestEveryPopupKeepsItsSizeWhileOpen: a popup's size is decided once, when it
// opens, and never changes while it is open, in either direction (operator,
// 2026-10-27). Each popup in the table is opened and its box measured, then
// the operator types a filter, takes it back, walks the list to its ends,
// is refused, or sees an entry join, and the box is measured again after every
// step. Rows a filter removed are blank; a joining entry scrolls.
func TestEveryPopupKeepsItsSizeWhileOpen(t *testing.T) {
	forceColor(t)
	steps := 0
	for _, c := range fitCases() {
		if len(c.steps) == 0 {
			continue
		}
		for _, size := range fitSizes {
			for _, mode := range fitModes {
				t.Run(fmt.Sprintf("%s/%s/%dx%d/%s", c.popup, c.state, size.w, size.h, mode.name), func(t *testing.T) {
					m := c.model(t, mode.cfg, size.w, size.h)
					rows, width := fitBoxRows(t, c.render(m, mode.cfg, size.w, size.h))
					for _, s := range c.steps {
						steps++
						m = s.do(t, m)
						box := c.render(m, mode.cfg, size.w, size.h)
						r, w := fitBoxRows(t, box)
						if w != width || len(r) != len(rows) {
							t.Errorf("after %q the box is %dx%d, it opened %dx%d:\n%s", s.name, w+6, len(r)+2, width+6, len(rows)+2, ansi.Strip(box))
							return
						}
					}
				})
			}
		}
	}
	if steps == 0 {
		t.Fatal("no popup was driven")
	}
}

// TestCursorStaysVisibleWhenAMessageTakesRows: a message that needs more than
// its row takes rows from the foot of the list, and the list scrolls to keep
// the cursor on screen (operator, 2026-10-27). The box does not change size,
// and the list has its rows back when the message clears.
func TestCursorStaysVisibleWhenAMessageTakesRows(t *testing.T) {
	cfg := Config{Accessible: true, Color: ColorNever}
	for _, size := range fitSizes {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			m := fitBase(t, cfg, size.w, size.h)
			m.startup = startupBodyOpen
			m.objectRegistry = syntheticObjects(80)
			m.bodyModal = openBodyModal(m.objectRegistry, recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
			cursorRow := func(box string) string {
				for _, l := range strings.Split(box, "\n") {
					if strings.Contains(l, "|  > [") {
						return l
					}
				}
				return ""
			}
			at := func(m Model) (string, int) {
				box := ansi.Strip(renderBodyModal(m.bodyModal, cfg, size.w, size.h))
				return box, strings.Count(box, "\n")
			}
			// listed is how many bodies are on screen.
			listed := func(box string) int { return len(regexp.MustCompile(`\[.\] (! )?Object`).FindAllString(box, -1)) }
			for _, where := range []string{"end", "home"} {
				m, _ = press(t, m, where)
				m, _ = press(t, m, "down", "down", "down")
				plain, height := at(m)
				row := cursorRow(plain)
				if row == "" {
					t.Fatalf("%s: no cursor row to begin with:\n%s", where, plain)
				}
				obj, ok := m.bodyModal.objectUnderCursor()
				if !ok {
					// still in the group block: step into the list
					m.bodyModal.cursor = groupRows() + 3
					obj, _ = m.bodyModal.objectUnderCursor()
				}
				noted := m
				noted.bodyModal.setProbe(obj.Number, noted.bodyModal.inputs, lookup.BodyProbe{MissingFiles: []string{"seas_18.se1", "sepl_18.se1", "semo_18.se1", "s136108s.se1", "s136199s.se1", "s136472s.se1"}})
				box, h2 := at(noted)
				if h2 != height {
					t.Errorf("%s: a message of several rows changed the box from %d to %d lines", where, height+1, h2+1)
				}
				if !strings.Contains(cursorRow(box), obj.Name) {
					t.Errorf("%s: the cursor row (%s) is not on screen while the message takes rows:\n%s", where, obj.Name, box)
				}
				if size.h >= 40 && listed(box) >= listed(plain) {
					t.Errorf("%s: the message took no list rows (%d bodies on screen, %d without it):\n%s", where, listed(box), listed(plain), box)
				}
				// The message clears: the rows come back.
				cleared := noted
				cleared.bodyModal.probes = nil
				back, h3 := at(cleared)
				if h3 != height || listed(back) != listed(plain) {
					t.Errorf("%s: the list did not get its rows back when the message cleared:\n%s", where, back)
				}
			}
		})
	}
}

// On the smallest terminal (66x15) the confirm step has no row to spare for the
// message zone: the zone is empty rows until something is said, then the message
// takes them from the foot of the text, and the box is the same size.
func TestMessageOnTheSmallestTerminal(t *testing.T) {
	cfg := Config{Accessible: true, Color: ColorNever}
	long := store.NamedChart{ID: "x", Name: "x", Untitled: true, PlaceLabel: "Munich",
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "Constructed Client With A Long Name.as"}}
	st := titleFormState{form: newTitleForm(long), chart: long, confirming: true}
	quiet := strings.Split(renderTitleForm(st, cfg, MinPanelWidth, MinTerminalHeight), "\n")
	st.refusal = "Couldn't write the file."
	said := strings.Split(renderTitleForm(st, cfg, MinPanelWidth, MinTerminalHeight), "\n")
	if len(quiet) > MinTerminalHeight || len(said) != len(quiet) || ansi.StringWidth(said[0]) != ansi.StringWidth(quiet[0]) {
		t.Errorf("the box moved: %dx%d quiet, %dx%d with a message (terminal %d rows)", ansi.StringWidth(quiet[0]), len(quiet), ansi.StringWidth(said[0]), len(said), MinTerminalHeight)
	}
	if !strings.Contains(strings.Join(said, "\n"), "Couldn't write the file.") {
		t.Errorf("the message is not shown:\n%s", strings.Join(said, "\n"))
	}
}

// TestKnownRefusalsFitOneRow: every refusal a form makes from a fixed sentence
// fits the one row of the message zone in the narrowest form that shows it
// (operator, 2026-10-27), so the whole sentence is visible with no row taken
// from the text above it. Sentences with a name in them (a file, a slug) wrap
// into more rows when they must.
func TestKnownRefusalsFitOneRow(t *testing.T) {
	var labels []string
	for _, what := range []string{"name", "title", "place"} {
		labels = append(labels, (&astrolog.LabelTooLongError{What: what, Bytes: 100}).Error())
	}
	for _, s := range append(labels, backupRefusal, emptyPlaceRefusal, nameRefusal(store.ErrSessionNameTaken),
		nameRefusal(store.ErrSessionNameEmpty), nameRefusal(store.ErrSessionOpenElsewhere), "That name has no letters or digits to save under.") {
		if w := ansi.StringWidth(s); w > modalRefusalRoom {
			t.Errorf("%q is %d cells, the message zone's row holds %d", s, w, modalRefusalRoom)
		}
	}
}
