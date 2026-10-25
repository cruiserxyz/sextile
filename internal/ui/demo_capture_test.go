package ui

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

var (
	// sgrParamRE captures the parameter body of one SGR sequence.
	sgrParamRE = regexp.MustCompile(`\x1b\[([0-9;]*)m`)
	// astrologSGRRE matches the palette family Astrolog's spans become once
	// they reach the terminal. Astrolog prints bold+basic ("1;31"); termenv
	// renders that as bright ("91"). Sextile's own chrome is 256-colour
	// ("38;5;N") and never lands in this family. Measured on this workflow:
	// 110 such spans with BodyStyles/AspectStyles present, 8 with them
	// stripped -- so the threshold below discriminates rather than decorates.
	astrologSGRRE = regexp.MustCompile(`^(?:[01];)?(?:3[0-7]|9[0-7])$`)
	// aspectEntryRE matches one waterfall cell's orb suffix, the unambiguous
	// end of an entry: "(12°34'A)".
	aspectEntryRE = regexp.MustCompile(`\(\d+°\d{2}'[AS]\)`)
)

// minAstrologSpans is the floor for Astrolog-owned colour in one frame.
const minAstrologSpans = 40

// the demo capture lane.
//
// WR-11's acceptance asks for the smallest deterministic capture path: a fixed
// terminal size, forced colour, fixture-backed state, and a scripted workflow
// yielding a SOURCE artifact first with images as derivatives. This is that
// path, and it deliberately owns everything it touches:
//
//   - It names no `.golden`. The demo may use internal/ui's test helpers but
//     may not reach into the corpus that pins behaviour. That keeps WR-7 true
//     by construction rather than by rule: nothing the demo wants can make an
//     assertion fixture prettier, because the demo has its own.
//   - It writes only under -demo. `go test -update` may write testdata/ and
//     must never become the publish verb.
//
// The data is REAL. An earlier revision of this lane built the frames from the
// synthetic taste fixtures, and the result was a demo that showed Sextile's own
// lipgloss chrome and none of Astrolog's colour: `tastePositions` carries no
// BodyStyles, so every body row rendered unstyled, and six hand-written aspects
// filled exactly one of the waterfall's four columns. Astrolog owns the colour
// decision for body label, zodiac position and occupied house
// (the design notes; internal/astrolog/run.go appends `-k`
// unconditionally) and Sextile parses its exact SGR spans, so a demo that omits
// BodyStyles/AspectStyles is not a screenshot of this program. These frames go
// through lookup, spawn Astrolog, and carry the spans it printed.
var demoCapture = flag.Bool("demo", false, "write the capture lane's files: the demo frames into docs/90-assets/demo/, the README screenshot into docs/screenshot.svg")

const demoDir = "../../docs/90-assets/demo/first-transit-check"

// The demo subject is invented. The moment matches no record and is not
// anyone's birth data; the README beside the captured frames says so.
var (
	demoBirth   = time.Date(1988, 9, 23, 6, 12, 0, 0, time.UTC)
	demoPlace   = location.PlaceInfo{Name: "Utrecht", Country: "NL", Lat: 52.0907, Lon: 5.1214, TZ: "Europe/Amsterdam"}
	demoTransit = time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
)

// demoMoment is one computed instant: the natal chart, the transiting chart at
// that moment, and the cross-aspects between them. Three Astrolog spawns each,
// exactly as computeOverlayCmd makes them in production.
type demoMoment struct {
	natal   lookup.Result
	transit lookup.Result
	cross   lookup.OverlayAspects
	at      time.Time
}

func computeDemoMoment(t *testing.T, chart store.AnchorChart, at time.Time) demoMoment {
	t.Helper()
	ctx := context.Background()
	natal, err := lookup.FirstCallNatalPositions(ctx, nil, chart, lookup.Hellenistic)
	if err != nil {
		t.Fatalf("natal at %s: %v", at, err)
	}
	transit, err := lookup.FirstCallTransits(ctx, nil, at, demoPlace, lookup.Hellenistic)
	if err != nil {
		t.Fatalf("transits at %s: %v", at, err)
	}
	cross, err := lookup.OverlayCrossAspects(ctx, lookup.Hellenistic, chart, at, demoPlace)
	if err != nil {
		t.Fatalf("cross-aspects at %s: %v", at, err)
	}
	return demoMoment{natal: natal, transit: transit, cross: cross, at: at}
}

