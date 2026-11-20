package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The refresh rows hold the calls a build makes (row_spawns.go), so a test
// stands in for them on its own Model. A stub helper does not touch a package
// variable any more: stubSpawns files the edit under the test's name, and
// newTestModel (the test constructor, the only place a test builds a Model)
// applies every edit filed under the test and its parents to the Model it
// returns. Nothing is shared between tests: the map is keyed by test name.
//
// A stub filed after the test already built a Model is missed by that Model,
// which with a package variable it never was. stubSpawns fails the test for it
// at the filing site: file the stub first, apply it with withSpawns(m, edit),
// or, in a fixture that builds a fresh Model after filing its own stubs, begin
// with freshModels(t) to say that the Models built so far are not meant to
// see them.

// seamT is the part of *testing.T the stub registry uses, so its own test can
// hand it a recorder and read what it would have reported.
type seamT interface {
	Helper()
	Name() string
	Errorf(format string, args ...any)
	Cleanup(func())
}

// spawnEdit changes the spawns of one Model's rows.
type spawnEdit func(*rowSpawns)

var seamBook = struct {
	sync.Mutex
	edits map[string][]spawnEdit // test name -> edits, in filing order
	built map[string]int         // test name -> Models built under it
	flags map[string]bool        // test name + "|" + flag -> set
	grab  map[string]bool        // test name -> withNewStubs is running
}{edits: map[string][]spawnEdit{}, built: map[string]int{}, flags: map[string]bool{}, grab: map[string]bool{}}

// testNames is the test's own name and each parent's, outermost first.
func testNames(t seamT) []string {
	parts := strings.Split(t.Name(), "/")
	names := make([]string, 0, len(parts))
	for i := range parts {
		names = append(names, strings.Join(parts[:i+1], "/"))
	}
	return names
}

// filedAt names the callers of stubSpawns, outermost first, for the message.
func filedAt() string {
	var out []string
	for skip := 3; skip < 7; skip++ {
		_, file, line, ok := runtime.Caller(skip)
		if !ok {
			break
		}
		out = append(out, fmt.Sprintf("%s:%d", filepath.Base(file), line))
	}
	return strings.Join(out, " < ")
}

// stubSpawns files an edit for this test (and its subtests), dropped when the
// test ends.
func stubSpawns(t seamT, edit spawnEdit) {
	t.Helper()
	name := t.Name()
	seamBook.Lock()
	defer seamBook.Unlock()
	for _, n := range testNames(t) {
		if seamBook.built[n] > 0 && !seamBook.grab[name] {
			t.Errorf("%s: a stub was filed (%s) after the test built a Model, which would not see it; file it first, apply it with withSpawns(m, edit), or begin the fixture with freshModels(t)", name, filedAt())
		}
	}
	seamBook.edits[name] = append(seamBook.edits[name], edit)
	t.Cleanup(func() {
		seamBook.Lock()
		defer seamBook.Unlock()
		delete(seamBook.edits, name)
	})
}

// stubsFor is every edit filed under the test and its parents, outermost first.
func stubsFor(t seamT) []spawnEdit {
	seamBook.Lock()
	defer seamBook.Unlock()
	var out []spawnEdit
	for _, n := range testNames(t) {
		out = append(out, seamBook.edits[n]...)
	}
	return out
}

// applySpawns changes the spawns of every row of m.
func applySpawns(m Model, edits ...spawnEdit) Model {
	for _, e := range edits {
		for s := range m.rows {
			e(&m.rows[s].spawn)
		}
	}
	return m
}

// withSpawns is applySpawns for a test that holds the Model and the edit.
func withSpawns(m Model, edit spawnEdit) Model { return applySpawns(m, edit) }

// withStubs applies the test's filed stubs to a Model built before they were.
func withStubs(t seamT, m Model) Model { return applySpawns(m, stubsFor(t)...) }

// noStations is the test default for the station probe: no test spawns it
// unless it says so (station_mark_test.go installs its own), because every
// compute path reaches Astrolog through a spawn and the probe is one more
// that a fixture with a slow, direct body (a zero speed is one) would run.
func noStations(context.Context, *lookup.Probers, []aspect.Position, time.Time, location.PlaceInfo, lookup.Lens) (map[int]lookup.Station, []string, error) {
	return nil, nil, nil
}

