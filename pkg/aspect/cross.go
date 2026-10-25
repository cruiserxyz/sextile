package aspect

// CrossAspect is an aspect BETWEEN two charts — astrolog's -r0 relationship
// output, where each aspect pairs a body from the first chart with a body from
// the second. It is deliberately distinct from Aspect:
//
//   - Roles are NOT canonicalized. Primary is the body from the first
//     (inner/left) chart, Secondary from the second (outer/right) chart;
//     swapping them would change meaning, so there is no A<=B reordering. A
//     consuming layer (e.g. a transit-over-natal overlay) maps Primary->natal
//     and Secondary->transit.
//   - Applying is Astrolog's own phase verdict, read from the `a`/`s` orb
//     marker the -aa aspect list prints. Its meaning depends on the
//     relation the list was computed under: -rt (transit to natal) zeroes the
//     first chart's velocities so only the second chart's motion counts; -r0
//     (synastry) propagates both charts at their own natal velocities. A list
//     printed without -aa (plain +/- orbs) leaves Applying false.
type CrossAspect struct {
	Primary   Body       // body from the first chart (inner wheel / left operand)
	Secondary Body       // body from the second chart (outer wheel / right operand)
	Kind      AspectKind // the angular relationship
	Orb       Orb        // unsigned magnitude of the deviation from exact
	Power     float64    // astrolog's aspect strength (the -r0 "power:" column)
	Applying  bool       // astrolog's -aa marker: `a` applying, `s` separating

	// PrimaryLabel and SecondaryLabel are the names Astrolog printed for a side
	// this enum cannot name, and are empty for every body it can; the left one
	// may be Astrolog's seven-character truncation. The enum stays
	// authoritative: nothing reads a label when its Body is set. Such a side is
	// BodyUnknown: it renders, and it is not addressable as a typed body.
	PrimaryLabel   string
	SecondaryLabel string
}
