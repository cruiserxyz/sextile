package lookup

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/store"
)

// The comparing timeline (the specification C10, C17, C17a):
// two anchored slots, ① born 1979-05-15 and ② born 1985-02-03, and the same
// floating May 2026 month the single-natal tests use. Lines are real May 2026
// Astrolog shapes; where two sources must share an instant the lines are
// re-timed to 5/ 7/2026 5:03am, and each such case says so.

const (
	lineT1   = " 5/ 1/2026 12:54pm trans Mercury (Ari) Con natal (Ari) Venus"
	lineT2   = " 5/ 3/2026  9:00am trans     Sun (Tau) Squ natal (Leo) Ascendant"
	lineBoth = " 5/ 7/2026  5:03am trans Mercury (Tau) Con natal (Tau) Mercury (Mercury Return)"
	lineSky  = "Thu  5/ 7/2026  5:03am    Moon (Sco) Squ (Aqu) Pluto"
	lineSky0 = "Fri  5/ 1/2026  7:50am    Moon (Sco) Squ (Aqu) Pluto"
	lineR1   = " 5/ 7/2026  5:03am trans     Sun (Tau) Con natal (Tau) Sun (Solar Return)"
	lineR2   = " 5/ 7/2026  5:03am trans    Mars (Ari) Con natal (Ari) Mars (Mars Return)"
	lineR2b  = " 5/ 6/2026 10:13am trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)"
)

func natalOne() store.AnchorChart {
	return store.AnchorChart{When: time.Date(1979, 5, 15, 12, 0, 0, 0, time.UTC), Place: timelinePlace()}
}

func natalTwo() store.AnchorChart {
	return store.AnchorChart{When: time.Date(1985, 2, 3, 12, 0, 0, 0, time.UTC), Place: timelinePlace()}
}

func comparingRequest() TimelineRequest {
	return TimelineRequest{
		When: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC), Place: timelinePlace(),
		Natals: []TimelineNatal{{Numeral: 1, Chart: natalOne()}, {Numeral: 2, Chart: natalTwo()}},
	}
}

// numeralSearch answers each (search, natal) pair with its canned output; a key
// in fail makes that search return an error. natal is "" for the sky.
type numeralSearch struct {
	out   map[string]string // "<mode>|<M D Y of the natal, or empty>"
	fail  map[string]error
	calls []string // keys, in spawn order
}

func (n *numeralSearch) run(_ context.Context, call BodyCall) (string, string, error) {
	argv := call.Argv()
	m := "-dm"
	for _, c := range []string{"-tr", "-t"} {
		if slices.Contains(argv, c) {
			m = c
		}
	}
	natal := ""
	if i := slices.Index(argv, "-qj"); i >= 0 {
		// The natal is a Julian Day (the specification C1); its key is the
		// civil date of that instant, which these post-reform fixtures name the
		// same in every calendar.
		jd, _ := strconv.ParseFloat(argv[i+1], 64)
		u := time.Unix(int64(math.Round((jd-2440587.5)*86400)), 0).UTC()
		natal = fmt.Sprintf("%d %d %d", int(u.Month()), u.Day(), u.Year())
	}
	key := m + "|" + natal
	n.calls = append(n.calls, key)
	command := "astrolog " + strings.Join(argv, " ")
	if err := n.fail[key]; err != nil {
		return "", command, err
	}
	return n.out[key], command, nil
}

const (
	k1t, k2t   = "-t|5 15 1979", "-t|2 3 1985"
	k1r, k2r   = "-tr|5 15 1979", "-tr|2 3 1985"
	kSky       = "-dm|"
	headerOnly = mayTransitHeader
)

