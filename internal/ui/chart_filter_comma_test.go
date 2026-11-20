package ui

import (
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// A chart titled "Kepler, Johannes" stays in the Open chart list for the filter
// text `kepler johannes`: matching skips the comma in a name.
func TestPickerFilterFindsANameWithACommaWithoutIt(t *testing.T) {
	p := chartPickerState{loaded: true, charts: []store.NamedChart{{Name: "Kepler, Johannes"}, {Name: "Goethe"}}}
	p.FilterText = "kepler johannes"
	p.refilter()
	if len(p.Matches) != 1 || p.Matches[0].Name != "Kepler, Johannes" {
		t.Errorf("the filter kept %+v, want Kepler, Johannes alone", p.Matches)
	}
}
