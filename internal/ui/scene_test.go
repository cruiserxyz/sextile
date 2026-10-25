package ui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// enteredByHand is rule 8 of the specification worked
// out without deriveScene: the pressed row holds while the floating target is
// its instant and its chart, if it has one, is still selected.
func enteredByHand(m Model) (held bool, chart *store.Slot) {
	if !m.details.pressedSet || !m.targetFloatingTime.Equal(landedAt(m.details.pressedAt.instant)) {
		return false, nil
	}
	if m.details.pressedAt.numeral == 0 {
		return true, nil
	}
	for _, s := range m.slots.SelectedInOrder() {
		if s == m.details.pressedChart {
			return true, s
		}
	}
	return false, nil
}

// the specification: one value, the scene, decides what every panel shows.

// sceneWorkspaces calls fn for every selection of a 3-chart + now workspace
// (16 subsets of the 4 slots) and every cursor position, handing it the model
// and the selection mask (bit i = slot i in list order, bit 3 = now).
func sceneWorkspaces(t *testing.T, fn func(t *testing.T, m Model, mask, cursor int)) {
	t.Helper()
	for mask := 0; mask < 16; mask++ {
		for cursor := 0; cursor < 4; cursor++ {
			m := listModel(t, 3, cursor)
			for i := 0; i < 4; i++ {
				m.slots.SetSlotSelected(i, mask&(1<<i) != 0)
			}
			m.base = nil
			fn(t, m, mask, cursor)
		}
	}
}

// pairChoices are the stored pair choices worth trying: none, and every slot.
func pairChoices(m Model) []*store.Slot {
	return append([]*store.Slot{nil}, m.slots.All()...)
}

// wantPairsFromSet is the pair rule written out again, independently of
// pairsOf: (s, cursor) for each other member s in numeral order that is not
// the floating slot `now`, which is never a base (operator 2026-10-24).
func wantPairsFromSet(set []*store.Slot, cursor, now *store.Slot) []scenePair {
	var want []scenePair
	for _, s := range set {
		if s == cursor || s == now {
			continue
		}
		want = append(want, scenePair{left: s, right: cursor})
	}
	return want
}

// checkPairsAndRoundTrip asserts the scene declares exactly want, the keys are
// pairwise distinct, and choosing each declared pair the way `[`/`]` does and
// deriving again yields that pair as the active one (the specification C5).
func checkPairsAndRoundTrip(t *testing.T, m Model, want []scenePair, where string) {
	t.Helper()
	m.base = nil
	sc := m.deriveScene()
	if len(sc.pairs) != len(want) {
		t.Fatalf("%s: %d pairs, want %d", where, len(sc.pairs), len(want))
	}
	for i := range want {
		if sc.pairs[i] != want[i] {
			t.Fatalf("%s: pair %d is not the rule's (left, right)", where, i)
		}
	}
	seen := map[scenePair]bool{}
	for i := range sc.pairs {
		if seen[sc.pairs[i]] {
			t.Fatalf("%s: pair %d repeats an earlier pair", where, i)
		}
		seen[sc.pairs[i]] = true
		got := choosePair(m, sc.pairs[i]).deriveScene()
		if got.pair == nil || *got.pair != sc.pairs[i] {
			t.Fatalf("%s: choosing pair %d does not make it the active pair", where, i)
		}
		if got.frame != sc.pairs[i].left {
			t.Fatalf("%s: choosing pair %d does not make its left chart the frame", where, i)
		}
	}
}

