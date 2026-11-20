package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// filterLine is one of the three Details filter lines, reached the same way.
type filterLine struct {
	name     string
	open     func(t *testing.T) Model // Details attached with rows to move over, line closed
	query    func(m Model) string
	setQuery func(m Model, q string) Model
	searches func(m Model) bool
	bindings func() []keys.BindingEntry
	// state is what no key of the open line may change.
	state func(m Model) [4]int
}

func filterLines() []filterLine {
	list := func(k timelineList) filterLine {
		return filterLine{
			name:     k.scope(),
			open:     func(t *testing.T) Model { return pagedList(t, k, 100) },
			query:    func(m Model) string { return m.list(k).query },
			setQuery: func(m Model, q string) Model { m.list(k).query = q; return m },
			searches: func(m Model) bool { return m.list(k).searching },
			bindings: timelineSearchBindings,
			state: func(m Model) [4]int {
				return [4]int{m.details.cursor, m.details.rowOffset, m.details.scrollOffset, len(m.timelineShown())}
			},
		}
	}
	report := filterLine{
		name: "report",
		open: func(t *testing.T) Model {
			m := attachedReport(t, false, "[0] Aspects", ownFilterBody(80))
			return m
		},
		query:    func(m Model) string { return m.details.query },
		setQuery: func(m Model, q string) Model { m.details.query = q; return m },
		searches: func(m Model) bool { return m.details.searching },
		bindings: reportSearchBindings,
		state: func(m Model) [4]int {
			return [4]int{m.details.cursor, m.details.rowOffset, m.details.scrollOffset, len(m.detailsView().Lines)}
		},
	}
	return []filterLine{list(listTimeline), list(listMoon), report}
}

var filterMovementKeys = []tea.KeyMsg{
	{Type: tea.KeyUp}, {Type: tea.KeyDown}, {Type: tea.KeyPgUp}, {Type: tea.KeyPgDown},
	{Type: tea.KeyCtrlPgUp}, {Type: tea.KeyCtrlPgDown},
}

// While a filter line is open it only types: no arrow, no paging key and no
// pane scroll does anything, and the list draws no cursor row.
func TestOpenFilterLineTakesOnlyTyping(t *testing.T) {
	forceColor(t)
	for _, l := range filterLines() {
		t.Run(l.name, func(t *testing.T) {
			m := l.open(t)
			m, _ = tpress(t, m, runes("/"))
			if !l.searches(m) {
				t.Fatal("fixture: `/` did not open the line")
			}
			if l.name != "report" {
				m2 := m
				m2.cfg = Config{Color: ColorAlways}
				for _, y := range parseDesignScreen(m2.View()).rowsWhere(func(c designCell) bool { return c.bg == styleBG(t, term.DefaultCursorRowColour) }) {
					if y >= m2.geometry()[PanelDetails].y && y < m2.geometry()[PanelDetails].y+m2.geometry()[PanelDetails].h {
						t.Errorf("a cursor row is drawn in the Details list while the line is open (row %d)", y)
					}
				}
			}
			before := l.state(m)
			for _, k := range filterMovementKeys {
				m, _ = tpress(t, m, k)
				if got := l.state(m); got != before || l.query(m) != "" || !l.searches(m) {
					t.Fatalf("%v in the open line: state %v -> %v, query %q, open %v", k, before, got, l.query(m), l.searches(m))
				}
			}
		})
	}
}

// A closed line that keeps a query is faint with colours on; the open line is
// plain; with colours off nothing changes.
func TestClosedFilterLineIsFaint(t *testing.T) {
	forceColor(t)
	for _, l := range filterLines() {
		t.Run(l.name, func(t *testing.T) {
			lineOf := func(m Model, prefix string) string {
				for _, ln := range strings.Split(m.View(), "\n") {
					if strings.Contains(ansi.Strip(ln), prefix) {
						return ln
					}
				}
				t.Fatalf("no row with %q on screen:\n%s", prefix, ansi.Strip(m.View()))
				return ""
			}
			m := l.open(t)
			m.cfg = Config{Color: ColorAlways}
			m, _ = tpress(t, m, runes("/"))
			m = typeQuery(t, m, "squ")
			if open := lineOf(m, "/squ█"); strings.Contains(open, term.SGRFaint) {
				t.Errorf("the open line is faint: %q", open)
			}
			m, _ = tpress(t, m, keyEnter)
			if closed := lineOf(m, "/squ  "); !strings.Contains(closed, term.SGRFaint) {
				t.Errorf("the closed line keeping a query is not faint: %q", closed)
			}
			plain := m
			plain.cfg = Config{Accessible: true, Color: ColorNever}
			if closed := lineOf(plain, "/squ  "); strings.Contains(closed, "\x1b") {
				t.Errorf("colours off, the closed line carries an escape: %q", closed)
			}
		})
	}
}

