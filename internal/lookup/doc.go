// Package lookup is the stateless arrangement layer over astrolog-served
// data. It does not modify, compose, or (re)create — it filters, arranges,
// compresses, suppresses, projects.
//
// cp2 lands the first cut: the lens-as-Go-constant Hellenistic profile and
// FirstCallTransits — the natal-UNSET + location-known first-frame query.
// Lens-by-name baking and FirstCallNatalPositions arrive in cp3.
//
// Architectural rules (the architecture rules):
//   - lookup never reads internal/baked directly; all baked data passes
//     through internal/astrolog.
//   - lookup is stateless; no caches, no in-package mutable globals.
//   - lookup never imports ui or store.
package lookup