// C1: the rule, spelled out from the selection mask alone.
func TestSceneRule(t *testing.T) {
	sceneWorkspaces(t, func(t *testing.T, m Model, mask, cursor int) {
		all := m.slots.All()
		var set []*store.Slot
		for i, s := range all {
			if mask&(1<<i) != 0 {
				set = append(set, s)
			}
		}
		wantComparing := mask&(1<<cursor) != 0 && len(set) >= 2
		sc := m.deriveScene()
		where := func() string {
			return "mask " + string(rune('0'+mask/10)) + string(rune('0'+mask%10)) + " cursor " + string(rune('0'+cursor))
		}
		if (sc.mode == sceneComparing) != wantComparing {
			t.Fatalf("%s: comparing = %v, want %v", where(), sc.mode == sceneComparing, wantComparing)
		}
		if sc.cursor != all[cursor] {
			t.Fatalf("%s: cursor is not the slot under the cursor", where())
		}
		if len(sc.set) != len(set) {
			t.Fatalf("%s: set has %d slots, want %d", where(), len(sc.set), len(set))
		}
		for i := range set {
			if sc.set[i] != set[i] {
				t.Fatalf("%s: set[%d] out of numeral order", where(), i)
			}
		}
		if !wantComparing {
			if len(sc.shown) != 1 || sc.shown[0] != all[cursor] || sc.focus != 0 {
				t.Fatalf("%s: alone shows %d slots focus %d, want [cursor] focus 0", where(), len(sc.shown), sc.focus)
			}
			if sc.pairs != nil || sc.pair != nil || sc.frame != nil {
				t.Fatalf("%s: alone carries pairs/pair/frame", where())
			}
			return
		}
		if len(sc.shown) != len(set) || sc.shown[sc.focus] != all[cursor] {
			t.Fatalf("%s: shown = %d slots focus %d, want the set with focus on the cursor", where(), len(sc.shown), sc.focus)
		}
		if sc.baseline != set[0] || sc.cursorN != sc.focus+1 {
			t.Fatalf("%s: baseline/cursorN wrong (cursorN %d focus %d)", where(), sc.cursorN, sc.focus)
		}
		if sc.frame != all[cursor] || sc.pair != nil {
			t.Fatalf("%s: with no visitor the tab is the cursor's own and the cursor frames (S4a A1, A5)", where())
		}
		// the specification C5, spelled out from the
		// mask alone: the cursor's chart is the right side of one pair per other
		// anchored member in numeral order; now is never a base.
		want := wantPairsFromSet(set, all[cursor], all[3])
		checkPairsAndRoundTrip(t, m, want, where())
	})
	// C5, dense and sparse selections of a larger set: five anchored charts
	// and now, all 64 selections of its 6 slots, every cursor.
	for mask := 0; mask < 64; mask++ {
		for cursor := 0; cursor < 6; cursor++ {
			m := listModel(t, 5, cursor)
			all := m.slots.All()
			var set []*store.Slot
			for i, s := range all {
				m.slots.SetSlotSelected(i, mask&(1<<i) != 0)
				if mask&(1<<i) != 0 {
					set = append(set, s)
				}
			}
			m.base = nil
			where := fmt.Sprintf("five+now mask %d cursor %d", mask, cursor)
			sc := m.deriveScene()
			if wantComparing := mask&(1<<cursor) != 0 && len(set) >= 2; (sc.mode == sceneComparing) != wantComparing {
				t.Fatalf("%s: comparing = %v, want %v", where, sc.mode == sceneComparing, wantComparing)
			}
			if sc.mode != sceneComparing {
				continue
			}
			checkPairsAndRoundTrip(t, m, wantPairsFromSet(set, all[cursor], all[5]), where)
		}
	}
	// "① has a moment": with ① selected but carrying no moment, two or more
	// selected and the cursor on a selected slot, the scene is still alone.
	// Slot.Time is an exported field, so this is a state the code can reach.
	m := listModel(t, 3, 1)
	for i := 0; i < 4; i++ {
		m.slots.SetSlotSelected(i, true)
	}
	m.slots.Baseline().Time = time.Time{}
	if !anchorOf(m.slots.Baseline()).IsZero() {
		t.Fatal("setup: baseline still has a moment")
	}
	if sc := m.deriveScene(); sc.mode != sceneAlone || len(sc.shown) != 1 || sc.shown[0] != m.slots.All()[1] || sc.pairs != nil {
		t.Fatalf("① without a moment: mode %v shown %d, want alone with [cursor]", sc.mode, len(sc.shown))
	}
}

