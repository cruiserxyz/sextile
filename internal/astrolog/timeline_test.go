package astrolog

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Every line in this file was pasted from a live Astrolog 8.00 run on
// 2026-09-30 at `-z -1` (UTC+1) unless its case says otherwise — never
// assembled by hand, which is how the spec's own evidence block once came to
// pin times Astrolog does not emit.

var utcPlusOne = time.FixedZone("UTC+1", 3600)

func at(zone *time.Location, y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, zone)
}

// timelineCorpus is C54's fixture corpus: one real line of every row kind the
// spec's § What the three grammars look like inventories, plus the shapes the
// 2026 census found beside them.
var timelineCorpus = []struct {
	name string
	line string
	want time.Time
}{
	{"-t transit row", "10/ 2/2026  5:08am trans Jupiter (Leo) Tri natal [Sag] Neptune", at(utcPlusOne, 2026, 10, 2, 5, 8)},
	{"-t row with a padded month and 12pm", " 1/ 1/2026 12:52pm trans Mercury (Sag) Tri natal (Ari) Mars", at(utcPlusOne, 2026, 1, 1, 12, 52)},
	{"-tr return row with its label", " 5/ 6/2026  9:13am trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)", at(utcPlusOne, 2026, 5, 6, 9, 13)},
	{"-t row carrying a return label", " 5/14/2026  7:20pm trans     Sun (Tau) Con natal (Tau) Sun (Solar Return)", at(utcPlusOne, 2026, 5, 14, 19, 20)},
	{"-dm aspect row with its weekday", "Thu 10/ 1/2026  3:43am    Moon (Gem) Con [Gem] Uranus", at(utcPlusOne, 2026, 10, 1, 3, 43)},
	{"-dm --> ingress", "Fri 10/ 2/2026  8:54pm    Moon (Gem) --> Cancer", at(utcPlusOne, 2026, 10, 2, 20, 54)},
	{"-dm <-- retrograde ingress", "Sun 10/25/2026 10:09am   Venus [Sco] <-- Libra", at(utcPlusOne, 2026, 10, 25, 10, 9)},
	{"-dm S/R station in <> brackets", "Sat 10/ 3/2026  8:15am   Venus <Sco> S/R", at(utcPlusOne, 2026, 10, 3, 8, 15)},
	{"-dm trailing phenomenon label", "Sat 10/10/2026  4:50pm     Sun (Lib) Con (Lib) Moon (New Moon)", at(utcPlusOne, 2026, 10, 10, 16, 50)},
	{"-dm two stacked labels", "Tue  3/ 3/2026 12:38pm     Sun (Pis) Opp (Vir) Moon (Full Moon) (Total Lunar Eclipse)", at(utcPlusOne, 2026, 3, 3, 12, 38)},
	{"-dm trailing (v/c HH:MM)", "Fri 10/ 2/2026  3:41am    Moon (Gem) Sex (Leo) Jupiter (v/c 17:12)", at(utcPlusOne, 2026, 10, 2, 3, 41)},
	{"-dm minor aspect BQn", "Thu 10/ 1/2026  7:08am    Moon (Gem) BQn (Sco) Mercury", at(utcPlusOne, 2026, 10, 1, 7, 8)},
	{"-dm minor aspect SSx", "Thu 10/ 1/2026  9:15pm    Moon (Gem) SSx (Can) Venus", at(utcPlusOne, 2026, 10, 1, 21, 15)},
	{"-t minor aspect SSq", "10/ 3/2026  4:20am trans    Mars (Lib) SSq natal (Sag) Neptune", at(utcPlusOne, 2026, 10, 3, 4, 20)},
	{"-dm 12am with a (v/c) whose clock is later", "Sun 10/11/2026 12:07am    Moon (Lib) Sex (Leo) Jupiter (v/c 16:13)", at(utcPlusOne, 2026, 10, 11, 0, 7)},
	{"-dm two-word body", "Tue  8/18/2026  9:34pm North N [Pis] <-- Aquarius", at(utcPlusOne, 2026, 8, 18, 21, 34)},
}

// C54: table-driven over the corpus, asserting the instant and the
// byte-identical line for every case, each parsed on its own.
func TestParseTimelineReadsEveryMeasuredGrammar(t *testing.T) {
	// These pin Astrolog's am/pm month-first text; the held clock says so
	// (the specification C3: the grammar is the preference's).
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	kinds := map[string]bool{}
	for _, tc := range timelineCorpus {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := ParseTimeline(tc.line+"\n", utcPlusOne)
			if err != nil {
				t.Fatalf("ParseTimeline refused a measured line: %v", err)
			}
			if len(rows) != 1 {
				t.Fatalf("rows = %d, want 1", len(rows))
			}
			if rows[0].Line != tc.line {
				t.Errorf("line = %q, want it byte-identical: %q", rows[0].Line, tc.line)
			}
			if !rows[0].Instant.Equal(tc.want) {
				t.Errorf("instant = %v, want %v", rows[0].Instant, tc.want)
			}
		})
		for _, marker := range []string{" trans ", "Return)", "-->", "S/R", "(New Moon)", "(v/c ", "(Leo)", "[Gem]", "<Sco>"} {
			if strings.Contains(tc.line, marker) {
				kinds[marker] = true
			}
		}
	}
	// The corpus itself is part of the contract: a case deleted later must
	// fail here rather than quietly shrink what the parser is proven against.
	// Each search is its own requirement — a `-t` row carrying a return label
	// does not stand in for the `-tr` case (review round 3).
	for _, src := range []struct{ prefix, marker string }{{"-t ", " trans "}, {"-tr ", "Return)"}, {"-dm ", "/2026"}} {
		found := false
		for _, tc := range timelineCorpus {
			if strings.HasPrefix(tc.name, src.prefix) && strings.Contains(tc.line, src.marker) {
				found = true
			}
		}
		if !found {
			t.Errorf("the corpus has no %scase carrying %q", src.prefix, src.marker)
		}
	}
	for _, marker := range []string{" trans ", "Return)", "-->", "S/R", "(New Moon)", "(v/c ", "(Leo)", "[Gem]", "<Sco>"} {
		if !kinds[marker] {
			t.Errorf("the corpus carries no line with %q; C54 requires one of every measured kind", marker)
		}
	}
}

