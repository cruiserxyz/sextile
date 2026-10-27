package ui

// the specification. The pure-model tests use stubbed lookups that record
// the lens (so the zodiac) each executed call carried; the end-to-end tests run
// the real Astrolog. All chart values are constructed synthetic test inputs.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

type frameCall struct {
	label  string
	houses recipe.HouseSystem
	zodiac recipe.Zodiac
}

// frameSpy records, for every stubbed lookup, which chart it was for and the
// house system and zodiac its lens carried.
type frameSpy struct {
	mu    sync.Mutex
	calls []frameCall
	used  func(label string, asked recipe.HouseSystem) string
}

func (s *frameSpy) record(label string, l lookup.Lens) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, frameCall{label, l.Houses, l.Zodiac})
}

func (s *frameSpy) snapshot() []frameCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]frameCall(nil), s.calls...)
}

func (s *frameSpy) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = nil
}

func (s *frameSpy) lastOf(t *testing.T, label string) recipe.Zodiac {
	t.Helper()
	var out []recipe.Zodiac
	for _, c := range s.snapshot() {
		if c.label == label {
			out = append(out, c.zodiac)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no executed call for %s; calls = %v", label, s.snapshot())
	}
	return out[len(out)-1]
}

// zodiacLongitude is the Sun longitude the stubs return: the chart's year, plus
// a hundredth of a degree per zodiac, so a committed frame names its cast.
func zodiacLongitude(year int, z recipe.Zodiac) float64 {
	return float64(year%360) + 0.01*float64(z)
}

// stubFrameLookups answers every text compute at once, recording both parts of
// the frame each carried.
func stubFrameLookups(t *testing.T) *frameSpy {
	t.Helper()
	spy := &frameSpy{}
	oldN, oldT, oldO, oldS := firstCallNatalPositions, firstCallTransits, overlayCrossAspects, synastryCrossAspects
	t.Cleanup(func() {
		firstCallNatalPositions, firstCallTransits, overlayCrossAspects, synastryCrossAspects = oldN, oldT, oldO, oldS
	})
	firstCallNatalPositions = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, lens lookup.Lens) (lookup.Result, error) {
		label := natalLabel(c)
		spy.record(label, lens)
		used := ""
		if spy.used != nil {
			used = spy.used(label, lens.Houses)
		}
		return lookup.Result{
			Positions:  []aspect.Position{{Body: aspect.Sun, Longitude: zodiacLongitude(c.When.Year(), lens.Zodiac)}},
			Cusps:      aspect.Cusps{100 + float64(lens.Houses)},
			HousesUsed: used,
			Commands:   []string{fmt.Sprintf("astrolog natal %s -c %d zodiac %s", label, int(lens.Houses), lens.Zodiac.Name())},
		}, nil
	}
	firstCallTransits = func(_ context.Context, _ *lookup.Probers, when time.Time, _ location.PlaceInfo, lens lookup.Lens) (lookup.Result, error) {
		spy.record("now", lens)
		return lookup.Result{
			Positions: []aspect.Position{{Body: aspect.Sun, Longitude: zodiacLongitude(when.Year(), lens.Zodiac) + 0.5}},
			Cusps:     aspect.Cusps{200 + float64(lens.Houses)},
			Commands:  []string{fmt.Sprintf("astrolog transit -c %d zodiac %s", int(lens.Houses), lens.Zodiac.Name())},
		}, nil
	}
	overlayCrossAspects = func(_ context.Context, lens lookup.Lens, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo) (lookup.OverlayAspects, error) {
		spy.record("cross", lens)
		return lookup.OverlayAspects{Report: "transit cross"}, nil
	}
	synastryCrossAspects = func(_ context.Context, lens lookup.Lens, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo, _ store.ReportLabel) (lookup.OverlayAspects, error) {
		spy.record("synastry", lens)
		return lookup.OverlayAspects{Report: "synastry cross"}, nil
	}
	return spy
}

// chosenZ is a session record of a zodiac choice for the anchored slot at i.
func chosenZ(m Model, i int, z recipe.Zodiac) store.FrameRecord {
	s := m.slots.All()[i]
	return store.FrameRecord{File: slotFile(s), When: s.Time, Zodiac: z.RecordName(), HasZodiac: true}
}

func floatingZ(z recipe.Zodiac) store.FrameRecord {
	return store.FrameRecord{Floating: true, Zodiac: z.RecordName(), HasZodiac: true}
}

// approvedZodiacCodes reads the eleven codes from the frames spec, in table
// order (Tropical first, then C2's ten).
func approvedZodiacCodes(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile("../lookup/testdata/house-zodiac-frames.md")
	if err != nil {
		t.Fatal(err)
	}
	const head = "## The 22 house codes and 11 zodiac codes"
	text := string(b)
	i := strings.Index(text, head)
	if i < 0 {
		t.Fatal("the approved table is missing from the frames spec")
	}
	paras := strings.Split(strings.TrimSpace(text[i+len(head):]), "\n\n")
	para := strings.ReplaceAll(strings.TrimSpace(paras[1]), "\n", " ")
	var codes []string
	for _, item := range strings.Split(para, " · ") {
		codes = append(codes, strings.Fields(item)[0])
	}
	return codes
}

