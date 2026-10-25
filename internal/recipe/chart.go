package recipe

import (
	"fmt"
	"math"
)

// ChartSpec is the shared semantic base of a computation: who/when/where, the
// reference frame, and which bodies participate. One ChartSpec is computed once
// and PROJECTED into different outputs (positions, aspects, wheel image) by
// pairing it with a Projection — only the output-owned tail differs.
//
// Build a ChartSpec only through NewChartSpec, which validates it. A zero-value
// ChartSpec{} is not valid; the compiler rejects specs that did not come from
// the constructor (see internal/astrolog Compile). This is the "parse, don't
// validate" discipline: once a ChartSpec exists, the compiler trusts it.
type ChartSpec struct {
	Source  Source
	Houses  HouseSystem
	Zodiac  Zodiac
	Objects ObjectPolicy

	// Aspects mirrors Objects for the aspect set. Its zero value means "no
	// override", so every caller predating it keeps compiling identically.
	Aspects AspectPolicy
}

// Source is the chart-entry root: the single way a chart's time and place are
// specified. It is a sealed sum type — the unexported isSource marker means the
// variant set is closed to this package, so the compiler's type switch over
// Source is exhaustively known. v0 ships exactly one variant, MomentAtPlace;
// QuickBirth (-qa/-qb/-qc), ChartFile (-i) and Comparison (-r*) roots land
// later as new variants.
type Source interface {
	isSource()
	// Label returns a short human label for diagnostics and plan labelling.
	Label() string
}

// MomentAtPlace is the v0 root: an absolute instant (Julian Day) at a
// geographic location. It is one atom on purpose — astrolog's `-qj` without
// `-zl` is not a meaningful supported chart, and Sextile always computes "a
// moment at a place". Lon is east-positive, Lat is north-positive, both decimal
// degrees.
type MomentAtPlace struct {
	JD  float64
	Lon float64
	Lat float64

	// TZ is the slot's IANA zone, set only for the single-chart compute
	// (the specification C4): the compiler then adds `-zZ <zone> <dst>` so
	// the text Astrolog prints reads the chart's own clock. Empty (the wheel,
	// the comparison probe, a staged chart) means no -zZ and a UT header. An
	// unloadable name is a compile error, never a silent fall back.
	TZ string

	// Name and Place are the chart's label as Astrolog prints it in a report
	// header (`-zi <Name> <Place>`, the specification): the
	// single-chart text calls send them on the argv, a staged chart file in
	// its `-zi` line. Both empty is the unnamed chart. The wheel image and the
	// timeline never carry them.
	Name  string
	Place string
}

func (MomentAtPlace) isSource()     {}
func (MomentAtPlace) Label() string { return "moment-at-place" }

// LocalMomentAtPlace is the same instant at the same place as MomentAtPlace,
// but asks Astrolog to receive it as the place's LOCAL clock time and zone
// (`-qb … ST|DT <zone>`) instead of as a bare Julian Day. The chart it
// computes is identical; only the header Astrolog prints changes, from
// "ST Zone 0W" to the clock the operator's slot line shows. TZ is the IANA
// name the zone and the daylight flag are read from; an unloadable name is a
// compile error, never a silent fall back to UT.
//
// Only the raw report uses it (the specification): positions,
// aspects, the wheel and the timeline stay on MomentAtPlace.
type LocalMomentAtPlace struct {
	JD  float64
	Lon float64
	Lat float64
	TZ  string

	// Name and Place: see MomentAtPlace.
	Name  string
	Place string
}

func (LocalMomentAtPlace) isSource()     {}
func (LocalMomentAtPlace) Label() string { return "local-moment-at-place" }

