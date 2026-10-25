// Package ir defines the intermediate representation the ir package emits
// and the astrolog package consumes. An IRPacket fully specifies the scope
// of an astrolog computation; its Command field is the concrete invocation
// astrolog will execute. Intent is the user's signal that drove the
// binding.
//
// Types only — no parse, no bind, no run behaviour. Those live in this same
// package but arrive with sxt-bind; the astrolog runtime arrives with
// sxt-liv. Keeping the type surface in one task and the behaviour in later
// tasks lets every downstream package code against a stable contract.
package ir
