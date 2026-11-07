package ui

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// All chart values in this file are constructed synthetic test inputs.

// `, n` starts a new, unnamed session (specs/reset-session.md): the open
// charts go, the saved choices go, the named session that was open stays as
// last saved.

const resetNamed = "Weekly review"

const resetLogLine = "session: new and unnamed, " + resetNamed + " stays as it was saved"

// namedResetModel is a window running the named session "Weekly review" with
// three charts, a house choice made in it, and the file written.
func namedResetModel(t *testing.T) Model {
	t.Helper()
	freshModels(t)
	stubHouseLookups(t)
	m := sessionsModel(t, 3)
	t.Cleanup(store.ReleaseInstanceLock)
	m = saveAs(t, m, resetNamed)
	if !m.named.running() {
		t.Fatalf("the save did not make the window run %q (refusal %q)", resetNamed, m.sessions.Refusal)
	}
	m, cmd := m.chooseHouseSystem(1, recipe.Regiomontanus)
	m = pump(t, m, cmd)
	return m
}

// fileIdentity is a file's inode and modification time: a rewrite through a
// temporary file changes the first even when the bytes come out the same.
func fileIdentity(t *testing.T, path string) (uint64, time.Time) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Sys().(*syscall.Stat_t).Ino, fi.ModTime()
}

func namedResetPath() string { return store.NamedSessionPath("weekly-review.yaml") }

func resetKey(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := pressReset(t, m)
	return pump(t, m, cmd)
}

func quitWindow(t *testing.T, m Model) {
	t.Helper()
	tm, _ := m.Update(escRunes("q"))
	if e := tm.(Model).SessionWriteError(); e != nil {
		t.Fatalf("quit: %v", e)
	}
}

// 1
func TestResetClosesEveryChart(t *testing.T) {
	m := namedResetModel(t)
	if m.slots.AnchoredCount() != 3 {
		t.Fatalf("fixture: %d charts", m.slots.AnchoredCount())
	}
	m = resetKey(t, m)
	if got := len(m.slots.All()); got != 1 || m.slots.AnchoredCount() != 0 {
		t.Errorf("after `, n` the Chart panel lists %d slot(s), %d anchored, want the floating slot alone", got, m.slots.AnchoredCount())
	}
	if m.slots.SlotSelected(0) {
		t.Error("the floating slot is still selected")
	}
	if m.cursorIdx != 0 {
		t.Errorf("cursor on row %d, want the floating slot", m.cursorIdx)
	}
}

// 2
func TestResetKeepsTheFloatingMomentAndDropsTheCursorChartAtOnce(t *testing.T) {
	m := namedResetModel(t)
	m.cursorIdx = 0
	before := m.slots.Floating()
	if !strings.Contains(m.View(), "Synthetic 1") {
		t.Fatal("fixture: the chart under the cursor is not on screen")
	}
	m = resetKey(t, m)
	after := m.slots.Floating()
	if !after.Time.Equal(before.Time) || after.Location != before.Location {
		t.Errorf("floating slot moved: %v %+v -> %v %+v", before.Time, before.Location, after.Time, after.Location)
	}
	if v := m.View(); strings.Contains(v, "Synthetic") {
		t.Errorf("a closed chart is still on screen right after `, n`:\n%s", v)
	}
}

