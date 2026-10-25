package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/templates"
)

// The reading contract rule the ui keeps (the specification):
// the tab body's two intensities (R8). R12 lives in internal/invariants.

// R8 — the body carries the default intensity and the muted grey, nothing
// else: no bold, no other colour.
func TestTabBodyHasTwoIntensities(t *testing.T) {
	forceColor(t)
	lines := []templates.Line{
		{Label: "elongation", Value: "14°52′ east", Aside: "longitude"},
		{Label: "under the beams", Value: "yes", Aside: "within 17°"},
		{Value: "no", Aside: "within 12°"},
		{Blank: true},
		{Label: "inner conjunction", Value: "Mar 23, 2025   2°39′ Ari"},
	}
	body := strings.Join(templateBodyLines(lines, 54, Config{Color: ColorAlways}), "\n")
	sgr := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	seen := map[string]bool{}
	for _, code := range sgr.FindAllString(body, -1) {
		seen[code] = true
	}
	if !seen["\x1b[38;5;244m"] {
		t.Errorf("the aside is not dimmed: %q", body)
	}
	for code := range seen {
		if code != "\x1b[38;5;244m" && code != "\x1b[0m" {
			t.Errorf("a third intensity in the body: %q", code)
		}
	}
	plain := strings.Join(templateBodyLines(lines, 54, Config{Color: ColorNever}), "\n")
	if sgr.MatchString(plain) {
		t.Errorf("colours off still styles: %q", plain)
	}
}