// C1, and the failure mode "bad default".
func TestOwnZodiacPrecedence(t *testing.T) {
	base := func() Model {
		m := listModel(t, 1, 0)
		m.frames.files = map[string]fileFrame{"synthetic-1.as": {when: m.slots.All()[0].Time}}
		return m
	}
	slotOf := func(m Model) *store.Slot { return m.slots.All()[0] }
	t.Run("nothing names a zodiac: tropical", func(t *testing.T) {
		m := base()
		if got := m.ownZodiac(slotOf(m)); got != recipe.Tropical {
			t.Errorf("own = %v", got)
		}
	})
	t.Run("the preferences default beats tropical", func(t *testing.T) {
		m := base().applyFrameConfig(store.FrameConfig{Zodiac: "sidereal Raman", ZodiacPresent: true})
		if got := m.ownZodiac(slotOf(m)); got != recipe.Raman {
			t.Errorf("own = %v, want Raman", got)
		}
	})
	t.Run("a session record beats the default", func(t *testing.T) {
		m := base().applyFrameConfig(store.FrameConfig{Zodiac: "sidereal Raman", ZodiacPresent: true})
		m.frames = m.frames.with(chosenZ(m, 0, recipe.Lahiri))
		if got := m.ownZodiac(slotOf(m)); got != recipe.Lahiri {
			t.Errorf("own = %v, want Lahiri", got)
		}
	})
	t.Run("a record that chose tropical beats a sidereal default", func(t *testing.T) {
		m := base().applyFrameConfig(store.FrameConfig{Zodiac: "sidereal Raman", ZodiacPresent: true})
		m.frames = m.frames.with(chosenZ(m, 0, recipe.Tropical))
		if got := m.ownZodiac(slotOf(m)); got != recipe.Tropical {
			t.Errorf("own = %v, want the recorded Tropical", got)
		}
	})
	t.Run("a houses-only record freezes no zodiac", func(t *testing.T) {
		m := base().applyFrameConfig(store.FrameConfig{Zodiac: "sidereal Raman", ZodiacPresent: true})
		m.frames = m.frames.with(chosen(m, 0, recipe.Koch))
		if got := m.ownZodiac(slotOf(m)); got != recipe.Raman {
			t.Errorf("own = %v, want the default Raman", got)
		}
	})
	t.Run("the chart file never names a zodiac", func(t *testing.T) {
		m := base()
		m.frames = m.frames.withFile("synthetic-1.as", fileFrame{when: slotOf(m).Time, houses: recipe.Equal, has: true})
		if got := m.ownZodiac(slotOf(m)); got != recipe.Tropical {
			t.Errorf("own = %v, want tropical whatever the file says", got)
		}
	})
	t.Run("a record made for another chart is ignored", func(t *testing.T) {
		m := base()
		rec := chosenZ(m, 0, recipe.Lahiri)
		rec.When = rec.When.Add(-time.Hour)
		m.frames = m.frames.with(rec)
		if got := m.ownZodiac(slotOf(m)); got != recipe.Tropical {
			t.Errorf("own = %v, want tropical", got)
		}
	})
	t.Run("the floating slot has its own record", func(t *testing.T) {
		m := base()
		m.frames = m.frames.with(floatingZ(recipe.Deluce))
		fl := m.slots.All()[len(m.slots.All())-1]
		if got := m.ownZodiac(fl); got != recipe.Deluce {
			t.Errorf("own = %v, want Deluce", got)
		}
	})
	t.Run("repeated keys", func(t *testing.T) {
		sessionSandbox(t)
		path := store.PreferencesPath()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		text := "frame:\n  zodiac: sidereal Krishnamurti\n  zodiac: sidereal Raman\nframe:\n  zodiac: sidereal Deluce\n"
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		m := base().applyFrameConfig(realPerimeter(t).Frame)
		if got := m.ownZodiac(slotOf(m)); got != recipe.Krishnamurti {
			t.Errorf("own = %v, want the first zodiac: key of the first frame: block (Krishnamurti)", got)
		}
	})
	t.Run("bad default", func(t *testing.T) {
		bad := store.FrameConfig{Zodiac: "sidereal Nonsense", ZodiacPresent: true}
		m := base().applyFrameConfig(bad).applyFrameConfig(bad)
		if got := m.ownZodiac(slotOf(m)); got != recipe.Tropical {
			t.Errorf("own = %v, want tropical", got)
		}
		if n := logHas(m, "sidereal Nonsense"); n != 1 {
			t.Errorf("the ignored value was named %d times, want once: %v", n, m.logLines)
		}
	})
	t.Run("a bare name is not the form", func(t *testing.T) {
		m := base().applyFrameConfig(store.FrameConfig{Zodiac: "Lahiri", ZodiacPresent: true})
		if got := m.ownZodiac(slotOf(m)); got != recipe.Tropical {
			t.Errorf("own = %v, want tropical", got)
		}
	})
}

// Each selected chart keeps its own zodiac (operator, 2026-10-24: "same for
// lahiri"): every call for a chart's own rows carries its own, whichever row
// the cursor is on, and every row is tagged with its own code.
func TestSelectedChartsKeepTheirOwnZodiac(t *testing.T) {
	spy := stubFrameLookups(t)
	build := func(cursor int) Model {
		m := selectedSet(t, 2, true, cursor)
		m.frames.records = []store.FrameRecord{chosenZ(m, 0, recipe.Lahiri), chosenZ(m, 1, recipe.Raman), floatingZ(recipe.Deluce)}
		return m
	}
	own := map[string]recipe.Zodiac{"n1": recipe.Lahiri, "n2": recipe.Raman, "now": recipe.Deluce}
	for _, c := range []struct {
		name   string
		cursor int
		view   recipe.Zodiac
	}{{"first chart", 0, recipe.Lahiri}, {"second chart", 1, recipe.Raman}, {"floating", 2, recipe.Deluce}} {
		t.Run("in the group, cursor on "+c.name, func(t *testing.T) {
			spy.reset()
			m := settleHouses(t, build(c.cursor))
			if got := m.viewLens().Zodiac; got != c.view {
				t.Errorf("viewLens = %v, want the cursor's own %v", got, c.view)
			}
			if len(spy.snapshot()) == 0 {
				t.Fatal("nothing was executed")
			}
			for _, call := range spy.snapshot() {
				if call.zodiac != own[call.label] {
					t.Errorf("executed %s call carried %v, want its own %v: %v", call.label, call.zodiac, own[call.label], spy.snapshot())
				}
			}
		})
	}
	t.Run("an unselected focused chart shows its own", func(t *testing.T) {
		spy.reset()
		m := build(1)
		m.slots.SetSlotSelected(1, false)
		m = settleHouses(t, m)
		if got := m.viewLens().Zodiac; got != recipe.Raman {
			t.Errorf("viewLens = %v, want its own Raman", got)
		}
		if got := spy.lastOf(t, "n2"); got != recipe.Raman {
			t.Errorf("the executed call carried %v, want Raman", got)
		}
	})
	t.Run("no anchored chart selected: the floating slot is its own", func(t *testing.T) {
		m := build(2)
		m.slots.SetSlotSelected(0, false)
		m.slots.SetSlotSelected(1, false)
		if got := m.viewLens().Zodiac; got != recipe.Deluce {
			t.Errorf("viewLens = %v, want the floating slot's own Deluce", got)
		}
	})
	t.Run("every row of the group is tagged with its own code", func(t *testing.T) {
		m := build(0)
		for i, want := range []recipe.Zodiac{recipe.Lahiri, recipe.Raman, recipe.Deluce} {
			if got := m.lensFor(slotAt(m, i)).Zodiac; got != want {
				t.Errorf("slot %d is shown in %v, want its own %v", i, got, want)
			}
		}
	})
}