// Comparison is a two-chart relationship root (astrolog's -r* family): the
// computation is the relationship BETWEEN Primary and Secondary, not either
// chart alone. The compiler lowers it to `-r0 <file0> <file1>` (the two charts
// become file arguments, since the inline -qj form carries a single chart).
//
// v1 restricts both sub-sources to MomentAtPlace or LocalMomentAtPlace — a comparison OF comparisons
// is rejected by Validate. The shared lens on the ChartSpec (Houses/Zodiac AND
// Objects) applies to the relationship chart. Ordering is meaningful: Primary
// is the inner wheel / first aspect operand, Secondary the outer wheel / second
// operand.
//
// The two charts are lowered to astrolog chart files (the only two-chart input
// form), which carry the moment to SECOND precision — full fidelity, not the
// minute granularity used for chart-library display.
type Comparison struct {
	Primary   Source
	Secondary Source
	// Relation says what the two charts are to each other, which decides how
	// Astrolog derives aspect phase (applying/separating). The zero value is
	// Synastry. See Relation.
	Relation Relation
}

// Relation is the kind of two-chart relationship a Comparison expresses. It
// matters for aspect PHASE, not geometry: an orb is the same under either,
// but "applying" needs a notion of motion, and two natal charts have none in
// common (their velocities belong to different epochs).
type Relation int

const (
	// Synastry compares two charts as fixed snapshots; Astrolog's -r0. For
	// phase Astrolog propagates BOTH charts at their own natal velocities — a
	// synthetic metric ("if each continued from its own birth motion"), never
	// a motion that occurred. Sextile does not show phase for it today.
	Synastry Relation = iota
	// TransitToNatal treats Secondary as transiting Primary: Astrolog's -rt,
	// which zeroes Primary's velocities so phase follows the transiting
	// body's motion against a fixed natal position. This is the ordinary
	// transit applying/separating.
	TransitToNatal
)

func (r Relation) valid() bool { return r == Synastry || r == TransitToNatal }

func (Comparison) isSource()     {}
func (Comparison) Label() string { return "comparison" }

// MultiWheel is the multi-ring root (astrolog's -r3..-r6): three to six charts
// drawn as concentric rings, Rings[0] innermost and carrying the houses
// (Astrolog's own legend: "#1: Inner Ring + Houses"). The compiler lowers it to
// `-i <f1> -i2 <f2> … -iN <fN> -rN`, every chart staged as a chart file.
//
// A set of two is not a MultiWheel: it stays a Comparison (Synastry, -r0). A
// MultiWheel is defined for the wheel image only; there is no N-way aspect
// grid in Astrolog (-a over -r3 prints chart 1 x chart 2 only), so every text
// projection refuses it. Each ring is a MomentAtPlace or LocalMomentAtPlace;
// a nested comparison or multi-wheel is rejected by Validate.
type MultiWheel struct {
	Rings []Source
}

// MaxRings and MinRings bound a MultiWheel (Astrolog's -r3..-r6).
const (
	MinRings = 3
	MaxRings = 6
)

func (MultiWheel) isSource()     {}
func (MultiWheel) Label() string { return "multi-wheel" }

// HouseSystem selects astrolog's system of house division. Its value IS
// astrolog's `-c` number (0..21), so Placidus stays 0 and WholeSign is 14.
// Astrolog's own `-c 22` (Null) is not a system Sextile offers.
// the specification C3: the stored names are Astrolog's own header names.
type HouseSystem int

const (
	Placidus HouseSystem = iota // astrolog -c 0 (astrolog default)
	Koch
	Equal
	Campanus
	Meridian
	Regiomontanus
	Porphyry
	Morinus
	Topocentric
	Alcabitius
	Krusinski
	EqualMC
	PullenSR
	PullenSD
	WholeSign // astrolog -c 14; its header reads "Whole"
	Vedic
	Sripati
	Horizon
	APC
	CarterPolarEquatorial
	Sunshine
	SavardA
)

// HouseSystemCount is the number of systems offered: -c 0..21.
const HouseSystemCount = 22

// houseNames are Astrolog's "<Name> houses" header names for -c 0..21. A live
// test (astrolog TestHouseSystemTableMatchesAstrolog) asserts each one against
// the installed binary.
var houseNames = [HouseSystemCount]string{
	"Placidus", "Koch", "Equal", "Campanus", "Meridian", "Regiomontanus",
	"Porphyry", "Morinus", "Topocentric", "Alcabitius", "Krusinski",
	"Equal (MC)", "Pullen (S.Ratio)", "Pullen (S.Delta)", "Whole", "Vedic",
	"Sripati", "Horizon", "APC", "Carter P.Equat.", "Sunshine", "Savard-A",
}

