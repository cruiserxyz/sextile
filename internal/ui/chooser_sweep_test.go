package ui

// chooser_sweep_test.go runs every value of every chooser against the real
// Astrolog and writes a table (specs/no-dead-choices.md). It finds dead choices
// and fixes none.
//
// TestChooserSweep runs only when SEXTILE_SWEEP_OUT names the file to write:
//
//	SEXTILE_SWEEP_OUT=/path/chooser-sweep.md go test ./internal/ui/ -run '^TestChooserSweep$' -count=1 -timeout 60m
//
// with the complete Astrolog first on PATH and SEXTILE_ATLAS_PATH set.
// TestChooserSweepKnowsEveryChooser always runs and is quick: it reads the
// choosers from the code and fails when one is neither swept nor listed as not
// a choice, with its reason.
//
// A row is one value on one fixture chart: the floating chart cast now beside
// an anchored natal chart (timelineModel). The value is applied through the
// chooser's own path, and the Astrolog calls it caused and the text of the
// panels it should change are compared with the same chart without the value.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// --- the recorder -----------------------------------------------------------

// sweepCall is one Astrolog process: its argv, what it wrote on stderr and how
// it ended.
type sweepCall struct {
	argv   []string
	stderr []string
	exit   string
}

// sweepShim puts a stand-in first on PATH that records each call's argv, its
// stderr and its exit code in a file of its own (parallel calls cannot
// interleave) and then runs the real binary with the same arguments, passing
// its stderr through. read returns every call so far, oldest first.
func sweepShim(t *testing.T) (read func() []sweepCall, binary string) {
	t.Helper()
	real, err := exec.LookPath("astrolog")
	if err != nil {
		t.Skip("astrolog not on PATH; the sweep needs the real binary")
	}
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	if err := os.Mkdir(calls, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/bash\n" +
		"f=" + strconv.Quote(calls) + "/$(date +%s%N).$$\n" +
		"printf '%s\\n' \"$@\" > \"$f.argv\"\n" +
		strconv.Quote(real) + " \"$@\" 2> >(tee \"$f.err\" >&2)\n" +
		"rc=$?\n" +
		"echo $rc > \"$f.rc\"\n" +
		"exit $rc\n"
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() []sweepCall {
		names, _ := filepath.Glob(filepath.Join(calls, "*.argv"))
		sort.Strings(names)
		var out []sweepCall
		for _, n := range names {
			base := strings.TrimSuffix(n, ".argv")
			raw, err := os.ReadFile(n)
			if err != nil {
				continue
			}
			c := sweepCall{argv: strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")}
			if e, err := os.ReadFile(base + ".err"); err == nil {
				for _, l := range strings.Split(strings.TrimSpace(string(e)), "\n") {
					if l = strings.TrimSpace(l); l != "" {
						c.stderr = append(c.stderr, l)
					}
				}
			}
			if rc, err := os.ReadFile(base + ".rc"); err == nil {
				c.exit = strings.TrimSpace(string(rc))
			}
			out = append(out, c)
		}
		return out
	}, real
}

// argvGroups splits an argv into its switches, each with its values
// (`-c 14`, `-R 5`): a word opens a group when it starts with one of Astrolog's
// switch marks and is not a negative number.
var sweepNumber = regexp.MustCompile(`^-?[0-9.:]+$`)

func argvGroups(argv []string) []string {
	var out []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.Join(cur, " "))
		}
		cur = nil
	}
	for _, a := range argv {
		if a == "" {
			continue
		}
		if len(a) > 1 && strings.ContainsRune("-_=:", rune(a[0])) && !sweepNumber.MatchString(a) {
			flush()
		}
		cur = append(cur, strings.ReplaceAll(a, os.TempDir(), "<tmp>"))
	}
	flush()
	return out
}

// --- one observation ----------------------------------------------------------

// sweepObs is what one settled state held: the groups of every Astrolog call it
// took to get there, what Astrolog said, and the text of each panel.
type sweepObs struct {
	calls  [][]string // the switch groups of each call
	said   map[string]bool
	failed []string
	texts  map[string]string
}

// sweepWorld is the fixture the rows share.
type sweepWorld struct {
	t       *testing.T
	read    func() []sweepCall
	binary  string
	base    Model
	pairing Model // three charts selected, for the chart kinds
	cusps   *Model
	cfg     Config
}

func (w *sweepWorld) settle(m Model, cmd tea.Cmd) Model {
	w.t.Helper()
	m = pumpPatient(w.t, m, cmd)
	m, d := m.requestDetails()
	m = pumpPatient(w.t, m, d)
	m, tc := syncTimelineVisited(m)
	m = pumpPatient(w.t, m, tc)
	return m
}

// panelTexts is the text of each panel a chooser could change, whole: the
// rows, not the window the screen has room for.
func (w *sweepWorld) panelTexts(m Model) map[string]string {
	state := m.paintedChart()
	out := map[string]string{}
	out["Bodies"], _, _ = panel.RenderBodies(resolveBodiesBag(m.bodiesView(), state, m.slots, m.cursorIdx, m.bodyCursorIdx, 140, 900, false, w.cfg))
	out["Aspects"] = renderAspectsBody(state, TabAspects, 140, 900, w.cfg)
	sc := m.deriveScene()
	for i := range sc.pairs {
		out["Aspects "+string(sc.pairTabID(i))] = renderAspectsBody(state, sc.pairTabID(i), 140, 900, w.cfg)
	}
	out["Details"] = m.details.body
	frame := ansi.Strip(m.View())
	if r, ok := m.geometry()[PanelChart]; ok {
		var rows []string
		for i, l := range strings.Split(frame, "\n") {
			if i >= r.y && i < r.y+r.h {
				rows = append(rows, ansi.Cut(l, r.x, r.x+r.w))
			}
		}
		out["Chart"] = strings.Join(rows, "\n")
	}
	out["Frame"] = frame
	out["Window title"] = m.windowTitle()
	var tl []string
	for _, r := range m.timeline.rows {
		tl = append(tl, r.Line)
	}
	out["Timeline"] = strings.Join(tl, "\n")
	for k, v := range out {
		out[k] = ansi.Strip(v)
	}
	out["Frame colours"] = m.View()
	return out
}

// observe settles what apply produced and reads the calls it took.
func (w *sweepWorld) observe(m Model, cmd tea.Cmd) (Model, sweepObs) {
	mark := len(w.read())
	m = w.settle(m, cmd)
	obs := sweepObs{said: map[string]bool{}, texts: w.panelTexts(m)}
	for _, c := range w.read()[mark:] {
		obs.calls = append(obs.calls, argvGroups(c.argv))
		for _, l := range c.stderr {
			obs.said[l] = true
		}
		if c.exit != "" && c.exit != "0" {
			obs.failed = append(obs.failed, "exit "+c.exit+": "+strings.Join(c.stderr, " "))
		}
	}
	return m, obs
}