// C1 failure mode: a slot removed under the cursor is seen on the next read.
func TestSceneIsDerivedOnEveryRead(t *testing.T) {
	m := listModel(t, 3, 1)
	for i := 0; i < 3; i++ {
		m.slots.SetSlotSelected(i, true)
	}
	if m.deriveScene().mode != sceneComparing {
		t.Fatal("setup: expected comparing")
	}
	m.slots.SetSlotSelected(0, false)
	m.slots.SetSlotSelected(2, false)
	m.slots.SetSlotSelected(3, false)
	if sc := m.deriveScene(); sc.mode != sceneAlone || len(sc.shown) != 1 {
		t.Fatalf("after deselecting: mode %v shown %d, want alone with [cursor]", sc.mode, len(sc.shown))
	}
}

// C2: the four helpers agree with the scene, and their bodies read nothing but it.
func TestSceneHelpersAgree(t *testing.T) {
	sceneWorkspaces(t, func(t *testing.T, m Model, mask, cursor int) {
		for _, choice := range pairChoices(m) {
			m.base = choice
			sc := m.deriveScene()
			if m.comparing() != (sc.mode == sceneComparing) {
				t.Fatalf("mask %d cursor %d: comparing() disagrees with the scene", mask, cursor)
			}
			if got := m.pairSlots(); len(got) != len(sc.pairs) {
				t.Fatalf("mask %d cursor %d: pairSlots has %d, scene %d", mask, cursor, len(got), len(sc.pairs))
			}
			if (m.activePairSlot() == nil) != (sc.pair == nil) || (sc.pair != nil && *m.activePairSlot() != *sc.pair) {
				t.Fatalf("mask %d cursor %d: activePairSlot disagrees with scene.pair", mask, cursor)
			}
			// An arrival keeps the base unless it lands on it from a selected
			// anchored chart, which becomes the base (V5); it records the row.
			for _, from := range m.slots.All() {
				prior := m
				prior.at = from
				arrived := prior.noteArrival()
				want := choice
				if choice == sc.cursor && from != sc.cursor && !from.Floating && sc.numeral(from) != 0 {
					want = from
				}
				if arrived.base != want || arrived.at != sc.cursor {
					t.Fatalf("mask %d cursor %d: arrival from %p left base %p at %p, want base %p", mask, cursor, from, arrived.base, arrived.at, want)
				}
			}
			wantSig := comparisonSig{}
			if sc.mode == sceneComparing {
				wantSig = comparisonSig{comparing: true, cursor: sc.cursor, frame: sc.frame}
				if sc.pair != nil {
					wantSig.pair = *sc.pair
				}
				copy(wantSig.shown[:], sc.shown) // the specification C2: the shown set is in the key
			}
			if m.comparisonSig() != wantSig {
				t.Fatalf("mask %d cursor %d: comparisonSig disagrees with the scene", mask, cursor)
			}
		}
	})

	// "and nothing else": none of the helper bodies touches the store or the
	// cursor itself; the scene is their only input.
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "pairs.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	helpers := map[string]bool{"comparing": false, "pairSlots": false, "activePairSlot": false, "comparisonSig": false}
	for _, d := range file.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Recv == nil {
			continue
		}
		if _, want := helpers[fd.Name.Name]; !want {
			continue
		}
		helpers[fd.Name.Name] = true
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				switch sel.Sel.Name {
				case "slots", "cursorIdx", "focusedSlot", "SelectedCount", "SelectedInOrder", "Baseline", "Numeral", "anchorOf":
					t.Errorf("%s reads %s; it must read the scene only", fd.Name.Name, sel.Sel.Name)
				}
			}
			if id, ok := n.(*ast.Ident); ok && id.Name == "anchorOf" {
				t.Errorf("%s calls anchorOf; it must read the scene only", fd.Name.Name)
			}
			return true
		})
	}
	for name, seen := range helpers {
		if !seen {
			t.Errorf("helper %s not found in pairs.go", name)
		}
	}
}

// constNames lists the names declared in const blocks of file whose declared
// type is typ (the first spec names it; iota specs after it inherit it).
func constNames(t *testing.T, file, typ string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		inType := false
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			if vs.Type != nil {
				id, ok := vs.Type.(*ast.Ident)
				inType = ok && id.Name == typ
			}
			if inType {
				for _, n := range vs.Names {
					out = append(out, n.Name)
				}
			}
		}
	}
	return out
}

type sceneProjection func(m Model, sc scene) string

