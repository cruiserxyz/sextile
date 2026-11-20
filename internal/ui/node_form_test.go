package ui

// The form of the Nodes and Lilith of a chart (specs/nodes-and-lilith.md):
// where it resolves, the chooser, the settings, the session. Chart values are
// constructed synthetic test inputs.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

var keyCtrlT = tea.KeyMsg{Type: tea.KeyCtrlT}

var nodeForms = []recipe.NodeForm{recipe.NodesMean, recipe.NodesTrue, recipe.NodesMeanNatural, recipe.NodesTrueNatural}

// nodeRec is a session record of a form for the anchored slot at index i.
func nodeRec(m Model, i int, f recipe.NodeForm) store.FrameRecord {
	s := slotAt(m, i)
	return store.FrameRecord{File: slotFile(s), When: s.Time, Nodes: f, HasNodes: true}
}

// nodeFileStates makes the chart file of slot i state the form f.
func nodeFileStates(m Model, i int, f recipe.NodeForm) Model {
	s := slotAt(m, i)
	m.frames = m.frames.withFile(slotFile(s), fileFrame{when: s.Time, nodes: f, nodesStated: true})
	return m
}

func formOf(m Model, i int) (recipe.NodeForm, nodeSource) { return m.ownNodes(slotAt(m, i)) }

// openNodes presses Ctrl+T on the main screen.
func openNodes(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = tpress(t, m, keyCtrlT)
	if m.startup != startupNodesOpen {
		t.Fatalf("Ctrl+T did not open the Nodes and Lilith chooser: startup = %q", m.startup)
	}
	return m
}

func nodeRowNames(m Model) []string {
	var out []string
	for _, f := range m.nodesFinder.Matches {
		out = append(out, nodeFormName(f))
	}
	return out
}

func nodeFormName(f recipe.NodeForm) string { return nodeFormWords(f) }

// N1: a chart's form is the session's record, else the form its file states,
// else the user's default, else mean and mean, and the lens carries it.
func TestNodeFormResolvesInFourSteps(t *testing.T) {
	m := finderModel(t, 2, 0)
	s := slotAt(m, 0)
	want := func(step string, f recipe.NodeForm, src nodeSource) {
		t.Helper()
		got, gotSrc := m.ownNodes(s)
		if got != f || gotSrc != src {
			t.Errorf("%s: form %v from %v, want %v from %v", step, got, gotSrc, f, src)
		}
		if l := m.lensFor(s).Objects.Nodes; l != f {
			t.Errorf("%s: the lens carries %v, want %v", step, l, f)
		}
	}
	want("nothing says", recipe.NodesMean, nodeFallback)
	m.frames.defN, m.frames.defNSet = recipe.NodesTrueNatural, true
	want("the user's default", recipe.NodesTrueNatural, nodeDefault)
	m = nodeFileStates(m, 0, recipe.NodesMeanNatural)
	want("the file states one", recipe.NodesMeanNatural, nodeFile)
	m.frames.records = append(m.frames.records, nodeRec(m, 0, recipe.NodesTrue))
	want("the session's record", recipe.NodesTrue, nodeChosen)

	// A record made for another chart (another moment in the same file) is no record.
	other := nodeRec(m, 0, recipe.NodesMean)
	other.When = other.When.Add(-time.Hour)
	m.frames.records = []store.FrameRecord{other}
	want("a record of another moment", recipe.NodesMeanNatural, nodeFile)

	// The floating chart has no file: its record, else the default.
	fl := m.slots.All()[len(m.slots.All())-1]
	if f, src := m.ownNodes(fl); f != recipe.NodesTrueNatural || src != nodeDefault {
		t.Errorf("Now: %v from %v, want the default", f, src)
	}
	m.frames.records = append(m.frames.records, store.FrameRecord{Floating: true, Nodes: recipe.NodesMeanNatural, HasNodes: true})
	if f, src := m.ownNodes(fl); f != recipe.NodesMeanNatural || src != nodeChosen {
		t.Errorf("Now with a record: %v from %v", f, src)
	}
	// A chart with no file keeps a choice for the run.
	m.frames.volatileN = map[string]recipe.NodeForm{volatileKey(fl): recipe.NodesTrue}
	if f, src := m.ownNodes(fl); f != recipe.NodesTrue || src != nodeSession {
		t.Errorf("a choice kept for the run, for a chart with no file: %v from %v, want true and true", f, src)
	}
}

