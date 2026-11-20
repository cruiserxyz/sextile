package ui

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The Moon tab is a second instance of the Timeline's list (spec: the Moon
// observatory, slice M). These tests pin what the instances do not share: when
// the tab is declared, that one implementation serves both, that the key names
// the list, that each list searches only while shown, and how a failure is
// named.

func registryWithoutMoon() aspect.Registry {
	return slices.DeleteFunc(slices.Clone(lookup.Hellenistic.Registry), func(o aspect.RegistryObject) bool { return o.Name == "Moon" })
}

// M1: the Moon tab follows the Timeline tab in every state the Timeline tab is
// declared, and is declared nowhere else.
func TestMoonTabFollowsTheTimelineTab(t *testing.T) {
	for _, c := range timelineTabRuleCases {
		t.Run(c.name, func(t *testing.T) {
			ids := tabIDs(ruleModel(t, c).panelTabs(PanelDetails))
			ti, mi := slices.Index(ids, TabDetailsTimeline), slices.Index(ids, TabDetailsMoon)
			if c.tab && (ti < 0 || mi != ti+1) {
				t.Errorf("tabs %v, want the Moon tab directly after the Timeline tab", ids)
			}
			if !c.tab && (ti >= 0 || mi >= 0) {
				t.Errorf("tabs %v, want neither list tab", ids)
			}
		})
	}
	// Whatever else changes, the Chart panel's Details is the only one with lists.
	for _, paired := range []bool{false, true} {
		for _, focus := range focusablePanels() {
			for _, attached := range []bool{false, true} {
				m := timelineModel(t, Config{Accessible: true})
				m.slots.SetComparisonSelected(paired)
				m.focused, m.details.attached = focus, attached
				want := []TabID{TabDetailsReport}
				if focus == PanelStatus {
					want = append(want, TabStatusSession)
				}
				if focus == PanelChart {
					want = append(want, TabDetailsTimeline, TabDetailsMoon)
				}
				if got := tabIDs(m.panelTabs(PanelDetails)); !slices.Equal(got, want) {
					t.Errorf("paired=%v focus=%s attached=%v: tabs %v, want %v", paired, focus, attached, got, want)
				}
			}
		}
	}
}

// A registry without a Moon declares the Timeline tab and no Moon tab.
func TestNoMoonInRegistry(t *testing.T) {
	m := timelineModel(t, Config{Accessible: true})
	m.lens.Registry = registryWithoutMoon()
	if got := tabIDs(m.panelTabs(PanelDetails)); !slices.Equal(got, []TabID{TabDetailsReport, TabDetailsTimeline}) {
		t.Errorf("tabs %v, want the report and the Timeline", got)
	}
	// A Moon choice remembered from another registry falls back to the report.
	m = m.withDetailsTab(PanelChart, TabDetailsMoon)
	if got := m.activeTabID(PanelDetails); got != TabDetailsReport {
		t.Errorf("a remembered Moon tab shows %s without a Moon, want the report", got)
	}
}

