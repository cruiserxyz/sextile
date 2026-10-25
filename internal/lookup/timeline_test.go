package lookup

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The canned outputs below are real Astrolog 8.00 lines for May 2026 at
// `-z -2` (Amsterdam is CEST all month, so May is one offset segment), taken
// 2026-09-30 on the spec's synthetic natal. Where a case needs a shape the
// real month does not happen to contain, the edited line says so.

const mayTransitHeader = "Astrolog 8.00 chart for Tue May 15, 1979  9:30am (ST Zone 0W)   4:54E 52:22N\n\nTransits during May 2026 (ST Zone 2E)\n"

var (
	mayTransit = mayTransitHeader +
		" 5/ 1/2026 12:54pm trans Mercury (Ari) Con natal (Ari) Venus\n" +
		" 5/ 1/2026  8:20pm trans     Sun (Tau) Squ natal (Leo) Ascendant\n" +
		" 5/ 7/2026  5:03am trans Mercury (Tau) Con natal (Tau) Mercury (Mercury Return)\n" +
		" 5/14/2026  8:20pm trans     Sun (Tau) Con natal (Tau) Sun (Solar Return)\n" +
		" 5/18/2026  5:31am trans    Mars (Ari) Con natal (Ari) Mars (Mars Return)\n"
	maySky = "Fri  5/ 1/2026  7:50am    Moon (Sco) Squ (Aqu) Pluto\n" +
		"Fri  5/ 1/2026  7:23pm     Sun (Tau) Opp (Sco) Moon (Full Moon)\n" +
		"Sat  5/ 2/2026 10:47am    Moon (Sco) Tri (Can) Jupiter (v/c 21:46)\n" +
		"Thu  5/14/2026  4:24pm     Sun (Tau) Con (Tau) Mercury (Total Solar Occultation)\n"
	mayReturn = mayTransitHeader +
		" 5/ 6/2026 10:13am trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)\n" +
		" 5/ 7/2026  5:03am trans Mercury (Tau) Con natal (Tau) Mercury (Mercury Return)\n" +
		" 5/14/2026  8:20pm trans     Sun (Tau) Con natal (Tau) Sun (Solar Return)\n" +
		" 5/18/2026  5:31am trans    Mars (Ari) Con natal (Ari) Mars (Mars Return)\n"
)

// cannedSearch answers each search mode with its canned output and records
// the argv of every call. Its command is the argv joined, so a row's command
// names its search unambiguously.
type cannedSearch struct {
	byMode map[string]string
	argvs  [][]string
}

func (c *cannedSearch) run(_ context.Context, call BodyCall) (string, string, error) {
	argv := call.Argv()
	c.argvs = append(c.argvs, slices.Clone(argv))
	for _, mode := range []string{"-tr", "-t", "-dm"} {
		if slices.Contains(argv, mode) {
			return c.byMode[mode], "astrolog " + strings.Join(argv, " "), nil
		}
	}
	return "", "", nil
}

// stubDefaults answers the default-set read with the sets astrolog 8.00
// listed on 2026-10-03, so the tests that script the searcher need no binary.
func stubDefaults(context.Context) (DefaultSets, error) {
	return DefaultSets{
		Main:    []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 16, 22, 31},
		Transit: []int{1, 3, 4, 5, 6, 7, 8, 9, 10, 16},
	}, nil
}

func mode(command string) string {
	for _, m := range []string{" -tr ", " -t ", " -dm"} {
		if strings.Contains(command+" ", m) {
			return strings.TrimSpace(m)
		}
	}
	return "?"
}

func timelinePlace() location.PlaceInfo {
	return location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lat: 52.3667, Lon: 4.9, TZ: "Europe/Amsterdam"}
}

func timelineNatal() store.AnchorChart {
	return store.AnchorChart{When: time.Date(1979, 5, 15, 9, 30, 0, 0, time.UTC), Place: timelinePlace()}
}

func mayRequest() TimelineRequest {
	return TimelineRequest{When: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC), Place: timelinePlace(), Natal: timelineNatal()}
}

func lines(rows []TimelineRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Line
	}
	return out
}

