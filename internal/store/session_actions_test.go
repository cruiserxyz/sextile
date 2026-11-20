package store

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// actionsFrames are two frames records, a chart's and the floating slot's,
// written after the actions block.
func actionsFrames() []FrameRecord {
	return []FrameRecord{
		{File: "a.as", When: frameWhen, Houses: recipe.Koch, HasHouses: true, From: FrameFromChosen},
		{Floating: true, Zodiac: "sidereal Lahiri", HasZodiac: true},
	}
}

// The titles come back in the order written, newest first, and a title with a
// colon, a quote or an apostrophe reads back as written, in the general file
// and in a named one, which is read byte for byte.
func TestSessionActionsRoundTrip(t *testing.T) {
	sessionSandbox(t)
	want := []string{"Choose zodiac", `Say "now": here`, "a: b", "it's", "Open chart"}
	s := Session{Actions: want, Slots: oneSlot(fixtureChart(), false), FloatingSelected: true, FloatingBodies: []string{"Moon"}, Frames: actionsFrames()}
	if err := SaveSession(s); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	const block = "actions:\n  - Choose zodiac\n  - \"Say \\\"now\\\": here\"\n  - \"a: b\"\n  - \"it's\"\n  - Open chart\n"
	if !strings.Contains(text, block) {
		t.Fatalf("the file lacks the block %q:\n%s", block, text)
	}
	got, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession: ok %v err %v", ok, err)
	}
	if !slices.Equal(got.Actions, want) {
		t.Errorf("Actions = %q, want %q", got.Actions, want)
	}
	if len(got.Slots) != 1 || !got.Slots[0].Usable {
		t.Errorf("the chart after the block did not read back: %+v", got.Slots)
	}
	if !got.FloatingSelected || !slices.Equal(got.FloatingBodies, []string{"Moon"}) {
		t.Errorf("the floating block after it did not read back: %v %q", got.FloatingSelected, got.FloatingBodies)
	}
	if len(got.Frames) != len(s.Frames) || got.DroppedFrames != 0 {
		t.Errorf("frames = %+v (dropped %d %q), want %+v", got.Frames, got.DroppedFrames, got.FrameNotes, s.Frames)
	}

	l, err := SaveNamedSessionAs("recent", s)
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	named, _, err := LoadNamedSession(l.File())
	if err != nil {
		t.Fatalf("the named session did not open: %v", err)
	}
	if !slices.Equal(named.Actions, want) {
		t.Errorf("named Actions = %q, want %q", named.Actions, want)
	}
}

// A file with no block reads as nil and is written back byte for byte: an older
// session file opens and saves unchanged.
func TestSessionWithoutActionsIsUnchanged(t *testing.T) {
	sessionSandbox(t)
	s := Session{Slots: oneSlot(fixtureChart(), true)}
	before := formatSession(s)
	if strings.Contains(before, "actions:") {
		t.Fatalf("a session with no recent commands wrote a block:\n%s", before)
	}
	got, ok, err := parseSessionFile("x", []byte(before))
	if err != nil || !ok {
		t.Fatalf("parse: ok %v err %v", ok, err)
	}
	if got.Actions != nil {
		t.Errorf("Actions = %#v, want nil", got.Actions)
	}
	if after := formatSessionAt(got.Version, got); after != before {
		t.Errorf("the file changed on a read and a write:\nbefore\n%s\nafter\n%s", before, after)
	}
}

// The store keeps every title as written, an unknown one and a repeat
// included; the bare line reads as empty, not nil. The block ends at the first
// line that is not an item, so the keys after it read as before.
func TestSessionActionsReadAsWritten(t *testing.T) {
	for _, c := range []struct {
		block string
		want  []string
	}{
		{"actions:\n  - Gone command\n  - Open chart\n  - Open chart\n", []string{"Gone command", "Open chart", "Open chart"}},
		{"actions:\n", []string{}},
	} {
		text := "version: 3\n" + c.block + "floating:\n  selected: true\n"
		got, ok, err := parseSessionFile("x", []byte(text))
		if err != nil || !ok {
			t.Fatalf("%q: ok %v err %v", c.block, ok, err)
		}
		if got.Actions == nil || !slices.Equal(got.Actions, c.want) {
			t.Errorf("%q read as %#v, want %#v", c.block, got.Actions, c.want)
		}
		if !got.FloatingSelected {
			t.Errorf("%q: the floating block after it was not read", c.block)
		}
		if after := formatSessionAt(got.Version, got); after != text {
			t.Errorf("%q did not write back as read:\n%s", c.block, after)
		}
	}
}
