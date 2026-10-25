package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// modal_list_behaviour_test.go holds the list modals to the promises a list
// modal has to keep, over EVERY cursor position rather than a sampled few.
//
// The promise that matters, and the one that was broken: the cursor is always
// on screen. The window used to be sliced from the top unconditionally, so the
// moment the cursor walked past the last visible row it simply vanished and the
// operator was navigating a list they could not see themselves in. Both the
// atlas picker and the aspect chooser had it, because both take their rows from
// the same chassis — which is also why one fix covers both.
//
// These drive the REAL render functions, not the state machine, because
// "the cursor is visible" is a claim about what is on screen.

// countCursors counts cursor rows: with colours off, reverse video opening
// on the cursor marker.
func countCursors(frame string) int { return strings.Count(frame, cursorRowOff+rowCursorASCII) }

// --- the chassis itself ---------------------------------------------------

// walkList drives a cursor from 0 to n-1 and back, the way arrow keys do,
// scrolling exactly as the modal does, and reports the window at each step.
func walkList(n, height int, down, up int) (offsets []int, cursors []int, list *fuzzyList[int]) {
	items := make([]int, n)
	for i := range items {
		items[i] = i
	}
	l := &fuzzyList[int]{matches: items}
	cur := 0
	for i := 0; i < down; i++ {
		if cur+1 < n {
			cur++
		}
		l.scrollTo(cur, height)
		offsets = append(offsets, l.offset)
		cursors = append(cursors, cur)
	}
	for i := 0; i < up; i++ {
		if cur > 0 {
			cur--
		}
		l.scrollTo(cur, height)
		offsets = append(offsets, l.offset)
		cursors = append(cursors, cur)
	}
	return offsets, cursors, l
}

func TestWindowKeepsTheCursorVisibleAtEveryIndex(t *testing.T) {
	const n = 40
	for _, height := range []int{1, 2, 3, 5, 7, 12, 40, 50} {
		offsets, cursors, l := walkList(n, height, n-1, n-1)
		for i := range cursors {
			cur, off := cursors[i], offsets[i]
			rows := height
			if n < height {
				rows = n
			}
			if off < 0 || off+rows > n {
				t.Fatalf("height %d step %d: window [%d,%d) runs off a list of %d", height, i, off, off+rows, n)
			}
			if cur < off || cur >= off+rows {
				t.Errorf("height %d step %d: cursor %d NOT VISIBLE in window [%d,%d)", height, i, cur, off, off+rows)
			}
		}
		if l.offset != 0 {
			t.Errorf("height %d: walking back to the top left the window at offset %d, want 0", height, l.offset)
		}
	}
}

// THE property that was wrong. With room on both sides the cursor keeps
// listScrollMargin rows above AND below it. The old rule had a bottom margin
// only: walking down felt like moving a cursor, walking back up felt like
// dragging the list, because the window moved on every single step.
func TestWindowKeepsTheMarginOnBothSides(t *testing.T) {
	const n, height = 60, 20
	offsets, cursors, _ := walkList(n, height, n-1, n-1)

	for i := range cursors {
		cur, off := cursors[i], offsets[i]
		above, below := cur-off, off+height-1-cur

		// The margin is necessarily spent at the two ends of the list:
		// there is nothing to show beyond them, and refusing to reach the
		// first or last row would be worse than losing the margin.
		if cur >= listScrollMargin && above < listScrollMargin && off != 0 {
			t.Errorf("step %d: cursor %d has %d rows above it (offset %d), want >= %d", i, cur, above, off, listScrollMargin)
		}
		if cur < n-listScrollMargin && below < listScrollMargin && off != n-height {
			t.Errorf("step %d: cursor %d has %d rows below it (offset %d), want >= %d", i, cur, below, off, listScrollMargin)
		}
	}
}

