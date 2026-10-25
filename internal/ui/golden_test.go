package ui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

var updateGolden = flag.Bool("update", false, "regenerate golden files")

// digestPath is a review aid written by -update, never committed (.gitignore).
const digestPath = "testdata/DIGEST.txt"

// goldenChange is one golden seen under -update.
type goldenChange struct {
	name     string
	old, new string
	isNew    bool // no golden existed before
}

var (
	goldenMu  sync.Mutex
	goldenLog []goldenChange
)

// checkGolden compares got with testdata/<name>.golden byte for byte. Under
// -update it writes the file instead and records the change for DIGEST.txt.
func checkGolden(t *testing.T, name string, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *updateGolden {
		old, err := os.ReadFile(path)
		existed := err == nil
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if !existed || string(old) != got {
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				t.Fatalf("write golden: %v", err)
			}
		}
		goldenMu.Lock()
		goldenLog = append(goldenLog, goldenChange{name: name, old: string(old), new: got, isNew: !existed})
		goldenMu.Unlock()
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `go test ./internal/ui/ -update` to generate)", path, err)
	}
	if string(want) != got {
		rem, add := lineDelta(string(want), got)
		t.Errorf("golden mismatch for %s (%s)\n%s", name, path, shortDiff(rem, add))
	}
}

// shortDiff shows at most 10 removed and 10 added lines.
func shortDiff(rem, add []string) string {
	var b strings.Builder
	for i, l := range rem {
		if i == 10 {
			fmt.Fprintf(&b, "... %d more removed\n", len(rem)-10)
			break
		}
		fmt.Fprintf(&b, "- %s\n", l)
	}
	for i, l := range add {
		if i == 10 {
			fmt.Fprintf(&b, "... %d more added\n", len(add)-10)
			break
		}
		fmt.Fprintf(&b, "+ %s\n", l)
	}
	if b.Len() == 0 {
		return "(same lines, different order or whitespace/escape bytes)"
	}
	return b.String()
}

// lineDelta returns the lines of old that are not in new and vice versa, as
// multisets, ANSI-stripped, blank lines dropped.
func lineDelta(old, new string) (removed, added []string) {
	count := map[string]int{}
	for _, l := range goldenLines(old) {
		count[l]++
	}
	var newLines []string
	for _, l := range goldenLines(new) {
		if count[l] > 0 {
			count[l]--
			continue
		}
		newLines = append(newLines, l)
	}
	for _, l := range goldenLines(old) {
		if count[l] > 0 {
			count[l]--
			removed = append(removed, l)
		}
	}
	return removed, newLines
}

func goldenLines(s string) []string {
	var out []string
	for _, l := range strings.Split(ansi.Strip(s), "\n") {
		l = strings.TrimRight(l, " ")
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// buildDigest groups the changed lines of many goldens: lines in exactly one
// file first (one-offs), then lines repeated across files by count.
func buildDigest(changes []goldenChange) string {
	var changed, unchanged, fresh int
	type lineKey struct{ sign, text string }
	files := map[lineKey]map[string]bool{}
	var newNames []string
	for _, c := range changes {
		switch {
		case c.isNew:
			fresh++
			newNames = append(newNames, c.name)
			continue
		case c.old == c.new:
			unchanged++
			continue
		}
		changed++
		rem, add := lineDelta(c.old, c.new)
		for _, p := range []struct {
			sign  string
			lines []string
		}{{"-", rem}, {"+", add}} {
			for _, l := range p.lines {
				k := lineKey{p.sign, l}
				if files[k] == nil {
					files[k] = map[string]bool{}
				}
				files[k][c.name] = true
			}
		}
	}
	var b strings.Builder
	if changed == 0 && fresh == 0 {
		return "no golden changed\n"
	}
	fmt.Fprintf(&b, "goldens written: %d  changed: %d  unchanged: %d  new: %d\n", len(changes), changed, unchanged, fresh)
	sort.Strings(newNames)
	for _, n := range newNames {
		fmt.Fprintf(&b, "new: %s\n", n)
	}
	type entry struct {
		key   lineKey
		names []string
	}
	var ones, many []entry
	for k, set := range files {
		e := entry{key: k}
		for n := range set {
			e.names = append(e.names, n)
		}
		sort.Strings(e.names)
		if len(e.names) == 1 {
			ones = append(ones, e)
		} else {
			many = append(many, e)
		}
	}
	sort.Slice(ones, func(i, j int) bool {
		if ones[i].names[0] != ones[j].names[0] {
			return ones[i].names[0] < ones[j].names[0]
		}
		if ones[i].key.sign != ones[j].key.sign {
			return ones[i].key.sign < ones[j].key.sign
		}
		return ones[i].key.text < ones[j].key.text
	})
	sort.Slice(many, func(i, j int) bool {
		if len(many[i].names) != len(many[j].names) {
			return len(many[i].names) > len(many[j].names)
		}
		if many[i].key.sign != many[j].key.sign {
			return many[i].key.sign < many[j].key.sign
		}
		return many[i].key.text < many[j].key.text
	})
	fmt.Fprintf(&b, "\n== one-offs (%d lines, each in exactly one golden) ==\n", len(ones))
	for _, e := range ones {
		fmt.Fprintf(&b, "%s %s   [%s]\n", e.key.sign, e.key.text, e.names[0])
	}
	fmt.Fprintf(&b, "\n== repeated (%d lines, by file count) ==\n", len(many))
	for _, e := range many {
		shown := e.names
		more := ""
		if len(shown) > 5 {
			more = fmt.Sprintf(" +%d more", len(shown)-5)
			shown = shown[:5]
		}
		fmt.Fprintf(&b, "%4d  %s %s   [%s%s]\n", len(e.names), e.key.sign, e.key.text, strings.Join(shown, ", "), more)
	}
	return b.String()
}

// writeGoldenDigest runs after every test, and only under -update.
func writeGoldenDigest() {
	if !*updateGolden {
		return
	}
	goldenMu.Lock()
	changes := append([]goldenChange(nil), goldenLog...)
	goldenMu.Unlock()
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "digest:", err)
		return
	}
	if err := os.WriteFile(digestPath, []byte(buildDigest(changes)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "digest:", err)
	}
}
