package lookup

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// TimelineKind says what a timeline row is (astrolog.TimelineKind), re-stated
// here because the ui reaches Astrolog only through this package.
type TimelineKind = astrolog.TimelineKind

const (
	TimelineKindNone    = astrolog.TimelineKindNone
	TimelineKindAspect  = astrolog.TimelineKindAspect
	TimelineKindIngress = astrolog.TimelineKindIngress
	TimelineKindStation = astrolog.TimelineKindStation
)

// TimelineRequest is everything the Details timeline depends on — the
// floating slot's Time and Location, the anchored chart when a slot is
// anchored, and the Lens the chart itself runs under. The lens is what makes
// the aspect chooser and the body selector reach the searches' argv: the
// timeline follows them silently, as the wheel and the Bodies panel do
// (the specification, which reverses details-timeline-
// tab.md C28).
type TimelineRequest struct {
	When  time.Time
	Place location.PlaceInfo
	// Natal is the anchored chart, or zero when no slot is anchored (C9).
	Natal store.AnchorChart
	// Lens supplies the object policy, aspect policy and node mode the
	// searches carry. The model's committed lens, never a preview.
	Lens Lens
	// Natals are the anchored selected slots a comparing timeline merges, in
	// numeral order. When it is empty the request is the single-natal (or
	// sky-only) timeline above and nothing about it changes. When it is set
	// it replaces Natal: one sky search plus each natal's own -t and -tr
	// searches, each run on its own so one failure does not lose the others.
	Natals []TimelineNatal
}

// TimelineNatal is one anchored selected slot of a comparing timeline: its
// numeral (its 1-based position in the selected set) and its chart.
type TimelineNatal struct {
	Numeral int
	Chart   store.AnchorChart
}

// TimelineRow is one presented event: its absolute instant, Astrolog's line
// verbatim (D9), and the command of the search that emitted it (gate 12).
type TimelineRow struct {
	Instant time.Time
	Line    string
	Command string

	// Numeral is the selected slot whose chart the row's search ran against
	// (TimelineNatal.Numeral): 0 for a sky row, and for every row when the
	// request carries no Natals. The ui draws it; nothing here formats it.
	Numeral int

	// The columns of Line as the parser cut them (astrolog.TimelineRow).
	Weekday string
	Date    string
	Time    string
	Transit bool
	Body    string
	Rest    string

	// What the row is, the words of its Rest and the registry names they
	// resolve to (astrolog.TimelineRow; the specification rule
	// 2). Resolved says every name the kind needs resolved to a body or
	// aspect Sextile knows; a row where it is false is unmarkable.
	Kind      TimelineKind
	Aspect    string
	Other     string
	Sign      string
	Direction string
	Station   string
	BodyID    aspect.Body
	OtherID   aspect.Body
	AspectID  aspect.AspectKind
	Resolved  bool

	// Julian is the branch Astrolog's reader took for the printed date: the day
	// is before 15 Oct 1582 (astrolog.TimelineRow). The ui marks the date.
	Julian bool
}

// TimelineResult is the merged list plus the commands of every spawn that
// started, in spawn order, for the console's argv lane.
type TimelineResult struct {
	Rows     []TimelineRow
	Commands []string
	// Failures are the searches that failed while others succeeded, in spawn
	// order (only a Natals request produces them: a lone search that fails is
	// the call's error, as before). Numeral 0 names the sky search.
	Failures []TimelineFailure
}

// TimelineFailure is one search that failed in a Natals request: the slot's
// numeral (0 for the sky search), Astrolog's own switch for the search ("-dm",
// "-t" or "-tr") and the error. The rows of every other search are still in
// the result.
type TimelineFailure struct {
	Numeral int
	Search  string
	Err     error
}

// TimelineSearcher runs one search call (its argv and the chooser band the
// compiler issued for it); the shape of astrolog's own. nil means the real
// binary. Tests inject canned output through it, the way ChartComputer is
// stubbed, so this package's rules are exercised without a spawn.
type TimelineSearcher func(ctx context.Context, call BodyCall) (out, command string, err error)

// BodyCall is one search spawn: its argv and the chooser band the compiler
// issued for it. It is an alias so tests above lookup can script a searcher
// without importing astrolog.
type BodyCall = astrolog.BodyCall

