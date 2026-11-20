package astrolog

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// voidTestRegistry is a registry whose Moon is NOT object 2 and that has
// objects above Pluto: the void search must name both by the registry.
func voidTestRegistry() aspect.Registry {
	names := map[int]string{0: "Earth", 1: "Sun", 2: "Venus", 3: "Moon", 4: "Mercury", 5: "Mars", 6: "Jupiter",
		7: "Saturn", 8: "Uranus", 9: "Neptune", 11: "Pluto", 12: "Ceres", 20: "Vesta"}
	var reg aspect.Registry
	for n := 0; n <= 20; n++ {
		if name, ok := names[n]; ok {
			reg = append(reg, aspect.RegistryObject{Object: aspect.Object{Number: n, Name: name}})
		}
	}
	return reg
}

func voidArgv(t *testing.T, req VoidRequest) []string {
	t.Helper()
	call, err := VoidCall(req)
	if err != nil {
		t.Fatal(err)
	}
	return call.Argv()
}

func TestVoidSearchArgv(t *testing.T) {
	reg := voidTestRegistry()
	nov := time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)
	argv := voidArgv(t, VoidRequest{Moment: nov, Offset: 3600, Zodiac: recipe.Tropical, Registry: reg})
	if argv[0] != "-z" || argv[1] != formatSearchZone(3600) {
		t.Errorf("argv starts %v, want the zone first", argv[:2])
	}
	joined := " " + strings.Join(argv, " ") + " "
	for _, want := range []string{" -qy 2026 -dY 2 ", " -R0 1 2 3 4 5 6 7 8 9 11 -RA0 1 2 3 4 5 ",
		" -~d Or Equ @u 3 And Gt @v 0 Equ @w 3"} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv %q lacks %q", joined, want)
		}
	}
	if strings.Contains(joined, " 12 ") || strings.Contains(joined, " 20 ") {
		t.Errorf("argv %q names an object above Pluto", joined)
	}
	if i, j := slices.Index(argv, "-dY"), slices.Index(argv, "-R0"); !(i < slices.Index(argv, "-c") && slices.Index(argv, "-c") < j) {
		t.Errorf("order: mode %d, frame %d, band %d, want mode, frame, band", i, slices.Index(argv, "-c"), j)
	}
	mar := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	argv = voidArgv(t, VoidRequest{Moment: mar, Offset: 3600, Zodiac: recipe.Tropical, Registry: reg})
	if !slices.Contains(argv, "-qy") || argv[slices.Index(argv, "-qy")+1] != "2025" {
		t.Errorf("March 2026 argv %v, want -qy 2025", argv)
	}
}

func TestVoidSearchFollowsZodiac(t *testing.T) {
	reg := voidTestRegistry()
	moment := time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)
	trop := VoidRequest{Moment: moment, Offset: 3600, Zodiac: recipe.Tropical, Registry: reg}
	sid := trop
	sid.Zodiac = recipe.Lahiri
	if slices.Equal(voidArgv(t, trop), voidArgv(t, sid)) {
		t.Error("a sidereal zodiac left the argv unchanged")
	}
	if VoidKeyFor(trop) == VoidKeyFor(sid) {
		t.Error("a sidereal zodiac left the key unchanged")
	}
	other := trop
	other.Registry = reg[:len(reg)-1]
	if VoidKeyFor(trop) == VoidKeyFor(other) {
		t.Error("a different registry left the key unchanged")
	}
}

// TestVoidSearchCarriesTheLensHouses: every displayed search carries the lens's
// frame, the void search too, though the houses move no void; the key follows
// the argv.
func TestVoidSearchCarriesTheLensHouses(t *testing.T) {
	reg := voidTestRegistry()
	moment := time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)
	plac := VoidRequest{Moment: moment, Offset: 3600, Zodiac: recipe.Tropical, Registry: reg}
	whole := plac
	whole.Houses = recipe.WholeSign
	want, err := frameBand(recipe.WholeSign, recipe.Tropical)
	if err != nil {
		t.Fatal(err)
	}
	if !containsRun(voidArgv(t, whole), want) {
		t.Errorf("the void search does not carry the lens frame %q", want)
	}
	if VoidKeyFor(plac) == VoidKeyFor(whole) {
		t.Error("another house system left the key unchanged")
	}
}

func TestVoidSearchNeedsAUniquePluto(t *testing.T) {
	moment := time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)
	reg := voidTestRegistry()
	noPluto := slices.DeleteFunc(slices.Clone(reg), func(o aspect.RegistryObject) bool { return o.Name == "Pluto" })
	twice := append(slices.Clone(reg), aspect.RegistryObject{Object: aspect.Object{Number: 30, Name: "Pluto"}})
	noMoon := slices.DeleteFunc(slices.Clone(reg), func(o aspect.RegistryObject) bool { return o.Name == "Moon" })
	for name, r := range map[string]aspect.Registry{"no Pluto": noPluto, "two Plutos": twice, "no Moon": noMoon} {
		if _, err := VoidCall(VoidRequest{Moment: moment, Registry: r}); !errors.Is(err, ErrVoidRegistry) {
			t.Errorf("%s: err = %v, want ErrVoidRegistry", name, err)
		}
	}
}

func TestVoidSearchRefusesRowsWithoutTheMoon(t *testing.T) {
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	reg := testRegistry(t)
	const row = "Sat 10/ 3/2026  8:15am   Venus <Sco> S/R"
	_, err := ReadVoids(row+"\n", 3600, 2026, reg, testAspects(t))
	if err == nil || !strings.Contains(err.Error(), "the Moon filter did not apply") || !strings.Contains(err.Error(), row) {
		t.Errorf("err = %v, want the filter-did-not-apply refusal naming the row", err)
	}
}