// 3
func TestResetReturnsHouseAndZodiacChoicesToTheDefaults(t *testing.T) {
	m := namedResetModel(t)
	// Saved defaults that are not the built-in fallback (Whole Sign, tropical).
	m.frames.def, m.frames.defSet = recipe.Koch, true
	m.frames.defZ, m.frames.defZSet = recipe.Lahiri, true
	floatIdx := len(m.slots.All()) - 1
	m, cmd := m.chooseHouseSystem(floatIdx, recipe.Regiomontanus)
	m = pump(t, m, cmd)
	m, cmd = m.chooseZodiac(floatIdx, recipe.FaganBradley)
	m = pump(t, m, cmd)
	if len(m.frames.records) == 0 {
		t.Fatal("fixture: no house choice recorded")
	}
	m.frames.volatile = map[string]recipe.HouseSystem{"x": recipe.Regiomontanus}
	m.frames.volatileZ = map[string]recipe.Zodiac{"x": recipe.FaganBradley}
	m = resetKey(t, m)
	if len(m.frames.records) != 0 || len(m.frames.volatile) != 0 || len(m.frames.volatileZ) != 0 {
		t.Errorf("choices survive the reset: %+v / %v / %v", m.frames.records, m.frames.volatile, m.frames.volatileZ)
	}
	if got := m.sessionSnapshot().Frames; len(got) != 0 {
		t.Errorf("the session would still save frames %+v", got)
	}
	// A chart opened now, and the floating one, show the saved defaults.
	if err := m.slots.AddAnchored(store.SessionChart{ID: "n", Name: "New", Source: store.ChartSource{Kind: store.ChartKindFile, File: "new.as"}}, fixtureOneWhen, fixtureOneP); err != nil {
		t.Fatal(err)
	}
	for _, s := range m.slots.All() {
		if got := m.ownHouses(s).hs; got != recipe.Koch {
			t.Errorf("%q shows houses %v, want the saved default Koch", s.Chart.Name, got)
		}
		if got := m.ownZodiac(s); got != recipe.Lahiri {
			t.Errorf("%q shows zodiac %v, want the saved default Lahiri", s.Chart.Name, got)
		}
	}
}

// 4
func TestResetLogsOneLineAndNothingElse(t *testing.T) {
	m := namedResetModel(t)
	before := logTexts(m)
	notice := m.footerIndicator()
	// Before the recompute: its own provenance lines are not the reset's.
	m, _ = pressReset(t, m)
	got := logTexts(m)
	if len(got) != len(before)+1 || got[len(got)-1] != resetLogLine {
		t.Errorf("Log gained %v, want exactly %q", got[min(len(before), len(got)):], resetLogLine)
	}
	if m.footerIndicator() != notice {
		t.Errorf("footer notice changed: %q -> %q", notice, m.footerIndicator())
	}
	// An unnamed window with a chart says it without the name.
	u := sessionsModel(t, 1)
	t.Cleanup(store.ReleaseInstanceLock)
	n := len(logTexts(u))
	u, _ = press(t, u, ",", "n", ",", "n")
	if got := logTexts(u); len(got) != n+1 || got[len(got)-1] != "session: new, unnamed" {
		t.Errorf("unnamed window Log = %v", got[min(n, len(got)):])
	}
}

// 5
func TestResetThenQuitSavesTheUnnamedSessionWithTheFloatingSlotAlone(t *testing.T) {
	m := namedResetModel(t)
	m = resetKey(t, m)
	quitWindow(t, m)
	s, ok, err := store.LoadSession()
	if err != nil || !ok {
		t.Fatalf("no unnamed session saved: ok %v err %v", ok, err)
	}
	if len(s.Slots) != 0 || len(s.Frames) != 0 || s.FloatingSelected || len(s.FloatingBodies) != 0 {
		t.Errorf("the unnamed session holds more than the floating slot: %+v", s)
	}
	p := readPerimeterCmd(false)().(ingest.StartupPerimeterReadMsg)
	if p.HasSession && len(p.Session.Slots) != 0 {
		t.Errorf("a relaunch would restore %d chart(s)", len(p.Session.Slots))
	}
}