// A lens that hides the Moon leaves the Moon tab declared, and its list is the
// empty month the Timeline shows for a month with no events.
func TestMoonTabWithMoonHidden(t *testing.T) {
	moon, ok := lookup.Hellenistic.Registry.ByName("Moon")
	if !ok {
		t.Fatal("fixture: no Moon in the registry")
	}
	empty := lookup.TimelineResult{Commands: []string{"astrolog -z -2 -qd 10 17 2026 -d"}}
	body := func(t *testing.T, k timelineList, hidden bool, answer func(lookup.TimelineRequest) (lookup.TimelineResult, error)) []string {
		t.Helper()
		freshModels(t)
		calls := stubTimeline(t, answer)
		m := timelineModel(t, Config{Accessible: true})
		m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
		if hidden {
			m.lens.Objects.Remove = recipe.NewObjectSet(moon.Number)
		}
		if !slices.Contains(tabIDs(m.panelTabs(PanelDetails)), k.tab()) {
			t.Fatalf("tabs %v lack %s", tabIDs(m.panelTabs(PanelDetails)), k.tab())
		}
		m, cmd := syncListVisited(m, k)
		m = pump(t, m, cmd)
		m = m.withDetailsTab(PanelChart, k.tab())
		if len(*calls) != 1 || (*calls)[0].req.Moon != k.moon() || (*calls)[0].req.Lens.Objects.Remove.Has(moon.Number) != hidden {
			t.Fatalf("searches %+v, want one %s search asked under the lens as it stood", *calls, k.scope())
		}
		if l := m.list(k); !l.complete || len(l.rows) != 0 {
			t.Fatalf("%s list: complete=%v rows=%d, want an empty landed list", k.scope(), l.complete, len(l.rows))
		}
		return detailsBodyRows(renderDetails(m))
	}
	week := weekRows(t)
	// Astrolog prints no event for a Moon list whose Moon the lens hides; the
	// Timeline under that lens still lists the other bodies.
	moonHidden := body(t, listMoon, true, func(req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		if req.Moon && req.Lens.Objects.Remove.Has(moon.Number) {
			return empty, nil
		}
		return lookup.TimelineResult{Rows: week}, nil
	})
	emptyMonth := body(t, listTimeline, false, func(lookup.TimelineRequest) (lookup.TimelineResult, error) { return empty, nil })
	if !slices.Equal(moonHidden, emptyMonth) {
		t.Errorf("the Moon tab with the Moon hidden draws\n%q\nwant the Timeline's empty month\n%q", moonHidden, emptyMonth)
	}
}

// M6a: the two lists' keys differ in the list and in nothing else, so a change
// that re-asks the Timeline re-asks the Moon list and nothing else does.
func TestMoonKeyIsTheTimelineKeyPlusTheList(t *testing.T) {
	berlin := testPlaceBerlin()
	keys := func(t *testing.T, m Model) (timelineKey, timelineKey) {
		t.Helper()
		tk, okT := m.timelineKeyForList(listTimeline)
		mk, okM := m.timelineKeyForList(listMoon)
		if !okT || !okM {
			t.Fatalf("no key: timeline %v moon %v", okT, okM)
		}
		if tk.list != listTimeline || mk.list != listMoon {
			t.Fatalf("keys name lists %v and %v", tk.list, mk.list)
		}
		return tk, mk
	}
	for _, c := range []struct {
		name   string
		change func(*Model)
	}{
		{"the month", func(m *Model) {
			slots, _ := store.NewSlots(tlFloatingTime.AddDate(0, 1, 0), testPlaceAmsterdam())
			_ = addAnchor(slots, tlNatalWhen, testPlaceAmsterdam())
			m.slots = slots
		}},
		{"the floating zone", func(m *Model) {
			slots, _ := store.NewSlots(tlFloatingTime, berlin)
			_ = addAnchor(slots, tlNatalWhen, testPlaceAmsterdam())
			m.slots = slots
		}},
		{"the anchored chart", func(m *Model) {
			setAnchor(*m, store.AnchorChart{When: time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC), Place: berlin})
		}},
		{"the pair is deselected", func(m *Model) { m.slots.SetComparisonSelected(false) }},
		{"the node form of the chart under the cursor", func(m *Model) {
			m.frames.volatileN = map[string]recipe.NodeForm{volatileKey(m.focusedSlot()): recipe.NodesTrue}
		}},
		{"the aspects", func(m *Model) { m.lens.Aspects = recipe.AspectPolicy{Explicit: true} }},
		{"the bodies", func(m *Model) { m.lens.Objects.Add = recipe.NewObjectSet(11) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := timelineModel(t, Config{Accessible: true})
			tk0, mk0 := keys(t, m)
			if copyOf := tk0; func() bool { copyOf.list = listMoon; return copyOf != mk0 }() {
				t.Fatalf("the keys differ beyond the list:\n%+v\n%+v", tk0, mk0)
			}
			c.change(&m)
			tk1, mk1 := keys(t, m)
			if tk1 == tk0 {
				t.Fatalf("fixture: changing %s did not change the Timeline's key", c.name)
			}
			if mk1 == mk0 {
				t.Errorf("changing %s changed the Timeline's key and not the Moon list's", c.name)
			}
			tk1.list = listMoon
			if tk1 != mk1 {
				t.Errorf("after changing %s the keys differ beyond the list:\n%+v\n%+v", c.name, tk1, mk1)
			}
		})
	}
}

