package aspect

import "testing"

// C14: every Body other than BodyUnknown round-trips String() -> ParseBody.
// The enumeration here is written out against the constants rather than
// derived the way ParseBody derives it, so a body added to the enum and
// forgotten by String() fails here instead of quietly parsing as unknown.
func TestBodyStringParseRoundTrip(t *testing.T) {
	for b := Sun; b <= ImumCoeli; b++ {
		name := b.String()
		if name == BodyUnknown.String() {
			t.Errorf("Body(%d) has no name; String() must name every body the session file can carry", b)
			continue
		}
		got, ok := ParseBody(name)
		if !ok {
			t.Errorf("ParseBody(%q) refused a name String() produced", name)
			continue
		}
		if got != b {
			t.Errorf("ParseBody(%q) = %v, want %v", name, got, b)
		}
	}
}

// A name no body answers to is refused rather than guessed at: the session
// file's body lists come off disk, and a mistyped one must be skipped and
// named, not silently turned into the Sun.
func TestParseBodyRefusesUnknownNames(t *testing.T) {
	for _, name := range []string{"", "unknown", "Sun", "sun ", "eris", "north node"} {
		if got, ok := ParseBody(name); ok {
			t.Errorf("ParseBody(%q) = %v, true; want refusal", name, got)
		}
	}
}
