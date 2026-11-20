package ui

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// referenceMissing is every command of the key table that REFERENCE's Keys
// section does not name by its key: Ctrl and the letter where it has one, else
// the comma and the letters.
func referenceMissing(keysSection string) []string {
	var missing []string
	for _, c := range keys.Commands() {
		want := "`,` `" + c.Letter + "`"
		if c.Then != "" {
			want += " `" + c.Then + "`"
		}
		if _, ok := keys.CommandCtrl(c); ok {
			want = "Ctrl+" + strings.ToUpper(c.Letter)
		}
		if !strings.Contains(keysSection, want) {
			missing = append(missing, want)
		}
	}
	return missing
}

// REFERENCE's Keys section names every command of the key table by its key,
// so the page and the Keybindings popup tell one story.
func TestReferenceNamesEveryCommand(t *testing.T) {
	raw, err := os.ReadFile("../../docs/REFERENCE.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	start := strings.Index(text, "\n## Keys\n")
	end := strings.Index(text[start+1:], "\n## ")
	if start < 0 || end < 0 {
		t.Fatal("REFERENCE has no Keys section")
	}
	if missing := referenceMissing(text[start : start+1+end]); len(missing) > 0 {
		t.Errorf("REFERENCE's Keys section does not name %q", missing)
	}
	// The control: a section that leaves out Ctrl+O is caught.
	if missing := referenceMissing(strings.ReplaceAll(text[start:start+1+end], "Ctrl+O", "")); !slices.Contains(missing, "Ctrl+O") {
		t.Errorf("a Keys section without Ctrl+O passed: %q", missing)
	}
}
