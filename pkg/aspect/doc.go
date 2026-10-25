// Package aspect defines the astrology-domain types spoken by the binary's
// cross-package surface and by the baked tables under internal/baked/. This is the only
// public package in the module; internal/ packages may import it, but it
// must never import any internal/ package.
//
// The vocabulary here mirrors astrolog's semantic terms — bodies, signs,
// houses, aspect kinds. Each enum names its astrolog truth source in its
// godoc. Values and structs only; no behaviour beyond accessors and the
// handful of constructors that enforce an invariant.
package aspect