// 6
func TestResetLeavesTheNamedSessionUnwritten(t *testing.T) {
	m := namedResetModel(t)
	saved := fileText(t, namedResetPath())
	ino, mod := fileIdentity(t, namedResetPath())
	untouched := func(when string) {
		t.Helper()
		if i, mt := fileIdentity(t, namedResetPath()); i != ino || !mt.Equal(mod) {
			t.Errorf("the named session file was rewritten %s (inode %d -> %d, modified %v -> %v)", when, ino, i, mod, mt)
		}
	}
	m = resetKey(t, m)
	untouched("by `, n`")
	if got := fileText(t, namedResetPath()); got != saved {
		t.Errorf("`, n` wrote the named session:\n--- before ---\n%s--- after ---\n%s", saved, got)
	}
	// A choice made afterwards belongs to the unnamed session.
	m, cmd := m.chooseHouseSystem(0, recipe.Regiomontanus)
	m = pump(t, m, cmd)
	if got := fileText(t, namedResetPath()); got != saved {
		t.Errorf("a choice after `, n` reached the named session:\n%s", got)
	}
	untouched("by a later choice")
	quitWindow(t, m)
	untouched("by the quit")
	if got := fileText(t, namedResetPath()); got != saved {
		t.Errorf("the quit wrote the named session:\n--- before ---\n%s--- after ---\n%s", saved, got)
	}
}

// 7
func TestSessionsListAfterResetOpensTheSavedSessionAsSaved(t *testing.T) {
	m := namedResetModel(t)
	names := anchoredNames(m)
	var marks []bool
	for i := range m.slots.All() {
		marks = append(marks, m.slots.SlotSelected(i))
	}
	frames := m.frameRecordsToSave()
	snapshot := m.sessionSnapshot()
	if len(frames) == 0 || !slices.Contains(marks, true) {
		t.Fatalf("fixture carries no frame record or selection: %v %v", frames, marks)
	}
	m = resetKey(t, m)
	m = openSessions(t, m)
	rows := sessionsRows(m)
	if len(rows) != 2 || !strings.Contains(rows[0].Left, "Save this session as") {
		t.Fatalf("rows = %+v, want the Save row then Weekly review", rows)
	}
	if rows[1].Left != resetNamed || slices.Contains(rows[1].Right, "this window") {
		t.Errorf("row 2 = %+v, want %q without \"this window\"", rows[1], resetNamed)
	}
	m, _ = press(t, m, "down")
	m, cmd := press(t, m, "enter")
	if cmd == nil {
		t.Fatal("Enter on the saved session started nothing")
	}
	tm, _ := m.Update(cmd())
	m = tm.(Model)
	if got := anchoredNames(m); !slices.Equal(got, names) {
		t.Errorf("opened charts = %v, want as saved %v", got, names)
	}
	if m.named.name != resetNamed {
		t.Errorf("the window runs %q", m.named.name)
	}
	var gotMarks []bool
	for i := range m.slots.All() {
		gotMarks = append(gotMarks, m.slots.SlotSelected(i))
	}
	if !slices.Equal(gotMarks, marks) {
		t.Errorf("selection = %v, want as saved %v", gotMarks, marks)
	}
	if got := m.sessionSnapshot(); !reflect.DeepEqual(got, snapshot) {
		t.Errorf("reopened session differs from the saved one:\n got %+v\nwant %+v", got, snapshot)
	}
	if got := m.frameRecordsToSave(); !reflect.DeepEqual(got, frames) {
		t.Errorf("frame records = %+v, want as saved %+v", got, frames)
	}
}

// 8
func TestResetOnAnEmptyUnnamedWindowOnlyLogs(t *testing.T) {
	m := sessionsModel(t, 0)
	t.Cleanup(store.ReleaseInstanceLock)
	slots, startup, n := m.slots, m.startup, len(logTexts(m))
	m, cmd := pressReset(t, m)
	if cmd != nil || m.slots != slots || m.startup != startup {
		t.Errorf("`, n` did work on an empty unnamed window: cmd %v, slots rebuilt %v, startup %q", cmd != nil, m.slots != slots, m.startup)
	}
	if got := logTexts(m); len(got) != n+1 || got[len(got)-1] != "session: new, unnamed" {
		t.Errorf("Log = %v", got[min(n, len(got)):])
	}
}

