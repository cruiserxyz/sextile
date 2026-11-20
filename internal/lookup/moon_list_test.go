package lookup

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The Moon list's rows: the transiting Moon's aspects to each natal, and the
// sky's Moon rows, at one shared instant for the tie-break.
const (
	moonT1    = " 5/ 7/2026  5:03am trans    Moon (Tau) Con natal (Tau) Mercury"
	moonT2    = " 5/ 7/2026  5:03am trans    Moon (Tau) Sex natal (Can) Venus"
	moonLunar = " 5/ 6/2026 10:13am trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)"
	moonSky   = "Thu  5/ 7/2026  5:03am    Moon (Tau) Squ (Aqu) Pluto"
	moonSky0  = "Fri  5/ 1/2026  7:50am    Moon (Sco) Squ (Aqu) Pluto"
)

func moonSearch() *numeralSearch {
	return &numeralSearch{out: map[string]string{
		k1t:  mayTransitHeader + moonLunar + "\n" + moonT1 + "\n",
		k2t:  mayTransitHeader + moonT2 + "\n",
		kSky: moonSky0 + "\n" + moonSky + "\n",
	}}
}

func moonComparing() TimelineRequest {
	req := comparingRequest()
	req.Moon = true
	req.Voids = nil
	return req
}

// M3: per natal one transit search whose transiting side is the Moon alone, the
// sky, and no return search.
func TestMoonTransitArgv(t *testing.T) {
	amPmMonthFirst(t)
	reg := lookupTestRegistry()
	moon, ok := reg.ByName("Moon")
	if !ok {
		t.Fatal("test registry has no Moon")
	}
	c := moonSearch()
	var argvs [][]string
	run := func(ctx context.Context, call BodyCall) (string, string, error) {
		argvs = append(argvs, call.Argv())
		return c.run(ctx, call)
	}
	if _, err := TimelineWith(context.Background(), moonComparing(), run, stubDefaults); err != nil {
		t.Fatal(err)
	}
	var transits int
	for _, argv := range argvs {
		if slices.Contains(argv, "-tr") {
			t.Errorf("a return search ran: %v", argv)
		}
		if slices.Contains(argv, "-t") {
			transits++
			i := slices.Index(argv, "-RT0")
			if i < 0 || argv[i+1] != strconv.Itoa(moon.Number) || (i+2 < len(argv) && !strings.HasPrefix(argv[i+2], "-") && !strings.HasPrefix(argv[i+2], "_") && !strings.HasPrefix(argv[i+2], ":")) {
				t.Errorf("transit call %v, want -RT0 %d alone", argv, moon.Number)
			}
		}
	}
	if transits != 2 {
		t.Errorf("%d transit searches, want one per natal (2)", transits)
	}
	if c.calls[0] != k1t || c.calls[1] != k2t {
		t.Errorf("spawn order %v, want each natal's -t first", c.calls[:2])
	}

	// A single anchored chart: -t and the sky, no -tr either.
	single := mayRequest()
	single.Moon = true
	c = moonSearch()
	var modes []string
	if _, err := TimelineWith(context.Background(), single, func(ctx context.Context, call BodyCall) (string, string, error) {
		for _, m := range []string{"-tr", "-t"} {
			if slices.Contains(call.Argv(), m) {
				modes = append(modes, m)
				return mayTransitHeader + moonLunar + "\n", "astrolog", nil
			}
		}
		return c.run(ctx, call)
	}, stubDefaults); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(modes, []string{"-t"}) {
		t.Errorf("single-natal Moon list ran %v, want one -t", modes)
	}
}

// With no natals the Moon list is the sky alone.
func TestMoonListWithoutNatalsIsTheSkyAlone(t *testing.T) {
	amPmMonthFirst(t)
	c := moonSearch()
	req := TimelineRequest{When: moonComparing().When, Place: timelinePlace(), Lens: Lens{Registry: lookupTestRegistry(), AspectRegistry: testAspects()}, Moon: true}
	res, err := TimelineWith(context.Background(), req, c.run, stubDefaults)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range c.calls {
		if strings.HasPrefix(k, "-t") {
			t.Errorf("search %s ran with no natal", k)
		}
	}
	if got := lines(res.Rows); !slices.Equal(got, []string{moonSky0, moonSky}) {
		t.Errorf("rows = %q, want the sky's Moon rows", got)
	}
}

// M4: Astrolog's lines as printed, the numeral on transit rows only, in time
// order with the Timeline's tie-break (transit, sky; then numeral).
func TestMoonRowsAreAstrologLines(t *testing.T) {
	amPmMonthFirst(t)
	res, err := TimelineWith(context.Background(), moonComparing(), moonSearch().run, stubDefaults)
	if err != nil {
		t.Fatal(err)
	}
	want := []numbered{{0, moonSky0}, {1, moonLunar}, {1, moonT1}, {2, moonT2}, {0, moonSky}}
	if got := numberedLines(res.Rows); !slices.Equal(got, want) {
		t.Errorf("rows =\n%v\nwant\n%v", got, want)
	}
}
