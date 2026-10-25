// Package recipe is the semantic input surface for astrolog computation: the
// typed, validated description of WHAT chart to compute and WHICH output to
// take from it. It is the source language the internal/astrolog compiler lowers
// into an astrolog argv.
//
// recipe deliberately speaks in chart semantics — a moment at a place, a house
// system, a zodiac, a set of bodies to show — never in astrolog's single-letter
// switch alphabet. The translation from these semantics to switches (and the
// load-bearing ordering / toggle / last-wins rules astrolog imposes) is the
// compiler's job, and lives in exactly one place there. Keeping the semantics
// here and the syntax in astrolog is what lets ordering safety be structural
// rather than smeared across call sites.
//
// Type surface only: recipe carries no behaviour beyond construction-time
// validation. It imports nothing but pkg/aspect (the public body/aspect leaf).
// internal/lookup constructs recipe values; internal/astrolog consumes them.
//
// "recipe" is canonical Sextile vocabulary for these micro-templates (see
// the architecture rules). The name is load-bearing: a recipe is a reusable, composable
// specification, not a one-shot command.
package recipe
