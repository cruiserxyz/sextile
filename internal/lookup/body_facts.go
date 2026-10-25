package lookup

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The facts a body template may name (the specification
// § Shape): each key names a primitive, never a body or a moment — the
// template binds the subject and the section binds the moment. Each fact is
// computed (as Astrolog printed it) or derived (Go arithmetic on Astrolog's
// output, with its divergence-ledger row and marker).

// FactKind is what a fact is.
type FactKind int

const (
	FactValue FactKind = iota + 1
	FactInstant
	FactSpan
	FactList
	FactCategory
)

// FactSpec is one registry entry.
type FactSpec struct {
	Key    string
	Kind   FactKind
	Class  string // "computed" or "derived"
	Unit   string
	Source string // the Astrolog call its inputs come from
	Max    int    // a list's most items
}

var factRegistry = []FactSpec{
	{Key: "elongation.lon", Kind: FactValue, Class: "derived", Unit: "degree", Source: "the Sun pair, -b0"},
	{Key: "elongation.lon.abs", Kind: FactValue, Class: "derived", Unit: "degree", Source: "the Sun pair, -b0"},
	{Key: "elongation.side", Kind: FactCategory, Class: "derived", Source: "the Sun pair, -b0"},
	{Key: "synodic.conjunction.inferior.last5", Kind: FactList, Class: "derived", Source: "the cycle search, -dY", Max: 5},
	// the specification: Venus at the floating moment.
	{Key: "synodic.phase", Kind: FactCategory, Class: "derived", Source: "the Sun pair, -b0"},
	{Key: "synodic.phase.alt", Kind: FactCategory, Class: "derived", Source: "the Sun pair, -b0"},
	{Key: "elongation.trend", Kind: FactCategory, Class: "derived", Source: "the Sun pair's velocities, -b0"},
	{Key: "sign.same-as-sun", Kind: FactCategory, Class: "derived", Source: "the Sun pair, -b0"},
	{Key: "motion.direction", Kind: FactCategory, Class: "computed", Source: "the Sun pair's R column, -v"},
	{Key: "station.last", Kind: FactInstant, Class: "derived", Source: "the cycle search, -dY"},
	{Key: "station.next", Kind: FactInstant, Class: "derived", Source: "the cycle search, -dY"},
	{Key: "synodic.conjunction.inferior.last", Kind: FactInstant, Class: "derived", Source: "the cycle search, -dY"},
	{Key: "synodic.conjunction.inferior.next", Kind: FactInstant, Class: "derived", Source: "the cycle search, -dY"},
	{Key: "synodic.conjunction.superior.last", Kind: FactInstant, Class: "derived", Source: "the cycle search, -dY"},
	{Key: "synodic.conjunction.superior.next", Kind: FactInstant, Class: "derived", Source: "the cycle search, -dY"},
	{Key: "brightness.magnitude", Kind: FactValue, Class: "computed", Unit: "magnitude", Source: "the Sun pair, -Z"},
	{Key: "brightness.lit", Kind: FactValue, Class: "computed", Unit: "percent", Source: "the Sun pair, -Z"},
	{Key: "synodic.conjunction.inferior.sixth", Kind: FactValue, Class: "derived", Source: "the cycle search, -dY"},
	{Key: "synodic.conjunction.inferior.drift", Kind: FactValue, Class: "derived", Unit: "degree", Source: "the cycle search, -dY"},
}

// FactSpecFor is the registry entry for a key.
func FactSpecFor(key string) (FactSpec, bool) {
	for _, s := range factRegistry {
		if s.Key == key {
			return s, true
		}
	}
	return FactSpec{}, false
}

// BodyFact is one fact's value for one section. Text is a value's display
// form; Term a category's term id; Items a list's entries; Num the value a
// rule judges, unrounded.
type BodyFact struct {
	Kind   FactKind
	Absent bool
	Text   string
	Term   string
	Items  []string
	Num    float64
}

// CycleSpan is the years one cycle search covers for a moment: 1 January of
// ten years before the moment's year to 1 January three years after it, in
// the moment's own zone (P11).
type CycleSpan struct {
	StartYear, Years int
}

// CycleSpanFor is the span a moment's search covers.
func CycleSpanFor(when time.Time, zone *time.Location) CycleSpan {
	return CycleSpan{StartYear: when.In(zone).Year() - 10, Years: 13}
}

// Contains reports whether the moment's year is inside the span, so a time
// step that stays inside it starts no new search.
func (s CycleSpan) Contains(when time.Time, zone *time.Location) bool {
	return s == CycleSpanFor(when, zone)
}

// CycleResult is one cycle search: its events and the command that ran.
type CycleResult struct {
	Span     CycleSpan
	Events   []astrolog.CycleEvent
	Commands []string
}

// SearchCycle runs the cycle search of the Sun and the body over the span.
func SearchCycle(ctx context.Context, body aspect.Body, span CycleSpan, lens Lens, search astrolog.TimelineSearcher) (CycleResult, error) {
	res := CycleResult{Span: span}
	call, err := astrolog.CycleCall(astrolog.CycleRequest{StartYear: span.StartYear, Years: span.Years, Other: body, Houses: lens.Houses, Zodiac: lens.Zodiac})
	if err != nil {
		return res, fmt.Errorf("lookup: SearchCycle: %w", err)
	}
	out, command, err := search(ctx, call)
	if command != "" {
		res.Commands = append(res.Commands, command)
	}
	if err != nil {
		return res, fmt.Errorf("lookup: SearchCycle: %w", err)
	}
	if res.Events, err = astrolog.ParseCycleEvents(out); err != nil {
		return res, fmt.Errorf("lookup: SearchCycle: %w", err)
	}
	return res, nil
}

// SunPairResult is the Sun and the body at a moment, to the second of arc.
type SunPairResult struct {
	Sun, Body astrolog.PrecisePosition
	Commands  []string
}

// SunPair runs the precise positions call for the moment.
func SunPair(ctx context.Context, body aspect.Body, when time.Time, place location.PlaceInfo, lens Lens, search astrolog.TimelineSearcher) (SunPairResult, error) {
	var res SunPairResult
	call, err := astrolog.SunPairCall(astrolog.SunPairRequest{When: when, Lon: place.Lon, Lat: place.Lat, Other: body, Houses: lens.Houses, Zodiac: lens.Zodiac})
	if err != nil {
		return res, fmt.Errorf("lookup: SunPair: %w", err)
	}
	out, command, err := search(ctx, call)
	if command != "" {
		res.Commands = append(res.Commands, command)
	}
	if err != nil {
		return res, fmt.Errorf("lookup: SunPair: %w", err)
	}
	if res.Sun, res.Body, err = astrolog.ParseSunPair(out, body); err != nil {
		return res, fmt.Errorf("lookup: SunPair: %w", err)
	}
	return res, nil
}

// NowFacts is the @now section's facts for a body: its elongation from the
// Sun's pair and its last inner conjunctions from the span's cycle search.
// The search must cover the moment; a span without five inner conjunctions
// before it lists the ones it has.
func NowFacts(pair SunPairResult, cycle CycleResult, when time.Time, zone *time.Location) (map[string]BodyFact, error) {
	if !cycle.Span.Contains(when, zone) {
		return nil, fmt.Errorf("lookup: NowFacts: the cycle search covers %d-%d, not the moment's year", cycle.Span.StartYear, cycle.Span.StartYear+cycle.Span.Years-1)
	}
	e := Elongation(pair.Sun.Longitude, pair.Body.Longitude)
	abs := math.Abs(e)
	facts := map[string]BodyFact{
		"elongation.lon":     {Kind: FactValue, Text: degMin(abs), Num: e},
		"elongation.lon.abs": {Kind: FactValue, Text: degMin(abs), Num: abs},
		"elongation.side":    {Kind: FactCategory, Term: ElongationSide(e)},
	}
	spec, _ := FactSpecFor("synodic.conjunction.inferior.last5")
	last := LastInferior(cycle.Events, pair.Body.Body, when, spec.Max)
	// Ten years hold at least six inner conjunctions of an inner planet; a
	// search that yields fewer lacks events the list needs, which is a
	// failure, never a short list (P5).
	if len(last) < spec.Max {
		return nil, &MissingEventError{Event: fmt.Sprintf("%d inner conjunctions before the moment", spec.Max)}
	}
	var items []string
	for _, ev := range last {
		// The date in the operator's clock order, like every date Sextile
		// spells itself, and the degree padded so the sign aligns down the
		// list (the specification R5, R6).
		items = append(items, listItem(ev, zone))
	}
	facts["synodic.conjunction.inferior.last5"] = BodyFact{Kind: FactList, Items: items}

	// the specification C1-C7: the rest of @now. A search that lacks
	// an event a line needs is a failure, never an absent line.
	phase, alt := SynodicPhase(e)
	facts["synodic.phase"] = BodyFact{Kind: FactCategory, Term: phase}
	facts["synodic.phase.alt"] = BodyFact{Kind: FactCategory, Term: alt}
	if trend, ok := ElongationTrend(e, pair.Body.Speed-pair.Sun.Speed); ok {
		facts["elongation.trend"] = BodyFact{Kind: FactCategory, Term: trend}
	} else {
		facts["elongation.trend"] = BodyFact{Kind: FactCategory, Absent: true}
	}
	facts["sign.same-as-sun"] = BodyFact{Kind: FactCategory, Term: SameSignAsSun(pair.Sun.Sign, pair.Body.Sign)}
	motion := "direct"
	if pair.Body.Retrograde {
		motion = "retrograde"
	}
	facts["motion.direction"] = BodyFact{Kind: FactCategory, Term: motion}
	body := pair.Body.Body
	picks := []struct {
		key, what string
		keep      func(astrolog.CycleEvent) bool
	}{
		{"station", "a station", func(ev astrolog.CycleEvent) bool { return ev.Kind == astrolog.CycleStation && ev.Body == body }},
		{"synodic.conjunction.inferior", "an inner conjunction", func(ev astrolog.CycleEvent) bool { return conjunctionIsInner(ev, body) }},
		{"synodic.conjunction.superior", "an outer conjunction", func(ev astrolog.CycleEvent) bool { return conjunctionIsOuter(ev, body) }},
	}
	for _, p := range picks {
		last, next, ok := LastAndNext(cycle.Events, when, p.keep)
		if !ok {
			return nil, &MissingEventError{Event: p.what + " before and after the moment"}
		}
		facts[p.key+".last"] = instantFact(last, body, zone)
		facts[p.key+".next"] = instantFact(next, body, zone)
	}
	facts["brightness.magnitude"] = BodyFact{Kind: FactValue, Text: magnitudeText(pair.Body.Magnitude), Num: pair.Body.Magnitude}
	facts["brightness.lit"] = BodyFact{Kind: FactValue, Text: fmt.Sprintf("%.0f%%", math.Round(pair.Body.LitPercent)), Num: pair.Body.LitPercent}
	six := LastInferior(cycle.Events, body, when, 6)
	if len(six) < 6 {
		return nil, &MissingEventError{Event: "six inner conjunctions before the moment"}
	}
	sixth := six[5]
	drift := EightYearDrift(six[0].OtherAt, sixth.OtherAt)
	facts["synodic.conjunction.inferior.sixth"] = BodyFact{Kind: FactValue, Text: listItem(sixth, zone), Num: pointLon(sixth.OtherAt)}
	sign := "+"
	if drift < 0 {
		sign = "−"
	}
	facts["synodic.conjunction.inferior.drift"] = BodyFact{Kind: FactValue, Text: sign + degMin(math.Abs(drift)), Num: drift}
	return facts, nil
}

// MissingEventError is a cycle search that succeeded but lacks an event a
// line needs: a failure, never an absent line (P5).
type MissingEventError struct{ Event string }

func (e *MissingEventError) Error() string {
	return "lookup: NowFacts: the cycle search lacks " + e.Event
}

// magnitudeText is a magnitude to one decimal with a true minus sign; a value
// that rounds to zero reads 0.0, never −0.0 (C6).
func magnitudeText(m float64) string {
	s := fmt.Sprintf("%.1f", m)
	if s == "-0.0" {
		return "0.0"
	}
	return strings.Replace(s, "-", "−", 1)
}

// listItem is a list entry: the date in the zone, the body's degree and sign.
func listItem(ev astrolog.CycleEvent, zone *time.Location) string {
	d := ev.Instant.In(zone)
	return fmt.Sprintf("%s  %2d°%02d′ %s", CivilDate(d.Year(), int(d.Month()), d.Day(), HeldClock()), ev.OtherAt.Deg, ev.OtherAt.Min, ev.OtherAt.Sign)
}

// instantFact is an event as @now shows it: its date and clock in the
// operator's held clock form (month first or day first, 24 or 12 hours, as
// every other moment on screen; the specification), Astrolog's seconds
// rounded to the nearest minute, in the zone, then the body's degree and sign;
// a station carries its kind as a term (P12, C4).
func instantFact(ev astrolog.CycleEvent, body aspect.Body, zone *time.Location) BodyFact {
	at := ev.At
	if ev.Kind == astrolog.CycleConjunction && ev.Other == body {
		at = ev.OtherAt
	}
	f := BodyFact{Kind: FactInstant, Text: fmt.Sprintf("%s %d°%02d′ %s", heldDateClock(ev.Instant.Round(time.Minute).In(zone)), at.Deg, at.Min, at.Sign)}
	switch ev.Station {
	case "S/R":
		f.Term = "station-retrograde"
	case "S/D":
		f.Term = "station-direct"
	}
	return f
}

// heldDateClock is t's date and clock in the held clock form, without a
// weekday: `Oct 3, 2026 7:16`, `3 Oct 2026 7:16`, `Oct 3, 2026 7:16am`. The
// cycle search spans 1583 onwards, so Go's Gregorian calendar names the date
// Astrolog would.
func heldDateClock(t time.Time) string {
	p := HeldClock()
	hour, mark := t.Hour(), ""
	if p.Hours12 {
		mark = "am"
		if hour >= 12 {
			mark = "pm"
		}
		hour %= 12
		if hour == 0 {
			hour = 12
		}
	}
	clock := fmt.Sprintf("%d:%02d%s", hour, t.Minute(), mark)
	mon := t.Month().String()[:3]
	if p.DayFirst {
		return fmt.Sprintf("%d %s %d %s", t.Day(), mon, t.Year(), clock)
	}
	return fmt.Sprintf("%s %d, %d %s", mon, t.Day(), t.Year(), clock)
}

// pointLon is a printed position's longitude in degrees, to the minute.
func pointLon(p astrolog.CyclePoint) float64 {
	return float64(signIndex[p.Sign])*30 + float64(p.Deg) + float64(p.Min)/60
}

var signIndex = map[string]int{"Ari": 0, "Tau": 1, "Gem": 2, "Can": 3, "Leo": 4, "Vir": 5, "Lib": 6, "Sco": 7, "Sag": 8, "Cap": 9, "Aqu": 10, "Pis": 11}

// SynodicPhase is the body's phase and its traditional sense from the
// unrounded elongation: east of the Sun it sets after it, an evening star,
// occidental; west it rises before it, a morning star, oriental (Lilly 1647
// p.114); exactly 0 is at conjunction (C1).
//
// derivation: synodic phase
func SynodicPhase(e float64) (phase, sense string) {
	switch {
	case e > 0:
		return "evening-star", "occidental"
	case e < 0:
		return "morning-star", "oriental"
	}
	return "at-conjunction", "at-conjunction"
}

// ElongationTrend is whether the body approaches the Sun (its distance in
// longitude shrinks) or leaves it, from the two velocities Astrolog printed;
// none at zero elongation or zero relative rate (C2).
//
// derivation: elongation trend
func ElongationTrend(e, rate float64) (string, bool) {
	switch {
	case e == 0 || rate == 0:
		return "", false
	case e*rate < 0:
		return "approaching", true
	}
	return "leaving", true
}

// SameSignAsSun is whether the body stands in the Sun's sign, as Astrolog
// printed both (the condition of Lilly's combustion).
//
// derivation: same sign as the Sun
func SameSignAsSun(sunSign, bodySign string) string {
	if sunSign == bodySign {
		return "yes"
	}
	return "no"
}

// conjunctionIsOuter reads Astrolog's bracket: a conjunction with the body
// printed in ( ) (direct) is the outer one (C5).
//
// derivation: outer conjunction from Astrolog's bracket
func conjunctionIsOuter(ev astrolog.CycleEvent, body aspect.Body) bool {
	return ev.Kind == astrolog.CycleConjunction && ev.Body == aspect.Sun && ev.Other == body && ev.OtherAt.Bracket == '('
}

// LastAndNext picks the newest kept event at or before the moment and the
// first strictly after it, compared in UT (C4, C5); ok is false when the
// list lacks either.
//
// derivation: last and next events
func LastAndNext(events []astrolog.CycleEvent, when time.Time, keep func(astrolog.CycleEvent) bool) (last, next astrolog.CycleEvent, ok bool) {
	var haveLast, haveNext bool
	for _, ev := range events {
		if !keep(ev) {
			continue
		}
		if !ev.Instant.After(when) {
			if !haveLast || ev.Instant.After(last.Instant) {
				last, haveLast = ev, true
			}
		} else if !haveNext || ev.Instant.Before(next.Instant) {
			next, haveNext = ev, true
		}
	}
	return last, next, haveLast && haveNext
}

// EightYearDrift is the signed shortest longitude difference from the sixth
// most recent inner conjunction to the newest, to the printed minute (C7).
//
// derivation: eight-year drift
func EightYearDrift(newest, sixth astrolog.CyclePoint) float64 {
	return Elongation(pointLon(sixth), pointLon(newest))
}

// degMin is a non-negative angle as whole degrees and arc-minutes, the
// seconds dropped as Astrolog drops them (it prints 4Ari57 for 4°57′41″).
func degMin(deg float64) string {
	totalMin := int(math.Floor(deg*60 + 1e-9))
	return fmt.Sprintf("%d°%02d′", totalMin/60, totalMin%60)
}

// Elongation is the body's longitude minus the Sun's, normalised to
// (−180°, 180°]: positive east of the Sun, negative west (P12).
//
// derivation: elongation by longitude
func Elongation(sunLon, bodyLon float64) float64 {
	e := math.Mod(bodyLon-sunLon, 360)
	if e <= -180 {
		e += 360
	}
	if e > 180 {
		e -= 360
	}
	return e
}

// ElongationSide is the term for the side of the Sun the body is on, on the
// unrounded elongation: east, west, or at conjunction at exactly 0.
//
// derivation: elongation side
func ElongationSide(e float64) string {
	switch {
	case e > 0:
		return "east"
	case e < 0:
		return "west"
	}
	return "at-conjunction"
}

// conjunctionIsInner reads Astrolog's own bracket: a conjunction with the
// body printed in [ ] (retrograde) is the inner one, in ( ) the outer
// (P11). Astrolog prints no word for either.
//
// derivation: inner conjunction from Astrolog's bracket
func conjunctionIsInner(ev astrolog.CycleEvent, body aspect.Body) bool {
	return ev.Kind == astrolog.CycleConjunction && ev.Body == aspect.Sun && ev.Other == body && ev.OtherAt.Bracket == '['
}

// LastInferior is the n most recent inner conjunctions of the body at or
// before the moment, newest first.
//
// derivation: last inner conjunctions
func LastInferior(events []astrolog.CycleEvent, body aspect.Body, when time.Time, n int) []astrolog.CycleEvent {
	var out []astrolog.CycleEvent
	for _, ev := range events {
		if conjunctionIsInner(ev, body) && !ev.Instant.After(when) {
			out = append(out, ev)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Instant.After(out[j].Instant) })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// BodyFactsRequest asks for a body's @now facts at the floating moment.
// Cycle, when it covers the moment's span for the same body and frame, is
// reused, so a time step inside the span starts no new search (P11).
type BodyFactsRequest struct {
	Body  aspect.Body
	When  time.Time
	Place location.PlaceInfo
	Lens  Lens
	Cycle *CycleResult
}

// BodyFactsResult is the facts, the cycle search they used (fresh or
// reused) and the commands that ran for them.
type BodyFactsResult struct {
	Now      map[string]BodyFact
	Cycle    CycleResult
	Commands []string
}

// BodyNowFacts runs what the @now facts need and computes them.
func BodyNowFacts(ctx context.Context, req BodyFactsRequest) (BodyFactsResult, error) {
	return bodyNowFactsWith(ctx, req, astrolog.RunTimelineSearch)
}

func bodyNowFactsWith(ctx context.Context, req BodyFactsRequest, search astrolog.TimelineSearcher) (BodyFactsResult, error) {
	var res BodyFactsResult
	zone, err := time.LoadLocation(req.Place.TZ)
	if err != nil || req.Place.TZ == "" {
		return res, fmt.Errorf("lookup: BodyNowFacts: timezone %q does not load", req.Place.TZ)
	}
	span := CycleSpanFor(req.When, zone)
	if req.Cycle != nil && req.Cycle.Span == span {
		res.Cycle = *req.Cycle
	} else {
		cycle, err := SearchCycle(ctx, req.Body, span, req.Lens, search)
		res.Commands = append(res.Commands, cycle.Commands...)
		if err != nil {
			return res, err
		}
		res.Cycle = cycle
	}
	pair, err := SunPair(ctx, req.Body, req.When, req.Place, req.Lens, search)
	res.Commands = append(res.Commands, pair.Commands...)
	if err != nil {
		return res, err
	}
	if res.Now, err = NowFacts(pair, res.Cycle, req.When, zone); err != nil {
		return res, err
	}
	return res, nil
}
