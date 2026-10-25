package ui

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cruiserxyz/sextile/internal/location"
)

// The step 1 — the characterization net for the chassis lift.
//
// `atlas_modal.go` is about to give up its filtered list, cursor and search
// editor to a shared fuzzy-checklist chassis that the aspect chooser and the
// body selector will both bind (the specification C8a,
// docs/99-archive/the specification). Before the lift, atlas was proven by exactly
// two behavioural tests and NO golden, so a rendering regression had nothing
// to fail against.
//
// These goldens are frozen against the PRE-EXTRACTION implementation and are
// the acceptance for the lift: after atlas is rebound to the chassis they must
// be byte-identical. If they cannot be, the extraction is wrong and the
// chassis changes — never the golden.
//
// They pin `renderAtlasModal` rather than a whole frame on purpose: the lift
// changes this function and its state type, so a pure-function golden fails
// exactly when the extraction changes rendering, with no unrelated frame
// noise. Every branch of renderResultList is covered — loading, load error,
// unfiltered hint, no matches, matches, cursor placement, truncation — plus
// both the Unicode and ASCII border/separator branches.
//
// Regenerate ONLY on a deliberate restyle: `go test ./internal/ui -update`.

// liftPlaces is a fixed, deterministic atlas. Names are chosen so "ber"
// matches several entries and "zzz" matches none.
func liftPlaces() []location.PlaceInfo {
	return []location.PlaceInfo{
		{Name: "Berlin", Country: "DE", Lat: 52.52, Lon: 13.405, TZ: "Europe/Berlin"},
		{Name: "Bern", Country: "CH", Lat: 46.948, Lon: 7.447, TZ: "Europe/Zurich"},
		{Name: "Bergen", Country: "NO", Lat: 60.39, Lon: 5.324, TZ: "Europe/Oslo"},
		{Name: "Canberra", Country: "AU", Lat: -35.28, Lon: 149.13, TZ: "Australia/Sydney"},
		{Name: "Amsterdam", Country: "NL", Lat: 52.37, Lon: 4.89, TZ: "Europe/Amsterdam"},
		{Name: "Reykjavik", Country: "IS", Lat: 64.146, Lon: -21.94, TZ: "Atlantic/Reykjavik"},
	}
}

// liftState builds a state through the REAL refilter(), so the goldens
// characterize the shipped pipeline and not a hand-assembled match list.
func liftState(filter string, cursor int) atlasModalState {
	// filterText is promoted from the embedded chassis, so it is assigned
	// rather than set in the literal — Go does not allow a promoted field in
	// a composite literal.
	st := atlasModalState{index: location.NewIndex(liftPlaces())}
	st.filterText = filter
	st.refilter()
	st.cursor = cursor
	return st
}

// liftCappedState drives Filter past maxAtlasResults (200) so state.truncated
// is set by the real cap rather than by hand.
//
// NOTE, and it is the reason this case is named "capped" and not "truncated":
// the "…%d more" hint in renderResultList is UNREACHABLE in any real terminal.
// It is gated on `state.truncated && len(lines) < height`, but the rows are
// built from `visible := matches[:height]`, so len(lines) == height exactly
// when the list was capped. The hint can only appear if the viewport is taller
// than the 200-row cap. atlas_modal.go's own comment says "The truncation hint
// surfaces to the user"; it does not.
//
// This golden therefore pins the CURRENT behaviour — a capped list fills the
// viewport and says nothing about the rows below — because that is what a
// characterization net is for. It is deliberately not fixed here: fixing it
// would move the baseline the lift is measured against. Recorded for the
// operator instead.
func liftCappedState() atlasModalState {
	places := make([]location.PlaceInfo, 0, 250)
	for i := 0; i < 250; i++ {
		places = append(places, location.PlaceInfo{
			Name:    fmt.Sprintf("Cityville %03d", i),
			Country: "XX",
			TZ:      "UTC",
		})
	}
	st := atlasModalState{index: location.NewIndex(places)}
	st.filterText = "cityville"
	st.refilter()
	return st
}

var (
	liftAccessible = Config{Accessible: true}
	liftColor      = Config{Color: ColorAlways}
)

// The three states the build order names, at both widths and in both modes.
func TestAtlasModalGolden(t *testing.T) {
	// Without this, lipgloss downgrades to the ASCII profile off a TTY and
	// strips every style, so the "-color" goldens would carry no SGR at all
	// and would differ from their accessible twins only in box glyphs. The
	// border colour is the ONE thing that differs between this modal's box
	// and the chart picker's ("10" green vs "12" blue), so a colourless
	// golden is blind to exactly the regression the extraction risks.
	forceColor(t)

	states := []struct {
		name  string
		state atlasModalState
	}{
		{"rest", liftState("", 0)},
		{"filtered", liftState("ber", 0)},
		{"empty", liftState("zzz", 0)},
	}
	sizes := []struct {
		label string
		w, h  int
	}{{"80x24", 80, 24}, {"132x40", 132, 40}}
	modes := []struct {
		label string
		cfg   Config
	}{{"accessible", liftAccessible}, {"color", liftColor}}

	for _, st := range states {
		for _, sz := range sizes {
			for _, md := range modes {
				name := fmt.Sprintf("atlas-lift-%s-%s-%s", st.name, sz.label, md.label)
				t.Run(name, func(t *testing.T) {
					checkGolden(t, name, renderAtlasModal(st.state, md.cfg, sz.w, sz.h))
				})
			}
		}
	}
}

// The remaining branches of renderResultList, at one size in accessible mode —
// they are about content, not layout, so a second width buys nothing.
func TestAtlasModalGoldenEdgeStates(t *testing.T) {
	forceColor(t)

	loading := atlasModalState{index: nil}
	loadErr := atlasModalState{index: location.NewIndex(liftPlaces()), loadErr: errors.New("atlas.as not found")}

	cases := []struct {
		name  string
		state atlasModalState
	}{
		{"atlas-lift-cursor-80x24-accessible", liftState("e", 2)},
		{"atlas-lift-capped-80x24-accessible", liftCappedState()},
		{"atlas-lift-loading-80x24-accessible", loading},
		{"atlas-lift-loaderr-80x24-accessible", loadErr},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkGolden(t, tc.name, renderAtlasModal(tc.state, liftAccessible, 80, 24))
		})
	}
}