// N8: Ctrl+T and `, t` open the chooser for the chart under the cursor, Now
// included, with the four pairs in order, the cursor and the mark on the pair
// that is cast, and nothing cast before Enter.
func TestNodeChooserOpensOnTheCastPair(t *testing.T) {
	for _, cursor := range []int{0, 2} { // an anchored chart, and Now
		for _, how := range []string{"ctrl+t", ", t"} {
			t.Run(fmt.Sprintf("cursor %d %s", cursor, how), func(t *testing.T) {
				m := finderModel(t, 2, cursor)
				if cursor == 0 {
					m.frames.records = append(m.frames.records, nodeRec(m, 0, recipe.NodesTrue))
				} else {
					m.frames.records = append(m.frames.records, store.FrameRecord{Floating: true, Nodes: recipe.NodesTrue, HasNodes: true})
				}
				before := fmt.Sprint(m.frames.records)
				var cmd tea.Cmd
				if how == "ctrl+t" {
					m, cmd = tpress(t, m, keyCtrlT)
				} else {
					m, cmd = press(t, m, ",", "t")
				}
				if m.startup != startupNodesOpen {
					t.Fatalf("%s left startup = %q", how, m.startup)
				}
				if got, want := strings.Join(nodeRowNames(m), "|"), "mean Node, mean Lilith|true Node, true Lilith|mean Node, natural Lilith|true Node, natural Lilith"; got != want {
					t.Errorf("rows = %s, want %s", got, want)
				}
				if got := m.nodesFinder.Matches[m.nodesFinder.Cursor]; got != recipe.NodesTrue {
					t.Errorf("the cursor opened on %v, want the cast true and true", got)
				}
				view := ansi.Strip(m.View())
				var marked []string
				for _, l := range strings.Split(view, "\n") {
					if strings.Contains(l, "●") && strings.Contains(l, "Node") {
						marked = append(marked, l)
					}
				}
				if len(marked) != 1 || !strings.Contains(marked[0], "True Node, true Lilith") {
					t.Errorf("the mark stands on %q, want the one row True Node, true Lilith:\n%s", marked, view)
				}
				if after := fmt.Sprint(m.frames.records); after != before || cmd != nil {
					t.Errorf("opening cast or wrote something: records %s -> %s, cmd %v", before, after, cmd != nil)
				}
				if sessionExists() {
					t.Error("opening wrote the session")
				}
			})
		}
	}
}

// pressDownTo moves the chooser's cursor to the pair f.
func pressDownTo(t *testing.T, m Model, f recipe.NodeForm) Model {
	t.Helper()
	for i := 0; i < 6 && m.nodesFinder.Matches[m.nodesFinder.Cursor] != f; i++ {
		if m.nodesFinder.Matches[m.nodesFinder.Cursor] > f {
			m, _ = tpress(t, m, keyUp)
		} else {
			m, _ = tpress(t, m, keyDown)
		}
	}
	if got := m.nodesFinder.Matches[m.nodesFinder.Cursor]; got != f {
		t.Fatalf("could not reach %v, the cursor is on %v", f, got)
	}
	return m
}

// N9: Enter on the cast pair closes and writes nothing, also when that pair is
// the chart's home form; Enter on the home form, when another is cast, removes
// the session's choice; Enter on any other pair records it at once and recasts.
func TestNodeChooserEnterOnTheCastPair(t *testing.T) {
	m := finderModel(t, 2, 0)
	m.frames.records = append(m.frames.records, nodeRec(m, 0, recipe.NodesTrue))
	before := fmt.Sprint(m.frames.records)
	m = openNodes(t, m)
	m, cmd := tpress(t, m, keyEnter)
	if m.startup != startupReady {
		t.Errorf("Enter left startup = %q", m.startup)
	}
	if fmt.Sprint(m.frames.records) != before || cmd != nil || sessionExists() {
		t.Errorf("Enter on the cast pair wrote something: records %v, cmd %v, session %v", m.frames.records, cmd != nil, sessionExists())
	}
}

