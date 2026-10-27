package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The taste lane (its specification § Taste lane). The
// frames below are the contact-sheet renders the operator approved; the
// goldens freeze that pick. Regenerate with `go test ./internal/ui -update`
// only after a deliberate restyle.

// tasteOwnAspects / tasteCrossAspects give the panel enough rows to show the
// waterfall shape at both widths.
func tasteOwnAspects() []aspect.Aspect {
	mk := func(a, b aspect.Body, k aspect.AspectKind, orb, power float64, applying bool) aspect.Aspect {
		asp := aspect.NewAspect(a, b, k, aspect.Orb(orb))
		asp.Power = power
		asp.Applying = applying
		return asp
	}
	return []aspect.Aspect{
		mk(aspect.Sun, aspect.Moon, aspect.Square, 0.5, 22.0, true),
		mk(aspect.Moon, aspect.Saturn, aspect.Opposition, 1.8, 14.5, false),
		mk(aspect.Sun, aspect.Mars, aspect.Trine, 2.1, 11.2, true),
		mk(aspect.Venus, aspect.Jupiter, aspect.Sextile, 3.0, 8.4, false),
		mk(aspect.Mercury, aspect.Neptune, aspect.Square, 4.2, 6.1, true),
		mk(aspect.Mars, aspect.Pluto, aspect.Trine, 5.5, 4.3, false),
	}
}

func tasteCrossAspects() []lookup.OverlayAspect {
	return []lookup.OverlayAspect{
		{Transit: aspect.Saturn, Natal: aspect.Sun, Kind: aspect.Square, Orb: 0.5, Power: 9.5, Applying: true},
		{Transit: aspect.Jupiter, Natal: aspect.Moon, Kind: aspect.Trine, Orb: 1.4, Power: 7.2},
		{Transit: aspect.Mars, Natal: aspect.Mercury, Kind: aspect.Sextile, Orb: 2.2, Power: 5.1, Applying: true},
		{Transit: aspect.Venus, Natal: aspect.Venus, Kind: aspect.Conjunction, Orb: 3.1, Power: 3.8},
	}
}

func tastePositions() []aspect.Position {
	return []aspect.Position{
		{Body: aspect.Sun, Longitude: 35.5, Speed: 0.97, House: 10},
		{Body: aspect.Moon, Longitude: 136.18, Speed: 1.05, House: 1},
		{Body: aspect.Mercury, Longitude: 15.3, Speed: -1.1, House: 9},
		{Body: aspect.Midheaven, Longitude: 52.0, House: 10},
		{Body: aspect.Ascendant, Longitude: 142.0, House: 1},
	}
}

func tasteOverlayFrame() FirstFrameReadyMsg {
	return FirstFrameReadyMsg{
		Overlay:      true,
		Positions:    tastePositions(),
		Aspects:      tasteOwnAspects(),
		CrossAspects: tasteCrossAspects(),
	}
}

// overflowStripTabs is the V3 synthetic 8-tab strip (titles 6–14 cells).
var overflowStripTabs = []Tab{
	{ID: "aspects", Title: "Aspects"},
	{ID: "cross", Title: "Transit→Natal"},
	{ID: "houses", Title: "Houses"},
	{ID: "dignities", Title: "Dignities"},
	{ID: "stars", Title: "Fixed Stars"},
	{ID: "asteroids", Title: "Asteroids"},
	{ID: "midpoints", Title: "Midpoints"},
	{ID: "parts", Title: "Arabic Parts"},
}

// renderOverflowSheet renders the V3 strip lines: widths 66/40/30 × active
// 0/3/7, through renderTopBorder in accessible mode.
func renderOverflowSheet() string {
	var lines []string
	for _, width := range []int{66, 40, 30} {
		for _, active := range []int{0, 3, 7} {
			strip := tabStrip{prefix: "[3] ", tabs: overflowStripTabs, active: active}
			lines = append(lines, renderTopBorder(width, strip, "240", "252", false, Config{Accessible: true}, "+", "+", "-"))
		}
	}
	return strings.Join(lines, "\n")
}