// C1: the entry point returns each event's instant and its line verbatim —
// including the leading space of a `-t` row and the variable padding before
// a body name, neither of which may be trimmed.
func TestParseTimelineReturnsInstantAndVerbatimLine(t *testing.T) {
	// These pin Astrolog's am/pm month-first text; the held clock says so
	// (the specification C3: the grammar is the preference's).
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	out := "Astrolog 8.00 chart for Tue May 15, 1979  9:30am (ST Zone 0W)   4:54E 52:22N\n" +
		"\n" +
		"Transits during May 2026 (ST Zone 1E)\n" +
		" 5/ 6/2026  9:13am trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)\n" +
		" 5/ 7/2026  4:03am trans Mercury (Tau) Con natal (Tau) Mercury (Mercury Return)\n"
	rows, err := ParseTimeline(out, utcPlusOne)
	if err != nil {
		t.Fatal(err)
	}
	want := []TimelineRow{
		{Instant: at(utcPlusOne, 2026, 5, 6, 9, 13), Line: " 5/ 6/2026  9:13am trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)"},
		{Instant: at(utcPlusOne, 2026, 5, 7, 4, 3), Line: " 5/ 7/2026  4:03am trans Mercury (Tau) Con natal (Tau) Mercury (Mercury Return)"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v, want %+v", rows, want)
	}
	for i := range want {
		if rows[i].Line != want[i].Line || !rows[i].Instant.Equal(want[i].Instant) {
			t.Errorf("row %d = {%v %q}, want {%v %q}", i, rows[i].Instant, rows[i].Line, want[i].Instant, want[i].Line)
		}
	}
}

// C1a: the text carries only a local clock, so the same output under two
// zones is two different instants — three hours apart for UTC+1 and UTC-2.
func TestParseTimelineResolvesInstantsAgainstTheGivenZone(t *testing.T) {
	// These pin Astrolog's am/pm month-first text; the held clock says so
	// (the specification C3: the grammar is the preference's).
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	out := "Thu 10/ 1/2026  3:43am    Moon (Gem) Con [Gem] Uranus\n"
	east, err := ParseTimeline(out, utcPlusOne)
	if err != nil {
		t.Fatal(err)
	}
	west, err := ParseTimeline(out, time.FixedZone("UTC-2", -7200))
	if err != nil {
		t.Fatal(err)
	}
	if got := west[0].Instant.Sub(east[0].Instant); got != 3*time.Hour {
		t.Errorf("UTC-2 minus UTC+1 = %v, want 3h: the zone was not applied", got)
	}
	if !east[0].Instant.Equal(time.Date(2026, 10, 1, 2, 43, 0, 0, time.UTC)) {
		t.Errorf("UTC+1 instant = %v, want 02:43Z", east[0].Instant.UTC())
	}
}

// C2: one unreadable line anywhere refuses the whole output, names the line,
// and returns no rows — never a list with a row silently missing.
func TestParseTimelineRefusesAnUnreadableLine(t *testing.T) {
	// These pin Astrolog's am/pm month-first text; the held clock says so
	// (the specification C3: the grammar is the preference's).
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	for _, junk := range []string{
		"Astrolog: Value 13 passed to parameter #1 of switch -t out of range.",
		// Near misses of the two measured "no events" notices stay refused: only
		// the exact lines are an empty search (TestParseTimelineReadsTheNoEventsNotices).
		"No transits found",
		"No transits found. Try again.",
		"   Moon (Gem) Con [Gem] Uranus",
	} {
		out := "Thu 10/ 1/2026  3:43am    Moon (Gem) Con [Gem] Uranus\n" + junk + "\n" +
			"Fri 10/ 2/2026  8:54pm    Moon (Gem) --> Cancer\n"
		rows, err := ParseTimeline(out, utcPlusOne)
		if err == nil {
			t.Errorf("accepted an output containing %q: %d rows", junk, len(rows))
			continue
		}
		if rows != nil {
			t.Errorf("refusal of %q still returned %d rows", junk, len(rows))
		}
		if !strings.Contains(err.Error(), junk) {
			t.Errorf("error %q does not name the refused line %q", err, junk)
		}
	}
}

// C55: a line that looks dated but is not readable is refused, not read at
// the wrong clock — the likeliest misreading being a (v/c HH:MM) trailer
// taken for the event time when the event time is missing.
func TestParseTimelineRefusesADatedButUnreadableLine(t *testing.T) {
	// These pin Astrolog's am/pm month-first text; the held clock says so
	// (the specification C3: the grammar is the preference's).
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	for _, line := range []string{
		"Fri 10/ 2/2026    Moon (Gem) Sex (Leo) Jupiter (v/c 17:12)", // event time missing
		"Fri 10/ 2/2026 13:41am    Moon (Gem) Sex (Leo) Jupiter",     // no 13 o'clock on a 12-hour clock
		"Fri 10/ 2/2026  3:61am    Moon (Gem) Sex (Leo) Jupiter",     // no 61st minute
		"Fri 10/ 2/2026  3:41      Moon (Gem) Sex (Leo) Jupiter",     // no am/pm
		"Mon  2/30/2026  3:41am    Moon (Gem) Sex (Leo) Jupiter",     // no 30 February
		" 2/30/2026  3:41am trans Jupiter (Leo) Tri natal [Sag] Neptune",
		"Sat 10/ 2/2026  3:41am    Moon (Gem) Sex (Leo) Jupiter",   // 2 October 2026 is a Friday
		"10/ 2/2026  5:08am Jupiter (Leo) Tri natal [Sag] Neptune", // -t row without `trans`
		"10/ 2/2026  5:08am trans",                                 // no event at all
		// Review round 1: a line cut short, or reshaped, after a correct
		// date and time. A prefix match accepted every one of these.
		"10/ 2/2026  5:08am trans Jupiter (Leo)",
		"10/ 2/2026  5:08am trans Jupiter (Leo) Tri natal [Sag]",
		"10/ 2/2026  5:08am trans Jupiter (Leo) Tri [Sag] Neptune",
		"Thu 10/ 1/2026  3:43am    Moon (Gem) Con [Gem]",
		"Thu 10/ 1/2026  3:43am    Moon Con Uranus",
		"Fri 10/ 2/2026  3:41am    Moon (Gem) Sex (Leo) Jupiter (v/c 17:",
		"Fri 10/ 2/2026  3:41am    Moon (Gem) Sex (Leo) Jupiter (v/c 17:12) trailing",
		"Sat 10/ 3/2026  8:15am   Venus <Sco> S/X",
		// Review round 2: a sign whose brackets do not pair.
		"Thu 10/ 1/2026  3:43am    Moon (Gem] Con [Gem] Uranus",
		"10/ 2/2026  5:08am trans Jupiter <Leo) Tri natal [Sag] Neptune",
		"Fri 10/ 2/2026  8:54pm    Moon (Gem) --> ",
	} {
		rows, err := ParseTimeline(line+"\n", utcPlusOne)
		if err == nil {
			t.Errorf("accepted %q as an event at %v", line, rows[0].Instant)
		}
	}
}

// C3: an output carrying only the headers and blank lines is zero rows and
// no error. Not reachable from the real binary (spec § Failure modes); the
// fixture is synthetic, which is right for a parse test.
func TestParseTimelineAcceptsAnOutputWithNoEvents(t *testing.T) {
	out := "Astrolog 8.00 chart for Tue May 15, 1979  9:30am (ST Zone 0W)   4:54E 52:22N\n" +
		"\n" +
		"Transits during Oct 2026 (ST Zone 1E)\n\n"
	rows, err := ParseTimeline(out, utcPlusOne)
	if err != nil {
		t.Fatalf("refused an empty search: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none", rows)
	}
	if rows, err := ParseTimeline("", utcPlusOne); err != nil || len(rows) != 0 {
		t.Fatalf("empty -dm output: rows=%v err=%v, want none and no error", rows, err)
	}
}

// A search whose band hides every body that month would have named is an
// ordinary empty list. Astrolog prints a notice instead of silence — measured on
// 8.00 (2026-10-03): `-t 10 2026 -R0` and the Moon-less `-tr 10 2026 -R0 1 3`
// print the first, `-qm 10 2026 -dm -R0` the second. Both read as zero rows. Before
// the choosers reached the searches this was unreachable, and the first of them
// was pinned as a refused line.
func TestParseTimelineReadsTheNoEventsNotices(t *testing.T) {
	for name, out := range map[string]string{
		"-t / -tr": "Astrolog 8.00 chart for Tue Apr 15, 1980 12:00pm (ST Zone 0W)   4:54E 52:22N\n\nTransits during Oct 2026 (ST Zone 2E)\nNo transits found.\n",
		"-dm":      "No transit events found.\n",
	} {
		rows, err := ParseTimeline(out, utcPlusOne)
		if err != nil || len(rows) != 0 {
			t.Errorf("%s: rows=%v err=%v, want an empty list and no error", name, rows, err)
		}
	}
}

// C4: the two headers and blank lines are recognised, excluded and not
// refused, and the rows are the same with them absent — which is the -dm
// case, whose output has no headers at all.
func TestParseTimelineDropsTheSearchHeader(t *testing.T) {
	// These pin Astrolog's am/pm month-first text; the held clock says so
	// (the specification C3: the grammar is the preference's).
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	rowsOnly := "10/ 2/2026  5:08am trans Jupiter (Leo) Tri natal [Sag] Neptune\n" +
		"10/ 2/2026  2:03pm trans Mercury (Sco) Squ natal (Leo) Jupiter\n"
	withHeaders := "Astrolog 8.00 chart for Tue May 15, 1979  9:30am (ST Zone 0W)   4:54E 52:22N\n" +
		"\n" +
		"Transits during Oct 2026 (ST Zone 1E)\n" + rowsOnly
	a, err := ParseTimeline(withHeaders, utcPlusOne)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseTimeline(rowsOnly, utcPlusOne)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 2 || !slices.EqualFunc(a, b, func(x, y TimelineRow) bool { return x.Line == y.Line && x.Instant.Equal(y.Instant) }) {
		t.Fatalf("with headers %+v, without %+v; want the same two rows", a, b)
	}
	for _, r := range a {
		if strings.HasPrefix(r.Line, "Astrolog ") || strings.HasPrefix(r.Line, "Transits during ") {
			t.Errorf("a header became a row: %q", r.Line)
		}
	}
}

func tlAmsterdam(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatalf("load Europe/Amsterdam: %v", err)
	}
	return loc
}

