package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// rowText is one chart row of the time at the place, as the Chart panel
// draws it alone at width: the name, the table's cells and the tag.
func rowText(at time.Time, place location.PlaceInfo, name, tag string, width int, cfg Config) string {
	return renderTable([]tableRow{{name: name, cells: placeCells(at, place, nil), tag: tag}}, width, cfg)[0]
}

// clockSeconds turns "9:05" or "9:05am" in s into "9:05:00" or "9:05:00am":
// the clock as the table shows it, with its seconds.
var hhmm = regexp.MustCompile(`(\d{1,2}:\d{2})(am|pm)?\b`)

func clockSeconds(s string) string { return hhmm.ReplaceAllString(s, "${1}:00$2") }

// mixRows is every kind of row at once: a Julian date, each zone form, a long
// name, every tag mark, the floating row, and two charts of a comparison.
func mixRows() []tableRow {
	cell := func(wd, date string, jul bool, clock, zone, words, coords string, ok bool) rowCells {
		return rowCells{ok: true, weekday: wd, date: date, julian: jul, clock: clock, zone: zone, words: words, coords: coords, confirmed: ok}
	}
	return []tableRow{
		{prefix: "① ", name: "Johannes Kepler", cells: cell("Thu", "Dec 27, 1571", true, "14:37:00", "LMT", "Weil der Stadt, Germany", "8°52'E 48°45'N", true), tag: "Po*Tr"},
		{prefix: "② ", name: "Seconds", cells: cell("Fri", "Jun  1, 1990", false, "16:30:45", "+2 DT", "Amsterdam, Netherlands", "4°53'E 52°22'N", true), tag: "#23Tr"},
		{prefix: "  ", name: "India", cells: cell("Mon", "Jan 15, 1990", false, "10:00:00", "+5:30", "New Delhi, India", "77°13'E 28°36'N", true), tag: "3PlTr"},
		{prefix: "  ", name: "No place", cells: cell("Fri", "Jun  1, 1990", false, "16:30:00", "+2 DT", "Amsterdam", "3°12'W 55°57'N", false), tag: "Wh+2.5"},
		{prefix: "  ", name: "Open sea", cells: cell("Wed", "May  5, 2010", false, "12:00:00", "-3", "", "30°00'W 40°00'S", false), tag: "WhTr"},
		{prefix: "  ", name: "Alexandria Catherine of Aragon-Habsburg", cells: cell("Tue", "Jun  1, 1920", false, "19:12:50", "+1:19:32 DT", "Amsterdam, NL", "4°53'E 52°22'N", true), tag: "WhLa"},
		{prefix: "③ ", name: "~ Now", float: true, cells: cell("Sun", "Nov 15, 2026", false, "21:40:07", "+1", "Amsterdam, NL", "4°53'E 52°22'N", true), tag: "WhTr"},
	}
}

// cellAt is the display cell where sub starts in line, or -1.
func cellAt(line, sub string) int {
	i := strings.Index(line, sub)
	if i < 0 {
		return -1
	}
	return ansi.StringWidth(line[:i])
}

// Acceptance 2 and 3: at every width each column starts at the same cell in
// every row, and no row shows a clock without its zone. The sweep starts at
// the narrowest width that holds this mix whole, which is below every Chart
// panel the layouts draw (MinPanelWidth less the border).
func TestChartTableColumnsStartTogether(t *testing.T) {
	rows := mixRows()
	const narrowest = 48
	if narrowest > MinPanelWidth-2 {
		t.Fatalf("the sweep starts at %d, above the narrowest panel %d", narrowest, MinPanelWidth-2)
	}
	for width := narrowest; width <= 140; width++ {
		lines := renderTable(rows, width, Config{})
		start := map[string]int{}
		for i, line := range lines {
			c := rows[i].cells
			if w := ansi.StringWidth(line); w != width {
				t.Errorf("w%d row %d: width %d: %q", width, i, w, line)
			}
			clock := cellAt(line, c.clock+" "+c.zone)
			if clock < 0 {
				t.Errorf("w%d row %d: the clock %q and its zone %q are not whole together: %q", width, i, c.clock, c.zone, line)
				continue
			}
			cols := map[string]int{
				"date":  cellAt(line, c.date) + ansi.StringWidth(c.date),
				"clock": clock,
				"zone":  clock + ansi.StringWidth(c.clock) + 1,
			}
			if sh, _ := narrowTable(rows, width, Config{}); sh.tagCap != 0 {
				tag := rows[i].tag
				if sh.tagCap > 0 {
					tag = tag[:min(len(tag), sh.tagCap)]
				}
				cols["tag"] = ansi.StringWidth(line[:strings.LastIndex(line, tag)])
			}
			if place := placeText(c, 1<<20, Config{}); strings.Contains(line, place[:2]) {
				cols["place"] = cellAt(line, place[:2])
			}
			for col, at := range cols {
				if want, seen := start[col]; seen && at != want {
					t.Errorf("w%d: the %s column starts at %d in row %d, at %d above: %q", width, col, at, i, want, line)
				} else if !seen {
					start[col] = at
				}
			}
		}
	}
}