func TestNodeChooserEnterOnTheCastPairThatIsAlsoHome(t *testing.T) {
	m := finderModel(t, 2, 0)
	m = nodeFileStates(m, 0, recipe.NodesMeanNatural)
	m = openNodes(t, m)
	if got := m.nodesFinder.Matches[m.nodesFinder.Cursor]; got != recipe.NodesMeanNatural {
		t.Fatalf("the cursor opened on %v, want the file's pair", got)
	}
	m, cmd := tpress(t, m, keyEnter)
	if len(m.frames.records) != 0 || cmd != nil || sessionExists() {
		t.Errorf("Enter on the cast home pair wrote something: records %v, cmd %v, session %v", m.frames.records, cmd != nil, sessionExists())
	}
}

func TestNodeChooserEnterOnTheHomePair(t *testing.T) {
	m := finderModel(t, 2, 0)
	m = nodeFileStates(m, 0, recipe.NodesMeanNatural)
	// A record that also holds houses keeps them when the form goes.
	rec := nodeRec(m, 0, recipe.NodesTrue)
	rec.Houses, rec.HasHouses, rec.From = recipe.Koch, true, store.FrameFromChosen
	m.frames.records = append(m.frames.records, rec)
	m = openNodes(t, m)
	m = pressDownTo(t, m, recipe.NodesMeanNatural)
	m, cmd := tpress(t, m, keyEnter)
	if cmd == nil {
		t.Error("removing the choice recast nothing")
	}
	got, src := formOf(m, 0)
	if got != recipe.NodesMeanNatural || src != nodeFile {
		t.Errorf("after Enter on the home pair the chart shows %v from %v, want the file's", got, src)
	}
	r, ok := m.frames.find(slotFile(slotAt(m, 0)), false)
	if !ok || r.HasNodes || !r.HasHouses || r.Houses != recipe.Koch {
		t.Errorf("the record = %+v, found %v, want the houses kept and no form", r, ok)
	}
	// With nothing else in the record, it goes.
	m2 := finderModel(t, 2, 0)
	m2.frames.records = append(m2.frames.records, nodeRec(m2, 0, recipe.NodesTrue))
	m2 = openNodes(t, m2)
	m2 = pressDownTo(t, m2, recipe.NodesMean)
	m2, _ = tpress(t, m2, keyEnter)
	if len(m2.frames.records) != 0 {
		t.Errorf("an emptied record stayed: %v", m2.frames.records)
	}
}

func TestNodeChooserEnterOnAnotherPair(t *testing.T) {
	m := finderModel(t, 2, 0)
	m = openNodes(t, m)
	m = pressDownTo(t, m, recipe.NodesTrueNatural)
	m, cmd := tpress(t, m, keyEnter)
	if cmd == nil {
		t.Error("the choice recast nothing")
	}
	if got, src := formOf(m, 0); got != recipe.NodesTrueNatural || src != nodeChosen {
		t.Errorf("the chart shows %v from %v, want true and natural, chosen", got, src)
	}
	b, err := os.ReadFile(store.SessionPath())
	if err != nil {
		t.Fatalf("the choice was not saved at once: %v", err)
	}
	if !strings.Contains(string(b), "    node: true\n    lilith: natural\n") {
		t.Errorf("the session holds no node and lilith in words:\n%s", b)
	}
}

// N10: the right column says `from its file` on the pair the file states, and
// `your default` on the default pair when it states nothing; nothing else.
func TestNodeChooserNamesTheHomeRow(t *testing.T) {
	rowsWith := func(m Model, sub string) []string {
		var out []string
		for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			if strings.Contains(l, sub) {
				out = append(out, l)
			}
		}
		return out
	}
	t.Run("the file states a pair", func(t *testing.T) {
		m := finderModel(t, 2, 0)
		m.frames.defN, m.frames.defNSet = recipe.NodesMeanNatural, true
		m = nodeFileStates(m, 0, recipe.NodesTrue)
		m = openNodes(t, m)
		if got := rowsWith(m, "from its file"); len(got) != 1 || !strings.Contains(got[0], "True Node, true Lilith") {
			t.Errorf("rows with `from its file`: %q", got)
		}
		if got := rowsWith(m, "your default"); len(got) != 0 {
			t.Errorf("a file that states a pair also marked the default: %q", got)
		}
	})
	t.Run("no file wish, a default", func(t *testing.T) {
		m := finderModel(t, 2, 0)
		m.frames.defN, m.frames.defNSet = recipe.NodesMeanNatural, true
		m = openNodes(t, m)
		if got := rowsWith(m, "your default"); len(got) != 1 || !strings.Contains(got[0], "Mean Node, natural Lilith") {
			t.Errorf("rows with `your default`: %q", got)
		}
		if got := rowsWith(m, "from its file"); len(got) != 0 {
			t.Errorf("rows with `from its file`: %q", got)
		}
	})
	t.Run("no file wish, no default", func(t *testing.T) {
		m := openNodes(t, finderModel(t, 2, 0))
		if got := rowsWith(m, "your default"); len(got) != 1 || !strings.Contains(got[0], "Mean Node, mean Lilith") {
			t.Errorf("rows with `your default`: %q", got)
		}
	})
}