// --- choosers and rows ---------------------------------------------------------

// sweepValue is one value of a chooser. with puts the value in force through
// the chooser's own path; without is the same chart held at another value (or
// without the value), under without's key, so a shared baseline is cast once.
// unreachable, when set, says why the sweep cannot try the value at all.
type sweepValue struct {
	label      string
	withKey    string
	with       func(w *sweepWorld, m Model) (Model, tea.Cmd, string)
	withoutKey string
	without    func(w *sweepWorld, m Model) (Model, tea.Cmd)
	// base is the model the row starts from (the fixture when nil).
	base func(w *sweepWorld) Model
}

type sweepChooser struct {
	name   string // the heading
	source string // where its values come from, one line
	// panels are the panel texts the value should change.
	panels []string
	// drawsOnly says the value changes what is drawn from data already cast, so
	// no new Astrolog call is expected and the text alone decides.
	drawsOnly bool
	values    func(w *sweepWorld) []sweepValue
}

type sweepRow struct {
	value, argv, came, verdict string
}

const (
	verdictWorks    = "works"
	verdictNothing  = "does nothing"
	verdictCannot   = "cannot be tried"
	sweepCameLimit  = 80
	sweepArgvLimit  = 90
	sweepFixtureWho = "the floating chart now (Sat 17 Oct 2026 12:00, Amsterdam) beside the natal chart of Tue 15 May 1979 11:30, Amsterdam"
)

