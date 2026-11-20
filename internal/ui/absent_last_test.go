package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// absent_last_test.go: the bottom line says what Astrolog said, and the
// startup account and the drained selection are one line each, in the large
// log only: a body without data is left out quietly.

func TestBottomLineIsAstrologsWords(t *testing.T) {
	reg := []aspect.RegistryObject{{Object: aspect.Object{Number: 1, Name: "Sun"}}, {Object: aspect.Object{Number: 12, Name: "Ceres"}}, {Object: aspect.Object{Number: 52, Name: "Titan"}}}
	said := "SwissEph file 'sepm9606.se1' not found in PATH '/usr/local/share/astrolog/'"
	st := popup.Open(reg, recipe.ObjectPolicy{}, nil)
	st.SetAbsent(map[int]string{12: "seas_18.se1", 52: "sepm9606.se1"}, map[int]string{52: said})

	if got, ok := popup.ProbeStatusFor(st, reg[2]); !ok || got != "Astrolog: "+said {
		t.Errorf("Titan's line = %q (%v), want Astrolog's own words", got, ok)
	}
	// Astrolog wrote nothing for Ceres: the line says so, naming no file even
	// though the probe knows which one the object needs.
	if got, ok := popup.ProbeStatusFor(st, reg[1]); !ok || got != "Astrolog cannot place Ceres here and did not say why" {
		t.Errorf("Ceres's line = %q (%v), want the no-words form", got, ok)
	}
	if got, ok := popup.ProbeStatusFor(st, reg[0]); ok {
		t.Errorf("a drawable body has the line %q", got)
	}

	// A line longer than two rows is cut after the second, with the mark.
	long := "Astrolog says " + strings.Repeat("a very long sentence ", 12)
	st.SetAbsent(map[int]string{52: "sepm9606.se1"}, map[int]string{52: long})
	st.Cursor = popup.GroupRows() + 2
	frame := ansi.Strip(renderBodyModal(st, Config{Accessible: true, Color: ColorNever}, 132, 40))
	rows := 0
	for _, l := range bottomLines(frame) {
		if strings.Contains(l, "a very long sentence") || strings.Contains(l, "Astrolog: Astrolog says") {
			rows++
		}
	}
	if rows != 2 {
		t.Errorf("the long line takes %d rows, want 2:\n%s", rows, frame)
	}
	if !strings.Contains(frame, "…") && !strings.Contains(frame, "...") {
		t.Errorf("the cut line carries no truncation mark:\n%s", frame)
	}
}

// absentMany is n absent bodies named "Body <i>", all lacking one file, on the
// host's default set so the view would draw them.
func absentMany(n int, file string) lookup.EphemerisAvailability {
	av := lookup.EphemerisAvailability{Folder: "many", Absent: map[int]string{}, Names: map[int]string{}, Default: []int{1, 2}}
	for i := 0; i < n; i++ {
		num := 100 + i
		av.Absent[num] = file
		av.Names[num] = fmt.Sprintf("Body %d", i)
		av.Default = append(av.Default, num)
	}
	return av
}

func TestOneAccountLine(t *testing.T) {
	for _, c := range []struct {
		name string
		av   lookup.EphemerisAvailability
		want string
	}{
		{"three bodies, one file", absentMany(3, "seas_18.se1"), "Body 0, Body 1, Body 2 can't be drawn here: no file seas_18.se1"},
		{"one body", absentMany(1, "sepm9606.se1"), "Body 0 can't be drawn here: no file sepm9606.se1"},
		{"one body, no file named", absentMany(1, ""), "Body 0 can't be drawn here: no data for this date"},
		{"four bodies, one file", absentMany(4, "seas_18.se1"), "4 bodies can't be drawn here. Press Ctrl+B to see which."},
		{"thirty-two bodies", absentMany(32, "sepm9401.se1"), "32 bodies can't be drawn here. Press Ctrl+B to see which."},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := timelineModel(t, Config{Accessible: true})
			panel, console := len(m.logPanelLines()), len(consoleLineTexts(m))
			m, cmd := applyEphemerisProbed(m, EphemerisProbedMsg{Availability: c.av})
			if cmd != nil {
				t.Error("a footer notice was started")
			}
			if got := m.logPanelLines()[panel:]; len(got) != 0 {
				t.Errorf("the Log panel took %q, want nothing", got)
			}
			if got := consoleLineTexts(m)[console:]; len(got) != 1 || got[0] != "bodies: "+strings.TrimSuffix(c.want, ".") {
				t.Errorf("the console took %q, want exactly %q", got, "bodies: "+c.want)
			}
			if got := m.footerIndicator(); strings.Contains(got, "can't be drawn") {
				t.Errorf("footer = %q, want no notice", got)
			}
		})
	}
	// Two files, three bodies: the count, not the names.
	av := absentMany(3, "a.se1")
	av.Absent[102] = "b.se1"
	m := timelineModel(t, Config{Accessible: true})
	console := len(consoleLineTexts(m))
	m, _ = applyEphemerisProbed(m, EphemerisProbedMsg{Availability: av})
	if got := consoleLineTexts(m)[console:]; len(got) != 1 || got[0] != "bodies: 3 bodies can't be drawn here. Press Ctrl+B to see which" {
		t.Errorf("two files: the console took %q", got)
	}
}