// C7: the union of the three searches, in ascending instant order.
func TestTimelineMergesThreeSearchesInInstantOrder(t *testing.T) {
	amPmMonthFirst(t)
	c := &cannedSearch{byMode: map[string]string{"-t": mayTransit, "-dm": maySky, "-tr": mayReturn}}
	res, err := TimelineWith(context.Background(), mayRequest(), c.run, stubDefaults)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Fri  5/ 1/2026  7:50am    Moon (Sco) Squ (Aqu) Pluto",
		" 5/ 1/2026 12:54pm trans Mercury (Ari) Con natal (Ari) Venus",
		"Fri  5/ 1/2026  7:23pm     Sun (Tau) Opp (Sco) Moon (Full Moon)",
		" 5/ 1/2026  8:20pm trans     Sun (Tau) Squ natal (Leo) Ascendant",
		"Sat  5/ 2/2026 10:47am    Moon (Sco) Tri (Can) Jupiter (v/c 21:46)",
		" 5/ 6/2026 10:13am trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)",
		" 5/ 7/2026  5:03am trans Mercury (Tau) Con natal (Tau) Mercury (Mercury Return)",
		"Thu  5/14/2026  4:24pm     Sun (Tau) Con (Tau) Mercury (Total Solar Occultation)",
		" 5/14/2026  8:20pm trans     Sun (Tau) Con natal (Tau) Sun (Solar Return)",
		" 5/18/2026  5:31am trans    Mars (Ari) Con natal (Ari) Mars (Mars Return)",
	}
	if got := lines(res.Rows); !slices.Equal(got, want) {
		t.Fatalf("merged =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for i := 1; i < len(res.Rows); i++ {
		if res.Rows[i].Instant.Before(res.Rows[i-1].Instant) {
			t.Errorf("row %d at %v precedes row %d at %v", i, res.Rows[i].Instant, i-1, res.Rows[i-1].Instant)
		}
	}
	if len(res.Commands) != 3 {
		t.Errorf("commands = %v, want one per spawn", res.Commands)
	}
}

