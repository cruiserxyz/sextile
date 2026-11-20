package astrolog

import (
	"context"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// invoke is the tests' way to start the (real or fake) binary with a bare argv.
// It is an exempt call on purpose and lives in a _test file: production code has
// no []string-only entry point.
func invoke(ctx context.Context, argv []string) (string, string, error) {
	return spawn(ctx, newExempt("test helper: drives the binary with a hand-written argv", argv...))
}

// testCall is a body call over a hand-written argv with the tokenless issued
// band, for the tests that drive Run and RunTimelineSearch directly. Production
// code makes a BodyCall only where the argv and its band are emitted together.
//
// It carries the test registry and the objects an argv without a restriction
// asks a bare host for (the installed default set, else the thirteen); a test
// whose argv asks for others uses testCallFor.
func testCall(argv []string) BodyCall {
	if replayObjects != nil {
		return testCallFor(argv, replayObjects...)
	}
	if def := CurrentAvailability().Default; len(def) > 0 {
		return testCallFor(argv, def...)
	}
	return testCallFor(argv, defaultSet...)
}

// testCallFor is testCall for an argv that asks for the given objects.
func testCallFor(argv []string, objects ...int) BodyCall {
	reg, err := loadTestRegistry()
	if err != nil {
		panic(err)
	}
	call := newBodyCall(argv, newBand())
	call.Registry, call.Objects = reg, objects
	call.Aspects = mustTestAspects()
	return call
}

// mustTestAspects is the fixture's aspect registry for a helper with no t.
func mustTestAspects() aspect.AspectRegistry {
	reg, err := loadTestAspects()
	if err != nil {
		panic(err)
	}
	return reg
}