// N11 (ui half): the choice is in the session file and a restart reads it back.
func TestNodeChoiceSurvivesARestart(t *testing.T) {
	for _, f := range nodeForms[1:] {
		t.Run(nodeFormName(f), func(t *testing.T) {
			m := finderModel(t, 2, 0)
			m, _ = m.chooseNodes(0, f)
			p := realPerimeter(t)
			if !p.HasSession {
				t.Fatal("no session was written")
			}
			fresh := finderModel(t, 2, 0)
			fresh = fresh.loadFrames(p)
			if got, src := formOf(fresh, 0); got != f || src != nodeChosen {
				t.Errorf("after a restart the chart shows %v from %v, want %v chosen", got, src, f)
			}
		})
	}
}

// N13: a session written by 0.1.3 holds no form: each chart opens on its home
// form and the Log says nothing.
func TestOldSessionTakesTheHomeNodeForm(t *testing.T) {
	m := finderModel(t, 2, 0)
	s := slotAt(m, 0)
	fresh := finderModel(t, 2, 0)
	fresh = nodeFileStates(fresh, 0, recipe.NodesTrue)
	seedSession(t, hereText("version: 3\nfloating:\n  selected: false\nframes:\n"+
		"  - file: "+slotFile(s)+"\n    when: \""+s.Time.UTC().Format(time.RFC3339Nano)+"\"\n    houses: Koch\n    from: chosen\n    zodiac: sidereal Lahiri\n"))
	fresh = fresh.loadFrames(realPerimeter(t))
	r, ok := fresh.frames.find(slotFile(s), false)
	if !ok || r.HasNodes || !r.HasHouses {
		t.Fatalf("the old record = %+v, found %v, want houses and no form", r, ok)
	}
	if got, src := formOf(fresh, 0); got != recipe.NodesTrue || src != nodeFile {
		t.Errorf("the chart shows %v from %v, want its file's", got, src)
	}
	if got, src := formOf(finderModel(t, 2, 1), 1); got != recipe.NodesMean || src != nodeFallback {
		t.Errorf("a chart with nothing shows %v from %v", got, src)
	}
	for _, l := range fresh.logLines {
		t.Errorf("a 0.1.3 session said %q", l.text)
	}
}

