package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// sampleArgv is a chart call as the spawn records it: the binary's path, `-YQ
// 0`, the AstroExpression-clearing pairs (here a few of the fifty-four, ending
// in the last one), then the chart entry and the rest.
const (
	sampleHead = "/usr/local/bin/astrolog -YQ 0 '-~g' '' '-~a' '' '-~Zd' '' '-~kO' '' '-~5i' ''"
	sampleTail = "-qj 2462084.299467593 -zl 13:25E 52:31N -zZ -1 0 -zi '' '' -c 0 _s _Ys _sr0 :sz -qag 10 15 1582 12:00am 0 0:00E 0:00N '=Yt' _Yd -Yn -R 11 -R 17 -R 18 -R 25 -R 28 '=j0' -k0"
	sampleFull = sampleHead + " " + sampleTail
)

// A5: the display form drops the opening every call shares (the path, -YQ 0
// and the clearing pairs) behind `astrolog … `, and a line still too wide keeps
// its START, ending in the cut mark (operator, 2026-10-24). A line with no
// clearing block shows whole, its path shortened to `astrolog`.
func TestArgvDisplayShort(t *testing.T) {
	whole := []struct{ name, line, want string }{
		{"the sample, given room", sampleFull, "astrolog … " + sampleTail},
		{"a short line", "/usr/local/bin/astrolog -qj 2462084.3 -k0", "astrolog -qj 2462084.3 -k0"},
		{"a path with a space", "'/opt/my tools/astrolog' -qj 2462084.3", "astrolog -qj 2462084.3"},
		{"a path with a quote", `'/o'\''k/astrolog' -qj 1`, "astrolog -qj 1"},
		{"a bare name", "astrolog -k0", "astrolog -k0"},
	}
	for _, c := range whole {
		t.Run(c.name, func(t *testing.T) {
			for _, w := range []int{0, ansi.StringWidth(c.want), 400} {
				if got := argvDisplay(c.line, w, Config{}); got != c.want {
					t.Errorf("argvDisplay(%q, %d)\n got %q\nwant %q", c.line, w, got, c.want)
				}
			}
		})
	}

	t.Run("every width cuts on the right and keeps the head", func(t *testing.T) {
		shown := "astrolog … " + sampleTail
		for w := ansi.StringWidth("astrolog … ") + 2; w < ansi.StringWidth(shown); w++ {
			got := argvDisplay(sampleFull, w, Config{})
			if !strings.HasSuffix(got, "…") {
				t.Fatalf("width %d: %q does not end in the cut mark", w, got)
			}
			if gw := ansi.StringWidth(got); gw > w || gw < w-1 {
				t.Fatalf("width %d: %q is %d cells, want the lane filled", w, got, gw)
			}
			head := strings.TrimSuffix(got, "…")
			if !strings.HasPrefix(shown, head) || !strings.HasPrefix(head, "astrolog … ") || strings.Contains(got, "-YQ") {
				t.Fatalf("width %d: %q is not the start of the command", w, head)
			}
		}
	})

	t.Run("accessible mode uses its own mark", func(t *testing.T) {
		got := argvDisplay(sampleFull, 60, Config{Accessible: true})
		if !strings.HasPrefix(got, "astrolog ... -qj ") || !strings.HasSuffix(got, "...") || ansi.StringWidth(got) > 60 || ansi.StringWidth(got) < 59 {
			t.Errorf("accessible display %q", got)
		}
	})

	t.Run("the console shows the head, filled to its width", func(t *testing.T) {
		m := readyModelWithSlots(t, berlinP)
		m.width, m.height = 144, 40
		m = m.logCommands([]string{sampleFull, "/usr/local/bin/astrolog -qj 1 -k0"})
		m, _ = press(t, m, ",", "l")
		if !m.console.open {
			t.Fatal("the console did not open")
		}
		view := ansi.Strip(m.View())
		if strings.Contains(view, "/usr/local/bin") || strings.Contains(view, "-YQ") {
			t.Errorf("the console shows the opening every call shares:\n%s", view)
		}
		if !strings.Contains(view, "astrolog -qj 1 -k0") {
			t.Errorf("the console does not show the short line whole as `astrolog -qj 1 -k0`:\n%s", view)
		}
		found := false
		for _, row := range strings.Split(view, "\n") {
			if i := strings.Index(row, "astrolog "+cutMark(m.cfg)+" -qj 2462084"); i >= 0 && strings.Count(row[i:], cutMark(m.cfg)) == 2 && !strings.Contains(row, "'=j0' -k0") {
				found = true
			}
		}
		if !found {
			t.Errorf("the console does not show the sample as `astrolog … `, the chart entry on, and the cut mark:\n%s", view)
		}
	})
}

// A5: Enter copies the stored line, byte for byte, not what is displayed.
func TestArgvCopyIsFull(t *testing.T) {
	last, calls, restore := stubYankRecording(t, nil)
	defer restore()
	m := readyModelWithSlots(t, berlinP)
	m.width, m.height = 144, 40
	m = m.logCommands([]string{sampleFull, "/usr/local/bin/astrolog -qj 1 -k0"})
	m, _ = press(t, m, ",", "l")
	for _, want := range []string{sampleFull, "/usr/local/bin/astrolog -qj 1 -k0"} {
		at := -1
		for i, l := range m.console.snapshot {
			if l.text == want {
				at = i
			}
		}
		if at < 0 {
			t.Fatalf("the console holds no line %q", want)
		}
		m.console.cursor = at
		m, _ = tpress(t, m, keyEnter)
		if *last != want {
			t.Errorf("Enter copied %q, want the whole stored line %q", *last, want)
		}
	}
	if *calls != 2 {
		t.Errorf("%d copies, want 2", *calls)
	}
}