// overlayFrame mirrors computeOverlayCmd's comparison branch with the FLOATING
// slot focused: transiting bodies, the anchored chart's house frame beside
// them, and the cross-aspect list.
func (m demoMoment) overlayFrame() FirstFrameReadyMsg {
	return FirstFrameReadyMsg{
		Overlay:            true,
		Positions:          m.transit.Positions,
		NatalPositions:     m.natal.Positions,
		NatalCusps:         m.natal.Cusps,
		Cusps:              m.transit.Cusps,
		BodyStyles:         m.transit.BodyStyles,
		Aspects:            m.transit.Aspects,
		AspectStyles:       m.transit.AspectStyles,
		AspectsReport:      m.transit.AspectsReport,
		CrossAspects:       m.cross.Aspects,
		CrossAspectStyles:  m.cross.AspectStyles,
		CrossAspectsReport: m.cross.Report,
	}
}

// natalFrame is the single-chart frame with the ANCHORED slot focused: the
// client's own bodies and their intra-natal aspects, no comparison. No natal
// house frame rides along — with the anchored slot focused a body's own house
// already is its natal house (startup.go, computeOverlayCmd).
func (m demoMoment) natalFrame() FirstFrameReadyMsg {
	return FirstFrameReadyMsg{
		Positions:     m.natal.Positions,
		Cusps:         m.natal.Cusps,
		BodyStyles:    m.natal.BodyStyles,
		Aspects:       m.natal.Aspects,
		AspectStyles:  m.natal.AspectStyles,
		AspectsReport: m.natal.AspectsReport,
	}
}

type demoFrame struct{ Name, Frame string }

// captureFirstTransitCheck drives the real startup stream and returns the
// workflow in order: the client's own chart, today laid over it, the transit
// aspects read, and the same pair swept a month forward.
func captureFirstTransitCheck(t *testing.T) []demoFrame {
	t.Helper()
	forceColor(t)

	chart := store.AnchorChart{When: demoBirth, Place: demoPlace}
	now := computeDemoMoment(t, chart, demoTransit)
	// `o` steps a month, so the swept frame is a genuinely different chart --
	// different bodies, different cross-aspects -- not the same frame with a
	// new date in the header.
	later := computeDemoMoment(t, chart, demoTransit.AddDate(0, 1, 0))

	raw := location.RawPlace{
		Name: demoPlace.Name, Country: demoPlace.Country,
		Lat: demoPlace.Lat, Lon: demoPlace.Lon, TZ: demoPlace.TZ,
		LatPresent: true, LonPresent: true,
	}
	var tm tea.Model = NewModelWithClock(Config{Color: ColorAlways}, FrozenClock(demoTransit))
	step := func(msg tea.Msg) { tm, _ = tm.Update(msg) }
	commit := func(frame FirstFrameReadyMsg) {
		m := tm.(Model)
		st := m.refresh[surfaceText]
		frame.RunID, frame.WantID = st.runningRunID, st.wantID
		frame.TransitTime = m.targetFloatingTime
		step(frame)
	}
	setPair := func(on bool) {
		m, _ := setPairSelection(tm.(Model), on)
		tm = m
	}

	step(tea.WindowSizeMsg{Width: 132, Height: 40})
	step(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{demoPlace})})
	// The client's chart comes back the way it does on any relaunch: from the
	// session's snapshot, with the pair selected.
	step(ingest.StartupPerimeterReadMsg{
		LocationCandidate: location.Candidate{Source: "stored-location", Place: raw},
		Session:           v1(&store.SessionChart{ID: "constructed-demo", Name: "Constructed Demo", When: demoBirth, Place: demoPlace}, true, true, nil, nil),
		HasSession:        true,
	})
	commit(now.overlayFrame())

	var out []demoFrame
	add := func(name string) { out = append(out, demoFrame{name, tm.(Model).View()}) }

	// 1. The client's own chart: `t` drops the comparison, `up` focuses the
	//    anchored slot so Bodies shows the natal positions.
	setPair(false) // `t` is retired (the specification C4): the same change, made directly
	step(tea.KeyMsg{Type: tea.KeyUp})
	commit(now.natalFrame())
	add("01-natal-chart")

	// 2. Today laid over it: back to the floating slot, the pair selected, and
	//    `]` on Aspects chooses "today in ①" (the tab starts on the chart alone).
	step(tea.KeyMsg{Type: tea.KeyDown})
	setPair(true)
	step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	commit(now.overlayFrame())
	add("02-today-over-natal")

	// 3. Read them: `3` focuses the transit-aspect panel.
	step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	add("03-transit-aspects")

	// 4. Sweep a month forward: `o` steps the floating slot; the anchor holds.
	step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	commit(later.overlayFrame())
	add("04-one-month-forward")

	return out
}

