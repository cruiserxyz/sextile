package ui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// All chart values in this file are constructed synthetic test inputs
// (specs/frames/residue.md).

// Verifiers for the specification item B: the anchored chart's name opens
// its Chart row, and the rows read as a table.

var (
	nameColUTC = location.PlaceInfo{Name: "Test Bay", Country: "TS", Lat: 12.34, Lon: 56.78, TZ: "Etc/UTC"}
	// A long place in a four-letter summer zone, so the place has to give way.
	nameColLongPlace = location.PlaceInfo{Name: "Constructedville-upon-Testwater", Country: "TS", Lat: 52.5, Lon: 13.4, TZ: "Europe/Berlin"}
)

func TestAbbreviateName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Constructed One", "Constructed One"},
		{"Fixture van der Longsurname", "Fixture v.d. L."},
		{"Fixturella van der Lin", "Fixturella v.d. Lin"},
		{"Fixture Q. Constructedsurname", "Fixture Q. C."},
		{"Supercalifragilisticexpialidocious", "Supercalifragilisti…"},
		{"Verylonggivenname Middlenameonly Doe", "Verylonggivenname M…"},
		{"van Fixture", "van Fixture"},
		{"Fixturella Van Der Longsurname", "Fixturella V.D. L."},
		{"De La Fixturesurname-Longer Fixture", "De La Fixturesurnam…"},
		// Failure modes: wide runes are measured in cells and never split;
		// whitespace collapses before anything is measured.
		{"構成 例", "構成 例"},
		{"  Constructed   One ", "Constructed One"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := abbreviateName(tc.in, Config{})
			if got != tc.want {
				t.Errorf("abbreviateName(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if w := ansi.StringWidth(got); w > nameColumnMax {
				t.Errorf("width %d exceeds %d", w, nameColumnMax)
			}
		})
	}

	// B6: every particle in the set, and nothing outside it. Each name is too
	// wide as written, so step (a) runs; (b) then fits.
	t.Run("exactly the particle set", func(t *testing.T) {
		spec := strings.Fields("van von der den de del della di da du la le ter te ten zu dos das do")
		want := map[string]bool{}
		for _, p := range spec {
			want[p] = true
		}
		if !reflect.DeepEqual(nameParticles, want) {
			t.Fatalf("nameParticles = %v, want exactly the B6 set %v", nameParticles, spec)
		}
		for _, p := range spec {
			for _, form := range []string{p, strings.ToUpper(p)} {
				in := "Fixtureone " + form + " Longsurnamexyz"
				want := "Fixtureone " + form[:1] + ". L."
				if got := abbreviateName(in, Config{}); got != want {
					t.Errorf("abbreviateName(%q) = %q, want %q (a particle)", in, got, want)
				}
			}
		}
		for _, notP := range []string{"vanx", "dello", "dal", "zum", "des", "y"} {
			in := "Fixtureone " + notP + " Longsurnamexyz"
			want := "Fixtureone " + notP + " L."
			if got := abbreviateName(in, Config{}); got != want {
				t.Errorf("abbreviateName(%q) = %q, want %q (not a particle)", in, got, want)
			}
		}
	})

	t.Run("12-rune CJK single token", func(t *testing.T) {
		in := strings.Repeat("構", 12) // 24 cells
		got := abbreviateName(in, Config{})
		if w := ansi.StringWidth(got); w > nameColumnMax {
			t.Errorf("width %d exceeds %d: %q", w, nameColumnMax, got)
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("%q does not end in …", got)
		}
		if strings.ContainsRune(got, '\uFFFD') || !strings.HasPrefix(in, strings.TrimSuffix(got, "…")) {
			t.Errorf("%q split a rune", got)
		}
	})
}

// twoSlots is an anchored slot at birth/birthPlace and the floating slot at
// the frozen test moment in Berlin.
func twoSlots(t *testing.T, birth time.Time, birthPlace location.PlaceInfo) *store.Slots {
	t.Helper()
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := addAnchor(slots, birth, birthPlace); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	return slots
}

// stripRows renders the Chart body and returns its rows without styling.
// stripRows renders the panel unfocused, so no cursor column takes cells from
// the layout these tests are about.
func stripRows(slots *store.Slots, name string, width int) []string {
	firstAnchor(slots).Chart.Name = name // the row's name is its own slot's chart
	body := chartSlotsBody(slots, 1, false, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic, width, Config{Accessible: true})
	return strings.Split(ansi.Strip(body), "\n")
}

// cellOf is the display-cell index at which sub first starts in line, or -1.
func cellOf(line, sub string) int {
	i := strings.Index(line, sub)
	if i < 0 {
		return -1
	}
	return ansi.StringWidth(line[:i])
}