// wheelFollowsScene is what the wheel would draw against the scene.
func wheelFollowsScene(m Model, sc scene) string {
	plan, ok := m.wheelPlan()
	if !ok {
		return "no wheel plan with a workspace"
	}
	if sc.mode == sceneAlone {
		if plan.mode != wheelSingle || plan.place != sc.shown[0].Location {
			return "alone: the wheel is not the cursor's chart alone"
		}
		return ""
	}
	// The wheel draws exactly the active tab (S4a A16, D2): a pair is a bi-wheel
	// with the pair's left chart inside and its visitor outside; the chart's own
	// tab is the cursor's chart alone.
	if sc.pair == nil {
		if plan.mode != wheelSingle || plan.place != sc.cursor.Location {
			return "comparing, own tab: the wheel is not the cursor's chart alone"
		}
		return ""
	}
	if plan.mode != wheelPair || len(plan.rings) != 2 || len(plan.labels) != 2 {
		return "comparing, a pair: the wheel is not a bi-wheel"
	}
	for i, s := range []*store.Slot{sc.pair.left, sc.pair.right} {
		if plan.rings[i].Place != s.Location || (!s.Floating && plan.rings[i].When != s.Time) {
			return "comparing, a pair: the rings are not the pair's left chart inside and its right chart outside"
		}
	}
	return ""
}

// overlayFrameOf is the natal the overlay compute is handed for a scene: its
// frame, except that the floating slot framing itself has no natal, so the
// compute is handed ① (unused, no house overlay and no cross call).
func overlayFrameOf(sc scene) store.AnchorChart {
	if sc.frame.Floating {
		return anchorOf(sc.baseline)
	}
	return anchorOf(sc.frame)
}

func textComputeFollowsScene(m Model, sc scene) string {
	mode := selectComputeMode(m)
	switch {
	case sc.mode == sceneComparing && mode != computeModeOverlay:
		return "comparing: the text compute is not the overlay"
	case sc.mode == sceneAlone && mode == computeModeOverlay:
		return "alone: the text compute is the overlay"
	}
	if sc.mode == sceneComparing {
		frame, focus, opts := m.overlayInputs(sc, transitTimeFromWorkspace(m))
		var p overlayPlan
		for _, o := range opts {
			o(&p)
		}
		switch {
		case frame != overlayFrameOf(sc):
			return "comparing: the overlay's house frame is not the scene's frame"
		case p.houseFrame != (sc.cursor != sc.frame):
			return "comparing: the frame rides along iff the cursor is not the frame"
		case focus.IsZero() != (sc.cursor == sc.frame || sc.cursor.Floating):
			return "comparing: the cursor's own chart is computed iff it is not the frame"
		}
	}
	// The Bodies columns follow the scene: the compute asks for one per shown
	// chart, and a committed comparing frame for this scene carries as many.
	if sc.mode == sceneComparing {
		if got := len(m.shownColumns(sc, transitTimeFromWorkspace(m))); got != len(sc.shown) {
			return "comparing: the Bodies compute does not ask for one column per shown chart"
		}
		if cols, ok := m.bodyColumnsForScene(); ok && len(cols) != len(sc.shown) {
			return "comparing: the committed Bodies columns are not one per shown chart"
		}
	}
	return ""
}

func timelineFollowsScene(m Model, sc scene) string {
	req, _, ok := m.timelineInputs()
	if !ok {
		return "timeline inputs unavailable"
	}
	// The entered row on the timeline (rules 8 and 9 of
	// the specification): while it holds on the floating
	// slot at its committed moment, it is the bold row, even beside another row
	// of the same instant that the nearest-row rule would pick first.
	if held, _ := enteredByHand(m); held && m.focusedIsFloating() && m.floatingTime().Equal(landedAt(m.details.pressedAt.instant)) {
		p := m.details.pressedAt
		rows := []lookup.TimelineRow{
			{Instant: p.instant, Line: p.line + " (another row at the same instant)"},
			{Instant: p.instant, Line: p.line, Numeral: p.numeral},
		}
		if got := m.nearestRow(rows); got != 1 {
			return "the timeline's bold row is not the entered row"
		}
	}
	var want []int
	if sc.mode == sceneComparing {
		for i, s := range sc.shown {
			if !s.Floating {
				want = append(want, i+1)
			}
		}
	}
	if len(req.Natals) != len(want) {
		return "timeline natals are not the shown anchored slots"
	}
	for i, n := range req.Natals {
		if n.Numeral != want[i] {
			return "timeline natal numerals are not the shown numerals"
		}
	}
	return ""
}