// N12: the default form is the user's: Save defaults keeps the floating chart's
// form, the same two lines written by hand are read at the next start, and a
// pair that cannot be read gives one Log line.
func TestDefaultNodeFormIsSavedAndRead(t *testing.T) {
	for _, f := range nodeForms {
		t.Run("save and restart "+nodeFormName(f), func(t *testing.T) {
			sessionSandbox(t)
			m := framesFixture(t, 2, 0)
			m.place = amsterdamNL
			m.frames = m.frames.with(store.FrameRecord{Floating: true, Nodes: f, HasNodes: true})
			m, cmd := press(t, m, ",", "c", "d")
			if cmd == nil {
				t.Fatal("`, c d` scheduled no save")
			}
			m = deliver(m, cmd)
			node, lilith := f.Words()
			b, err := os.ReadFile(store.PreferencesPath())
			if err != nil {
				t.Fatal(err)
			}
			if want := "  node: " + node + "\n  lilith: " + lilith + "\n"; !strings.Contains(string(b), want) {
				t.Errorf("preferences.yaml lacks %q:\n%s", want, b)
			}
			if n := noteCount(m, "defaults: ") - noteCount(m, "defaults: houses shown as"); n != 1 {
				t.Errorf("the Log names the saved frame %d times: %v", n, m.logLines)
			}
			if n := noteCount(m, nodeFormWords(f)); n != 1 {
				t.Errorf("the saved-frame line does not name the form %q: %v", nodeFormWords(f), m.logLines)
			}
			fresh := framesFixture(t, 2, 0).applyFrameConfig(realPerimeter(t).Frame)
			if got, _ := fresh.ownNodes(slotAt(fresh, 0)); got != f {
				t.Errorf("after a restart a chart with no choice shows %v, want %v", got, f)
			}
			if n := noteCount(fresh, "settings:"); n != 0 {
				t.Errorf("a good default logged at start: %v", fresh.logLines)
			}
		})
	}
	for _, f := range nodeForms {
		t.Run("by hand "+nodeFormName(f), func(t *testing.T) {
			node, lilith := f.Words()
			fresh := framesFixture(t, 2, 0).applyFrameConfig(store.FrameConfig{Node: node, NodePresent: true, Lilith: lilith, LilithPresent: true})
			if got, src := formOf(fresh, 0); got != f || (f != recipe.NodesMean && src != nodeDefault) {
				t.Errorf("written by hand %s/%s: %v from %v", node, lilith, got, src)
			}
			if len(fresh.logLines) != 0 {
				t.Errorf("a good default logged: %v", fresh.logLines)
			}
		})
	}
	for _, tc := range []struct {
		name string
		cfg  store.FrameConfig
	}{
		{"node alone", store.FrameConfig{Node: "true", NodePresent: true}},
		{"lilith alone", store.FrameConfig{Lilith: "natural", LilithPresent: true}},
		{"an unknown word", store.FrameConfig{Node: "sorta", NodePresent: true, Lilith: "mean", LilithPresent: true}},
		{"the pair Astrolog cannot cast", store.FrameConfig{Node: "true", NodePresent: true, Lilith: "mean", LilithPresent: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fresh := framesFixture(t, 2, 0).applyFrameConfig(tc.cfg)
			if got, src := formOf(fresh, 0); got != recipe.NodesMean || src != nodeFallback {
				t.Errorf("the default is %v from %v, want mean and mean", got, src)
			}
			const want = "settings: the default Node and Lilith do not go together, Mean is used"
			if n := logHas(fresh, want); n != 1 || len(fresh.logLines) != 1 {
				t.Errorf("the Log says %d times %q among %v, want that line once and nothing else", n, want, fresh.logLines)
			}
			if w := ansi.StringWidth(want); w > 76 {
				t.Errorf("the line is %d cells, want at most 76", w)
			}
		})
	}
}

// N17: no action of this piece changes a byte in the charts folder, and a
// chart saved from Now carries no Node line.
func TestNodeChoiceWritesNoChartFile(t *testing.T) {
	m := finderModel(t, 2, 0)
	dir := store.ChartsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const original = "@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n=Yn\n"
	file := dir + "/" + slotFile(slotAt(m, 0))
	if err := os.WriteFile(file, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		var sb strings.Builder
		ents, _ := os.ReadDir(dir)
		for _, e := range ents {
			b, _ := os.ReadFile(dir + "/" + e.Name())
			fmt.Fprintf(&sb, "%s\x00%s\x00", e.Name(), b)
		}
		return sb.String()
	}
	before := snapshot()
	for _, f := range nodeForms {
		m, _ = m.chooseNodes(0, f)
	}
	m, _ = m.forgetNodesChoice(0)
	m, _ = m.chooseNodes(2, recipe.NodesTrueNatural) // Now
	if after := snapshot(); after != before {
		t.Errorf("the charts folder changed:\n%q\n%q", before, after)
	}
}

// A chart saved from Now carries no Node line, whatever its form.
func TestNodeChoiceWritesNoChartFileFromNow(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	for i, f := range nodeForms {
		t.Run(nodeFormName(f), func(t *testing.T) {
			m := saveBase(t)
			floating := len(m.slots.All()) - 1
			m, _ = m.chooseNodes(floating, f)
			if got, _ := m.ownNodes(m.slots.All()[floating]); got != f {
				t.Fatalf("setup: Now shows %v, want %v", got, f)
			}
			name := fmt.Sprintf("Form Test %d", i)
			m = runSave(t, m, name)
			b, err := os.ReadFile(filepath.Join(chartsDirOf(t), fmt.Sprintf("form-test-%d.as", i)))
			if err != nil {
				t.Fatalf("the chart was not written: %v", err)
			}
			if strings.Contains(string(b), "Yn") {
				t.Errorf("the saved chart carries a Node switch:\n%s", b)
			}
		})
	}
}