func TestVoidSearchFailureFailsTheSkySearch(t *testing.T) {
	boom := errors.New("void search broke")
	req := TimelineRequest{Year: 2026, Month: time.October, Zone: time.UTC, AspectRegistry: testAspects(t),
		Voids: func(context.Context, VoidRequest) (VoidResult, bool, error) { return VoidResult{}, false, boom }}
	search := func(_ context.Context, call BodyCall) (string, string, error) { return "", "astrolog", nil }
	if _, _, err := ComputeTimeline(context.Background(), req, []TimelineSource{TimelineSky}, search); !errors.Is(err, boom) {
		t.Errorf("ComputeTimeline err = %v, want the void search's", err)
	}
	results, err := ComputeTimelineCalls(context.Background(), req, TimelineSky, search)
	if err != nil || len(results) == 0 || !errors.Is(results[0].Err, boom) || len(results[0].Rows) != 0 {
		t.Errorf("segment results = %+v, %v, want a failed segment with no rows", results, err)
	}
}

func TestSkyMarksComeFromVoidSearch(t *testing.T) {
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	const (
		bare   = "Sat 10/ 3/2026  1:10am    Moon (Leo) Sex [Lib] Venus"
		marked = bare + " (v/c 8:17)"
		other  = "Sat 10/ 3/2026 11:50pm      Io (Leo) Con (Leo) Europa"
	)
	parsed, err := ParseTimeline(marked+"\n", time.UTC, nil, testAspects(t))
	if err != nil {
		t.Fatal(err)
	}
	parsed[0].Source, parsed[0].Command = TimelineSky, "astrolog void"
	req := TimelineRequest{Year: 2026, Month: time.October, Zone: time.UTC, AspectRegistry: testAspects(t),
		Voids: func(_ context.Context, vr VoidRequest) (VoidResult, bool, error) {
			return VoidResult{Rows: parsed, Command: "astrolog void"}, true, nil
		}}
	search := func(_ context.Context, call BodyCall) (string, string, error) {
		argv := call.Argv()
		command := "astrolog " + strings.Join(argv, " ")
		if slices.Contains(argv, "-d") && argv[slices.Index(argv, "-qd")+2] == "3" {
			return bare + "\n" + other + "\n", command, nil
		}
		return "", command, nil
	}
	rows, commands, err := ComputeTimeline(context.Background(), req, []TimelineSource{TimelineSky}, search)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Line != marked || rows[1].Line != other {
		t.Fatalf("rows =\n%s want the void search's marked row and the day search's other row", dumpRows(rows))
	}
	if rows[0].Command != "astrolog void" || !strings.Contains(rows[1].Command, " -qd 10 3 2026 -d ") {
		t.Errorf("commands %q and %q, want the void search's and the 3rd's day search's", rows[0].Command, rows[1].Command)
	}
	if !slices.Contains(commands, "astrolog void") {
		t.Errorf("commands %v lack the void search's", commands)
	}
}

// The October 2026 list marks the last Moon aspect of the month, which the
// month search missed because the Moon's next sign change is in November
// (Astrolog_8.0 NEXT.md BUG 9).
func TestMonthEndVoidIsMarked(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	reg := testRegistry(t)
	ams, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Skip(err)
	}
	rows, _, err := ComputeTimeline(context.Background(),
		TimelineRequest{Year: 2026, Month: time.October, Zone: ams, Registry: reg, AspectRegistry: testAspects(t)}, []TimelineSource{TimelineSky}, nil)
	if err != nil {
		t.Fatal(err)
	}
	const want = "Sat 10/31/2026 10:59pm    Moon (Can) Squ [Lib] Venus (v/c 6:18)"
	for _, row := range rows {
		if row.Line == want {
			return
		}
	}
	t.Errorf("no row %q among %d rows", want, len(rows))
}

// noVoids stands in for the void search in a test that scripts the day
// searches and does not look at the marks.
func noVoids(context.Context, VoidRequest) (VoidResult, bool, error) { return VoidResult{}, false, nil }

func withNoVoids(req TimelineRequest) TimelineRequest {
	req.Voids = noVoids
	return req
}

// Astrolog ends a long search with a false-alarm line after its last row. It is
// read only there: last in the output, after a row on 31 December of the
// window's second year.
func TestVoidSearchToleratesOnlyTheLastDayFalseAlarm(t *testing.T) {
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	reg := testRegistry(t)
	const (
		early = "Thu 12/30/2027  2:14am    Moon (Gem) Tri [Aqu] Venus\n"
		last  = "Fri 12/31/2027  6:05pm    Moon (Gem) Tri (Lib) Pluto\n"
		alarm = skyOverflowLine + "\n"
	)
	res, err := ReadVoids(early+last+alarm, 0, 2026, reg, testAspects(t))
	if err != nil || len(res.Rows) != 2 {
		t.Errorf("after a 31 Dec row: %d rows, err %v, want both rows read", len(res.Rows), err)
	}
	for name, out := range map[string]string{
		"mid-output":                     early + alarm + last,
		"after an earlier day":           early + alarm,
		"after 31 Dec of the wrong year": "Thu 12/31/2026  6:05pm    Moon (Gem) Tri (Lib) Pluto\n" + alarm,
		"alone":                          alarm,
	} {
		_, err := ReadVoids(out, 0, 2026, reg, testAspects(t))
		var lineErr *TimelineLineError
		if !errors.As(err, &lineErr) || lineErr.Line != skyOverflowLine {
			t.Errorf("%s: err = %v, want a refusal naming the line", name, err)
		}
	}
}
