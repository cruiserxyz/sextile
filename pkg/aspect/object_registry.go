package aspect

// object_registry.go carries the shape of one row of the user's Astrolog object
// registry. Like registry.go for aspects, the PARSE lives in internal/astrolog
// — the only package allowed to run the binary — and the TYPE lives here
// because internal/ui renders these rows and may not import an active package.
//
// The registry is read, not tabulated, for the same reason the aspect one is:
// a user's binary carries 134 objects. A selector bounded by a compiled-in
// table would decide on the operator's behalf, silently, that the others do
// not exist.

// RegistryObject is one row of `astrolog -R1 -HO`.
//
// The `-R1` is load-bearing and is not decoration on the read. `-HO` reports
// only UNRESTRICTED objects, so a read taken while this program's own
// restrictions are in force returns the current selection and nothing else.
// `-R1` unrestricts everything first, so the registry is the binary's full set
// however the host's astrolog.as or a previous switch left it.
type RegistryObject struct {
	// Object carries the registry's identity for the row: Number is
	// Astrolog's own object number and is what the `-R0` band emits; Name is
	// the registry's display name, which may contain spaces ("North Node")
	// and may be abbreviated with a trailing period ("Zeta Retic.",
	// "Galactic C."). The registry is 0-BASED (row zero is Earth) and runs to
	// 133.
	Object

	// Star marks the fifty fixed stars, whose rows carry a different set of
	// columns from the planetary rows. It is derived from the row SHAPE at
	// parse time rather than from a number range, so a binary that orders its
	// registry differently still classifies correctly.
	Star bool
}
