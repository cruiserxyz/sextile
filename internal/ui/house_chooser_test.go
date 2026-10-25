package ui

// the specification. The pure-model tests use the stubbed lookups below so
// they can read the lens (and so the house system) each executed call carried;
// the rename and relaunch tests run the real Astrolog against a sandboxed chart
// folder. All chart values are constructed synthetic test inputs.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

type houseCall struct {
	label  string
	houses recipe.HouseSystem
}

// houseSpy records, for every stubbed lookup, which chart it was for and the
// house system its lens carried.
type houseSpy struct {
	mu    sync.Mutex
	calls []houseCall
	// used, when set, is the system the stub says Astrolog used.
	used func(label string, asked recipe.HouseSystem) string
}

func (s *houseSpy) record(label string, h recipe.HouseSystem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, houseCall{label, h})
}

func (s *houseSpy) snapshot() []houseCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]houseCall(nil), s.calls...)
}

func (s *houseSpy) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = nil
}

// of is every house system the calls for label carried, in order.
func (s *houseSpy) of(label string) []recipe.HouseSystem {
	var out []recipe.HouseSystem
	for _, c := range s.snapshot() {
		if c.label == label {
			out = append(out, c.houses)
		}
	}
	return out
}

func (s *houseSpy) lastOf(t *testing.T, label string) recipe.HouseSystem {
	t.Helper()
	h := s.of(label)
	if len(h) == 0 {
		t.Fatalf("no executed call for %s; calls = %v", label, s.snapshot())
	}
	return h[len(h)-1]
}

func (s *houseSpy) usedFor(label string, asked recipe.HouseSystem) string {
	if s.used == nil {
		return ""
	}
	return s.used(label, asked)
}

func natalLabel(a store.AnchorChart) string { return fmt.Sprintf("n%d", a.When.Year()-1960) }

// stubHouseLookups answers every text compute at once. A natal chart's Ascendant
// cusp is 100+its system, the transit chart's 200+its system, so a frame can be
// traced back to the call and the system that made it.
func stubHouseLookups(t *testing.T) *houseSpy {
	t.Helper()
	spy := &houseSpy{}
	oldN, oldT, oldO, oldS := firstCallNatalPositions, firstCallTransits, overlayCrossAspects, synastryCrossAspects
	t.Cleanup(func() {
		firstCallNatalPositions, firstCallTransits, overlayCrossAspects, synastryCrossAspects = oldN, oldT, oldO, oldS
	})
	firstCallNatalPositions = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, lens lookup.Lens) (lookup.Result, error) {
		label := natalLabel(c)
		spy.record(label, lens.Houses)
		return lookup.Result{
			Positions:  []aspect.Position{{Body: aspect.Sun, Longitude: float64(c.When.Year() % 360)}},
			Cusps:      aspect.Cusps{100 + float64(lens.Houses)},
			HousesUsed: spy.usedFor(label, lens.Houses),
			Commands:   []string{fmt.Sprintf("astrolog natal %s -c %d", label, int(lens.Houses))},
		}, nil
	}
	firstCallTransits = func(_ context.Context, _ *lookup.Probers, when time.Time, _ location.PlaceInfo, lens lookup.Lens) (lookup.Result, error) {
		spy.record("now", lens.Houses)
		return lookup.Result{
			Positions:  []aspect.Position{{Body: aspect.Sun, Longitude: float64(when.Year()%360) + 0.5}},
			Cusps:      aspect.Cusps{200 + float64(lens.Houses)},
			HousesUsed: spy.usedFor("now", lens.Houses),
			Commands:   []string{fmt.Sprintf("astrolog transit -c %d", int(lens.Houses))},
		}, nil
	}
	overlayCrossAspects = func(_ context.Context, lens lookup.Lens, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo) (lookup.OverlayAspects, error) {
		spy.record("cross", lens.Houses)
		return lookup.OverlayAspects{Report: "transit cross"}, nil
	}
	synastryCrossAspects = func(_ context.Context, lens lookup.Lens, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo, _ store.ReportLabel) (lookup.OverlayAspects, error) {
		spy.record("synastry", lens.Houses)
		return lookup.OverlayAspects{Report: "synastry cross"}, nil
	}
	return spy
}

// chosen is a session record for the anchored slot at index i.
func chosen(m Model, i int, hs recipe.HouseSystem) store.FrameRecord {
	s := m.slots.All()[i]
	return store.FrameRecord{File: slotFile(s), When: s.Time, Houses: hs, HasHouses: true, From: store.FrameFromChosen}
}

func floatingChoice(hs recipe.HouseSystem) store.FrameRecord {
	return store.FrameRecord{Floating: true, Houses: hs, HasHouses: true, From: store.FrameFromChosen}
}

func settleHouses(t *testing.T, m Model) Model {
	t.Helper()
	m.cfg = Config{Color: ColorNever}
	m.focused = PanelChart
	m = m.noteArrival()
	m, cmd := m.afterComparisonChange()
	return pump(t, m, cmd)
}

func logHas(m Model, sub string) int {
	n := 0
	for _, l := range m.logLines {
		if strings.Contains(l.text, sub) {
			n++
		}
	}
	return n
}

// C1, C11d.
func TestOwnHouseSystemPrecedence(t *testing.T) {
	base := func() Model {
		m := listModel(t, 1, 0)
		m.frames.files = map[string]fileFrame{"synthetic-1.as": {when: m.slots.All()[0].Time}}
		return m
	}
	slotOf := func(m Model) *store.Slot { return m.slots.All()[0] }
	t.Run("nothing names a system: Whole Sign", func(t *testing.T) {
		m := base()
		if got := m.ownHouses(slotOf(m)); got.hs != recipe.WholeSign || got.src != houseFallback {
			t.Errorf("own = %+v", got)
		}
	})
	t.Run("the preferences default beats Whole Sign", func(t *testing.T) {
		m := base().applyFrameConfig(store.FrameConfig{Houses: "Koch", HousesPresent: true})
		if got := m.ownHouses(slotOf(m)); got.hs != recipe.Koch || got.src != houseDefault {
			t.Errorf("own = %+v", got)
		}
	})
	t.Run("the chart file's own line beats the default", func(t *testing.T) {
		m := base().applyFrameConfig(store.FrameConfig{Houses: "Koch", HousesPresent: true})
		m.frames = m.frames.withFile("synthetic-1.as", fileFrame{when: slotOf(m).Time, houses: recipe.Equal, has: true})
		if got := m.ownHouses(slotOf(m)); got.hs != recipe.Equal || got.src != houseFile {
			t.Errorf("own = %+v", got)
		}
	})
	t.Run("a session record beats the file", func(t *testing.T) {
		m := base()
		m.frames = m.frames.withFile("synthetic-1.as", fileFrame{when: slotOf(m).Time, houses: recipe.Equal, has: true})
		m.frames = m.frames.with(chosen(m, 0, recipe.Porphyry))
		if got := m.ownHouses(slotOf(m)); got.hs != recipe.Porphyry || got.src != houseChosen {
			t.Errorf("own = %+v", got)
		}
	})
	t.Run("a zodiac-only record freezes nothing", func(t *testing.T) {
		m := base()
		m.frames = m.frames.withFile("synthetic-1.as", fileFrame{when: slotOf(m).Time, houses: recipe.Equal, has: true})
		m.frames = m.frames.with(store.FrameRecord{File: "synthetic-1.as", When: slotOf(m).Time, Zodiac: "Lahiri", HasZodiac: true})
		if got := m.ownHouses(slotOf(m)); got.hs != recipe.Equal || got.src != houseFile {
			t.Errorf("own = %+v, want the file's", got)
		}
	})
	t.Run("the floating slot has its own record", func(t *testing.T) {
		m := base()
		m.frames = m.frames.with(floatingChoice(recipe.Campanus))
		fl := m.slots.All()[len(m.slots.All())-1]
		if got := m.ownHouses(fl); got.hs != recipe.Campanus || got.src != houseChosen {
			t.Errorf("own = %+v", got)
		}
	})
	t.Run("repeated keys", func(t *testing.T) {
		sessionSandbox(t)
		path := store.PreferencesPath()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		text := "frame:\n  houses: Koch\n  houses: Equal\n  zodiac: Lahiri\nframe:\n  houses: Porphyry\n"
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		p := realPerimeter(t)
		m := base().applyFrameConfig(p.Frame)
		if got := m.ownHouses(slotOf(m)); got.hs != recipe.Koch || got.src != houseDefault {
			t.Errorf("own = %+v, want the first houses: key of the first frame: block (Koch)", got)
		}
	})
	t.Run("bad default", func(t *testing.T) {
		m := base().applyFrameConfig(store.FrameConfig{Houses: "Nonsense", HousesPresent: true})
		m = m.applyFrameConfig(store.FrameConfig{Houses: "Nonsense", HousesPresent: true})
		if got := m.ownHouses(slotOf(m)); got.hs != recipe.WholeSign {
			t.Errorf("own = %+v, want Whole Sign", got)
		}
		if n := logHas(m, "Nonsense"); n != 1 {
			t.Errorf("the ignored value was named %d times, want once: %v", n, m.logLines)
		}
	})
}