// C45: October 2026 in Amsterdam is two segments whose boundary is the
// INSTANT of the fall-back — 01:00Z on 25 October, 03:00 CEST — not a date.
// A month with no transition is one segment.
func TestTimelineSegmentsAMonthByUTCOffsetInstants(t *testing.T) {
	ams := tlAmsterdam(t)
	segs := TimelineSegments(2026, time.October, ams)
	want := []TimelineSegment{
		{Start: time.Date(2026, 9, 30, 22, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 25, 1, 0, 0, 0, time.UTC), Offset: 7200},
		{Start: time.Date(2026, 10, 25, 1, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC), Offset: 3600},
	}
	if len(segs) != len(want) {
		t.Fatalf("segments = %+v, want %+v", segs, want)
	}
	for i := range want {
		if !segs[i].Start.Equal(want[i].Start) || !segs[i].End.Equal(want[i].End) || segs[i].Offset != want[i].Offset {
			t.Errorf("segment %d = [%v, %v) %+d, want [%v, %v) %+d", i,
				segs[i].Start.UTC(), segs[i].End.UTC(), segs[i].Offset, want[i].Start, want[i].End, want[i].Offset)
		}
	}
	// The boundary falls inside the date: the 25th belongs to both segments.
	if segs[0].End.In(ams).Day() != 25 || segs[1].Start.In(ams).Day() != 25 {
		t.Errorf("the boundary is not inside 25 October: %v / %v", segs[0].End.In(ams), segs[1].Start.In(ams))
	}

	// Spring forward is the same rule in the other direction.
	march := TimelineSegments(2026, time.March, ams)
	if len(march) != 2 || !march[0].End.Equal(time.Date(2026, 3, 29, 1, 0, 0, 0, time.UTC)) || march[1].Offset != 7200 {
		t.Errorf("March 2026 = %+v, want a split at 01:00Z on the 29th", march)
	}

	for _, month := range []time.Month{time.January, time.July, time.November} {
		if segs := TimelineSegments(2026, month, ams); len(segs) != 1 {
			t.Errorf("%v 2026 has no transition but made %d segments", month, len(segs))
		}
	}
	if segs := TimelineSegments(2026, time.October, time.UTC); len(segs) != 1 || segs[0].Offset != 0 {
		t.Errorf("UTC October = %+v, want one segment at offset 0", segs)
	}
}

// fakeSearch is a TimelineSearcher returning canned output keyed on the
// argv's `-z` value, recording every argv it was handed.
type fakeSearch struct {
	byZone map[string]string
	argvs  [][]string
}

func (f *fakeSearch) run(_ context.Context, call BodyCall) (string, string, error) {
	argv := call.Argv()
	f.argvs = append(f.argvs, slices.Clone(argv))
	return f.byZone[argv[1]], "astrolog " + strings.Join(argv, " "), nil
}

// C46: each search runs once per segment at that segment's `-z`, and a row
// is kept iff its printed time, read at its segment's offset, lands in that
// segment. Each canned output is the whole month at its own offset — the
// same four events in both — so every event arrives twice and exactly one
// copy must survive.
func TestTimelineRunsEachSearchPerSegmentAndKeepsOnlyItsInstants(t *testing.T) {
	// These pin Astrolog's am/pm month-first text; the held clock says so
	// (the specification C3: the grammar is the preference's).
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	ams := tlAmsterdam(t)
	fake := &fakeSearch{byZone: map[string]string{
		"-2": "Thu 10/ 1/2026  4:43am    Moon (Gem) Con [Gem] Uranus\n" +
			"Sun 10/25/2026 12:05am    Moon (Ari) Tri (Leo) Mars\n" +
			"Sun 10/25/2026 11:09am   Venus [Sco] <-- Libra\n" +
			"Sun 11/ 1/2026 12:59am    Moon (Can) Squ [Lib] Venus\n",
		"-1": "Wed 9/30/2026 11:05pm    Moon (Ari) Tri (Leo) Mars\n" +
			"Thu 10/ 1/2026  3:43am    Moon (Gem) Con [Gem] Uranus\n" +
			"Sat 10/24/2026 11:05pm    Moon (Ari) Tri (Leo) Mars\n" +
			"Sun 10/25/2026 10:09am   Venus [Sco] <-- Libra\n" +
			"Sat 10/31/2026 10:59pm    Moon (Can) Squ [Lib] Venus\n",
	}}
	// The -1 output's first line is out of the month on purpose, and the -2
	// output's last line too: Astrolog never prints them, but a retention by
	// printed date rather than by instant is exactly what they would expose.
	fake.byZone["-1"] = strings.Replace(fake.byZone["-1"], "Wed 9/30", "Wed  9/30", 1)
	rows, commands, err := ComputeTimeline(context.Background(),
		TimelineRequest{Year: 2026, Month: time.October, Zone: ams}, []TimelineSource{TimelineSky}, fake.run)
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.argvs) != 2 || fake.argvs[0][1] != "-2" || fake.argvs[1][1] != "-1" {
		t.Fatalf("argvs = %v, want one -z -2 run then one -z -1 run", fake.argvs)
	}
	if len(commands) != 2 {
		t.Errorf("commands = %v, want one per spawn", commands)
	}
	want := []struct {
		line string
		utc  time.Time
		zone string
	}{
		{"Thu 10/ 1/2026  4:43am    Moon (Gem) Con [Gem] Uranus", time.Date(2026, 10, 1, 2, 43, 0, 0, time.UTC), "-2"},
		{"Sun 10/25/2026 12:05am    Moon (Ari) Tri (Leo) Mars", time.Date(2026, 10, 24, 22, 5, 0, 0, time.UTC), "-2"},
		{"Sun 10/25/2026 10:09am   Venus [Sco] <-- Libra", time.Date(2026, 10, 25, 9, 9, 0, 0, time.UTC), "-1"},
		{"Sat 10/31/2026 10:59pm    Moon (Can) Squ [Lib] Venus", time.Date(2026, 10, 31, 21, 59, 0, 0, time.UTC), "-1"},
	}
	if len(rows) != len(want) {
		t.Fatalf("kept %d rows, want %d:\n%s", len(rows), len(want), dumpRows(rows))
	}
	for i, w := range want {
		if rows[i].Line != w.line || !rows[i].Instant.Equal(w.utc) {
			t.Errorf("row %d = %q at %v, want %q at %v", i, rows[i].Line, rows[i].Instant.UTC(), w.line, w.utc)
		}
		if !strings.Contains(rows[i].Command, "-z "+w.zone+" ") {
			t.Errorf("row %d came from %q, want the -z %s invocation", i, rows[i].Command, w.zone)
		}
	}
}