// Acceptance 3: the table narrows in one order as the panel narrows, and a
// step once taken stays taken: the place words are cut, the weekday goes, the
// names shorten, the place goes, the tag is cut.
func TestChartTableNarrowsInOrder(t *testing.T) {
	rows := mixRows()
	full := 0
	for _, r := range rows {
		full = max(full, placeWidth(r.cells))
	}
	steps := []string{"place cut", "weekday gone", "names short", "place gone", "tag cut"}
	first := map[string]int{}
	for width := 160; width >= 30; width-- {
		sh, placeW := narrowTable(rows, width, Config{})
		on := map[string]bool{
			"place cut":    !sh.place || placeW < full,
			"weekday gone": !sh.weekday,
			"names short":  sh.nameCap < nameColumnMax,
			"place gone":   !sh.place,
			"tag cut":      sh.tagCap >= 0,
		}
		for _, st := range steps {
			if on[st] {
				if _, seen := first[st]; !seen {
					first[st] = width
				}
			} else if _, seen := first[st]; seen {
				t.Errorf("w%d: %q was taken at %d and undone at %d", width, st, first[st], width)
			}
		}
	}
	for i := 1; i < len(steps); i++ {
		a, b := first[steps[i-1]], first[steps[i]]
		if b > a {
			t.Errorf("%q came at %d, before %q at %d", steps[i], b, steps[i-1], a)
		}
	}
	for _, st := range steps[:4] {
		if _, seen := first[st]; !seen {
			t.Errorf("the sweep never reached %q", st)
		}
	}
}

// The place rule: confirmed words stand alone, a place with no words is its
// coordinates, and unconfirmed words follow the coordinates in brackets. A cut
// takes letters from the words, never from the coordinates.
func TestChartTablePlaceRule(t *testing.T) {
	c := rowCells{words: "Amsterdam", coords: "3°12'W 55°57'N"}
	for _, tc := range []struct {
		cells rowCells
		w     int
		want  string
	}{
		{rowCells{words: "Edinburgh, GB", coords: "3°12'W 55°57'N", confirmed: true}, 40, "Edinburgh, GB"},
		{rowCells{words: "Edinburgh, GB", coords: "3°12'W 55°57'N", confirmed: true}, 8, "Edinbur…"},
		{rowCells{coords: "30°00'W 40°00'S"}, 40, "30°00'W 40°00'S"},
		{c, 40, "3°12'W 55°57'N (Amsterdam)"},
		{c, 21, "3°12'W 55°57'N (Ams…)"},
		{c, 20, "3°12'W 55°57'N"},
		{c, 15, "3°12'W 55°57'N"},
	} {
		if got := placeText(tc.cells, tc.w, Config{}); got != tc.want {
			t.Errorf("placeText(%+v, %d) = %q, want %q", tc.cells, tc.w, got, tc.want)
		}
	}
}

// Acceptance 6 for Now: the clock's real offset from its city's zone, the
// daylight hour included and marked DT as a saved chart marks it, seconds where the offset has them,
// LMT before standard time, no letters of a zone.
func TestNowZoneCellIsTheClocksOffset(t *testing.T) {
	zone := func(tz string, lon, lat float64, at time.Time) string {
		return placeCells(at, location.PlaceInfo{Name: "X", TZ: tz, Lon: lon, Lat: lat}, nil).zone
	}
	for _, c := range []struct {
		tz       string
		lon, lat float64
		at       time.Time
		want     string
	}{
		{"Europe/Berlin", 13.4, 52.5, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), "+1"},
		{"Europe/Berlin", 13.4, 52.5, time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), "+2 DT"},
		{"Asia/Kolkata", 77.2, 28.6, time.Date(1990, 1, 15, 4, 30, 0, 0, time.UTC), "+5:30"},
		{"Asia/Kathmandu", 85.3, 27.7, time.Date(2026, 1, 15, 4, 30, 0, 0, time.UTC), "+5:45"},
		{"America/New_York", -74, 40.7, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), "-5"},
		{"Europe/Amsterdam", 4.9, 52.37, time.Date(1920, 7, 1, 12, 0, 0, 0, time.UTC), "+1:19:32 DT"},
		{"Europe/Amsterdam", 4.9, 52.37, time.Date(1800, 7, 1, 12, 0, 0, 0, time.UTC), "LMT"},
		{"Europe/London", -0.1, 51.5, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), "+0"},
		// Ireland's data marks its winter as the shifted time; a saved chart
		// states its summer as standard +1, so Now reads the same.
		{"Europe/Dublin", -6.3, 53.3, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), "+0"},
		{"Europe/Dublin", -6.3, 53.3, time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), "+1"},
	} {
		if got := zone(c.tz, c.lon, c.lat, c.at); got != c.want {
			t.Errorf("%s at %s: zone cell %q, want %q", c.tz, c.at.Format(time.RFC3339), got, c.want)
		}
	}
}