func sweepCut(s string, n int) string {
	s = strings.NewReplacer("|", "/", "\n", " ").Replace(strings.TrimSpace(s))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func sortedKeys2(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// diffGroups is what the calls of one run changed against the other's: each
// call is matched with the call of the other run it differs from least, and the
// switches that differ are gathered (a run that also took a call the other did
// not, a timeline, adds nothing).
func diffGroups(with, without [][]string) string {
	set := func(g []string) map[string]bool {
		m := map[string]bool{}
		for _, x := range g {
			m[x] = true
		}
		return m
	}
	plus, minus := map[string]bool{}, map[string]bool{}
	if len(without) == 0 {
		return ""
	}
	for _, c := range with {
		a := set(c)
		best, bestN := []string(nil), -1
		var bp, bm []string
		for _, o := range without {
			b := set(o)
			var p, m []string
			for g := range a {
				if !b[g] {
					p = append(p, g)
				}
			}
			for g := range b {
				if !a[g] {
					m = append(m, g)
				}
			}
			if n := len(p) + len(m); bestN < 0 || n < bestN {
				best, bestN, bp, bm = o, n, p, m
			}
		}
		_ = best
		for _, g := range bp {
			plus[g] = true
		}
		for _, g := range bm {
			minus[g] = true
		}
	}
	var parts []string
	if p := sortedKeys2(plus); len(p) > 0 {
		parts = append(parts, "+"+strings.Join(p, " +"))
	}
	if m := sortedKeys2(minus); len(m) > 0 {
		parts = append(parts, "-"+strings.Join(m, " -"))
	}
	return strings.Join(parts, "  ")
}

// firstChange is the first line the panel gained.
func firstChange(with, without string) string {
	had := map[string]bool{}
	for _, l := range strings.Split(without, "\n") {
		had[strings.TrimSpace(l)] = true
	}
	for _, l := range strings.Split(with, "\n") {
		if l = strings.TrimSpace(l); l != "" && !had[l] {
			return l
		}
	}
	lost := map[string]bool{}
	for _, l := range strings.Split(with, "\n") {
		lost[strings.TrimSpace(l)] = true
	}
	for _, l := range strings.Split(without, "\n") {
		if l = strings.TrimSpace(l); l != "" && !lost[l] {
			return "gone: " + l
		}
	}
	return ""
}

func (w *sweepWorld) judge(c sweepChooser, with, without sweepObs) sweepRow {
	argvDiff := diffGroups(with.calls, without.calls)
	var changed []string
	came := ""
	for _, p := range c.panels {
		for name, text := range with.texts {
			if name != p && !strings.HasPrefix(name, p+" ") {
				continue
			}
			if text != without.texts[name] {
				changed = append(changed, name)
				if came == "" {
					came = name + ": " + firstChange(text, without.texts[name])
				}
			}
		}
	}
	sort.Strings(changed)
	var said []string
	for _, l := range sortedKeys2(with.said) {
		if !without.said[l] {
			said = append(said, l)
		}
	}
	row := sweepRow{argv: sweepCut(argvDiff, sweepArgvLimit)}
	if row.argv == "" {
		row.argv = "none"
	}
	// A value the model already holds the cast for (the chart was cast at it
	// earlier) issues no new call: the other value's call is then what differs.
	held := len(with.calls) == 0 && len(without.calls) > 0
	if held {
		row.argv = "no new call: the cast for it was already held"
	}
	argvOK := argvDiff != "" || c.drawsOnly || held
	switch {
	case len(with.failed) > 0 && len(without.failed) == 0:
		row.verdict = verdictNothing
		row.came = "Astrolog failed: " + with.failed[0]
	case argvOK && len(changed) > 0:
		row.verdict = verdictWorks
		row.came = came
	default:
		row.verdict = verdictNothing
		switch {
		case len(said) > 0:
			row.came = "Astrolog said: " + said[0]
		case argvDiff == "" && !c.drawsOnly:
			row.came = "the call did not change"
		default:
			row.came = "the call changed and no panel text did"
		}
		if len(said) > 0 && len(changed) == 0 && argvDiff != "" {
			row.came = "Astrolog said: " + said[0]
		}
	}
	row.came = sweepCut(row.came, sweepCameLimit)
	return row
}

// runChooser casts every value of one chooser.
func (w *sweepWorld) runChooser(c sweepChooser) ([]sweepRow, time.Duration) {
	start := time.Now()
	type held struct {
		obs sweepObs
	}
	baselines := map[string]sweepObs{}
	var rows []sweepRow
	for _, v := range w.chooserValues(c) {
		base := w.base
		if v.base != nil {
			base = v.base(w)
		}
		m1, cmd, why := v.with(w, base)
		if why != "" {
			rows = append(rows, sweepRow{value: v.label, argv: "none", came: sweepCut(why, sweepCameLimit), verdict: verdictCannot})
			continue
		}
		_, with := w.observe(m1, cmd)
		key := c.name + "/" + v.withoutKey
		without, ok := baselines[key]
		if !ok {
			m0, cmd0 := v.without(w, base)
			_, without = w.observe(m0, cmd0)
			baselines[key] = without
		}
		r := w.judge(c, with, without)
		r.value = v.label
		rows = append(rows, r)
	}
	return rows, time.Since(start)
}

func (w *sweepWorld) chooserValues(c sweepChooser) []sweepValue { return c.values(w) }

// --- driving the model ----------------------------------------------------------

func (w *sweepWorld) press(m Model, k tea.KeyMsg) Model {
	tm, _ := m.Update(k)
	return tm.(Model)
}

func (w *sweepWorld) pressN(m Model, k tea.KeyMsg, n int) Model {
	for i := 0; i < n; i++ {
		m = w.press(m, k)
	}
	return m
}

func (w *sweepWorld) enter(m Model) (Model, tea.Cmd) {
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return tm.(Model), cmd
}

var (
	sweepDown  = tea.KeyMsg{Type: tea.KeyDown}
	sweepUp    = tea.KeyMsg{Type: tea.KeyUp}
	sweepSpace = tea.KeyMsg{Type: tea.KeySpace}
)

// chooseInFinder opens the finder with open, walks the cursor to row want of
// its list and presses Enter: the finder's own path to the chooser's door.
func (w *sweepWorld) chooseInFinder(m Model, open func(Model) (tea.Model, tea.Cmd), cursor func(Model) (cur, rows int), index func(Model) int) (Model, tea.Cmd, string) {
	tm, _ := open(m)
	m = tm.(Model)
	idx := index(m)
	if idx < 0 {
		return m, nil, "the finder does not list it"
	}
	cur, _ := cursor(m)
	for cur < idx {
		m = w.press(m, sweepDown)
		cur, _ = cursor(m)
	}
	for cur > idx {
		m = w.press(m, sweepUp)
		cur, _ = cursor(m)
	}
	m, cmd := w.enter(m)
	return m, cmd, ""
}

// --- the fixture --------------------------------------------------------------------

func newSweepWorld(t *testing.T) *sweepWorld {
	t.Helper()
	read, binary := sweepShim(t)
	w := &sweepWorld{t: t, read: read, binary: binary, cfg: Config{Accessible: true, Color: ColorNever}}
	t.Cleanup(func() { lookup.HoldClock(lookup.ClockPref{}) })
	m, _ := liveScreens(t)
	m.width, m.height = 200, 100
	m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
	// The binary's own lists, as the startup read lands them.
	ctx := t.Context()
	asp, err := lookup.AspectRegistry(ctx)
	if err != nil {
		t.Fatalf("aspect registry: %v", err)
	}
	m, _ = applyAspectRegistryLoaded(m, AspectRegistryLoadedMsg{Registry: asp})
	objs, err := lookup.ObjectRegistry(ctx)
	if err != nil {
		t.Fatalf("object registry: %v", err)
	}
	m, _ = applyObjectRegistryLoaded(m, ObjectRegistryLoadedMsg{Registry: objs})
	m, cmd := m.bumpIdentity(surfaceText)
	m = w.settle(m, cmd)
	w.base = m
	w.pairing = w.comparison(m)
	return w
}

var sweepCuspName = regexp.MustCompile(`^[0-9]+(st|nd|rd|th) Cusp$`)

// cuspBase is the fixture held at the Sun, the Moon and every house cusp the
// object registry lists: a house system that differs from another only in
// its intermediate cusps changes no text unless the cusps are drawn.
func (w *sweepWorld) cuspBase() Model {
	if w.cusps != nil {
		return *w.cusps
	}
	m := w.base
	sel := []int{1, 2}
	for _, r := range m.objectRegistry {
		if sweepCuspName.MatchString(r.Object.Name) {
			sel = append(sel, r.Object.Number)
		}
	}
	m.lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: sel}
	m, cmd := m.bumpIdentity(surfaceText)
	m = w.settle(m, cmd)
	w.cusps = &m
	return m
}

// withCuspBase holds a house chooser's rows at the fixture with the cusps drawn.
func withCuspBase(c sweepChooser) sweepChooser {
	inner := c.values
	c.values = func(w *sweepWorld) []sweepValue {
		vs := inner(w)
		for i := range vs {
			v := vs[i]
			vs[i].base = (*sweepWorld).cuspBase
			// The chart may already be at the value (Whole Sign is the default):
			// choosing it must be a change, so it is chosen from the other value.
			vs[i].with = func(w *sweepWorld, m Model) (Model, tea.Cmd, string) {
				m0, cmd0 := v.without(w, m)
				return v.with(w, w.settle(m0, cmd0))
			}
		}
		return vs
	}
	c.source += "; held at the Sun, the Moon and every house cusp object"
	return c
}

// comparison is the fixture with a second anchored chart and all three
// selected, so the Aspects panel offers every kind of pair.
func (w *sweepWorld) comparison(m Model) Model {
	if err := addAnchor(m.slots, time.Date(1971, 8, 20, 4, 40, 0, 0, time.UTC), testPlaceAmsterdam()); err != nil {
		w.t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		m.slots.SetSlotSelected(i, true)
	}
	m.cursorIdx = 1
	m.base = slotAt(m, 0)
	m.focused = PanelAspects
	m, cmd := m.noteWant(surfaceText, true)
	return w.settle(m, cmd)
}

// --- the choosers --------------------------------------------------------------------

func floatingIdx(m Model) int { return len(m.slots.All()) - 1 }

func sweepHouses() sweepChooser {
	kind := popup.HouseKind()
	return sweepChooser{
		name:   "Houses",
		source: "popup.HouseKind().All, the house finder (`, u`) and its door chooseHouseSystem",
		panels: []string{"Bodies", "Details"},
		values: func(w *sweepWorld) []sweepValue {
			set := func(h recipe.HouseSystem) func(*sweepWorld, Model) (Model, tea.Cmd, string) {
				return func(w *sweepWorld, m Model) (Model, tea.Cmd, string) {
					return w.chooseInFinder(m, openHouseFinder,
						func(m Model) (int, int) { return m.houseFinder.Cursor, len(m.houseFinder.Matches) },
						func(m Model) int {
							for i, x := range m.houseFinder.Matches {
								if x == h {
									return i
								}
							}
							return -1
						})
				}
			}
			ref, ref2 := kind.All[0], kind.All[1]
			var out []sweepValue
			for _, h := range kind.All {
				other := ref
				if h == ref {
					other = ref2
				}
				otherSet := set(other)
				out = append(out, sweepValue{
					label: lookup.HouseName(h), withKey: lookup.HouseName(h), with: set(h),
					withoutKey: lookup.HouseName(other),
					without: func(w *sweepWorld, m Model) (Model, tea.Cmd) {
						m, cmd, _ := otherSet(w, m)
						return m, cmd
					},
				})
			}
			return out
		},
	}
}

func sweepZodiacs() sweepChooser {
	kind := popup.ZodiacKind()
	return sweepChooser{
		name:   "Zodiacs",
		source: "popup.ZodiacKind().All, the zodiac finder (`, z`) and its door chooseZodiac",
		panels: []string{"Bodies", "Aspects", "Details"},
		values: func(w *sweepWorld) []sweepValue {
			set := func(z recipe.Zodiac) func(*sweepWorld, Model) (Model, tea.Cmd, string) {
				return func(w *sweepWorld, m Model) (Model, tea.Cmd, string) {
					return w.chooseInFinder(m, openZodiacFinder,
						func(m Model) (int, int) { return m.zodiacFinder.Cursor, len(m.zodiacFinder.Matches) },
						func(m Model) int {
							for i, x := range m.zodiacFinder.Matches {
								if x == z {
									return i
								}
							}
							return -1
						})
				}
			}
			ref, ref2 := kind.All[0], kind.All[1]
			var out []sweepValue
			for _, z := range kind.All {
				other := ref
				if z == ref {
					other = ref2
				}
				otherSet := set(other)
				out = append(out, sweepValue{
					label: z.Name(), withKey: z.Name(), with: set(z),
					withoutKey: other.Name(),
					without: func(w *sweepWorld, m Model) (Model, tea.Cmd) {
						m, cmd, _ := otherSet(w, m)
						return m, cmd
					},
				})
			}
			return out
		},
	}
}

func sweepNodes() sweepChooser {
	kind := popup.NodesKind(recipe.NodesMean, "")
	return sweepChooser{
		name:   "Nodes and Lilith",
		source: "popup.NodesKind().All, the Nodes and Lilith chooser (`, t`) and its door chooseNodes",
		panels: []string{"Bodies", "Details"},
		values: func(w *sweepWorld) []sweepValue {
			set := func(f recipe.NodeForm) func(*sweepWorld, Model) (Model, tea.Cmd, string) {
				return func(w *sweepWorld, m Model) (Model, tea.Cmd, string) {
					return w.chooseInFinder(m, openNodesFinder,
						func(m Model) (int, int) { return m.nodesFinder.Cursor, len(m.nodesFinder.Matches) },
						func(m Model) int {
							for i, x := range m.nodesFinder.Matches {
								if x == f {
									return i
								}
							}
							return -1
						})
				}
			}
			var out []sweepValue
			for _, f := range kind.All {
				other := kind.All[0]
				if f == other {
					other = kind.All[1]
				}
				otherSet := set(other)
				out = append(out, sweepValue{
					label: popup.NodeFormName(f), withKey: popup.NodeFormName(f), with: set(f),
					withoutKey: popup.NodeFormName(other),
					without: func(w *sweepWorld, m Model) (Model, tea.Cmd) {
						m, cmd, _ := otherSet(w, m)
						return m, cmd
					},
				})
			}
			return out
		},
	}
}

// sweepAspectBase is the two aspects the chart is held at: the without side of
// every aspect row is these two, less the value.
func sweepAspectBase(reg []aspect.RegistryAspect) []int {
	var out []int
	for _, r := range reg {
		if r.CountsAnything() && len(out) < 2 {
			out = append(out, r.Kind.Index)
		}
	}
	return out
}

func without(set []int, x int) []int {
	var out []int
	for _, n := range set {
		if n != x {
			out = append(out, n)
		}
	}
	return out
}

func sweepAspects() sweepChooser {
	return sweepChooser{
		name:   "Aspects",
		source: "the binary's aspect registry (`astrolog -HA`, lookup.AspectRegistry), the Aspects chooser (Ctrl+A)",
		panels: []string{"Aspects", "Details", "Timeline"},
		values: func(w *sweepWorld) []sweepValue {
			reg := w.base.aspectRegistry
			held := sweepAspectBase(reg)
			var out []sweepValue
			for i, r := range reg {
				idx, row := r.Kind.Index, i
				start := recipe.AspectPolicy{Explicit: true, Indices: without(held, idx)}
				out = append(out, sweepValue{
					label:   fmt.Sprintf("%s (%s, %g°, orb %g)", r.Kind.Name, r.Kind.Abbrev, r.Angle, r.Orb),
					withKey: strconv.Itoa(idx),
					with: func(w *sweepWorld, m Model) (Model, tea.Cmd, string) {
						m.lens.Aspects = start
						tm, _ := openAspectChooser(m)
						m = tm.(Model)
						if m.startup != startupAspectOpen {
							return m, nil, "the chooser did not open"
						}
						m = w.pressN(m, sweepDown, popup.FamilyRows()+row)
						if got := m.aspectModal.Matches[m.aspectModal.Cursor-popup.FamilyRows()]; got.Kind.Index != idx {
							return m, nil, "the cursor did not reach the row"
						}
						m = w.press(m, sweepSpace)
						if !m.aspectModal.Selected[idx] {
							if !m.aspectModal.Selectable(idx) {
								return m, nil, fmt.Sprintf("the row is greyed in the chooser: orb %g counts nothing", r.Orb)
							}
							return m, nil, "the toggle did not tick the row"
						}
						m, cmd := w.enter(m)
						return m, cmd, ""
					},
					withoutKey: "base-" + strconv.Itoa(idx),
					without: func(w *sweepWorld, m Model) (Model, tea.Cmd) {
						m.lens.Aspects = start
						return m.bumpIdentity(surfaceText)
					},
				})
			}
			return out
		},
	}
}

func sweepBodies() sweepChooser {
	return sweepChooser{
		name:   "Bodies",
		source: "the binary's object registry (`astrolog -R1 -HO`, lookup.ObjectRegistry), the Bodies chooser (Ctrl+B)",
		panels: []string{"Bodies", "Aspects", "Details", "Timeline"},
		values: func(w *sweepWorld) []sweepValue {
			reg := w.base.objectRegistry
			held := []int{1, 2}
			var out []sweepValue
			for _, r := range reg {
				num := r.Object.Number
				label := fmt.Sprintf("%d %s", num, r.Object.Name)
				if r.Star {
					label += " (star)"
				}
				start := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: without(held, num)}
				out = append(out, sweepValue{
					label:   label,
					withKey: strconv.Itoa(num),
					with: func(w *sweepWorld, m Model) (Model, tea.Cmd, string) {
						m.lens.Objects = start
						tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
						m = tm.(Model)
						if m.startup != startupBodyOpen {
							return m, nil, "the chooser did not open"
						}
						idx := -1
						for i, x := range m.bodyModal.Matches {
							if x.Object.Number == num {
								idx = i
							}
						}
						if idx < 0 {
							return m, nil, "the chooser does not list it"
						}
						m = w.pressN(m, sweepDown, popup.GroupRows()+idx)
						m = w.press(m, sweepSpace)
						if !m.bodyModal.Selected[num] {
							if e, no := m.bodyModal.Undrawable(num); no {
								said := e.Said
								if said == "" && len(e.Files) > 0 {
									said = "missing " + strings.Join(e.Files, ", ")
								}
								return m, nil, "the chooser marks it as not drawable here: " + said
							}
							return m, nil, "the toggle did not tick the row"
						}
						m, cmd := w.enter(m)
						return m, cmd, ""
					},
					withoutKey: "base-" + strconv.Itoa(num),
					without: func(w *sweepWorld, m Model) (Model, tea.Cmd) {
						m.lens.Objects = start
						return m.bumpIdentity(surfaceText)
					},
				})
			}
			return out
		},
	}
}

