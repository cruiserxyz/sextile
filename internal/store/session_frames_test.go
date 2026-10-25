package store

// the specification C11-C11d: the session file's version-3 `frames:`
// sequence. All chart values are constructed synthetic test inputs.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

var frameWhen = time.Date(1995, 3, 20, 10, 0, 0, 0, time.UTC)

func framesFile(t *testing.T, body string) Session {
	t.Helper()
	dir := sessionSandbox(t)
	writeSessionText(t, dir, "version: 3\nfloating:\n  selected: false\n"+body, 0o644)
	s, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession = %v, %v", ok, err)
	}
	return s
}

const goodEntry = "  - file: good.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n"

func TestSessionV3Frames(t *testing.T) {
	dir := sessionSandbox(t)
	in := listSession()
	in.Frames = []FrameRecord{
		{File: "a.as", When: frameWhen, Houses: recipe.Koch, HasHouses: true, From: FrameFromChosen},
		{File: "b b.as", When: frameWhen, Houses: recipe.WholeSign, HasHouses: true, From: FrameFromFile, Zodiac: "sidereal Lahiri", HasZodiac: true},
		{File: "z.as", When: frameWhen, Zodiac: "sidereal Fagan-Bradley", HasZodiac: true},
		{Floating: true, Houses: recipe.CarterPolarEquatorial, HasHouses: true, From: FrameFromChosen},
	}
	if err := SaveSession(in); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "session.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.HasPrefix(text, "version: 3\n") || strings.Count(text, "\nframes:\n") != 1 {
		t.Fatalf("want version 3 and one frames block:\n%s", text)
	}
	want := "frames:\n" +
		"  - file: a.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n" +
		"  - file: b b.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Whole\n    from: file\n    zodiac: sidereal Lahiri\n" +
		"  - file: z.as\n    when: \"1995-03-20T10:00:00Z\"\n    zodiac: sidereal Fagan-Bradley\n" +
		"  - slot: floating\n    houses: Carter P.Equat.\n    from: chosen\n"
	if !strings.Contains(text, want) {
		t.Errorf("frames block differs from the spec's order; got:\n%s", text)
	}
	got, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("reload: %v %v", ok, err)
	}
	if got.Version != 3 || got.DroppedFrames != 0 || len(got.Frames) != 4 {
		t.Fatalf("Version=%d dropped=%d frames=%d", got.Version, got.DroppedFrames, len(got.Frames))
	}
	for i, w := range in.Frames {
		g := got.Frames[i]
		if g.File != w.File || g.Floating != w.Floating || !g.When.Equal(w.When) || g.HasHouses != w.HasHouses ||
			(w.HasHouses && g.Houses != w.Houses) || g.From != w.From || g.HasZodiac != w.HasZodiac || g.Zodiac != w.Zodiac {
			t.Errorf("frame %d came back %+v, want %+v", i, g, w)
		}
	}
	// A zodiac-only record leaves the houses unset, so C1 resolves them.
	if got.Frames[2].HasHouses {
		t.Error("a zodiac-only record froze the houses")
	}
}

func TestSessionV3FramesDropsBadEntries(t *testing.T) {
	bad := map[string]string{
		"unknown key":      "  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n    colour: red\n",
		"unknown system":   "  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Nonsense\n    from: chosen\n",
		"houses no from":   "  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n",
		"from no houses":   "  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    from: chosen\n",
		"bad from":         "  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: guessed\n",
		"no when":          "  - file: x.as\n    houses: Koch\n    from: chosen\n",
		"bad when":         "  - file: x.as\n    when: yesterday\n    houses: Koch\n    from: chosen\n",
		"neither field":    "  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n",
		"path separator":   "  - file: sub/x.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n",
		"dotdot":           "  - file: ..\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n",
		"empty file":       "  - file: \"\"\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n",
		"floating w/ when": "  - slot: floating\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n",
		"out of order":     "  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    from: chosen\n    houses: Koch\n",
		"stray line":       "  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n  garbage\n    houses: Koch\n    from: chosen\n",
	}
	for name, entry := range bad {
		t.Run(name, func(t *testing.T) {
			s := framesFile(t, "frames:\n"+entry+goodEntry)
			if len(s.Frames) != 1 || s.Frames[0].File != "good.as" {
				t.Fatalf("frames = %+v, want only good.as", s.Frames)
			}
			if s.DroppedFrames != 1 || len(s.FrameNotes) != 1 {
				t.Errorf("DroppedFrames=%d notes=%v, want exactly one drop named", s.DroppedFrames, s.FrameNotes)
			}
		})
	}
	t.Run("unknown zodiac drops the field only", func(t *testing.T) {
		s := framesFile(t, "frames:\n  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n    zodiac: Nonsense\n")
		if len(s.Frames) != 1 || !s.Frames[0].HasHouses || s.Frames[0].Houses != recipe.Koch || s.Frames[0].HasZodiac {
			t.Fatalf("frames = %+v, want the houses kept and the zodiac dropped", s.Frames)
		}
		if s.DroppedFrames != 1 || len(s.FrameNotes) != 1 {
			t.Errorf("DroppedFrames=%d notes=%v", s.DroppedFrames, s.FrameNotes)
		}
	})
}

