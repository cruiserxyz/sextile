package ui

import (
	"errors"
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// actions_test.go holds the seam under the Actions palette (palette P1): the 24
// commands run by their id (runAction) and each one's single reason for not
// running now (blocked). Nothing here is on screen.

// paletteIDs are the commands: the entries whose table row carries Palette.
var paletteIDs = []keys.BindingID{
	keys.BindingAttach, keys.BindingSelect, keys.BindingSlotMoveUp, keys.BindingSlotMoveDown, keys.BindingSlotDelete,
	keys.BindingJumpNow, keys.BindingToggleBack,
	keys.BindingOpenAtlas, keys.BindingOpenCharts, keys.BindingSaveChart,
	keys.BindingBodies, keys.BindingHouses, keys.BindingZodiac, keys.BindingAspects,
	keys.BindingRenderWheel, keys.BindingLogConsole, keys.BindingConsole, keys.BindingSaveCity,
	keys.BindingHouseDisplay, keys.BindingSessions, keys.BindingNewSession,
	keys.BindingCopy, keys.BindingHelp, keys.BindingQuit,
}

// paletteModel is a ready Chart-focused workspace: three opened charts above
// Now, the cursor on the middle one, both registries read, and a window wide
// enough for Details.
func paletteModel(t *testing.T) Model {
	t.Helper()
	m := listModel(t, 3, 1)
	m.aspectRegistry = testRegistry()
	m.objectRegistry = testObjectRegistry()
	m.width, m.height = 200, 50
	return m
}

// floatingOnly is paletteModel with no opened chart: only Now.
func floatingOnly(t *testing.T) Model {
	t.Helper()
	m := paletteModel(t)
	m.slots = listWorkspace(t, 0)
	m.targetFloatingTime = m.slots.Floating().Time
	m.cursorIdx = 0
	return m
}

// stubQuitCleanup keeps Quit from ending the process-wide preview session.
func stubQuitCleanup(t *testing.T) {
	t.Helper()
	prev := cleanupPreview
	t.Cleanup(func() { cleanupPreview = prev })
	cleanupPreview = func() error { return nil }
}

// actionSnap is what a command that is blocked must leave as it was: the
// startup phase (which modal is open), Help, the floating chart's time and its
// target, the order and number of the slots and which are selected, the
// cursor and focus, the Log, the footer notice, the console and the wheel.
type actionSnap struct {
	startup  startupState
	help     bool
	floating time.Time
	target   time.Time
	previous time.Time
	order    []string // each slot's chart id ("" is Now), in list order
	selected int
	cursor   int
	focused  PanelID
	logs     []string
	notice   string
	console  bool
	wheel    bool
}

func snapOf(m Model) actionSnap {
	s := actionSnap{
		startup: m.startup, help: m.helpOpen, target: m.targetFloatingTime, previous: m.previousFloatingTime,
		cursor: m.cursorIdx, focused: m.focused, logs: m.logPanelLines(), notice: m.yank.notice,
		console: m.console.open, wheel: m.previewActive,
	}
	if m.slots != nil {
		s.floating = m.slots.Floating().Time
		for _, sl := range m.slots.All() {
			s.order = append(s.order, sl.Chart.ID)
		}
		s.selected = m.slots.SelectedCount()
	}
	return s
}

func (a actionSnap) equal(b actionSnap) bool {
	if a.startup != b.startup || a.help != b.help || !a.floating.Equal(b.floating) || !a.target.Equal(b.target) ||
		!a.previous.Equal(b.previous) || a.selected != b.selected || a.cursor != b.cursor || a.focused != b.focused ||
		a.notice != b.notice || a.console != b.console || a.wheel != b.wheel ||
		len(a.order) != len(b.order) || len(a.logs) != len(b.logs) {
		return false
	}
	for i := range a.order {
		if a.order[i] != b.order[i] {
			return false
		}
	}
	for i := range a.logs {
		if a.logs[i] != b.logs[i] {
			return false
		}
	}
	return true
}

// commandTitle is the command's title in the main or the leader-only table.
func commandTitle(id keys.BindingID) string {
	for _, e := range append(mainBindings(keys.DefaultKeys()), leaderOnlyBindings()...) {
		if e.ID == id && e.Title != "" {
			return e.Title
		}
	}
	return strconv.Itoa(int(id))
}

// The ids are the 24 commands and no other table row says it is one.
func TestPaletteRowsAreTheCommands(t *testing.T) {
	got := map[keys.BindingID]int{}
	for _, e := range append(mainBindings(keys.DefaultKeys()), leaderOnlyBindings()...) {
		if e.Palette {
			got[e.ID]++
		}
	}
	want := map[keys.BindingID]bool{}
	for _, id := range paletteIDs {
		want[id] = true
	}
	if len(want) != 24 {
		t.Fatalf("the test lists %d distinct commands, want 24", len(want))
	}
	for id := range want {
		if got[id] != 1 {
			t.Errorf("command %d is marked Palette %d times, want once", int(id), got[id])
		}
	}
	for id, n := range got {
		if !want[id] {
			t.Errorf("binding %d (%d rows) is marked Palette but is not one of the 24 commands", int(id), n)
		}
	}
	// No popup's, the console's or Details' own table lists a command.
	m := paletteModel(t)
	for o := keys.OwnerMain; o <= keys.OwnerSessionName; o++ {
		if o == keys.OwnerMain {
			continue
		}
		for _, e := range m.ownerBindings(o) {
			if e.Palette {
				t.Errorf("owner %d marks binding %d as a palette row; only the main screen's commands are", int(o), int(e.ID))
			}
		}
	}
}

// Every command runs by its id on a ready model, Copy too, and a motion, a
// page, a tab or a time step is not one.
func TestRunActionRunsEveryPaletteRow(t *testing.T) {
	stubQuitCleanup(t)
	for _, id := range append(append([]keys.BindingID(nil), paletteIDs...), keys.BindingCopy) {
		m := paletteModel(t)
		if _, _, ok := m.runAction(id); !ok {
			t.Errorf("runAction(%d) reports that it is not a command", int(id))
		}
	}
	for _, id := range []keys.BindingID{
		keys.BindingNextView, keys.BindingPrevView, keys.BindingFocusChart, keys.BindingDetailsTab, keys.BindingSlotCycleUp,
		keys.BindingSlotCycleDown, keys.BindingTimeStepYear, keys.BindingTimeStepMinute, keys.BindingScrollUp, keys.BindingPageUp,
		keys.BindingPageHome, keys.BindingTabNext, keys.BindingTabPrev, keys.BindingForceQuit, keys.BindingCancelLeader,
	} {
		m := paletteModel(t)
		before := snapOf(m)
		tm, cmd, ok := m.runAction(id)
		if ok {
			t.Errorf("runAction(%d) runs a motion, a page, a tab or a time step", int(id))
		}
		if cmd != nil || !before.equal(snapOf(tm.(Model))) {
			t.Errorf("runAction(%d) said it is no command and still changed the model or returned a command", int(id))
		}
	}
}

// A command that cannot run now opens nothing and changes nothing, except
// where the opener says something today: exactly that Log line or notice.
func TestBlockedActionOpensNothing(t *testing.T) {
	stubQuitCleanup(t)
	noHostDefault := func(t *testing.T, m Model) Model {
		t.Helper()
		prev := astrolog.CurrentAvailability()
		astrolog.SetAvailability(astrolog.Availability{})
		t.Cleanup(func() { astrolog.SetAvailability(prev) })
		return m
	}
	loading := func(t *testing.T, m Model) Model { m.startup = startupComputing; return m }
	noWorkspace := func(t *testing.T, m Model) Model { m.slots = nil; return m }
	onCursor := func(i int) func(*testing.T, Model) Model {
		return func(t *testing.T, m Model) Model { m.cursorIdx = i; return m }
	}
	cases := []struct {
		name    string
		id      keys.BindingID
		why     string
		fixture func(*testing.T, Model) Model
		log     func(Model) Model // today's Log line, when the guard says one
		notice  string            // today's footer notice, when the guard says one
	}{
		{"Details is not shown", keys.BindingAttach, whyNoDetails, func(t *testing.T, m Model) Model { m.width, m.height = 80, 24; return m }, nil, ""},
		{"only Now is open", keys.BindingSelect, whyNothingToPick, func(t *testing.T, _ Model) Model { return floatingOnly(t) }, nil, ""},
		{"Bodies has no row", keys.BindingSelect, whyNothingToPick, func(t *testing.T, m Model) Model { m.focused = PanelBodies; m.chart = chartViewState{}; return m }, nil, ""},
		{"first opened chart", keys.BindingSlotMoveUp, whyNoneAbove, onCursor(0), nil, ""},
		{"cursor on Now", keys.BindingSlotMoveUp, whyNotOnOpened, onCursor(3), nil, ""},
		{"Bodies has the focus", keys.BindingSlotMoveUp, whyNotOnOpened, func(t *testing.T, m Model) Model { m.focused = PanelBodies; return m }, nil, ""},
		{"last opened chart", keys.BindingSlotMoveDown, whyNoneBelow, onCursor(2), nil, ""},
		{"cursor on Now", keys.BindingSlotMoveDown, whyNotOnOpened, onCursor(3), nil, ""},
		{"Bodies has the focus", keys.BindingSlotMoveDown, whyNotOnOpened, func(t *testing.T, m Model) Model { m.focused = PanelBodies; return m }, nil, ""},
		{"cursor on Now", keys.BindingSlotDelete, whyNotOnOpened, onCursor(3), nil, ""},
		{"Bodies has the focus", keys.BindingSlotDelete, whyNotOnOpened, func(t *testing.T, m Model) Model { m.focused = PanelBodies; return m }, nil, ""},
		{"no workspace", keys.BindingJumpNow, whyNoCity, noWorkspace, nil, ""},
		{"no workspace", keys.BindingToggleBack, whyNoCity, noWorkspace, nil, ""},
		{"nothing to go back to", keys.BindingToggleBack, whyNoBack, func(t *testing.T, m Model) Model { return m }, nil, ""},
		{"no workspace", keys.BindingOpenCharts, whyNoCity, noWorkspace, nil, ""},
		{"city without a time zone", keys.BindingOpenCharts, whyNoCity, func(t *testing.T, m Model) Model { m.place = location.PlaceInfo{}; return m }, nil, ""},
		{"no workspace", keys.BindingSaveChart, whyNoCity, noWorkspace, nil, ""},
		{"cursor past the charts", keys.BindingSaveChart, whyNoChart, onCursor(99), nil, ""},
		{"cursor on an opened chart", keys.BindingSaveChart, whyOnlyNow, func(t *testing.T, m Model) Model { return m }, nil, noticeOnlyNow},
		{"no workspace", keys.BindingRenderWheel, whyNoCity, noWorkspace, nil, ""},
		{"loading", keys.BindingHouses, whyLoading, loading, nil, ""},
		{"no workspace", keys.BindingHouses, whyNoCity, noWorkspace, nil, ""},
		{"loading", keys.BindingZodiac, whyLoading, loading, nil, ""},
		{"no workspace", keys.BindingZodiac, whyNoCity, noWorkspace, nil, ""},
		{"loading", keys.BindingAspects, whyLoading, loading, nil, ""},
		{"registry unreadable", keys.BindingAspects, whyAspectsUnread, func(t *testing.T, m Model) Model {
			m.aspectRegistry, m.aspectRegErr = nil, errors.New("astrolog exited 1")
			return m
		}, nil, ""},
		{"loading", keys.BindingBodies, whyLoading, loading, nil, ""},
		{"registry unreadable", keys.BindingBodies, whyBodiesUnread, func(t *testing.T, m Model) Model {
			m.objectRegistry, m.objectRegErr = nil, errors.New("astrolog exited 1")
			return m
		}, nil, ""},
		{"drawn set unreadable", keys.BindingBodies, whyDrawnUnread, noHostDefault, func(m Model) Model {
			_, err := m.lens.VisibleObjectNumbers()
			return m.logNote("bodies", "the body list cannot open: %v", err)
		}, ""},
		{"loading", keys.BindingHouseDisplay, whyLoading, loading, nil, ""},
		{"loading", keys.BindingSessions, whyLoading, loading, nil, ""},
		{"no workspace", keys.BindingSessions, whyNoCity, noWorkspace, nil, ""},
		{"loading", keys.BindingNewSession, whyLoading, loading, nil, ""},
		{"no workspace", keys.BindingNewSession, whyNoCity, noWorkspace, nil, ""},
		{"nothing to put away", keys.BindingNewSession, whySessionEmpty, func(t *testing.T, _ Model) Model { return floatingOnly(t) }, func(m Model) Model {
			return m.logNote("session", "new, unnamed")
		}, ""},
		{"second window", keys.BindingSaveCity, whyNotFirstWindow, func(t *testing.T, m Model) Model { m.cfg.SecondWindow = true; return m }, func(m Model) Model {
			return m.logErrorLine("Save defaults is for the first window, which keeps the session. This window keeps nothing")
		}, ""},
		{"city without a time zone", keys.BindingSaveCity, whyNoCity, func(t *testing.T, m Model) Model { m.place = location.PlaceInfo{}; return m }, func(m Model) Model {
			return m.logErrorLine("default city not saved: no city chosen yet")
		}, ""},
	}
	guarded := map[keys.BindingID]bool{}
	for _, c := range cases {
		guarded[c.id] = true
		t.Run(commandTitle(c.id)+"/"+c.name, func(t *testing.T) {
			m := c.fixture(t, paletteModel(t))
			if got := m.blocked(c.id); got != c.why {
				t.Fatalf("blocked(%d) = %q, want %q (the fixture does not make the command wait)", int(c.id), got, c.why)
			}
			before := snapOf(m)
			want := before
			if c.log != nil {
				want.logs = c.log(m).logPanelLines()
			}
			if c.notice != "" {
				want.notice = c.notice
			}
			tm, cmd, ok := m.runAction(c.id)
			if !ok {
				t.Fatalf("runAction(%d) reports that it is not a command", int(c.id))
			}
			after := snapOf(tm.(Model))
			if !after.equal(want) {
				t.Errorf("a blocked command changed more than today's Log line or notice:\n before %+v\n after  %+v\n want   %+v", before, after, want)
			}
			if (c.notice != "") != (cmd != nil) {
				t.Errorf("cmd = %v, want a command only for a notice (notice %q)", cmd != nil, c.notice)
			}
		})
	}
	// The commands that nothing stops: blocked answers "" however the model stands.
	never := []keys.BindingID{keys.BindingOpenAtlas, keys.BindingLogConsole, keys.BindingConsole, keys.BindingHelp, keys.BindingQuit}
	// Copy is blocked only when there is nothing to copy (TestCopyBlockedWhenNothing).
	guarded[keys.BindingCopy] = true
	for _, id := range never {
		guarded[id] = true
		for name, m := range map[string]Model{"ready": paletteModel(t), "loading, no workspace": func() Model { m := paletteModel(t); m.startup, m.slots = startupBootstrapping, nil; return m }()} {
			if got := m.blocked(id); got != "" {
				t.Errorf("blocked(%d) on a %s model = %q, want it never blocked", int(id), name, got)
			}
		}
	}
	// A command missing from both lists would be one nobody asked.
	for _, id := range paletteIDs {
		if !guarded[id] {
			t.Errorf("command %d is in neither the blocked table nor the never-blocked list", int(id))
		}
	}
}

// The key and runAction are the same action: on a ready model each of the
// commands that has a key on the main screen leaves the same phase, Help and
// slots, and returns a command or none alike, pressed or run by its id.
func TestKeyAndRunActionAgree(t *testing.T) {
	stubQuitCleanup(t)
	leaderOnly := map[keys.BindingID]bool{
		keys.BindingLogConsole: true, keys.BindingSaveCity: true, keys.BindingHouseDisplay: true,
		keys.BindingSessions: true, keys.BindingNewSession: true, keys.BindingConsole: true, keys.BindingCopy: true,
	}
	isCommand := map[keys.BindingID]bool{}
	for _, id := range paletteIDs {
		isCommand[id] = true
	}
	m0 := paletteModel(t)
	if !m0.canAttachDetails() {
		t.Fatal("the ready model cannot attach Details, so Enter would agree by doing nothing")
	}
	n := 0
	for _, e := range mainBindings(keys.DefaultKeys()) {
		if !e.Palette || leaderOnly[e.ID] {
			continue
		}
		n++
		key := keyMsgFor(t, e.Binding.Keys()[0])
		pressed, pressCmd := tpress(t, paletteModel(t), key)
		tm, runCmd, ok := paletteModel(t).runAction(e.ID)
		if !ok {
			t.Errorf("%s: runAction does not run it", e.Title)
			continue
		}
		run := tm.(Model)
		if pressed.startup != run.startup || pressed.helpOpen != run.helpOpen || (pressCmd == nil) != (runCmd == nil) {
			t.Errorf("%s: the key leaves startup %q, help %v, cmd %v but runAction leaves %q, %v, %v",
				e.Title, pressed.startup, pressed.helpOpen, pressCmd != nil, run.startup, run.helpOpen, runCmd != nil)
		}
		if !snapOf(pressed).equal(snapOf(run)) {
			t.Errorf("%s: the key and runAction leave different slots, cursor, selection, Log or notice", e.Title)
		}
	}
	if n != 17 {
		t.Errorf("%d of the commands have a main key in the table, want 17 (the other five are reached through the leader)", n)
	}
}

// Every reason is short, starts lower case and carries no semicolon.
func TestBlockedReasonsAreShort(t *testing.T) {
	file := parseFile(t, "actions.go")
	var reasons []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			if !strings.HasPrefix(name.Name, "why") || i >= len(spec.Values) {
				continue
			}
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				t.Errorf("%s is not a string literal", name.Name)
				continue
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatal(err)
			}
			reasons = append(reasons, s)
		}
		return true
	})
	if len(reasons) < 10 {
		t.Fatalf("found %d reasons in actions.go, want the whole list", len(reasons))
	}
	for _, s := range reasons {
		if n := len([]rune(s)); n > 32 {
			t.Errorf("reason %q is %d characters, want at most 32", s, n)
		}
		if r := []rune(s)[0]; !unicode.IsLower(r) {
			t.Errorf("reason %q does not start lower case", s)
		}
		if strings.Contains(s, ";") {
			t.Errorf("reason %q holds a semicolon", s)
		}
	}
}