// Every drained-selection sentence is short enough for the Log at 132 columns,
// including the widest the rule lets through and the names that push a sentence
// over into the count: the mixed case (a body the chart lacks) is still on screen.
func TestDrainLinesFitTheLog(t *testing.T) {
	long := func(av lookup.EphemerisAvailability) lookup.EphemerisAvailability {
		for n := range av.Names {
			av.Names[n] = fmt.Sprintf("Asteroid %d with a very long registry name", n)
		}
		return av
	}
	for _, c := range []struct {
		name string
		av   lookup.EphemerisAvailability
	}{
		{"three short names", absentMany(3, "seas_18.se1")},
		{"three long names", long(absentMany(3, "se_a_long_file_name_for_one_asteroid.se1"))},
		{"many", absentMany(32, "sepm9401.se1")},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := readyModelWithSlots(t, testPlaceBerlin())
			m.cfg = Config{Accessible: true, Color: ColorNever}
			m.width, m.height = 132, 40
			limit := m.logLineLimit()
			if limit < 40 {
				t.Fatalf("the Log's line limit at 132 columns is %d", limit)
			}
			m.ephemeris = c.av
			for _, nums := range [][]int{{100}, {100, 101, 102}, {100, 101, 102, 103}} {
				if d := m.drainedLine(nums); ansi.StringWidth(d) > limit {
					t.Errorf("%q is %d wide, the Log holds %d", d, ansi.StringWidth(d), limit)
				}
			}
		})
	}
}

func TestOneDrainedLine(t *testing.T) {
	m := timelineModel(t, Config{Accessible: true})
	m.objectRegistry = []aspect.RegistryObject{
		{Object: aspect.Object{Number: 5, Name: "Mars"}}, {Object: aspect.Object{Number: 6, Name: "Mercury"}},
		{Object: aspect.Object{Number: 7, Name: "Jupiter"}}, {Object: aspect.Object{Number: 8, Name: "Saturn"}},
	}
	m.ephemeris = absentMany(4, "sepm9606.se1")
	m.ephemeris.Names[100] = "Titan"
	m.ephemeris.Names[101] = "Io"
	const tail = ", all are shown"
	const short = tail
	for _, c := range []struct {
		name string
		nums []int
		want string
	}{
		{"one absent", []int{100}, "Titan selected but can't be drawn here" + tail},
		{"two absent", []int{100, 101}, "the selected bodies can't be drawn here" + tail},
		{"one not drawn", []int{5}, "Mars not among this chart's bodies" + short},
		{"two not drawn", []int{5, 6}, "the selected bodies are not drawn" + short},
		{"absent and not drawn", []int{5, 100}, "Mars, Titan not among this chart's bodies" + short},
		{"more than three absent", []int{100, 101, 102, 103}, "the selected bodies can't be drawn here" + tail},
		{"more than three not drawn", []int{5, 6, 7, 8}, "the selected bodies are not drawn" + short},
		{"nothing", nil, ""},
	} {
		if got := m.drainedLine(c.nums); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	// A sentence wider than the Log falls back to the general one.
	m.ephemeris.Names[100] = strings.Repeat("Long ", 40)
	if got := m.drainedLine([]int{100}); got != "the selected bodies can't be drawn here"+tail {
		t.Errorf("a name wider than the Log: %q", got)
	}
}

// The bound the one-line accounts are measured against is the Log's inner
// width at 132x40 with one chart shown, read from the layout.
func TestLogLineLimitIsTheLogsInnerWidth(t *testing.T) {
	m := timelineModel(t, Config{Accessible: true})
	r, ok := rectsAt(132, 40, 1, 1)[PanelLog]
	if !ok {
		t.Fatal("no Log at 132x40")
	}
	t.Logf("Log rect %+v", r)
	if got := m.logLineLimit(); got != r.w-2 {
		t.Errorf("limit %d, the Log's inner width is %d", got, r.w-2)
	}
}