var panelProjections = map[string]sceneProjection{
	"PanelChart":  wheelFollowsScene,
	"PanelBodies": textComputeFollowsScene,
	"PanelAspects": func(m Model, sc scene) string {
		tabs := m.panelTabs(PanelAspects)
		if len(tabs) != 1+len(sc.pairs) {
			return "aspects tabs are not Aspects plus one per pair"
		}
		for i, p := range sc.pairs {
			l, r := numeralOf(sc, p.left), numeralOf(sc, p.right)
			wantID := aspectsPairTabID(l, r)
			if i == 0 {
				wantID = TabAspectsCross
			}
			if tabs[i+1].ID != wantID {
				return "aspects pair tab does not name its pair's numerals"
			}
			if tabs[i+1].Title != pairTabTitle(l, r, m.cfg) {
				return "aspects pair tab title does not name its pair's numerals"
			}
		}
		want := 0
		if sc.pair != nil {
			for i, p := range sc.pairs {
				if p == *sc.pair {
					want = i + 1
				}
			}
		}
		if got := activeTabIndex(tabs, m.tabChoice(PanelAspects)); got != want {
			return "the active Aspects tab is not the scene's pair"
		}
		// The pressed row no longer chooses the tab (S4a A19): Enter stores what
		// it shows in the visitor, so the scene's pair above is the whole answer.
		// The scene still knows the pressed row (it marks the cell and the row).
		held, _ := enteredByHand(m)
		if held != (sc.entered != nil) {
			return "the scene's entered row is not the pressed row"
		}
		return ""
	},
	"PanelDetails": timelineFollowsScene,
	"PanelLog": func(m Model, sc scene) string {
		// The log is not chart-scoped: it is the same single tab in every scene.
		if tabs := m.panelTabs(PanelLog); len(tabs) != 1 || tabs[0].ID != TabLog {
			return "the log changed with the scene"
		}
		return ""
	},
}

var surfaceProjections = map[string]sceneProjection{
	"surfaceImage":    wheelFollowsScene,
	"surfaceText":     textComputeFollowsScene,
	"surfaceTimeline": timelineFollowsScene,
	// A body template's @now binds the floating moment, in every scene
	// (the specification P12).
	"surfaceTemplate": func(m Model, sc scene) string {
		_, req, _, ok := m.templateInputs()
		if !ok {
			return ""
		}
		f := m.slots.Floating()
		if !req.When.Equal(f.Time) || req.Place != f.Location {
			return "the template tab's @now is not the floating slot's moment and place"
		}
		return ""
	},
	"surfaceDetails": func(m Model, sc scene) string {
		sig := m.comparisonSig()
		if (sc.mode == sceneComparing) != sig.comparing || (sig.comparing && (sig.frame != sc.frame || sig.pair != scenePairOf(sc))) {
			return "the Details request signature is not the scene's frame and pair"
		}
		return ""
	},
}

func indexOf(slots []*store.Slot, s *store.Slot) int {
	for i, x := range slots {
		if x == s {
			return i
		}
	}
	return -1
}

func scenePairOf(sc scene) scenePair {
	if sc.pair == nil {
		return scenePair{}
	}
	return *sc.pair
}

func numeralOf(sc scene, s *store.Slot) int { return indexOf(sc.set, s) + 1 }

