// Package astrolog is the single site of astrological computation in the
// shipped binary. It receives an ir.CommandSpec, invokes the astrolog CLI,
// and returns a structured payload.RawOutput. It does not interpret; it
// does not decide.
//
// It honours the switch-toggle invariant recorded in
// the astrolog switch-toggle semantics neuron: every invocation
// must carry one of the safe chart-entry switches (-qa, -qb, -qc) and must
// not contain a naked -b, which would toggle off astrolog.as's Swiss
// Ephemeris baseline and render every body as 0Ari00.
//
// CommandSpec.Argv carries the chart-entry portion — e.g. "-qa <month> <day>
// <year> <time> <zone> <long> <lat>". Run internally invokes the astrolog
// CLI twice per call: once with the argv as-is to collect body positions
// (the default -v chart), and once with "-a" appended to collect the aspect
// list. Both outputs are parsed and combined into a single RawOutput.
// Astrolog's chart-type switches are toggles (per the bake-pipeline neuron) and
// do not stack, which is why a second invocation is needed.
//
// The positions output is checked against the 0Ari00 forbid pattern before
// parsing; a failure returns ErrEphemerisDisabled rather than populating
// RawOutput, so untrustworthy positions never reach the lookup package.
package astrolog