// Each selected chart keeps its own system (operator, 2026-10-24): every call
// for a chart's own rows carries that chart's own, whichever row the cursor is
// on, and a comparison call carries the frame's, the chart you are on.
func TestSelectedChartsKeepTheirOwnHouses(t *testing.T) {
	spy := stubHouseLookups(t)
	build := func(cursor int) Model {
		m := selectedSet(t, 2, true, cursor)
		m.frames.records = []store.FrameRecord{chosen(m, 0, recipe.Koch), chosen(m, 1, recipe.Equal), floatingChoice(recipe.Porphyry)}
		return m
	}
	own := map[string]recipe.HouseSystem{"n1": recipe.Koch, "n2": recipe.Equal, "now": recipe.Porphyry}
	for _, c := range []struct {
		name   string
		cursor int
		visits int // list index of the chart now visits; -1 for none
		view   recipe.HouseSystem
		frame  recipe.HouseSystem
	}{
		{"first chart", 0, -1, recipe.Koch, recipe.Koch},
		{"second chart", 1, -1, recipe.Equal, recipe.Equal},
		{"floating on its own tab", 2, -1, recipe.Porphyry, recipe.Porphyry},
		{"floating visiting the first chart", 2, 0, recipe.Porphyry, recipe.Koch},
		{"floating visiting the second chart", 2, 1, recipe.Porphyry, recipe.Equal},
	} {
		t.Run("in the group, cursor on "+c.name, func(t *testing.T) {
			spy.reset()
			m := build(c.cursor)
			if c.visits >= 0 {
				m = choosePair(m, scenePair{left: slotAt(m, c.visits), right: slotAt(m, 2)})
			}
			m = settleHouses(t, m)
			if got := m.viewLens().Houses; got != c.view {
				t.Errorf("viewLens = %v, want the cursor's own %v", got, c.view)
			}
			for _, call := range spy.snapshot() {
				want, mine := own[call.label]
				if !mine {
					want = c.frame // the cross call: a comparison
				}
				if call.houses != want {
					t.Errorf("executed %s call carried %v, want %v: %v", call.label, call.houses, want, spy.snapshot())
				}
			}
		})
	}
	t.Run("an unselected focused chart shows its own", func(t *testing.T) {
		spy.reset()
		m := build(1)
		m.slots.SetSlotSelected(1, false)
		m = settleHouses(t, m)
		if got := m.viewLens().Houses; got != recipe.Equal {
			t.Errorf("viewLens = %v, want its own Equal", got)
		}
		if got := spy.lastOf(t, "n2"); got != recipe.Equal {
			t.Errorf("the executed call carried %v, want Equal", got)
		}
	})
	t.Run("no anchored chart selected: the floating slot is its own", func(t *testing.T) {
		m := build(2)
		m.slots.SetSlotSelected(0, false)
		m.slots.SetSlotSelected(1, false)
		if got := m.viewLens().Houses; got != recipe.Porphyry {
			t.Errorf("viewLens = %v, want the floating slot's own Porphyry", got)
		}
	})
}

// C6 and the pass-6 build note: the natal-house overlay is a comparison, cast
// in the frame's system and read from the cusps that executed call returned;
// the floating chart's own rows stay in its own system.
func TestHouseOverlayUsesTheFramesCusps(t *testing.T) {
	spy := stubHouseLookups(t)
	m := selectedSet(t, 1, true, 1) // ① and now selected, the cursor on now
	m.base = m.slots.All()[0]       // now read in ①: the faint overlay is in ①'s cusps
	m.frames.records = []store.FrameRecord{chosen(m, 0, recipe.Koch), floatingChoice(recipe.Porphyry)}
	m = settleHouses(t, m)
	if got := spy.lastOf(t, "n1"); got != recipe.Koch {
		t.Errorf("the frame was cast in %v, want Koch", got)
	}
	if got := spy.lastOf(t, "now"); got != recipe.Porphyry {
		t.Errorf("the floating chart was cast in %v, want its own Porphyry", got)
	}
	if got := spy.lastOf(t, "cross"); got != recipe.Koch {
		t.Errorf("the cross call carried %v, want the frame's Koch", got)
	}
	if got, want := m.chart.natalCusps[0], 100+float64(recipe.Koch); got != want {
		t.Errorf("overlay cusp = %v, want %v (the frame's own cusps in Koch)", got, want)
	}
	if got, want := m.chart.cusps[0], 200+float64(recipe.Porphyry); got != want {
		t.Errorf("now's own cusp = %v, want %v (its own Porphyry)", got, want)
	}
}

// The overlay places the moving bodies in the frame's zodiac: with ① sidereal
// and now tropical, now's rows stay tropical, and the house each body transits
// in ①'s chart is read from its position in ①'s Lahiri.
func TestHouseOverlayPlacesInTheFramesZodiac(t *testing.T) {
	spy := stubFrameLookups(t)
	m := selectedSet(t, 1, true, 1) // ① and now selected, the cursor on now
	m.base = m.slots.All()[0]       // now read in ①
	m.frames.records = []store.FrameRecord{chosenZ(m, 0, recipe.Lahiri)}
	m = settleHouses(t, m)
	year := transitTimeFromWorkspace(m).Year()
	if got, want := m.chart.positions[0].Longitude, zodiacLongitude(year, recipe.Tropical)+0.5; got != want {
		t.Errorf("now's Sun row is at %v, want its own tropical %v", got, want)
	}
	if len(m.chart.framedPositions) != 1 {
		t.Fatalf("no framed positions committed: %v (calls %v)", m.chart.framedPositions, spy.snapshot())
	}
	if got, want := m.chart.framedPositions[0].Longitude, zodiacLongitude(year, recipe.Lahiri)+0.5; got != want {
		t.Errorf("now's Sun is placed from %v, want its Lahiri position %v", got, want)
	}
	if got := spy.lastOf(t, "cross"); got != recipe.Lahiri {
		t.Errorf("the cross call carried %v, want the frame's Lahiri", got)
	}
	// One zodiac for both: nothing is cast twice.
	spy.reset()
	m.frames.records = nil
	m = settleHouses(t, m)
	if m.chart.framedPositions != nil {
		t.Errorf("framed positions with one zodiac on both sides: %v", m.chart.framedPositions)
	}
}