// frameSubstitution is rule 2 of the specification: hand the overlay builder a
// scene whose frame is another slot and its cusps follow it, with no panel
// edited. (The wheel no longer reads the frame: it draws the active pair, and
// TestFrameIdentity pins that the two agree.)
func frameSubstitution(m Model, sc scene) string {
	if sc.mode != sceneComparing {
		return ""
	}
	t := transitTimeFromWorkspace(m)
	try := func(o *store.Slot) string {
		sub := sc
		sub.frame = o
		frame, focus, opts := m.overlayInputs(sub, t)
		var p overlayPlan
		for _, f := range opts {
			f(&p)
		}
		if frame != overlayFrameOf(sub) {
			return "the overlay's cusps did not follow the substituted frame"
		}
		if p.houseFrame != (sc.cursor != o) {
			return "the frame-rides-along flag did not follow the substituted frame"
		}
		if (sc.cursor == o) != focus.IsZero() && !sc.cursor.Floating {
			return "the cursor's own chart did not follow the substituted frame"
		}
		return ""
	}
	if !sc.cursor.Floating {
		if msg := try(sc.cursor); msg != "" {
			return "frame = cursor: " + msg
		}
	}
	for _, o := range sc.set {
		if o.Floating || o == sc.cursor || o == sc.baseline {
			continue
		}
		if msg := try(o); msg != "" {
			return "frame = another member: " + msg
		}
	}
	return ""
}

// C3: every PanelID and every refresh surface, read from source, has a
// projection; one without fails by name. Each projection holds in every scene.
func TestEveryPanelFollowsTheScene(t *testing.T) {
	var panels []string
	for _, n := range constNames(t, "panel_id.go", "PanelID") {
		if n == "PanelNone" { // not a panel
			continue
		}
		panels = append(panels, n)
	}
	var surfaces []string
	for _, n := range constNames(t, "refresh.go", "surface") {
		if n == "numSurfaces" { // a count, not a surface
			continue
		}
		surfaces = append(surfaces, n)
	}
	sort.Strings(panels)
	sort.Strings(surfaces)
	if len(panels) == 0 || len(surfaces) == 0 {
		t.Fatalf("read %d panels and %d surfaces from source", len(panels), len(surfaces))
	}
	var missing []string
	for _, n := range panels {
		if panelProjections[n] == nil {
			missing = append(missing, "panel "+n)
		}
	}
	for _, n := range surfaces {
		if surfaceProjections[n] == nil {
			missing = append(missing, "surface "+n)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("no scene projection for: %s", strings.Join(missing, ", "))
	}
	sceneWorkspaces(t, func(t *testing.T, m Model, mask, cursor int) {
		// Every visitor, with no entered row and with a pressed row of
		// every numeral (0 is a sky row) held by the floating moment, so a panel
		// that ignores the entered row fails by name (rule 8 of
		// the specification).
		// Also the same row once the moment has moved on, and pinned to a
		// chart that is not selected: neither may hold.
		variants := []Model{m}
		for pressed := 0; pressed <= len(m.slots.All()); pressed++ {
			e := m
			e.details.pressedAt = timelineMark{instant: e.targetFloatingTime.UTC().Add(-59 * time.Second), line: "pressed", numeral: pressed}
			e.details.pressedSet = true
			e.details.pressedChart = nil
			if set := e.slots.SelectedInOrder(); pressed >= 1 && pressed <= len(set) {
				e.details.pressedChart = set[pressed-1]
			}
			late := e
			late.details.pressedAt.instant = late.details.pressedAt.instant.Add(time.Hour)
			variants = append(variants, e, late)
			for _, s := range e.slots.All() {
				if pressed >= 1 && !s.Floating && e.deriveScene().numeral(s) == 0 {
					gone := e
					gone.details.pressedChart = s
					variants = append(variants, gone)
					break
				}
			}
		}
		for pressed, e := range variants {
			for _, choice := range pairChoices(e) {
				e.base = choice
				sc := e.deriveScene()
				for _, n := range panels {
					if msg := panelProjections[n](e, sc); msg != "" {
						t.Fatalf("%s, mask %d cursor %d pressed %d: %s", n, mask, cursor, pressed, msg)
					}
				}
				for _, n := range surfaces {
					if msg := surfaceProjections[n](e, sc); msg != "" {
						t.Fatalf("%s, mask %d cursor %d pressed %d: %s", n, mask, cursor, pressed, msg)
					}
				}
				if msg := frameSubstitution(e, sc); msg != "" {
					t.Fatalf("substitution, mask %d cursor %d pressed %d: %s", mask, cursor, pressed, msg)
				}
			}
		}
	})
}