func comparingSearch() *numeralSearch {
	return &numeralSearch{out: map[string]string{
		k1t:  mayTransitHeader + lineT1 + "\n" + lineBoth + "\n",
		k2t:  mayTransitHeader + lineT2 + "\n" + lineBoth + "\n",
		kSky: lineSky0 + "\n" + lineSky + "\n",
		k1r:  mayTransitHeader + lineBoth + "\n" + lineR1 + "\n",
		k2r:  mayTransitHeader + lineR2b + "\n" + lineBoth + "\n" + lineR2 + "\n",
	}}
}

type numbered struct {
	numeral int
	line    string
}

func numberedLines(rows []TimelineRow) []numbered {
	out := make([]numbered, len(rows))
	for i, r := range rows {
		out[i] = numbered{r.Numeral, r.Line}
	}
	return out
}

// Two natals interleave in instant order, each row carries its slot's numeral
// and the sky's is 0; at the shared instant the order is today's source order
// (transit, sky, return), then numeral, and a line both slots print at one
// instant is kept for BOTH while one slot's -t/-tr repeat is dropped (C10).
func TestTimelineNatalsMergeInOrderWithNumerals(t *testing.T) {
	amPmMonthFirst(t)
	c := comparingSearch()
	res, err := TimelineWith(context.Background(), comparingRequest(), c.run, stubDefaults)
	if err != nil {
		t.Fatal(err)
	}
	want := []numbered{
		{0, lineSky0},
		{1, lineT1},
		{2, lineT2},
		{2, lineR2b},
		{1, lineBoth},
		{2, lineBoth},
		{0, lineSky},
		{1, lineR1},
		{2, lineR2},
	}
	if got := numberedLines(res.Rows); !slices.Equal(got, want) {
		t.Fatalf("rows =\n%v\nwant\n%v", got, want)
	}
	// One sky search, and per natal its -t and -tr, in today's source order
	// across the set: every -t, the sky, every -tr.
	if wantCalls := []string{k1t, k2t, kSky, k1r, k2r}; !slices.Equal(c.calls, wantCalls) {
		t.Errorf("spawn order = %v, want %v", c.calls, wantCalls)
	}
	if len(res.Commands) != 5 || len(res.Failures) != 0 {
		t.Errorf("commands = %d, failures = %v", len(res.Commands), res.Failures)
	}
	// Each row keeps the command of the search that emitted it: slot 2's
	// repeated line is slot 2's -t, which spawned before its -tr.
	for _, r := range res.Rows {
		if r.Numeral == 2 && r.Line == lineBoth && mode(r.Command) != "-t" {
			t.Errorf("slot 2's kept repeat came from %q, want the -t that printed it first", mode(r.Command))
		}
	}
}

// One -t failing keeps the same slot's -tr, the other slot's rows and the
// sky; exactly one Failure names the slot and the search (C17).
func TestTimelineNatalsOneFailedSearchKeepsTheOthers(t *testing.T) {
	amPmMonthFirst(t)
	c := comparingSearch()
	boom := errors.New("astrolog: boom")
	c.fail = map[string]error{k1t: boom}
	res, err := TimelineWith(context.Background(), comparingRequest(), c.run, stubDefaults)
	if err != nil {
		t.Fatalf("one failed search failed the timeline: %v", err)
	}
	if len(res.Failures) != 1 || res.Failures[0].Numeral != 1 || res.Failures[0].Search != "-t" || !errors.Is(res.Failures[0].Err, boom) {
		t.Fatalf("failures = %+v, want one {1 -t boom}", res.Failures)
	}
	got := numberedLines(res.Rows)
	for _, w := range []numbered{{1, lineR1}, {1, lineBoth}, {2, lineT2}, {2, lineBoth}, {0, lineSky}, {0, lineSky0}} {
		if !slices.Contains(got, w) {
			t.Errorf("row %v lost to slot 1's failed -t: %v", w, got)
		}
	}
	for _, r := range got {
		if r == (numbered{1, lineT1}) {
			t.Errorf("a row of the failed search survived: %v", r)
		}
	}
	// The failed spawn's command is still listed.
	if len(res.Commands) != 5 {
		t.Errorf("commands = %d, want all five started spawns", len(res.Commands))
	}
}