// 9
func TestHelpListsResetAndAMistypedSecondKeyStillCancels(t *testing.T) {
	m := namedResetModel(t)
	found := false
	for _, l := range m.helpLines() {
		if strings.Contains(l.Text, ",n") && strings.Contains(strings.ToLower(l.Text), "new session") {
			found = true
		}
	}
	if !found {
		t.Error("the Keys popup does not list `,n` with a new-session description")
	}
	n := len(logTexts(m))
	m, _ = press(t, m, ",", "x")
	if m.leader.pending || m.slots.AnchoredCount() != 3 || len(logTexts(m)) != n {
		t.Errorf("`, x` did not cancel quietly: pending %v, charts %d, log %d -> %d", m.leader.pending, m.slots.AnchoredCount(), n, len(logTexts(m)))
	}
}

// 10
func TestResetGivesTheNamedLockBackAndTakesTheUnnamedOne(t *testing.T) {
	m := namedResetModel(t)
	m = resetKey(t, m)
	if m.named.running() || m.cfg.SecondWindow {
		t.Fatalf("after `, n`: named %v, second window %v", m.named.running(), m.cfg.SecondWindow)
	}
	_, err := store.AcquireInstanceLock()
	var held *store.LockHeldError
	if !errors.As(err, &held) {
		t.Errorf("the unnamed session's lock is free after `, n` (err %v): this window does not hold it", err)
	}
	_, _, lock, err := store.OpenNamedSession("weekly-review.yaml", store.FolderID(store.ChartsDir()))
	if err != nil {
		t.Errorf("the named session's lock was not given back: %v", err)
	} else {
		lock.Release()
	}
}

func TestResetWhenAnotherWindowOwnsTheUnnamedSessionKeepsNothing(t *testing.T) {
	m := namedResetModel(t)
	general := fileText(t, store.SessionPath())
	store.ReleaseInstanceLock()
	if _, err := store.AcquireInstanceLock(); err != nil { // another window takes it
		t.Fatal(err)
	}
	m, _ = pressReset(t, m)
	if got := len(m.slots.All()); got != 1 {
		t.Errorf("`, n` left %d slots", got)
	}
	if !m.cfg.SecondWindow || m.ownsSession() {
		t.Error("the window still claims the unnamed session another window owns")
	}
	want := "session: new: another window owns the unnamed session, this one keeps nothing"
	if got := logTexts(m); got[len(got)-1] != want {
		t.Errorf("last Log line = %q, want %q", got[len(got)-1], want)
	}
	quitWindow(t, m)
	if got := fileText(t, store.SessionPath()); got != general {
		t.Errorf("the window wrote the unnamed session another window owns:\n%s", got)
	}
}

// The Chart panel after `, n` from a named session with three charts, at
// 80x24 with colours off: the floating slot alone, its "1 of 1" counter.
func TestResetChartPanelFrame(t *testing.T) {
	forceColor(t)
	m := namedResetModel(t)
	m.cfg = Config{Color: ColorNever}
	m.width, m.height = 80, 24
	m = resetKey(t, m)
	v := m.View()
	assertFrameGeometry(t, v, 80, 24)
	checkGolden(t, "reset-session-chart-80x24-never", strings.Join(frameRows(v)[:3], "\n"))
}

// Review round 1 (fix 1): a window with no chart but a selected floating slot
// is not empty; `, n` clears the selection and the quit saves none.
func TestResetClearsAFloatingSelectionWithNoCharts(t *testing.T) {
	m := sessionsModel(t, 0)
	t.Cleanup(store.ReleaseInstanceLock)
	m.slots.ToggleSlotSelection(0)
	if !m.slots.SlotSelected(0) {
		t.Fatal("fixture: floating slot not selected")
	}
	m = resetKey(t, m)
	if m.slots.SlotSelected(0) {
		t.Error("the floating slot is still selected after `, n`")
	}
	quitWindow(t, m)
	if s, ok, err := store.LoadSession(); err != nil || !ok || s.FloatingSelected {
		t.Errorf("the quit saved a selection: ok %v err %v %+v", ok, err, s)
	}
}