// sweepKinds is the Aspects panel's tabs: the chart alone, the floating chart
// over an anchored one, an anchored chart over another. The tab is read from
// data cast when the pair was selected, so the text alone decides.
func sweepKinds() sweepChooser {
	return sweepChooser{
		name:      "Chart kinds (the Aspects panel's tabs)",
		source:    "Model.panelTabs(PanelAspects) with three charts selected, once with the cursor on an anchored chart and once on the floating one, chosen with `]`",
		panels:    []string{"Aspects"},
		drawsOnly: true,
		values: func(w *sweepWorld) []sweepValue {
			var out []sweepValue
			for _, at := range []struct {
				name string
				idx  int
			}{{"cursor on chart 2", 1}, {"cursor on the floating chart", 2}} {
				at := at
				place := func(w *sweepWorld) Model {
					m := w.pairing
					m.cursorIdx = at.idx
					m, cmd := m.noteWant(surfaceText, true)
					return w.settle(m, cmd)
				}
				tabs := place(w).panelTabs(PanelAspects)
				for i, tab := range tabs {
					n := i
					other := 1
					if n > 0 {
						other = 0
					}
					out = append(out, sweepValue{
						label:   fmt.Sprintf("%s: %s (%s)", at.name, tab.Title, tab.ID),
						withKey: at.name + string(tab.ID),
						base:    place,
						with: func(w *sweepWorld, m Model) (Model, tea.Cmd, string) {
							return w.tabAt(m, n)
						},
						withoutKey: fmt.Sprintf("%s-tab%d", at.name, other),
						without: func(w *sweepWorld, m Model) (Model, tea.Cmd) {
							m, cmd, _ := w.tabAt(m, other)
							return m, cmd
						},
					})
				}
			}
			return out
		},
	}
}