// The sky failing keeps the transit and return rows, one Failure naming -dm
// with numeral 0 (C17a).
func TestTimelineNatalsFailedSkyKeepsTheTransits(t *testing.T) {
	amPmMonthFirst(t)
	c := comparingSearch()
	c.fail = map[string]error{kSky: errors.New("astrolog: no sky")}
	res, err := TimelineWith(context.Background(), comparingRequest(), c.run, stubDefaults)
	if err != nil {
		t.Fatalf("a failed sky search failed the timeline: %v", err)
	}
	if len(res.Failures) != 1 || res.Failures[0].Numeral != 0 || res.Failures[0].Search != "-dm" {
		t.Fatalf("failures = %+v, want one {0 -dm}", res.Failures)
	}
	if len(res.Rows) == 0 {
		t.Fatal("no transit rows survived the sky failure")
	}
	for _, r := range res.Rows {
		if r.Numeral == 0 {
			t.Errorf("a sky row survived its failed search: %q", r.Line)
		}
	}
}

// When every search fails there is nothing to show: the call fails, and the
// failures and the spawned commands still ride in the result.
func TestTimelineNatalsAllSearchesFailing(t *testing.T) {
	c := comparingSearch()
	boom := errors.New("astrolog: down")
	c.fail = map[string]error{k1t: boom, k2t: boom, kSky: boom, k1r: boom, k2r: boom}
	res, err := TimelineWith(context.Background(), comparingRequest(), c.run, stubDefaults)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the first failure", err)
	}
	if len(res.Failures) != 5 || len(res.Commands) != 5 || len(res.Rows) != 0 {
		t.Errorf("failures = %d, commands = %d, rows = %d", len(res.Failures), len(res.Commands), len(res.Rows))
	}
}

// A cancelled context is the call's error, never a Failure: a superseded run
// must drop silently.
func TestTimelineNatalsCancellationIsNotAFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	search := func(_ context.Context, call BodyCall) (string, string, error) {
		cancel()
		return "", "", context.Canceled
	}
	res, err := TimelineWith(ctx, comparingRequest(), search, stubDefaults)
	if !errors.Is(err, context.Canceled) || len(res.Failures) != 0 {
		t.Errorf("err = %v, failures = %v", err, res.Failures)
	}
}

// The default-set refusal still refuses the whole timeline before any search.
func TestTimelineNatalsRefusedWithoutDefaultSetsSpawnsNothing(t *testing.T) {
	c := comparingSearch()
	broken := func(context.Context) (DefaultSets, error) { return DefaultSets{}, errors.New("unreadable") }
	_, err := TimelineWith(context.Background(), comparingRequest(), c.run, broken)
	if !DefaultSetsRefused(err) || len(c.calls) != 0 {
		t.Errorf("err = %v, spawns = %v", err, c.calls)
	}
}

func TestTimelineNatalsRefusesBadNumerals(t *testing.T) {
	for name, natals := range map[string][]TimelineNatal{
		"zero numeral":      {{Numeral: 0, Chart: natalOne()}},
		"duplicate numeral": {{Numeral: 1, Chart: natalOne()}, {Numeral: 1, Chart: natalTwo()}},
		"zero chart":        {{Numeral: 1}},
	} {
		c := comparingSearch()
		req := comparingRequest()
		req.Natals = natals
		if _, err := TimelineWith(context.Background(), req, c.run, stubDefaults); err == nil || len(c.calls) != 0 {
			t.Errorf("%s: err = %v, spawns = %v", name, err, c.calls)
		}
	}
}