// DefaultSets are the object sets Astrolog lists unfiltered, as read from the
// binary; the timeline's body band is hide-only against them.
type DefaultSets = astrolog.DefaultSets

// DefaultSetReader reads the default sets. nil means the real binary.
type DefaultSetReader func(ctx context.Context) (DefaultSets, error)

// Timeline computes the Details timeline for the floating slot's month.
func Timeline(ctx context.Context, req TimelineRequest) (TimelineResult, error) {
	return TimelineWith(ctx, req, nil, nil)
}

// TimelineWith is Timeline through an injected searcher.
//
// With a chart anchored it runs the transit, sky and return searches; with
// none, only the sky search (C9) — a natal search with no natal does not
// spawn at all. Astrolog computes every row; this function only ARRANGES
// them: the union in ascending instant order (C7), rows sharing an instant in
// source order then each search's own output order (C8), and one row kept of
// any that repeat both instant and line — the first under that order, with
// its own command (C27, C53). `-t` already prints every return but the lunar
// one with the label `-tr` prints, byte-identically, which is where the
// duplicates come from.
//
// The default object sets are read BEFORE any search, and a failed or unusable
// read refuses the whole timeline with no search spawned: an unbanded search
// would silently show the bodies the operator hid (C12).
func TimelineWith(ctx context.Context, req TimelineRequest, search TimelineSearcher, defaults DefaultSetReader) (TimelineResult, error) {
	zone, err := time.LoadLocation(req.Place.TZ)
	if err != nil || req.Place.TZ == "" {
		return TimelineResult{}, fmt.Errorf("lookup: Timeline: the floating slot has no usable zone %q", req.Place.TZ)
	}
	// A moment before Julian Day 0 starts nothing: not the default-set read, not
	// a search (the specification C10).
	if astrolog.BeforeJulianDayZero(req.When) {
		return TimelineResult{}, fmt.Errorf("lookup: Timeline: %w", astrolog.ErrBeforeJulianDayZero)
	}
	for _, n := range req.Natals {
		if astrolog.BeforeJulianDayZero(n.Chart.When) {
			return TimelineResult{}, fmt.Errorf("lookup: Timeline: natal %d: %w", n.Numeral, astrolog.ErrBeforeJulianDayZero)
		}
	}
	if !req.Natal.IsZero() && astrolog.BeforeJulianDayZero(req.Natal.When) {
		return TimelineResult{}, fmt.Errorf("lookup: Timeline: natal: %w", astrolog.ErrBeforeJulianDayZero)
	}
	if defaults == nil {
		defaults = astrolog.ReadDefaultSets
	}
	sets, err := defaults(ctx)
	if err != nil {
		return TimelineResult{}, &DefaultSetsError{Err: err}
	}
	year, month := TimelineMonth(req.When, zone)
	areq := astrolog.TimelineRequest{
		Year: year, Month: month, Zone: zone,
		Houses: req.Lens.Houses, Zodiac: req.Lens.Zodiac,
		Objects: req.Lens.Objects, Aspects: req.Lens.Aspects, Defaults: sets,
	}
	if len(req.Natals) > 0 {
		return timelineNatals(ctx, req.Natals, areq, search)
	}
	srcs := []astrolog.TimelineSource{astrolog.TimelineSky}
	if !req.Natal.IsZero() {
		areq.Natal = &astrolog.TimelineNatal{When: req.Natal.When, Lon: req.Natal.Place.Lon, Lat: req.Natal.Place.Lat}
		srcs = []astrolog.TimelineSource{astrolog.TimelineTransit, astrolog.TimelineSky, astrolog.TimelineReturn}
	}
	var run astrolog.TimelineSearcher
	if search != nil {
		run = astrolog.TimelineSearcher(search)
	}
	rows, commands, err := astrolog.ComputeTimeline(ctx, areq, srcs, run)
	if err != nil {
		return TimelineResult{Commands: commands}, fmt.Errorf("lookup: Timeline: %w", err)
	}
	return TimelineResult{Rows: mergeTimeline(rows), Commands: commands}, nil
}

// numberedRow is a row with the numeral of the selected slot whose search
// emitted it (0 for the sky and for every row of a request without Natals).
type numberedRow struct {
	astrolog.TimelineRow
	numeral int
}

