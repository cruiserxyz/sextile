// Package events defines the control-plane signal alphabet — Events. Events
// carry signals, never payload; they coordinate packages without moving data.
//
// An Event that begins to carry astrological data has become payload, and
// the two planes collapse. Signal logic (filter, debounce, transform,
// dispatch) does not live in this package — it lives with the package that
// composes those operations.
//
// The package holds type declarations only. Channel plumbing, dispatch, and
// the event-loop orchestrator arrive with sxt-tick; until then the cross-package
// call chain is synchronous and the Event alphabet is the contract a future
// event bus will speak against.
package events