func dumpRows(rows []TimelineRow) string {
	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, "  %v  %q\n", r.Instant.UTC(), r.Line)
	}
	return b.String()
}

// C47 against the real binary, October 2026 in Amsterdam: every row's
// printed local time is the Amsterdam wall clock at its own instant — on the
// transition date itself as well as either side of it.
func TestTimelineIsCorrectAcrossADSTTransition(t *testing.T) {
	// These pin Astrolog's am/pm month-first text; the held clock says so
	// (the specification C3: the grammar is the preference's).
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	ams := tlAmsterdam(t)
	req := TimelineRequest{Year: 2026, Month: time.October, Zone: ams,
		Natal: &TimelineNatal{When: time.Date(1979, 5, 15, 9, 30, 0, 0, time.UTC), Lon: 4.9, Lat: 52.3667}}
	rows, _, err := ComputeTimeline(context.Background(), req,
		[]TimelineSource{TimelineTransit, TimelineSky, TimelineReturn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The two anchors the goal statement reproduces: correct Amsterdam times
	// that no single -z can print together.
	mustHave := map[string]time.Time{
		"Thu 10/ 1/2026  4:43am    Moon (Gem) Con [Gem] Uranus": time.Date(2026, 10, 1, 2, 43, 0, 0, time.UTC),
		"Sat 10/31/2026 10:59pm    Moon (Can) Squ [Lib] Venus":  time.Date(2026, 10, 31, 21, 59, 0, 0, time.UTC),
		// On the transition date, one either side of 03:00 CEST.
		"Sun 10/25/2026 12:05am    Moon (Ari) Tri (Leo) Mars": time.Date(2026, 10, 24, 22, 5, 0, 0, time.UTC),
		"Sun 10/25/2026 10:09am   Venus [Sco] <-- Libra":      time.Date(2026, 10, 25, 9, 9, 0, 0, time.UTC),
	}
	onThe25th := 0
	for _, r := range rows {
		if want, ok := mustHave[r.Line]; ok {
			if !r.Instant.Equal(want) {
				t.Errorf("%q at %v, want %v", r.Line, r.Instant.UTC(), want)
			}
			delete(mustHave, r.Line)
		}
		local := r.Instant.In(ams)
		if printed := astrologClock(local); !strings.Contains(r.Line, printed) {
			t.Errorf("%q resolved to %v; the Amsterdam wall clock there prints as %q", r.Line, local, printed)
		}
		if local.Day() == 25 {
			onThe25th++
		}
	}
	for line := range mustHave {
		t.Errorf("missing row %q", line)
	}
	if onThe25th == 0 {
		t.Error("no row on 25 October: the fixture month cannot see a date-grained segmentation")
	}
	if dup := duplicateInstantLines(rows); dup != "" {
		t.Errorf("a row survived from both segments: %s", dup)
	}

	// The repeated hour: 02:30 happens twice on 25 October. Astrolog printed
	// nothing inside it this month, so the case is canned — each copy read at
	// its own segment's offset lands in its own segment, and both survive
	// with the absolute instants a local clock alone cannot tell apart.
	fake := &fakeSearch{byZone: map[string]string{
		"-2": "Sun 10/25/2026  2:30am    Moon (Ari) Tri (Leo) Mars\n",
		"-1": "Sun 10/25/2026  2:30am    Moon (Ari) Tri (Leo) Mars\n",
	}}
	twice, _, err := ComputeTimeline(context.Background(),
		TimelineRequest{Year: 2026, Month: time.October, Zone: ams}, []TimelineSource{TimelineSky}, fake.run)
	if err != nil {
		t.Fatal(err)
	}
	if len(twice) != 2 ||
		!twice[0].Instant.Equal(time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)) ||
		!twice[1].Instant.Equal(time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC)) {
		t.Errorf("repeated hour = %s, want 00:30Z (CEST) and 01:30Z (CET)", dumpRows(twice))
	}
}

// astrologClock prints t the way Astrolog prints a row's date and time:
// space-padded month, day and 12-hour clock.
func astrologClock(t time.Time) string {
	h, ampm := t.Hour()%12, "am"
	if h == 0 {
		h = 12
	}
	if t.Hour() >= 12 {
		ampm = "pm"
	}
	return fmt.Sprintf("%2d/%2d/%04d %2d:%02d%s", int(t.Month()), t.Day(), t.Year(), h, t.Minute(), ampm)
}

func duplicateInstantLines(rows []TimelineRow) string {
	seen := map[string]bool{}
	for _, r := range rows {
		k := r.Instant.UTC().String() + "|" + r.Line
		if seen[k] && r.Source == TimelineSky {
			return k
		}
		seen[k] = true
	}
	return ""
}

var liveNatal = &TimelineNatal{When: time.Date(1979, 5, 15, 9, 30, 0, 0, time.UTC), Lon: 4.9, Lat: 52.3667}

