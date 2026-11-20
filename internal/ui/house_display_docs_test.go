package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"os"
	"strconv"
	"strings"
	"testing"
)

// H5 (docs): the reference page's House display section names each form, its
// width and the comparison rule.
func TestHouseDisplayDocs(t *testing.T) {
	readme, err := os.ReadFile("../../docs/REFERENCE.md")
	if err != nil {
		t.Fatal(err)
	}
	// The House display section says each form by its word and title, the
	// shared width, and the comparison rule, so it cannot drift from the code.
	text := string(readme)
	i := strings.Index(text, "\n### House display\n")
	if i < 0 {
		t.Fatal("the reference page has no House display section")
	}
	sec := text[i+1:]
	if j := strings.Index(sec[len("### House display"):], "\n#"); j >= 0 {
		sec = sec[:len("### House display")+j]
	}
	for _, f := range houseForms {
		for _, want := range []string{"**" + f.title + "**", "`" + f.value + "`"} {
			if !strings.Contains(sec, want) {
				t.Errorf("the House display section lacks %s", want)
			}
		}
	}
	for _, want := range []string{
		"All four fill the same " + strconv.Itoa(panel.HouseColumnWidth) + " cells",
		"the chart's own house faint, the house in the compared chart bold",
		"`house_display:`",
		"`12 1 2 | 3 4 5 | 6 7 8 | 9 10 11`",
		"`1 2 3 | 4 5 6 | 7 8 9 | 10 11 12`",
	} {
		if !strings.Contains(sec, want) {
			t.Errorf("the House display section lacks %q", want)
		}
	}
}
