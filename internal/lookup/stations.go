package lookup

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Station is the mark a slow, direct body carries in Bodies' speed lane
// (the specification, D38).
type Station uint8

const (
	// StationNone is no mark: the bar shows as it always has.
	StationNone Station = iota
	// StationRetro is `S/R`: the body is about to turn retrograde (its speed
	// is falling toward Astrolog's R).
	StationRetro
	// StationDirect is `S/D`: the body has just turned direct (its speed is
	// rising away from zero).
	StationDirect
)

// String is the text the Bodies lane prints; StationNone is "".
func (s Station) String() string {
	switch s {
	case StationRetro:
		return "S/R"
	case StationDirect:
		return "S/D"
	}
	return ""
}

// trend is the probe's own result: whether a body's speed ratio, as Astrolog
// prints it, is falling, rising or level from the moment to one UT day later.
// The station mark is a selection on it (D38), kept apart so a later view of
// a planet increasing or diminishing in course reads the same comparison
// (council, 2026-10-14, Cartographer recommendation 5).
type trend uint8

const (
	trendLevel trend = iota
	trendFalling
	trendRising
)

// trendOf compares two of Astrolog's printed ratios for one body.
func trendOf(now, later float64) trend {
	switch {
	case later < now:
		return trendFalling
	case later > now:
		return trendRising
	}
	return trendLevel
}

// stationBand is the edge of the band: 5% of the body's average motion, in
// Astrolog's own `=v0` ratio (aspect.Position.Speed). A convention for the
// direct side of a station, not the traditional station period (D38).
const stationBand = 0.05

// stationProbeDay is how far ahead the probe looks: one UT day of Julian Day,
// no civil-clock arithmetic, so a DST change cannot move it.
const stationProbeDay = 1.0

// moonsLo and moonsHi bound Astrolog's planetary moons by object number.
const (
	moonsLo = 52
	moonsHi = 78
)

// stationShape reports whether an object can carry a mark by its number and
// speed alone: a resolved object outside the excluded class, not retrograde by
// the sign bit (Astrolog prints no R), and inside the band. The excluded class
// is a fact by number: the house cusp points (aspect.CuspHouse, which holds the
// four angles), the lunar nodes (16, 17), Lilith (18) and the three points
// derived from the angles, Fortune, Vertex and East Point (19, 20, 21) — their
// speed does not move one way across a day (D38), the last three because they
// move with the angles, not on an orbit. Every other object is a candidate by
// the speed test: Ceres and Sedna as much as Mars, except the planetary moons.
func stationShape(p aspect.Position) bool {
	n := p.Object.Number
	if p.Object == (aspect.Object{}) {
		return false
	}
	if _, cusp := aspect.CuspHouse(p.Object); cusp {
		return false
	}
	switch n {
	case 16, 17, 18, 19, 20, 21:
		return false
	}
	// The planetary moons, Astrolog's objects 52 to 78 (moonsLo..moonsHi: 21
	// core, 12 sign, 9 Uranus, 9 dwarf moons), have no mark. A moon's geocentric
	// speed crosses zero several times a day (Phobos every 8 hours), so the
	// one-day probe's trend is aliased and S/R against S/D would be a coin toss
	// (council Custodian, 2026-11-07). The centres of body, 79 to 83, orbit the
	// Sun and keep theirs.
	if n >= moonsLo && n <= moonsHi {
		return false
	}
	return !math.Signbit(p.Speed) && p.Speed < stationBand
}

// stationCandidate is stationShape and not a fixed star by the registry (a
// star's speed column is not a motion).
func stationCandidate(p aspect.Position, reg aspect.Registry) bool {
	return stationShape(p) && !reg.IsStar(p.Object.Number)
}

// StationMarks is the one place the station rule lives. Given the focused
// chart's positions (as Astrolog printed them at moment/place under lens) it
// finds the slow direct candidates and, when there is at least one, asks
// Astrolog for the same chart one UT day later (ONE positions-only compute) and
// compares each candidate's speed ratio with the probe's: lower later is
// `S/R`, higher is `S/D`, equal or absent is none. With no candidate it makes
// no Astrolog call.
//
// This probe is not the t+epsilon probe an earlier change retired: that one fed
// applying/separating, which Astrolog's -aa now prints; this one exists only to
// say which way a slow body's speed is going.
//
// commands is the probe's ready-to-paste command (also on failure when the
// spawn happened), for the caller to append to its Result's Commands so the
// argv lane shows it like every other compute.
func StationMarks(ctx context.Context, probers *Probers, positions []aspect.Position, moment time.Time, place location.PlaceInfo, lens Lens) (marks map[int]Station, commands []string, err error) {
	var candidates []aspect.Position
	for _, p := range positions {
		if stationCandidate(p, lens.Registry) {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return nil, nil, nil
	}
	spec, err := lens.chartSpecAt(AtMoment(astrolog.SlotJD(moment)+stationProbeDay, place, store.ReportLabel{}))
	if err != nil {
		return nil, nil, fmt.Errorf("lookup: StationMarks: %w", err)
	}
	raw, err := probers.positionsOnly()(ctx, spec)
	if err != nil {
		return nil, raw.Commands, fmt.Errorf("lookup: StationMarks: %w", err)
	}
	return StationMarksBetween(candidates, raw.Positions), raw.Commands, nil
}

// StationMarksBetween is the rule on two readings of the same chart a UT day
// apart: each candidate of now (see stationCandidate) is marked by the trend of
// its ratio into later. StationMarks runs it on the probe's positions; it is
// exported so a test of a caller can put real rule behind a stubbed Astrolog.
// The marks are keyed on Object.Number. It judges the shape only (no registry):
// StationMarks has already dropped the fixed stars from its candidates.
//
// derivation: Bodies station mark (S/R, S/D)
func StationMarksBetween(now, later []aspect.Position) map[int]Station {
	next := make(map[int]float64, len(later))
	for _, q := range later {
		if q.Object != (aspect.Object{}) {
			next[q.Object.Number] = q.Speed
		}
	}
	var marks map[int]Station
	for _, c := range now {
		if !stationShape(c) {
			continue
		}
		speed, ok := next[c.Object.Number]
		if !ok {
			continue
		}
		var mark Station
		switch trendOf(c.Speed, speed) {
		case trendFalling:
			mark = StationRetro
		case trendRising:
			mark = StationDirect
		}
		if mark != StationNone {
			if marks == nil {
				marks = make(map[int]Station)
			}
			marks[c.Object.Number] = mark
		}
	}
	return marks
}