// The commands have one runner and one question, read from the source:
// runLeaderAction is gone, dispatchMainBinding has no case for a command,
// runAction has a case for each, every opener asks blocked, and none of the
// openers whose condition was the startup phase tests that phase itself.
func TestCommandsRunThroughRunActionAndAskBlocked(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	funcs := map[string]*ast.FuncDecl{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		for _, d := range parseFile(t, f).Decls {
			if fn, ok := d.(*ast.FuncDecl); ok {
				funcs[fn.Name.Name] = fn
			}
		}
	}
	if _, ok := funcs["runLeaderAction"]; ok {
		t.Error("runLeaderAction still exists: the leader runs its actions through runAction")
	}
	idName := func(e ast.Expr) string {
		if sel, ok := e.(*ast.SelectorExpr); ok {
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "keys" {
				return sel.Sel.Name
			}
		}
		return ""
	}
	cases := func(fn *ast.FuncDecl) map[string]bool {
		out := map[string]bool{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if cc, ok := n.(*ast.CaseClause); ok {
				for _, e := range cc.List {
					if name := idName(e); name != "" {
						out[name] = true
					}
				}
			}
			return true
		})
		return out
	}
	// The commands' names as keys.go spells them (the rows that carry Palette),
	// and Copy.
	commandNames := map[string]bool{"BindingCopy": true}
	for _, name := range paletteNames(t) {
		commandNames[name] = true
	}
	if len(commandNames) != 24 {
		t.Fatalf("found %d command names in keys.go, want the 24 commands", len(commandNames))
	}
	dispatch, ok := funcs["dispatchMainBinding"]
	if !ok {
		t.Fatal("dispatchMainBinding is gone")
	}
	for name := range cases(dispatch) {
		if commandNames[name] {
			t.Errorf("dispatchMainBinding has a case for %s, which is a command and runs through runAction", name)
		}
	}
	run, ok := funcs["runAction"]
	if !ok {
		t.Fatal("runAction is gone")
	}
	have := cases(run)
	for name := range commandNames {
		if !have[name] {
			t.Errorf("runAction has no case for %s", name)
		}
	}
	// Every opener asks blocked; the ones that tested the phase do not test it.
	// asks: a call blocked(keys.X) for the id, or blocked(v) with a variable v
	// where the function names keys.X itself (moveFocusedSlot picks up or down
	// by its delta).
	asks := func(fn *ast.FuncDecl, wantID string) bool {
		called, named := false, false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if e, ok := n.(ast.Expr); ok && idName(e) == wantID {
				named = true
			}
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "blocked" {
				if _, isVar := call.Args[0].(*ast.Ident); isVar || idName(call.Args[0]) == wantID {
					called = true
				}
			}
			return true
		})
		return called && named
	}
	openers := map[string][]string{
		"attachDetails": {"BindingAttach"}, "toggleFocusedSelection": {"BindingSelect"},
		"moveFocusedSlot": {"BindingSlotMoveUp", "BindingSlotMoveDown"}, "removeFocusedSlot": {"BindingSlotDelete"},
		"dispatchRenderWheel": {"BindingRenderWheel"}, "jumpFloatingToNow": {"BindingJumpNow"},
		"toggleFloatingBack": {"BindingToggleBack"}, "openChartPicker": {"BindingOpenCharts"},
		"openSavePrompt": {"BindingSaveChart"}, "openAspectChooser": {"BindingAspects"},
		"openBodySelector": {"BindingBodies"}, "openHouseFinder": {"BindingHouses"},
		"openZodiacFinder": {"BindingZodiac"}, "openHouseDisplay": {"BindingHouseDisplay"},
		"openSessionsList": {"BindingSessions"}, "resetSession": {"BindingNewSession"},
		"saveDefaultCity": {"BindingSaveCity"}, "applyAspectRegistryLoaded": {"BindingAspects"},
		"applyObjectRegistryLoaded": {"BindingBodies"},
	}
	names := make([]string, 0, len(openers))
	for name := range openers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fn, ok := funcs[name]
		if !ok {
			t.Errorf("%s is gone, so the guard cannot read it", name)
			continue
		}
		for _, id := range openers[name] {
			if !asks(fn, id) {
				t.Errorf("%s does not ask blocked(keys.%s)", name, id)
			}
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			be, ok := n.(*ast.BinaryExpr)
			if !ok || (be.Op != token.EQL && be.Op != token.NEQ) {
				return true
			}
			sel, selOK := be.X.(*ast.SelectorExpr)
			ready, readyOK := be.Y.(*ast.Ident)
			if selOK && readyOK && sel.Sel.Name == "startup" && ready.Name == "startupReady" {
				t.Errorf("%s tests the startup phase itself, which is blocked's question", name)
			}
			return true
		})
	}
}

// paletteNames are the ids of the entries that carry Palette in keys.go,
// as the source spells them (Binding...).
func paletteNames(t *testing.T) []string {
	t.Helper()
	var out []string
	ast.Inspect(parseFile(t, "keys.go"), func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		var id string
		palette := false
		for _, el := range lit.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			switch k, _ := kv.Key.(*ast.Ident); {
			case k != nil && k.Name == "ID":
				if sel, ok := kv.Value.(*ast.SelectorExpr); ok {
					id = sel.Sel.Name
				}
			case k != nil && k.Name == "Palette":
				if v, ok := kv.Value.(*ast.Ident); ok && v.Name == "true" {
					palette = true
				}
			}
		}
		if palette && id != "" {
			out = append(out, id)
		}
		return true
	})
	return out
}
