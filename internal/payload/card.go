package payload

import "github.com/cruiserxyz/sextile/pkg/aspect"

// Card is one unit of rendered meaning. The lookup package produces a Card
// by applying a baked lookup_rule to the RawOutput; the ui renders it into
// the UI surface.
//
// Rule: one rule → one card. The lookup package does not compose new cards
// from retrieved ones. Each Card was scored by a named score function in
// the baked scoring registry; the lookup never recomputes Score at render
// time, and the ui never rescores.
//
// Fields:
//
//   - Title and Text are the card template's resolved strings — placeholder
//     substitution from the matched Aspect has already happened upstream.
//   - Hooks are the baked categorical tags the bake pipeline attached to the
//     lookup_rule; the ui uses them for perimeter-side filtering and the
//     store reads them for preference-driven dampening. Hooks never drive
//     selection inside the lookup package.
//   - Score is the card's ranking value, already computed by the named
//     scoring function at lookup-retrieval time.
//   - Refs cite the aspects this card was produced from, so the ui can
//     cross-reference back to raw data if the user expands a card.
type Card struct {
	Title string
	Text  string
	Hooks []string
	Score float64
	Refs  []aspect.Aspect
}