// tabAt chooses the Aspects tab n from the first, with `]`.
func (w *sweepWorld) tabAt(m Model, n int) (Model, tea.Cmd, string) {
	m = w.withFirstTab(m)
	var cmd tea.Cmd
	for k := 0; k < n; k++ {
		var c tea.Cmd
		m, c = w.pressTab(m)
		cmd = tea.Batch(cmd, c)
	}
	return m, cmd, ""
}

func (w *sweepWorld) withFirstTab(m Model) Model { return m.withDetailsTab(PanelAspects, TabAspects) }

func (w *sweepWorld) pressTab(m Model) (Model, tea.Cmd) {
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	return tm.(Model), cmd
}

// sweepSetting is a chooser whose values are the words a preference key may
// carry. A value is put in force through the Model's own apply function for
// that key (the path the startup read takes) and the chart is cast again.
func sweepSetting(name, source string, panels []string, drawsOnly bool, words []string, apply func(Model, string) Model) sweepChooser {
	return sweepChooser{
		name: name, source: source, panels: panels, drawsOnly: drawsOnly,
		values: func(w *sweepWorld) []sweepValue {
			var out []sweepValue
			for _, v := range words {
				other := words[0]
				if v == other {
					other = words[1]
				}
				v, other := v, other
				out = append(out, sweepValue{
					label: v, withKey: v,
					with: func(w *sweepWorld, m Model) (Model, tea.Cmd, string) {
						m, cmd := apply(m, v).bumpIdentity(surfaceText)
						return m, cmd, ""
					},
					withoutKey: other,
					without: func(w *sweepWorld, m Model) (Model, tea.Cmd) {
						return apply(m, other).bumpIdentity(surfaceText)
					},
				})
			}
			return out
		},
	}
}

// sweepUnreachable is a chooser whose values the sweep cannot try, each with the reason.
func sweepUnreachable(name, source string, words []string, why string) sweepChooser {
	return sweepChooser{
		name: name, source: source,
		values: func(w *sweepWorld) []sweepValue {
			var out []sweepValue
			for _, v := range words {
				out = append(out, sweepValue{label: v, with: func(*sweepWorld, Model) (Model, tea.Cmd, string) { return Model{}, nil, why }})
			}
			return out
		},
	}
}