// The window holds STILL while the cursor moves through the middle of it. This
// is what "the cursor moves, not the list" means, and it is the half the old
// rule could not express: an offset derived from the cursor necessarily moves
// whenever the cursor does.
func TestWindowHoldsStillWhileTheCursorCrossesIt(t *testing.T) {
	const n, height = 60, 20
	offsets, cursors, _ := walkList(n, height, n-1, 0)

	moves := 0
	for i := 1; i < len(offsets); i++ {
		if offsets[i] != offsets[i-1] {
			moves++
		}
	}
	// Walking the whole list scrolls only as far as the list is longer than
	// the window; anything more means the window is being dragged.
	if want := n - height; moves != want {
		t.Errorf("walking down %d rows moved the window %d times, want exactly %d", n-1, moves, want)
	}

	// The first scroll happens on the step AFTER the cursor reaches the
	// bottom margin row — the cursor is allowed to stand on that row with
	// listScrollMargin rows still visible below it, and only moving beyond
	// it costs a scroll.
	firstMove := -1
	for i := 1; i < len(offsets); i++ {
		if offsets[i] != offsets[i-1] {
			firstMove = cursors[i]
			break
		}
	}
	if want := height - listScrollMargin; firstMove != want {
		t.Errorf("the window first moved at cursor %d, want %d", firstMove, want)
	}
}

// Both directions must behave the same WAY. Per-cursor offsets deliberately do
// NOT match between the two directions — that hysteresis is what lets the
// window hold still instead of re-centring — so the property to hold is the
// shape of the motion, not the numbers:
//
//   - the window moves the same number of times going up as going down;
//   - every move is exactly one row, never a jump;
//   - the walk ends where it started.
//
// An earlier version of this test asserted equal offsets per cursor and was
// wrong: it would have failed the correct implementation and passed the
// lockstep one that caused the bad feel in the first place.
func TestScrollingIsSymmetric(t *testing.T) {
	const n, height = 60, 20

	countMoves := func(offsets []int) (moves int, maxStep int) {
		for i := 1; i < len(offsets); i++ {
			d := offsets[i] - offsets[i-1]
			if d < 0 {
				d = -d
			}
			if d > 0 {
				moves++
			}
			if d > maxStep {
				maxStep = d
			}
		}
		return moves, maxStep
	}

	all, _, l := walkList(n, height, n-1, n-1)
	down, up := all[:n-1], all[n-1:]

	downMoves, downStep := countMoves(down)
	upMoves, upStep := countMoves(up)

	if downMoves != upMoves {
		t.Errorf("the window moved %d times going down but %d going up — the directions do not behave alike", downMoves, upMoves)
	}
	if want := n - height; downMoves != want {
		t.Errorf("going down moved the window %d times, want %d", downMoves, want)
	}
	if downStep > 1 || upStep > 1 {
		t.Errorf("the window jumped (down step %d, up step %d); every scroll must be one row", downStep, upStep)
	}
	if l.offset != 0 {
		t.Errorf("the round trip ended at offset %d, want back at 0", l.offset)
	}
}