// V3 — overflow.
func TestTabsTasteOverflow(t *testing.T) {
	checkGolden(t, "tabs-overflow", renderOverflowSheet())
}

// renderPairSequence drives the real startup stream at 144x40 accessible and
// returns the four V4 frames: (a) natal-set startup with the first overlay
// frame committed, (b) after `3` focuses Aspects, (c) after the pair is deselected, the
// pair and its single-chart frame commits, (d) after it is reselected and the
// overlay frame commits.
func renderPairSequence(t *testing.T) map[string]string {
	t.Helper()
	berlin := location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"}
	birth := location.RawPlace{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin", LatPresent: true, LonPresent: true}
	var tm tea.Model = NewModelWithClock(Config{Accessible: true}, FrozenClock(frozenInstantForGolden()))
	step := func(msg tea.Msg) {
		tm, _ = tm.Update(msg)
	}
	step(tea.WindowSizeMsg{Width: 144, Height: 40})
	step(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlin})})
	step(ingest.StartupPerimeterReadMsg{
		LocationCandidate: location.Candidate{Source: "stored-location", Place: birth},
		Session:           v1(&store.SessionChart{ID: "fixture-one", Name: "Constructed One", When: time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC), Place: berlin}, true, true, nil, nil),
		HasSession:        true,
		FolderID:          testFolderID,
	})
	setPair := func(on bool) {
		m, _ := setPairSelection(tm.(Model), on)
		tm = m
	}
	frames := map[string]string{}
	commit := func(frame FirstFrameReadyMsg) {
		m := tm.(Model)
		st := m.refresh[surfaceText]
		frame.RunID, frame.WantID = st.runningRunID, st.wantID
		frame.TransitTime = m.targetFloatingTime
		step(frame)
		tm = withSceneColumns(tm.(Model))
	}
	commit(tasteOverlayFrame())
	frames["a"] = tm.View()
	// `3` focuses Aspects without changing what it shows (C9).
	step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	frames["b"] = tm.View()
	setPair(false) // `t` is retired (the specification C4): the same change, made directly
	commit(FirstFrameReadyMsg{Positions: tastePositions(), Aspects: tasteOwnAspects()})
	frames["c"] = tm.View()
	setPair(true)
	commit(tasteOverlayFrame())
	frames["d"] = tm.View()
	return frames
}

// V4 — the comparison title appearing and disappearing.
func TestTabsTastePairSequence(t *testing.T) {
	frames := renderPairSequence(t)
	// "d" is byte-identical to "b", so it has no golden of its own; its
	// title is asserted below.
	for _, k := range []string{"a", "b", "c"} {
		t.Run(k, func(t *testing.T) {
			checkGolden(t, "tabs-pair-sequence-"+k, frames[k])
		})
	}
	// The frames tell the story the spec asks for.
	// Amended by C1 (2026-10-04): `Aspects` opens the strip with the pair after
	// it, and by the specification C1: with the cursor on
	// now, `Aspects` itself is the active tab and the pair is not compared.
	if !strings.Contains(frames["a"], "[3] [Aspects] - 2 in 1-") {
		t.Error("(a) an overlay frame must title the panel with `Aspects` active and its pair, 2 in 1, after it (selected-charts-compare C1)")
	}
	if !strings.Contains(frames["b"], "[3] [Aspects] - 2 in 1-") {
		t.Error("(b) focusing Aspects must not change what it shows")
	}
	if !strings.Contains(frames["c"], "[3] Aspects----") || strings.Contains(frames["c"], "2 in 1") {
		t.Error("(c) deselecting the pair must return the plain Aspects title")
	}
	if !strings.Contains(frames["d"], "[3] [Aspects] - 2 in 1-") {
		t.Error("(d) reselecting the pair must bring the pair title back")
	}
}
