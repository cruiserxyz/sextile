package ui

import (
	"strconv"
	"strings"
	"testing"
)

// The taste lane of the specification. What the C rows cannot
// decide — the bar's action words, their order, which entries are in it, and
// how a popup reads with no hint line — the operator decides on these. Under
// -design they are written out for a look; otherwise each is compared with
// its frozen golden.

// V1 — the key bar of each owner at 80 and 144 columns, and the main
// screen's at 60 to show the overflow.
func TestDesignKeysV1(t *testing.T) {
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	for _, c := range keyOwnerCases() {
		for _, size := range [][2]int{{80, 24}, {144, 40}} {
			if c.name != "main" {
				continue // the other owners' bars are pinned by TestKeyBarIsTheOwnersBindings
			}
			m := reachOwner(t, c, cfg, size[0], size[1])
			lines := strings.Split(m.View(), "\n")
			name := "design-keybar-" + strings.ReplaceAll(c.name, " ", "-") + "-" + strconv.Itoa(size[0])
			writeDesignFrame(t, name, lines[len(lines)-1])
		}
	}
}