// The property the other tests could NOT see, and the one that was actually
// wrong: arriving at the bottom and turning around, the cursor must travel back
// UP through the window before the list moves at all.
//
// A rule that simply assigns `offset = cursor - (height-1-margin)` satisfies
// every other test here — same number of moves, same first move, same margins —
// and still feels wrong, because it pins the cursor to one screen row and drags
// the list under it in both directions. This is the test that rejects it.
func TestTurningAroundMovesTheCursorBeforeTheList(t *testing.T) {
	const n, height = 60, 20

	// Walk to the bottom, then turn around.
	offsets, cursors, _ := walkList(n, height, n-1, n-1)
	up := offsets[n-1:]
	upCursors := cursors[n-1:]
	atBottom := offsets[n-2]

	still := 0
	for i := range up {
		if up[i] != atBottom {
			break
		}
		still++
	}

	// From the bottom row the cursor has `height-1` rows of window above it
	// and must keep `margin` of them, so it can climb height-1-margin steps
	// before the window has to move.
	want := height - 1 - listScrollMargin
	if still != want {
		t.Errorf("turning around at the bottom, the list moved after %d upward steps, want %d\n"+
			"  (cursor went %d -> %d while the window %s)",
			still, want, upCursors[0], upCursors[min(still, len(upCursors)-1)],
			map[bool]string{true: "held still", false: "was dragged"}[still > 0])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestWindowHandlesDegenerateSizes(t *testing.T) {
	l := &fuzzyList[int]{matches: []int{1, 2, 3}}
	for _, height := range []int{0, -1} {
		if rows, off := l.windowAt(0, height); rows != nil || off != 0 {
			t.Errorf("height %d gave %v/%d, want nothing", height, rows, off)
		}
	}
	empty := &fuzzyList[int]{}
	if rows, off := empty.windowAt(0, 5); rows != nil || off != 0 {
		t.Errorf("an empty list gave %v/%d, want nothing", rows, off)
	}
	// A window taller than the list never scrolls.
	short := &fuzzyList[int]{matches: []int{1, 2, 3}}
	short.scrollTo(2, 10)
	if short.offset != 0 {
		t.Errorf("a list shorter than the window scrolled to %d", short.offset)
	}
}

// Whatever moved the cursor or the window — a filter re-anchoring the window
// at the top, a terminal that shrank — the window drawn contains the cursor's
// row, because that is the row Enter acts on. Drawing still stores nothing.
func TestWindowKeepsTheCursorOnScreen(t *testing.T) {
	l := &fuzzyList[int]{matches: make([]int, 40)}
	l.scrollTo(30, 10)
	l.offset = 0 // what setMatches does to the window after a filter
	rows, off := l.windowAt(30, 10)
	if 30 < off || 30 >= off+len(rows) {
		t.Errorf("window %d..%d does not hold the cursor at 30", off, off+len(rows)-1)
	}
	if l.offset != 0 {
		t.Errorf("drawing stored offset %d", l.offset)
	}
	l.offset = 25 // a window laid out for a taller terminal
	rows, off = l.windowAt(10, 5)
	if 10 < off || 10 >= off+len(rows) {
		t.Errorf("window %d..%d does not hold the cursor at 10", off, off+len(rows)-1)
	}
	// A negative cursor is "not in this list" (the aspect chooser's cursor
	// up in its family block): the stored window stands.
	l.offset = 12
	if _, off := l.windowAt(-1, 5); off != 12 {
		t.Errorf("a cursor outside the list moved the window to %d", off)
	}
}

// Drawing the same state twice must not move the list. Rendering is not
// navigation.
func TestDrawingDoesNotScroll(t *testing.T) {
	items := make([]int, 40)
	l := &fuzzyList[int]{matches: items}
	l.scrollTo(30, 10)
	first := l.offset
	for i := 0; i < 5; i++ {
		l.windowAt(30, 10)
	}
	if l.offset != first {
		t.Errorf("drawing moved the window from %d to %d", first, l.offset)
	}
}

// --- the aspect chooser, rendered -----------------------------------------

// bigRegistry is longer than any window the modal gives its list, so walking it
// is guaranteed to scroll.
func bigRegistry(n int) []aspect.RegistryAspect {
	reg := make([]aspect.RegistryAspect, 0, n)
	for i := 1; i <= n; i++ {
		reg = append(reg, aspect.RegistryAspect{
			Index:  i,
			Name:   fmt.Sprintf("Aspect%02d", i),
			Abbrev: fmt.Sprintf("A%02d", i),
			Angle:  float64(i),
			Orb:    1,
		})
	}
	return reg
}

// Walk the cursor down the whole chooser and back up. At every single step the
// marker must be on screen exactly once — never zero (vanished) and never twice
// (a stale marker left in the family block).
func TestAspectChooserCursorStaysVisibleWhileWalking(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"132x40", 132, 40}} {
		t.Run(size.name, func(t *testing.T) {
			st := openAspectModal(bigRegistry(40), recipe.AspectPolicy{})
			cfg := Config{Accessible: true, Color: ColorNever}
			total := st.totalRows()
			// Scroll for the window this size actually draws. Passing a
			// height that does not match the render is the precise hazard
			// modalListHeight exists to remove, and an earlier version of
			// this test walked into it: it scrolled for 132x40's 18-row
			// window while rendering 80x24's 2-row one, and the cursor was
			// legitimately off screen.
			listH := aspectListHeight(size.h)

			for step := 0; step < total-1; step++ {
				st, _ = updateAspectModalKey(st, tea.KeyMsg{Type: tea.KeyDown}, listH)
				frame := renderAspectModal(st, cfg, size.w, size.h)
				if got := countCursors(frame); got != 1 {
					t.Fatalf("after %d downs (cursor %d of %d): %d cursor markers on screen, want exactly 1",
						step+1, st.cursor, total, got)
				}
			}
			for step := 0; step < total-1; step++ {
				st, _ = updateAspectModalKey(st, tea.KeyMsg{Type: tea.KeyUp}, listH)
				frame := renderAspectModal(st, cfg, size.w, size.h)
				if got := countCursors(frame); got != 1 {
					t.Fatalf("after %d ups (cursor %d): %d cursor markers on screen, want exactly 1",
						step+1, st.cursor, got)
				}
			}
			if st.cursor != 0 {
				t.Errorf("walking back up left the cursor at %d, want 0", st.cursor)
			}
		})
	}
}

