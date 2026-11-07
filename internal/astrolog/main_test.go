package astrolog

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

// TestMain installs the host's own default object set as the availability the
// app has before it casts anything (the startup probe runs first). Without it
// the compiler has no answer to "what does this chart draw" and refuses to
// compile one (slice 8c: there is no typed list to fall back on), and a bare
// host prints its whole default set. A test that wants another answer installs
// its own and restores this one.
func TestMain(m *testing.M) {
	if _, err := exec.LookPath("astrolog"); err == nil {
		if def, err := readMainDefault(context.Background()); err == nil {
			hostDefaultAvailability = Availability{Default: def}
			SetAvailability(hostDefaultAvailability)
		}
	}
	os.Exit(m.Run())
}

// hostDefaultAvailability is what TestMain installed.
var hostDefaultAvailability Availability

// replaySet is the eighteen objects testdata/provenance/positions-k.txt prints,
// in Astrolog's order: the ten planets, Chiron, the nodes, Lilith and the four
// angles. A test that replays that fixture asks for exactly these.
var replaySet = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 16, 17, 18, 22, 25, 28, 31}

// useReplayAvailability makes the host default the replayed fixture's own set
// for the length of the test, so a compiled call asks for what the replay
// prints, and puts the real host default back after.
func useReplayAvailability(t *testing.T) {
	t.Helper()
	SetAvailability(Availability{Default: replaySet})
	t.Cleanup(func() { SetAvailability(hostDefaultAvailability) })
}

// replayObjects, while a test replays testdata/provenance, is what testCall
// asks for: the replay prints replaySet whatever the argv says. The host
// default stays as it was, because the compiler's argv is judged against it.
var replayObjects []int

func useReplayObjects(t *testing.T) {
	t.Helper()
	replayObjects = replaySet
	t.Cleanup(func() { replayObjects = nil })
}
