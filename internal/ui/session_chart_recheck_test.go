package ui

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
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

// Verifiers for an earlier change: a `.as` chart the astrologer edited in Astrolog wins on
// the next relaunch, and one that has become somebody else does not.
//
// All chart values in this file are constructed synthetic test inputs.

// chartFileWhen is what the snapshot remembers; datEditedWhen is the same chart after
// the astrologer corrected the birth minute in Astrolog.
var (
	chartFileWhen = time.Date(1971, 9, 3, 8, 15, 0, 0, time.UTC)
	datEditedWhen = time.Date(1971, 9, 3, 8, 42, 0, 0, time.UTC)
)

// chartFileSnapshot is a session chart that came from a `.as`: an id, the locator
// beside it, and the moment and place resolved when it was picked.
func chartFileSnapshot() store.SessionChart {
	return store.SessionChart{
		ID:     "rosa-lang",
		Name:   "Rosa Lang",
		When:   chartFileWhen,
		Place:  fixtureOneP,
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "rosa-lang.as"},
	}
}

// chartFileSession is the perimeter read of a session that anchored that chart, with
// no library row matching it — which is every `.as`, since a restore always
// anchors from the snapshot.
func chartFileSession(rec store.SessionChart) ingest.StartupPerimeterReadMsg {
	return withSession(perimeter(), v1(&rec, true, true, nil, nil))
}

// chartFileModel is a relaunch that has restored that chart from the snapshot: the
// state the re-read then arrives into.
func chartFileModel(t *testing.T) Model {
	t.Helper()
	sessionSandbox(t)
	m := startedModel(t, chartFileSession(chartFileSnapshot()))
	if got := anchoredSlot(t, m).Time; !got.Equal(chartFileWhen) {
		t.Fatalf("the snapshot should anchor the first frame: anchored at %v, want %v", got, chartFileWhen)
	}
	return m
}

// chartFileRow is the library row the file yields today.
func chartFileRow(name string, place store.SessionChart) store.NamedChart {
	return store.NamedChart{
		ID: "rosa-lang", Name: name, Year: 1971, Place: place.Place,
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "rosa-lang.as"},
	}
}

func recheck(m Model, msg SessionChartRecheckedMsg) (Model, tea.Cmd) {
	msg.Remembered = chartFileSnapshot()
	if i := m.slotHolding(chartFileSnapshot()); i >= 0 {
		msg.Slot = m.slots.All()[i]
	}
	return applySessionChartRecheck(m, msg)
}

func sessionSaid(m Model, want string) bool {
	for _, l := range sessionLines(m) {
		if strings.Contains(l, want) {
			return true
		}
	}
	return false
}

// The ticket itself: a restored `.as` anchor owes a re-read, and nothing else
// does. Without the dispatch the edit can never win, however correct the
// handler below it is.
func TestOnlyAChartFileAnchorOwesARereadOfItsFile(t *testing.T) {
	sessionSandbox(t)
	legacyRec := chartFileSnapshot()
	legacyRec.Source = store.ChartSource{Kind: store.ChartKindNone, File: "rosa-lang.yaml"}
	noSource := chartFileSnapshot()
	noSource.Source = store.ChartSource{}

	cases := []struct {
		name     string
		p        ingest.StartupPerimeterReadMsg
		restored bool
		want     bool
	}{
		{"a .as restored from its snapshot", chartFileSession(chartFileSnapshot()), true, true},
		{"a record naming a file this build does not read", chartFileSession(legacyRec), true, false},
		{"a record carrying no locator at all", chartFileSession(noSource), true, false},
		{"nothing anchored", chartFileSession(chartFileSnapshot()), false, false},
		{"no session at all", perimeter(), true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := startedModel(t, tc.p) // the re-check is issued for the slots the restore made
			if got := m.sessionRecheckCmd(tc.p, tc.restored) != nil; got != tc.want {
				t.Errorf("owes a re-read = %v, want %v", got, tc.want)
			}
		})
	}
}

