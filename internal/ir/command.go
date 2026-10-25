package ir

// CommandSpec is a single astrolog invocation, resolved from a baked command
// template plus the IRPacket's scope parameters. The astrolog executes Argv
// verbatim; CommandID is kept for diagnostics and cross-reference only.
//
// Argv is the argument vector passed to astrolog — not including the
// executable name. The astrolog decides what program to invoke; the ir
// decides which arguments.
//
// The switch-toggle invariant (provenance: bake pipeline neuron,
// astrolog-switch-toggle-semantics.yaml) requires every real invocation to
// start from the known baseline -qa; that discipline belongs to the baked
// command template the ir resolved, not to this type. CommandSpec
// carries the already-resolved argv and does not re-verify it.
type CommandSpec struct {
	CommandID string
	Argv      []string
}