// M7: each list's surface searches only while its tab is active, and a switch
// to a list that already landed for the same inputs starts nothing.
func TestMoonAndTimelineSearchOnlyWhenShown(t *testing.T) {
	calls := stubTimeline(t, nil)
	count := func() (timeline, moon int) {
		for _, c := range *calls {
			if c.req.Moon {
				moon++
			} else {
				timeline++
			}
		}
		return timeline, moon
	}
	want := func(what string, wantTimeline, wantMoon int) {
		t.Helper()
		if tl, mn := count(); tl != wantTimeline || mn != wantMoon {
			t.Fatalf("%s: %d Timeline and %d Moon searches, want %d and %d", what, tl, mn, wantTimeline, wantMoon)
		}
	}
	commit := func(m Model, when time.Time) Model {
		m, cmd := applyTextCommit(m, FirstFrameReadyMsg{TransitTime: when})
		return pump(t, m, cmd)
	}
	switchTo := func(m Model, k timelineList) Model {
		t.Helper()
		m = m.withDetailsTab(PanelChart, k.tab())
		m, cmd := m.syncTimeline()
		return pump(t, m, cmd)
	}

	m := loadedList(t, listTimeline, Config{Accessible: true})
	want("the Timeline loaded", 1, 0)
	if m.moonList.keyed {
		t.Fatal("the Moon list was keyed while the Timeline was shown")
	}
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
	m = commit(m, tlFloatingTime)
	want("a text commit with the Timeline shown", 1, 0)

	m = switchTo(m, listMoon)
	want("the Moon tab chosen the first time", 1, 1)
	m = switchTo(m, listTimeline)
	want("back to the Timeline, landed for the same inputs", 1, 1)
	m = switchTo(m, listMoon)
	want("back to the Moon list, landed for the same inputs", 1, 1)

	// A new month with the Moon list shown re-asks the Moon list only.
	m = commit(m, tlFloatingTime.AddDate(0, 1, 0))
	want("a new month with the Moon list shown", 1, 2)
	m = switchTo(m, listTimeline)
	want("the Timeline chosen after the month moved", 2, 2)
	if tl, _ := count(); tl != 2 {
		t.Fatalf("%d Timeline searches", tl)
	}
}

// M8: a failed search names the list it was asked for, in the Timeline's
// failure shapes.
func TestMoonSearchFailureIsNamed(t *testing.T) {
	forEachList(t, func(t *testing.T, lk timelineList) {
		errorLines := func(m Model) []string {
			var lane []string
			for _, l := range m.logLines {
				if l.lane == laneError {
					lane = append(lane, l.text)
				}
			}
			return lane
		}
		t.Run("the whole search", func(t *testing.T) {
			stubTimeline(t, func(lookup.TimelineRequest) (lookup.TimelineResult, error) {
				return lookup.TimelineResult{Commands: []string{"astrolog -z -2 -qd 10 17 2026 -d"}}, errors.New("astrolog exec: exit status 1")
			})
			m := timelineModel(t, Config{Accessible: true})
			m, cmd := syncListVisited(m, lk)
			m = pump(t, m, cmd)
			lane := errorLines(m)
			if len(lane) != 1 || !strings.HasPrefix(lane[0], lk.scope()+": "+lk.noun()+" not read,") {
				t.Fatalf("error lane = %q, want one line opening %q", lane, lk.scope()+": "+lk.noun()+" not read,")
			}
		})
		t.Run("one search of several", func(t *testing.T) {
			week := weekRows(t)
			stubTimeline(t, func(lookup.TimelineRequest) (lookup.TimelineResult, error) {
				return lookup.TimelineResult{Rows: week, Commands: []string{"astrolog -z -2 -qd 10 17 2026 -d"},
					Failures: []lookup.TimelineFailure{{Numeral: 0, Search: "-d", Err: errors.New("astrolog exec: exit status 1")}}}, nil
			})
			m := timelineModel(t, Config{Accessible: true})
			m, cmd := syncListVisited(m, lk)
			m = pump(t, m, cmd)
			lane := errorLines(m)
			if len(lane) != 1 || !strings.HasPrefix(lane[0], lk.scope()+": the events of the sky (-d) not listed") {
				t.Fatalf("error lane = %q, want the sky failure naming %s", lane, lk.noun())
			}
		})
	})
}