// The edit wins: same chart, corrected moment.
func TestAnEditedChartFileChartWinsOnRelaunch(t *testing.T) {
	m := chartFileModel(t)
	m, cmd := recheck(m, SessionChartRecheckedMsg{
		Row: chartFileRow("Rosa Lang", chartFileSnapshot()), Found: true, When: datEditedWhen,
	})
	if got := anchoredSlot(t, m).Time; !got.Equal(datEditedWhen) {
		t.Errorf("anchored at %v, want the edited moment %v", got, datEditedWhen)
	}
	if anchorRecord(m).When != datEditedWhen {
		t.Errorf("the record still names %v; the next quit would write the stale moment back", anchorRecord(m).When)
	}
	if !sessionSaid(m, "was edited — re-anchored") {
		t.Errorf("the astrologer was not told the chart moved: %q", sessionLines(m))
	}
	if cmd == nil {
		t.Error("a re-anchor must dispatch a recompute; the panels would otherwise show the old chart")
	}
}

// The defect Slots.WithAnchor hides: it REPLACES the anchored slot, so a naive
// re-anchor drops every mark the operator had on it. Positive control: remove
// the capture/restore in reanchorEditedChart and this fails by name.
func TestAnEditedChartFileChartKeepsTheMarksTheOperatorMade(t *testing.T) {
	m := chartFileModel(t)
	for _, b := range []aspect.Body{aspect.Sun, aspect.Moon} {
		if _, ok := m.slots.ToggleBodySelection(0, b); !ok {
			t.Fatalf("could not mark %v on the anchored slot", b)
		}
	}
	if !m.slots.SetComparisonSelected(false) {
		t.Fatal("the pair could not be deselected")
	}

	m, _ = recheck(m, SessionChartRecheckedMsg{
		Row: chartFileRow("Rosa Lang", chartFileSnapshot()), Found: true, When: datEditedWhen,
	})

	got := m.slots.SelectedBodies(0)
	if len(got) != 2 {
		t.Fatalf("marked bodies after the re-anchor = %v, want sun and moon", got)
	}
	if m.slots.ComparisonSelected() {
		t.Error("the comparison was re-selected; the operator had turned it off")
	}
}

// The case NEXT.md says an astrologer must be told about rather than shown.
func TestADifferentChartAtAFamiliarPathIsRefusedAndNamed(t *testing.T) {
	other := chartFileSnapshot()
	other.Place = berlinP
	cases := map[string]store.NamedChart{
		"a different person": chartFileRow("Marta Feld", chartFileSnapshot()),
		"a different place":  chartFileRow("Rosa Lang", other),
	}
	for name, row := range cases {
		t.Run(name, func(t *testing.T) {
			m := chartFileModel(t)
			m, cmd := recheck(m, SessionChartRecheckedMsg{Row: row, Found: true, When: datEditedWhen})
			if got := anchoredSlot(t, m).Time; !got.Equal(chartFileWhen) {
				t.Errorf("anchored at %v — the substituted chart was taken; want the snapshot %v", got, chartFileWhen)
			}
			if !sessionSaid(m, "different chart") {
				t.Errorf("the substitution was not reported: %q", sessionLines(m))
			}
			if cmd != nil {
				t.Error("nothing moved, so nothing should recompute")
			}
		})
	}
}

// Silence is correct in exactly one case.
func TestAChartFileThatDidNotChangeSaysNothing(t *testing.T) {
	m := chartFileModel(t)
	before := len(sessionLines(m))
	m, cmd := recheck(m, SessionChartRecheckedMsg{
		Row: chartFileRow("Rosa Lang", chartFileSnapshot()), Found: true, When: chartFileWhen,
	})
	if got := len(sessionLines(m)); got != before {
		t.Errorf("an unchanged file wrote a Log line: %q", sessionLines(m))
	}
	if cmd != nil {
		t.Error("an unchanged file must not recompute")
	}
}

// Everything that is not an edit keeps the snapshot, and says so without
// blaming the wrong thing.
func TestARereadThatCannotConcludeKeepsTheSnapshot(t *testing.T) {
	cases := []struct {
		name string
		msg  SessionChartRecheckedMsg
		want string
	}{
		{"the file is gone", SessionChartRecheckedMsg{}, "is no longer in the library"},
		{"the library could not be read", SessionChartRecheckedMsg{Err: errors.New("x")}, "couldn't re-read"},
		{"the moment could not be resolved", SessionChartRecheckedMsg{Found: true, Err: errors.New("x")}, "couldn't re-read"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := chartFileModel(t)
			m, cmd := recheck(m, tc.msg)
			if got := anchoredSlot(t, m).Time; !got.Equal(chartFileWhen) {
				t.Errorf("the anchor moved to %v; the snapshot must survive", got)
			}
			if !sessionSaid(m, tc.want) {
				t.Errorf("lines = %q, want one containing %q", sessionLines(m), tc.want)
			}
			if cmd != nil {
				t.Error("nothing moved, so nothing should recompute")
			}
		})
	}
}

