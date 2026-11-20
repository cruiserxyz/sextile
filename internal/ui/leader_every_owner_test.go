package ui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Every owner that has a leader state: the main screen, attached Details, the
// focused Log and the console beside the lists with a filter and Details' two
// search lines. The command line over each lists exactly the chords it runs.

// leaderOwners are the owners with a leader state. The first three take every
// chord; the rest take only the ones they offer (commandsOver).
func leaderOwners() []overCase {
	all := []overCase{
		{"main", ownerMain, func(t *testing.T) Model {
			m := kbModel(t)
			m.startup = startupReady
			return m
		}, func(Model) string { return "" }},
		{"details", ownerDetails, func(t *testing.T) Model {
			m := onList(t, listTimeline, Config{Accessible: true})
			if m.detailsSearchLineOpen() {
				t.Fatal("fixture: the search line is open")
			}
			return m
		}, func(Model) string { return "" }},
		{"log", ownerLog, func(t *testing.T) Model {
			m := consoleModel(t, 5)
			m = m.logNote("test", "a line for the Log to hold").logNote("test", "and another")
			m, _ = press(t, m, ",", "L")
			if !m.logFocus.active {
				t.Fatal("the Log did not take the focus")
			}
			return m
		}, func(Model) string { return "" }},
	}
	return append(all, overCases()...)
}

// leaderCallers maps each file that arms or runs the leader, read from the
// code, to the owners of leaderOwners it serves.
var leaderCallers = map[string][]string{
	"model.go": {"main", "details", "city", "open chart", "bodies", "aspects", "houses", "zodiac", "nodes",
		"keybindings", "timeline search", "report search", "console"},
	"log_focus.go": {"log"},
}

// chordsThatRun are the letters that change something when pressed after the
// leader over this owner: the chords and the comma forms of Ctrl keys alike.
func chordsThatRun(t *testing.T, c overCase) []string {
	t.Helper()
	var ran []string
	for _, letter := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		ch := struct{ Letter string }{string(letter)}
		copied := captureYank(t)
		m := c.open(t)
		before := overSnapshot(m, c) + fmt.Sprint(m.previewActive)
		m, _ = tpress(t, m, runes(","))
		if !m.leader.pending {
			// Nothing is armed: the letter that follows is plain text, not a chord.
			continue
		}
		m, _ = tpress(t, m, runes(ch.Letter))
		if m.leader.pending {
			continue // the first key of a longer chord
		}
		if overSnapshot(m, c)+fmt.Sprint(m.previewActive) != before || len(*copied) > 0 {
			ran = append(ran, ch.Letter)
		}
	}
	return ran
}

// listedChords are the single-letter chords the comma menu names after the leader.
func listedChords(t *testing.T, c overCase) []string {
	t.Helper()
	m := c.open(t)
	m, _ = tpress(t, m, runes(","))
	if !m.leader.pending {
		return nil
	}
	var out []string
	for _, e := range strings.Split(barOf(m), "  ") {
		if f := strings.Fields(e); len(f) == 2 {
			out = append(out, f[0])
		}
	}
	return out
}

// Over every owner with a leader state, a chord the menu lists runs and a chord
// that runs is listed. Where the owner takes only some of the chords, the two
// sets are the same; where it takes all, every chord that runs is listed.
func TestEveryOwnerListsTheChordsItRuns(t *testing.T) {
	for _, c := range leaderOwners() {
		t.Run(c.name, func(t *testing.T) {
			stubShowWheel(t)
			stubRenderWheelImage(t)
			stubRenderPairWheelImage(t)
			sessionSandbox(t)
			listed := listedChords(t, c)
			ran := chordsThatRun(t, c)
			limited := c.open(t).limitedLeader(c.owner)
			slices.Sort(listed)
			slices.Sort(ran)
			if limited {
				if !slices.Equal(listed, ran) {
					t.Errorf("listed %v, runs %v: a listed chord must run and a running chord must be listed", listed, ran)
				}
				return
			}
			for _, l := range ran {
				if !slices.Contains(listed, l) {
					t.Errorf("the chord %q runs and is not listed (listed %v)", l, listed)
				}
			}
			if len(ran) < 5 {
				t.Errorf("only %d chords ran over %s: %v", len(ran), c.name, ran)
			}
		})
	}
}

// Every file that calls the leader dispatch, read from the code, is known to
// this walk, and every owner it names is walked.
func TestEveryLeaderCallerIsWalked(t *testing.T) {
	walked := map[string]bool{}
	for _, c := range leaderOwners() {
		walked[c.name] = true
	}
	callers := map[string]bool{}
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		f, perr := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if perr != nil {
			return perr
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "armLeader" || sel.Sel.Name == "updateLeaderKey" || sel.Sel.Name == "leaderOverPopup") {
					callers[filepath.ToSlash(p)] = true
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(callers) < 2 {
		t.Fatalf("only %d callers read: %v", len(callers), callers)
	}
	for p := range callers {
		names, ok := leaderCallers[p]
		if !ok {
			t.Errorf("%s calls the leader dispatch and is not in the walk: add its owners to leaderOwners and leaderCallers", p)
		}
		for _, n := range names {
			if !walked[n] {
				t.Errorf("%s serves %q, which leaderOwners does not walk", p, n)
			}
		}
	}
}

// In the comma state `?` closes it and opens Keybindings, over every owner.
func TestQuestionMarkClosesTheCommaStateOverEveryOwner(t *testing.T) {
	for _, c := range leaderOwners() {
		if c.owner == ownerKeybindings {
			continue // `?` closes the popup itself, which is not open over itself
		}
		t.Run(c.name, func(t *testing.T) {
			m := c.open(t)
			m, _ = tpress(t, m, runes(","))
			m, _ = tpress(t, m, runes("?"))
			if !m.keybindingsOpen || m.leader.pending {
				t.Errorf("after `,` then `?`: Keybindings open %v, leader pending %v", m.keybindingsOpen, m.leader.pending)
			}
		})
	}
}

// The key bars of Details and the Log spell the chord from the configured
// leader, not a literal comma.
func TestBarsSpellTheChordFromTheLeader(t *testing.T) {
	for _, c := range leaderOwners() {
		if c.name != "details" && c.name != "log" {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			m := c.open(t)
			if bar := barOf(m); !strings.Contains(bar, ", y") {
				t.Fatalf("fixture: the bar has no `, y` with the default leader: %q", bar)
			}
			m.leaderKey = "x"
			bar := barOf(m)
			if !strings.Contains(bar, "x y") || strings.Contains(bar, ", y") {
				t.Errorf("with leader x the bar reads %q, want `x y` and no comma", bar)
			}
		})
	}
}