func (h HouseSystem) valid() bool { return h >= 0 && int(h) < HouseSystemCount }

// Valid reports whether h is one of the 22 offered systems.
func (h HouseSystem) Valid() bool { return h.valid() }

// StoredName is Astrolog's header name for h, the name the session file stores.
// It is "" for a value that is not a system.
func (h HouseSystem) StoredName() string {
	if !h.valid() {
		return ""
	}
	return houseNames[h]
}

// ParseHouseSystem matches an Astrolog header name against the verified table.
func ParseHouseSystem(name string) (HouseSystem, bool) {
	for i, n := range houseNames {
		if n == name {
			return HouseSystem(i), true
		}
	}
	return 0, false
}

// Zodiac selects the tropical zodiac or one of Astrolog's ten named ayanamsas
// (the specification C2). Tropical is Astrolog's default; every other
// value is a sidereal zodiac whose offset from Fagan-Bradley lives in one
// table, in internal/astrolog.
type Zodiac int

// The order is C2's: Tropical, then the ten ayanamsas as Astrolog 8.00 lists
// them. Nothing persists the integer: a record stores RecordName.
const (
	Tropical Zodiac = iota
	FaganBradley
	Lahiri
	Deluce
	Raman
	UshaShasi
	Krishnamurti
	DjwhalKhul
	Yukteshwar
	JNBhasin
	GalacticCenter

	// ZodiacCount is the number of zodiacs, Tropical included.
	ZodiacCount int = iota
)

// Sidereal is Fagan-Bradley, Astrolog's own default ayanamsa and the zero point
// every other offset is measured from. It keeps the name callers had before the
// ten were offered.
const Sidereal = FaganBradley

func (z Zodiac) valid() bool { return z >= 0 && int(z) < ZodiacCount }

// Valid reports whether z is one of the eleven zodiacs.
func (z Zodiac) Valid() bool { return z.valid() }

// IsSidereal reports whether z is one of the ten ayanamsas.
func (z Zodiac) IsSidereal() bool { return z.valid() && z != Tropical }

// NewChartSpec validates and returns a ChartSpec. It is the sanctioned way to
// construct one — but because ChartSpec is an exported struct, callers can also
// build one directly, so the compiler re-runs Validate before lowering (a
// zero-value or hand-built spec is caught there). Errors carry a "recipe:"
// prefix.
func NewChartSpec(src Source, houses HouseSystem, zodiac Zodiac, objects ObjectPolicy) (ChartSpec, error) {
	s := ChartSpec{Source: src, Houses: houses, Zodiac: zodiac, Objects: objects}
	if err := s.Validate(); err != nil {
		return ChartSpec{}, err
	}
	return s, nil
}

// Validate reports whether the spec is well-formed: a known Source variant with
// in-range parameters, a known house/zodiac, and an ObjectPolicy that satisfies
// its own invariants. The default Source type case also makes the sealed sum's
// exhaustiveness a runtime guarantee, not just a compile-time one.
func (s ChartSpec) Validate() error {
	if err := validateSource(s.Source, true); err != nil {
		return err
	}
	if !s.Houses.valid() {
		return fmt.Errorf("recipe: ChartSpec: unknown HouseSystem %d", int(s.Houses))
	}
	if !s.Zodiac.valid() {
		return fmt.Errorf("recipe: ChartSpec: unknown Zodiac %d", int(s.Zodiac))
	}
	if err := s.Aspects.validate(); err != nil {
		return err
	}
	return s.Objects.validate()
}