// An edit can leave a row that still LISTS and can no longer be cast, because
// the library lists on a weaker bar than anchoring needs. Same answer
// as the pick path: refuse and name what is missing.
func TestAnEditedChartFileThatCanNoLongerBeCastIsRefusedAndNamed(t *testing.T) {
	m := chartFileModel(t)
	placeless := chartFileSnapshot()
	placeless.Place.Name = ""
	row := chartFileRow("Rosa Lang", placeless)

	m, cmd := recheck(m, SessionChartRecheckedMsg{Row: row, Found: true, When: datEditedWhen})
	if got := anchoredSlot(t, m).Time; !got.Equal(chartFileWhen) {
		t.Errorf("an uncastable edit was anchored at %v", got)
	}
	if !sessionSaid(m, "no birth place") {
		t.Errorf("the refusal did not name what is missing: %q", sessionLines(m))
	}
	// It must not guess at WHY. The missing field is one sameChart compares,
	// so neither "edited" nor "a different chart" is a claim the evidence
	// supports.
	if sessionSaid(m, "different chart") || sessionSaid(m, "was edited") {
		t.Errorf("an uncastable row was reported as an identity change: %q", sessionLines(m))
	}
	if cmd != nil {
		t.Error("nothing moved, so nothing should recompute")
	}
}

// A re-pick of this very chart while its re-read is in flight outranks the
// re-read: the pick replaced the slot, and the background result must not
// anchor over the operator's explicit choice. (A pick of some OTHER chart does
// not: see TestPickDoesNotDiscardAnotherChartsRecheck.)
func TestAPickOutranksAnInFlightReread(t *testing.T) {
	m := chartFileModel(t)
	slot := firstAnchor(m.slots)
	// The operator re-picks the chart; the pick re-anchors the slot in place.
	picked := chartFileWhen.Add(90 * time.Minute)
	if err := m.slots.ReplaceAnchored(0, chartFileSnapshot(), picked, fixtureOneP); err != nil {
		t.Fatal(err)
	}
	before := len(sessionLines(m))

	m, cmd := applySessionChartRecheck(m, SessionChartRecheckedMsg{
		Remembered: chartFileSnapshot(),
		Row:        chartFileRow("Rosa Lang", chartFileSnapshot()),
		Found:      true,
		When:       datEditedWhen,
		Slot:       slot,
	})
	if got := anchoredSlot(t, m).Time; !got.Equal(picked) {
		t.Errorf("a superseded re-read anchored at %v, want the pick's %v", got, picked)
	}
	if got := len(sessionLines(m)); got != before {
		t.Errorf("a superseded re-read wrote a Log line: %q", sessionLines(m))
	}
	if cmd != nil {
		t.Error("a superseded re-read must be dropped whole")
	}
}

// A substitute that shares the display name AND rounds to the same atlas city
// is the case comparing `Place.Name` alone could not see: `location.Nearest`
// gives two births thirty kilometres apart one city label, so the only thing
// separating them is the coordinates the row already carries.
//
// Positive control: narrow sameChart back to `a.Place.Name == b.Place.Name` and
// this fails by name while every other test here still passes.
func TestASubstituteSharingNameAndCityIsStillRefused(t *testing.T) {
	m := chartFileModel(t)
	nearby := chartFileSnapshot()
	nearby.Place.Lat += 0.3 // same nearest city, a different birthplace
	nearby.Place.Lon -= 0.2

	m, cmd := recheck(m, SessionChartRecheckedMsg{
		Row: chartFileRow("Rosa Lang", nearby), Found: true, When: datEditedWhen,
	})
	if got := anchoredSlot(t, m).Time; !got.Equal(chartFileWhen) {
		t.Errorf("anchored at %v — a different person was taken as an edit", got)
	}
	if !sessionSaid(m, "different chart") {
		t.Errorf("the substitution was not reported: %q", sessionLines(m))
	}
	if cmd != nil {
		t.Error("nothing moved, so nothing should recompute")
	}
}