// When the line closes the cursor comes back: on its row when it is still
// shown, else on the next event after the moment, else on the last row.
func TestCursorReturnsWhenTheFilterClosesOnTheTimeline(t *testing.T) {
	for _, k := range allLists {
		t.Run(k.scope(), func(t *testing.T) {
			m := pagedList(t, k, 100)
			rows := m.timelineShown()
			// Rows of the sky search all match "Sun"? Use a query every row ending
			// in an even number keeps: the Body column carries the row's own text.
			target := 40
			m.details.cursor = target
			m = m.recordTimelineCursor()
			at := markOf(rows[target])
			q := rows[target].Body
			m, _ = tpress(t, m, runes("/"))
			m = typeQuery(t, m, q)
			m, _ = tpress(t, m, keyEnter)
			shown := m.timelineShown()
			if len(shown) == 0 || markOf(shown[m.details.cursor]) != at {
				t.Fatalf("the cursor is on %v, want its row %v back", markOf(shown[m.details.cursor]), at)
			}

			// A query that drops the row: the next event after the moment.
			m = pagedList(t, k, 100)
			rows = m.timelineShown()
			m.details.cursor = 0
			m = m.recordTimelineCursor()
			m, _ = tpress(t, m, runes("/"))
			m = typeQuery(t, m, rows[90].Body)
			shown = m.timelineShown()
			if len(shown) < 2 || markOf(shown[0]) == markOf(rows[0]) {
				t.Skip("fixture: the query keeps the first row or too few rows")
			}
			m, _ = tpress(t, m, keyEnter)
			want := len(shown) - 1
			for i, r := range shown {
				if !r.Instant.Before(m.floatingTime()) {
					want = i
					break
				}
			}
			if m.details.cursor != want {
				t.Errorf("the cursor is on %d, want the next event after the moment, %d", m.details.cursor, want)
			}

			// Nothing after the moment: the last row.
			m = pagedList(t, k, 100)
			rows = m.timelineShown()
			if m.nearestRow(rows) < 1 {
				t.Skip("fixture: no row before the moment")
			}
			m, _ = tpress(t, m, runes("/"))
			m = typeQuery(t, m, strings.TrimSpace(panel.DisplayRow(rows[0], m.cfg)))
			shown = m.timelineShown()
			m, _ = tpress(t, m, keyEnter)
			if m.nearestRow(shown) >= 0 || m.details.cursor != len(shown)-1 {
				t.Errorf("the cursor is on %d of %d, want the last row", m.details.cursor, len(shown))
			}
		})
	}
	t.Run("report", func(t *testing.T) {
		m := attachedReport(t, false, "[0] Aspects", ownFilterBody(80))
		m.details.scrollOffset = 20
		m, _ = tpress(t, m, runes("/"))
		m = typeQuery(t, m, "squ")
		m, _ = tpress(t, m, keyEnter)
		if m.details.scrollOffset != 0 {
			t.Errorf("the report stands at line %d, want the first row shown", m.details.scrollOffset)
		}
	})
}

// Esc in the line brings back the query it opened with; Esc on the list never
// clears a filter.
func TestEscInTheFilterLineAndOnTheList(t *testing.T) {
	for _, l := range filterLines() {
		t.Run(l.name, func(t *testing.T) {
			m := l.open(t)
			m, _ = tpress(t, m, runes("/"))
			m = typeQuery(t, m, "ura")
			m, _ = tpress(t, m, keyEnter)
			m, _ = tpress(t, m, runes("/"))
			m = typeQuery(t, m, "nus")
			m, _ = tpress(t, m, keyEsc)
			if l.query(m) != "ura" || l.searches(m) {
				t.Fatalf("Esc in the line: query %q open %v, want ura, closed", l.query(m), l.searches(m))
			}
			m, _ = tpress(t, m, keyEsc)
			if l.query(m) != "ura" {
				t.Errorf("Esc on the list: query %q, want the filter kept", l.query(m))
			}
		})
	}
}

