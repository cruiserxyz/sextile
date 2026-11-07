package aspect

// CrossAspect is an aspect BETWEEN two charts — astrolog's -r0 relationship
// output, where each aspect pairs a body from the first chart with a body from
// the second. It is deliberately distinct from Aspect:
//
//   - Roles are NOT canonicalized. PrimaryObject is the object from the first
//     (inner/left) chart, SecondaryObject from the second (outer/right) chart;
//     swapping them would change meaning, so there is no number-order
//     reordering. A consuming layer (e.g. a transit-over-natal overlay) maps
//     PrimaryObject->natal and SecondaryObject->transit.
//   - Applying is Astrolog's own phase verdict, read from the `a`/`s` orb
//     marker the -aa aspect list prints. Its meaning depends on the
//     relation the list was computed under: -rt (transit to natal) zeroes the
//     first chart's velocities so only the second chart's motion counts; -r0
//     (synastry) propagates both charts at their own natal velocities. A list
//     printed without -aa (plain +/- orbs) leaves Applying false.
type CrossAspect struct {
	Kind     AspectKind // the angular relationship
	Orb      Orb        // unsigned magnitude of the deviation from exact
	Power    float64    // astrolog's aspect strength (the -r0 "power:" column)
	Applying bool       // astrolog's -aa marker: `a` applying, `s` separating

	// PrimaryObject and SecondaryObject are Astrolog's objects for the two
	// sides, resolved through the registry: the first chart's (inner wheel /
	// left operand) and the second's (outer wheel / right operand); the zero
	// Object for a side it could not resolve.
	PrimaryObject   Object
	SecondaryObject Object
}
