package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// assertFrameGeometry holds every taste frame to exactly w cells on exactly h
// rows. The indicator and the rule are ANSI-aware overlays onto rendered
// boxes; a frame that grew or lost a cell would tear the whole layout, and no
// golden comparison would say why.
func assertFrameGeometry(t *testing.T, view string, w, h int) {
	t.Helper()
	rows := strings.Split(view, "\n")
	if len(rows) != h {
		t.Errorf("frame is %d rows, want %d", len(rows), h)
	}
	for i, row := range rows {
		if got := ansi.StringWidth(row); got != w {
			t.Errorf("row %d is %d cells, want %d: %q", i, got, w, ansi.Strip(row))
		}
	}
}

// The taste lane its specification
// § Taste lane). Frames render through the real Model.View() with the earlier change
// influence chart (52 lines) in Details and a real 14-planet + 4-angle Bodies
// list, so the clipping is the operator's, not a synthetic one.
//
// Defaults frozen here: thumb `┃` in the panel's border colour, rule `┈`
// border-to-border with ├/┤ tees. `go test ./internal/ui -update` re-freezes
// after a deliberate restyle; TestScrollTasteContactSheet writes the
// alternatives to docs/90-assets/t102-*.txt for the pick.

// scrollTasteModel is a ready pair workspace at w×h with a real chart
// committed and the influence report in Details, scrolled to `offset`.
func scrollTasteModel(t *testing.T, cfg Config, w, h, offset int, focus PanelID) Model {
	t.Helper()
	m := tabsPairModel(t, cfg)
	m.width, m.height = w, h
	m = m.testNote("active city: Berlin, DE (source=stored-location)")
	m.limitedViewShown = true // the first-frame notice is not what this fixture is about
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{
		Positions:    fullChartPositions(),
		Aspects:      tasteOwnAspects(),
		CrossAspects: tasteCrossAspects(),
		Overlay:      true,
	})
	m = withSceneColumns(m)
	m.focused = focus
	// The last planet: the window has scrolled as far as the cursor takes it.
	m.bodyCursorIdx = fullChartPlanets - 1
	m.details = detailsViewState{title: "[0] Chart", body: influenceReport(t), scrollOffset: offset}
	return m
}

// V1 — the thumb on a Details report three screens long: top, middle and the
// last page, where it must touch the bottom of the track.
func TestScrollTasteDetails(t *testing.T) {
	forceColor(t)
	for _, tc := range []struct {
		name   string
		offset int
	}{
		{"scroll-details-top", 0},
		{"scroll-details-mid", 11},
		{"scroll-details-bottom", 22},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := scrollTasteModel(t, Config{Color: ColorAlways}, 144, 40, tc.offset, PanelChart)
			view := m.View()
			assertFrameGeometry(t, view, 144, 40)
			// The un-suffixed scroll-details-*.golden files are frozen as they
			// stood before the Details timeline tab: they are C12's baseline
			// (TestDetailsReportTabIsUnchangedBelowTheStrip), and a golden
			// regenerated in that slice may not serve as its own baseline.
			checkGolden(t, tc.name+"-tabbed", view)
		})
	}
}

// V2 — the rule and the Bodies thumb where the panel actually clips. Bodies
// holds 18 rows; at 132x40 it has 26 inner rows and nothing is hidden, so the
// clipped dual-column frame is 132x24 (V4 freezes the unclipped 132x40 one).
func TestScrollTasteBodiesClipped(t *testing.T) {
	forceColor(t)
	for _, tc := range []struct {
		name string
		w, h int
	}{
		{"scroll-bodies-clipped-132x24", 132, 24},
		{"scroll-bodies-clipped-80x24", 80, 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := scrollTasteModel(t, Config{Color: ColorAlways}, tc.w, tc.h, 11, PanelBodies)
			view := m.View()
			assertFrameGeometry(t, view, tc.w, tc.h)
			checkGolden(t, tc.name, view)
		})
	}
}

// V3 — the no-colour fallbacks: thumb `#`, rule `-`, tees `+`, pure ASCII.
func TestScrollTasteAccessible(t *testing.T) {
	t.Run("scroll-details-mid-accessible", func(t *testing.T) {
		m := scrollTasteModel(t, Config{Accessible: true}, 144, 40, 11, PanelChart)
		view := m.View()
		assertFrameGeometry(t, view, 144, 40)
		checkGolden(t, "scroll-details-mid-accessible-tabbed", view)
	})
	t.Run("scroll-bodies-clipped-132x24-accessible", func(t *testing.T) {
		m := scrollTasteModel(t, Config{Accessible: true}, 132, 24, 11, PanelBodies)
		view := m.View()
		assertFrameGeometry(t, view, 132, 24)
		checkGolden(t, "scroll-bodies-clipped-132x24-accessible", view)
	})
}