// Slot selection is per slot: Space on the Chart panel toggles ONE of them, so
// the anchored slot can be deselected while the floating one stays selected.
// ComparisonSelected() is the AND of the two and cannot describe that, so
// restoring through SetComparisonSelected would clear the floating slot's own
// marker.
//
// Positive control: restore via `m.slots.SetComparisonSelected(comparison)` and
// this fails by name.
func TestAnEditedChartFileChartKeepsAnAsymmetricSelection(t *testing.T) {
	m := chartFileModel(t)
	if _, ok := m.slots.ToggleSlotSelection(0); !ok {
		t.Fatal("the anchored slot could not be deselected")
	}
	if m.slots.SlotSelected(0) || !m.slots.SlotSelected(floatingIndex(m.slots)) {
		t.Fatalf("the fixture is not asymmetric: anchored=%v floating=%v",
			m.slots.SlotSelected(0), m.slots.SlotSelected(floatingIndex(m.slots)))
	}

	m, _ = recheck(m, SessionChartRecheckedMsg{
		Row: chartFileRow("Rosa Lang", chartFileSnapshot()), Found: true, When: datEditedWhen,
	})

	if m.slots.SlotSelected(0) {
		t.Error("the anchored slot was re-selected; the operator had deselected it")
	}
	if !m.slots.SlotSelected(floatingIndex(m.slots)) {
		t.Error("the floating slot lost its own selection to the re-anchor")
	}
}

// Every line this slice can write has to survive the 6-row Log's render whole,
// the way the error lane's do (TestFailureLinesFitTheLog). Two things make this
// a guarantee rather than a sample.
//
// It DRIVES each of the five outcomes and measures the line actually emitted,
// so a reworded message cannot pass a stale expectation. And it drives them
// with a chart whose name and filename are absurd, which is the point: the
// lines DO name the chart, so the only thing keeping them inside 64 cells is
// logIdent's own bound, and a test using a realistic-length fixture would pass
// while the tightest message still truncated for an ordinary two-part surname.
// That is exactly what an earlier version of this test did.
//
// The refusal clauses are read out of SessionChart.UnusableReason's own source,
// so a sixth clause cannot ship unmeasured.
func TestSessionRecheckLinesFitTheLog(t *testing.T) {
	absurd := chartFileSnapshot()
	absurd.Name = strings.Repeat("Alexandria Featherstonhaugh ", 4)
	absurd.Source.File = strings.Repeat("alexandria-featherstonhaugh-", 4) + "natal.as"

	// wantIdent is the fragment of the absurd name or filename the line must
	// still carry, or "" for the one line that carries no identifier. Passing it
	// through check() rather than asserting per-subtest is what keeps the
	// guarantee uniform: an outcome added later cannot quietly opt out.
	check := func(t *testing.T, msg SessionChartRecheckedMsg, wantIdent string) string {
		t.Helper()
		m := chartFileModel(t)
		firstAnchor(m.slots).Chart = absurd
		msg.Remembered = absurd
		if i := m.slotHolding(chartFileSnapshot()); i >= 0 {
			msg.Slot = m.slots.All()[i]
		}
		m, _ = applySessionChartRecheck(m, msg)
		lines := sessionLines(m)
		if len(lines) == 0 {
			t.Fatal("the outcome wrote no line to measure")
		}
		line := lines[len(lines)-1]
		sized, _ := m.Update(tea.WindowSizeMsg{Width: 144, Height: 40})
		if got := sized.(Model).View(); !strings.Contains(got, line) {
			t.Errorf("the Log truncated %q; it renders as:\n%s", line, logRegion(got))
		}
		if wantIdent != "" {
			// Bounded is not the same as absent. Nothing in the persistent UI
			// renders a chart's name, so a line that stopped naming its chart
			// would leave this warning untraceable to a client — which is the
			// regression the identifier exists to prevent, and it would
			// otherwise pass every other assertion here.
			if !strings.Contains(line, wantIdent) {
				t.Errorf("the line names no chart or file: %q", line)
			}
			if !strings.Contains(line, cutMark(m.cfg)) {
				t.Errorf("an identifier past the budget was not marked as cut: %q", line)
			}
		}
		return line
	}

	row := func(name string, place store.SessionChart) store.NamedChart {
		return store.NamedChart{Name: name, Place: place.Place, Source: absurd.Source}
	}
	elsewhere := absurd
	elsewhere.Place = berlinP

	t.Run("edited", func(t *testing.T) {
		check(t, SessionChartRecheckedMsg{Row: row(absurd.Name, absurd), Found: true, When: datEditedWhen}, "Alexandria")
	})
	t.Run("different", func(t *testing.T) {
		check(t, SessionChartRecheckedMsg{Row: row("Constructed Two", elsewhere), Found: true, When: datEditedWhen}, "alexandria")
	})
	t.Run("gone", func(t *testing.T) {
		check(t, SessionChartRecheckedMsg{}, "alexandria")
	})
	t.Run("unreadable", func(t *testing.T) {
		check(t, SessionChartRecheckedMsg{Err: errors.New("x")}, "Alexandria")
	})
	// One subtest per refusal clause, each read from SessionChart.UnusableReason.
	for _, clause := range refusalClauses(t) {
		t.Run("uncastable/"+clause, func(t *testing.T) {
			broken := absurd
			switch clause {
			case "no name":
				broken.Name = ""
			case "no birth time":
				broken.When = time.Time{}
			case "no birth place":
				broken.Place.Name = ""
			case "no numeric coordinates":
				broken.Place.Lat = math.NaN()
			case "an unknown timezone":
				broken.Place.TZ = "Nowhere/Nothing"
			default:
				t.Fatalf("unmeasured refusal clause %q — add it here", clause)
			}
			line := check(t, SessionChartRecheckedMsg{
				Row:   row(broken.Name, broken),
				Found: true,
				When:  broken.When,
			}, "")
			if !strings.Contains(line, clause) {
				t.Errorf("line %q does not name the clause %q it refused for", line, clause)
			}
		})
	}
}

