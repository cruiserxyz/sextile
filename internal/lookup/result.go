package lookup

import (
	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Result is one stateless arrangement of astrolog-served data. cp2's first
// cut carries body positions + aspects — exactly what FirstCallTransits
// returns from a `-qj -zl` chart compute.
//
// Result is a payload-shaped value; lookup functions create it but do not
// mutate it after return. The ui consumes it read-only.
type Result struct {
	Positions []aspect.Position
	// Unavailable names the objects the operator selected that this Astrolog
	// cannot compute. Carried from payload unchanged; the UI reports it once
	// per change rather than once per frame.
	Unavailable  []string
	BodyStyles   astrologstyle.Bodies
	Aspects      []aspect.Aspect
	AspectStyles astrologstyle.Aspects
	// AspectsReport is Astrolog's printed aspect list for this chart (the
	// -aa stdout minus color) — what Aspects were parsed from.
	AspectsReport string
	// Commands is every Astrolog spawn behind this arrangement, one
	// ready-to-paste line each, ordered by the moment each spawn completed.
	// Carried from payload unchanged; present beside the error
	// too when a spawn ran and then failed (C14).
	Commands []string
	// Cusps is this chart's house frame, carried so another chart's bodies
	// can be placed into these houses — the difference between "where is
	// this body in a chart cast for now" and "which of the client's houses
	// is it transiting".
	Cusps aspect.Cusps
	// HousesUsed is the house system Astrolog's own header says it used, ""
	// when it printed none (the specification C7).
	HousesUsed string
}

// Empty reports whether the arrangement has no data to render. An empty
// Result is a legitimate steady state (e.g. the chart-compute path failed
// upstream and the ui chose to render an empty body table rather than an
// error overlay).
func (r Result) Empty() bool { return len(r.Positions) == 0 && len(r.Aspects) == 0 }