// C8: rows sharing an instant go transit, then sky, then return, and keep
// each search's own output order. The shared instant is edited in (Astrolog
// does not happen to print three sources at one minute in May): every line
// below is a real line re-timed to 5/ 1/2026 8:20pm, and the two transit
// rows are deliberately NOT in alphabetical order, so a sort on the text
// would show.
func TestTimelineOrdersEqualInstantsBySourceThenOutputOrder(t *testing.T) {
	amPmMonthFirst(t)
	c := &cannedSearch{byMode: map[string]string{
		"-t": mayTransitHeader +
			" 5/ 1/2026  8:20pm trans     Sun (Tau) Squ natal (Leo) Ascendant\n" +
			" 5/ 1/2026  8:20pm trans Mercury (Ari) Con natal (Ari) Venus\n",
		"-dm": "Fri  5/ 1/2026  8:20pm     Sun (Tau) Opp (Sco) Moon (Full Moon)\n" +
			"Fri  5/ 1/2026  8:20pm    Moon (Sco) Squ (Aqu) Pluto\n",
		"-tr": mayTransitHeader +
			" 5/ 1/2026  8:20pm trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)\n",
	}}
	res, err := TimelineWith(context.Background(), mayRequest(), c.run, stubDefaults)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		" 5/ 1/2026  8:20pm trans     Sun (Tau) Squ natal (Leo) Ascendant",
		" 5/ 1/2026  8:20pm trans Mercury (Ari) Con natal (Ari) Venus",
		"Fri  5/ 1/2026  8:20pm     Sun (Tau) Opp (Sco) Moon (Full Moon)",
		"Fri  5/ 1/2026  8:20pm    Moon (Sco) Squ (Aqu) Pluto",
		" 5/ 1/2026  8:20pm trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)",
	}
	if got := lines(res.Rows); !slices.Equal(got, want) {
		t.Fatalf("equal-instant order =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// C9 and § Failure modes: with no slot anchored, only the sky search runs —
// once per offset segment, and the natal searches never spawn.
func TestTimelineWithNoAnchorRunsOnlyTheSkySearch(t *testing.T) {
	amPmMonthFirst(t)
	c := &cannedSearch{byMode: map[string]string{"-t": mayTransit, "-dm": maySky, "-tr": mayReturn}}
	req := mayRequest()
	req.Natal = store.AnchorChart{}
	res, err := TimelineWith(context.Background(), req, c.run, stubDefaults)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.argvs) != 1 || !slices.Equal(c.argvs[0], []string{"-z", "-2", "-qm", "5", "2026", "-dm", "-c", "0", "_s", "_Ys", "_sr0", ":sz", "-qag", "10", "15", "1582", "12:00am", "0", "0:00E", "0:00N", "_Yt", "_Yd"}) {
		t.Fatalf("spawned %v, want exactly the one -dm search", c.argvs)
	}
	if got, want := lines(res.Rows), strings.Split(strings.TrimSuffix(maySky, "\n"), "\n"); !slices.Equal(got, want) {
		t.Fatalf("rows = %q, want only the sky rows %q", got, want)
	}

	// A transition month: still sky only, now once per segment.
	c.argvs = nil
	req.When = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	if _, err := TimelineWith(context.Background(), req, c.run, stubDefaults); err != nil {
		t.Fatal(err)
	}
	if len(c.argvs) != 2 || !slices.Contains(c.argvs[0], "-dm") || !slices.Contains(c.argvs[1], "-dm") {
		t.Fatalf("October spawned %v, want the -dm search once per segment", c.argvs)
	}
}

// C27: a row that two searches both emit — every return but the lunar one
// comes from -t and -tr alike, byte-identically — is presented once, with
// the transit search's command because transit comes first under C8.
// May 2026 because it HAS non-lunar returns; October's only return is lunar,
// and a dedupe removed from October would go unnoticed (positive control 4).
func TestTimelineDropsRowsDuplicatedAcrossSearches(t *testing.T) {
	amPmMonthFirst(t)
	c := &cannedSearch{byMode: map[string]string{"-t": mayTransit, "-dm": maySky, "-tr": mayReturn}}
	res, err := TimelineWith(context.Background(), mayRequest(), c.run, stubDefaults)
	if err != nil {
		t.Fatal(err)
	}
	for _, ret := range []string{"(Mercury Return)", "(Solar Return)", "(Mars Return)", "(Lunar Return)"} {
		n := 0
		for _, r := range res.Rows {
			if strings.HasSuffix(r.Line, ret) {
				n++
				if want := "-t"; ret != "(Lunar Return)" && mode(r.Command) != want {
					t.Errorf("%s kept the %s search's command %q, want the transit search's", ret, mode(r.Command), r.Command)
				}
			}
		}
		if n != 1 {
			t.Errorf("%s presented %d times, want once", ret, n)
		}
	}
	if len(res.Rows) != 10 {
		t.Errorf("rows = %d, want 5 transit + 4 sky + 4 return - 3 duplicates = 10", len(res.Rows))
	}
}

// C53: provenance is asserted on the MERGED, deduplicated list — every
// presented row carries the command of a search whose output holds exactly
// that line, and a deduplicated row carries the winning source's.
func TestMergedTimelineRowsAllRetainTheirArgv(t *testing.T) {
	amPmMonthFirst(t)
	c := &cannedSearch{byMode: map[string]string{"-t": mayTransit, "-dm": maySky, "-tr": mayReturn}}
	res, err := TimelineWith(context.Background(), mayRequest(), c.run, stubDefaults)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) == 0 {
		t.Fatal("no rows to check")
	}
	for _, r := range res.Rows {
		if r.Command == "" {
			t.Errorf("%q carries no command", r.Line)
			continue
		}
		out := c.byMode[mode(r.Command)]
		if !slices.Contains(strings.Split(out, "\n"), r.Line) {
			t.Errorf("%q carries %q, whose output does not contain it", r.Line, r.Command)
		}
		if strings.HasSuffix(r.Line, "(Solar Return)") && mode(r.Command) != "-t" {
			t.Errorf("the deduplicated %q carries %q, want the winning transit search's", r.Line, r.Command)
		}
		if strings.HasSuffix(r.Line, "(Lunar Return)") && mode(r.Command) != "-tr" {
			t.Errorf("%q is only in -tr but carries %q", r.Line, r.Command)
		}
	}
	mergedArgvAcrossSegments(t)
}