func TestOverlayCuspsComeFromExecutedBaselineCast(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	// Real lookups, no stubs: ① (record Koch) and now are the group, the cursor
	// is on now, so the Bodies overlay places now's bodies into ①'s houses.
	m := selectedSet(t, 1, true, 1)
	m.base = m.slots.All()[0] // now read in ①
	m.frames.records = []store.FrameRecord{chosen(m, 0, recipe.Koch)}
	m = settleHouses(t, m)
	base := anchorOf(m.slots.All()[0])
	koch := m.lens
	koch.Houses = recipe.Koch
	direct, err := lookup.FirstCallNatalPositions(context.Background(), nil, base, koch)
	if err != nil || !direct.Cusps.Valid() {
		t.Fatalf("the live Koch cast: cusps %v, err %v", direct.Cusps, err)
	}
	if m.chart.natalCusps != direct.Cusps {
		t.Errorf("overlay cusps = %v, want the baseline cast's own %v", m.chart.natalCusps, direct.Cusps)
	}
	whole := m.lens
	whole.Houses = recipe.WholeSign
	ws, err := lookup.FirstCallNatalPositions(context.Background(), nil, base, whole)
	if err != nil || ws.Cusps == direct.Cusps {
		t.Fatalf("control: Whole Sign cusps %v equal Koch's (%v)", ws.Cusps, err)
	}
	// Every body the cast itself printed a house for lands in that house when
	// placed through the overlay's cusps: Astrolog's printed houses, not a Go
	// computation, are what the overlay is held to.
	ov := houseOverlay{cusps: m.chart.natalCusps, positions: m.chart.natalPositions}
	checked := 0
	for _, p := range direct.Positions {
		if !p.House.Valid() {
			continue
		}
		if _, live := ov.houses(p); live != p.House {
			t.Errorf("%v: overlay house %d, Astrolog printed %d", p.Body, live, p.House)
		}
		checked++
	}
	if checked < 8 {
		t.Fatalf("only %d bodies carried a printed house", checked)
	}
}

// C7: the tag carries the substitution mark and the Log says it once.
func TestSubstitutionMarksTheTagAndLogsOnce(t *testing.T) {
	spy := stubHouseLookups(t)
	spy.used = func(label string, asked recipe.HouseSystem) string {
		if label == "n2" && asked == recipe.Placidus {
			return "Porphyry"
		}
		return asked.StoredName()
	}
	m := selectedSet(t, 2, true, 0)
	m.frames.records = []store.FrameRecord{chosen(m, 0, recipe.Placidus), chosen(m, 1, recipe.Placidus)}
	m = settleHouses(t, m)
	if got := m.rowTag(m.slots.All()[1]); got != "Po*Tr" {
		t.Errorf("polar row tag = %q, want Po*Tr", got)
	}
	if got := m.rowTag(m.slots.All()[0]); got != "PlTr" {
		t.Errorf("ordinary row tag = %q, want PlTr", got)
	}
	m = settleHouses(t, m)
	line := "Placidus asked, Astrolog used Porphyry"
	if n := logHas(m, line); n != 1 {
		t.Errorf("the substitution was logged %d times, want once: %v", n, m.logLines)
	}
	for _, l := range m.logLines {
		if strings.Contains(l.text, line) && l.lane != laneNote {
			t.Errorf("the substitution is on lane %v, want the note lane", l.lane)
		}
	}
}