// Acceptance 6 for a file chart: the cell from Astrolog's header words.
func TestFileZoneCellFromAstrologsWords(t *testing.T) {
	for _, c := range []struct{ dst, zone, want string }{
		{"ST", "1E", "+1"}, {"DT", "1E", "+2 DT"}, {"ST", "5:30E", "+5:30"}, {"ST", "3W", "-3"},
		{"DT", "5W", "-4 DT"}, {"ST", "0W", "+0"}, {"ST", "LMT", "LMT"}, {"AT", "1E", "+1 AT"}, {"ST", "0:19E", "+0:19"},
	} {
		if got := lookup.ZoneCell(c.dst, c.zone); got != c.want {
			t.Errorf("ZoneCell(%q, %q) = %q, want %q", c.dst, c.zone, got, c.want)
		}
	}
	if got := fmt.Sprint(lookup.OffsetCell(4772)); got != "+1:19:32" {
		t.Errorf("OffsetCell(4772) = %q", got)
	}
}

// Acceptance 7 on the screen: a chart whose place words no atlas city near
// its coordinates carries reads its coordinates first and the words in
// brackets; once the atlas confirms them, the words stand alone.
func TestChartPanelShowsUnconfirmedWordsBesideTheCoordinates(t *testing.T) {
	defaultClock(t)
	m := readyModelWithSlots(t, testPlaceAmsterdam())
	m.cfg = Config{Color: ColorNever}
	m.width, m.height = 132, 40
	if err := addAnchor(m.slots, time.Date(1990, 6, 1, 14, 30, 0, 0, time.UTC), testPlaceAmsterdam()); err != nil {
		t.Fatal(err)
	}
	s := firstAnchor(m.slots)
	s.Chart.Name = "No place"
	s.Chart.Clock = store.ChartClock{Year: 1990, Month: 6, Day: 1, Hour: 16, Minute: 30, Weekday: time.Friday, Zone: "+2 DT",
		Place: "Amsterdam", Coords: "3°12'W 55°57'N", Lon: -3.2, Lat: 55.95}
	m.atlas.Index = location.NewIndex([]location.PlaceInfo{{Name: "Edinburgh", Country: "GB", Lon: -3.1965, Lat: 55.9521}})
	if v := ansi.Strip(m.View()); !strings.Contains(v, "3°12'W 55°57'N (Amsterdam)") {
		t.Errorf("the unconfirmed place does not read coordinates first:\n%s", v)
	}
	s.Chart.Clock.Place = "Edinburgh"
	if v := ansi.Strip(m.View()); !strings.Contains(v, " Edinburgh ") || strings.Contains(v, "3°12'W 55°57'N") {
		t.Errorf("the confirmed place does not stand alone:\n%s", v)
	}
}

// Coordinates a confirmed place never shows do not hold the place column
// open: the try's charts beside Now, whose atlas coordinates run to the
// second, keep nine cells of name at the 132x40 panel, as in a drive.
func TestConfirmedPlacesDoNotWidenThePlaceFloor(t *testing.T) {
	cell := func(wd, date string, jul bool, clock, zone, words, coords string, ok bool) rowCells {
		return rowCells{ok: true, weekday: wd, date: date, julian: jul, clock: clock, zone: zone, words: words, coords: coords, confirmed: ok}
	}
	rows := []tableRow{
		{name: "Johannes Kepler", cells: cell("Thu", "Dec 27, 1571", true, "14:37:00", "LMT", "Weil der Stadt, Germany", "8°52'E 48°45'N", true), tag: "WhTr"},
		{name: "Seconds", cells: cell("Fri", "Jun  1, 1990", false, "16:30:45", "+2 DT", "Amsterdam, Netherlands", "4°53'E 52°22'N", true), tag: "WhTr"},
		{name: "India", cells: cell("Mon", "Jan 15, 1990", false, "10:00:00", "+5:30", "New Delhi, India", "77°13'E 28°36'N", true), tag: "WhTr"},
		{name: "Open sea", cells: cell("Wed", "May  5, 2010", false, "12:00:00", "-3", "South Atlantic", "30°00'W 40°00'S", false), tag: "WhTr"},
		{name: "~ Now", float: true, cells: cell("Sun", "Nov 15, 2026", false, "21:53:00", "+1", "Amsterdam, NL", "4°53'23\"E 52°22'26\"N", true), tag: "WhTr"},
	}
	if sh, _ := narrowTable(rows, 63, Config{}); sh.nameCap < 9 {
		t.Errorf("the names were cut to %d cells for coordinates no row shows: %+v", sh.nameCap, sh)
	}
}