// TestFirstTransitCheckDemo runs the workflow on every test run and checks each
// frame, so the demo cannot rot between captures. It publishes only under
// -demo.
func TestFirstTransitCheckDemo(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog is not on PATH; the demo captures real Astrolog output and cannot be synthesised")
	}
	frames := captureFirstTransitCheck(t)
	if len(frames) != 4 {
		t.Fatalf("captured %d frames, want 4", len(frames))
	}
	byName := map[string]string{}
	for _, f := range frames {
		assertFrameGeometry(t, f.Frame, 132, 40)
		assertSelfContainedFrame(t, f.Name, f.Frame)
		byName[f.Name] = ansi.Strip(f.Frame)
	}

	// The frames must tell the story the walkthrough claims, or the demo
	// silently captures the wrong loop and the README beside it lies.
	if !strings.Contains(byName["01-natal-chart"], "[3] Aspects") || strings.Contains(byName["01-natal-chart"], " in ①") || strings.Contains(byName["01-natal-chart"], " in 1") {
		t.Error("(1) dropping the comparison must leave the plain Aspects title")
	}
	for _, name := range []string{"02-today-over-natal", "03-transit-aspects", "04-one-month-forward"} {
		// Amended by C1 (2026-10-04): the strip opens with `Aspects`, and the
		// cursor's own tab, the pair, is the active one.
		if !strings.Contains(byName[name], "[3] Aspects - [2 in 1]") && !strings.Contains(byName[name], "[3] Aspects - ② in ①") {
			t.Errorf("(%s) a comparison frame must title the panel with its pair (selected-charts-compare C1)", name)
		}
	}
	if !strings.Contains(byName["02-today-over-natal"], "Sat Apr 25, 2026") {
		t.Error("(2) the floating slot should read Apr 25")
	}
	if !strings.Contains(byName["04-one-month-forward"], "Mon May 25, 2026") {
		t.Error("(4) `o` should sweep the floating slot to May 25")
	}
	if !strings.Contains(byName["04-one-month-forward"], "Fri Sep 23, 1988") {
		t.Error("(4) the anchored slot must not move when the floating slot is swept")
	}

	// Astrolog's own colour must reach the frame. A demo built from fixtures
	// that carry no BodyStyles renders every body row unstyled and looks like
	// a different program; this is the assertion that caught that.
	assertAstrologColour(t, frames)
	// The waterfall is column-major and 15 cells wide per column, so a
	// comparison frame at 132 must show more than one column of cross-aspects.
	assertMultiColumnAspects(t, byName["03-transit-aspects"])

	if !*demoCapture {
		t.Skip("frames checked; pass -demo to write them into docs/90-assets/demo/")
	}
	if err := os.MkdirAll(demoDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", demoDir, err)
	}
	for _, f := range frames {
		path := filepath.Join(demoDir, fmt.Sprintf("%s.txt", f.Name))
		if err := os.WriteFile(path, []byte(f.Frame), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

// assertAstrologColour proves the frames carry Astrolog's palette and not only
// Sextile's chrome. Astrolog prints bold-colour SGR (`\x1b[1;3Xm`) for body
// label, zodiac position and house; lipgloss emits 256-colour (`38;5;N`) for
// panel chrome. Both must be present.
func assertAstrologColour(t *testing.T, frames []demoFrame) {
	t.Helper()
	for _, f := range frames {
		var astrolog, chrome int
		for _, m := range sgrParamRE.FindAllStringSubmatch(f.Frame, -1) {
			switch code := m[1]; {
			case astrologSGRRE.MatchString(code):
				astrolog++
			case strings.HasPrefix(code, "38;5;"):
				chrome++
			}
		}
		if astrolog < minAstrologSpans {
			t.Errorf("%s carries %d Astrolog palette spans, want >= %d: the frame is missing the colour a user sees on body rows and aspect cells (a frame built without BodyStyles/AspectStyles scores about 8)",
				f.Name, astrolog, minAstrologSpans)
		}
		if chrome == 0 {
			t.Errorf("%s carries no lipgloss 256-colour span: panel chrome is missing", f.Name)
		}
	}
}

// assertMultiColumnAspects checks the waterfall actually packs columns. Each
// column is aspectColWidth cells, so a filled row has a second entry starting
// at that offset rather than trailing blanks.
func assertMultiColumnAspects(t *testing.T, plainFrame string) {
	t.Helper()
	best := 0
	for _, row := range strings.Split(plainFrame, "\n") {
		if n := len(aspectEntryRE.FindAllString(row, -1)); n > best {
			best = n
		}
	}
	if best < 2 {
		t.Errorf("the transit-aspect waterfall shows at most %d column(s); the real panel packs several -- the fixture is too small", best)
	}
}