// V4 — nothing to scroll: a 23-line report and a Bodies panel showing every
// planet. No thumb anywhere; the Bodies rule stays, because the angle block is
// still pinned.
func TestScrollTasteNothingToScroll(t *testing.T) {
	forceColor(t)
	short := strings.Join(detailsReportLines(influenceReport(t))[:23], "\n")
	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{"scroll-none-color", Config{Color: ColorAlways}},
		{"scroll-none-accessible", Config{Accessible: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := scrollTasteModel(t, tc.cfg, 144, 40, 0, PanelChart)
			m.details.body = short
			view := m.View()
			assertFrameGeometry(t, view, 144, 40)
			for _, glyph := range []string{scrollThumbGlyph, scrollThumbASCII} {
				if strings.Contains(view, glyph) {
					t.Errorf("a frame with nothing hidden drew %q", glyph)
				}
			}
			checkGolden(t, tc.name, view)
		})
	}
}

// TestScrollTasteContactSheet writes the glyph alternatives to
// docs/90-assets/ for the operator's pick. It never fails on content; it is
// the render step of the approval loop.
func TestScrollTasteContactSheet(t *testing.T) {
	if os.Getenv("SEXTILE_TASTE_SHEET") == "" {
		t.Skip("set SEXTILE_TASTE_SHEET=1 to re-render docs/90-assets/t102-*.txt")
	}
	forceColor(t)

	thumbs := []struct{ name, glyph string }{
		{"A (default) — ┃ heavy vertical", "┃"},
		{"B — █ full block", "█"},
	}
	var v1 strings.Builder
	v1.WriteString("requirement V1 — Details scroll thumb, colour mode (Config{Color: ColorAlways}, TrueColor; view with `cat`)\n")
	v1.WriteString("The 52-line an earlier change influence chart in the 30-row Details panel at 132x40: offsets 0, 11 and 22 (the last page).\n")
	v1.WriteString("The thumb replaces the right border glyph and inherits the border colour, so it brightens with focus.\n\n")
	for _, variant := range thumbs {
		restore := scrollThumbGlyph
		scrollThumbGlyph = variant.glyph
		for _, offset := range []int{0, 11, 22} {
			m := scrollTasteModel(t, Config{Color: ColorAlways}, 132, 40, offset, PanelChart)
			v1.WriteString("==== Variant " + variant.name + " | offset " + strconv.Itoa(offset) + " ====\n")
			v1.WriteString(m.View() + "\n\n")
		}
		scrollThumbGlyph = restore
	}
	writeTasteSheet(t, "t102-v1-details-thumb.txt", v1.String())

	rules := []struct{ name, glyph string }{
		{"A (default) — ┈ dotted", "┈"},
		{"B — ─ thin", "─"},
		{"C — ━ heavy", "━"},
	}
	var v2 strings.Builder
	v2.WriteString("requirement V2 — Bodies rule above the pinned angle block, colour mode.\n")
	v2.WriteString("Bodies holds 18 rows (14 planets + 4 angles). At 132x40 it has 26 inner rows and nothing clips,\n")
	v2.WriteString("so the clipped frames are 132x24 and 80x24; the rule renders in both, the thumb only where rows are hidden.\n\n")
	for _, variant := range rules {
		restore := panelRuleGlyph
		panelRuleGlyph = variant.glyph
		for _, size := range []struct{ w, h int }{{132, 24}, {80, 24}, {132, 40}} {
			m := scrollTasteModel(t, Config{Color: ColorAlways}, size.w, size.h, 11, PanelBodies)
			v2.WriteString("==== Variant " + variant.name + " | " + strconv.Itoa(size.w) + "x" + strconv.Itoa(size.h) + " ====\n")
			v2.WriteString(m.View() + "\n\n")
		}
		panelRuleGlyph = restore
	}
	writeTasteSheet(t, "t102-v2-bodies-rule.txt", v2.String())

	var v3 strings.Builder
	v3.WriteString("requirement V3 — no-colour fallbacks (--accessible): thumb `#`, rule `-`, tees `+`, pure ASCII.\n")
	v3.WriteString("Last frame: --color=never, which keeps the Unicode frame — the heavy thumb still reads against `│`.\n\n")
	v3.WriteString("==== accessible | Details offset 11 | 132x40 ====\n")
	v3.WriteString(scrollTasteModel(t, Config{Accessible: true}, 132, 40, 11, PanelChart).View() + "\n\n")
	v3.WriteString("==== accessible | Bodies clipped | 132x24 ====\n")
	v3.WriteString(scrollTasteModel(t, Config{Accessible: true}, 132, 24, 11, PanelBodies).View() + "\n\n")
	v3.WriteString("==== --color=never | Details offset 11 | 132x40 ====\n")
	v3.WriteString(scrollTasteModel(t, Config{Color: ColorNever}, 132, 40, 11, PanelChart).View() + "\n")
	writeTasteSheet(t, "t102-v3-no-colour.txt", v3.String())
}

func writeTasteSheet(t *testing.T, name, body string) {
	t.Helper()
	path := filepath.Join("..", "..", "docs", "90-assets", name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("wrote %s", path)
}