func sweepSettings() []sweepChooser {
	var forms, houses, zodiacs, nodes []string
	for _, f := range houseForms {
		forms = append(forms, f.value)
	}
	for h := recipe.HouseSystem(0); h < recipe.HouseSystemCount; h++ {
		houses = append(houses, lookup.HouseName(h))
	}
	for z := recipe.Zodiac(0); int(z) < recipe.ZodiacCount; z++ {
		zodiacs = append(zodiacs, z.RecordName())
	}
	for f := recipe.NodesMean; f <= recipe.NodesTrueNatural; f++ {
		n, l := f.Words()
		nodes = append(nodes, n+" / "+l)
	}
	return []sweepChooser{
		sweepSetting("Settings: clock hours", "preferences.yaml `clock: hours:` (store.ClockConfig, Model.applyClockConfig)",
			[]string{"Chart", "Details", "Timeline"}, false, []string{"24", "12"},
			func(m Model, v string) Model {
				return m.applyClockConfig(store.ClockConfig{Hours: v, Dates: "month-first"})
			}),
		sweepSetting("Settings: clock dates", "preferences.yaml `clock: dates:` (store.ClockConfig, Model.applyClockConfig)",
			[]string{"Chart", "Details", "Timeline"}, false, []string{"month-first", "day-first"},
			func(m Model, v string) Model { return m.applyClockConfig(store.ClockConfig{Hours: "24", Dates: v}) }),
		sweepSetting("Settings: house display", "preferences.yaml `house_display:` (the four forms of the house finder's Shown as part, Model.applyHouseDisplay)",
			[]string{"Bodies"}, true, forms,
			func(m Model, v string) Model {
				return m.applyHouseDisplay(store.HouseDisplayConfig{Value: v, Present: true})
			}),
		sweepSetting("Settings: frame houses", "preferences.yaml `frame: houses:` (every house system, Model.applyFrameConfig)",
			[]string{"Bodies", "Details"}, false, houses,
			func(m Model, v string) Model {
				return m.applyFrameConfig(store.FrameConfig{Houses: v, HousesPresent: true})
			}),
		sweepSetting("Settings: frame zodiac", "preferences.yaml `frame: zodiac:` (every zodiac in its record form, Model.applyFrameConfig)",
			[]string{"Bodies", "Aspects", "Details"}, false, zodiacs,
			func(m Model, v string) Model {
				return m.applyFrameConfig(store.FrameConfig{Zodiac: v, ZodiacPresent: true})
			}),
		sweepSetting("Settings: frame Nodes and Lilith", "preferences.yaml `frame: node:` and `lilith:` (the four castable pairs, Model.applyFrameConfig)",
			[]string{"Bodies", "Details"}, false, nodes,
			func(m Model, v string) Model {
				parts := strings.SplitN(v, " / ", 2)
				return m.applyFrameConfig(store.FrameConfig{Node: parts[0], NodePresent: true, Lilith: parts[1], LilithPresent: true})
			}),
		sweepUnreachable("Settings: window title", "preferences.yaml `window_title:` (Model.applyWindowTitle)", []string{"session"},
			"the title names the session only while a named session runs; the sweep runs the unnamed one"),
		sweepUnreachable("Settings: wheel format", "preferences.yaml `wheel_format:` (Model.applyWheelFormat)", []string{"svg", "png"},
			"the format reaches only the wheel image served to a browser; the sweep reads panel text"),
	}
}

// sweepExperiments is the Experiments popup: each switch is turned on through
// the popup (Space on its row) and the one thing it changes is then done, an
// Enter on a Timeline row with colours on; the whole frame is compared with the
// switch off.
func sweepExperiments() sweepChooser {
	return sweepChooser{
		name:      "Experiments",
		source:    "the `experiments` table (internal/ui/experiments.go), the Experiments popup (`, L p`)",
		panels:    []string{"Frame"},
		drawsOnly: true,
		values: func(w *sweepWorld) []sweepValue {
			var out []sweepValue
			for i, row := range experiments {
				i, row := i, row
				drive := func(w *sweepWorld, m Model, on bool) (Model, tea.Cmd, string) {
					m.cfg.Color = ColorAlways
					m.cfg.Accessible = false
					if row.id == expAspectDot {
						// The switch changes how an aspect with no glyph is
						// drawn, so Septile is ticked first and a row of it is
						// on the screen, with the switch on and off alike.
						m = sweepTickSeptile(w, m)
					}
					if on {
						tm, _ := m.openExperiments()
						m = tm.(Model)
						m = w.pressN(m, sweepDown, i)
						tm, cmd := m.Update(sweepSpace)
						m = tm.(Model)
						if cmd == nil {
							return m, nil, "Space on the row started no write"
						}
						tm, _ = m.Update(cmd())
						m = tm.(Model)
						m = w.press(m, tea.KeyMsg{Type: tea.KeyEnter})
						if !m.experiment(row.id) {
							return m, nil, "the popup did not switch it on"
						}
					}
					if row.id == expAspectDot {
						return m, nil, ""
					}
					tm, open := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
					m = pumpPatient(w.t, tm.(Model), open)
					m = w.press(m, tea.KeyMsg{Type: tea.KeyEnter})
					at := -1
					for j, r := range m.timeline.rows {
						for _, a := range []string{" Con ", " Opp ", " Squ ", " Tri ", " Sex "} {
							if at < 0 && strings.Contains(r.Line, a) {
								at = j
							}
						}
					}
					if at < 0 {
						return m, nil, "the Timeline has no aspect row to press Enter on"
					}
					m.details.cursor = at
					tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
					return tm.(Model), cmd, ""
				}
				out = append(out, sweepValue{
					label: row.id, withKey: row.id,
					with:       func(w *sweepWorld, m Model) (Model, tea.Cmd, string) { return drive(w, m, true) },
					withoutKey: "off-" + row.id,
					without: func(w *sweepWorld, m Model) (Model, tea.Cmd) {
						m, cmd, _ := drive(w, m, false)
						return m, cmd
					},
				})
			}
			return out
		},
	}
}

// sweepTickSeptile ticks Septile in the lens, as the Aspects chooser does, casts
// again, and makes sure a Septile row is on the screen: the fixture chart has
// none of its own within orb, so one is added to the cast's own list and to the
// pair list, as Astrolog lists it for a pair.
func sweepTickSeptile(w *sweepWorld, m Model) Model {
	const septile = 13
	held := sweepAspectBase(w.base.aspectRegistry)
	m.lens.Aspects = recipe.AspectPolicy{Explicit: true, Indices: append(append([]int(nil), held...), septile)}
	m2, cmd := m.bumpIdentity(surfaceText)
	m = w.settle(m2, cmd)
	row, ok := m.lens.AspectRegistry.ByIndex(septile)
	if !ok {
		w.t.Fatal("the registry holds no Septile")
	}
	has := false
	for _, a := range m.chart.aspects {
		has = has || a.Kind == row.Kind
	}
	if !has {
		m.chart.aspects = append(m.chart.aspects, aspect.NewObjectAspect(objMercuryFixture, objSaturn, row.Kind, aspect.Orb(20.0/60)))
		m.chart.crossAspects = append(m.chart.crossAspects, lookup.OverlayAspect{TransitObject: objSaturn, NatalObject: objMercuryFixture, Kind: row.Kind, Orb: aspect.Orb(20.0 / 60)})
	}
	return m
}

// --- the inventory, read from the code -----------------------------------------------