// nodeSpy records, for every stubbed lookup, the chart it was for and the form,
// house system and zodiac its lens carried.
type nodeSpy struct {
	mu    sync.Mutex
	calls []nodeCall
}

type nodeCall struct {
	label  string
	nodes  recipe.NodeForm
	houses recipe.HouseSystem
	zodiac recipe.Zodiac
}

func (s *nodeSpy) record(label string, l lookup.Lens) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, nodeCall{label, l.Objects.Nodes, l.Houses, l.Zodiac})
}

func (s *nodeSpy) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = nil
}

func (s *nodeSpy) snapshot() []nodeCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]nodeCall(nil), s.calls...)
}

func stubNodeLookups(t *testing.T) *nodeSpy {
	t.Helper()
	spy := &nodeSpy{}
	stubSpawns(t, func(s *rowSpawns) {
		s.cast.natal = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, lens lookup.Lens) (lookup.Result, error) {
			label := natalLabel(c)
			spy.record(label, lens)
			return lookup.Result{
				Positions: []aspect.Position{{Object: objSun, Longitude: float64(c.When.Year() % 360)}},
				Cusps:     aspect.Cusps{100 + float64(lens.Houses)},
				Nodes:     lens.Objects.Nodes,
				Commands:  []string{fmt.Sprintf("astrolog natal %s", label)},
			}, nil
		}
		s.cast.transit = func(_ context.Context, _ *lookup.Probers, when time.Time, _ location.PlaceInfo, lens lookup.Lens) (lookup.Result, error) {
			spy.record("now", lens)
			return lookup.Result{
				Positions: []aspect.Position{{Object: objSun, Longitude: float64(when.Year()%360) + 0.5}},
				Cusps:     aspect.Cusps{200 + float64(lens.Houses)},
				Nodes:     lens.Objects.Nodes,
				Commands:  []string{"astrolog transit"},
			}, nil
		}
		s.cast.overlay = func(_ context.Context, lens lookup.Lens, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo) (lookup.OverlayAspects, error) {
			spy.record("cross", lens)
			return lookup.OverlayAspects{Report: "transit cross"}, nil
		}
		s.cast.synastry = func(_ context.Context, lens lookup.Lens, _ store.AnchorChart, _ store.AnchorChart) (lookup.OverlayAspects, error) {
			spy.record("synastry", lens)
			return lookup.OverlayAspects{Report: "synastry cross"}, nil
		}
	})
	return spy
}

// N19: Keybindings lists the command as "Nodes and Lilith", found by typing
// `node` or `lilith`; on the comma key bar its entry is the first one cut when
// the bar does not fit.
func TestNodeCommandIsListedAndCutFirst(t *testing.T) {
	m := finderModel(t, 2, 0)
	var listed bool
	for _, l := range keybindingLines(m) {
		if strings.HasPrefix(l.Text, "Nodes and Lilith") && strings.Contains(l.Text, "<ctrl+t>") {
			listed = true
		}
	}
	if !listed {
		t.Errorf("Keybindings does not list Nodes and Lilith with Ctrl+T: %v", keybindingLines(m))
	}
	for _, word := range []string{"node", "lilith"} {
		m2, _ := openKeybindings(m, false)
		st := m2.(Model).keybindings
		st.FilterText = word
		st.Matches = keys.FilterRows(st.All, st.FilterText)
		found := false
		for _, r := range st.Matches {
			found = found || r.Desc == "Nodes and Lilith"
		}
		if !found {
			t.Errorf("typing %q finds %d rows, none of them Nodes and Lilith", word, len(st.Matches))
		}
	}

	// The comma bar: `t nodes` leads no other entry out of the bar. Wherever any
	// other unpinned entry is cut, it is cut too, and a width exists where it
	// alone is.
	m, _ = press(t, m, ",")
	line := func(w int) string { return strings.TrimSpace(ansi.Strip(keys.MenuLine(m.keysContext(), w))) }
	if !strings.Contains(line(200), "t nodes") {
		t.Fatalf("a wide bar lacks `t nodes`: %q", line(200))
	}
	var alone bool
	for w := 200; w >= 30; w-- {
		l := line(w)
		hasNodes := strings.Contains(l, "t nodes")
		for _, other := range []string{"o open", "s save", "l city", "u houses", "z zodiac", "a aspects", "b bodies", "x wheel", "y copy", "O sessions", "L log"} {
			if hasNodes && !strings.Contains(l, other) {
				t.Errorf("width %d keeps `t nodes` and cuts `%s`: %q", w, other, l)
			}
			if !hasNodes && strings.Contains(l, "L log") {
				alone = true
			}
		}
		if !strings.HasSuffix(l, "<esc>") {
			t.Errorf("width %d cut the pinned `<esc>`: %q", w, l)
		}
	}
	if !alone {
		t.Error("no width cuts `t nodes` while keeping every other entry: it is not the first cut")
	}
}