// C24: with a chart anchored, the three searches run for the floating month
// and year, with the anchored chart as the natal input to -t and -tr, once
// per search per segment — and -dm takes no natal.
func TestComputeTimelineTargetsTheFloatingMonthAndTheAnchoredChart(t *testing.T) {
	// These pin Astrolog's am/pm month-first text; the held clock says so
	// (the specification C3: the grammar is the preference's).
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	fake := &fakeSearch{byZone: map[string]string{}}
	req := TimelineRequest{Year: 2026, Month: time.May, Zone: time.FixedZone("UTC+1", 3600), Natal: liveNatal}
	if _, _, err := ComputeTimeline(context.Background(), req,
		[]TimelineSource{TimelineTransit, TimelineSky, TimelineReturn}, fake.run); err != nil {
		t.Fatal(err)
	}
	natal := []string{"-qj", "2444008.895833334", "-zl", "4:54E", "52:22N"}
	// A zero-value request carries the zero-value frame: Placidus, tropical
	// (the specification C5).
	frame := []string{"-c", "0", "_s", "_Ys", "_sr0", ":sz", "-qag", "10", "15", "1582", "12:00am", "0", "0:00E", "0:00N", "_Yt", "_Yd"} // the frame band, the reform pin, then the held clock's pins
	want := [][]string{
		append(append(append([]string{"-z", "-1"}, natal...), "-t", "5", "2026"), frame...),
		append([]string{"-z", "-1", "-qm", "5", "2026", "-dm"}, frame...),
		append(append(append([]string{"-z", "-1"}, natal...), "-tr", "5", "2026"), frame...),
	}
	if !slices.EqualFunc(fake.argvs, want, slices.Equal) {
		t.Fatalf("argvs =\n%v\nwant\n%v", fake.argvs, want)
	}

	// Two segments: each search twice, one per offset, and never more.
	fake = &fakeSearch{byZone: map[string]string{}}
	req.Month, req.Zone = time.October, tlAmsterdam(t)
	if _, _, err := ComputeTimeline(context.Background(), req,
		[]TimelineSource{TimelineTransit, TimelineSky, TimelineReturn}, fake.run); err != nil {
		t.Fatal(err)
	}
	var modes []string
	for _, a := range fake.argvs {
		for _, tok := range a {
			if tok == "-t" || tok == "-dm" || tok == "-tr" {
				modes = append(modes, tok+" -z "+a[1])
			}
		}
	}
	if got := strings.Join(modes, ", "); got != "-t -z -2, -t -z -1, -dm -z -2, -dm -z -1, -tr -z -2, -tr -z -1" {
		t.Errorf("spawns = %s", got)
	}
}

// C6 against the real binary: the floating zone moves the search's clock and
// never the natal chart. `-z` must lead the natal switches; a test that
// passes with it anywhere else has not tested C6 (positive control 2).
func TestComputeTimelineUsesTheFloatingZoneWithoutMovingTheNatalChart(t *testing.T) {
	// These pin Astrolog's am/pm month-first text; the held clock says so
	// (the specification C3: the grammar is the preference's).
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	outs := map[int]string{}
	var argvs [][]string
	capture := func(ctx context.Context, call BodyCall) (string, string, error) {
		argvs = append(argvs, call.Argv())
		out, cmd, err := RunTimelineSearch(ctx, call)
		outs[len(outs)] = out
		return out, cmd, err
	}
	run := func(zone *time.Location) []TimelineRow {
		rows, _, err := ComputeTimeline(context.Background(),
			TimelineRequest{Year: 2026, Month: time.October, Zone: zone, Natal: liveNatal},
			[]TimelineSource{TimelineTransit}, capture)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	east := run(time.FixedZone("UTC+1", 3600))
	west := run(time.FixedZone("UTC-2", -7200))

	for _, argv := range argvs {
		if z, q := slices.Index(argv, "-z"), slices.Index(argv, "-qj"); z < 0 || q < 0 || z > q {
			t.Errorf("argv %v: -z must come before the natal -qj", argv)
		}
	}
	natalHeader := "Astrolog 8.00 chart for Tue May 15, 1979  9:30am (ST Zone 0W)"
	for i, zone := range []string{"(ST Zone 1E)", "(ST Zone 2W)"} {
		lines := strings.Split(outs[i], "\n")
		if len(lines) < 3 || !strings.HasPrefix(lines[0], natalHeader) {
			t.Errorf("the natal chart moved under the %s search: %q", zone, lines[0])
		}
		if !strings.HasPrefix(lines[2], "Transits during Oct 2026 "+zone) {
			t.Errorf("search header = %q, want it reported in %s", lines[2], zone)
		}
	}
	// The rows move, not only the header: the same Jupiter trine prints
	// three hours apart and is one and the same instant.
	find := func(rows []TimelineRow) TimelineRow {
		for _, r := range rows {
			if strings.Contains(r.Line, "trans Jupiter (Leo) Tri natal [Sag] Neptune") {
				return r
			}
		}
		t.Fatal("no Jupiter trine Neptune row")
		return TimelineRow{}
	}
	e, w := find(east), find(west)
	if !strings.HasPrefix(e.Line, "10/ 2/2026  5:08am") || !strings.HasPrefix(w.Line, "10/ 2/2026  2:08am") {
		t.Errorf("rows = %q / %q, want 5:08am at 1E and 2:08am at 2W", e.Line, w.Line)
	}
	if !e.Instant.Equal(w.Instant) {
		t.Errorf("the same event resolved to %v and %v: a natal chart moved with the zone", e.Instant.UTC(), w.Instant.UTC())
	}
}

// C5 against the real binary: every row carries the exact command of a
// search that emitted it, and re-running that command's argv prints the row.
// May 2026 because three of its returns come from both -t and -tr — the
// case where provenance could silently be the wrong search's. The merged,
// deduplicated half of this row is C53, in lookup, where the merge lives.
func TestComputeTimelineRetainsPerRowArgv(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	argvByCommand := map[string][]string{}
	capture := func(ctx context.Context, call BodyCall) (string, string, error) {
		out, cmd, err := RunTimelineSearch(ctx, call)
		argvByCommand[cmd] = call.Argv()
		return out, cmd, err
	}
	rows, commands, err := ComputeTimeline(context.Background(),
		TimelineRequest{Year: 2026, Month: time.May, Zone: tlAmsterdam(t), Natal: liveNatal},
		[]TimelineSource{TimelineTransit, TimelineSky, TimelineReturn}, capture)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 3 || len(rows) == 0 {
		t.Fatalf("commands=%d rows=%d, want three spawns and some rows", len(commands), len(rows))
	}
	// Re-run each distinct command once, independently of the capture above.
	rerun := map[string][]string{}
	for _, r := range rows {
		argv, ok := argvByCommand[r.Command]
		if !ok {
			t.Fatalf("%q carries a command no spawn ran: %q", r.Line, r.Command)
		}
		if _, done := rerun[r.Command]; !done {
			out, _, err := RunTimelineSearch(context.Background(), testCall(argv))
			if err != nil {
				t.Fatal(err)
			}
			rerun[r.Command] = strings.Split(out, "\n")
		}
		if !slices.Contains(rerun[r.Command], r.Line) {
			t.Errorf("%q: re-running its command %q does not print it", r.Line, r.Command)
		}
		wantMode := map[TimelineSource]string{TimelineTransit: " -t ", TimelineSky: " -dm", TimelineReturn: " -tr "}[r.Source]
		if !strings.Contains(r.Command+" ", wantMode) {
			t.Errorf("%q is a %s row carrying %q", r.Line, r.Source, r.Command)
		}
	}
}

// The strict grammar against the real binary at scale, so that a parser
// narrow enough to refuse a reshaped line is not also too narrow to read what
// Astrolog actually prints: two whole years of -dy (every sky row kind), and
// a quarter of -t and -tr on the spec's natal. Every nonblank line that is
// not a header must parse, and parse to exactly one row.
func TestParseTimelineReadsWholeYearsOfRealOutput(t *testing.T) {
	// These pin Astrolog's am/pm month-first text; the held clock says so
	// (the specification C3: the grammar is the preference's).
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	var runs [][]string
	for _, year := range []string{"2026", "2027"} {
		runs = append(runs, []string{"-z", "-1", "-qy", year, "-dy"})
	}
	natal := []string{"-qj", "2444008.895833334", "-zl", "4:54E", "52:22N"}
	for _, month := range []string{"3", "6", "10"} {
		for _, mode := range []string{"-t", "-tr"} {
			runs = append(runs, append(append([]string{"-z", "-1"}, natal...), mode, month, "2026"))
		}
	}
	for _, argv := range runs {
		out, _, err := RunTimelineSearch(context.Background(), testCall(argv))
		if err != nil {
			t.Fatal(err)
		}
		rows, err := ParseTimeline(out, utcPlusOne)
		if err != nil {
			t.Errorf("%v: %v", argv, err)
			continue
		}
		want := 0
		for _, l := range strings.Split(out, "\n") {
			if strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "Astrolog ") && !strings.HasPrefix(l, "Transits during ") {
				want++
			}
		}
		if len(rows) != want || want == 0 {
			t.Errorf("%v: %d rows from %d event lines", argv, len(rows), want)
		}
	}
}