// notAChoice is a surface the code names that offers no list of values to
// sweep, with the reason.
var notAChoice = map[string]string{
	// modals named in startupShowsModal
	"startupAtlasOpen":       "the place search: a typed search over the atlas file, not a list of values",
	"startupChartOpen":       "the chart picker opens a chart; which chart is data, not a setting",
	"startupSessionsOpen":    "the sessions popup opens or removes a session; a session is data, not a setting",
	"startupSessionNameOpen": "the session name form takes typed text",
	"startupSaveOpen":        "the save form takes typed text and a place",
	"startupTitleOpen":       "the title form takes typed text",
	// files that own a filtered list
	"internal/ui/chart_modal.go":         "the chart picker (see startupChartOpen)",
	"internal/ui/details_filter.go":      "the `/` search over the Details report's lines: it keeps rows, it offers no values",
	"internal/ui/timeline.go":            "the `/` search over the Timeline's rows: it keeps rows, it offers no values",
	"internal/ui/search_line.go":         "the key reader shared by the Details `/` search lines: it types a query, it offers no values",
	"internal/ui/keys/keybindings.go":    "the Keybindings popup lists keys to read; nothing in it is chosen",
	"internal/ui/panel/timeline_body.go": "draws the Timeline's `/` search line: no values",
	"internal/ui/popup/atlas.go":         "the place search (see startupAtlasOpen)",
	"internal/ui/popup/aspects.go":       "swept as Aspects",
	"internal/ui/popup/body.go":          "swept as Bodies",
	"internal/ui/popup/finder.go":        "swept as Houses, Zodiacs and Nodes and Lilith",
	// preference keys the store reads
	"pref:version":          "the file's own generation number, written by the program",
	"pref:location":         "the default place: a name, a country, a latitude, a longitude and a zone, typed",
	"pref:name":             "the default place's name, typed",
	"pref:country":          "the default place's country, typed",
	"pref:lat":              "the default place's latitude, a number",
	"pref:lon":              "the default place's longitude, a number",
	"pref:tz":               "the default place's zone, a name from the zone database",
	"pref:display":          "the block of the two colours; each is a free value, see its keys",
	"pref:focus_color":      "a colour number or #rrggbb typed freely: no list of values, and colour is not text",
	"pref:cursor_row_color": "a colour number or #rrggbb typed freely: no list of values, and colour is not text",
	"pref:row_stripe":       "retired; read only to say it is retired",
	"pref:keys":             "the block of key settings",
	"pref:leader":           "the key that starts a command: one typed key, no list",
	"pref:timeoutlen":       "read and kept; nothing uses it",
	"pref:clock":            "the block of clock settings (swept as Settings, its two keys)",
	"pref:frame":            "the block of defaults (swept as Settings, its keys)",
	"pref:experimental":     "the block of switches (swept as Experiments)",
}

// sweptPrefs maps a preference key the sweep covers to the chooser that holds it.
var sweptPrefs = map[string]string{
	"hours":         "Settings: clock hours",
	"dates":         "Settings: clock dates",
	"house_display": "Settings: house display",
	"window_title":  "Settings: window title",
	"wheel_format":  "Settings: wheel format",
	"houses":        "Settings: frame houses",
	"zodiac":        "Settings: frame zodiac",
	"node":          "Settings: frame Nodes and Lilith",
	"lilith":        "Settings: frame Nodes and Lilith",
}

// sweptModals maps a modal state to the chooser that sweeps it.
var sweptModals = map[string]string{
	"startupAspectOpen":      "Aspects",
	"startupBodyOpen":        "Bodies",
	"startupHouseOpen":       "Houses",
	"startupZodiacOpen":      "Zodiacs",
	"startupNodesOpen":       "Nodes and Lilith",
	"startupExperimentsOpen": "Experiments",
}

// codeInventory is every chooser the code holds: the modal states named in
// startupShowsModal, the files that own a filtered list, and the preference
// keys the store's readers declare.
func codeInventory(t *testing.T) (modals, owners, prefs []string) {
	t.Helper()
	parse := func(path string) *ast.File {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		return f
	}
	// 1. the modals
	ast.Inspect(parse("model.go"), func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "startupShowsModal" {
			return true
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if cc, ok := n.(*ast.CaseClause); ok {
				for _, e := range cc.List {
					if id, ok := e.(*ast.Ident); ok {
						modals = append(modals, id.Name)
					}
				}
			}
			return true
		})
		return false
	})
	// 2. the owners of a filtered list: a file that names menu.FuzzyList or
	// menu.Filter, anywhere under internal/ui.
	filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		hit := false
		ast.Inspect(parse(p), func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "menu" && (sel.Sel.Name == "FuzzyList" || sel.Sel.Name == "Filter") {
					hit = true
				}
			}
			return !hit
		})
		if hit {
			owners = append(owners, "internal/ui/"+filepath.ToSlash(p))
		}
		return nil
	})
	// 3. the preference keys: in every store file that reads preferences.yaml,
	// the blocks it opens (findBlock, mergeBlock, mergeLine, a compare of the
	// top-level key) and the keys it reads inside one (a switch on the key).
	storeFiles, _ := filepath.Glob(filepath.Join("..", "store", "*.go"))
	seen := map[string]bool{}
	add := func(k string) {
		if k != "" && !seen[k] {
			seen[k] = true
			prefs = append(prefs, k)
		}
	}
	for _, p := range storeFiles {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f := parse(p)
		reads := false
		consts := map[string]string{}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				if x.Name == "PreferencesPath" {
					reads = true
				}
			case *ast.ValueSpec:
				for i, id := range x.Names {
					if i < len(x.Values) {
						if lit, ok := x.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
							s, _ := strconv.Unquote(lit.Value)
							consts[id.Name] = s
						}
					}
				}
			}
			return true
		})
		if !reads {
			continue
		}
		str := func(e ast.Expr) string {
			switch x := e.(type) {
			case *ast.BasicLit:
				if x.Kind == token.STRING {
					s, _ := strconv.Unquote(x.Value)
					return s
				}
			case *ast.Ident:
				return consts[x.Name]
			}
			return ""
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			readsKeys := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "splitKV" {
						readsKeys = true
					}
				}
				return true
			})
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					if id, ok := x.Fun.(*ast.Ident); ok {
						switch id.Name {
						case "findBlock", "mergeBlock", "mergeLine":
							if len(x.Args) >= 2 {
								add(str(x.Args[1]))
							}
						case "value":
							if len(x.Args) == 1 {
								add(str(x.Args[0]))
							}
						}
					}
				case *ast.BinaryExpr:
					if x.Op == token.EQL || x.Op == token.NEQ {
						if id, ok := x.X.(*ast.Ident); ok && (id.Name == "key" || id.Name == "k") {
							add(str(x.Y))
						}
					}
				case *ast.SwitchStmt:
					if id, ok := x.Tag.(*ast.Ident); ok && id.Name == "key" && readsKeys {
						for _, c := range x.Body.List {
							for _, e := range c.(*ast.CaseClause).List {
								add(str(e))
							}
						}
					}
				}
				return true
			})
		}
	}
	sort.Strings(modals)
	sort.Strings(owners)
	sort.Strings(prefs)
	return modals, owners, prefs
}