// Fix 2: a window that launched as a second window, then ran a named session,
// takes the unnamed session when it is free and keeps it.
func TestResetFromNamedTakesAFreeUnnamedSessionEvenAfterASecondWindowLaunch(t *testing.T) {
	m := namedResetModel(t)
	m.cfg.SecondWindow = true
	m = resetKey(t, m)
	if m.cfg.SecondWindow || !m.ownsSession() {
		t.Fatalf("the window does not own the free unnamed session (second window %v)", m.cfg.SecondWindow)
	}
	// The flag proves nothing: the lock itself must be held by this window.
	var held *store.LockHeldError
	if _, err := store.AcquireInstanceLock(); !errors.As(err, &held) {
		t.Errorf("a second take of the unnamed lock = %v, want *store.LockHeldError (this window holds it)", err)
	}
	quitWindow(t, m)
	store.ReleaseInstanceLock() // the process-wide handle; quit ends the real process
	if _, err := store.AcquireInstanceLock(); err != nil {
		t.Errorf("the unnamed lock is still held after the quit: %v", err)
	}
	if s, ok, err := store.LoadSession(); err != nil || !ok || len(s.Slots) != 0 {
		t.Errorf("the quit did not save the unnamed session empty: ok %v err %v %d charts", ok, err, len(s.Slots))
	}
}

// Fix 3: Bodies and Aspects do not keep the former chart while the recompute
// runs.
func TestResetShowsNoFormerChartBeforeTheRecompute(t *testing.T) {
	m := namedResetModel(t)
	m.cursorIdx = 0
	// What only the former chart's frame paints: a Bodies row and the Aspects
	// panel's text (the name is in the Chart panel and proves nothing here).
	const bodyRow, aspectText = "Vir 11°00'", "(no aspects in orb)"
	before := m.View()
	if !strings.Contains(before, bodyRow) || !strings.Contains(before, aspectText) {
		t.Fatalf("fixture: the frame paints neither %q nor %q:\n%s", bodyRow, aspectText, before)
	}
	m, _ = pressReset(t, m) // no pump
	v := m.View()
	for _, gone := range []string{"Synthetic", bodyRow, aspectText} {
		if strings.Contains(v, gone) {
			t.Errorf("%q is still on screen before the recompute:\n%s", gone, v)
		}
	}
}

// Fix 4: a time step still on its way does not make the moment jump.
func TestResetKeepsTheDisplayedMomentAfterAPendingStep(t *testing.T) {
	m := namedResetModel(t)
	m, _ = press(t, m, "d") // steps the target; the slot has not committed it
	shown := m.slots.Floating().Time
	if m.targetFloatingTime.Equal(shown) {
		t.Fatal("fixture: no step pending")
	}
	m = resetKey(t, m)
	if got := m.slots.Floating().Time; !got.Equal(shown) {
		t.Errorf("the moment jumped from %v to %v", shown, got)
	}
}

// Fix 5: a lock failure that is not contention keeps nothing and says why.
func TestResetLockFailureIsNamedInTheLog(t *testing.T) {
	m := namedResetModel(t)
	dir := store.SessionDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "elsewhere"), filepath.Join(dir, "sextile.lock")); err != nil {
		t.Fatal(err)
	}
	_, lockErr := store.AcquireInstanceLock()
	var le *store.LockError
	if !errors.As(lockErr, &le) {
		t.Fatalf("fixture: the lock error is %v, want a *store.LockError", lockErr)
	}
	want := firstLine(lockErr.Error())
	m, _ = pressReset(t, m)
	got := logTexts(m)
	last := got[len(got)-1]
	if !strings.Contains(last, want) {
		t.Errorf("last Log line = %q, want the lock error's own text %q", last, want)
	}
	if strings.Contains(last, "another window owns") || !strings.HasPrefix(last, "session: new, this window keeps nothing: ") {
		t.Errorf("last Log line = %q, want the lock failure's own text", last)
	}
	if m.ownsSession() {
		t.Error("the window keeps writing the unnamed session it could not lock")
	}
}
