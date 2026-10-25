package ir

// Intent is the user's signal about what they want to see. It originates at
// the UI, is relayed (unparsed, uninterpreted) by the ingest, and binds to
// fragments in the ir. Intent is never generated inside the system;
// it always arrives from the outside.
//
// Kind matches a baked intent_rule id from internal/baked/intent_rules.json;
// the string is the primary key the ir uses to pick a binding rule.
// Args carry any parameters the UI surfaced alongside the kind — e.g. the
// focused chart id, the selected aspect, a numeric range the user dragged.
//
// A zero-value Intent (empty Kind) represents no intent and must never reach
// the ir's bind phase. The UI is expected to emit a non-empty Kind or
// no signal at all.
type Intent struct {
	Kind string
	Args map[string]string
}