// C8 and its failure modes.
func TestHouseChoiceSavesTheSession(t *testing.T) {
	savedFrames := func(t *testing.T) []store.FrameRecord {
		t.Helper()
		s, ok, err := store.LoadSession()
		if err != nil || !ok {
			return nil
		}
		return s.Frames
	}
	t.Run("a choice is written at once, not only at quit", func(t *testing.T) {
		sessionSandbox(t)
		stubHouseLookups(t)
		m := listModel(t, 2, 0)
		m, cmd := m.chooseHouseSystem(1, recipe.Regiomontanus)
		m = pump(t, m, cmd)
		got := savedFrames(t)
		if len(got) != 1 || got[0].File != "synthetic-2.as" || got[0].Houses != recipe.Regiomontanus || got[0].From != store.FrameFromChosen {
			t.Fatalf("session frames = %+v", got)
		}
	})
	t.Run("the floating slot's choice persists as its own record", func(t *testing.T) {
		sessionSandbox(t)
		stubHouseLookups(t)
		m := listModel(t, 1, 0)
		m, cmd := m.chooseHouseSystem(1, recipe.Koch)
		_ = pump(t, m, cmd)
		got := savedFrames(t)
		if len(got) != 1 || !got[0].Floating || got[0].Houses != recipe.Koch {
			t.Fatalf("session frames = %+v", got)
		}
	})
	t.Run("no file", func(t *testing.T) {
		sessionSandbox(t)
		stubHouseLookups(t)
		m := listModel(t, 1, 0)
		m.slots.All()[0].Chart.Source = store.ChartSource{}
		m, cmd := m.chooseHouseSystem(0, recipe.Koch)
		m = pump(t, m, cmd)
		if got := m.ownHouses(m.slots.All()[0]).hs; got != recipe.Koch {
			t.Errorf("the choice does not apply until quit: %v", got)
		}
		if len(savedFrames(t)) != 0 {
			t.Errorf("a chart with no file got a record: %+v", savedFrames(t))
		}
		if logHas(m, "can't be remembered") != 1 {
			t.Errorf("the Log does not say it cannot be remembered: %v", m.logLines)
		}
	})
	t.Run("write fails", func(t *testing.T) {
		sessionSandbox(t)
		stubHouseLookups(t)
		// A plain file where the session folder should be: the write cannot
		// create its temp file.
		if err := os.MkdirAll(filepath.Dir(store.SessionDir()), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(store.SessionDir(), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		m := listModel(t, 1, 0)
		m, cmd := m.chooseHouseSystem(0, recipe.Koch)
		m = pump(t, m, cmd)
		if got := m.ownHouses(m.slots.All()[0]).hs; got != recipe.Koch {
			t.Errorf("the choice must still apply on screen: %v", got)
		}
		if logHas(m, "Couldn't save the house choice") != 1 {
			t.Errorf("the failure is not on the error lane: %v", m.logLines)
		}
		for _, l := range m.logLines {
			if strings.Contains(l.text, "Couldn't save the house choice") && l.lane != laneError {
				t.Errorf("failure on lane %v", l.lane)
			}
		}
	})
	t.Run("a launch whose refused session could not be kept writes nothing", func(t *testing.T) {
		sessionSandbox(t)
		stubHouseLookups(t)
		m := listModel(t, 1, 0)
		m.sessionKept = true
		m, cmd := m.chooseHouseSystem(0, recipe.Koch)
		m = pump(t, m, cmd)
		if _, err := os.Stat(store.SessionPath()); !os.IsNotExist(err) {
			t.Errorf("a session was written over the refused file: %v", err)
		}
		if got := m.ownHouses(m.slots.All()[0]).hs; got != recipe.Koch {
			t.Errorf("the choice must still apply: %v", got)
		}
	})
}

// C4: the EXECUTED -t and -tr searches carry the chosen system's -c, when the
// choice is the chart the cursor is on (the timeline asks in its lens).
func TestHouseChoiceReachesTimeline(t *testing.T) {
	sessionSandbox(t)
	stubHouseLookups(t)
	searcher := &mergeSearcher{}
	stubTimelineSearch(t, searcher)
	m := octoberSet(t, 0) // comparing, the cursor on ①: -t/-tr natals ride along
	m = loadTimeline(t, m)
	// The timeline follows its inputs while its tab is on screen.
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
	searcher.mu.Lock()
	before := len(searcher.cmds)
	searcher.mu.Unlock()
	if before == 0 {
		t.Fatalf("fixture: no timeline search ran (details visible: %v)", m.detailsVisible())
	}
	m, cmd := m.chooseHouseSystem(0, recipe.Campanus)
	m = pump(t, m, cmd)
	searcher.mu.Lock()
	after := append([]string(nil), searcher.cmds[before:]...)
	searcher.mu.Unlock()
	sawT, sawTR := false, false
	for _, argv := range after {
		fields := strings.Fields(argv)
		hasC := false
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "-c" && fields[i+1] == strconv.Itoa(int(recipe.Campanus)) {
				hasC = true
			}
		}
		for _, f := range fields {
			switch f {
			case "-t":
				sawT = true
				if !hasC {
					t.Errorf("an executed -t search lacks -c %d: %s", int(recipe.Campanus), argv)
				}
			case "-tr":
				sawTR = true
				if !hasC {
					t.Errorf("an executed -tr search lacks -c %d: %s", int(recipe.Campanus), argv)
				}
			}
		}
	}
	if !sawT || !sawTR {
		t.Errorf("a house choice re-ran -t: %v, -tr: %v; searches: %v", sawT, sawTR, after)
	}
}

// Pass-7 build note: after a choice the next committed columns match their casts.
func TestHouseChoiceInvalidatesShownColumns(t *testing.T) {
	sessionSandbox(t)
	spy := stubHouseLookups(t)
	m := settleComparingWith(t, 2, 0)
	if _, ok := m.bodyColumnsForScene(); !ok {
		t.Fatal("fixture: no comparing columns")
	}
	// Columns cast in another system are not this scene's columns, whatever
	// changed the system (a re-read of the file, not only a choice).
	stale := m
	stale.frames.records = []store.FrameRecord{chosen(m, 0, recipe.Porphyry)}
	if _, ok := stale.bodyColumnsForScene(); ok {
		t.Error("columns cast in the old system are presented as the scene's after the system changed under them")
	}
	spy.reset()
	m, cmd := m.chooseHouseSystem(0, recipe.Porphyry)
	if _, ok := m.bodyColumnsForScene(); ok {
		t.Error("columns cast in the old system are still presented as the scene's columns")
	}
	m = pump(t, m, cmd)
	cols, ok := m.bodyColumnsForScene()
	if !ok || len(cols) != 3 {
		t.Fatalf("no new columns after the recast: ok=%v n=%d", ok, len(cols))
	}
	// ① alone is recast in Porphyry; the others keep their own columns.
	if got := spy.lastOf(t, "n1"); got != recipe.Porphyry {
		t.Errorf("n1 was recast in %v, want Porphyry", got)
	}
	for _, c := range spy.snapshot() {
		if c.label != "n1" && c.houses == recipe.Porphyry {
			t.Errorf("%s was cast in ①'s Porphyry: %v", c.label, spy.snapshot())
		}
	}
}

func settleComparingWith(t *testing.T, anchored, cursor int) Model {
	t.Helper()
	m := selectedSet(t, anchored, true, cursor)
	return settleHouses(t, m)
}

// Pass-3 build note: the tag is per row, each row's own cast, selected or not.
func TestRowTagsFollowEachRowsOwnCast(t *testing.T) {
	spy := stubHouseLookups(t)
	spy.used = func(label string, asked recipe.HouseSystem) string {
		if label == "n2" && asked == recipe.Koch {
			return "Porphyry"
		}
		return asked.StoredName()
	}
	m := selectedSet(t, 3, true, 0)
	m.slots.SetSlotSelected(2, false) // ③ stays out of the group
	m.frames.records = []store.FrameRecord{chosen(m, 0, recipe.Placidus), chosen(m, 1, recipe.Koch), chosen(m, 2, recipe.Equal)}
	m = settleHouses(t, m)
	body := ansi.Strip(chartSlotsBodyTagged(m.slots, m.cursorIdx, false, testFrozenTime(), testPlaceBerlin(), m.lens, 110, m.cfg, m.rowTag))
	want := []string{"PlTr", "Po*Tr", "EqTr", "WhTr"}
	lines := strings.Split(body, "\n")
	if len(lines) != len(want) {
		t.Fatalf("%d rows, want %d:\n%s", len(lines), len(want), body)
	}
	for i, w := range want {
		if got := strings.TrimRight(lines[i], " "); !strings.HasSuffix(got, w) {
			t.Errorf("row %d = %q, want the tag %q", i+1, got, w)
		}
	}
}

// Pass-4 build note: the group's system is the only one any comparison call
// carries, and an unselected chart's own system never reaches them.
func TestUnselectedHouseDoesNotLeakIntoComparison(t *testing.T) {
	spy := stubHouseLookups(t)
	var raw []recipe.HouseSystem
	var mu sync.Mutex
	old := computeRawReport
	computeRawReport = func(_ context.Context, _ time.Time, _ location.PlaceInfo, lens lookup.Lens, _ lookup.RawReportRequest) (string, string, error) {
		mu.Lock()
		raw = append(raw, lens.Houses)
		mu.Unlock()
		return "report", "astrolog report", nil
	}
	t.Cleanup(func() { computeRawReport = old })

	m := selectedSet(t, 3, true, 2)
	m.slots.SetSlotSelected(2, false)
	m.frames.records = []store.FrameRecord{chosen(m, 0, recipe.Placidus), chosen(m, 2, recipe.Equal)}
	// The cursor is on the unselected chart: it alone is cast, in its own system.
	spy.reset()
	m = settleHouses(t, m)
	for _, c := range spy.snapshot() {
		if c.label != "n3" || c.houses != recipe.Equal {
			t.Errorf("single frame call %v, want only ③ in Equal", c)
		}
	}
	// The cursor moves into the group: ③'s Equal reaches no call; ① frames,
	// and the others keep their own (no record: Whole Sign).
	spy.reset()
	mu.Lock()
	raw = nil
	mu.Unlock()
	m = m.focusSlot(0)
	m, cmd := m.afterFocusChange()
	m = pump(t, m, cmd)
	if len(spy.snapshot()) == 0 {
		t.Fatal("the comparison frame executed nothing")
	}
	for _, c := range spy.snapshot() {
		want := recipe.WholeSign
		if c.label == "n1" {
			want = recipe.Placidus
		}
		if c.houses != want {
			t.Errorf("comparison call %v carried the wrong system, want %v", c, want)
		}
	}
	// The Details report of the focused chart follows the same lens.
	m.focused = PanelChart
	m.width, m.height = 132, 40
	if cmd := buildDetailsCmd(m, context.Background(), 1, 1); cmd != nil {
		cmd()
	}
	mu.Lock()
	defer mu.Unlock()
	for _, h := range raw {
		if h != recipe.Placidus {
			t.Errorf("the Details report carried %v, want Placidus", h)
		}
	}
}

// Pass-4 build note: the executed wheel follows the focused chart's system.
func TestWheelChangesWithFocusedHouseSystem(t *testing.T) {
	stubHouseLookups(t)
	var mu sync.Mutex
	var drawn []recipe.HouseSystem
	oldS, oldShow := renderWheelImage, showWheel
	t.Cleanup(func() { renderWheelImage, showWheel = oldS, oldShow })
	renderWheelImage = func(_ context.Context, _ time.Time, _ location.PlaceInfo, lens lookup.Lens, _ lookup.WheelHighlight) ([]byte, error) {
		mu.Lock()
		drawn = append(drawn, lens.Houses)
		mu.Unlock()
		return []byte("wheel"), nil
	}
	showWheel = func([]byte) (string, error) { return "/tmp/stub.png", nil }

	m := listModel(t, 2, 0)
	for i := 0; i < 3; i++ {
		m.slots.SetSlotSelected(i, false)
	}
	m.frames.records = []store.FrameRecord{chosen(m, 0, recipe.Koch), chosen(m, 1, recipe.Equal)}
	m.cfg = Config{Color: ColorNever}
	m = openWheel(t, m)
	m, cmd := m.cycleSlot(1)
	m = pump(t, m, cmd)
	mu.Lock()
	defer mu.Unlock()
	if len(drawn) < 2 || drawn[0] != recipe.Koch || drawn[len(drawn)-1] != recipe.Equal {
		t.Errorf("wheel renders carried %v, want Koch then Equal", drawn)
	}
}

// C11a and the pass-6 build note.
func TestReplacedChartDoesNotInheritHouseChoice(t *testing.T) {
	spy := stubHouseLookups(t)
	m := listModel(t, 1, 0)
	slot := m.slots.All()[0]
	rec := chosen(m, 0, recipe.Koch)
	rec.When = rec.When.Add(-time.Hour) // the file now holds a different chart
	m.frames.records = []store.FrameRecord{rec}
	m.frames = m.frames.withFile("synthetic-1.as", fileFrame{when: slot.Time, houses: recipe.Equal, has: true})
	if own := m.ownHouses(slot); own.hs != recipe.Equal || !own.rejected {
		t.Fatalf("own = %+v, want the file's Equal and the record rejected", own)
	}
	m = settleHouses(t, m)
	if got := spy.lastOf(t, "n1"); got != recipe.Equal {
		t.Errorf("the first cast carried %v, want the file's Equal", got)
	}
	if n := logHas(m, "different chart"); n != 1 {
		t.Errorf("the rejected record was named %d times, want once: %v", n, m.logLines)
	}
}

// C11b (ui half).
func TestInvalidFrameEntryIsLogged(t *testing.T) {
	sessionSandbox(t)
	seedSession(t, "version: 3\nfloating:\n  selected: false\nframes:\n"+
		"  - file: good.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n"+
		"  - file: bad.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Nonsense\n    from: chosen\n")
	m := startedModel(t, withLocation(realPerimeter(t)))
	if n := logHas(m, "frames entry was dropped"); n != 1 {
		t.Errorf("the dropped entry was named %d times, want once: %v", n, m.logLines)
	}
	if _, ok := m.frames.find("good.as", false); !ok {
		t.Error("the good entry beside the bad one was lost")
	}
	if len(refusedFilesIn(t)) != 1 {
		t.Errorf("the file with a dropped entry was not kept aside: %v", refusedFilesIn(t))
	}
}

func refusedFilesIn(t *testing.T) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(store.SessionDir(), "session.yaml.refused-*"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// C12.
func TestHouseSourceIsLoggedOnce(t *testing.T) {
	stubHouseLookups(t)
	m := listModel(t, 1, 0)
	m.frames = m.frames.withFile("synthetic-1.as", fileFrame{when: m.slots.All()[0].Time, houses: recipe.Koch, has: true})
	m = settleHouses(t, m)
	m = settleHouses(t, m)
	if n := logHas(m, "from its chart file"); n != 1 {
		t.Errorf("the file source was logged %d times, want once: %v", n, m.logLines)
	}
	d := listModel(t, 1, 0).applyFrameConfig(store.FrameConfig{Houses: "Equal", HousesPresent: true})
	d = settleHouses(t, d)
	d = settleHouses(t, d)
	if n := logHas(d, "from preferences"); n != 1 {
		t.Errorf("the default source was logged %d times, want once: %v", n, d.logLines)
	}
	f := settleHouses(t, listModel(t, 1, 0))
	if n := logHas(f, "houses, from"); n != 0 {
		t.Errorf("a Whole Sign fallback named a source: %v", f.logLines)
	}
}

// C10.
func TestRecordsOfDeletedChartsArePruned(t *testing.T) {
	dir := titleLibrary(t, nil, map[string]string{
		"present.as": "not a chart astrolog can read\n", // exists, but a listing drops it
		"listed.as":  kochChart("Listed One", true),
	})
	sessionSandbox(t)
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	rec := func(name string) store.FrameRecord {
		return store.FrameRecord{File: name, When: frameWhenUI, Houses: recipe.Koch, HasHouses: true, From: store.FrameFromChosen}
	}
	build := func() Model {
		m := listModel(t, 1, 0)
		m.frames.records = []store.FrameRecord{rec("gone.as"), rec("present.as"), rec("listed.as")}
		return m
	}
	files := func(m Model) []string {
		var out []string
		for _, r := range m.frameRecordsToSave() {
			out = append(out, r.File)
		}
		return out
	}
	t.Run("after a listing that lists a chart, only a vanished file's record goes", func(t *testing.T) {
		m := build()
		m, _ = applyChartsLoaded(m, ChartsLoadedMsg{Charts: []store.NamedChart{{ID: "listed", Name: "Listed One"}}, Gen: m.pickerGen})
		if got := files(m); strings.Join(got, ",") != "present.as,listed.as" {
			t.Errorf("records = %v, want gone.as dropped and the present-but-dropped file kept", got)
		}
	})
	t.Run("no listing yet: nothing is dropped", func(t *testing.T) {
		if got := files(build()); len(got) != 3 {
			t.Errorf("records = %v", got)
		}
	})
	t.Run("a failed or empty listing drops nothing", func(t *testing.T) {
		m := build()
		m, _ = applyChartsLoaded(m, ChartsLoadedMsg{Err: os.ErrPermission, Gen: m.pickerGen})
		m, _ = applyChartsLoaded(m, ChartsLoadedMsg{Gen: m.pickerGen})
		if got := files(m); len(got) != 3 {
			t.Errorf("records = %v", got)
		}
	})
	t.Run("the pruned set is what the session write carries", func(t *testing.T) {
		m := build()
		m, _ = applyChartsLoaded(m, ChartsLoadedMsg{Charts: []store.NamedChart{{ID: "listed"}}, Gen: m.pickerGen})
		if err := m.writeSessionNow(); err != nil {
			t.Fatal(err)
		}
		s, _, err := store.LoadSession()
		if err != nil || len(s.Frames) != 2 {
			t.Fatalf("session frames = %+v, %v", s.Frames, err)
		}
	})
	t.Run("a chart chosen, closed, quit, relaunched and reopened keeps its choice", func(t *testing.T) {
		sessionSandbox(t)
		t.Setenv("SEXTILE_CHARTS_DIR", dir)
		stubHouseLookups(t)
		m := openedPicker(t)
		m, cmd := pickRow(t, m, "listed.as")
		m = runToAnchor(t, m, cmd)
		idx := m.slots.AnchoredCount() - 1
		m, _ = m.chooseHouseSystem(idx, recipe.Porphyry)
		m.slots.RemoveAnchored(idx) // closed
		m.cursorIdx = 0
		m, _ = applyChartsLoaded(m, ChartsLoadedMsg{Charts: []store.NamedChart{{ID: "listed"}}, Gen: m.pickerGen})
		_, qcmd := m.quit()
		assertQuits(t, qcmd)
		relaunched := startedModel(t, withLocation(realPerimeter(t)))
		r := openedPickerOn(t, relaunched)
		r, cmd = pickRow(t, r, "listed.as")
		r = runToAnchor(t, r, cmd)
		slot := r.slots.All()[r.slots.AnchoredCount()-1]
		if got := r.ownHouses(slot); got.hs != recipe.Porphyry || got.src != houseChosen {
			t.Errorf("reopened chart own = %+v, want the remembered Porphyry", got)
		}
	})
}

var frameWhenUI = time.Date(1995, 3, 20, 10, 0, 0, 0, time.UTC)

// openedPickerOn opens the picker on an existing model.
func openedPickerOn(t *testing.T, m Model) Model {
	t.Helper()
	m.startup = startupReady
	u, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = u.(Model)
	if cmd == nil {
		t.Fatal("Ctrl+O dispatched no load")
	}
	m, _ = runCmd(t, m, cmd)
	return m
}

// kochChart is a chart file naming Koch (or none), in the shape of Equinox-2026-09.as.
func kochChart(title string, withC bool) string {
	text := "@AI780  ; Astrolog chart info.\n-qb Mar 20 1995 12:00pm DT 1E   8:48E 53:04N\n-zi \"" + title + "\" \"Bremen, Germany\"\n"
	if withC {
		text += "-c Koch\n"
	}
	return text
}

// The rename tests: the real Astrolog rewrites the real file, and a rewrite
// drops the file's house line (measured 2026-10-08), so the record is the only
// thing that keeps the chart's houses.
func fileNamesNoSystem(t *testing.T, dir, file string) {
	t.Helper()
	cf, err := lookup.ReadChartFile(context.Background(), file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	if cf.HasHouses {
		t.Fatalf("premise: the rewritten %s still names %v", file, cf.Houses)
	}
}

func TestRenameKeepsTheFilesHouseSystem(t *testing.T) {
	t.Run("Ctrl+R in the picker", func(t *testing.T) {
		sessionSandbox(t)
		dir := titleLibrary(t, nil, map[string]string{"K.as": kochChart("Client K", true)})
		stubHouseLookups(t)
		m := openedPicker(t)
		for i, c := range m.picker.matches {
			if c.Source.File == "K.as" {
				m.picker.cursor = i
			}
		}
		m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
		if m.startup != startupTitleOpen || !m.title.rename {
			t.Fatalf("Ctrl+R did not open the rename form (startup %v)", m.startup)
		}
		m = typeText(t, m, "Client Kay")
		m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		m, _ = finishRename(t, m, cmd)
		assertPreserved(t, m, dir, "K.as")
	})
	t.Run("the title form of a pick", func(t *testing.T) {
		sessionSandbox(t)
		dir := titleLibrary(t, nil, map[string]string{"U.as": kochChart("", true)})
		stubHouseLookups(t)
		m := openedPicker(t)
		m, _ = pickRow(t, m, "U.as")
		if m.startup != startupTitleOpen {
			t.Fatalf("the untitled pick did not open the title form (startup %v)", m.startup)
		}
		m = typeText(t, m, "Client U")
		m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // the second step
		m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		m, _ = finishRename(t, m, cmd)
		assertPreserved(t, m, dir, "U.as")
	})
	t.Run("a chart that already has a choice is not touched", func(t *testing.T) {
		sessionSandbox(t)
		titleLibrary(t, nil, map[string]string{"K.as": kochChart("Client K", true)})
		m := openedPicker(t)
		m.frames.records = []store.FrameRecord{{File: "K.as", When: frameWhenUI, Houses: recipe.Equal, HasHouses: true, From: store.FrameFromChosen}}
		for i, c := range m.picker.matches {
			if c.Source.File == "K.as" {
				m.picker.cursor = i
			}
		}
		m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
		m = typeText(t, m, "Client Kay")
		m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		m, _ = finishRename(t, m, cmd)
		rec, _ := m.frames.find("K.as", false)
		if rec.Houses != recipe.Equal || rec.From != store.FrameFromChosen {
			t.Errorf("the chosen record was replaced: %+v", rec)
		}
	})
	t.Run("the preservation write fails: the rename does not proceed", func(t *testing.T) {
		sessionSandbox(t)
		dir := titleLibrary(t, nil, map[string]string{"K.as": kochChart("Client K", true)})
		before := fileText(t, filepath.Join(dir, "K.as"))
		if err := os.MkdirAll(filepath.Dir(store.SessionDir()), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(store.SessionDir(), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		m := openedPicker(t)
		for i, c := range m.picker.matches {
			if c.Source.File == "K.as" {
				m.picker.cursor = i
			}
		}
		m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
		m = typeText(t, m, "Client Kay")
		m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		m, _ = finishRename(t, m, cmd)
		if got := fileText(t, filepath.Join(dir, "K.as")); got != before {
			t.Errorf("the chart file was rewritten although its houses could not be saved:\n%s", got)
		}
		noBackup(t, filepath.Join(dir, "K.as"))
		if m.title.refusal == "" {
			t.Error("the form does not say why the rename was refused")
		}
		if logHas(m, "house system") == 0 {
			t.Errorf("the error lane does not say why: %v", m.logLines)
		}
	})
}

func assertPreserved(t *testing.T, m Model, dir, file string) {
	t.Helper()
	rec, ok := m.frames.find(file, false)
	if !ok || rec.Houses != recipe.Koch || rec.From != store.FrameFromFile {
		t.Fatalf("record for %s = %+v (found %v), want Koch from file", file, rec, ok)
	}
	s, _, err := store.LoadSession()
	if err != nil || len(s.Frames) != 1 || s.Frames[0].File != file || s.Frames[0].From != store.FrameFromFile {
		t.Errorf("session frames = %+v (%v), want the preserving record on disk", s.Frames, err)
	}
	fileNamesNoSystem(t, dir, file)
}

// Pass-4 build note.
func TestFailedRenameDoesNotFreezeFileHouseSystem(t *testing.T) {
	sessionSandbox(t)
	dir := titleLibrary(t, nil, map[string]string{"K.as": kochChart("Client K", true)})
	stubHouseLookups(t)
	m := openedPicker(t)
	for i, c := range m.picker.matches {
		if c.Source.File == "K.as" {
			m.picker.cursor = i
		}
	}
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = typeText(t, m, "Client Kay")
	// The file changes after the picker listed it: the rename is refused, after
	// the preservation record was written.
	path := filepath.Join(dir, "K.as")
	if err := os.WriteFile(path, []byte(kochChart("Client K", true)+"; changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = finishRename(t, m, cmd)
	if m.title.refusal == "" {
		t.Fatal("fixture: the rename was not refused")
	}
	if _, ok := m.frames.find("K.as", false); ok {
		t.Error("the failed rename left its record in the model")
	}
	if s, ok, _ := store.LoadSession(); ok && len(s.Frames) != 0 {
		t.Errorf("the failed rename left its record on disk: %+v", s.Frames)
	}
	// The file's line is edited afterwards: the new system is what is read.
	if err := os.WriteFile(path, []byte(strings.Replace(kochChart("Client K", true), "Koch", "Equal", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	r, _ := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	r, cmd = pickRow(t, r, "K.as")
	r = runToAnchor(t, r, cmd)
	slot := r.slots.All()[r.slots.AnchoredCount()-1]
	if got := r.ownHouses(slot); got.hs != recipe.Equal || got.src != houseFile {
		t.Errorf("own = %+v, want the edited file's Equal", got)
	}
}

// Pass-5 build note: Esc while the rename is in flight.
func TestAbandonedRenameKeepsHouseRecordConsistent(t *testing.T) {
	open := func(t *testing.T) (Model, string, tea.Cmd) {
		sessionSandbox(t)
		dir := titleLibrary(t, nil, map[string]string{"K.as": kochChart("Client K", true)})
		m := openedPicker(t)
		for i, c := range m.picker.matches {
			if c.Source.File == "K.as" {
				m.picker.cursor = i
			}
		}
		m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
		m = typeText(t, m, "Client Kay")
		return m, dir, nil
	}
	t.Run("Esc after the rewrite was issued: it lands and the record stays", func(t *testing.T) {
		m, dir, _ := open(t)
		m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		m, rewrite := runCmd(t, m, cmd)
		m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
		m, _ = runCmd(t, m, rewrite)
		assertPreserved(t, m, dir, "K.as")
	})
	t.Run("Esc after the rewrite was issued and it fails: the record goes", func(t *testing.T) {
		m, dir, _ := open(t)
		path := filepath.Join(dir, "K.as")
		if err := os.WriteFile(path, []byte(kochChart("Client K", true)+"; changed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		m, rewrite := runCmd(t, m, cmd)
		if _, ok := m.frames.find("K.as", false); !ok {
			t.Fatal("fixture: the record was not in the model while the rewrite was in flight")
		}
		m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
		m, _ = runCmd(t, m, rewrite)
		if _, ok := m.frames.find("K.as", false); ok {
			t.Error("a failed late rename left its record in the model")
		}
		if s, ok, _ := store.LoadSession(); ok && len(s.Frames) != 0 {
			t.Errorf("a failed late rename left its record on disk: %+v", s.Frames)
		}
	})
	t.Run("Esc before the read lands: nothing is written", func(t *testing.T) {
		m, dir, _ := open(t)
		before := fileText(t, filepath.Join(dir, "K.as"))
		m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
		m, next := runCmd(t, m, cmd)
		if next != nil {
			t.Error("a rewrite was issued for a rename Esc walked away from")
		}
		if _, ok := m.frames.find("K.as", false); ok {
			t.Error("an abandoned rename left a record")
		}
		if got := fileText(t, filepath.Join(dir, "K.as")); got != before {
			t.Error("an abandoned rename rewrote the file")
		}
	})
}

// Pass-7 build note: a chart no slot holds.
func TestUnopenedChartRenamePreservesFileHouseSystem(t *testing.T) {
	sessionSandbox(t)
	titleLibrary(t, nil, map[string]string{"K.as": kochChart("Client K", true)})
	spy := stubHouseLookups(t)
	m := openedPicker(t)
	if m.slots.AnchoredCount() != 0 {
		t.Fatal("fixture: a slot holds a chart")
	}
	for i, c := range m.picker.matches {
		if c.Source.File == "K.as" {
			m.picker.cursor = i
		}
	}
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = typeText(t, m, "Client Kay")
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = finishRename(t, m, cmd) // the rename lands; the picker reloads
	m, _ = runCmd(t, m, cmd)
	m, cmd = pickRow(t, m, "K.as")
	m = runToAnchor(t, m, cmd)
	if got := m.ownHouses(m.slots.All()[0]).hs; got != recipe.Koch {
		t.Errorf("opened after the rename it is cast in %v, want Koch", got)
	}
	m = pump(t, m, nil)
	if calls := spy.snapshot(); len(calls) > 0 {
		for _, c := range calls {
			if c.houses == recipe.WholeSign {
				t.Errorf("a cast after the rename was in Whole Sign: %v", calls)
			}
		}
	}
}

// Pass-3 build note: on relaunch the FIRST cast of a restored chart is already
// in its file's system.
func TestRestoredChartFirstCastUsesFileSystem(t *testing.T) {
	sessionSandbox(t)
	titleLibrary(t, nil, map[string]string{"K.as": kochChart("Client K", true)})
	lib, err := lookup.LoadChartLibrary()
	if err != nil || len(lib.Charts) != 1 {
		t.Fatalf("fixture library: %+v %v", lib.Charts, err)
	}
	row := lib.Charts[0]
	read, err := lookup.ResolvePickedChart(context.Background(), row)
	if err != nil || !read.HasHouses || read.Houses != recipe.Koch {
		t.Fatalf("fixture: the file reads as %+v (%v), want Koch", read, err)
	}
	rec := store.SessionChart{ID: row.ID, Name: row.Name, When: read.When, Place: row.Place, Source: row.Source}
	if err := store.SaveSession(store.Session{FloatingSelected: true, Slots: []store.SessionSlot{{Chart: rec, Usable: true, Selected: true}}}); err != nil {
		t.Fatal(err)
	}
	spy := stubHouseLookups(t)

	var m tea.Model = NewModelWithClock(Config{Accessible: true}, FrozenClock(sessionFrozen))
	m, _ = m.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP, row.Place})})
	msg := readPerimeterCmd(false)()
	p, ok := msg.(ingest.StartupPerimeterReadMsg)
	if !ok {
		t.Fatalf("perimeter read emitted %T", msg)
	}
	if p.LocationCandidate.Place.Empty() {
		p.LocationCandidate = location.Candidate{Source: "stored-location", Place: berlinRaw}
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
	m, cmd := m.Update(p)
	// The restored chart's row; now's row keeps its own Whole Sign.
	rowOfK := func(screen string) string {
		for _, l := range strings.Split(screen, "\n") {
			if strings.Contains(l, "Client K") {
				return l
			}
		}
		return ""
	}
	// The FIRST rendered frame, before any cast has landed.
	if first := ansi.Strip(m.(Model).View()); !strings.Contains(rowOfK(first), "KoTr") || strings.Contains(rowOfK(first), "WhTr") {
		t.Errorf("the first frame tags the restored chart wrongly, want Ko and never Wh:\n%s", first)
	}
	pumped := pump(t, m.(Model), cmd)
	if later := ansi.Strip(pumped.View()); !strings.Contains(rowOfK(later), "KoTr") || strings.Contains(rowOfK(later), "WhTr") {
		t.Errorf("the settled frame tags the restored chart wrongly:\n%s", later)
	}
	calls := spy.snapshot()
	if len(calls) == 0 {
		t.Fatal("nothing was cast")
	}
	for _, c := range calls {
		if c.label != "now" && c.houses != recipe.Koch {
			t.Errorf("the restored chart was cast in %v, want its file's Koch; calls %v", c.houses, calls)
		}
	}
	if first := calls[0]; first.houses == recipe.WholeSign {
		t.Errorf("the first cast was in Whole Sign: %v", first)
	}
	if pumped.slots == nil || pumped.slots.AnchoredCount() != 1 {
		t.Fatal("the chart was not restored")
	}
}

// Pass-6 build note: a failed chart-file read casts nothing with a guess.
func TestChartFileReadFailureCastsNothing(t *testing.T) {
	sessionSandbox(t)
	spy := stubHouseLookups(t)
	m := readyModelWithSlots(t, berlinP)
	gone := store.NamedChart{ID: "gone", Name: "Gone", Year: 1995, Place: fixtureOneP, PlaceLabel: "X",
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "gone.as"}}
	m, cmd := applyChartPicked(m, ChartPickedMsg{Chart: gone})
	m = pump(t, m, cmd)
	if m.slots.AnchoredCount() != 0 {
		t.Error("an unreadable chart was anchored")
	}
	if logHas(m, "Couldn't open that chart") != 1 {
		t.Errorf("the existing failure path did not report it: %v", m.logLines)
	}
	if len(spy.snapshot()) != 0 {
		t.Errorf("a chart whose file could not be read was cast: %v", spy.snapshot())
	}
}

// withLocation gives a real perimeter read a location that resolves on its own,
// so startup builds the workspace and restores the session.
func withLocation(p ingest.StartupPerimeterReadMsg) ingest.StartupPerimeterReadMsg {
	p.LocationCandidate = location.Candidate{Source: "stored-location", Place: berlinRaw}
	return p
}

// finishRename runs a rename's commands until the write has landed: a chart
// that needs its houses preserved takes one more step than one that does not.
func finishRename(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	for i := 0; i < 3 && cmd != nil; i++ {
		m, cmd = runCmd(t, m, cmd)
		if !m.title.saving {
			break
		}
	}
	return m, cmd
}

// restoredK is a relaunch whose session holds K.as (a -c Koch chart) and
// whose startup read of that file is made by read.
func restoredK(t *testing.T, read func(context.Context, string) (lookup.PickedChart, error)) (Model, tea.Cmd, *houseSpy, string) {
	t.Helper()
	sessionSandbox(t)
	dir := titleLibrary(t, nil, map[string]string{"K.as": kochChart("Client K", true)})
	lib, err := lookup.LoadChartLibrary()
	if err != nil || len(lib.Charts) != 1 {
		t.Fatalf("fixture library: %+v %v", lib.Charts, err)
	}
	row := lib.Charts[0]
	real, err := lookup.ResolvePickedChart(context.Background(), row)
	if err != nil {
		t.Fatal(err)
	}
	rec := store.SessionChart{ID: row.ID, Name: row.Name, When: real.When, Place: row.Place, Source: row.Source}
	if err := store.SaveSession(store.Session{FloatingSelected: true, Slots: []store.SessionSlot{{Chart: rec, Usable: true, Selected: true}}}); err != nil {
		t.Fatal(err)
	}
	spy := stubHouseLookups(t)
	old := readChartFile
	t.Cleanup(func() { readChartFile = old })
	if read != nil {
		readChartFile = read
	}
	var m tea.Model = NewModelWithClock(Config{Accessible: true}, FrozenClock(sessionFrozen))
	m, _ = m.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP, row.Place})})
	p := withLocation(readPerimeterCmd(false)().(ingest.StartupPerimeterReadMsg))
	readChartFile = old
	m, cmd := m.Update(p)
	return m.(Model), cmd, spy, dir
}

// F1: a restored chart whose startup read FAILED has no house system yet. It is
// not cast with a guess: the cast fails the way any failed compute does, and
// the re-check that follows the first frame resolves it and casts it.
func TestRestoredChartFileReadFailureCastsNothing(t *testing.T) {
	m, cmd, spy, _ := restoredK(t, func(context.Context, string) (lookup.PickedChart, error) {
		return lookup.PickedChart{}, errors.New("astrolog exploded")
	})
	msg, ok := buildTextCmd(m, context.Background(), 1, 1)().(FirstFrameReadyMsg)
	if !ok || msg.Err == nil {
		t.Fatalf("the unresolved chart was cast: %+v", msg)
	}
	if calls := spy.snapshot(); len(calls) != 0 {
		t.Fatalf("a chart with no resolved house system was cast: %v", calls)
	}
	// The failure frame, committed the way any failed compute is, names the
	// file on the error lane. (Run on a copy: in a live run the re-check can
	// supersede the failed frame before it commits.)
	failed, _ := applyTextCommit(m, msg)
	found := false
	for _, l := range failed.logLines {
		if l.lane == laneError && strings.Contains(l.text, "K.as") && strings.Contains(l.text, "could not be read") {
			found = true
		}
	}
	if !found {
		t.Errorf("the error lane does not name the file and why: %v", failed.logLines)
	}
	m = pump(t, m, cmd)
	calls := spy.snapshot()
	if len(calls) == 0 {
		t.Fatal("the re-check never cast the chart")
	}
	for _, c := range calls {
		if c.label != "now" && c.houses != recipe.Koch {
			t.Errorf("cast %v after the re-check, want its file's Koch: %v", c, calls)
		}
	}
}

// F2: a record made for another chart under the same file name does not stop
// the file being read, so the file's system applies and a rename preserves it.
func TestStaleRecordStillReadsTheFile(t *testing.T) {
	sessionSandbox(t)
	dir := titleLibrary(t, nil, map[string]string{"K.as": kochChart("Client K", true)})
	stubHouseLookups(t)
	stale := store.FrameRecord{File: "K.as", When: frameWhenUI.Add(-24 * time.Hour), Houses: recipe.Equal, HasHouses: true, From: store.FrameFromChosen}
	m := openedPicker(t)
	m.frames.records = []store.FrameRecord{stale}
	for i, c := range m.picker.matches {
		if c.Source.File == "K.as" {
			m.picker.cursor = i
		}
	}
	// First cast: the stale record is rejected and the file's Koch applies.
	p, cmd := pickRow(t, m, "K.as")
	p = runToAnchor(t, p, cmd)
	slot := p.slots.All()[p.slots.AnchoredCount()-1]
	if own := p.ownHouses(slot); own.hs != recipe.Koch || !own.rejected {
		t.Fatalf("own = %+v, want the file's Koch with the record rejected", own)
	}
	// Rename: the file is read, and its Koch is preserved.
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = typeText(t, m, "Client Kay")
	m, cmd = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = finishRename(t, m, cmd)
	assertPreserved(t, m, dir, "K.as")
}

// F3: from the moment the rewrite is issued, any session write already holds
// the preservation record.
func TestQuitDuringRenameKeepsThePreservationRecord(t *testing.T) {
	sessionSandbox(t)
	dir := titleLibrary(t, nil, map[string]string{"K.as": kochChart("Client K", true)})
	before := fileText(t, filepath.Join(dir, "K.as"))
	m := openedPicker(t)
	for i, c := range m.picker.matches {
		if c.Source.File == "K.as" {
			m.picker.cursor = i
		}
	}
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = typeText(t, m, "Client Kay")
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, rewrite := runCmd(t, m, cmd) // the read lands; the rewrite is issued, not run
	if rewrite == nil || !m.title.saving {
		t.Fatalf("no rewrite was issued (saving %v)", m.title.saving)
	}
	if got := fileText(t, filepath.Join(dir, "K.as")); got != before {
		t.Fatalf("the chart file was rewritten before the record was in the session:\n%s", got)
	}
	_, qcmd := m.quit()
	assertQuits(t, qcmd)
	s, ok, err := store.LoadSession()
	if err != nil || !ok || len(s.Frames) != 1 || s.Frames[0].File != "K.as" || s.Frames[0].Houses != recipe.Koch || s.Frames[0].From != store.FrameFromFile {
		t.Fatalf("the session written at quit = %+v (%v %v), want the preserving record", s.Frames, ok, err)
	}
}

// F4: picking an already-open chart whose file's house line changed recasts it.
func TestRepickedChartWithChangedHouseLineRecasts(t *testing.T) {
	sessionSandbox(t)
	dir := titleLibrary(t, nil, map[string]string{"K.as": kochChart("Client K", true)})
	spy := stubHouseLookups(t)
	m := openedPicker(t)
	m, cmd := pickRow(t, m, "K.as")
	m, cmd = runCmd(t, m, cmd)
	m = pump(t, m, cmd)
	label := ""
	for _, c := range spy.snapshot() {
		if c.label != "now" {
			label = c.label
		}
	}
	if label == "" {
		t.Fatalf("fixture: the chart was never cast: %v", spy.snapshot())
	}
	if got := spy.lastOf(t, label); got != recipe.Koch {
		t.Fatalf("fixture: first cast in %v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "K.as"), []byte(strings.Replace(kochChart("Client K", true), "Koch", "Equal", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	m = openedPickerOn(t, m)
	spy.reset()
	m, cmd = pickRow(t, m, "K.as")
	m, cmd = runCmd(t, m, cmd)
	m = pump(t, m, cmd)
	calls := spy.snapshot()
	if len(calls) == 0 {
		t.Fatal("the re-pick recast nothing")
	}
	if got := spy.lastOf(t, label); got != recipe.Equal {
		t.Errorf("after the re-pick the chart was cast in %v, want the file's new Equal", got)
	}
}

// Review round 2: a rename trusts no cached read. K.as was read once as chart
// A (a record for A exists), then replaced on disk by chart B naming Koch; the
// rename re-reads the file, finds the record is not B's, and preserves B's Koch.
func TestRenameRereadsAReplacedFile(t *testing.T) {
	for _, route := range []string{"Ctrl+R in the picker", "the title form of a pick"} {
		t.Run(route, func(t *testing.T) {
			sessionSandbox(t)
			titled := route == "Ctrl+R in the picker"
			titleA := "Client A"
			if !titled {
				titleA = ""
			}
			dir := titleLibrary(t, nil, map[string]string{"K.as": kochChart(titleA, false)})
			path := filepath.Join(dir, "K.as")
			a, err := lookup.ReadChartFile(context.Background(), "K.as")
			if err != nil {
				t.Fatal(err)
			}
			m := openedPicker(t)
			// What the model last knew of K.as: chart A, with a record for A.
			m.frames = m.frames.withFile("K.as", fileFrame{when: a.When})
			m.frames.records = []store.FrameRecord{{File: "K.as", When: a.When, Houses: recipe.Equal, HasHouses: true, From: store.FrameFromChosen}}
			// K.as is replaced by chart B, which names Koch; the picker is not
			// reloaded, but its fingerprint is the replaced file's, so the
			// rewrite itself is not refused.
			replacement := strings.Replace(kochChart(titleA, true), "12:00pm", "1:00pm", 1)
			if err := os.WriteFile(path, []byte(replacement), 0o600); err != nil {
				t.Fatal(err)
			}
			b, err := lookup.ReadChartFile(context.Background(), "K.as")
			if err != nil || !b.HasHouses || b.Houses != recipe.Koch || b.When.Equal(a.When) {
				t.Fatalf("fixture: B reads as %+v (%v), A at %v", b, err, a.When)
			}
			sum := sha256.Sum256([]byte(replacement))
			m.picker.seen = map[string]string{"K.as": hex.EncodeToString(sum[:])}
			for i, c := range m.picker.matches {
				if c.Source.File == "K.as" {
					m.picker.cursor = i
				}
			}
			if titled {
				m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
			} else {
				var cmd tea.Cmd
				m, cmd = pickRow(t, m, "K.as")
				_ = cmd
			}
			if m.startup != startupTitleOpen {
				t.Fatalf("no form opened (startup %v)", m.startup)
			}
			m = typeText(t, m, "Client B")
			if !titled {
				m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // a pick confirms first; a rename writes at once
			}
			m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
			m, _ = finishRename(t, m, cmd)
			rec, ok := m.frames.find("K.as", false)
			if !ok || rec.Houses != recipe.Koch || rec.From != store.FrameFromFile || !rec.When.Equal(b.When) {
				t.Fatalf("record = %+v (%v), want Koch from file at B's moment %v", rec, ok, b.When)
			}
			s, _, err := store.LoadSession()
			if err != nil || len(s.Frames) != 1 || s.Frames[0].Houses != recipe.Koch || !s.Frames[0].When.Equal(b.When) {
				t.Errorf("session frames = %+v (%v), want the Koch record for B", s.Frames, err)
			}
			// The next cast of B is Koch although the rewrite dropped its line.
			fileNamesNoSystem(t, dir, "K.as")
			m = m.withFileForTest("K.as", fileFrame{when: b.When})
			slotChart := store.SessionChart{ID: "k", Name: "Client B", Source: store.ChartSource{Kind: store.ChartKindFile, File: "K.as"}}
			if err := m.slots.AddAnchored(slotChart, b.When, berlinP); err != nil {
				t.Fatal(err)
			}
			if got := m.ownHouses(m.slots.All()[0]); got.hs != recipe.Koch {
				t.Errorf("B is cast in %v, want Koch", got.hs)
			}
		})
	}
}

func (m Model) withFileForTest(file string, ff fileFrame) Model {
	m.frames = m.frames.withFile(file, ff)
	return m
}
