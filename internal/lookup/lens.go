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

	// HousesChosen and ZodiacChosen say the operator chose this chart's house
	// system or zodiac. For a chart cast from its file (recipe.FileChart) a
	// chosen setting overrides the file's own, and an unchosen one is only the
	// default for a file that names none.
	HousesChosen bool
	ZodiacChosen bool

	// Objects is the body-visibility policy: the baseline profile plus the typed
	// Add/Remove deltas and the form of the Nodes and Lilith. The compiler alone
	// turns this into astrolog `-R` and `=Yn`/`=Ynn` switches.
	Objects recipe.ObjectPolicy

	// Aspects is which aspects the chart counts. It is deliberately NOT part
	// of any named lens: `Hellenistic` leaves it zero, which means "no
	// override" and lets the host's own astrolog.as setting stand. The Model
	// supplies it per call from the operator's choice, which is what keeps
	// the chosen set ephemeral Model state rather than a property of a lens
	// (the specification). The day a lens
	// registry exists, a default can move here without the chooser changing.
	Aspects recipe.AspectPolicy

	// Registry is the binary's object list, set by the Model once it has read
	// it. Every call built from the lens carries it, so the parsers name each
	// printed row by Astrolog's own object.
	Registry aspect.Registry

	// AspectRegistry is the binary's aspect list, set by the Model once it has
	// read it, and carried by every call built from the lens beside Registry.
	AspectRegistry aspect.AspectRegistry
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
		// Chiron (11), the South Node (17), Lilith (18), the Nadir (25) and the
		// Descendant (28): Astrolog's own numbers for the five the lens adds.
		Add: recipe.NewObjectSet(11, 17, 18, 25, 28),
	},
}

// specAt assembles the validated recipe.ChartSpec for a moment (Julian Day `jd`)
// At is one chart as a spec takes it: its moment and place, the label its
// reports print, and, for a chart read from its file, that file's record.
type At struct {
	JD     float64
	Place  location.PlaceInfo
	Label  store.ReportLabel
	Record string
}

// AtChart is the At of an anchored chart: a chart that holds its file's record
// is cast from that record.
func AtChart(c store.AnchorChart) At {
	return At{JD: astrolog.SlotJD(c.When), Place: c.Place, Label: c.Label, Record: c.Record}
}

// AtMoment is the At of a moment at a place with no file behind it: Now, a
// transit, a probe.
func AtMoment(jd float64, place location.PlaceInfo, label store.ReportLabel) At {
	return At{JD: jd, Place: place, Label: label}
}

// sourceForm says how a moment at a place is written for Astrolog when the
// chart has no record: in UT, in UT with its zone for the header, or as the
// place's local clock (the raw report).
type sourceForm int

const (
	formUT sourceForm = iota
	formZoned
	formLocal
)

// source is the one door from a chart to Astrolog's source. A chart that holds
// its file's record IS that record (recipe.FileChart): Astrolog reads the
// file itself, so its moment, zone, place and own settings are the file's,
// exact by construction, and form does not apply. Every other chart is a
// moment at a place, written in form.
func (l Lens) source(a At, form sourceForm) recipe.Source {
	if a.Record != "" {
		return recipe.FileChart{
			Record: a.Record, JD: a.JD, Lon: a.Place.Lon, Lat: a.Place.Lat,
			Name: a.Label.Name, Place: a.Label.Place,
			HousesChosen: l.HousesChosen, ZodiacChosen: l.ZodiacChosen,
		}
	}
	if a.Place.TZ == "" {
		// A chart file's place before its file is read: the instant at the
		// coordinates, which needs no zone.
		form = formUT
	}
	switch form {
	case formZoned:
		return recipe.MomentAtPlace{JD: a.JD, Lon: a.Place.Lon, Lat: a.Place.Lat, TZ: a.Place.TZ, Name: a.Label.Name, Place: a.Label.Place}
	case formLocal:
		return recipe.LocalMomentAtPlace{JD: a.JD, Lon: a.Place.Lon, Lat: a.Place.Lat, TZ: a.Place.TZ, Name: a.Label.Name, Place: a.Label.Place}
	}
	return recipe.MomentAtPlace{JD: a.JD, Lon: a.Place.Lon, Lat: a.Place.Lat}
}

// spec is the validated recipe.ChartSpec for src under this lens.
func (l Lens) spec(src recipe.Source) (recipe.ChartSpec, error) {
	spec, err := recipe.NewChartSpec(src, l.Houses, l.Zodiac, l.Objects)
	if err != nil {
		return recipe.ChartSpec{}, err
	}
	spec.Aspects = l.Aspects
	spec.Registry = l.Registry
	spec.AspectRegistry = l.AspectRegistry
	return spec, spec.Validate()
}

// specAt assembles the validated recipe.ChartSpec for one chart under this
// lens: the lens contributes the reference frame and body policy, the chart
// its source. The wheel takes it.
func (l Lens) specAt(a At) (recipe.ChartSpec, error) {
	return l.spec(l.source(a, formUT))
}