func TestSlotRowNameColumn(t *testing.T) {
	birth := time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC)
	slots := twoSlots(t, birth, nameColUTC)
	prefixW := ansi.StringWidth(rowPrefix(false, false, true, true, Config{Accessible: true}))

	t.Run("short name sets N to its width", func(t *testing.T) {
		rows := stripRows(slots, "Constructed One", 78)
		const want = "Constructed One  Sat Feb  3, 2001  4:05 UTC"
		if got := cellOf(rows[0], want); got != prefixW {
			t.Errorf("anchored row = %q, want %q right after the %d-cell prefix", rows[0], want, prefixW)
		}
		if got := cellOf(rows[1], "Sat Apr 25, 2026"); got != prefixW+15+2 {
			t.Errorf("floating moment at cell %d, want %d (N = 15)", got, prefixW+17)
		}
	})

	t.Run("a 31-cell name caps N at 20", func(t *testing.T) {
		name := "Constructedname-Longerthanthecap" // one token, wider than the cap
		rows := stripRows(slots, name, 78)
		if got := cellOf(rows[0], "Sat Feb  3"); got != prefixW+20+2 {
			t.Errorf("anchored moment at cell %d, want %d (N = 20): %q", got, prefixW+22, rows[0])
		}
		if !strings.Contains(rows[0], abbreviateName(name, Config{Accessible: true})+"  Sat") {
			t.Errorf("anchored row does not show the shortened name: %q", rows[0])
		}
	})

	t.Run("an empty name renders as before names", func(t *testing.T) {
		// Written out, not rendered by the code under test: the row exactly as
		// it was before names existed.
		// Changed by selected-charts-compare C14: the selected row's marker is
		// its numeral, `1` in this ASCII-mode fixture, no longer `*`.
		const left, place = "1 Sat Feb  3, 2001  4:05 UTC ", "Test Bay, TS"
		want := left + place + strings.Repeat(" ", 78-len(left)-len(place)-len("WhTr")) + "WhTr"
		for _, name := range []string{"", "   "} {
			rows := stripRows(slots, name, 78)
			if rows[0] != want {
				t.Errorf("anchored row with name %q = %q, want the nameless rendering %q", name, rows[0], want)
			}
		}
	})

	// B9: the name is the slot's Chart.Name, and only that.
	t.Run("the name comes from the slot's Chart.Name, not the id", func(t *testing.T) {
		m := readyModelWithSlots(t, testPlaceBerlin())
		if err := addAnchor(m.slots, birth, nameColUTC); err != nil {
			t.Fatal(err)
		}
		firstAnchor(m.slots).Chart = store.SessionChart{ID: "fixture-id-zz", Name: "Constructed One", When: birth, Place: nameColUTC}
		tm, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
		view := ansi.Strip(tm.(Model).View())
		if !strings.Contains(view, "Constructed One  Sat Feb  3, 2001") {
			t.Error("the Chart panel does not show the slot's Chart.Name before the moment")
		}
		if strings.Contains(view, "fixture-id-zz") {
			t.Error("the Chart panel shows the chart id")
		}
	})
}

// A width that cannot hold the name column still yields a row no wider than
// asked for.
func TestSlotRowNarrowerThanNameColumn(t *testing.T) {
	slots := twoSlots(t, time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC), nameColUTC)
	for _, width := range []int{18, 22} {
		for _, row := range stripRows(slots, "Fixture-Constructed-Longname", width) {
			if w := ansi.StringWidth(row); w > width {
				t.Errorf("width %d: row is %d cells: %q", width, w, row)
			}
		}
	}
}

func TestSlotRowsAlignMoment(t *testing.T) {
	slots := twoSlots(t, time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC), nameColUTC)
	for _, width := range []int{64, 78} {
		rows := stripRows(slots, "Constructed One", width)
		a, f := cellOf(rows[0], "Sat Feb  3"), cellOf(rows[1], "Sat Apr 25")
		if a < 0 || a != f {
			t.Errorf("width %d: moments start at cells %d and %d:\n%q\n%q", width, a, f, rows[0], rows[1])
		}
	}
}

func TestSlotRowsAlignPlace(t *testing.T) {
	// UTC (3 cells) against Berlin in summer, CEST (4 cells).
	slots := twoSlots(t, time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC), nameColUTC)
	rows := stripRows(slots, "Constructed One", 78)
	if !strings.Contains(rows[0], " UTC ") || !strings.Contains(rows[1], " CEST ") {
		t.Fatalf("fixture zones are not UTC and CEST:\n%q\n%q", rows[0], rows[1])
	}
	a, f := cellOf(rows[0], nameColUTC.Display()), cellOf(rows[1], testPlaceBerlin().Display())
	if a < 0 || a != f {
		t.Errorf("places start at cells %d and %d:\n%q\n%q", a, f, rows[0], rows[1])
	}
}

func TestSlotRowWidth64(t *testing.T) {
	// A summer birth in Berlin: a four-letter zone, CEST.
	slots := twoSlots(t, time.Date(2001, 7, 3, 4, 5, 0, 0, time.UTC), nameColLongPlace)
	rows := stripRows(slots, "Fixture-Constructed-Longname", 64) // N = 20
	for _, row := range rows {
		if w := ansi.StringWidth(row); w != 64 {
			t.Errorf("row width %d, want 64: %q", w, row)
		}
		if !strings.HasSuffix(row, "WhTr") {
			t.Errorf("WhTr is not pinned to the right edge: %q", row)
		}
	}
	row := rows[0]
	prefixW := ansi.StringWidth(rowPrefix(false, false, true, true, Config{Accessible: true}))
	if got := cellOf(row, "Tue Jul  3, 2001  6:05 CEST "); got != prefixW+nameColumnMax+2 {
		t.Fatalf("fixture is not N = 20 with a CEST anchored zone: moment at cell %d in %q", got, row)
	}
	start := strings.Index(row, "CEST ") + len("CEST ")
	place := strings.TrimRight(strings.TrimSuffix(row, "WhTr"), " ")[start:]
	if w := ansi.StringWidth(place); w < 7 {
		t.Errorf("place kept %d cells (%q), want at least 7", w, place)
	}
	if !strings.HasSuffix(place, "...") {
		t.Errorf("long place %q was not truncated with ...", place)
	}
}