// Without Natals nothing changes: every row's numeral is 0.
func TestTimelineWithoutNatalsCarriesNoNumerals(t *testing.T) {
	amPmMonthFirst(t)
	c := &cannedSearch{byMode: map[string]string{"-t": mayTransit, "-dm": maySky, "-tr": mayReturn}}
	res, err := TimelineWith(context.Background(), mayRequest(), c.run, stubDefaults)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res.Rows {
		if r.Numeral != 0 {
			t.Errorf("row %q carries numeral %d", r.Line, r.Numeral)
		}
	}
	if res.Failures != nil {
		t.Errorf("failures = %v", res.Failures)
	}
}

// Equal-instant rows of one source are ordered by NUMERAL, not by the order
// the caller listed the natals in: slot 2 listed first still prints after
// slot 1.
func TestTimelineNatalsTieBreakIsTheNumeralNotTheListOrder(t *testing.T) {
	amPmMonthFirst(t)
	c := comparingSearch()
	req := comparingRequest()
	req.Natals = []TimelineNatal{{Numeral: 2, Chart: natalTwo()}, {Numeral: 1, Chart: natalOne()}}
	res, err := TimelineWith(context.Background(), req, c.run, stubDefaults)
	if err != nil {
		t.Fatal(err)
	}
	var both []int
	for _, r := range res.Rows {
		if r.Line == lineBoth {
			both = append(both, r.Numeral)
		}
	}
	if !slices.Equal(both, []int{1, 2}) {
		t.Errorf("equal-instant rows came in numeral order %v, want [1 2]", both)
	}
}

// C17 in a DST month (review round 1): October 2026 in Berlin is two offset
// segments, each its own spawn. When the second segment's -t fails, the first
// segment's -t rows are still shown, the other searches' rows too, and the
// failure is one Failure for the one failed call.
func TestTimelineNatalsDSTSegmentFailureKeepsTheOtherSegment(t *testing.T) {
	amPmMonthFirst(t)
	head := "Astrolog 8.00 chart for Tue May 15, 1979  9:30am (ST Zone 0W)   4:54E 52:22N\n\nTransits during Oct 2026 (ST Zone 2E)\n"
	berlin := timelinePlace()
	berlin.Name, berlin.TZ = "Berlin", "Europe/Berlin"
	req := TimelineRequest{
		When: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC), Place: berlin,
		Natals: []TimelineNatal{{Numeral: 1, Chart: natalOne()}},
	}
	var calls []string
	search := func(_ context.Context, call BodyCall) (string, string, error) {
		argv := call.Argv()
		command := "astrolog " + strings.Join(argv, " ")
		calls = append(calls, command)
		switch {
		case slices.Contains(argv, "-tr"):
			return head, command, nil
		case slices.Contains(argv, "-t"):
			if argv[1] == "-1" { // the CET segment
				return "", command, errors.New("astrolog exec: exit status 1")
			}
			return head + "10/ 2/2026  6:08am trans Jupiter (Leo) Tri natal [Sag] Neptune\n", command, nil
		}
		return "Thu 10/ 1/2026  4:43am    Moon (Gem) Con [Gem] Uranus\n", command, nil
	}
	res, err := TimelineWith(context.Background(), req, search, stubDefaults)
	if err != nil {
		t.Fatalf("one failed call of several failed the timeline: %v", err)
	}
	if len(calls) != 6 {
		t.Fatalf("%d spawns, want six (three searches, two segments): %v", len(calls), calls)
	}
	got := lines(res.Rows)
	if len(got) != 2 || !strings.Contains(got[0], "Moon (Gem)") || !strings.Contains(got[1], "Jupiter (Leo)") {
		t.Fatalf("rows = %q, want the sky row and the CEST segment's -t row", got)
	}
	if len(res.Failures) != 1 || res.Failures[0].Numeral != 1 || res.Failures[0].Search != "-t" {
		t.Errorf("failures = %+v, want exactly one, numeral 1, search -t", res.Failures)
	}
	if len(res.Commands) != 6 {
		t.Errorf("commands = %d, want all six spawns", len(res.Commands))
	}
}
