package aspect

import (
	"fmt"
	"strings"
)

// registry.go carries the shape of one row of the user's Astrolog aspect
// registry. The PARSE lives in internal/astrolog, which is the only package
// allowed to run the binary; the TYPE lives here because `internal/ui` renders
// these rows and may not import an active package (the isolation invariant).
// Keeping the vocabulary passive is what lets both sides speak it.

// RegistryAspect is one row of `astrolog -HA`.
type RegistryAspect struct {
	// Kind is the aspect's identity: Astrolog's own registry index (1-based,
	// ordered by importance; it is what the `-RA0` band emits), the name and
	// the three letters its lists print. The orb and angle are not part of
	// who an aspect is, so they sit beside it.
	Kind AspectKind
	// Angle is the separation in degrees, as printed — rounded to two
	// decimals, so the harmonic aspects read 51.43, 102.86, 130.91.
	Angle float64
	// Orb is the EFFECTIVE orb. Zero means the aspect cannot be reported at
	// all, whatever the restriction band says.
	Orb float64
	// InForce reports whether the binary COUNTS this aspect in the state
	// `-HA` was run in, read from that output's trailing "Unrestricted
	// aspects" section. It is what the host's own astrolog.as has decided,
	// and it is the set the chooser must open on when the operator has not
	// overridden anything.
	//
	// It is orthogonal to Orb in both directions. An aspect can be
	// unrestricted with an effective orb of zero — `-Ao 5 0 -HA` prints the
	// sextile in the unrestricted section as `covers 60 to 60 - span 0` —
	// and it can carry a wide orb while being restricted out, which is the
	// ordinary case: a default astrolog.as counts five of twenty-four and
	// every one of the other nineteen still prints a positive orb.
	//
	// Reading the orb instead of this field is how the chooser came to claim
	// the host counted all twenty-four: the first toggle then
	// confirmed that claim as an explicit policy and silently widened the
	// chart the operator was looking at.
	InForce bool
}

// CountsAnything reports whether selecting this aspect could actually produce a
// row. An orb of zero is how a host astrolog.as disables an aspect, and it
// survives every restriction switch, so a chooser that does not surface it
// shows a ticked row that yields nothing.
func (a RegistryAspect) CountsAnything() bool { return a.Orb > 0 }

// AspectRegistry is the binary's aspect list as the reader of `astrolog -HA`
// built it, in Astrolog's order. It is the only source of an aspect's
// identity and travels with every call beside the object registry. The zero
// value is a registry not yet read.
type AspectRegistry struct {
	rows []RegistryAspect
	// unreadable holds the rows whose letters the reader of the aspect lists
	// cannot match (see NewAspectRegistryChecked).
	unreadable []RegistryAspect
}

// NewAspectRegistry holds rows as the registry, in the order given. The slice
// is copied.
func NewAspectRegistry(rows []RegistryAspect) AspectRegistry {
	return AspectRegistry{rows: append([]RegistryAspect(nil), rows...)}
}

// NewAspectRegistryChecked is NewAspectRegistry that also records which rows
// have letters the lists cannot be read with: fits says whether a row's three
// letters have the form every Astrolog list prints. A user may rename an
// aspect's letters to any text of three or more characters; the registry holds
// the rename, and the rows it does not fit are what Unreadable and Refusal name.
func NewAspectRegistryChecked(rows []RegistryAspect, fits func(letters string) bool) AspectRegistry {
	r := NewAspectRegistry(rows)
	for _, a := range r.rows {
		if !fits(a.Kind.Abbrev) {
			r.unreadable = append(r.unreadable, a)
		}
	}
	return r
}

// Unreadable returns the aspects whose letters do not fit the form the lists
// are read with, in Astrolog's order; none for a registry that has no such row.
func (r AspectRegistry) Unreadable() []RegistryAspect { return r.unreadable }

// Refusal is the sentence every aspect list shows in place of its rows while
// Unreadable is not empty, and "" otherwise. It is the one text: a list never
// shows rows with an aspect missing, it shows this.
func (r AspectRegistry) Refusal() string { return refusalText(r.unreadable...) }

// RefusalFor is Refusal for the one aspect with Astrolog index n: "" when that
// aspect's letters fit, else the sentence naming only it. A search that needs
// one aspect (the cycle search needs the conjunction) refuses only for it.
func (r AspectRegistry) RefusalFor(n int) string {
	for _, a := range r.unreadable {
		if a.Kind.Index == n {
			return refusalText(a)
		}
	}
	return ""
}

func refusalText(rows ...RegistryAspect) string {
	if len(rows) == 0 {
		return ""
	}
	var parts []string
	for _, a := range rows {
		parts = append(parts, fmt.Sprintf("%s is renamed %q", a.Kind.Name, a.Kind.Abbrev))
	}
	list := parts[0]
	if n := len(parts); n > 1 {
		list = strings.Join(parts[:n-1], ", ") + " and " + parts[n-1]
	}
	return "Can't read the aspect list. " + list + " in your Astrolog settings."
}

// Ready reports whether the registry was read: false for the zero value.
func (r AspectRegistry) Ready() bool { return len(r.rows) > 0 }

// Rows returns the registry's rows in Astrolog's order. The caller must not
// change them.
func (r AspectRegistry) Rows() []RegistryAspect { return r.rows }

// ByIndex returns the row whose Astrolog index is n.
func (r AspectRegistry) ByIndex(n int) (RegistryAspect, bool) {
	for _, a := range r.rows {
		if a.Kind.Index == n {
			return a, true
		}
	}
	return RegistryAspect{}, false
}

// ByAbbrev returns the row whose three letters are exactly abbrev. Letters
// that two rows share resolve to nothing: never a guess.
func (r AspectRegistry) ByAbbrev(abbrev string) (RegistryAspect, bool) {
	var found RegistryAspect
	n := 0
	for _, a := range r.rows {
		if a.Kind.Abbrev == abbrev {
			found = a
			n++
		}
	}
	return found, n == 1
}