// timelineNatals is the comparing timeline: the sky search plus, per natal in
// numeral order, its -t and its -tr, each run through ComputeTimeline on its
// own so one failure costs only that search's rows. Spawn order follows
// today's source order across the whole set: every -t, then the sky, then
// every -tr, so one natal reads exactly as the single-natal timeline does.
// The search is merged by mergeNumbered.
//
// A search that fails is a Failure beside the rows of the others. If NO search
// succeeded the first failure is also the call's error (the result still
// carries the failures and every started spawn's command); a cancelled
// context is the call's error at once, so a superseded run drops silently.
func timelineNatals(ctx context.Context, natals []TimelineNatal, areq astrolog.TimelineRequest, search TimelineSearcher) (TimelineResult, error) {
	seen := make(map[int]bool, len(natals))
	for _, n := range natals {
		if n.Numeral < 1 || seen[n.Numeral] {
			return TimelineResult{}, fmt.Errorf("lookup: Timeline: natal numeral %d is not a distinct position in the selected set", n.Numeral)
		}
		if n.Chart.IsZero() {
			return TimelineResult{}, fmt.Errorf("lookup: Timeline: natal %d has no chart", n.Numeral)
		}
		seen[n.Numeral] = true
	}
	var run astrolog.TimelineSearcher
	if search != nil {
		run = astrolog.TimelineSearcher(search)
	}
	type one struct {
		numeral int
		src     astrolog.TimelineSource
		natal   *astrolog.TimelineNatal
	}
	var plan []one
	asNatal := func(n TimelineNatal) *astrolog.TimelineNatal {
		return &astrolog.TimelineNatal{When: n.Chart.When, Lon: n.Chart.Place.Lon, Lat: n.Chart.Place.Lat}
	}
	for _, n := range natals {
		plan = append(plan, one{n.Numeral, astrolog.TimelineTransit, asNatal(n)})
	}
	plan = append(plan, one{0, astrolog.TimelineSky, nil})
	for _, n := range natals {
		plan = append(plan, one{n.Numeral, astrolog.TimelineReturn, asNatal(n)})
	}

	var rows []numberedRow
	var res TimelineResult
	succeeded := 0
	for _, p := range plan {
		req := areq
		req.Natal = p.natal
		// Failure is isolated per CALL: a DST month runs one search as several
		// offset segments, and one failed segment costs only its own rows.
		calls, err := astrolog.ComputeTimelineCalls(ctx, req, p.src, run)
		if err != nil {
			return res, fmt.Errorf("lookup: Timeline: %w", err)
		}
		for _, c := range calls {
			if c.Command != "" {
				res.Commands = append(res.Commands, c.Command)
			}
			if c.Err != nil {
				if ctx.Err() != nil {
					return res, fmt.Errorf("lookup: Timeline: %w", c.Err)
				}
				res.Failures = append(res.Failures, TimelineFailure{Numeral: p.numeral, Search: p.src.Switch(), Err: c.Err})
				continue
			}
			succeeded++
			for _, r := range c.Rows {
				rows = append(rows, numberedRow{r, p.numeral})
			}
		}
	}
	if succeeded == 0 {
		return res, fmt.Errorf("lookup: Timeline: %w", res.Failures[0].Err)
	}
	res.Rows = mergeNumbered(rows)
	return res, nil
}

// mergeTimeline orders rows by instant, then source, then output position,
// and drops a row that repeats an earlier row's instant and line. ComputeTimeline
// already returns rows grouped by source in C8's order, each in output order,
// so a stable sort on the instant alone yields the whole ordering — but the
// source is compared explicitly so that property is not load-bearing.
func mergeTimeline(rows []astrolog.TimelineRow) []TimelineRow {
	numbered := make([]numberedRow, len(rows))
	for i, r := range rows {
		numbered[i] = numberedRow{r, 0}
	}
	return mergeNumbered(numbered)
}

