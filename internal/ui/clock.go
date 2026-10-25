package ui

import "time"

// Clock is the ambient-state seam for "now" inside the ui package. The
// shipped binary uses RealClock (time.Now passthrough); tests inject
// FrozenClock(t) to make snapshot rendering deterministic.
//
// Provenance: MINDSET.md §"no ambient state" — production code may not
// read time.Now() directly. The Clock interface lives on Model and is the
// only seam through which time enters the ui package.
type Clock interface {
	Now() time.Time
}

// RealClock is the shipping default — time.Now() passthrough.
type RealClock struct{}

// Now returns time.Now() in the local timezone.
func (RealClock) Now() time.Time { return time.Now() }

// FrozenClock returns a Clock fixed at t. Used by invariants tests so the
// first-frame golden does not rotate every second.
func FrozenClock(t time.Time) Clock { return frozenClock{t: t} }

type frozenClock struct{ t time.Time }

func (f frozenClock) Now() time.Time { return f.t }
