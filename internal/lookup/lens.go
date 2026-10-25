package lookup

import (
	"fmt"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Lens is the named context profile a chart calculation runs under: house
// system, zodiac (tropical vs sidereal), and the body policy astrolog applies
// when emitting the chart. It is the source-free part of a recipe.ChartSpec —
// pair it with a moment-at-place (via specAt) to get the full validated spec the
// astrolog compiler consumes.
//
// cp2 ships exactly one Lens (Hellenistic) as a Go constant. cp3+ moves the lens
// registry into internal/baked/lenses.json and introduces the switching gesture
// in Unit #2 — see contract §11 Deferred.
type Lens struct {
	Name string

	// Houses is the typed house-division system (compiles to astrolog's `-c`).
	Houses recipe.HouseSystem

	// Zodiac selects the tropical or sidereal reference frame (sidereal is pinned by the frame band, never a bare `-s`).
	Zodiac recipe.Zodiac

	// Objects is the body-visibility policy: the baseline profile plus the typed
	// Add/Remove deltas and the true-node/Lilith mode. The compiler alone turns
	// this into astrolog `-R`/`-Yn` switches.
	Objects recipe.ObjectPolicy

	// Aspects is which aspects the chart counts. It is deliberately NOT part
	// of any named lens: `Hellenistic` leaves it zero, which means "no
	// override" and lets the host's own astrolog.as setting stand. The Model
	// supplies it per call from the operator's choice, which is what keeps
	// the chosen set ephemeral Model state rather than a property of a lens
	// (the specification). The day a lens
	// registry exists, a default can move here without the chooser changing.
	Aspects recipe.AspectPolicy
}

// Hellenistic is the cp2 default: Whole Sign houses, tropical zodiac, true
// node/Lilith, and the v1 body set that surfaces Chiron, the South Node, Lilith,
// the Imum Coeli and the Descendant on top of astrolog's default-visible set.
//
// Provenance: user-mental-model.md §7 — Hellenistic is the consulting-
// astrologer default Sextile is built for.
var Hellenistic = Lens{
	Name:   "Hellenistic",
	Houses: recipe.WholeSign,
	Zodiac: recipe.Tropical,
	Objects: recipe.ObjectPolicy{
		Baseline: recipe.AstrologDefault,
		Add: recipe.NewBodySet(
			aspect.Chiron, aspect.SouthNode, aspect.Lilith,
			aspect.ImumCoeli, aspect.Descendant,
		),
		TrueNodeMode: true,
	},
}

// specAt assembles the validated recipe.ChartSpec for a moment (Julian Day `jd`)
// at `place` under this lens — the typed semantic input the astrolog compiler
// consumes. It is the single construction site that replaced astrolog's
// specFromPrimitives bridge: the lens contributes the reference frame + body
// policy, the arguments contribute the source.
func (l Lens) specAt(jd float64, place location.PlaceInfo) (recipe.ChartSpec, error) {
	spec, err := recipe.NewChartSpec(
		recipe.MomentAtPlace{JD: jd, Lon: place.Lon, Lat: place.Lat},
		l.Houses, l.Zodiac, l.Objects,
	)
	if err != nil {
		return recipe.ChartSpec{}, err
	}
	spec.Aspects = l.Aspects
	return spec, spec.Validate()
}

// chartSpecAt is specAt for the single-chart compute (positions and the aspect
// text Details/Aspects reads): the source also carries the slot's IANA zone, so
// Astrolog's header reads the chart's own clock (the specification C4). An
// unloadable zone refuses here, before any spawn. The wheel keeps specAt.
func (l Lens) chartSpecAt(jd float64, place location.PlaceInfo, label store.ReportLabel) (recipe.ChartSpec, error) {
	if _, err := time.LoadLocation(place.TZ); err != nil {
		return recipe.ChartSpec{}, fmt.Errorf("timezone %q does not load: %w", place.TZ, err)
	}
	spec, err := recipe.NewChartSpec(
		recipe.MomentAtPlace{JD: jd, Lon: place.Lon, Lat: place.Lat, TZ: place.TZ, Name: label.Name, Place: label.Place},
		l.Houses, l.Zodiac, l.Objects,
	)
	if err != nil {
		return recipe.ChartSpec{}, err
	}
	spec.Aspects = l.Aspects
	return spec, spec.Validate()
}

// localSpecAt is specAt for the raw report: the same instant at the same
// place, handed to Astrolog as the place's local clock and zone so the
// report header names the slot's own time (the specification). Only
// the raw report asks for it; positions, aspects and the wheel keep specAt.
func (l Lens) localSpecAt(jd float64, place location.PlaceInfo, label store.ReportLabel) (recipe.ChartSpec, error) {
	spec, err := recipe.NewChartSpec(
		recipe.LocalMomentAtPlace{JD: jd, Lon: place.Lon, Lat: place.Lat, TZ: place.TZ, Name: label.Name, Place: label.Place},
		l.Houses, l.Zodiac, l.Objects,
	)
	if err != nil {
		return recipe.ChartSpec{}, err
	}
	spec.Aspects = l.Aspects
	return spec, spec.Validate()
}

// localComparisonSpecAt is comparisonSpecAt for the pair raw report: both
// staged charts carry their own local clock and zone. The cross-aspect list
// and the wheel image keep comparisonSpecAt and its UT files.
func (l Lens) localComparisonSpecAt(jdNatal float64, natalPlace location.PlaceInfo, natalLabel store.ReportLabel, jdTransit float64, transitPlace location.PlaceInfo, transitLabel store.ReportLabel) (recipe.ChartSpec, error) {
	spec, err := recipe.NewChartSpec(
		recipe.Comparison{
			Primary:   localMomentAt(jdNatal, natalPlace, natalLabel),
			Secondary: localMomentAt(jdTransit, transitPlace, transitLabel),
			Relation:  recipe.TransitToNatal,
		},
		l.Houses, l.Zodiac, l.Objects,
	)
	if err != nil {
		return recipe.ChartSpec{}, err
	}
	spec.Aspects = l.Aspects
	return spec, spec.Validate()
}

// comparisonSpecAt assembles the validated two-chart recipe.ChartSpec for a
// natal moment (jdNatal at natalPlace) compared against a transit moment
// (jdTransit at transitPlace) under this lens — the input astrolog's -r*
// relationship path consumes. Primary = natal (first operand, drawn by
// Astrolog on the bi-wheel's outer ring with its houses), Secondary = transit
// (second operand, the inner ring), Relation =
// TransitToNatal so aspect phase follows the transiting body against the
// fixed natal (astrolog -rt, an earlier change); the lens contributes the shared
// reference frame + body policy applied to both charts.
func (l Lens) comparisonSpecAt(jdNatal float64, natalPlace location.PlaceInfo, jdTransit float64, transitPlace location.PlaceInfo) (recipe.ChartSpec, error) {
	spec, err := recipe.NewChartSpec(
		recipe.Comparison{
			Primary:   recipe.MomentAtPlace{JD: jdNatal, Lon: natalPlace.Lon, Lat: natalPlace.Lat},
			Secondary: recipe.MomentAtPlace{JD: jdTransit, Lon: transitPlace.Lon, Lat: transitPlace.Lat},
			Relation:  recipe.TransitToNatal,
		},
		l.Houses, l.Zodiac, l.Objects,
	)
	if err != nil {
		return recipe.ChartSpec{}, err
	}
	spec.Aspects = l.Aspects
	return spec, spec.Validate()
}

// synastrySpecAt is comparisonSpecAt for two ANCHORED charts: the same two-chart
// input with Relation Synastry (astrolog -r0), chart 1 the frame (Primary,
// first operand, the bi-wheel's outer ring with its houses) and chart 2 the
// other selected slot (the inner ring). Neither
// chart transits the other, so no velocity is zeroed and the aspect list is
// Astrolog's plain relationship list (its `a`/`s` is the synthetic both-natal-
// motions metric, never shown for a pair).
func (l Lens) synastrySpecAt(jd1 float64, place1 location.PlaceInfo, jd2 float64, place2 location.PlaceInfo) (recipe.ChartSpec, error) {
	spec, err := recipe.NewChartSpec(
		recipe.Comparison{
			Primary:   momentAt(jd1, place1),
			Secondary: momentAt(jd2, place2),
			Relation:  recipe.Synastry,
		},
		l.Houses, l.Zodiac, l.Objects,
	)
	if err != nil {
		return recipe.ChartSpec{}, err
	}
	spec.Aspects = l.Aspects
	return spec, spec.Validate()
}

// localSynastrySpecAt is synastrySpecAt for the pair raw report: both staged
// charts carry their own local clock and zone, as localComparisonSpecAt's do.
func (l Lens) localSynastrySpecAt(jd1 float64, place1 location.PlaceInfo, label1 store.ReportLabel, jd2 float64, place2 location.PlaceInfo, label2 store.ReportLabel) (recipe.ChartSpec, error) {
	spec, err := recipe.NewChartSpec(
		recipe.Comparison{
			Primary:   localMomentAt(jd1, place1, label1),
			Secondary: localMomentAt(jd2, place2, label2),
			Relation:  recipe.Synastry,
		},
		l.Houses, l.Zodiac, l.Objects,
	)
	if err != nil {
		return recipe.ChartSpec{}, err
	}
	spec.Aspects = l.Aspects
	return spec, spec.Validate()
}

// multiWheelSpecAt is the validated N-ring spec for the selected set in
// numeral order: rings[0] is chart 1, the innermost ring that carries the
// houses. Three to six rings; a pair is a comparison, not a multi-wheel.
func (l Lens) multiWheelSpecAt(jds []float64, places []location.PlaceInfo) (recipe.ChartSpec, error) {
	rings := make([]recipe.Source, len(jds))
	for i := range jds {
		rings[i] = momentAt(jds[i], places[i])
	}
	spec, err := recipe.NewChartSpec(recipe.MultiWheel{Rings: rings}, l.Houses, l.Zodiac, l.Objects)
	if err != nil {
		return recipe.ChartSpec{}, err
	}
	spec.Aspects = l.Aspects
	return spec, spec.Validate()
}

func momentAt(jd float64, place location.PlaceInfo) recipe.MomentAtPlace {
	return recipe.MomentAtPlace{JD: jd, Lon: place.Lon, Lat: place.Lat}
}

func localMomentAt(jd float64, place location.PlaceInfo, label store.ReportLabel) recipe.LocalMomentAtPlace {
	return recipe.LocalMomentAtPlace{JD: jd, Lon: place.Lon, Lat: place.Lat, TZ: place.TZ, Name: label.Name, Place: label.Place}
}

// houseNames and houseCodes are the approved display name and two-letter tag
// code of each -c 0..21 system (the specification, "The 22 house
// codes"; TestHouseCodesMatchTheApprovedTable reads that table).
var (
	houseNames = [recipe.HouseSystemCount]string{
		"Placidus", "Koch", "Equal", "Campanus", "Meridian", "Regiomontanus",
		"Porphyry", "Morinus", "Topocentric", "Alcabitius", "Krusinski",
		"Equal (MC)", "Pullen (S.Ratio)", "Pullen (S.Delta)", "Whole Sign",
		"Vedic", "Sripati", "Horizon", "APC", "Carter P.Equat.", "Sunshine",
		"Savard-A",
	}
	houseCodes = [recipe.HouseSystemCount]string{
		"Pl", "Ko", "Eq", "Ca", "Me", "Re", "Po", "Mo", "To", "Al", "Ks",
		"EM", "PR", "PD", "Wh", "Ve", "Sr", "Hz", "AP", "CP", "Su", "Sa",
	}
)

// HouseName is the human-readable house-system label for the chart header.
func (l Lens) HouseName() string { return HouseName(l.Houses) }

// HouseName is the approved display name of a house system.
func HouseName(h recipe.HouseSystem) string {
	if h.Valid() {
		return houseNames[h]
	}
	return fmt.Sprintf("House %d", int(h))
}

// HouseCode is the approved two-letter tag code of a house system ("??" for a
// value that is not one of the 22).
func HouseCode(h recipe.HouseSystem) string {
	if h.Valid() {
		return houseCodes[h]
	}
	return "??"
}

// ZodiacName is the human-readable zodiac label for the chart header:
// "Tropical", or the ayanamsa's own name. A sidereal chart always says which
// ayanamsa it is cast in (the specification C5).
func (l Lens) ZodiacName() string { return l.Zodiac.Name() }

// HouseAbbrev is the 2-char condensed house-system code for the compact slot
// line (e.g. Whole Sign -> "Wh", Placidus -> "Pl"). A value that is not one of
// the 22 systems has no code ("??"); the old "Ho" stand-in is gone.
func (l Lens) HouseAbbrev() string { return HouseCode(l.Houses) }

// zodiacCodes are the eleven approved tag codes, in recipe.Zodiac order
// (the specification, "The 22 house codes and 11 zodiac codes";
// TestZodiacCodesMatchTheApprovedTable reads that table).
var zodiacCodes = [recipe.ZodiacCount]string{
	"Tr", "Fa", "La", "De", "Ra", "Us", "Kr", "Dj", "Yu", "Bh", "Gc",
}

// ZodiacCode is the approved two-letter tag code of a zodiac ("Tr" tropical,
// "La" Lahiri, and so on: one per ayanamsa, never a bare sidereal code; "??"
// for a value that is no zodiac).
func ZodiacCode(z recipe.Zodiac) string {
	if z.Valid() {
		return zodiacCodes[z]
	}
	return "??"
}

// ZodiacAbbrev is the lens's zodiac code. Concatenated with HouseAbbrev it
// forms the slot line's right tag, e.g. "WhTr".
func (l Lens) ZodiacAbbrev() string { return ZodiacCode(l.Zodiac) }

// VisibleObjectNumbers is the lens's drawn set as Astrolog object numbers.
//
// It exists for the body selector, which must seed its checklist from what the
// chart is drawing NOW rather than from every object the binary carries: the
// registry holds 134 objects and the shipped lens draws eighteen, so a
// checklist that opened with everything ticked would turn the operator's first
// keystroke into a 116-body chart. The body→number map lives in
// internal/astrolog, which internal/ui may not import, so the seam is here.
func (l Lens) VisibleObjectNumbers() []int {
	return astrolog.DrawnObjectNumbers(l.Objects)
}

// ZodiacOffsetText is the finder's account of an ayanamsa: its offset from
// Fagan-Bradley, signed and in degrees, as Astrolog's table has it
// (the specification C5). Fagan-Bradley is Astrolog's own default and
// Tropical has no offset to state.
func ZodiacOffsetText(z recipe.Zodiac) string {
	off, ok := astrolog.AyanamsaOffset(z)
	switch {
	case !ok:
		return ""
	case off == 0:
		return "Astrolog's default"
	case off < 0:
		return "Fagan-Bradley \u2212 " + astrolog.OffsetArg(-off) + "\u00b0"
	}
	return "Fagan-Bradley + " + astrolog.OffsetArg(off) + "\u00b0"
}