// chartSpecAt is specAt for the single-chart compute (positions and the aspect
// text Details/Aspects reads): a moment at a place also carries the slot's IANA
// zone, so Astrolog's header reads the chart's own clock (the specification
// C4), and an unloadable zone refuses here, before any spawn. A chart cast from
// its file carries the file's own zone.
func (l Lens) chartSpecAt(a At) (recipe.ChartSpec, error) {
	if a.Record == "" && a.Place.TZ != "" {
		if _, err := time.LoadLocation(a.Place.TZ); err != nil {
			return recipe.ChartSpec{}, fmt.Errorf("timezone %q does not load: %w", a.Place.TZ, err)
		}
	}
	return l.spec(l.source(a, formZoned))
}

// localSpecAt is specAt for the raw report: a moment at a place is handed to
// Astrolog as the place's local clock and zone, so the report header names the
// slot's own time (the specification).
func (l Lens) localSpecAt(a At) (recipe.ChartSpec, error) {
	return l.spec(l.source(a, formLocal))
}

// localComparisonSpecAt is comparisonSpecAt for the pair raw report: both
// staged charts carry their own local clock and zone. The cross-aspect list
// and the wheel image keep comparisonSpecAt and its UT files.
func (l Lens) localComparisonSpecAt(natal, transit At) (recipe.ChartSpec, error) {
	return l.spec(recipe.Comparison{Primary: l.source(natal, formLocal), Secondary: l.source(transit, formLocal), Relation: recipe.TransitToNatal})
}

// comparisonSpecAt assembles the validated two-chart recipe.ChartSpec for a
// natal chart compared against a transit moment under this lens — the input
// astrolog's -r* relationship path consumes. Primary = natal (first operand,
// drawn by Astrolog on the bi-wheel's outer ring with its houses), Secondary =
// transit (the inner ring), Relation = TransitToNatal so aspect phase follows
// the transiting body against the fixed natal (astrolog -rt); the lens
// contributes the shared reference frame + body policy applied to both charts.
func (l Lens) comparisonSpecAt(natal, transit At) (recipe.ChartSpec, error) {
	return l.spec(recipe.Comparison{Primary: l.source(natal, formUT), Secondary: l.source(transit, formUT), Relation: recipe.TransitToNatal})
}

// synastrySpecAt is comparisonSpecAt for two ANCHORED charts: the same two-chart
// input with Relation Synastry (astrolog -r0), chart 1 the frame (Primary,
// first operand, the bi-wheel's outer ring with its houses) and chart 2 the
// other selected slot (the inner ring). Neither chart transits the other, so no
// velocity is zeroed and the aspect list is Astrolog's plain relationship list
// (its `a`/`s` is the synthetic both-natal-motions metric, never shown for a
// pair).
func (l Lens) synastrySpecAt(one, two At) (recipe.ChartSpec, error) {
	return l.spec(recipe.Comparison{Primary: l.source(one, formUT), Secondary: l.source(two, formUT), Relation: recipe.Synastry})
}

// localSynastrySpecAt is synastrySpecAt for the pair raw report: both staged
// charts carry their own local clock and zone, as localComparisonSpecAt's do.
func (l Lens) localSynastrySpecAt(one, two At) (recipe.ChartSpec, error) {
	return l.spec(recipe.Comparison{Primary: l.source(one, formLocal), Secondary: l.source(two, formLocal), Relation: recipe.Synastry})
}

// multiWheelSpecAt is the validated N-ring spec for the selected set in
// numeral order: rings[0] is chart 1, the innermost ring that carries the
// houses. Three to six rings; a pair is a comparison, not a multi-wheel.
func (l Lens) multiWheelSpecAt(charts []At) (recipe.ChartSpec, error) {
	rings := make([]recipe.Source, len(charts))
	for i, c := range charts {
		rings[i] = l.source(c, formUT)
	}
	return l.spec(recipe.MultiWheel{Rings: rings})
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

// VisibleObjectNumbers is the lens's drawn set as Astrolog object numbers: the
// objects the call really draws on this host (astrolog.ArgvObjectNumbers, the
// one rule for it), the ones with no glyph that the host enables included and an object
// without its ephemeris file left out. It fails while the host's default
// object set is not known.
//
// It exists for the body selector, which must seed its checklist from what the
// chart is drawing NOW rather than from every object the binary carries: the
// registry holds 134 objects and the shipped lens draws eighteen, so a
// checklist that opened with everything ticked would turn the operator's first
// keystroke into a 116-body chart. A tick shows what is drawn, so the first
// change of a tick changes one object and no other. The host's availability
// lives in internal/astrolog, which internal/ui may not import, so the seam is
// here.
func (l Lens) VisibleObjectNumbers() ([]int, error) {
	return astrolog.ArgvObjectNumbers(l.Objects)
}

// DrainedObjects is every object of the lens's Only narrowing, when none of
// them is drawn (no ephemeris file, or not in the chart's set): the chart then
// draws the lens's baseline instead (see astrolog.DrainedObjects).
func (l Lens) DrainedObjects() ([]int, error) {
	return astrolog.DrainedObjects(l.Objects)
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