// C53 in a DST month, by EXACT invocation (review round 1): two searches of
// the same mode differ only in their -z, so a check by mode passes while a
// row carries the other segment's argv. Here each of the six invocations of
// October in Amsterdam answers with its own output, keyed by the full argv,
// and every presented row must carry the command of the one invocation
// whose output holds it AND whose segment holds its instant.
func mergedArgvAcrossSegments(t *testing.T) {
	t.Helper()
	head := func(zone string) string {
		return "Astrolog 8.00 chart for Tue May 15, 1979  9:30am (ST Zone 0W)   4:54E 52:22N\n\nTransits during Oct 2026 (ST Zone " + zone + ")\n"
	}
	byArgv := map[string]string{
		// -z -2 (CEST): each output is the whole month at +2; the first row of
		// each lands in the CEST segment, the last in neither's-but-CET's.
		"-t -2":  head("2E") + "10/ 2/2026  6:08am trans Jupiter (Leo) Tri natal [Sag] Neptune\n10/31/2026  2:12am trans     Sun (Sco) Tri natal [Pis] Saturn\n",
		"-t -1":  head("1E") + "10/ 2/2026  5:08am trans Jupiter (Leo) Tri natal [Sag] Neptune\n10/31/2026  1:12am trans     Sun (Sco) Tri natal [Pis] Saturn\n",
		"-dm -2": "Thu 10/ 1/2026  4:43am    Moon (Gem) Con [Gem] Uranus\nSat 10/31/2026 11:59pm    Moon (Can) Squ [Lib] Venus\n",
		"-dm -1": "Thu 10/ 1/2026  3:43am    Moon (Gem) Con [Gem] Uranus\nSat 10/31/2026 10:59pm    Moon (Can) Squ [Lib] Venus\n",
		// A second return after the fall-back (edited in: the real month has
		// one lunar return) so -tr has a row that only the -z -1 invocation
		// may carry (review round 2).
		"-tr -2": head("2E") + "10/17/2026  4:05am trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)\n10/30/2026  1:05pm trans     Sun (Sco) Con natal (Sco) Mars (Mars Return)\n",
		"-tr -1": head("1E") + "10/17/2026  3:05am trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)\n10/30/2026 12:05pm trans     Sun (Sco) Con natal (Sco) Mars (Mars Return)\n",
	}
	ident := func(argv []string) string {
		for _, m := range []string{"-tr", "-t", "-dm"} {
			if slices.Contains(argv, m) {
				return m + " " + argv[1]
			}
		}
		return "?"
	}
	outByCommand := map[string]string{}
	search := func(_ context.Context, call BodyCall) (string, string, error) {
		argv := call.Argv()
		cmd := "astrolog " + strings.Join(argv, " ")
		outByCommand[cmd] = byArgv[ident(argv)]
		return byArgv[ident(argv)], cmd, nil
	}
	req := mayRequest()
	req.When = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	res, err := TimelineWith(context.Background(), req, search, stubDefaults)
	if err != nil {
		t.Fatal(err)
	}
	if len(outByCommand) != 6 {
		t.Fatalf("%d distinct invocations, want six: three searches, two segments", len(outByCommand))
	}
	fallBack := time.Date(2026, 10, 25, 1, 0, 0, 0, time.UTC)
	lateReturn := false
	for _, r := range res.Rows {
		if strings.HasSuffix(r.Line, "(Mars Return)") {
			lateReturn = true
			if r.Command != "astrolog -z -1 "+strings.Join(natalArgv(), " ")+" -tr 10 2026 -c 0 _s _Ys _sr0 :sz -qag 10 15 1582 12:00am 0 0:00E 0:00N _Yt _Yd" {
				t.Errorf("the post-transition return carries %q, want exactly the -z -1 -tr invocation", r.Command)
			}
		}
	}
	if !lateReturn {
		t.Fatal("no -tr row after the transition: the fixture cannot see a wrong-segment return")
	}
	if len(res.Rows) != 6 {
		t.Fatalf("rows = %d, want 6, one per event:\n%v", len(res.Rows), lines(res.Rows))
	}
	for _, r := range res.Rows {
		out, ok := outByCommand[r.Command]
		if !ok {
			t.Fatalf("%q carries %q, which no invocation ran", r.Line, r.Command)
		}
		if !slices.Contains(strings.Split(out, "\n"), r.Line) {
			t.Errorf("%q carries %q, whose output does not print it", r.Line, r.Command)
		}
		wantZone := " -z -1 "
		if r.Instant.Before(fallBack) {
			wantZone = " -z -2 "
		}
		if !strings.Contains(r.Command+" ", wantZone) {
			t.Errorf("%q at %v carries %q, the other segment's invocation", r.Line, r.Instant.UTC(), r.Command)
		}
	}
}

// § Failure modes: a search that returns no events is not a failure; the
// other searches' rows stand. Synthetic — the real binary never returned an
// empty -t or -tr in 24 scanned months.
func TestTimelineAcceptsAnEmptySearch(t *testing.T) {
	amPmMonthFirst(t)
	c := &cannedSearch{byMode: map[string]string{"-t": mayTransit, "-dm": maySky, "-tr": mayTransitHeader}}
	res, err := TimelineWith(context.Background(), mayRequest(), c.run, stubDefaults)
	if err != nil {
		t.Fatalf("an empty return search failed the timeline: %v", err)
	}
	if len(res.Rows) != 9 {
		t.Fatalf("rows = %d, want the 5 transit and 4 sky rows", len(res.Rows))
	}
	for _, r := range res.Rows {
		if mode(r.Command) == "-tr" {
			t.Errorf("a row came from the empty search: %q", r.Line)
		}
	}
}