// N18: the lens field TrueNodeMode and the Log lines "Sextile shows the true
// Node" are gone, no source calls Ynn a natural Node, and no screen or lookup
// code fixes a form of its own: a form comes from the chart's resolution.
func TestNoFixedNodeFormIsLeft(t *testing.T) {
	banned := []struct {
		re   *regexp.Regexp
		what string
	}{
		{regexp.MustCompile(`(?i)\btrueNodeMode\b`), "the lens field TrueNodeMode"},
		{regexp.MustCompile(`Sextile shows the true Node`), "the Log line about the true Node"},
		{regexp.MustCompile(`(?i)natural nodes?\b`), "a natural Node (Ynn moves Lilith only)"},
		{regexp.MustCompile(`SwitchNodeMode`), "the retired file-switch class"},
	}
	fixed := regexp.MustCompile(`\bNodes\s*(:|=)\s*recipe\.Nodes\w+`)
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		inUI := strings.HasPrefix(filepath.ToSlash(path), "../ui/") || strings.HasPrefix(filepath.ToSlash(path), "../lookup/")
		for i, line := range strings.Split(string(b), "\n") {
			for _, c := range banned {
				if c.re.MatchString(line) {
					t.Errorf("%s:%d: %s: %s", path, i+1, c.what, strings.TrimSpace(line))
				}
			}
			if inUI && fixed.MatchString(line) && !strings.HasPrefix(strings.TrimSpace(line), "//") {
				t.Errorf("%s:%d: a form fixed in code that should resolve it: %s", path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// nodeComparisonModel is three charts and Now selected, each on its own form,
// its own house system and its own zodiac, the cursor on slot cursor and the
// base on slot base (-1: the cursor's own tab).
//
//	① mean and mean, Koch, tropical        ② true and true, Placidus, Lahiri
//	③ mean and natural, Equal, Raman       Now true and natural, Campanus, tropical
func nodeComparisonModel(t *testing.T, cursor, base int) (Model, *nodeSpy) {
	t.Helper()
	freshModels(t)
	sessionSandbox(t)
	spy := stubNodeLookups(t)
	m := selectedSet(t, 3, true, cursor)
	m.cfg = Config{Accessible: true, Color: ColorNever}
	forms := []recipe.NodeForm{recipe.NodesMean, recipe.NodesTrue, recipe.NodesMeanNatural}
	houses := []recipe.HouseSystem{recipe.Koch, recipe.Placidus, recipe.Equal}
	zodiacs := []recipe.Zodiac{recipe.Tropical, recipe.Lahiri, recipe.Raman}
	for i := 0; i < 3; i++ {
		rec := nodeRec(m, i, forms[i])
		rec.Houses, rec.HasHouses, rec.From = houses[i], true, store.FrameFromChosen
		rec.Zodiac, rec.HasZodiac = zodiacs[i].RecordName(), true
		m.frames.records = append(m.frames.records, rec)
	}
	m.frames.records = append(m.frames.records, store.FrameRecord{
		Floating: true, Nodes: recipe.NodesTrueNatural, HasNodes: true,
		Houses: recipe.Campanus, HasHouses: true, From: store.FrameFromChosen,
	})
	m.base = nil
	if base >= 0 {
		m.base = slotAt(m, base)
	}
	return m, spy
}

// labelOf names the slot as the spy records its casts.
func labelOf(s *store.Slot) string {
	if s.Floating {
		return "now"
	}
	return natalLabel(anchorOf(s))
}

// N7: a comparison is cast in the base's form and only the form comes from the
// base: every call carries it, and every column keeps its chart's own houses
// and zodiac.
func TestNodeFormInComparisons(t *testing.T) {
	for _, tc := range []struct {
		name         string
		cursor, base int
		want         recipe.NodeForm
	}{
		{"base under the cursor", 0, -1, recipe.NodesMean},
		{"the other chart under the cursor", 1, 0, recipe.NodesMean},
		{"a third chart under the cursor, another base", 2, 1, recipe.NodesTrue},
		{"a pair with Now", 3, 2, recipe.NodesMeanNatural},
		{"Now's own tab", 3, -1, recipe.NodesTrueNatural},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, spy := nodeComparisonModel(t, tc.cursor, tc.base)
			if sc := m.deriveScene(); sc.mode != sceneComparing || (tc.base >= 0) != (sc.pair != nil) {
				t.Fatalf("setup: scene mode %v, pair %v", sc.mode, sc.pair)
			}
			spy.reset()
			m = settleHouses(t, m)
			calls := spy.snapshot()
			if len(calls) == 0 {
				t.Fatal("no call was made")
			}
			seen := map[string]bool{}
			for _, c := range calls {
				seen[c.label] = true
				if c.nodes != tc.want {
					t.Errorf("a call for %s carried %v, want the base's %v: %+v", c.label, c.nodes, tc.want, calls)
				}
			}
			sc := m.deriveScene()
			for _, s := range sc.shown {
				if !seen[labelOf(s)] {
					t.Errorf("no call was made for the column of %s: %+v", labelOf(s), calls)
				}
				// Each column keeps its chart's own houses; its zodiac is its own on
				// the own tab and the frame's while a pair is on screen.
				wantZ := m.ownZodiac(s)
				if sc.pair != nil {
					wantZ = m.ownZodiac(sc.frame)
				}
				for _, c := range calls {
					if c.label != labelOf(s) || c.label == "cross" {
						continue
					}
					if c.houses == m.ownHouses(s).hs && c.zodiac == wantZ {
						goto ok
					}
				}
				t.Errorf("no call for %s carried its own houses %v and zodiac %v: %+v", labelOf(s), m.ownHouses(s).hs, wantZ, calls)
			ok:
			}
		})
	}

	// The Timeline and the Moon tab of a chart use the chart's form, whoever the
	// base is; the sky list, with the cursor on Now, uses Now's.
	for _, tc := range []struct {
		name         string
		cursor, base int
		want         recipe.NodeForm
	}{
		{"timeline of ②, base ①", 1, 0, recipe.NodesTrue},
		{"timeline of ③ on its own tab", 2, -1, recipe.NodesMeanNatural},
		{"the sky list on Now", 3, -1, recipe.NodesTrueNatural},
	} {
		for _, list := range []timelineList{listTimeline, listMoon} {
			t.Run(fmt.Sprintf("%s list %v", tc.name, list), func(t *testing.T) {
				m, _ := nodeComparisonModel(t, tc.cursor, tc.base)
				req, key, ok := m.timelineInputsFor(list)
				if !ok {
					t.Fatal("no timeline request")
				}
				if req.Lens.Objects.Nodes != tc.want || key.nodes != tc.want {
					t.Errorf("the request carries %v (key %v), want %v", req.Lens.Objects.Nodes, key.nodes, tc.want)
				}
				for _, n := range req.Natals {
					if n.Objects.Nodes != tc.want {
						t.Errorf("a natal of the search carries %v, want %v", n.Objects.Nodes, tc.want)
					}
				}
			})
		}
	}
}

// A new default form does not move the charts that were open when it was saved:
// each keeps the form it showed over a restart, and a chart opened later takes
// the default (the same freeze as for houses and zodiac).
func TestSaveDefaultsFreezesTheNodeFormOfTheOpenCharts(t *testing.T) {
	_ = savedFrameModel(t, 2, func(m Model) Model {
		m.frames.records[len(m.frames.records)-1].Nodes = recipe.NodesTrueNatural
		m.frames.records[len(m.frames.records)-1].HasNodes = true
		return m
	})
	r := restarted(t)
	for i := 0; i < 2; i++ {
		if got, src := formOf(r, i); got != recipe.NodesMean || src != nodeChosen {
			t.Errorf("chart %d that was open shows %v from %v after the restart, want mean and mean, kept", i, got, src)
		}
	}
	openChart(t, r, 3)
	if got, src := formOf(r, 2); got != recipe.NodesTrueNatural || src != nodeDefault {
		t.Errorf("a chart opened after the save shows %v from %v, want the saved default", got, src)
	}
}