// C5: every tag code is the approved one, the finder row names the ayanamsa and
// its offset, and the console shows the `=s <offset>` actually sent.
func TestSiderealIsAlwaysNamed(t *testing.T) {
	codes := approvedZodiacCodes(t)
	if len(codes) != recipe.ZodiacCount {
		t.Fatalf("the approved table lists %d codes, want %d", len(codes), recipe.ZodiacCount)
	}
	t.Run("all eleven tag codes", func(t *testing.T) {
		for z := recipe.Zodiac(0); int(z) < recipe.ZodiacCount; z++ {
			m := listModel(t, 1, 0)
			m.frames.records = []store.FrameRecord{chosenZ(m, 0, z)}
			tag := m.rowTag(m.slots.All()[0])
			if want := "Wh" + codes[z]; tag != want {
				t.Errorf("%s: tag %q, want %q", z.Name(), tag, want)
			}
			if strings.HasSuffix(tag, "Si") {
				t.Errorf("%s: a bare sidereal code: %q", z.Name(), tag)
			}
		}
	})
	t.Run("the finder row names the ayanamsa and its offset", func(t *testing.T) {
		st := zodiacFinderFrame("Constructed One", "lah", recipe.Lahiri, 40)
		body := ansi.Strip(renderZodiacFinder(st, Config{Accessible: true}, 132, 40))
		for _, want := range []string{"Lahiri", "Fagan-Bradley + 0.883208°", "La"} {
			if !strings.Contains(body, want) {
				t.Errorf("the finder lacks %q:\n%s", want, body)
			}
		}
		st = zodiacFinderFrame("Constructed One", "delu", recipe.Tropical, 40)
		body = ansi.Strip(renderZodiacFinder(st, Config{Accessible: true}, 132, 40))
		if !strings.Contains(body, "Fagan-Bradley − 3.075453°") {
			t.Errorf("the finder lacks Deluce's signed offset:\n%s", body)
		}
	})
	t.Run("the console shows the =s offset sent", func(t *testing.T) {
		if _, err := exec.LookPath("astrolog"); err != nil {
			t.Skip("astrolog not on PATH; skipping live-binary test")
		}
		sessionSandbox(t)
		m := listModel(t, 1, 0)
		m.frames.records = []store.FrameRecord{chosenZ(m, 0, recipe.Lahiri)}
		m = settleHouses(t, m)
		found := false
		for _, l := range argvLines(m) {
			if strings.Contains(l, " '=s' 0.883208 ") || strings.Contains(l, " =s 0.883208 ") {
				found = true
			}
			if strings.Contains(l, " -s ") || strings.Contains(l, " '-s' ") {
				t.Errorf("a bare -s was sent: %s", l)
			}
		}
		if !found {
			t.Errorf("no console line carries =s 0.883208: %v", argvLines(m))
		}
	})
}