// validateSource validates one Source variant, keeping the sealed-sum
// exhaustiveness check in a single place. allowComparison gates the two-chart
// root: true at the top level, false when recursing into a Comparison's sub-
// sources, so a nested Comparison (a comparison of comparisons) is rejected in
// v1. The default case makes the closed variant set a runtime guarantee.
func validateSource(src Source, allowComparison bool) error {
	if src == nil {
		return fmt.Errorf("recipe: ChartSpec: nil Source")
	}
	switch v := src.(type) {
	case MomentAtPlace:
		return validateMoment(v)
	case LocalMomentAtPlace:
		if v.TZ == "" {
			return fmt.Errorf("recipe: LocalMomentAtPlace: no IANA timezone")
		}
		return validateMoment(MomentAtPlace{JD: v.JD, Lon: v.Lon, Lat: v.Lat})
	case Comparison:
		if !allowComparison {
			return fmt.Errorf("recipe: Comparison: nested Comparison is not supported")
		}
		if err := validateSource(v.Primary, false); err != nil {
			return fmt.Errorf("recipe: Comparison.Primary: %w", err)
		}
		if err := validateSource(v.Secondary, false); err != nil {
			return fmt.Errorf("recipe: Comparison.Secondary: %w", err)
		}
		if !v.Relation.valid() {
			return fmt.Errorf("recipe: Comparison: unknown Relation %d", int(v.Relation))
		}
		return nil
	case MultiWheel:
		if !allowComparison {
			return fmt.Errorf("recipe: MultiWheel: nested MultiWheel is not supported")
		}
		if n := len(v.Rings); n < MinRings || n > MaxRings {
			return fmt.Errorf("recipe: MultiWheel: %d rings, want %d to %d (a pair is a Comparison)", n, MinRings, MaxRings)
		}
		for i, ring := range v.Rings {
			if err := validateSource(ring, false); err != nil {
				return fmt.Errorf("recipe: MultiWheel.Rings[%d]: %w", i, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("recipe: ChartSpec: unknown Source type %T", src)
	}
}

// validateMoment guards the Julian Day and coordinate ranges. The JD bound is
// generous (astrolog's Swiss Ephemeris spans millennia); the point is to reject
// the zero value and NaN/Inf, not to police astronomy.
func validateMoment(m MomentAtPlace) error {
	if math.IsNaN(m.JD) || math.IsInf(m.JD, 0) || m.JD <= 0 {
		return fmt.Errorf("recipe: NewChartSpec: invalid Julian Day %v", m.JD)
	}
	if math.IsNaN(m.Lon) || m.Lon < -180 || m.Lon > 180 {
		return fmt.Errorf("recipe: NewChartSpec: longitude %v out of range [-180,180]", m.Lon)
	}
	if math.IsNaN(m.Lat) || m.Lat < -90 || m.Lat > 90 {
		return fmt.Errorf("recipe: NewChartSpec: latitude %v out of range [-90,90]", m.Lat)
	}
	return nil
}

// zodiacNames are Astrolog's own spellings, in the order of the constants
// (verified against `=s <name>` by the live test in internal/astrolog).
var zodiacNames = [ZodiacCount]string{
	"Tropical", "Fagan-Bradley", "Lahiri", "Deluce", "Raman", "Usha-Shasi",
	"Krishnamurti", "Djwhal Khul", "Yukteshwar", "J.N. Bhasin", "Galactic Center",
}

// Name is the zodiac's display name: "Tropical" or the ayanamsa's name.
func (z Zodiac) Name() string {
	if !z.valid() {
		return ""
	}
	return zodiacNames[z]
}

// RecordName is the form a session record and the preferences carry
// (the specification C6): "tropical" or "sidereal <name>".
func (z Zodiac) RecordName() string {
	switch {
	case z == Tropical:
		return "tropical"
	case z.valid():
		return "sidereal " + zodiacNames[z]
	}
	return ""
}

// ParseZodiacRecord reads RecordName's form back, exactly: the name's case and
// spelling are C2's, and anything else is not a zodiac.
func ParseZodiacRecord(v string) (Zodiac, bool) {
	for z := Zodiac(0); int(z) < ZodiacCount; z++ {
		if z.RecordName() == v {
			return z, true
		}
	}
	return 0, false
}