// A move of the floating chart in time or place clears every Details filter.
func TestAMoveClearsEveryDetailsFilter(t *testing.T) {
	setup := func(t *testing.T) Model {
		m := pagedList(t, listTimeline, 100)
		m.list(listTimeline).query = "a"
		m.list(listMoon).query = "b"
		m.details.query = "c"
		return m
	}
	empty := func(t *testing.T, m Model, what string) {
		t.Helper()
		if m.list(listTimeline).query != "" || m.list(listMoon).query != "" || m.details.query != "" {
			t.Errorf("%s left filters: timeline %q moon %q report %q", what, m.list(listTimeline).query, m.list(listMoon).query, m.details.query)
		}
	}
	m, _ := setup(t).advanceTime(unitDay, +1)
	empty(t, m, "a time step")
	p := setup(t).refreshSlotsForPlace(location.PlaceInfo{Name: "Rome", Country: "IT", Lat: 41.9, Lon: 12.5, TZ: "Europe/Rome"})
	empty(t, p, "a place change")
}

// A filter that keeps nothing leaves one plain line and the count 0 of N.
func TestNoRowMatchesLine(t *testing.T) {
	for _, l := range filterLines() {
		t.Run(l.name, func(t *testing.T) {
			m := l.open(t)
			m, _ = tpress(t, m, runes("/"))
			m = typeQuery(t, m, "zzzq")
			m, _ = tpress(t, m, keyEnter)
			rows := strings.Join(detailsBodyRows(renderDetails(m)), "\n")
			if !strings.Contains(rows, "no row matches") || !strings.Contains(rows, "0 of ") {
				t.Errorf("the filtered list reads:\n%s", rows)
			}
		})
	}
}

// With a query the report hides every line below its list; with none it shows
// them.
func TestReportHidesItsSummaryWhileFiltered(t *testing.T) {
	m := attachedReport(t, false, "[0] Aspects", ownFilterBody(8))
	if body := viewText(m); !strings.Contains(body, "Sum power") {
		t.Fatal("fixture: the unfiltered report has no summary")
	}
	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "squ")
	if body := viewText(m); strings.Contains(body, "Sum power") || strings.Contains(body, "Con:  3") {
		t.Errorf("the filtered report shows lines below the list:\n%s", body)
	}
	m, _ = tpress(t, m, keyEnter)
	if body := viewText(m); strings.Contains(body, "Sum power") {
		t.Error("the closed filtered report shows its summary")
	}
	m, _ = tpress(t, m, runes("/"))
	for range "squ" {
		m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	if body := viewText(m); !strings.Contains(body, "Sum power") {
		t.Error("with the query empty the summary is back")
	}
}

// The three lines behave alike: Enter and Backspace on empty close, and their
// bars offer no movement or paging key.
func TestThreeFilterLinesBehaveAlike(t *testing.T) {
	for _, l := range filterLines() {
		t.Run(l.name, func(t *testing.T) {
			for _, e := range l.bindings() {
				switch e.BarKey {
				case "up", "down", "pgup", "pgdown", "ctrl+pgup", "ctrl+pgdown":
					t.Errorf("the bar offers %q", e.BarKey)
				}
			}
			m := l.open(t)
			m, _ = tpress(t, m, runes("/"))
			m = typeQuery(t, m, "squ")
			m, _ = tpress(t, m, keyEnter)
			if l.searches(m) || l.query(m) != "squ" {
				t.Errorf("Enter: open %v query %q", l.searches(m), l.query(m))
			}
			m, _ = tpress(t, m, runes("/"))
			before := l.state(m)
			for _, k := range filterMovementKeys {
				m, _ = tpress(t, m, k)
			}
			if l.state(m) != before || l.query(m) != "squ" {
				t.Errorf("movement keys changed %v -> %v, query %q", before, l.state(m), l.query(m))
			}
			for range "squ" {
				m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
			}
			if !l.searches(m) || l.query(m) != "" {
				t.Fatalf("after deleting the query: open %v query %q", l.searches(m), l.query(m))
			}
			m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
			if l.searches(m) {
				t.Error("Backspace on the empty line left it open")
			}
		})
	}
}
