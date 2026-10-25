package astrolog

import "context"

// invoke is the tests' way to start the (real or fake) binary with a bare argv.
// It is an exempt call on purpose and lives in a _test file: production code has
// no []string-only entry point (TestAstrologSpawnSeamIsTyped scans the non-test
// sources).
func invoke(ctx context.Context, argv []string) (string, string, error) {
	return spawn(ctx, newExempt("test helper: drives the binary with a hand-written argv", argv...))
}

// testCall is a body call over a hand-written argv with the tokenless issued
// band, for the tests that drive Run and RunTimelineSearch directly. Production
// code makes a BodyCall only where the argv and its band are emitted together.
func testCall(argv []string) BodyCall { return newBodyCall(argv, newBand()) }