// C7 and its failure modes.
func TestZodiacChoiceSavesTheSession(t *testing.T) {
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
		stubFrameLookups(t)
		m := listModel(t, 2, 0)
		m, cmd := m.chooseZodiac(1, recipe.Lahiri)
		m = pump(t, m, cmd)
		got := savedFrames(t)
		if len(got) != 1 || got[0].File != "synthetic-2.as" || got[0].Zodiac != "sidereal Lahiri" || !got[0].HasZodiac || got[0].HasHouses {
			t.Fatalf("session frames = %+v", got)
		}
		if m.ownZodiac(m.slots.All()[1]) != recipe.Lahiri {
			t.Error("the choice does not apply on screen")
		}
	})
	t.Run("choosing tropical is recorded too", func(t *testing.T) {
		sessionSandbox(t)
		stubFrameLookups(t)
		m := listModel(t, 1, 0).applyFrameConfig(store.FrameConfig{Zodiac: "sidereal Raman", ZodiacPresent: true})
		m, cmd := m.chooseZodiac(0, recipe.Tropical)
		m = pump(t, m, cmd)
		got := savedFrames(t)
		if len(got) != 1 || got[0].Zodiac != "tropical" {
			t.Fatalf("session frames = %+v", got)
		}
		if m.ownZodiac(m.slots.All()[0]) != recipe.Tropical {
			t.Error("the recorded tropical lost to the sidereal default")
		}
	})
	t.Run("one write per choice, merged into the chart's record", func(t *testing.T) {
		sessionSandbox(t)
		stubFrameLookups(t)
		m := listModel(t, 1, 0)
		m, cmd := m.chooseHouseSystem(0, recipe.Koch)
		m = pump(t, m, cmd)
		m, cmd = m.chooseZodiac(0, recipe.Lahiri)
		m = pump(t, m, cmd)
		got := savedFrames(t)
		if len(got) != 1 || !got[0].HasHouses || got[0].Houses != recipe.Koch || got[0].Zodiac != "sidereal Lahiri" {
			t.Fatalf("session frames = %+v, want one record holding both", got)
		}
	})
	t.Run("the floating slot's choice persists as its own record", func(t *testing.T) {
		sessionSandbox(t)
		stubFrameLookups(t)
		m := listModel(t, 1, 0)
		m, cmd := m.chooseZodiac(1, recipe.Deluce)
		_ = pump(t, m, cmd)
		got := savedFrames(t)
		if len(got) != 1 || !got[0].Floating || got[0].Zodiac != "sidereal Deluce" {
			t.Fatalf("session frames = %+v", got)
		}
	})
	t.Run("no file", func(t *testing.T) {
		sessionSandbox(t)
		stubFrameLookups(t)
		m := listModel(t, 1, 0)
		m.slots.All()[0].Chart.Source = store.ChartSource{}
		m, cmd := m.chooseZodiac(0, recipe.Lahiri)
		m = pump(t, m, cmd)
		if got := m.ownZodiac(m.slots.All()[0]); got != recipe.Lahiri {
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
		stubFrameLookups(t)
		if err := os.MkdirAll(filepath.Dir(store.SessionDir()), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(store.SessionDir(), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		m := listModel(t, 1, 0)
		m, cmd := m.chooseZodiac(0, recipe.Lahiri)
		m = pump(t, m, cmd)
		if got := m.ownZodiac(m.slots.All()[0]); got != recipe.Lahiri {
			t.Errorf("the choice must still apply on screen: %v", got)
		}
		if logHas(m, "Couldn't save the zodiac choice") != 1 {
			t.Errorf("the failure is not on the error lane: %v", m.logLines)
		}
	})
	t.Run("a launch whose refused session could not be kept writes nothing", func(t *testing.T) {
		sessionSandbox(t)
		stubFrameLookups(t)
		m := listModel(t, 1, 0)
		m.sessionKept = true
		m, cmd := m.chooseZodiac(0, recipe.Lahiri)
		m = pump(t, m, cmd)
		if _, err := os.Stat(store.SessionPath()); !os.IsNotExist(err) {
			t.Errorf("a session was written over the refused file: %v", err)
		}
		if got := m.ownZodiac(m.slots.All()[0]); got != recipe.Lahiri {
			t.Errorf("the choice must still apply: %v", got)
		}
	})
}

// Pass-3 build note: houses then zodiac, quit and reload, both survive.
func TestHouseThenZodiacBothSurviveReload(t *testing.T) {
	sessionSandbox(t)
	stubFrameLookups(t)
	m := listModel(t, 2, 0)
	m, cmd := m.chooseHouseSystem(1, recipe.Koch)
	m = pump(t, m, cmd)
	m, cmd = m.chooseZodiac(1, recipe.Lahiri)
	m = pump(t, m, cmd)
	// A relaunch: the session file is read back and its records installed.
	p := realPerimeter(t)
	if !p.HasSession || len(p.Session.Frames) != 1 {
		t.Fatalf("the saved session holds %d frames records, want 1 (%+v)", len(p.Session.Frames), p.Session.Frames)
	}
	re := listModel(t, 2, 0).loadFrames(p)
	slot := re.slots.All()[1]
	if got := re.ownHouses(slot).hs; got != recipe.Koch {
		t.Errorf("after the reload the houses are %v, want Koch", got)
	}
	if got := re.ownZodiac(slot); got != recipe.Lahiri {
		t.Errorf("after the reload the zodiac is %v, want Lahiri", got)
	}
	// And the other order.
	sessionSandbox(t)
	m = listModel(t, 2, 0)
	m, cmd = m.chooseZodiac(1, recipe.Raman)
	m = pump(t, m, cmd)
	m, cmd = m.chooseHouseSystem(1, recipe.Equal)
	m = pump(t, m, cmd)
	re = listModel(t, 2, 0).loadFrames(realPerimeter(t))
	slot = re.slots.All()[1]
	if re.ownHouses(slot).hs != recipe.Equal || re.ownZodiac(slot) != recipe.Raman {
		t.Errorf("zodiac then houses: got %v and %v, want Equal and Raman", re.ownHouses(slot).hs, re.ownZodiac(slot))
	}
}

// Pass-6 build note: an unknown zodiac drops the field, a valid house choice in
// the same entry still applies, and the file is kept aside.
func TestUnknownZodiacPreservesValidHouseChoice(t *testing.T) {
	sessionSandbox(t)
	seedSession(t, hereText("version: 3\nfloating:\n  selected: false\nframes:\n"+
		"  - file: good.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Koch\n    from: chosen\n    zodiac: Lahiri\n"))
	m := startedModel(t, withLocation(realPerimeter(t)))
	rec, ok := m.frames.find("good.as", false)
	if !ok || !rec.HasHouses || rec.Houses != recipe.Koch {
		t.Fatalf("the valid house choice was lost: %+v (found %v)", rec, ok)
	}
	if rec.HasZodiac {
		t.Errorf("the unknown zodiac was kept: %+v", rec)
	}
	if n := logHas(m, "zodiac is not one this build knows"); n != 1 {
		t.Errorf("the dropped field was named %d times, want once: %v", n, m.logLines)
	}
	if len(refusedFilesIn(t)) != 1 {
		t.Errorf("the file with a dropped field was not kept aside: %v", refusedFilesIn(t))
	}
}

// C10: the EXECUTED -t, -tr and -dm searches carry the zodiac chosen for the
// chart the cursor is on.
func TestSelectedZodiacReachesTimeline(t *testing.T) {
	sessionSandbox(t)
	stubFrameLookups(t)
	searcher := &mergeSearcher{}
	stubTimelineSearch(t, searcher)
	m := octoberSet(t, 0) // on ①: the timeline asks in ①'s lens
	m = loadTimeline(t, m)
	// The timeline follows its inputs while its tab is on screen.
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
	searcher.mu.Lock()
	before := len(searcher.cmds)
	searcher.mu.Unlock()
	if before == 0 {
		t.Fatalf("fixture: no timeline search ran (details visible: %v)", m.detailsVisible())
	}
	if len(m.timeline.rows) == 0 || !m.timeline.complete {
		t.Fatalf("fixture: the timeline is not populated and visible: complete=%v rows=%d", m.timeline.complete, len(m.timeline.rows))
	}
	m, cmd := m.chooseZodiac(0, recipe.Lahiri)
	// The visible Timeline's old rows go at once; new searches run under the new frame.
	if len(m.timeline.rows) != 0 || m.timeline.complete {
		t.Errorf("the old rows stayed after the choice: complete=%v rows=%d", m.timeline.complete, len(m.timeline.rows))
	}
	m = pump(t, m, cmd)
	searcher.mu.Lock()
	after := append([]string(nil), searcher.cmds[before:]...)
	searcher.mu.Unlock()
	modes := map[string]bool{}
	for _, argv := range after {
		fields := strings.Fields(argv)
		hasBand := strings.Contains(argv, " =s 0.883208 _Ys ")
		for _, f := range fields {
			switch f {
			case "-t", "-tr", "-dm":
				modes[f] = true
				if !hasBand {
					t.Errorf("an executed %s search lacks =s 0.883208: %s", f, argv)
				}
			}
		}
		if strings.Contains(argv, " _s ") {
			t.Errorf("a search still carries the tropical pin: %s", argv)
		}
	}
	if !modes["-t"] || !modes["-tr"] || !modes["-dm"] {
		t.Errorf("a zodiac choice re-ran %v, want -t, -tr and -dm; searches: %v", modes, after)
	}
	if !m.timeline.complete || len(m.timeline.rows) == 0 {
		t.Errorf("the Timeline did not come back: complete=%v rows=%d", m.timeline.complete, len(m.timeline.rows))
	}
}

// C10, live: a sidereal sky really gives other rows than the tropical one.
func TestSelectedZodiacTimelineLiveResultDiffers(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	sessionSandbox(t)
	old := computeTimeline
	computeTimeline = lookup.Timeline
	t.Cleanup(func() { computeTimeline = old })
	rows := func(z recipe.Zodiac) []string {
		m := timelineModel(t, Config{Accessible: true})
		// The fixture's cursor is on the floating slot, and its natal may be the
		// group's first chart: both carry the choice, so either way decides.
		m, _ = m.chooseZodiac(0, z)
		m, _ = m.chooseZodiac(len(m.slots.All())-1, z)
		m, _ = syncTimelineVisited(m)
		cmd := buildTimelineCmd(m, context.Background(), 1, 1)
		if cmd == nil {
			t.Fatal("the model started no timeline search")
		}
		msg, ok := cmd().(refreshResultMsg)
		if !ok || msg.err != nil {
			t.Fatalf("timeline search: %#v", msg.err)
		}
		m, _ = m.commitTimeline(msg)
		var out []string
		for _, r := range m.timeline.rows {
			out = append(out, r.Line)
		}
		if p, ok := msg.payload.(timelinePayload); ok && len(p.commands) > 0 {
			t.Logf("%s: first search %s", z.Name(), p.commands[0])
		}
		return out
	}
	trop, lahiri := rows(recipe.Tropical), rows(recipe.Lahiri)
	if len(trop) == 0 || len(lahiri) == 0 {
		t.Fatalf("no rows: tropical %d, Lahiri %d", len(trop), len(lahiri))
	}
	if strings.Join(trop, "\n") == strings.Join(lahiri, "\n") {
		t.Error("the Lahiri timeline equals the tropical one: the chosen zodiac never reached the searches")
	}
}

// Pass-7 build note, turned by the operator on 2026-10-24: moving between
// differently chosen selected charts, each search uses the zodiac of the chart
// the cursor is on, and is asked again whenever that zodiac changes.
func TestSelectedTimelineUsesTheCursorsZodiac(t *testing.T) {
	sessionSandbox(t)
	stubFrameLookups(t)
	searcher := &mergeSearcher{}
	stubTimelineSearch(t, searcher)
	m := octoberSet(t, 0)
	m.frames.records = []store.FrameRecord{chosenZ(m, 0, recipe.Lahiri), chosenZ(m, 1, recipe.Raman)}
	searches := func() []string {
		searcher.mu.Lock()
		defer searcher.mu.Unlock()
		return append([]string(nil), searcher.cmds...)
	}
	m = loadTimeline(t, m)
	from := 0
	for _, c := range []struct {
		cursor     int
		band, name string
	}{
		{0, " =s 0.883208 _Ys ", "①'s Lahiri"},
		{2, " _s ", "now's tropical"},
		{1, " =s 2.329509 ", "②'s Raman"},
	} {
		if c.cursor != 0 {
			m = m.focusSlot(c.cursor)
			var cmd tea.Cmd
			m, cmd = m.afterFocusChange()
			m = pump(t, m, cmd)
			m = loadTimeline(t, m)
		}
		after := searches()[from:]
		if len(after) == 0 {
			t.Errorf("cursor %d: nothing was asked in %s", c.cursor, c.name)
		}
		for _, argv := range after {
			if !strings.Contains(argv, c.band) {
				t.Errorf("cursor %d: a search was not cast in %s: %s", c.cursor, c.name, argv)
			}
		}
		from += len(after)
	}
}

// Pass-3 build note: the focused unselected chart's call carries its zodiac,
// the comparison's calls carry the group's.
func TestUnselectedZodiacDoesNotLeakIntoTheComparison(t *testing.T) {
	spy := stubFrameLookups(t)
	var raw []recipe.Zodiac
	var cross []recipe.Zodiac
	var mu sync.Mutex
	oldRaw, oldCross := computeRawReport, computeCrossBodyReport
	computeRawReport = func(_ context.Context, _ time.Time, _ location.PlaceInfo, lens lookup.Lens, _ lookup.RawReportRequest) (string, string, error) {
		mu.Lock()
		raw = append(raw, lens.Zodiac)
		mu.Unlock()
		return "report", "astrolog report", nil
	}
	computeCrossBodyReport = func(_ context.Context, lens lookup.Lens, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo, _ aspect.Body) (string, string, error) {
		mu.Lock()
		cross = append(cross, lens.Zodiac)
		mu.Unlock()
		return "report", "astrolog cross report", nil
	}
	t.Cleanup(func() { computeRawReport, computeCrossBodyReport = oldRaw, oldCross })

	m := selectedSet(t, 3, true, 2)
	m.slots.SetSlotSelected(2, false)
	m.frames.records = []store.FrameRecord{chosenZ(m, 0, recipe.Raman), chosenZ(m, 2, recipe.Lahiri)}
	spy.reset()
	m = settleHouses(t, m)
	if len(spy.snapshot()) == 0 {
		t.Fatal("the single frame executed nothing")
	}
	for _, c := range spy.snapshot() {
		if c.label != "n3" || c.zodiac != recipe.Lahiri {
			t.Errorf("single frame call %v, want only ③ in Lahiri", c)
		}
	}
	// The cursor moves into the group: ③'s Lahiri reaches no call; ① frames,
	// and the others keep their own (no record: tropical).
	spy.reset()
	mu.Lock()
	raw, cross = nil, nil
	mu.Unlock()
	m = m.focusSlot(0)
	m, cmd := m.afterFocusChange()
	m = pump(t, m, cmd)
	if len(spy.snapshot()) == 0 {
		t.Fatal("the comparison frame executed nothing")
	}
	for _, c := range spy.snapshot() {
		want := recipe.Tropical
		if c.label == "n1" {
			want = recipe.Raman
		}
		if c.zodiac != want {
			t.Errorf("comparison call %v carried the wrong zodiac, want %v", c, want)
		}
	}
	m.width, m.height = 132, 40
	for _, panel := range []PanelID{PanelChart, PanelBodies} {
		m.focused = panel
		if cmd := buildDetailsCmd(m, context.Background(), 1, 1); cmd != nil {
			cmd()
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(raw)+len(cross) == 0 {
		t.Fatal("no Details report was requested")
	}
	for _, z := range raw {
		if z != recipe.Raman {
			t.Errorf("a raw Details report carried %v, want the group's Raman (raw %v, cross %v)", z, raw, cross)
		}
	}
	for _, z := range cross {
		if z != recipe.Raman {
			t.Errorf("a cross Details report carried %v, want the group's Raman (raw %v, cross %v)", z, raw, cross)
		}
	}
}

// Pass-5 build note: the focused chart's raw Details report carries its own
// zodiac, selected or not.
func TestFocusedRawDetailsUsesOwnZodiac(t *testing.T) {
	stubFrameLookups(t)
	var got []recipe.Zodiac
	var mu sync.Mutex
	old := computeRawReport
	computeRawReport = func(_ context.Context, _ time.Time, _ location.PlaceInfo, lens lookup.Lens, _ lookup.RawReportRequest) (string, string, error) {
		mu.Lock()
		got = append(got, lens.Zodiac)
		mu.Unlock()
		return "report", "astrolog report", nil
	}
	t.Cleanup(func() { computeRawReport = old })
	m := selectedSet(t, 2, true, 1)
	m.slots.SetSlotSelected(1, false)
	m.frames.records = []store.FrameRecord{chosenZ(m, 0, recipe.Raman), chosenZ(m, 1, recipe.Lahiri)}
	m = settleHouses(t, m)
	m.width, m.height = 132, 40
	m.focused = PanelChart
	mu.Lock()
	got = nil
	mu.Unlock()
	cmd := buildDetailsCmd(m, context.Background(), 1, 1)
	if cmd == nil {
		t.Fatal("no Details report was requested")
	}
	cmd()
	mu.Lock()
	if len(got) != 1 || got[0] != recipe.Lahiri {
		t.Errorf("the raw Details report carried %v, want [Lahiri]", got)
	}
	got = nil
	mu.Unlock()
	// In the group, ② on its own tab: still its own Lahiri, not ①'s Raman.
	m.slots.SetSlotSelected(1, true)
	m = settleHouses(t, m)
	m.width = max(m.width, dualColumnMinWidth(len(m.slots.SelectedInOrder())))
	m.focused = PanelChart
	if cmd := buildDetailsCmd(m, context.Background(), 1, 1); cmd != nil {
		cmd()
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != recipe.Lahiri {
		t.Errorf("in the group, the raw Details report carried %v, want [Lahiri]", got)
	}
}

// Pass-4 build note: Lahiri to Raman on the same chart re-casts the positions
// and the wheel, and no column cast in the old zodiac is presented as current.
func TestChangingAyanamsaInvalidatesPositionsAndWheel(t *testing.T) {
	sessionSandbox(t)
	spy := stubFrameLookups(t)
	var mu sync.Mutex
	var drawn []recipe.Zodiac
	oldS, oldShow := renderWheelImage, showWheel
	t.Cleanup(func() { renderWheelImage, showWheel = oldS, oldShow })
	renderWheelImage = func(_ context.Context, _ time.Time, _ location.PlaceInfo, lens lookup.Lens, _ lookup.WheelHighlight) ([]byte, error) {
		mu.Lock()
		drawn = append(drawn, lens.Zodiac)
		mu.Unlock()
		return []byte("wheel"), nil
	}
	showWheel = func([]byte) (string, error) { return "/tmp/stub.png", nil }

	m := listModel(t, 1, 0)
	m.slots.SetSlotSelected(0, false)
	m.slots.SetSlotSelected(1, false)
	m.frames.records = []store.FrameRecord{chosenZ(m, 0, recipe.Lahiri)}
	m.cfg = Config{Color: ColorNever}
	m = settleHouses(t, m)
	m = openWheel(t, m)
	if got := m.chart.positions[0].Longitude; got != zodiacLongitude(m.slots.All()[0].Time.Year(), recipe.Lahiri) {
		t.Fatalf("fixture: the first frame is %v, not the Lahiri cast", got)
	}
	spy.reset()
	m, cmd := m.chooseZodiac(0, recipe.Raman)
	m = pump(t, m, cmd)
	if got := spy.lastOf(t, "n1"); got != recipe.Raman {
		t.Errorf("the chart was recast in %v, want Raman", got)
	}
	if got, want := m.chart.positions[0].Longitude, zodiacLongitude(m.slots.All()[0].Time.Year(), recipe.Raman); got != want {
		t.Errorf("the committed positions are %v, want the Raman cast's %v", got, want)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(drawn) < 2 || drawn[0] != recipe.Lahiri || drawn[len(drawn)-1] != recipe.Raman {
		t.Errorf("wheel renders carried %v, want Lahiri then Raman", drawn)
	}
}

// The columns of a comparing scene are the scene's only when each was cast in
// the zodiac it is shown in now.
func TestZodiacChoiceInvalidatesShownColumns(t *testing.T) {
	sessionSandbox(t)
	spy := stubFrameLookups(t)
	m := settleComparingWith(t, 2, 0)
	if _, ok := m.bodyColumnsForScene(); !ok {
		t.Fatal("fixture: no comparing columns")
	}
	stale := m
	stale.frames.records = []store.FrameRecord{chosenZ(m, 0, recipe.Lahiri)}
	if _, ok := stale.bodyColumnsForScene(); ok {
		t.Error("columns cast in the old zodiac are presented as the scene's after the zodiac changed under them")
	}
	spy.reset()
	m, cmd := m.chooseZodiac(0, recipe.Lahiri)
	if _, ok := m.bodyColumnsForScene(); ok {
		t.Error("columns cast in the old zodiac are still presented as the scene's columns")
	}
	m = pump(t, m, cmd)
	if cols, ok := m.bodyColumnsForScene(); !ok || len(cols) != 3 {
		t.Fatalf("no new columns after the recast: ok=%v n=%d", ok, len(cols))
	}
	// ① alone is recast in Lahiri; the others keep their own tropical columns.
	if got := spy.lastOf(t, "n1"); got != recipe.Lahiri {
		t.Errorf("n1 was recast in %v, want Lahiri", got)
	}
	for _, c := range spy.snapshot() {
		if c.label != "n1" && c.zodiac == recipe.Lahiri {
			t.Errorf("%s was cast in ①'s Lahiri: %v", c.label, spy.snapshot())
		}
	}
}

// Pass-5 build note: two quick choices; only the latest frame's cast commits.
func TestRapidZodiacChoicesKeepOnlyTheLatestFrame(t *testing.T) {
	sessionSandbox(t)
	stubFrameLookups(t)
	m := listModel(t, 1, 0)
	m.slots.SetSlotSelected(1, false)
	m = settleHouses(t, m)
	year := m.slots.All()[0].Time.Year()
	m, first := m.chooseZodiac(0, recipe.Lahiri)
	m, second := m.chooseZodiac(0, recipe.Raman)
	m = pump(t, m, first, second)
	if got, want := m.chart.positions[0].Longitude, zodiacLongitude(year, recipe.Raman); got != want {
		t.Errorf("the committed frame is %v, want the latest choice's Raman cast %v", got, want)
	}
	if got := m.ownZodiac(m.slots.All()[0]); got != recipe.Raman {
		t.Errorf("own zodiac = %v, want Raman", got)
	}
	s, ok, err := store.LoadSession()
	if err != nil || !ok || len(s.Frames) != 1 || s.Frames[0].Zodiac != "sidereal Raman" {
		t.Errorf("the saved record is %+v (ok %v, err %v), want Raman", s.Frames, ok, err)
	}
}

// Pass-6 build note: one row with a polar substitution AND an ayanamsa shows both.
func TestPolarSiderealTagMatchesExecutedCast(t *testing.T) {
	spy := stubFrameLookups(t)
	spy.used = func(label string, asked recipe.HouseSystem) string {
		if label == "n1" && asked == recipe.Placidus {
			return "Porphyry"
		}
		return asked.StoredName()
	}
	m := selectedSet(t, 1, true, 0)
	m.frames.records = []store.FrameRecord{{File: "synthetic-1.as", When: m.slots.All()[0].Time, Houses: recipe.Placidus, HasHouses: true, From: store.FrameFromChosen, Zodiac: "sidereal Lahiri", HasZodiac: true}}
	m = settleHouses(t, m)
	if tag := m.rowTag(m.slots.All()[0]); tag != "Po*La" {
		t.Errorf("tag = %q, want Po*La: the house used and the ayanamsa", tag)
	}
	// The substitution belongs to the chart Astrolog cast at that latitude; the
	// group's other row keeps its own frame, ayanamsa included.
	if tag := m.rowTag(m.slots.All()[1]); tag != "WhTr" {
		t.Errorf("the group's other row = %q, want its own WhTr", tag)
	}
}

// Pass-3 build note, end to end: choose Lahiri through the zodiac finder (⌃z, `lah`, Enter) and compare
// the live position with a direct `=s Lahiri` Astrolog call.
// Positive control: removing the chosen offset (the table's Lahiri entry)
// makes this test fail by name.
func TestFinderLahiriMatchesAstrologsLahiri(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	sessionSandbox(t)
	m := listModel(t, 1, 0)
	m.slots.SetSlotSelected(1, false)
	m.cfg = Config{Accessible: true, Color: ColorNever}
	m = settleHouses(t, m)
	tropical := sunLongitude(t, m)
	m = openZodiacFinderOn(t, m)
	m = typeInFinder(t, m, "lah")
	m, cmd := tpress(t, m, keyEnter)
	m = pump(t, m, cmd)
	if got := m.ownZodiac(m.slots.All()[0]); got != recipe.Lahiri {
		t.Fatalf("the finder chose %v, not Lahiri", got)
	}
	got := sunLongitude(t, m)
	if got == tropical {
		t.Fatal("the Lahiri frame carries the tropical Sun")
	}
	slot := m.slots.All()[0]
	direct := directSunLongitude(t, slot, "Lahiri", m.viewLens().Houses)
	if diff := got - direct; diff > 0.01 || diff < -0.01 {
		t.Errorf("Sextile's Lahiri Sun is %.4f, a direct =s Lahiri call gives %.4f", got, direct)
	}
}

func sunLongitude(t *testing.T, m Model) float64 {
	t.Helper()
	for _, p := range m.chart.positions {
		if p.Body == aspect.Sun {
			return p.Longitude
		}
	}
	t.Fatal("the frame has no Sun")
	return 0
}

var directSunPattern = regexp.MustCompile(`Sun : *(\d+)([A-Z][a-z]{2})(\d+)`)

// directSunLongitude asks Astrolog itself, with a hand-written argv and the
// ayanamsa's NAME, for the Sun of the slot's chart: the oracle the offset table
// is held to. It shares no code with Sextile's compile path.
func directSunLongitude(t *testing.T, slot *store.Slot, ayanamsa string, houses recipe.HouseSystem) float64 {
	t.Helper()
	signs := []string{"Ari", "Tau", "Gem", "Can", "Leo", "Vir", "Lib", "Sco", "Sag", "Cap", "Aqu", "Pis"}
	u := slot.Time.UTC()
	dm := func(v float64, pos, neg string) string {
		hemi := pos
		if v < 0 {
			v, hemi = -v, neg
		}
		deg := int(v)
		return fmt.Sprintf("%d:%02d%s", deg, int((v-float64(deg))*60+0.5), hemi)
	}
	file := filepath.Join(t.TempDir(), "direct.as")
	content := fmt.Sprintf("@AI780  ; x\n-qb %d %d %d %d:%02d:%02d ST 0W %s %s\n-zi \"\" \"\"\n",
		int(u.Month()), u.Day(), u.Year(), u.Hour(), u.Minute(), u.Second(),
		dm(slot.Location.Lon, "E", "W"), dm(slot.Location.Lat, "N", "S"))
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("astrolog", "-i", file, "-c", strconv.Itoa(int(houses)), "=s", ayanamsa, "_Ys", "_sr0", ":sz", "-v0", "-k")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("direct astrolog: %v", err)
	}
	text := ansi.Strip(string(out))
	g := directSunPattern.FindStringSubmatch(text)
	if g == nil {
		t.Fatalf("no Sun row in the direct run:\n%s", text)
	}
	deg, _ := strconv.Atoi(g[1])
	min, _ := strconv.Atoi(g[3])
	idx := -1
	for i, s := range signs {
		if s == g[2] {
			idx = i
		}
	}
	return float64(idx*30+deg) + float64(min)/60
}

// C9: the rendered Help frame at 80x24 says it, whole, once the list is
// walked to its end (the note closes the list).
func TestHelpExplainsAyanamsas(t *testing.T) {
	m := finderModel(t, 1, 0)
	m.width, m.height = 80, 24
	m.cfg = Config{Accessible: true, Color: ColorNever}
	tm, _ := m.Update(keyMsg("?"))
	m = tm.(Model)
	if !m.helpOpen {
		t.Fatal("? did not open Help")
	}
	for i := 0; i < len(m.helpLines()); i++ {
		tm, _ = m.Update(keyMsg("down"))
		m = tm.(Model)
	}
	view := ansi.Strip(m.View())
	// Every line of the note is on the last page, whole: none is cut.
	for _, line := range helpZodiacNote {
		if !strings.Contains(view, line) {
			t.Errorf("the rendered Help frame lacks the whole line %q:\n%s", line, view)
		}
	}
	// And what the lines say, read from the frame itself.
	popup := ansi.Strip(renderHelpModal(m.helpLines(), m.cfg, m.help, 80, 24))
	var words []string
	for _, l := range strings.Split(popup, "\n") {
		l = strings.Trim(strings.TrimSpace(l), "|+-#")
		words = append(words, strings.TrimSpace(l))
	}
	flat := strings.Join(strings.Fields(strings.Join(words, " ")), " ")
	for _, want := range []string{
		"ayanamsas are fixed offsets from Fagan-Bradley",
		"Each chart is precessed to its own date",
		"a sidereal comparison's pair differs from the tropical pair",
		"by the precession between the two dates",
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("the rendered Help frame does not say %q:\n%s", want, popup)
		}
	}
	if strings.Contains(view, "…") {
		t.Errorf("the Help frame clips a line:\n%s", view)
	}
}

// A record made for another chart (the same file name, a different birth
// moment) is no record: a zodiac choice on the replaced chart starts clean and
// carries none of the old chart's houses or zodiac.
func TestZodiacChoiceOnReplacedChartInheritsNothing(t *testing.T) {
	sessionSandbox(t)
	stubFrameLookups(t)
	m := listModel(t, 1, 0)
	stale := chosen(m, 0, recipe.Koch)
	stale.When = stale.When.Add(-time.Hour) // the file now holds a different chart
	stale.Zodiac, stale.HasZodiac = "sidereal Raman", true
	m.frames = m.frames.with(stale)
	m, cmd := m.chooseZodiac(0, recipe.Lahiri)
	m = pump(t, m, cmd)
	rec, ok := m.frames.find("synthetic-1.as", false)
	if !ok || rec.HasHouses || !rec.When.Equal(m.slots.All()[0].Time) || rec.Zodiac != "sidereal Lahiri" {
		t.Errorf("record = %+v (found %v), want a clean one for this chart holding only Lahiri", rec, ok)
	}
	if got := m.ownHouses(m.slots.All()[0]); got.hs == recipe.Koch {
		t.Errorf("the old chart's Koch came across: %+v", got)
	}
}
