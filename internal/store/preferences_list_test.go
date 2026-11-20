package store

import (
	"strings"
	"testing"
)

// These tests seed a hand-edited preferences file with YAML list lines and
// push it through every writer, asserting that every byte outside the part
// the writer owns is kept and that the file reads back.

// The owned block may hold a list, at column zero or indented, under a bare
// header or one that carries only an anchor or a tag. The location write
// regenerates the whole block, so the list goes with it; what comes after the
// block stays, and nothing of the list is left behind the new block.
func TestSetLocationReplacesAListHeldByTheLocationBlock(t *testing.T) {
	const before = "version: 1\n# my notes\nkeys:\n  leader: \";\"\n"
	const after = "\n# the display\ndisplay:\n  focus_color: 10\n"
	lists := map[string]string{
		"column zero": "- name: Berlin\n  tz: Europe/Berlin\n- name: Hamburg\n",
		"indented":    "  - name: Berlin\n    tz: Europe/Berlin\n  - name: Hamburg\n",
		"bare dash":   "-\n  name: Berlin\n-\tname: Hamburg\n",
	}
	headers := map[string]string{
		"bare":   "location:\n",
		"anchor": "location: &saved\n",
		"tag":    "location: !!seq  # by hand\n",
	}
	for hname, header := range headers {
		for lname, list := range lists {
			t.Run(hname+", "+lname, func(t *testing.T) {
				path := prefPath(t)
				seedPreferences(t, path, before+header+list+after)
				if err := SetLocation(tokyo); err != nil {
					t.Fatalf("SetLocation: %v", err)
				}
				got := readPreferences(t, path)
				if want := before + tokyoBlock + after; got != want {
					t.Fatalf("bytes outside the location block changed or the list survived:%s", diffText(got, want))
				}
				assertReadsBack(t, path)
			})
		}
	}
}

// The list sits in a block the location write does not own. Every byte of it
// is kept, and the location block after it is still found and replaced.
func TestSetLocationKeepsAListInAnotherBlock(t *testing.T) {
	const notes = "notes:\n- name: Berlin\n  tz: Europe/Berlin\n- plain\n-\n"
	path := prefPath(t)
	seedPreferences(t, path, versionLine+notes+berlinBlock+"display:\n  focus_color: 10\n")
	if err := SetLocation(tokyo); err != nil {
		t.Fatalf("SetLocation: %v", err)
	}
	got := readPreferences(t, path)
	if want := versionLine + notes + tokyoBlock + "display:\n  focus_color: 10\n"; got != want {
		t.Fatalf("a list outside the location block changed:%s", diffText(got, want))
	}
	assertReadsBack(t, path)
}

// A header that holds a value owns only its own line: a list or an indented
// line under the `version:` line or under a `location:` that holds a value is
// not the block's, and every byte of it is kept.
func TestSetLocationKeepsAListThatIsNotTheBlocks(t *testing.T) {
	cases := map[string]struct{ seed, want string }{
		"under the version line": {
			seed: versionLine + "- keep: this-byte\n" + berlinBlock,
			want: versionLine + "- keep: this-byte\n" + tokyoBlock,
		},
		"under the version line, without a colon": {
			seed: versionLine + "- keep\n  indented: too\n" + berlinBlock,
			want: versionLine + "- keep\n  indented: too\n" + tokyoBlock,
		},
		"under a location with a value": {
			seed: versionLine + "location: Berlin\n- keep: this-byte\n- keep\n",
			want: versionLine + tokyoBlock + "- keep: this-byte\n- keep\n",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			path := prefPath(t)
			seedPreferences(t, path, c.seed)
			if err := SetLocation(tokyo); err != nil {
				t.Fatalf("SetLocation: %v", err)
			}
			if got := readPreferences(t, path); got != c.want {
				t.Fatalf("a list that is not the location block's changed:%s", diffText(got, c.want))
			}
		})
	}
}