func TestSessionV3FramesFirstWins(t *testing.T) {
	second := strings.Replace(goodEntry, "Koch", "Equal", 1)
	cases := map[string]string{
		"second frames block": "frames:\n" + goodEntry + "frames:\n  - file: other.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n",
		"same file twice":     "frames:\n" + goodEntry + second,
		"floating twice":      "frames:\n  - slot: floating\n    houses: Koch\n    from: chosen\n  - slot: floating\n    houses: Equal\n    from: chosen\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			s := framesFile(t, body)
			if len(s.Frames) != 1 {
				t.Fatalf("frames = %+v, want exactly the first", s.Frames)
			}
			if s.Frames[0].HasHouses && s.Frames[0].Houses != recipe.Koch {
				t.Errorf("kept %v, the first said Koch", s.Frames[0].Houses)
			}
			if s.DroppedFrames != 1 || len(s.FrameNotes) != 1 {
				t.Errorf("DroppedFrames=%d notes=%v, want the duplicate named", s.DroppedFrames, s.FrameNotes)
			}
		})
	}
}

func TestV2SessionLoadsWithNoFrameRecordsAndItsOriginalVersion(t *testing.T) {
	dir := sessionSandbox(t)
	in := listSession()
	if err := SaveSession(in); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "session.yaml"))
	text := strings.Replace(string(b), "version: 3\n", "version: 2\n", 1) + "frames:\n" + goodEntry
	writeSessionText(t, dir, text, 0o644)
	got, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession = %v, %v", ok, err)
	}
	if got.Version != 2 || len(got.Frames) != 0 || got.DroppedFrames != 0 || len(got.Slots) != 3 {
		t.Errorf("Version=%d frames=%d dropped=%d slots=%d, want a plain version-2 read", got.Version, len(got.Frames), got.DroppedFrames, len(got.Slots))
	}
}

// C11b: a dropped frames entry makes the file count as refused for
// the specification C1.
func TestSessionV3BadFrameIsKeptAside(t *testing.T) {
	dir := sessionSandbox(t)
	text := "version: 3\nfloating:\n  selected: false\nframes:\n" + goodEntry + "  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Nonsense\n    from: chosen\n"
	path := writeSessionText(t, dir, text, 0o640)
	s, _, keep, err := LoadSessionKeepingRefused()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Frames) != 1 || keep.Problem != "" || !keptName.MatchString(keep.Kept) {
		t.Fatalf("frames=%d keep=%+v, want the good record read and the file kept aside", len(s.Frames), keep)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("session.yaml still exists: %v", err)
	}
	// And a clean v3 file is not kept.
	dir = sessionSandbox(t)
	writeSessionText(t, dir, "version: 3\nfloating:\n  selected: false\nframes:\n"+goodEntry, 0o640)
	_, _, keep, err = LoadSessionKeepingRefused()
	if err != nil || keep.Kept != "" || keep.Problem != "" {
		t.Errorf("a clean v3 file was kept: %+v %v", keep, err)
	}
}

// the specification C6: the record's `zodiac:` is `tropical` or
// `sidereal <name>`, the name exactly as C2 spells it; anything else drops that
// field only.
func TestSessionV3Zodiac(t *testing.T) {
	t.Run("all eleven are written and read back as C6 spells them", func(t *testing.T) {
		dir := sessionSandbox(t)
		in := listSession()
		for z := recipe.Zodiac(0); int(z) < recipe.ZodiacCount; z++ {
			in.Frames = append(in.Frames, FrameRecord{File: fmt.Sprintf("z%d.as", int(z)), When: frameWhen, Zodiac: z.RecordName(), HasZodiac: true})
		}
		if err := SaveSession(in); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(dir, "session.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range []string{"    zodiac: tropical\n", "    zodiac: sidereal Lahiri\n", "    zodiac: sidereal Djwhal Khul\n", "    zodiac: sidereal J.N. Bhasin\n", "    zodiac: sidereal Galactic Center\n"} {
			if !strings.Contains(string(b), line) {
				t.Errorf("the file lacks %q:\n%s", line, b)
			}
		}
		got, ok, err := LoadSession()
		if err != nil || !ok || got.DroppedFrames != 0 || len(got.Frames) != recipe.ZodiacCount {
			t.Fatalf("reload: ok=%v err=%v dropped=%d frames=%d", ok, err, got.DroppedFrames, len(got.Frames))
		}
		for i, f := range got.Frames {
			if !f.HasZodiac || f.Zodiac != recipe.Zodiac(i).RecordName() {
				t.Errorf("frame %d came back %+v", i, f)
			}
		}
	})
	t.Run("any other value drops the field, never the entry", func(t *testing.T) {
		for _, v := range []string{"Tropical", "Lahiri", "Fagan-Bradley", "sidereal", "sidereal lahiri", "Sidereal Lahiri", "sidereal Nonsense", "tropical Lahiri", "sidereal  Lahiri", "sidereal Lahiri ayanamsa"} {
			s := framesFile(t, "frames:\n  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n    zodiac: "+v+"\n")
			if len(s.Frames) != 1 || !s.Frames[0].HasHouses || s.Frames[0].Houses != recipe.Koch || s.Frames[0].HasZodiac || s.Frames[0].Zodiac != "" {
				t.Errorf("zodiac %q: frames = %+v, want the houses kept and the zodiac gone", v, s.Frames)
			}
			if s.DroppedFrames != 1 || len(s.FrameNotes) != 1 {
				t.Errorf("zodiac %q: DroppedFrames=%d notes=%v, want one drop named", v, s.DroppedFrames, s.FrameNotes)
			}
		}
	})
	t.Run("a zodiac-only entry with an unknown value has nothing left and goes", func(t *testing.T) {
		s := framesFile(t, "frames:\n  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    zodiac: Lahiri\n"+goodEntry)
		if len(s.Frames) != 1 || s.Frames[0].File != "good.as" {
			t.Errorf("frames = %+v, want only good.as", s.Frames)
		}
	})
	t.Run("the writer will not emit a value the reader would drop", func(t *testing.T) {
		sessionSandbox(t)
		in := listSession()
		in.Frames = []FrameRecord{{File: "a.as", When: frameWhen, Zodiac: "Lahiri", HasZodiac: true}}
		if err := SaveSession(in); err != nil {
			t.Fatal(err)
		}
		got, _, _ := LoadSession()
		if len(got.Frames) != 0 {
			t.Errorf("a bare ayanamsa name was written: %+v", got.Frames)
		}
	})
}