// newTestModel is NewModelWithClock for a test: the station probe off, then
// the test's stubs.
func newTestModel(t seamT, cfg Config, clock Clock) Model {
	t.Helper()
	m := plainTestModel(t, cfg, clock)
	seamBook.Lock()
	for name := range seamBook.edits {
		if strings.HasPrefix(name, t.Name()+"/") {
			t.Errorf("%s: a Model was built with the outer test's t while the subtest %s holds stubs, which the Model would not see; build it with the subtest's t", t.Name(), name)
			break
		}
	}
	seamBook.built[t.Name()]++
	seamBook.Unlock()
	t.Cleanup(func() {
		seamBook.Lock()
		defer seamBook.Unlock()
		delete(seamBook.built, t.Name())
	})
	return m
}

// plainTestModel is newTestModel without the note that the test built a Model.
func plainTestModel(t seamT, cfg Config, clock Clock) Model {
	t.Helper()
	m := withObjectList(NewModelWithClock(cfg, clock))
	m = applySpawns(m, func(s *rowSpawns) {
		if s.cast.stations != nil {
			s.cast.stations = noStations
		}
		if s.voidsAhead != nil {
			s.voidsAhead = noVoids
		}
	})
	return applySpawns(m, stubsFor(t)...)
}

// testCast is the cast spawns of a Model a test would build, for the tests
// that call a compute command directly.
func testCast(t seamT) castSpawns {
	t.Helper()
	return plainTestModel(t, Config{}, FrozenClock(testFrozenTime())).rows[surfaceText].spawn.cast
}

// rowOf is the default row of surface s, as a Model starts with it.
func defaultRowOf(s surface) (surfaceRow, bool) { return Model{rows: defaultRows()}.rowFor(s) }

// debounceOf is the debounce a Model starts with for surface s.
func debounceOf(s surface) time.Duration { return Model{rows: defaultRows()}.debounceFor(s) }

// markStubbed notes that the test (and its subtests) already replaced a spawn,
// so a fixture that would otherwise install its default leaves it alone.
func markStubbed(t seamT, flag string) {
	t.Helper()
	name := t.Name() + "|" + flag
	seamBook.Lock()
	seamBook.flags[name] = true
	seamBook.Unlock()
	t.Cleanup(func() {
		seamBook.Lock()
		defer seamBook.Unlock()
		delete(seamBook.flags, name)
	})
}

// isStubbed is whether markStubbed was called for the flag by the test or a parent.
func isStubbed(t seamT, flag string) bool {
	seamBook.Lock()
	defer seamBook.Unlock()
	for _, n := range testNames(t) {
		if seamBook.flags[n+"|"+flag] {
			return true
		}
	}
	return false
}

// sameModel is reflect.DeepEqual on two Models, the rows aside: a row holds
// functions, which DeepEqual never calls equal, and carries no screen state.
func sameModel(a, b Model) bool {
	a.rows, b.rows = [numSurfaces]surfaceRow{}, [numSurfaces]surfaceRow{}
	// The idle prefetch's period number moves on at every input (prefetch.go):
	// it names a rest, and says nothing about what the screen holds.
	a.pre.seq, b.pre.seq = 0, 0
	return reflect.DeepEqual(a, b)
}

// freshModels says that the Models this test built so far are not meant to see
// the stubs filed from here on: a fixture that files its own stubs and then
// builds a new Model calls it first, so a test may call the fixture twice.
func freshModels(t seamT) {
	t.Helper()
	seamBook.Lock()
	defer seamBook.Unlock()
	for _, n := range testNames(t) {
		delete(seamBook.built, n)
	}
}

// withNewStubs runs fn, which files stubs (a stub helper, or stubSpawns), and
// applies just those to m, a Model the test built before them. The stubs stay
// filed, so a Model built later sees them too.
func withNewStubs(t seamT, m Model, fn func()) Model {
	t.Helper()
	name := t.Name()
	seamBook.Lock()
	before := len(seamBook.edits[name])
	seamBook.grab[name] = true
	seamBook.Unlock()
	fn()
	seamBook.Lock()
	delete(seamBook.grab, name)
	added := append([]spawnEdit(nil), seamBook.edits[name][before:]...)
	seamBook.Unlock()
	return applySpawns(m, added...)
}

// fakeSeamT records what the registry would report against a test.
type fakeSeamT struct {
	name     string
	errs     []string
	cleanups []func()
}

func (f *fakeSeamT) Helper()      {}
func (f *fakeSeamT) Name() string { return f.name }
func (f *fakeSeamT) Errorf(format string, args ...any) {
	f.errs = append(f.errs, fmt.Sprintf(format, args...))
}
func (f *fakeSeamT) Cleanup(fn func()) { f.cleanups = append(f.cleanups, fn) }
func (f *fakeSeamT) finish() {
	for i := len(f.cleanups) - 1; i >= 0; i-- {
		f.cleanups[i]()
	}
}

