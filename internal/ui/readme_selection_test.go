package ui

import (
	"os"
	"strings"
	"testing"
)

// B8: the reference page says what Space does to the surfaces, in its Keys row
// and in one sentence beside the Bodies description.
func TestReferenceSaysSelectedBodiesShowOnlyThemselves(t *testing.T) {
	b, err := os.ReadFile("../../docs/REFERENCE.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	const row = "| `Space` | Select the focused chart or body. Two selected charts engage comparison. Selected bodies show only themselves on the chart's wheel, aspects and report (nothing selected shows all) |"
	if strings.Count(text, row) != 1 {
		t.Errorf("the reference page does not carry the Space row exactly once:\n%s", row)
	}
	var bullet string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "- **Shows only what you select.**") {
			bullet = line
		}
	}
	if bullet == "" {
		t.Fatal("the reference page has no bullet on showing only what you select")
	}
	for _, want := range []string{
		"the Bodies table keeps every body so you can add to the selection",
		"a body the `Ctrl+B` chooser hides stays hidden",
		"in a comparison the selection does not yet reach the cross surfaces",
	} {
		if !strings.Contains(bullet, want) {
			t.Errorf("the bullet does not say %q", want)
		}
	}
	// One sentence after its bold lead: no full stop but the last one.
	lead := strings.TrimPrefix(bullet, "- **Shows only what you select.**")
	if strings.Contains(strings.TrimSuffix(lead, "."), ". ") || !strings.HasSuffix(lead, ".") {
		t.Errorf("the bullet is not one sentence: %q", lead)
	}
}
