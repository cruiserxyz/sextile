package ui

import (
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// Save defaults (`, c d`) keeps the frame of every chart that is open: the
// saved default is for the floating chart on a fresh start and for charts
// opened afterwards (operator, 2026-10-25). The restart in these tests is a
// real one: the session file the save wrote is read back by the cold-start
// read and its records loaded into a model that starts with none.

// savedFrameModel is a workspace of n anchored charts with the floating chart
// in Placidus and sidereal Lahiri, `, c d` pressed and delivered.
func savedFrameModel(t *testing.T, n int, prepare func(Model) Model) Model {
	t.Helper()
	sessionSandbox(t)
	m := framesFixture(t, n, 0)
	m.place = amsterdamNL
	m.frames = m.frames.with(store.FrameRecord{
		Floating: true, Houses: recipe.Placidus, HasHouses: true, From: store.FrameFromChosen,
		Zodiac: recipe.Lahiri.RecordName(), HasZodiac: true,
	})
	if prepare != nil {
		m = prepare(m)
	}
	m, cmd := press(t, m, ",", "c", "d")
	if cmd == nil {
		t.Fatal("`,c d` scheduled no save")
	}
	return deliver(m, cmd)
}

// restarted is the model a start from the profile and the session on disk
// builds: the cold-start read, then the startup path that rebuilds the slots
// from session.yaml and loads its frame records and the preferences default.
func restarted(t *testing.T) Model {
	t.Helper()
	m := startedModel(t, withLocation(realPerimeter(t)))
	m.cfg.Color = ColorAlways
	return m
}

// openChart adds a chart the way an opened one is: anchored, with a file.
func openChart(t *testing.T, m Model, n int) {
	t.Helper()
	chart := store.SessionChart{
		ID: fmt.Sprintf("synthetic-%d", n), Name: fmt.Sprintf("Synthetic %d", n),
		Source: store.ChartSource{Kind: store.ChartKindFile, File: fmt.Sprintf("synthetic-%d.as", n)},
	}
	when := time.Date(1960+n, time.Month(n), 10+n, 6, n, 0, 0, time.UTC)
	place := location.PlaceInfo{Name: fmt.Sprintf("Testford %d", n), Country: "TS", Lat: 40 + float64(n), Lon: -70 - float64(n), TZ: "America/New_York"}
	if err := m.slots.AddAnchored(chart, when, place); err != nil {
		t.Fatal(err)
	}
}

// tagOf is the row tag of the anchored chart named name.
func tagOf(t *testing.T, m Model, name string) string {
	t.Helper()
	for _, s := range m.slots.All() {
		if !s.Floating && s.Chart.Name == name {
			return m.rowTag(s)
		}
	}
	t.Fatalf("no chart named %q is open: %v", name, order(m))
	return ""
}

func wantTagOf(t *testing.T, m Model, name, want, where string) {
	t.Helper()
	if got := tagOf(t, m, name); got != want {
		t.Errorf("%s: %s is tagged %q, want %q", where, name, got, want)
	}
}

func wantTag(t *testing.T, m Model, i int, want, where string) {
	t.Helper()
	if got := m.rowTag(slotAt(m, i)); got != want {
		t.Errorf("%s: row %d is tagged %q, want %q", where, i, got, want)
	}
}

// Two open charts that name no frame keep Whole Sign and tropical over a
// restart; a chart opened after the save, and the floating chart of a fresh
// start, take the saved Placidus and Lahiri.
func TestSaveDefaultsKeepsTheOpenChartsFrames(t *testing.T) {
	m := savedFrameModel(t, 2, nil)
	wantTag(t, m, 0, "WhTr", "after the save, before the restart")
	wantTag(t, m, 1, "WhTr", "after the save, before the restart")

	r := restarted(t)
	if got := order(r); !reflect.DeepEqual(got, []string{"Synthetic 1", "Synthetic 2", "now"}) {
		t.Fatalf("the session restored %v, want both charts that were open", got)
	}
	wantTagOf(t, r, "Synthetic 1", "WhTr", "restart")
	wantTagOf(t, r, "Synthetic 2", "WhTr", "restart")
	openChart(t, r, 3)
	wantTagOf(t, r, "Synthetic 3", "PlLa", "a chart opened after the save")

	// A fresh start has no session: its floating chart takes the default.
	fresh := framesFixture(t, 1, 0).applyFrameConfig(realPerimeter(t).Frame)
	floating := fresh.slots.Floating()
	if got := fresh.rowTag(&floating); got != "PlLa" {
		t.Errorf("the floating chart of a fresh start is tagged %q, want PlLa", got)
	}
}

// A chart opened between the keypress and the save landing is not one of the
// charts that were open: it takes the saved default after a restart, with no
// record of the frame it showed in between.
func TestSaveDefaultsFreezesTheChartsOpenAtTheKeypress(t *testing.T) {
	sessionSandbox(t)
	m := framesFixture(t, 2, 0)
	m.place = amsterdamNL
	m.frames = m.frames.with(store.FrameRecord{
		Floating: true, Houses: recipe.Placidus, HasHouses: true, From: store.FrameFromChosen,
		Zodiac: recipe.Lahiri.RecordName(), HasZodiac: true,
	})
	m, cmd := press(t, m, ",", "c", "d")
	if cmd == nil {
		t.Fatal("`,c d` scheduled no save")
	}
	openChart(t, m, 3) // opened while the save is in flight
	wantTag(t, m, 2, "WhTr", "while the save is in flight")
	m = deliver(m, cmd)

	if rec, ok := m.frames.find("synthetic-3.as", false); ok {
		t.Errorf("the chart opened after the keypress was given a record: %+v", rec)
	}
	r := restarted(t)
	wantTagOf(t, r, "Synthetic 1", "WhTr", "restart")
	wantTagOf(t, r, "Synthetic 2", "WhTr", "restart")
	wantTagOf(t, r, "Synthetic 3", "PlLa", "the chart opened after the keypress")
}

// A chart whose file names a system keeps it over the restart, in the
// zodiac it was shown in.
func TestSaveDefaultsKeepsAFileChartsSystem(t *testing.T) {
	file := func(m Model) Model {
		m.frames = m.frames.withFile("synthetic-1.as", fileFrame{when: slotAt(m, 0).Time, houses: recipe.Koch, has: true})
		return m
	}
	m := savedFrameModel(t, 2, file)
	wantTag(t, m, 0, "KoTr", "after the save")

	r := file(restarted(t))
	wantTagOf(t, r, "Synthetic 1", "KoTr", "restart")
	wantTagOf(t, r, "Synthetic 2", "WhTr", "restart")
	// The record freezes what the chart showed; it did not come from a rename.
	if rec, _ := m.frames.find("synthetic-1.as", false); rec.From != store.FrameFromChosen {
		t.Errorf("the frozen record says from: %q, want %q", rec.From, store.FrameFromChosen)
	}
}

// A chart that already had a record of its own keeps that record as it was.
func TestSaveDefaultsLeavesAChartsOwnRecord(t *testing.T) {
	var before store.FrameRecord
	own := func(m Model) Model {
		m, _ = m.chooseHouseSystem(0, recipe.Koch)
		m, _ = m.chooseZodiac(0, recipe.Lahiri)
		before, _ = m.frames.find("synthetic-1.as", false)
		return m
	}
	m := savedFrameModel(t, 2, own)
	if after, _ := m.frames.find("synthetic-1.as", false); !reflect.DeepEqual(after, before) {
		t.Errorf("the chart's record changed:\n--- before ---\n%+v\n--- after ---\n%+v", before, after)
	}
	r := restarted(t)
	if onDisk, _ := r.frames.find("synthetic-1.as", false); !reflect.DeepEqual(onDisk, before) {
		t.Errorf("the record on disk differs:\n--- chosen ---\n%+v\n--- on disk ---\n%+v", before, onDisk)
	}
	wantTagOf(t, r, "Synthetic 1", "KoLa", "restart")
	wantTagOf(t, r, "Synthetic 2", "WhTr", "restart")
}

// A record that names only houses gets the zodiac the chart shows, and its
// houses are left as they were.
func TestSaveDefaultsFillsTheZodiacOfAHousesOnlyRecord(t *testing.T) {
	own := func(m Model) Model {
		m, _ = m.chooseHouseSystem(0, recipe.Koch)
		return m
	}
	savedFrameModel(t, 1, own)
	r := restarted(t)
	wantTagOf(t, r, "Synthetic 1", "KoTr", "restart")
}

// A second window keeps no session, so it keeps no default: `, c d` says so
// and writes neither the preferences nor the session.
func TestSaveDefaultsRefusesInASecondWindow(t *testing.T) {
	sessionSandbox(t)
	holder := framesFixture(t, 2, 0)
	if err := store.SaveSession(holder.sessionSnapshot()); err != nil {
		t.Fatal(err)
	}
	if err := store.SetFrame(recipe.Koch.StoredName(), recipe.Tropical.RecordName()); err != nil {
		t.Fatal(err)
	}
	prefsBefore := fileText(t, store.PreferencesPath())
	sessionBefore := fileText(t, store.SessionPath())

	m := framesFixture(t, 2, 0)
	m.place = amsterdamNL
	m.cfg.SecondWindow = true
	m, cmd := press(t, m, ",", "c", "d")
	if cmd != nil {
		t.Fatal("`,c d` scheduled a save in a second window")
	}
	want := "Save defaults is for the first window, which keeps the session. This window keeps nothing"
	if lastLine(m).text != want || lastLine(m).lane != laneError {
		t.Errorf("last Log line = %q (lane %d), want the error line %q", lastLine(m).text, lastLine(m).lane, want)
	}
	if got := fileText(t, store.PreferencesPath()); got != prefsBefore {
		t.Errorf("preferences.yaml changed:\n--- before ---\n%s--- after ---\n%s", prefsBefore, got)
	}
	if got := fileText(t, store.SessionPath()); got != sessionBefore {
		t.Errorf("the session changed:\n--- before ---\n%s--- after ---\n%s", sessionBefore, got)
	}
}

// The records that keep the open charts' frames could not be written: one
// error line says so, and the default, which was written first, stays saved.
func TestSaveDefaultsOpenChartsFailureIsOneErrorLine(t *testing.T) {
	kept := func(m Model) Model {
		m.sessionKept = true // the launch left a refused session file in place
		return m
	}
	m := savedFrameModel(t, 2, kept)
	want := "open charts' frames not kept: the refused session file was left in place"
	if lastLine(m).text != want || lastLine(m).lane != laneError {
		t.Errorf("last Log line = %q (lane %d), want the error line %q", lastLine(m).text, lastLine(m).lane, want)
	}
	if noteCount(m, "default frame saved: Placidus, Lahiri") != 1 {
		t.Errorf("the saved default is not in the Log: %v", m.logLines)
	}
	if got := store.Frame(); !got.HousesPresent || got.Houses != "Placidus" {
		t.Errorf("the default was not kept: %+v", got)
	}
	if _, err := os.Stat(store.SessionPath()); !os.IsNotExist(err) {
		t.Errorf("a session was written (%v)", err)
	}
}
