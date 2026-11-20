package store

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// N11, the store's half: a session record holds the form of the Nodes and
// Lilith as two words, and a record whose form is not both words of a castable
// pair loses the form alone.
func TestFrameRecordHoldsNodeAndLilith(t *testing.T) {
	dir := sessionSandbox(t)
	in := listSession()
	in.Frames = []FrameRecord{
		{File: "a.as", When: frameWhen, Houses: recipe.Koch, HasHouses: true, From: FrameFromChosen, Nodes: recipe.NodesTrueNatural, HasNodes: true},
		{File: "b.as", When: frameWhen, Nodes: recipe.NodesMeanNatural, HasNodes: true},
		{File: "c.as", When: frameWhen, Zodiac: "tropical", HasZodiac: true, Nodes: recipe.NodesMean, HasNodes: true},
		{Floating: true, Nodes: recipe.NodesTrue, HasNodes: true},
	}
	if err := SaveSession(in); err != nil {
		t.Fatal(err)
	}
	text := readSessionText(t, dir)
	for _, want := range []string{
		"  - file: a.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n    node: true\n    lilith: natural\n",
		"  - file: b.as\n    when: \"1995-03-20T10:00:00Z\"\n    node: mean\n    lilith: natural\n",
		"    zodiac: tropical\n    node: mean\n    lilith: mean\n",
		"  - slot: floating\n    node: true\n    lilith: true\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the frames block lacks %q:\n%s", want, text)
		}
	}
	got, ok, err := LoadSession()
	if err != nil || !ok || got.DroppedFrames != 0 || len(got.Frames) != 4 {
		t.Fatalf("reload: ok %v err %v dropped %d frames %d", ok, err, got.DroppedFrames, len(got.Frames))
	}
	for i, w := range in.Frames {
		if g := got.Frames[i]; g.HasNodes != w.HasNodes || g.Nodes != w.Nodes {
			t.Errorf("frame %d came back with form %v (%v), want %v", i, g.Nodes, g.HasNodes, w.Nodes)
		}
	}

	head := "frames:\n  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n    zodiac: tropical\n"
	tail := "  - file: y.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Equal\n    from: chosen\n"
	for _, tc := range []struct{ name, lines string }{
		{"node alone", "    node: true\n"},
		{"lilith alone", "    lilith: natural\n"},
		{"an unknown Node word", "    node: purple\n    lilith: true\n"},
		{"an unknown Lilith word", "    node: true\n    lilith: purple\n"},
		{"true Node with the mean Lilith, which Astrolog cannot cast", "    node: true\n    lilith: mean\n"},
		{"natural for the Node", "    node: natural\n    lilith: natural\n"},
		{"a quoted word", "    node: \"true\"\n    lilith: true\n"},
	} {
		s := framesFile(t, head+tc.lines+tail)
		if len(s.Frames) != 2 {
			t.Errorf("%s: %d entries survive, want 2: %+v", tc.name, len(s.Frames), s.Frames)
			continue
		}
		f := s.Frames[0]
		if f.HasNodes || f.Nodes != recipe.NodesMean {
			t.Errorf("%s: the form %v (%v) was kept", tc.name, f.Nodes, f.HasNodes)
		}
		if f.File != "x.as" || !f.HasHouses || f.Houses != recipe.Koch || !f.HasZodiac || f.Zodiac != "tropical" {
			t.Errorf("%s: houses and zodiac did not stay: %+v", tc.name, f)
		}
		if s.DroppedFrames != 1 || len(s.FrameNotes) != 1 || !strings.Contains(s.FrameNotes[0], "Nodes and Lilith") {
			t.Errorf("%s: DroppedFrames=%d notes=%v, want one drop named as the form", tc.name, s.DroppedFrames, s.FrameNotes)
		}
	}
	// A record that holds only a form nobody can read has nothing left.
	s := framesFile(t, "frames:\n  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    node: true\n"+tail)
	if len(s.Frames) != 1 || s.Frames[0].File != "y.as" {
		t.Errorf("a record with only an unreadable form survives: %+v", s.Frames)
	}
	// A record with a form and no houses or zodiac is a record.
	s = framesFile(t, "frames:\n  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    node: mean\n    lilith: natural\n")
	if len(s.Frames) != 1 || !s.Frames[0].HasNodes || s.Frames[0].Nodes != recipe.NodesMeanNatural || s.Frames[0].HasHouses {
		t.Errorf("a form-only record: %+v", s.Frames)
	}
}

// N12, the store's half: the settings hold `node:` and `lilith:` under
// `frame:`, read as written and written in words with the other keys.
func TestSetFrameNodesWritesTheFormAndReadsItBack(t *testing.T) {
	path := prefPath(t)
	seedPreferences(t, path, "frame:\n  houses: Koch  # mine\n  lilith: mean\nlater: 1\n")
	if err := SetFrameNodes("Placidus", "tropical", recipe.NodesTrueNatural); err != nil {
		t.Fatal(err)
	}
	want := "frame:\n  houses: Placidus  # mine\n  lilith: natural\n  zodiac: tropical\n  node: true\nlater: 1\n"
	if got := readPreferences(t, path); got != want {
		t.Errorf("preferences:%s", diffText(got, want))
	}
	if got, wantCfg := Frame(), (FrameConfig{Houses: "Placidus", HousesPresent: true, Zodiac: "tropical", ZodiacPresent: true,
		Node: "true", NodePresent: true, Lilith: "natural", LilithPresent: true}); got != wantCfg {
		t.Errorf("read back %+v, want %+v", got, wantCfg)
	}
	if err := SetFrameNodes("Placidus", "tropical", recipe.NodeForm(9)); err == nil {
		t.Error("a form outside the four was written")
	}
	// SetFrame leaves a form that is there alone.
	if err := SetFrame("Koch", "tropical"); err != nil {
		t.Fatal(err)
	}
	if cfg := Frame(); cfg.Node != "true" || cfg.Lilith != "natural" {
		t.Errorf("SetFrame moved the form: %+v", cfg)
	}
}
