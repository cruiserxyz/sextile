package ui

import (
	"fmt"
	"strings"
	"testing"
)

// C50 (leader spec): the key bar is not where a leader is discovered — the
// Keys popup is — so the leader adds nothing to the main screen's bar at rest,
// at any width or in any colour mode. The right-hand indicator slot is empty
// at rest, which is what makes that true. The golden was re-frozen once, when
// design piece 4 rebuilt the bar from the main table
// (the specification); regenerating it with -update is a
// deliberate act, not a way past a failure.
func TestKeyBarUnchangedByLeader(t *testing.T) {
	all := defaultKeys().mainBindings()
	rest := NewModel(Config{}).available(all) // no Details on screen
	var sb strings.Builder
	for _, width := range []int{80, 132, 200} {
		for _, cfg := range []struct {
			name string
			cfg  Config
		}{
			{"accessible", Config{Accessible: true}},
			{"color-always", Config{Color: ColorAlways}},
			{"color-never", Config{Color: ColorNever}},
		} {
			for _, withDetails := range []bool{false, true} {
				entries := rest
				if withDetails {
					entries = all
				}
				fmt.Fprintf(&sb, "== %d %s details=%v ==\n%s\n", width, cfg.name, withDetails,
					renderKeyBar(width, entries, cfg.cfg, ""))
			}
		}
	}
	got := sb.String()

	checkGolden(t, "keybar-frozen", got)
}