// The cursor cannot leave the rows that exist, in either direction.
func TestAspectChooserCursorCannotLeaveTheList(t *testing.T) {
	st := openAspectModal(bigRegistry(12), recipe.AspectPolicy{})
	for i := 0; i < 200; i++ {
		st, _ = updateAspectModalKey(st, tea.KeyMsg{Type: tea.KeyDown}, testListHeight)
	}
	if st.cursor != st.totalRows()-1 {
		t.Errorf("cursor ran to %d, want it pinned at the last row %d", st.cursor, st.totalRows()-1)
	}
	for i := 0; i < 200; i++ {
		st, _ = updateAspectModalKey(st, tea.KeyMsg{Type: tea.KeyUp}, testListHeight)
	}
	if st.cursor != 0 {
		t.Errorf("cursor ran to %d, want it pinned at 0", st.cursor)
	}
}

// Filtering to a shorter list must not leave the cursor pointing past the end.
func TestAspectChooserCursorSurvivesFiltering(t *testing.T) {
	st := openAspectModal(bigRegistry(40), recipe.AspectPolicy{})
	for i := 0; i < 30; i++ {
		st, _ = updateAspectModalKey(st, tea.KeyMsg{Type: tea.KeyDown}, testListHeight)
	}
	st, _ = updateAspectModalKey(st, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Aspect01")}, testListHeight)

	if st.cursor >= st.totalRows() {
		t.Fatalf("after filtering, cursor %d is past the %d rows that exist", st.cursor, st.totalRows())
	}
	frame := renderAspectModal(st, Config{Accessible: true, Color: ColorNever}, 132, 40)
	if got := countCursors(frame); got != 1 {
		t.Errorf("%d cursor markers after filtering, want exactly 1", got)
	}

	// An empty result parks the cursor in the family block rather than
	// nowhere, so the next keystroke still has somewhere to be.
	st, _ = updateAspectModalKey(st, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzzz")}, testListHeight)
	if st.cursor >= st.totalRows() {
		t.Errorf("cursor %d past %d rows on an empty result", st.cursor, st.totalRows())
	}
}

// --- the atlas picker, rendered -------------------------------------------

// The same chassis, the same promise: the atlas picker had this bug too.
func TestAtlasCursorStaysVisibleWhileWalking(t *testing.T) {
	places := make([]location.PlaceInfo, 0, 40)
	for i := 0; i < 40; i++ {
		places = append(places, location.PlaceInfo{Name: fmt.Sprintf("Town%02d", i), Country: "XX", TZ: "UTC"})
	}
	st := atlasModalState{index: location.NewIndex(places)}
	st.filterText = "town"
	st.refilter()

	cfg := Config{Accessible: true, Color: ColorNever}
	height := modalListHeight(24)
	for cursor := 0; cursor < len(st.matches); cursor++ {
		// Move the way the key handler does: a cursor that is assigned
		// without scrolling is not navigation, and asserting against it
		// would be testing a state the app can never be in.
		st.cursor = cursor
		st.scrollTo(cursor, height)
		frame := renderAtlasModal(st, cfg, 80, 24)
		if got := countCursors(frame); got != 1 {
			t.Fatalf("atlas cursor %d of %d: %d markers on screen, want exactly 1", cursor, len(st.matches), got)
		}
	}
}

// testListHeight is the window the aspect chooser's list gets at 132x40 — the
// state-machine tests scroll against a real height rather than a made-up one.
var testListHeight = aspectListHeight(40)

// --- the body selector, rendered ------------------------------------------

// bigObjectRegistry builds n objects with distinct names, shaped like the real
// read: 0-based numbering, so a walk covers the row the enum cannot name.
func bigObjectRegistry(n int) []aspect.RegistryObject {
	reg := make([]aspect.RegistryObject, 0, n)
	for i := 0; i < n; i++ {
		reg = append(reg, aspect.RegistryObject{Number: i, Name: fmt.Sprintf("Object%02d", i)})
	}
	return reg
}

// The third caller of the chassis, held to the same promise. Its list is FLAT —
// no pinned block above — so the cursor indexes the rows directly, which is a
// different arithmetic from the aspect chooser's and therefore a different way
// to get it wrong.
func TestBodySelectorCursorStaysVisibleWhileWalking(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"132x40", 132, 40}} {
		t.Run(size.name, func(t *testing.T) {
			st := openBodyModal(bigObjectRegistry(40), recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
			cfg := Config{Accessible: true, Color: ColorNever}
			total := len(st.matches)
			// Scroll for the window this size actually draws — the hazard
			// modalListHeight exists to remove. The body list subtracts the
			// tag bar, so its height is NOT the aspect chooser's.
			listH := bodyListHeight(size.h)

			for step := 0; step < total-1; step++ {
				st, _ = updateBodyModalKey(st, tea.KeyMsg{Type: tea.KeyDown}, listH)
				frame := renderBodyModal(st, cfg, size.w, size.h)
				if got := countCursors(frame); got != 1 {
					t.Fatalf("after %d downs (cursor %d of %d): %d cursor markers on screen, want exactly 1",
						step+1, st.cursor, total, got)
				}
			}
			for step := 0; step < total-1; step++ {
				st, _ = updateBodyModalKey(st, tea.KeyMsg{Type: tea.KeyUp}, listH)
				frame := renderBodyModal(st, cfg, size.w, size.h)
				if got := countCursors(frame); got != 1 {
					t.Fatalf("after %d ups (cursor %d): %d cursor markers on screen, want exactly 1",
						step+1, st.cursor, got)
				}
			}
			if st.cursor != 0 {
				t.Errorf("walking back up left the cursor at %d, want 0", st.cursor)
			}
		})
	}
}

func TestBodySelectorCursorCannotLeaveTheList(t *testing.T) {
	st := openBodyModal(bigObjectRegistry(12), recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	for i := 0; i < 200; i++ {
		st, _ = updateBodyModalKey(st, tea.KeyMsg{Type: tea.KeyDown}, testListHeight)
	}
	if st.cursor != st.totalRows()-1 {
		t.Errorf("cursor ran to %d, want it pinned at the last row %d", st.cursor, st.totalRows()-1)
	}
	for i := 0; i < 200; i++ {
		st, _ = updateBodyModalKey(st, tea.KeyMsg{Type: tea.KeyUp}, testListHeight)
	}
	if st.cursor != 0 {
		t.Errorf("cursor ran to %d, want it pinned at 0", st.cursor)
	}
}

// Filtering to a shorter list must not leave the cursor past the end — and on
// an EMPTY result the flat list has nowhere to park it, where the aspect
// chooser still has its family block. Zero is the only answer, and the render
// must survive it.
func TestBodySelectorCursorSurvivesFiltering(t *testing.T) {
	st := openBodyModal(bigObjectRegistry(40), recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	for i := 0; i < 30; i++ {
		st, _ = updateBodyModalKey(st, tea.KeyMsg{Type: tea.KeyDown}, testListHeight)
	}
	st, _ = updateBodyModalKey(st, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Object01")}, testListHeight)

	if st.cursor >= st.totalRows() {
		t.Fatalf("after filtering, cursor %d is past the %d rows that exist", st.cursor, st.totalRows())
	}
	frame := renderBodyModal(st, Config{Accessible: true, Color: ColorNever}, 132, 40)
	if got := countCursors(frame); got != 1 {
		t.Errorf("%d cursor markers after filtering, want exactly 1", got)
	}

	st, _ = updateBodyModalKey(st, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzzz")}, testListHeight)
	// The group block never shrinks, so an empty result parks the cursor on
	// the last group row, as in the aspect chooser (the specification 2).
	if st.cursor != groupRows()-1 {
		t.Errorf("cursor %d on an empty result, want the last group row %d", st.cursor, groupRows()-1)
	}
	// The list has no rows, so the one cursor marker is on a group row.
	frame = renderBodyModal(st, Config{Accessible: true, Color: ColorNever}, 132, 40)
	if got := countCursors(frame); got != 1 {
		t.Errorf("%d cursor markers on an empty result, want 1 (on the last group row)", got)
	}
}
