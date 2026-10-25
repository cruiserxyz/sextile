// Package payload defines the data-plane shapes — values in motion.
// RawOutput flows from the astrolog package to the lookup package; Card
// flows from lookup to ui.
//
// Invariants: payload transforms nothing; it does not decide who should
// receive what; it is one-way. The types here carry data only; they do not
// carry control signals (that is the events package) and they do not hold
// persistent state (that is the store package). A control-plane enum
// riding in a payload field, or a persistent marker clinging to a Card,
// means the planes have bled into one another and the code needs to move.
package payload