func marked(called *string, mark string) spawnEdit {
	return func(s *rowSpawns) {
		s.timeline = func(context.Context, lookup.TimelineRequest) (lookup.TimelineResult, error) {
			*called = mark
			return lookup.TimelineResult{}, nil
		}
	}
}

func askTimeline(t *testing.T, m Model) {
	t.Helper()
	if _, err := m.rows[surfaceTimeline].spawn.timeline(context.Background(), lookup.TimelineRequest{}); err != nil {
		t.Fatal(err)
	}
}

// The stub registry is what keeps one test's stub off every other Model, and
// what refuses a stub the test's own Model cannot see. Each line below is a
// promise the converted tests lean on; reverting the check named in its
// message fails this test by that line.
func TestSpawnStubsReachOnlyTheModelsTheyAreFiledFor(t *testing.T) {
	var called string

	// A stub filed first reaches the Model built after it, and no other test's.
	a := &fakeSeamT{name: "TestA"}
	stubSpawns(a, marked(&called, "A"))
	askTimeline(t, newTestModel(a, Config{}, RealClock{}))
	if called != "A" {
		t.Errorf("the Model built after the stub did not hold it (called %q)", called)
	}
	called = ""
	b := &fakeSeamT{name: "TestB"}
	stubSpawns(b, marked(&called, "B"))
	askTimeline(t, newTestModel(b, Config{}, RealClock{}))
	if called != "B" {
		t.Errorf("TestB's Model ran %q, want its own stub, not TestA's", called)
	}
	b.finish()

	// A subtest sees its parent's stubs.
	sub := &fakeSeamT{name: "TestA/sub"}
	called = ""
	askTimeline(t, newTestModel(sub, Config{}, RealClock{}))
	if called != "A" {
		t.Errorf("a subtest's Model did not hold the parent's stub (called %q)", called)
	}
	sub.finish()
	a.finish()

	// A stub filed after the test built a Model is refused at the filing.
	late := &fakeSeamT{name: "TestLate"}
	stubSpawns(late, marked(&called, "early"))
	before := newTestModel(late, Config{}, RealClock{})
	stubSpawns(late, marked(&called, "late"))
	if len(late.errs) != 1 || !strings.Contains(late.errs[0], "after the test built a Model") {
		t.Errorf("a late stub was not refused: %q", late.errs)
	}
	called = ""
	askTimeline(t, before)
	if called != "early" {
		t.Errorf("the Model built before the late stub ran %q, want early", called)
	}
	late.finish()

	// freshModels, withSpawns and withNewStubs are the three ways out of that
	// refusal, each saying which Model is meant.
	ok := &fakeSeamT{name: "TestOK"}
	stubSpawns(ok, marked(&called, "base"))
	m := newTestModel(ok, Config{}, RealClock{})
	freshModels(ok)
	stubSpawns(ok, marked(&called, "fresh"))
	m2 := newTestModel(ok, Config{}, RealClock{})
	m3 := withNewStubs(ok, m, func() { stubSpawns(ok, marked(&called, "grabbed")) })
	if len(ok.errs) != 0 {
		t.Errorf("a sanctioned stub was refused: %q", ok.errs)
	}
	for want, model := range map[string]Model{"fresh": m2, "grabbed": m3} {
		called = ""
		askTimeline(t, model)
		if called != want {
			t.Errorf("the Model for %q ran %q", want, called)
		}
	}
	ok.finish()

	// A Model built with the outer t while a subtest holds stubs would miss them.
	outer := &fakeSeamT{name: "TestOuter"}
	inner := &fakeSeamT{name: "TestOuter/inner"}
	stubSpawns(inner, marked(&called, "inner"))
	newTestModel(outer, Config{}, RealClock{})
	if len(outer.errs) != 1 || !strings.Contains(outer.errs[0], "outer test's t") {
		t.Errorf("a Model built with the outer t was not refused: %q", outer.errs)
	}
	inner.finish()
	outer.finish()
}

// noVoids is the test default for the Moon lane's void search: it finds none and
// spawns nothing, so a Model that shows a Moon never reaches Astrolog for its
// lane. A test that wants the search stubs it with stubVoids or stubVoidsAhead.
func noVoids(context.Context, lookup.VoidRequest) (lookup.VoidResult, error) {
	return lookup.VoidResult{}, nil
}

// stubVoidsAhead stands in for both void searches with fn.
func stubVoidsAhead(t *testing.T, fn func(context.Context, lookup.VoidRequest) (lookup.VoidResult, error)) {
	t.Helper()
	stubSpawns(t, func(s *rowSpawns) { s.voids, s.voidsAhead = fn, fn })
}