// The neighbours of the list line: a column-zero comment and a blank line
// after the block, a quoted key, and a YAML document marker. The block ends
// at the marker, as findBlock's does, so the marker and what follows stay.
func TestSetLocationKeepsTheNeighboursOfABlock(t *testing.T) {
	cases := map[string]struct{ seed, want string }{
		"comment and blank after the block": {
			seed: versionLine + berlinBlock + "\n# the display\n\ndisplay:\n  focus_color: 10\n",
			want: versionLine + tokyoBlock + "\n# the display\n\ndisplay:\n  focus_color: 10\n",
		},
		"quoted key inside the block": {
			seed: versionLine + "location:\n  \"name\": \"Berlin: Mitte\"\n  'tz': Europe/Berlin\n\"display\":\n  focus_color: 10\n",
			want: versionLine + tokyoBlock + "\"display\":\n  focus_color: 10\n",
		},
		"document marker after the block": {
			seed: versionLine + berlinBlock + "---\nkeys:\n  leader: \";\"\n",
			want: versionLine + tokyoBlock + "---\nkeys:\n  leader: \";\"\n",
		},
		"directive after the block": {
			seed: versionLine + berlinBlock + "%YAML 1.2\n---\nkeys:\n  leader: \";\"\n",
			want: versionLine + tokyoBlock + "%YAML 1.2\n---\nkeys:\n  leader: \";\"\n",
		},
		"document end after the block": {
			seed: versionLine + berlinBlock + "... # end\n",
			want: versionLine + tokyoBlock + "... # end\n",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			path := prefPath(t)
			seedPreferences(t, path, c.seed)
			if err := SetLocation(tokyo); err != nil {
				t.Fatalf("SetLocation: %v", err)
			}
			if got := readPreferences(t, path); got != c.want {
				t.Fatalf("a neighbour of the location block changed:%s", diffText(got, c.want))
			}
		})
	}
}

// The other writers that find a block or a line by topLevelKey, each given a
// file with a hand-made list beside the part they own. Every byte of the
// list is kept and the file reads back.
func TestEveryPreferencesWriterKeepsAListBesideIt(t *testing.T) {
	const notes = "notes:\n- name: Berlin\n  tz: Europe/Berlin\n- house_display: number\n- version: 9\n"
	writers := map[string]struct {
		write func() error
		want  string
		reads func(path string) bool
	}{
		"house display": {
			write: func() error { return SetHouseDisplay("quadrants") },
			want:  versionLine + notes + berlinBlock + "house_display: quadrants\n",
			reads: func(path string) bool { return readHouseDisplay(path).Value == "quadrants" },
		},
		"frame": {
			write: func() error { return SetFrame("Koch", "tropical") },
			want:  versionLine + notes + berlinBlock + "frame:\n  houses: Koch\n  zodiac: tropical\n",
			reads: func(path string) bool {
				f := readFrame(path)
				return f.Houses == "Koch" && f.Zodiac == "tropical"
			},
		},
		"experimental": {
			write: func() error { return SetExperimental("moon", true) },
			want:  versionLine + notes + berlinBlock + "experimental:\n  moon: on\n",
			reads: func(path string) bool { return readExperimental(path).On["moon"] },
		},
		"location": {
			write: func() error { return SetLocation(tokyo) },
			want:  versionLine + notes + tokyoBlock,
			reads: func(path string) bool { p, ok := readLocation(path); return ok && p.Name == "Tokyo" },
		},
	}
	for name, w := range writers {
		t.Run(name, func(t *testing.T) {
			path := prefPath(t)
			seedPreferences(t, path, versionLine+notes+berlinBlock)
			if err := w.write(); err != nil {
				t.Fatalf("write: %v", err)
			}
			if got := readPreferences(t, path); got != w.want {
				t.Fatalf("the list beside the owned part changed:%s", diffText(got, w.want))
			}
			assertReadsBack(t, path)
			if !w.reads(path) {
				t.Fatalf("the written value does not read back:\n%s", readPreferences(t, path))
			}
		})
	}
}

// assertReadsBack is the file reading back as Sextile reads it: its
// generation is readable and the location block yields a place.
func assertReadsBack(t *testing.T, path string) {
	t.Helper()
	if _, err := readableSchema(path); err != nil {
		t.Fatalf("the file no longer reads: %v", err)
	}
	p, ok := readLocation(path)
	if !ok || p.Name == "" {
		t.Fatalf("the location does not read back: %+v, %v\n%s", p, ok, readPreferences(t, path))
	}
	if strings.Count(readPreferences(t, path), "\nlocation:\n")+boolInt(strings.HasPrefix(readPreferences(t, path), "location:\n")) != 1 {
		t.Fatalf("the file holds other than one location block:\n%s", readPreferences(t, path))
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