// sweptChooserNames is the headings the sweep writes.
func sweptChooserNames() map[string]bool {
	out := map[string]bool{}
	for _, c := range sweepChoosers() {
		out[c.name] = true
	}
	return out
}

func sweepChoosers() []sweepChooser {
	out := []sweepChooser{
		sweepAspects(), sweepBodies(), withCuspBase(sweepHouses()), sweepZodiacs(), sweepNodes(),
		sweepKinds(), sweepExperiments(),
	}
	for _, c := range sweepSettings() {
		if c.name == "Settings: frame houses" {
			c = withCuspBase(c)
		}
		out = append(out, c)
	}
	return out
}

// ownerChoosers maps a file that owns a filtered list to the chooser it feeds,
// where it feeds one.
var sweptOwners = map[string]string{
	"internal/ui/popup/aspects.go": "Aspects",
	"internal/ui/popup/body.go":    "Bodies",
	"internal/ui/popup/finder.go":  "Houses",
}

func TestChooserSweepKnowsEveryChooser(t *testing.T) {
	modals, owners, prefs := codeInventory(t)
	if len(modals) < 10 || len(owners) < 6 || len(prefs) < 10 {
		t.Fatalf("the code reading found %d modals, %d list owners and %d preference keys: too few, the reader is broken", len(modals), len(owners), len(prefs))
	}
	swept := sweptChooserNames()
	known := func(kind, name string, sweptBy map[string]string) {
		if c, ok := sweptBy[name]; ok {
			if !swept[c] {
				t.Errorf("%s %s is said to be swept as %q, which the sweep does not run", kind, name, c)
			}
			return
		}
		if _, ok := notAChoice[name]; ok {
			return
		}
		if _, ok := notAChoice["pref:"+name]; ok && kind == "preference key" {
			return
		}
		t.Errorf("%s %s is neither swept nor listed as not a choice: add it to the sweep (chooser_sweep_test.go), or to notAChoice with its reason", kind, name)
	}
	for _, m := range modals {
		known("modal", m, sweptModals)
	}
	for _, o := range owners {
		known("filtered-list owner", o, sweptOwners)
	}
	for _, p := range prefs {
		known("preference key", p, sweptPrefs)
	}
	// a stale row would hide a removed chooser
	for name := range notAChoice {
		switch {
		case strings.HasPrefix(name, "startup"):
			if !sweepHas(modals, name) {
				t.Errorf("notAChoice names %s, which startupShowsModal no longer lists", name)
			}
		case strings.HasPrefix(name, "internal/"):
			if !sweepHas(owners, name) {
				t.Errorf("notAChoice names %s, which no longer owns a filtered list", name)
			}
		case strings.HasPrefix(name, "pref:"):
			if !sweepHas(prefs, strings.TrimPrefix(name, "pref:")) {
				t.Errorf("notAChoice names %s, which the settings code no longer declares", name)
			}
		}
	}
	for name := range sweptModals {
		if !sweepHas(modals, name) {
			t.Errorf("sweptModals names %s, which startupShowsModal no longer lists", name)
		}
	}
}

func sweepHas(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// --- the table ---------------------------------------------------------------------------

func TestChooserSweep(t *testing.T) {
	out := os.Getenv("SEXTILE_SWEEP_OUT")
	if out == "" {
		t.Skip("SEXTILE_SWEEP_OUT names the table to write; the sweep is not run without it")
	}
	w := newSweepWorld(t)
	var b strings.Builder
	var timings []string
	fmt.Fprintf(&b, "# Chooser sweep\n\n")
	fmt.Fprintf(&b, "- Fixture chart: %s (internal/ui `timelineModel`, the floating chart selected).\n", sweepFixtureWho)
	fmt.Fprintf(&b, "- Astrolog binary: `%s`\n", w.binary)
	fmt.Fprintf(&b, "- Astrolog data folder: `%s`\n", astrologDataFolder(w.binary))
	fmt.Fprintf(&b, "- Atlas: `%s`\n", os.Getenv("SEXTILE_ATLAS_PATH"))
	fmt.Fprintf(&b, "- Rule: a row is *works* when the calls to Astrolog changed and the text of a panel the value should change changed; *does nothing* when either did not; *cannot be tried* when the sweep cannot reach the value. Each value is compared with the same chart held at another value.\n\n")
	var summary, sections []string
	for _, c := range sweepChoosers() {
		c := c
		t.Run(strings.ReplaceAll(c.name, " ", "_"), func(t *testing.T) {
			rows, took := w.runChooser(c)
			count := map[string]int{}
			for _, r := range rows {
				count[r.verdict]++
			}
			var sec strings.Builder
			fmt.Fprintf(&sec, "## %s\n\nValues from: %s. Panels compared: %s.\n\n", c.name, c.source, strings.Join(c.panels, ", "))
			fmt.Fprintf(&sec, "| value | argv change | what came back | verdict |\n|---|---|---|---|\n")
			for _, r := range rows {
				fmt.Fprintf(&sec, "| %s | %s | %s | %s |\n", sweepCut(r.value, 60), cutCell(r.argv), cutCell(r.came), r.verdict)
			}
			fmt.Fprintf(&sec, "\n")
			sections = append(sections, sec.String())
			summary = append(summary, fmt.Sprintf("| %s | %d | %d | %d | %d |", c.name, len(rows), count[verdictWorks], count[verdictNothing], count[verdictCannot]))
			timings = append(timings, fmt.Sprintf("%s: %s", c.name, took.Round(time.Second)))
		})
	}
	b.WriteString(strings.Join(sections, ""))
	fmt.Fprintf(&b, "## Counts\n\n| chooser | values | works | does nothing | cannot be tried |\n|---|---|---|---|---|\n%s\n\n", strings.Join(summary, "\n"))
	fmt.Fprintf(&b, "Run time by chooser: %s.\n", strings.Join(timings, "; "))
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("table written to %s", out)
}

func cutCell(s string) string { return strings.NewReplacer("|", "/", "\n", " ").Replace(s) }

// astrologDataFolder is where the binary reads its ephemeris from: the
// directory the binary's own astrolog.as sits in, as `astrolog -Hc` prints.
func astrologDataFolder(binary string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(binary)), "share", "astrolog")
}

var _ = store.ChartKindFile