// the specification rule 1: the parser cuts the aspect, the
// second body, an ingress's sign and direction and a station beside the first
// body, from the same match, and the line stays Astrolog's. Every grammar of
// the corpus is asserted here, beside the unchanged
// TestParseTimelineReadsEveryMeasuredGrammar.
func TestParseTimelineCutsTheAspectAndSecondBody(t *testing.T) {
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	want := map[string]struct {
		kind                     TimelineKind
		body, aspectAbbr, other  string
		sign, direction, station string
		bodyID, otherID          aspect.Body
		aspectID                 aspect.AspectKind
		resolved                 bool
	}{
		"-t transit row":                             {kind: TimelineKindAspect, body: "Jupiter", aspectAbbr: "Tri", other: "Neptune", bodyID: aspect.Jupiter, otherID: aspect.Neptune, aspectID: aspect.Trine, resolved: true},
		"-t row with a padded month and 12pm":        {kind: TimelineKindAspect, body: "Mercury", aspectAbbr: "Tri", other: "Mars", bodyID: aspect.Mercury, otherID: aspect.Mars, aspectID: aspect.Trine, resolved: true},
		"-tr return row with its label":              {kind: TimelineKindAspect, body: "Moon", aspectAbbr: "Con", other: "Moon", bodyID: aspect.Moon, otherID: aspect.Moon, aspectID: aspect.Conjunction, resolved: true},
		"-t row carrying a return label":             {kind: TimelineKindAspect, body: "Sun", aspectAbbr: "Con", other: "Sun", bodyID: aspect.Sun, otherID: aspect.Sun, aspectID: aspect.Conjunction, resolved: true},
		"-dm aspect row with its weekday":            {kind: TimelineKindAspect, body: "Moon", aspectAbbr: "Con", other: "Uranus", bodyID: aspect.Moon, otherID: aspect.Uranus, aspectID: aspect.Conjunction, resolved: true},
		"-dm --> ingress":                            {kind: TimelineKindIngress, body: "Moon", sign: "Cancer", direction: "-->", bodyID: aspect.Moon, resolved: true},
		"-dm <-- retrograde ingress":                 {kind: TimelineKindIngress, body: "Venus", sign: "Libra", direction: "<--", bodyID: aspect.Venus, resolved: true},
		"-dm S/R station in <> brackets":             {kind: TimelineKindStation, body: "Venus", station: "S/R", bodyID: aspect.Venus, resolved: true},
		"-dm trailing phenomenon label":              {kind: TimelineKindAspect, body: "Sun", aspectAbbr: "Con", other: "Moon", bodyID: aspect.Sun, otherID: aspect.Moon, aspectID: aspect.Conjunction, resolved: true},
		"-dm two stacked labels":                     {kind: TimelineKindAspect, body: "Sun", aspectAbbr: "Opp", other: "Moon", bodyID: aspect.Sun, otherID: aspect.Moon, aspectID: aspect.Opposition, resolved: true},
		"-dm trailing (v/c HH:MM)":                   {kind: TimelineKindAspect, body: "Moon", aspectAbbr: "Sex", other: "Jupiter", bodyID: aspect.Moon, otherID: aspect.Jupiter, aspectID: aspect.Sextile, resolved: true},
		"-dm 12am with a (v/c) whose clock is later": {kind: TimelineKindAspect, body: "Moon", aspectAbbr: "Sex", other: "Jupiter", bodyID: aspect.Moon, otherID: aspect.Jupiter, aspectID: aspect.Sextile, resolved: true},
		"-dm two-word body":                          {kind: TimelineKindIngress, body: "North N", sign: "Aquarius", direction: "<--", bodyID: aspect.NorthNode, resolved: true},
		"-dm minor aspect BQn":                       {kind: TimelineKindAspect, body: "Moon", aspectAbbr: "BQn", other: "Mercury", bodyID: aspect.Moon, otherID: aspect.Mercury, aspectID: aspect.Biquintile, resolved: true},
		"-dm minor aspect SSx":                       {kind: TimelineKindAspect, body: "Moon", aspectAbbr: "SSx", other: "Venus", bodyID: aspect.Moon, otherID: aspect.Venus, aspectID: aspect.Semisextile, resolved: true},
		"-t minor aspect SSq":                        {kind: TimelineKindAspect, body: "Mars", aspectAbbr: "SSq", other: "Neptune", bodyID: aspect.Mars, otherID: aspect.Neptune, aspectID: aspect.Semisquare, resolved: true},
	}
	for _, tc := range timelineCorpus {
		w, ok := want[tc.name]
		if !ok {
			t.Errorf("corpus case %q has no expected columns: add it to this test", tc.name)
			continue
		}
		rows, err := ParseTimeline(tc.line+"\n", utcPlusOne)
		if err != nil || len(rows) != 1 {
			t.Fatalf("%s: ParseTimeline = %v rows, err %v", tc.name, len(rows), err)
		}
		r := rows[0]
		if r.Line != tc.line {
			t.Errorf("%s: line = %q, want it byte-identical", tc.name, r.Line)
		}
		if r.Kind != w.kind || r.Body != w.body || r.Aspect != w.aspectAbbr || r.Other != w.other ||
			r.Sign != w.sign || r.Direction != w.direction || r.Station != w.station {
			t.Errorf("%s: columns = {kind %d body %q aspect %q other %q sign %q dir %q station %q}, want %+v", tc.name,
				r.Kind, r.Body, r.Aspect, r.Other, r.Sign, r.Direction, r.Station, w)
		}
		if r.Resolved != w.resolved || r.BodyID != w.bodyID || r.OtherID != w.otherID || r.AspectID != w.aspectID {
			t.Errorf("%s: resolved = %v body %d other %d aspect %d, want %v %d %d %d", tc.name,
				r.Resolved, r.BodyID, r.OtherID, r.AspectID, w.resolved, w.bodyID, w.otherID, w.aspectID)
		}
	}
	// A name the registries cannot name leaves the row unresolved (rule 2 of
	// the spec), and the line is still read.
	rows, err := ParseTimeline("Thu 10/ 1/2026  3:43am    Moon (Gem) Con [Gem] Ceres\n", utcPlusOne)
	if err != nil || len(rows) != 1 {
		t.Fatalf("an unnamed second body: %d rows, err %v", len(rows), err)
	}
	if rows[0].Other != "Ceres" || rows[0].Resolved {
		t.Errorf("an unnamed second body: other %q resolved %v, want %q unresolved", rows[0].Other, rows[0].Resolved, "Ceres")
	}
}

