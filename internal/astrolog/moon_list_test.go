package astrolog

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

const moonFilterTokens = "-~d Or Equ @u 2 And Gt @v 0 Equ @w 2"

func moonListRequest(t *testing.T) TimelineRequest {
	t.Helper()
	req := bandReq(t, hellenistic(), recipe.AspectPolicy{})
	req.Registry = testRegistry(t)
	req.AspectRegistry = testAspects(t)
	req.Voids = noVoids
	req.Moon = true
	return req
}

// M2: the Moon list's sky rows are Astrolog's day searches with the Moon filter
// added, one per local-day portion of each offset segment, and the marks come
// from the void search.
func TestMoonSkyArgv(t *testing.T) {
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	moonReq := moonListRequest(t)
	plain := moonReq
	plain.Moon = false
	var days = func(req TimelineRequest) [][]string {
		fake := &fakeSearch{byZone: map[string]string{}}
		if _, _, err := ComputeTimeline(context.Background(), req, []TimelineSource{TimelineSky}, fake.run); err != nil {
			t.Fatal(err)
		}
		return fake.argvs
	}
	got, base := days(moonReq), days(plain)
	if len(got) == 0 || len(got) != len(base) {
		t.Fatalf("the Moon list ran %d day calls, the timeline %d, want the same count (a clock-change date is two calls)", len(got), len(base))
	}
	for i, argv := range got {
		if !strings.Contains(" "+strings.Join(argv, " ")+" ", " "+moonFilterTokens+" ") {
			t.Errorf("day call %d %v lacks the Moon filter", i, argv)
		}
		if !slices.Equal(argv[:len(argv)-2], base[i]) {
			t.Errorf("day call %d is not the timeline's plus the filter:\n%v\n%v", i, argv, base[i])
		}
	}

	// The marks come from the void search, the rows from the day search.
	const (
		bare   = "Sat 10/ 3/2026  1:10am    Moon (Leo) Sex [Lib] Venus"
		marked = bare + " (v/c 8:17)"
	)
	parsed, err := ParseTimeline(marked+"\n", time.UTC, nil, testAspects(t))
	if err != nil {
		t.Fatal(err)
	}
	parsed[0].Source, parsed[0].Command = TimelineSky, "astrolog void"
	moonReq.Zone = time.UTC
	moonReq.Voids = func(context.Context, VoidRequest) (VoidResult, bool, error) {
		return VoidResult{Rows: parsed, Command: "astrolog void"}, true, nil
	}
	search := func(_ context.Context, call BodyCall) (string, string, error) {
		argv := call.Argv()
		if i := slices.Index(argv, "-qd"); i >= 0 && argv[i+2] == "3" {
			return bare + "\n", "astrolog day", nil
		}
		return "", "astrolog day", nil
	}
	rows, _, err := ComputeTimeline(context.Background(), moonReq, []TimelineSource{TimelineSky}, search)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Line != marked {
		t.Errorf("rows =\n%s want the one Moon row with the void search's mark", dumpRows(rows))
	}
}

// M3: the Moon list's transit search has the Moon alone on its transiting side,
// the natal side as the Timeline's, and no return search is asked for.
func TestMoonTransitArgv(t *testing.T) {
	req := moonListRequest(t)
	plain := req
	plain.Moon = false
	moonCall, err := TimelineCall(TimelineTransit, TimelineSegment{Offset: 3600}, req)
	if err != nil {
		t.Fatal(err)
	}
	plainCall, err := TimelineCall(TimelineTransit, TimelineSegment{Offset: 3600}, plain)
	if err != nil {
		t.Fatal(err)
	}
	mn, mt := bandSides(strings.Join(moonCall.Argv(), " "))
	pn, pt := bandSides(strings.Join(plainCall.Argv(), " "))
	if !slices.Equal(mt, []int{2}) {
		t.Errorf("the Moon list's transiting side = %v, want [2]", mt)
	}
	if !slices.Equal(mn, pn) || len(pt) == 1 {
		t.Errorf("natal side %v vs the timeline's %v (its transiting side %v)", mn, pn, pt)
	}
	if !slices.Contains(moonCall.Argv(), "-t") || slices.Contains(moonCall.Argv(), "-tr") {
		t.Errorf("argv %v, want -t and no -tr", moonCall.Argv())
	}
	// The now slot's marks never add to it.
	marked := req
	marked.Objects.Only = onlyObjects(objSun)
	call, err := TimelineCall(TimelineTransit, TimelineSegment{Offset: 3600}, marked)
	if err != nil {
		t.Fatal(err)
	}
	if _, tr := bandSides(strings.Join(call.Argv(), " ")); !slices.Equal(tr, []int{2}) {
		t.Errorf("with a mark on the Sun the transiting side = %v, want [2]", tr)
	}
}

// The guard: AstroExpression filters fail silently, so a Moon list refuses a
// sky row without the Moon.
func TestMoonListRefusesRowsWithoutTheMoon(t *testing.T) {
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	req := moonListRequest(t)
	req.Zone = time.UTC
	const row = "Sat 10/ 3/2026  8:15am   Venus <Sco> S/R"
	search := func(_ context.Context, call BodyCall) (string, string, error) {
		if i := slices.Index(call.Argv(), "-qd"); i >= 0 && call.Argv()[i+2] == "3" {
			return row + "\n", "astrolog day", nil
		}
		return "", "astrolog day", nil
	}
	_, _, err := ComputeTimeline(context.Background(), req, []TimelineSource{TimelineSky}, search)
	if err == nil || !strings.Contains(err.Error(), "Moon list: the Moon filter did not apply") || !strings.Contains(err.Error(), row) {
		t.Errorf("err = %v, want the Moon list's filter-did-not-apply refusal naming the row", err)
	}
	req.Moon = false
	if _, _, err := ComputeTimeline(context.Background(), req, []TimelineSource{TimelineSky}, search); err != nil {
		t.Errorf("the Timeline refused a row without the Moon: %v", err)
	}
}