// mergeNumbered is mergeTimeline over rows that know their slot: by instant,
// ties by source (transit, sky, return), then by numeral, then the search's
// own output order (the sort is stable). A row is dropped only when an earlier
// row of the SAME numeral repeats its instant and line, so two slots' identical
// lines at one instant are both kept.
func mergeNumbered(rows []numberedRow) []TimelineRow {
	sorted := append([]numberedRow(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].Instant.Equal(sorted[j].Instant) {
			return sorted[i].Instant.Before(sorted[j].Instant)
		}
		if sorted[i].Source != sorted[j].Source {
			return sorted[i].Source < sorted[j].Source
		}
		return sorted[i].numeral < sorted[j].numeral
	})
	type identity struct {
		unix    int64
		line    string
		numeral int
	}
	seen := make(map[identity]bool, len(sorted))
	out := make([]TimelineRow, 0, len(sorted))
	for _, r := range sorted {
		id := identity{r.Instant.UnixNano(), r.Line, r.numeral}
		if seen[id] {
			continue
		}
		seen[id] = true
		r.TimelineRow = relabelNodeStation(r.TimelineRow)
		out = append(out, TimelineRow{Instant: r.Instant, Line: r.Line, Command: r.Command, Numeral: r.numeral,
			Weekday: r.Weekday, Date: r.Date, Time: r.Time, Transit: r.Transit, Body: r.Body, Rest: r.Rest, Julian: r.Julian,
			Kind: r.Kind, Aspect: r.Aspect, Other: r.Other, Sign: r.Sign, Direction: r.Direction, Station: r.Station,
			BodyID: r.BodyID, OtherID: r.OtherID, AspectID: r.AspectID, Resolved: r.Resolved})
	}
	return out
}

// stationWord is the `<Sign> S/R` or `<Sign> S/D` token of a station row.
var stationWord = regexp.MustCompile(`(<[A-Za-z]+> )S/([RD])`)

// relabelNodeStation is C17 of the specification (D39): Astrolog
// words the North and South Node station rows the wrong way round against its
// own R column (214 node stations reversed over 2026-27; the planets and
// Lilith agree). The nodes' Station and the station token of Line and Rest
// are swapped, S/R <-> S/D, so the timeline, Bodies and the R column say the
// same thing. Every other body's row passes verbatim (D25).
//
// correction: node station words on the timeline
func relabelNodeStation(r astrolog.TimelineRow) astrolog.TimelineRow {
	if r.Kind != astrolog.TimelineKindStation || (r.BodyID != aspect.NorthNode && r.BodyID != aspect.SouthNode) {
		return r
	}
	swap := func(text string) string {
		return stationWord.ReplaceAllStringFunc(text, func(tok string) string {
			if strings.HasSuffix(tok, "S/R") {
				return strings.TrimSuffix(tok, "S/R") + "S/D"
			}
			return strings.TrimSuffix(tok, "S/D") + "S/R"
		})
	}
	r.Line, r.Rest = swap(r.Line), swap(r.Rest)
	switch r.Station {
	case "S/R":
		r.Station = "S/D"
	case "S/D":
		r.Station = "S/R"
	}
	return r
}

// DefaultSetsError is the timeline's refusal to run when Astrolog's default
// object sets could not be read or were unusable (C12). Without them the
// searches cannot be banded hide-only, and an unbanded search would show the
// bodies the operator hid, so nothing is spawned.
type DefaultSetsError struct{ Err error }

func (e *DefaultSetsError) Error() string {
	return fmt.Sprintf("lookup: Timeline: refused, Astrolog's default object sets are unusable: %v", e.Err)
}

func (e *DefaultSetsError) Unwrap() error { return e.Err }

// DefaultSetsRefused reports whether err is the default-set refusal.
func DefaultSetsRefused(err error) bool {
	var refused *DefaultSetsError
	return errors.As(err, &refused)
}

// TimelineRefusedLine reports the line a timeline failure refused to go on
// with, when the failure was a refusal rather than a spawn that went wrong:
// the Astrolog line the parser could not read, or the sentence naming why the
// default object sets could not be used (DefaultSetsRefused tells them apart).
func TimelineRefusedLine(err error) (string, bool) {
	var defaults *DefaultSetsError
	if errors.As(err, &defaults) {
		return "Astrolog's default body lists could not be read: " + defaults.Err.Error(), true
	}
	var refused *astrolog.TimelineLineError
	if errors.As(err, &refused) {
		return refused.Line, true
	}
	return "", false
}