// ---- One implementation, two instances (M5, M6) ------------------------------

// moonSources parses the package's source files, tests or not.
func moonSources(t *testing.T, tests bool) map[string]*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{}
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") != tests {
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, p, src, 0)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		files[p] = f
	}
	return files
}

// exprText prints the source form of a simple type expression.
func exprText(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return exprText(x.X) + "." + x.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprText(x.Elt)
	case *ast.StarExpr:
		return "*" + exprText(x.X)
	case *ast.IndexListExpr:
		parts := make([]string, len(x.Indices))
		for i, ix := range x.Indices {
			parts[i] = exprText(ix)
		}
		return exprText(x.X) + "[" + strings.Join(parts, ", ") + "]"
	case *ast.IndexExpr:
		return exprText(x.X) + "[" + exprText(x.Index) + "]"
	}
	return ""
}

// M5: the Moon list is a kind of the one list, not a copy of it.
func TestMoonIsOnlyAListKind(t *testing.T) {
	files := moonSources(t, false)

	// The Moon list's kind constant is named in the file that declares the lists
	// and, from slice P on, in the phase-key handler.
	allowed := map[string]bool{"timeline_lists.go": true, "moon_phase.go": true}
	for name, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == "listMoon" && !allowed[name] {
				t.Errorf("%s names listMoon at %v", name, id.Pos())
			}
			return true
		})
	}

	// No function of timeline.go is a Moon copy.
	ast.Inspect(files["timeline.go"], func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok && strings.Contains(fn.Name.Name, "Moon") {
			t.Errorf("timeline.go declares %s", fn.Name.Name)
		}
		return true
	})

	// One list state type: of the struct types the Model holds as fields, exactly
	// one carries `rows []lookup.TimelineRow`. (A payload and a draw frame carry
	// rows too, and neither is a list: the Model does not hold them.)
	structs := map[string]*ast.StructType{}
	for _, f := range files {
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok {
				for _, s := range g.Specs {
					if ts, ok := s.(*ast.TypeSpec); ok {
						if st, ok := ts.Type.(*ast.StructType); ok {
							structs[ts.Name.Name] = st
						}
					}
				}
			}
		}
	}
	hasRows := func(st *ast.StructType) bool {
		for _, fld := range st.Fields.List {
			for _, n := range fld.Names {
				if n.Name == "rows" && exprText(fld.Type) == "[]lookup.TimelineRow" {
					return true
				}
			}
		}
		return false
	}
	held := map[string]int{}
	for _, fld := range structs["Model"].Fields.List {
		if name := exprText(fld.Type); name != "" && structs[name] != nil && hasRows(structs[name]) {
			held[name] += max(1, len(fld.Names))
		}
	}
	if len(held) != 1 || held["timelineState"] != len(allLists) {
		t.Errorf("list state types held by Model: %v, want timelineState once per list (%d)", held, len(allLists))
	}

	// One refresh-row builder for lists, one memory entry type, one renderer.
	builders := 0
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Type.Results == nil {
				continue
			}
			returnsRow, takesList := false, false
			for _, r := range fn.Type.Results.List {
				returnsRow = returnsRow || exprText(r.Type) == "surfaceRow"
			}
			for _, p := range fn.Type.Params.List {
				takesList = takesList || exprText(p.Type) == "timelineList"
			}
			if returnsRow && takesList {
				builders++
			}
		}
	}
	if builders != 1 {
		t.Errorf("%d funcs return a surfaceRow from a timelineList, want 1", builders)
	}
	entries := 0
	for _, fld := range structs["memory"].Fields.List {
		if strings.HasPrefix(exprText(fld.Type), "lru[timelineKey,") {
			entries++
		}
	}
	if entries != 1 {
		t.Errorf("memory holds %d lru[timelineKey, …] entries, want 1", entries)
	}
	renders := 0
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok && exprText(sel) == "panel.RenderTimeline" {
					renders++
				}
			}
			return true
		})
	}
	if renders != 1 {
		t.Errorf("panel.RenderTimeline is called from %d places, want 1", renders)
	}
}