// ---- the specification ---------------------------------------

const skyOverflowText = "Too many transit events found."

// dayRows is n sky rows on one printed date, one minute apart from hour h.
func dayRows(date, weekday string, hour12 int, ampm string, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "%s %s %2d:%02d%s    Moon (Leo) Tri [Ari] Neptune\n", weekday, date, hour12, i%60, ampm)
	}
	return b.String()
}

// skyCall runs the sky search once over October 2026 in UTC (one segment)
// through a canned searcher and returns the call's result.
func skyCall(t *testing.T, src TimelineSource, out string) TimelineCallResult {
	t.Helper()
	return skyCallClock(t, Clock{Hours: Hours12, Dates: MonthFirst}, src, out)
}

// skyCallClock is skyCall under the given clock pins.
func skyCallClock(t *testing.T, c Clock, src TimelineSource, out string) TimelineCallResult {
	t.Helper()
	withClock(t, c)
	req := TimelineRequest{Year: 2026, Month: time.October, Zone: time.UTC,
		Natal: &TimelineNatal{When: time.Date(1979, 5, 15, 9, 30, 0, 0, time.UTC), Lon: 4.9, Lat: 52.3667}}
	canned := func(context.Context, BodyCall) (string, string, error) { return out, "astrolog canned", nil }
	res, err := ComputeTimelineCalls(context.Background(), req, src, canned)
	if err != nil || len(res) != 1 {
		t.Fatalf("ComputeTimelineCalls = %v, %v; want one result", res, err)
	}
	return res[0]
}

// Acceptance 1: the line with no busy day yields exactly its rows, no error.
func TestSkyOverflowLineWithNoBusyDayIsReadAsItsRows(t *testing.T) {
	out := "Thu 10/ 1/2026  3:43am    Moon (Gem) Con [Gem] Uranus\n" +
		"Fri 10/ 2/2026  3:41am    Moon (Gem) Sex (Leo) Jupiter (v/c 17:12)\n" + skyOverflowText + "\n"
	res := skyCall(t, TimelineSky, out)
	if res.Err != nil {
		t.Fatalf("false-alarm line refused: %v", res.Err)
	}
	if len(res.Rows) != 2 || !strings.Contains(res.Rows[1].Line, "(v/c 17:12)") {
		t.Fatalf("rows = %s, want the two event rows, v/c text included", dumpRows(res.Rows))
	}
}

// Acceptance 2: 74 rows on one date are accepted, 75 are refused naming the line.
func TestSkyOverflowLineBoundaryIs74AcceptedAnd75Refused(t *testing.T) {
	ok := skyCall(t, TimelineSky, dayRows("10/15/2026", "Thu", 1, "am", 74)+skyOverflowText+"\n")
	if ok.Err != nil || len(ok.Rows) != 74 {
		t.Fatalf("74 rows on one date: err=%v rows=%d, want accepted with 74", ok.Err, len(ok.Rows))
	}
	bad := skyCall(t, TimelineSky, dayRows("10/15/2026", "Thu", 1, "am", 75)+skyOverflowText+"\n")
	var lineErr *TimelineLineError
	if !errors.As(bad.Err, &lineErr) || lineErr.Line != skyOverflowText || len(bad.Rows) != 0 {
		t.Fatalf("75 rows on one date: err=%v rows=%d, want a refusal naming the line", bad.Err, len(bad.Rows))
	}
}

// clockDayRows is n sky rows on 15 October 2026 (a Thursday) in the form
// the clock pins print: month-first `10/15/2026` or day-first `15-10-2026`,
// 24-hour `1:05` or 12-hour `1:05am`.
func clockDayRows(c Clock, n int) string {
	date := "10/15/2026"
	if c.Dates == DayFirst {
		date = "15-10-2026"
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		tm := fmt.Sprintf("%2d:%02d", 1, i%60)
		if c.Hours == Hours12 {
			tm += "am"
		}
		fmt.Fprintf(&b, "Thu %s %s    Moon (Leo) Tri [Ari] Neptune\n", date, tm)
	}
	return b.String()
}

// Acceptance 2 under every clock the operator can pin: the date grouping is
// the printed date in each of the four forms (review round 1, P3).
func TestSkyOverflowLineBoundaryHoldsUnderEveryClock(t *testing.T) {
	for _, c := range []Clock{{Hours24, MonthFirst}, {Hours24, DayFirst}, {Hours12, MonthFirst}, {Hours12, DayFirst}} {
		ok := skyCallClock(t, c, TimelineSky, clockDayRows(c, 74)+skyOverflowText+"\n")
		if ok.Err != nil || len(ok.Rows) != 74 {
			t.Errorf("%v: 74 rows on one date: err=%v rows=%d, want accepted with 74", c, ok.Err, len(ok.Rows))
		}
		bad := skyCallClock(t, c, TimelineSky, clockDayRows(c, 75)+skyOverflowText+"\n")
		var lineErr *TimelineLineError
		if !errors.As(bad.Err, &lineErr) || lineErr.Line != skyOverflowText {
			t.Errorf("%v: 75 rows on one date: err=%v rows=%d, want a refusal naming the line", c, bad.Err, len(bad.Rows))
		}
	}
}

// Astrolog prints the line inside its day loop (charts3.cpp:665), so rows can
// follow it: they are read too. A bad line after it is still named (the reviewer
// review round 1, P2).
func TestSkyOverflowLineMidOutput(t *testing.T) {
	first := "Thu 10/ 1/2026  3:43am    Moon (Gem) Con [Gem] Uranus\n"
	later := "Fri 10/ 2/2026  3:41am    Moon (Gem) Sex (Leo) Jupiter (v/c 17:12)\n"
	res := skyCall(t, TimelineSky, first+skyOverflowText+"\n"+later)
	if res.Err != nil || len(res.Rows) != 2 || res.Rows[1].Line != strings.TrimSuffix(later, "\n") {
		t.Fatalf("warning between rows: err=%v rows=%s, want both rows read", res.Err, dumpRows(res.Rows))
	}
	bad := skyCall(t, TimelineSky, first+skyOverflowText+"\nnot an event line\n"+later)
	var lineErr *TimelineLineError
	if !errors.As(bad.Err, &lineErr) || lineErr.Line != "not an event line" {
		t.Fatalf("a bad line after the warning: err=%v, want a refusal naming that line", bad.Err)
	}
}

// Acceptance 2: rows of one date printed in separate flushes count together.
func TestSkyOverflowLineCountsOneDateAcrossFlushes(t *testing.T) {
	out := dayRows("10/15/2026", "Thu", 1, "am", 40) +
		"Fri 10/16/2026  3:43am    Moon (Gem) Con [Gem] Uranus\n" +
		dayRows("10/15/2026", "Thu", 2, "am", 35) + skyOverflowText + "\n"
	if res := skyCall(t, TimelineSky, out); res.Err == nil {
		t.Fatalf("40+35 rows of one date were accepted (%d rows)", len(res.Rows))
	}
}