func natalArgv() []string {
	return []string{"-qj", "2444008.895833334", "-zl", "4:54E", "52:22N"}
}

// C12: when the default object sets cannot be read, the timeline is refused
// with a line and NO search is spawned — it never runs unfiltered. The
// control at the end proves the same fixture does spawn when the read works,
// so the zero spawns above it are the refusal and not an inert fixture.
func TestTimelineRefusesWithoutDefaultSets(t *testing.T) {
	amPmMonthFirst(t)
	failures := map[string]DefaultSetReader{
		"the read fails": func(context.Context) (DefaultSets, error) {
			return DefaultSets{}, errors.New("astrolog exec: signal: killed")
		},
		"the registry parser refuses the listing": func(context.Context) (DefaultSets, error) {
			return DefaultSets{}, errors.New("astrolog: -R1 -HO: object number 3 appears twice")
		},
	}
	for name, read := range failures {
		t.Run(name, func(t *testing.T) {
			c := &cannedSearch{byMode: map[string]string{"-t": mayTransit, "-dm": maySky, "-tr": mayReturn}}
			res, err := TimelineWith(context.Background(), mayRequest(), c.run, read)
			if err == nil {
				t.Fatal("the timeline ran without its default sets")
			}
			line, ok := TimelineRefusedLine(err)
			if !ok || !strings.Contains(line, "default body lists") {
				t.Errorf("TimelineRefusedLine = %q, %v; want a line naming the default lists", line, ok)
			}
			if !DefaultSetsRefused(err) {
				t.Error("DefaultSetsRefused = false for the refusal")
			}
			if len(c.argvs) != 0 || len(res.Rows) != 0 {
				t.Errorf("spawned %v and returned %d rows; a refused timeline spawns nothing", c.argvs, len(res.Rows))
			}
		})
	}
	c := &cannedSearch{byMode: map[string]string{"-t": mayTransit, "-dm": maySky, "-tr": mayReturn}}
	if _, err := TimelineWith(context.Background(), mayRequest(), c.run, stubDefaults); err != nil || len(c.argvs) == 0 {
		t.Fatalf("control: with the sets readable the timeline must spawn (err %v, spawns %d)", err, len(c.argvs))
	}
}

// C1 through lookup: the request's lens reaches every search, so hiding the
// Moon puts the band in all three, the natal side banded and the transiting
// side not.
func TestTimelineCarriesTheLensBand(t *testing.T) {
	amPmMonthFirst(t)
	c := &cannedSearch{byMode: map[string]string{"-t": mayTransit, "-dm": maySky, "-tr": mayReturn}}
	req := mayRequest()
	req.Lens = Lens{Objects: recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 3, 4, 5, 6, 7, 8, 9, 10, 16, 22, 31}}}
	if _, err := TimelineWith(context.Background(), req, c.run, stubDefaults); err != nil {
		t.Fatal(err)
	}
	if len(c.argvs) != 3 {
		t.Fatalf("spawns = %v, want three searches", c.argvs)
	}
	for _, argv := range c.argvs {
		joined := " " + strings.Join(argv, " ") + " "
		if !strings.Contains(joined, " -R0 1 3 4 5 6 7 8 9 10 16 22 31") {
			t.Errorf("%v carries no Moon-less main band", argv)
		}
		if strings.Contains(joined, " -RT0 ") {
			t.Errorf("%v bands the transiting side, whose default already omits the Moon", argv)
		}
	}
}