// moonParityTests are the tests M6 turns into one subtest per list.
var moonParityTests = []string{
	"TestTimelineRunsOnItsFirstDeclaration", "TestTimelineRecomputesWhenASearchInputChanges",
	"TestTimelineDoesNotRecomputeWithinTheSameInputs", "TestTimelineFailureRetainsTheVisibleFrame",
	"TestNoRowsSurviveASearchInputChange", "TestNoRowsSurviveAnAnchorOrCityPick",
	"TestStaleTimelineResultIsDiscarded", "TestTabSwitchStartsNoCompute",
	"TestEnterOnATimelineRowMovesTheFloatingSlotAndClosesDetails", "TestEnterOnATimelineRowFromTheAnchoredSlot",
	"TestTimelineJumpRecomputesThroughTheTimeStepPath", "TestTimelineCursorStaysWithinTheViewport",
	"TestTimelineCursorStopsAtTheEnds", "TestEnterOnAnEmptyTimelineMovesNothing",
	"TestDetailsAttachedArrowsAndJKMoveTheRowCursor",
}

// parityFiles are the files whose every test is one of them, but for the frame
// tests, which wait for the operator's approval and are left as they are.
var parityFiles = []string{"timeline_filter_test.go", "timeline_keys_copy_test.go", "timeline_page_test.go",
	"timeline_bold_row_test.go", "timeline_lead_centre_test.go"}

var parityExempt = map[string]string{"TestTimelineFilterFrames": "goldens"}

// M6: every named test runs once per list.
func TestMoonParityCoversTheNamedTests(t *testing.T) {
	calls := map[string]bool{}
	for _, f := range moonSources(t, true) {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "Test") {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					if id, ok := c.Fun.(*ast.Ident); ok && id.Name == "forEachList" {
						calls[fn.Name.Name] = true
					}
				}
				return true
			})
		}
	}
	required := slices.Clone(moonParityTests)
	for name, f := range moonSources(t, true) {
		if !slices.Contains(parityFiles, name) {
			continue
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && strings.HasPrefix(fn.Name.Name, "Test") && parityExempt[fn.Name.Name] == "" {
				required = append(required, fn.Name.Name)
			}
		}
	}
	slices.Sort(required)
	for _, name := range slices.Compact(required) {
		if !calls[name] {
			t.Errorf("%s does not run once per list (forEachList)", name)
		}
	}
	// The parity files are the ones M6 names.
	for _, name := range parityFiles {
		if _, err := os.Stat(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// With a Moon in the registry the strip is the report, the Timeline and the
// Moon list, and `0`, `]` and `[` go round it (the tests that fix the report
// and Timeline pair ask for a registry without a Moon: twoTabs).
func TestMoonTabIsInTheTabCycle(t *testing.T) {
	stubTimeline(t, nil)
	m := loadedTimeline(t, Config{Accessible: true})
	if got := m.activeTabID(PanelDetails); got != TabDetailsReport {
		t.Fatalf("fixture: tab %s, want the report", got)
	}
	press := func(key string) {
		t.Helper()
		var cmd tea.Cmd
		m, cmd = tpress(t, m, runes(key))
		m = pump(t, m, cmd)
	}
	for _, step := range []struct {
		key  string
		want TabID
	}{
		{"0", TabDetailsTimeline}, {"0", TabDetailsMoon}, {"0", TabDetailsReport}, {"0", TabDetailsTimeline},
	} {
		press(step.key)
		if got := m.activeTabID(PanelDetails); got != step.want {
			t.Fatalf("%s: tab %s, want %s", step.key, got, step.want)
		}
	}
	m, _ = tpress(t, m, keyEnter) // attached: `]` and `[` move the Details tabs
	for _, step := range []struct {
		key  string
		want TabID
	}{
		{"]", TabDetailsMoon}, {"]", TabDetailsReport}, {"[", TabDetailsMoon}, {"[", TabDetailsTimeline},
	} {
		press(step.key)
		if got := m.activeTabID(PanelDetails); got != step.want {
			t.Fatalf("attached %s: tab %s, want %s", step.key, got, step.want)
		}
	}
	if !m.moonList.complete {
		t.Error("the Moon list was passed through and never searched")
	}
}