// Acceptance 3: the count is over the raw rows, before the segment filter,
// and a DST month counts each invocation on its own.
func TestSkyOverflowLineCountsRawRowsPerInvocation(t *testing.T) {
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	ams := tlAmsterdam(t)
	req := TimelineRequest{Year: 2026, Month: time.October, Zone: ams}
	// -2 covers up to 25 Oct 03:00 CEST. 74 rows inside on the 25th plus one
	// printed 11pm on the 25th (outside the segment): 75 raw, 74 kept.
	rawBusy := dayRows("10/25/2026", "Sun", 12, "am", 74) + "Sun 10/25/2026 11:00pm    Moon (Leo) Tri [Ari] Neptune\n" + skyOverflowText + "\n"
	// Two invocations of 40 rows on the 25th each: 80 together, never counted together.
	split := map[string]string{
		"-2": dayRows("10/25/2026", "Sun", 12, "am", 40) + skyOverflowText + "\n",
		"-1": dayRows("10/25/2026", "Sun", 10, "am", 40) + skyOverflowText + "\n",
	}
	run := func(byZone map[string]string) []TimelineCallResult {
		f := &fakeSearch{byZone: byZone}
		res, err := ComputeTimelineCalls(context.Background(), req, TimelineSky, f.run)
		if err != nil || len(res) != 2 {
			t.Fatalf("ComputeTimelineCalls = %v, %v", res, err)
		}
		return res
	}
	busy := run(map[string]string{"-2": rawBusy, "-1": skyOverflowText + "\n"})
	if busy[0].Err == nil {
		t.Errorf("75 raw rows on a date (74 inside the segment) were accepted: %d rows", len(busy[0].Rows))
	}
	each := run(split)
	for i, r := range each {
		if r.Err != nil || len(r.Rows) != 40 {
			t.Errorf("invocation %d: err=%v rows=%d, want 40 accepted on its own", i, r.Err, len(r.Rows))
		}
	}
}

// Acceptance 4 and 5: through the call path, the sky exception is the sky
// reader's alone, and the transits warning refuses everywhere.
func TestOverflowLinesRefuseWhereTheyAreNotTheSkyException(t *testing.T) {
	row := "10/ 2/2026  5:08am trans Jupiter (Leo) Tri natal [Sag] Neptune\n"
	cases := []struct {
		name string
		src  TimelineSource
		out  string
		line string
	}{
		{"-t refuses the sky line", TimelineTransit, row + skyOverflowText + "\n", skyOverflowText},
		{"-tr refuses the sky line", TimelineReturn, row + skyOverflowText + "\n", skyOverflowText},
		{"sky refuses the transits warning", TimelineSky, "Thu 10/ 1/2026  3:43am    Moon (Gem) Con [Gem] Uranus\nToo many transits found.\n", "Too many transits found."},
		{"-t refuses the transits warning", TimelineTransit, row + "Too many transits found.\n", "Too many transits found."},
		{"-tr refuses the transits warning", TimelineReturn, row + "Too many transits found.\n", "Too many transits found."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := skyCall(t, c.src, c.out)
			var lineErr *TimelineLineError
			if !errors.As(res.Err, &lineErr) || lineErr.Line != c.line || len(res.Rows) != 0 {
				t.Fatalf("err=%v rows=%d, want a refusal naming %q", res.Err, len(res.Rows), c.line)
			}
		})
	}
}

// Acceptance 4: ParseTimeline itself still refuses the line.
func TestParseTimelineStillRefusesTheSkyOverflowLine(t *testing.T) {
	_, err := ParseTimeline(skyOverflowText+"\n", time.UTC)
	var lineErr *TimelineLineError
	if !errors.As(err, &lineErr) || lineErr.Line != skyOverflowText {
		t.Fatalf("ParseTimeline err = %v, want a refusal naming the line", err)
	}
}

// Acceptance 6: the installed astrolog, October 2026, six default aspects
// plus inconjunct: the warning is printed and every dated row is read.
func TestSkyOverflowLiveOctober2026ReadsEveryRow(t *testing.T) {
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	req := TimelineRequest{Year: 2026, Month: time.October, Zone: time.UTC,
		Aspects: recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2, 3, 4, 5, 6}}}
	var raw string
	capture := func(ctx context.Context, call BodyCall) (string, string, error) {
		out, cmd, err := RunTimelineSearch(ctx, call)
		raw = out
		return out, cmd, err
	}
	res, err := ComputeTimelineCalls(context.Background(), req, TimelineSky, capture)
	if err != nil || len(res) != 1 {
		t.Fatalf("ComputeTimelineCalls = %v, %v", res, err)
	}
	if !strings.Contains(raw, "\n"+skyOverflowText) {
		t.Fatalf("astrolog no longer prints %q for this search; the fixture proves nothing:\n...%s", skyOverflowText, raw[max(0, len(raw)-200):])
	}
	if res[0].Err != nil {
		t.Fatalf("the false alarm was refused: %v", res[0].Err)
	}
	// Row by row, in order: a lost or duplicated row cannot hide behind an
	// equal count (review round 1, P2).
	var dated []string
	for _, l := range strings.Split(raw, "\n") {
		if len(l) > 3 && strings.Contains("SunMonTueWedThuFriSat", l[:3]) && l[3] == ' ' {
			dated = append(dated, l)
		}
	}
	if len(dated) < 100 {
		t.Fatalf("raw output has %d dated rows; a month at six aspects has well over 100", len(dated))
	}
	if len(res[0].Rows) != len(dated) {
		t.Fatalf("accepted %d rows, raw output has %d dated rows", len(res[0].Rows), len(dated))
	}
	for i, l := range dated {
		if res[0].Rows[i].Line != l {
			t.Fatalf("row %d = %q, raw dated row %d = %q", i, res[0].Rows[i].Line, i, l)
		}
	}
}

// TestParseTimelineReadsAMinorAspectRow is the operator's 2026-10-22 line: a
// biquintile the Aspects chooser had turned on, printed under the default
// 24-hour clock, refused the whole month with "a line it could not place".
func TestParseTimelineReadsAMinorAspectRow(t *testing.T) {
	withClock(t, Clock{})
	line := "Thu 10/ 1/2026  7:08    Moon (Gem) BQn (Sco) Mercury"
	rows, err := ParseTimeline(line+"\n", utcPlusOne)
	if err != nil || len(rows) != 1 {
		t.Fatalf("ParseTimeline = %d rows, err %v", len(rows), err)
	}
	r := rows[0]
	if r.Kind != TimelineKindAspect || r.Aspect != "BQn" || r.AspectID != aspect.Biquintile || r.Other != "Mercury" || !r.Resolved {
		t.Fatalf("row = kind %v aspect %q id %v other %q resolved %v", r.Kind, r.Aspect, r.AspectID, r.Other, r.Resolved)
	}
	if !r.Instant.Equal(at(utcPlusOne, 2026, 10, 1, 7, 8)) {
		t.Fatalf("instant = %v", r.Instant)
	}
}
