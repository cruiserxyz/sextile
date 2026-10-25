package ir

import "github.com/cruiserxyz/sextile/pkg/aspect"

// IRPacket is the ir package's output and the astrolog package's input. It
// fully specifies the scope and limits of the computation astrolog will run,
// along with the concrete invocation to run.
//
// Invariant: the IR must fully specify scope and limits of computation —
// focus, include-lists, aspect types, orb bounds. The astrolog package has
// capability but no agency; if the IR does not bound scope, astrolog
// floods lookup.
//
// Field semantics:
//
//   - Focus names the higher-level frame — a baked lookup_rules id such as
//     "relationship_patterns". The lookup uses Focus during retrieval; the
//     astrolog ignores it.
//   - Include lists the bodies the astrolog should emit data for. An empty
//     slice means "whatever the command template produces"; the ir is
//     responsible for populating this when the intent narrows scope.
//   - Aspects lists the aspect kinds the computation should report. Same
//     semantics as Include.
//   - OrbLimit caps orbs reported (degrees). Zero means "use astrolog's
//     configured default"; non-zero means the ir read a store
//     preference and overrode.
//   - Command carries the resolved astrolog invocation.
type IRPacket struct {
	Focus    string
	Include  []aspect.Body
	Aspects  []aspect.AspectKind
	OrbLimit float64
	Command  CommandSpec
}