// A coordinate-only correction — the birth time untouched, the coordinates
// tightened — is REPORTED as a different chart and not applied. That is the
// stated limit of a comparison that must run without a spawn: nothing here
// separates it from a substitution, and refusing-and-saying-so is the safe
// direction. Pinned so the behaviour is a decision on the record rather than a
// side effect nobody measured.
func TestACoordinateOnlyCorrectionIsReportedAndNotApplied(t *testing.T) {
	m := chartFileModel(t)
	tightened := chartFileSnapshot()
	tightened.Place.Lat += 0.02
	tightened.Place.Lon += 0.02

	m, cmd := recheck(m, SessionChartRecheckedMsg{
		Row:   chartFileRow("Rosa Lang", tightened),
		Found: true,
		When:  chartFileWhen, // the recorded moment did not move
	})
	if got := anchoredSlot(t, m).Time; !got.Equal(chartFileWhen) {
		t.Errorf("the anchor moved to %v", got)
	}
	if got := anchoredSlot(t, m).Location; got != fixtureOneP {
		t.Errorf("the coordinates were applied silently: %+v", got)
	}
	if !sessionSaid(m, "different chart") {
		t.Errorf("the change was not reported at all: %q", sessionLines(m))
	}
	if cmd != nil {
		t.Error("nothing moved, so nothing should recompute")
	}
}

