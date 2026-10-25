package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestVelocityBar pins the 4-cell velocity field: angles get no bar, the field
// is always 4 display cells, retrograde marks position 0 with ℞, and a body at
// or above the speed cap fills all three bar cells.
func TestVelocityBar(t *testing.T) {
	if got := velocityBar(1.0, true, false); got != "    " {
		t.Errorf("angle bar = %q, want 4 spaces (angles get no bar)", got)
	}

	for _, sp := range []float64{0, 0.03, 0.5, 1.0, 3.0, -1.2} {
		if w := ansi.StringWidth(velocityBar(sp, false, false)); w != 4 {
			t.Errorf("velocityBar(%v) width = %d, want 4", sp, w)
		}
	}

	if got := velocityBar(-1.0, false, false); !strings.HasPrefix(got, "℞") {
		t.Errorf("retrograde bar should start with ℞, got %q", got)
	}
	if got := velocityBar(1.0, false, false); strings.HasPrefix(got, "℞") {
		t.Errorf("direct bar must not start with ℞, got %q", got)
	}

	if got := velocityBar(3.0, false, false); got != " ███" {
		t.Errorf("at/above cap, bar should be full: got %q, want %q", got, " ███")
	}
}

// TestRenderBarCells pins the partial-block fill across the 3 cells × 8 levels.
func TestRenderBarCells(t *testing.T) {
	cases := map[int]string{
		1:  "▏  ", // one sublevel in the first cell
		8:  "█  ", // first cell full
		16: "██ ", // two cells full
		24: "███", // all full
	}
	for fill, want := range cases {
		if got := renderBarCells(fill); got != want {
			t.Errorf("renderBarCells(%d) = %q, want %q", fill, got, want)
		}
	}
}

// TestBarColor pins the gradient stops for the sqrt-normalised magnitude
// (matching legacy pickColor(normalized, gradient)), including a mid stop so a
// regression back to raw-t indexing would be caught.
func TestBarColor(t *testing.T) {
	if got := barColor(false, 0); string(got) != "255" {
		t.Errorf("direct/slow = %q, want 255 (white)", got)
	}
	if got := barColor(false, 0.5); string(got) != "75" {
		t.Errorf("direct/mid = %q, want 75 (gradient[3] at normalized 0.5)", got)
	}
	if got := barColor(false, 1.0); string(got) != "27" {
		t.Errorf("direct/fast = %q, want 27 (blue)", got)
	}
	if got := barColor(true, 1.0); string(got) != "124" {
		t.Errorf("retro/fast = %q, want 124 (red)", got)
	}
}
