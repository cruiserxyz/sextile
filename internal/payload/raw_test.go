package payload

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// requirement C10: RawOutput and CrossAspectOutput expose Commands — ready-to-paste
// command lines, in spawn order — and nothing that could interpret them.
// payload carries; it has no behaviour of its own, which the method-set
// check pins so a helper that parses or re-joins a command cannot creep in.
func TestRawOutputCarriesCommands(t *testing.T) {
	raw := RawOutput{Commands: []string{"/usr/bin/astrolog -qa 3 21 1985 14:30 0 0:10W 51:30N -v0 -k", "/usr/bin/astrolog -qa 3 21 1985 14:30 0 0:10W 51:30N -v0 -aa -k"}}
	if len(raw.Commands) != 2 || !strings.Contains(raw.Commands[1], " -aa ") {
		t.Fatalf("RawOutput.Commands did not carry the lines: %q", raw.Commands)
	}
	cross := CrossAspectOutput{Commands: []string{"probe", "relationship"}}
	if want := []string{"probe", "relationship"}; !slices.Equal(cross.Commands, want) {
		t.Fatalf("CrossAspectOutput.Commands = %q, want %q", cross.Commands, want)
	}
	for _, typ := range []reflect.Type{reflect.TypeOf(raw), reflect.TypeOf(cross)} {
		if n := typ.NumMethod() + reflect.PointerTo(typ).NumMethod(); n != 0 {
			t.Errorf("%s has %d methods; payload carries commands and must not interpret them", typ, n)
		}
	}
}