// C3.21 (the specification): a session written while `;sextile-id:` named
// charts carries that id. When the re-check finds the file by name, the
// stem-derived id is adopted even though the moment is unchanged, so the next
// quit writes the id the library resolves.
//
// End to end through the real re-read: the library read, the live astrolog
// resolve, the handler, and the quit's session write.
func TestLegacySextileIDReconcilesOnRecheck(t *testing.T) {
	sessionSandbox(t)
	dir := store.ChartsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	const legacy = "@AI780  ; Astrolog chart info.\n" +
		"-qb Sep 23 1995 6:00pm DT 1E 11:34E 48:08N\n" +
		"-zi \"Constructed Old\" \"Munich, Germany\"\n" +
		";sextile-id: old-id\n"
	if err := os.WriteFile(filepath.Join(dir, "Constructed-Old.as"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := lookup.LoadChartLibrary()
	if err != nil || len(lib.Charts) != 1 {
		t.Fatalf("library = %+v, %v; want the one fixture", lib, err)
	}
	row := lib.Charts[0]
	when, err := lookup.ResolvePicked(context.Background(), row)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	rec := store.SessionChart{ID: "old-id", Name: row.Name, When: when, Place: row.Place, Source: row.Source}

	m := startedModel(t, chartFileSession(rec))
	if anchorRecord(m).ID != "old-id" {
		t.Fatalf("precondition: restored id = %q, want old-id", anchorRecord(m).ID)
	}
	msg := recheckSessionChartCmd(rec, firstAnchor(m.slots))()
	tm, _ := m.Update(msg)
	m = tm.(Model)
	if anchorRecord(m).ID != "constructed-old" {
		t.Errorf("after the re-check anchorChart.ID = %q, want constructed-old", anchorRecord(m).ID)
	}
	if got := anchoredSlot(t, m).Time; !got.Equal(when) {
		t.Errorf("the re-check moved the anchor to %v; the moment was unchanged", got)
	}

	if _, cmd := press(t, m, "q"); cmd == nil {
		t.Fatal("q did not quit")
	}
	s, ok, err := store.LoadSession()
	if !ok || err != nil || len(s.Slots) != 1 {
		t.Fatalf("session after quit: ok=%v err=%v slots=%v", ok, err, s.Slots)
	}
	if s.Slots[0].Chart.ID != "constructed-old" {
		t.Errorf("the quit wrote id %q, want constructed-old", s.Slots[0].Chart.ID)
	}
}

// A re-check that corrects a slot's moment takes an open wheel with it, through
// the same refresh a focus change uses.
func TestRecheckCorrectionRefreshesAnOpenWheel(t *testing.T) {
	var whens []time.Time
	prev := renderWheelImage
	renderWheelImage = func(_ context.Context, when time.Time, _ location.PlaceInfo, _ lookup.Lens, _ lookup.WheelHighlight) ([]byte, error) {
		whens = append(whens, when)
		return []byte("BM-wheel"), nil
	}
	t.Cleanup(func() { renderWheelImage = prev })

	stubTextCompute(t)
	stubShowWheel(t)
	m := chartFileModel(t)
	m.slots.SetComparisonSelected(false)
	m.slots.ToggleSlotSelection(0) // the chart alone: a single wheel
	m.cursorIdx = 0
	m.previewActive = true
	wheelID := m.refresh[surfaceImage].wantID

	m, cmd := recheck(m, SessionChartRecheckedMsg{Row: chartFileRow("Rosa Lang", chartFileSnapshot()), Found: true, When: datEditedWhen})
	if m.refresh[surfaceImage].wantID == wheelID {
		t.Fatal("the correction requested no wheel render")
	}
	// Drive the returned commands — batches, the debounce tick and the render —
	// through Update until the model is quiet: the render must really be issued.
	whens = nil
	pump(t, m, cmd)
	if len(whens) == 0 || !whens[len(whens)-1].Equal(datEditedWhen) {
		t.Errorf("wheel renders = %v, want one for the corrected moment %v", whens, datEditedWhen)
	}

	// With no preview open the correction starts no wheel work.
	m = chartFileModel(t)
	wheelID = m.refresh[surfaceImage].wantID
	m, _ = recheck(m, SessionChartRecheckedMsg{Row: chartFileRow("Rosa Lang", chartFileSnapshot()), Found: true, When: datEditedWhen})
	if m.refresh[surfaceImage].wantID != wheelID {
		t.Error("a correction with no preview open requested a wheel render")
	}
}

// One file, one row: a re-check result carrying ANOTHER file's content never
// touches the row, whatever its name and place say.
func TestRecheckNeverAppliesAnotherFilesContent(t *testing.T) {
	m := chartFileModel(t)
	row := chartFileRow("Rosa Lang", chartFileSnapshot())
	row.Source = store.ChartSource{Kind: store.ChartKindFile, File: "someone-else.as"}
	before := *firstAnchor(m.slots)
	m, cmd := recheck(m, SessionChartRecheckedMsg{Row: row, Found: true, When: datEditedWhen})
	if got := *firstAnchor(m.slots); got.Time != before.Time || got.Chart != before.Chart {
		t.Errorf("another file's content was applied: %+v", got)
	}
	if cmd != nil || len(sessionLines(m)) != 0 {
		t.Errorf("a result for another file acted: cmd %v, lines %q", cmd != nil, sessionLines(m))
	}
}