// the specification rule 2: a lookup.TimelineRow carries its
// kind, its aspect and its second body as Astrolog named them, and the
// registry values those names resolve to; a name no registry holds leaves the
// row unresolved.
func TestTimelineRowsCarryTheirKind(t *testing.T) {
	amPmMonthFirst(t)
	sky := "Fri  5/ 1/2026  7:23pm     Sun (Tau) Opp (Sco) Moon (Full Moon)\n" +
		"Fri  5/ 1/2026  8:54pm    Moon (Gem) --> Cancer\n" +
		"Sat  5/ 2/2026  8:15am   Venus <Sco> S/R\n" +
		"Sun  5/ 3/2026  9:00am    Moon (Gem) Con [Gem] Ceres\n"
	c := &cannedSearch{byMode: map[string]string{"-t": mayTransit, "-dm": sky, "-tr": mayReturn}}
	res, err := TimelineWith(context.Background(), mayRequest(), c.run, stubDefaults)
	if err != nil {
		t.Fatal(err)
	}
	byLine := func(frag string) TimelineRow {
		t.Helper()
		for _, r := range res.Rows {
			if strings.Contains(r.Line, frag) {
				return r
			}
		}
		t.Fatalf("no row carries %q", frag)
		return TimelineRow{}
	}
	cases := []struct {
		frag     string
		kind     TimelineKind
		aspect   string
		other    string
		body     aspect.Body
		otherID  aspect.Body
		aspectID aspect.AspectKind
		resolved bool
	}{
		{"Sun (Tau) Opp (Sco) Moon", TimelineKindAspect, "Opp", "Moon", aspect.Sun, aspect.Moon, aspect.Opposition, true},
		{"trans     Sun (Tau) Squ natal (Leo) Ascendant", TimelineKindAspect, "Squ", "Ascendant", aspect.Sun, aspect.Ascendant, aspect.Square, true},
		{"Moon (Gem) --> Cancer", TimelineKindIngress, "", "", aspect.Moon, 0, 0, true},
		{"Venus <Sco> S/R", TimelineKindStation, "", "", aspect.Venus, 0, 0, true},
		{"Con [Gem] Ceres", TimelineKindAspect, "Con", "Ceres", aspect.Moon, 0, aspect.Conjunction, false},
	}
	for _, tc := range cases {
		r := byLine(tc.frag)
		if r.Kind != tc.kind || r.Aspect != tc.aspect || r.Other != tc.other || r.BodyID != tc.body ||
			r.OtherID != tc.otherID || r.AspectID != tc.aspectID || r.Resolved != tc.resolved {
			t.Errorf("%q: kind %d aspect %q other %q ids %d/%d/%d resolved %v, want %d %q %q %d/%d/%d %v", tc.frag,
				r.Kind, r.Aspect, r.Other, r.BodyID, r.OtherID, r.AspectID, r.Resolved,
				tc.kind, tc.aspect, tc.other, tc.body, tc.otherID, tc.aspectID, tc.resolved)
		}
	}
	if r := byLine("Moon (Gem) --> Cancer"); r.Sign != "Cancer" || r.Direction != "-->" {
		t.Errorf("ingress sign/direction = %q %q", r.Sign, r.Direction)
	}
	if r := byLine("Venus <Sco> S/R"); r.Station != "S/R" {
		t.Errorf("station = %q", r.Station)
	}
}

// C17 (the specification): Astrolog words the lunar nodes' stations
// the wrong way round against its own R column, so the node rows come back
// relabelled S/R <-> S/D in both the parsed Station and the row's text; every
// other body's row is verbatim (D39).
func TestNodeStationWordsFollowTheRColumn(t *testing.T) {
	amPmMonthFirst(t)
	sky := "Sat  5/ 2/2026  8:15am   Venus <Sco> S/R\n" +
		"Sat  5/ 2/2026  9:15am North N <Pis> S/R\n" +
		"Sat  5/ 2/2026 10:15am South N <Vir> S/D\n" +
		"Sat  5/ 2/2026 11:15am   Venus <Sco> S/D\n"
	c := &cannedSearch{byMode: map[string]string{"-t": mayTransit, "-dm": sky, "-tr": mayReturn}}
	res, err := TimelineWith(context.Background(), mayRequest(), c.run, stubDefaults)
	if err != nil {
		t.Fatal(err)
	}
	find := func(frag string) TimelineRow {
		t.Helper()
		for _, r := range res.Rows {
			if r.Kind == TimelineKindStation && strings.Contains(r.Line, frag) {
				return r
			}
		}
		t.Fatalf("no station row with %q in %d rows", frag, len(res.Rows))
		return TimelineRow{}
	}
	cases := []struct{ frag, station, word string }{
		{"North N <Pis>", "S/D", "S/D"},
		{"South N <Vir>", "S/R", "S/R"},
		{"Venus <Sco> S/R", "S/R", "S/R"},
		{"Venus <Sco> S/D", "S/D", "S/D"},
	}
	for _, tc := range cases {
		r := find(tc.frag)
		if r.Station != tc.station || !strings.HasSuffix(strings.TrimSpace(r.Line), tc.word) {
			t.Errorf("%s: Station %q, line %q; want %q in both", tc.frag, r.Station, r.Line, tc.station)
		}
	}
}
