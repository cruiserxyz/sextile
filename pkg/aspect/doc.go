// Package aspect defines the astrology-domain types spoken by the binary's
// cross-package surface and by the baked tables under internal/baked/. This is the only
// public package in the module; internal/ packages may import it, but it
// must never import any internal/ package.
//
// An object is Astrolog's number and name (Object). A Registry holds the
// objects read from the Astrolog binary in use. Comparing two Objects with ==
// or as map keys compares both fields, so callers take Objects from the
// Registry and do not build them. An aspect is the same: an AspectKind is the
// index, name and three letters of a row of the binary's aspect list, and an
// AspectRegistry holds that list; the package declares no aspect of its own. Values and structs only; no behaviour
// beyond accessors and the handful of constructors that enforce an invariant.
package aspect