// C6 / house-chooser C11b: a zodiac value that is not exactly the canonical
// `tropical` or `sidereal <name>`, however it is written (quoted, bare, wrong
// case, doubled space), drops the FIELD only. The entry's valid houses still
// apply, and the drop is counted so the file is kept aside with a note.
func TestNonCanonicalZodiacDropsOnlyTheField(t *testing.T) {
	for _, v := range []string{`"Lahiri"`, `Lahiri`, `sidereal lahiri`, `sidereal  Lahiri`, `"sidereal Lahiri"`, `'sidereal Lahiri'`} {
		s := framesFile(t, "frames:\n  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n    zodiac: "+v+"\n"+
			"  - file: y.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Equal\n    from: chosen\n")
		if len(s.Frames) != 2 {
			t.Errorf("zodiac %s: %d entries survive, want 2 (the whole entry was lost): %+v", v, len(s.Frames), s.Frames)
			continue
		}
		f := s.Frames[0]
		if f.File != "x.as" || !f.HasHouses || f.Houses != recipe.Koch || f.HasZodiac || f.Zodiac != "" {
			t.Errorf("zodiac %s: entry = %+v, want houses Koch kept and the zodiac dropped", v, f)
		}
		if s.DroppedFrames != 1 || len(s.FrameNotes) != 1 || !strings.Contains(s.FrameNotes[0], "zodiac") {
			t.Errorf("zodiac %s: DroppedFrames=%d notes=%v, want one drop, named as a zodiac", v, s.DroppedFrames, s.FrameNotes)
		}
	}
}

// A line past the scanner's limit inside the frames block stops the scan:
// the end is counted and said, instead of the block passing for a complete
// short one while the records after the line are gone (transfer
// bugs/20261017T0300Z). LoadSession never gets here, because parseSessionV2
// refuses such a file first, so the parser is asked directly.
func TestFramesBlockEndingEarlyIsSaid(t *testing.T) {
	long := "  - file: " + strings.Repeat("x", sessionMaxLine+2) + "\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n"
	after := strings.Replace(goodEntry, "good.as", "after.as", 1)
	frames, dropped, notes := parseFrames("version: 3\nfloating:\n  selected: false\nframes:\n" + goodEntry + long + after)
	if len(frames) != 1 || frames[0].File != "good.as" {
		t.Fatalf("frames = %+v, want only good.as (the records after the long line cannot be read)", frames)
	}
	if dropped != 1 || len(notes) != 1 || !strings.Contains(notes[0], "ended early") || !strings.Contains(notes[0], "not read") {
		t.Errorf("dropped=%d notes=%v, want the early end counted once and said", dropped, notes)
	}
	// The same block without the long line reads whole: the end is not said.
	frames, dropped, notes = parseFrames("version: 3\nfloating:\n  selected: false\nframes:\n" + goodEntry + after)
	if len(frames) != 2 || dropped != 0 || len(notes) != 0 {
		t.Errorf("the whole block: frames=%d dropped=%d notes=%v", len(frames), dropped, notes)
	}
	// A long line in a later, unrelated block is not the frames block's
	// problem: the block read whole and nothing is said about it.
	frames, dropped, notes = parseFrames("version: 3\nfloating:\n  selected: false\nframes:\n" + goodEntry + after + "future:\n  - note: " + strings.Repeat("y", sessionMaxLine+2) + "\n")
	if len(frames) != 2 || dropped != 0 || len(notes) != 0 {
		t.Errorf("a long line after the block: frames=%d dropped=%d notes=%v", len(frames), dropped, notes)
	}
}
