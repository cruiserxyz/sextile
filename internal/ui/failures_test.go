package ui

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// Verifiers for the error-lane pass: a line the operator reads is a sentence
// that fits, names the one cause worth naming, and hands over the command when
// it cannot explain itself.

// errLaneModel is a ready 132x40 workspace — the width where the Log panel
// exists at all — with the Log empty and ready to receive one line.
func errLaneModel(t *testing.T) Model {
	t.Helper()
	m := readyModelWithSlots(t, berlinP)
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
	return sized.(Model)
}

// notInstalled is a failure wrapped the way lookup hands one up from a spawn
// against a machine with no astrolog binary.
func notInstalled() error {
	return fmt.Errorf("lookup: FirstCallTransits: %w", lookup.ErrAstrologNotInstalled)
}

// probeLine and atlasLine are the two whole-line errors with one cause each.
const (
	probeLine = "Couldn't find the Astrolog program. Install it and restart."
	atlasLine = "Couldn't find atlas.as. Put it beside Astrolog and restart."
)

// The regression item 4 names: the six-row Log cut the reason off. Every line
// the error lane can produce has to survive the render intact, so the test
// renders a real frame and looks for the WHOLE sentence in it — no width
// constant to drift from the layout.
func TestFailureLinesFitTheLog(t *testing.T) {
	m := errLaneModel(t)
	lines := []string{
		probeLine,
		atlasLine,
		"Couldn't cast the chart. " + m.failureReason(notInstalled(), m.consolePointer(true)),
		"Couldn't cast the chart. " + m.failureReason(errors.New("astrolog: exit status 1"), m.consolePointer(true)),
		"Couldn't open that chart. " + m.failureReason(notInstalled(), pickFallback),
		"Couldn't open that chart. " + m.failureReason(errors.New("astrolog: exit status 1"), pickFallback),
		"Couldn't draw the wheel. " + m.failureReason(errors.New("astrolog: exit status 1"), wheelRenderFallback),
		"Couldn't show the wheel. " + m.failureReason(errors.New("write failed"), wheelPublishFallback),
		"Couldn't read the full report. " + m.failureReason(errors.New("astrolog: exit status 1"), m.consolePointer(true)),
		"Couldn't cast the chart. " + m.failureReason(errors.New("fork/exec: no such file"), m.consolePointer(false)),
		"Couldn't copy that line to the clipboard.",
	}
	// the refusals, read out of the predicate's own source so this list
	// cannot fall behind it. A hand-kept table here would pass while a sixth
	// clause shipped unmeasured, which is the exact failure the width contract
	// exists to prevent.
	for _, clause := range refusalClauses(t) {
		lines = append(lines, "Couldn't anchor that chart. It has "+clause+".")
	}
	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			got := errLaneModel(t).testError("%s", line).View()
			if !strings.Contains(got, line) {
				t.Errorf("the Log truncated the line; it renders as:\n%s", logRegion(got))
			}
		})
	}
}

// logRegion pulls the rows carrying the Log panel out of a rendered frame, so
// a failure shows what the operator would actually have seen.
func logRegion(view string) string {
	var out []string
	for _, row := range strings.Split(view, "\n") {
		if strings.Contains(row, "Log") || strings.Contains(row, "Couldn't") || strings.Contains(row, "Astrolog") {
			out = append(out, row)
		}
	}
	return strings.Join(out, "\n")
}

// Item 4: the likeliest failure on a machine that has never run Sextile is
// named, once, by the startup probe — and a working install says nothing at
// all, because a tool announcing that its dependencies are present is talking
// about itself.
func TestAstrologProbeNamesAMissingInstall(t *testing.T) {
	m, _ := applyAstrologProbe(errLaneModel(t), AstrologProbedMsg{Err: lookup.ErrAstrologNotInstalled})
	if got := errorLines(m); len(got) != 1 || got[0] != probeLine {
		t.Errorf("probe wrote %v, want exactly [%q]", got, probeLine)
	}
	if !strings.Contains(got0(t, m), "Install it") {
		t.Error("the line does not say what to do about it")
	}

	quiet, _ := applyAstrologProbe(errLaneModel(t), AstrologProbedMsg{})
	if got := quiet.logLines; len(got) != 0 {
		t.Errorf("a working install wrote %v", got)
	}
}

func got0(t *testing.T, m Model) string {
	t.Helper()
	lines := errorLines(m)
	if len(lines) == 0 {
		t.Fatal("no error line")
	}
	return lines[0]
}

func errorLines(m Model) []string {
	var out []string
	for _, l := range m.logLines {
		if l.lane == laneError {
			out = append(out, l.text)
		}
	}
	return out
}

// A missing Astrolog keeps its identity all the way from the spawn to the
// sentence, so the operation that failed can say WHY rather than point at a
// command that never ran.
func TestMissingAstrologIsNamedAtEveryOperation(t *testing.T) {
	m := errLaneModel(t)
	if got, want := m.failureReason(notInstalled(), m.consolePointer(true)), "Astrolog is not installed."; got != want {
		t.Errorf("failureReason = %q, want %q", got, want)
	}
	if strings.Contains(m.failureReason(notInstalled(), m.consolePointer(true)), "Press") {
		t.Error("a cause this specific must not send the operator to the console")
	}
}

// Item 6a: a failed pick used to print its wrapped Go error verbatim. It now
// says what failed and, when it cannot know why, where the command is.
func TestChartPickFailureIsASentence(t *testing.T) {
	m := errLaneModel(t)
	m.pickGen = 1
	m.picker = chartPickerState{prior: startupReady, resolving: true}

	got, _ := applyChartResolved(m, ChartResolvedMsg{
		Chart: testNamedChart("Constructed One", fixtureOneP),
		Err:   fmt.Errorf("astrolog: ResolveChartFile: astrolog exec: %w", lookup.ErrAstrologNotInstalled),
		Gen:   1,
	})
	want := "Couldn't open that chart. Astrolog is not installed."
	if lines := errorLines(got); len(lines) != 1 || lines[0] != want {
		t.Errorf("pick failure wrote %v, want [%q]", errorLines(got), want)
	}
}

// The fallback names the leader the operator actually configured. A line that
// names the wrong key is worse than one that names none, so a workspace with
// no leader at all says nothing about keys.
func TestFailureFallbackNamesTheConfiguredLeader(t *testing.T) {
	m := errLaneModel(t)
	m = m.applyKeyConfig(store.KeysConfig{Leader: ";", LeaderPresent: true})
	if got := m.failureReason(errors.New("astrolog: exit status 1"), m.consolePointer(true)); !strings.Contains(got, ";l") {
		t.Errorf("failureReason = %q, want it to name the configured leader", got)
	}

	m.leaderKey = ""
	if got := m.failureReason(errors.New("astrolog: exit status 1"), m.consolePointer(true)); strings.Contains(got, "Press") {
		t.Errorf("failureReason = %q, want no key named when there is no leader", got)
	}
}

// A line may only promise what the log can deliver. Three surfaces record no
// Astrolog command at all, so none of them may send the operator to the console
// to read one — they would find some earlier frame's command and diagnose the
// wrong failure.
func TestOnlyRecordedCommandsPointAtTheConsole(t *testing.T) {
	m := errLaneModel(t)
	opaque := errors.New("astrolog: exit status 1")

	for name, clause := range map[string]string{
		"chart pick":   m.failureReason(opaque, pickFallback),
		"wheel render": m.failureReason(opaque, wheelRenderFallback),
		"wheel show":   m.failureReason(opaque, wheelPublishFallback),
	} {
		if strings.Contains(clause, "Press") || strings.Contains(clause, "ran") {
			t.Errorf("%s records no command but its line says %q", name, clause)
		}
	}

	// A publish failure happens AFTER Astrolog succeeded; blaming it would send
	// the operator to check an install that is working.
	if got := m.failureReason(opaque, wheelPublishFallback); strings.Contains(got, "Astrolog") {
		t.Errorf("the publish failure blames Astrolog: %q", got)
	}
}

// The compute records a command only once a spawn has started. A failure before
// that has nothing in the console, and says so instead of pointing at the
// previous frame's command. This is the shape a binary that exists but cannot
// execute produces — a shebang naming a missing interpreter, say.
func TestComputeWithNoRecordedCommandDoesNotPointAtTheConsole(t *testing.T) {
	m := errLaneModel(t)
	got, _ := applyTextCommit(m, FirstFrameReadyMsg{
		Err:    errors.New("astrolog exec: fork/exec /usr/bin/astrolog: no such file or directory"),
		RunID:  m.refresh[surfaceText].runningRunID,
		WantID: m.refresh[surfaceText].wantID,
	})
	want := "Couldn't cast the chart. Astrolog could not be started."
	if lines := errorLines(got); len(lines) != 1 || lines[0] != want {
		t.Errorf("compute failure wrote %v, want [%q]", errorLines(got), want)
	}
}

// translators are the functions allowed to carry an error into the error lane.
// The list is a WHITELIST rather than a shape rule, because every shape rule
// has a way around it: `cause := msg.Err.Error()` is neither a bare identifier
// nor an obvious dump, and it puts the wrapped Go error back on screen. Adding
// a name here is a deliberate act; reaching an error any other way is not.
var translators = map[string]bool{
	"failureReason":  true,
	"consolePointer": true,
}

// The structural half of the pass: an error-lane line is composed, never
// dumped. A bare error handed to logErrorf is the shape that produced
// `chart compute error: lookup: FirstCallTransits: astrolog positio`, and no
// amount of care at one call site keeps it from coming back at the next.
//
// The rule is therefore about the whole argument: if the expression mentions an
// error at all, it must BE a call to one of the translators above.
func TestErrorLaneCarriesNoRawError(t *testing.T) {
	fset, files := uiSources(t)
	sites := 0
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "logError" {
				return true
			}
			sites++
			for _, arg := range call.Args[min(1, len(call.Args)):] {
				if translatedCall(arg) || !mentionsError(arg) {
					continue
				}
				pos := fset.Position(arg.Pos())
				t.Errorf("%s:%d reaches an error without a translator; see the design notes and the whitelist above",
					filepath.Base(pos.Filename), pos.Line)
			}
			return true
		})
	}
	if sites == 0 {
		t.Fatal("found no logError call; this test would pass vacuously")
	}
}

// translatedCall reports whether the whole argument is one of the sanctioned
// translators being called. Its own arguments are then free to be errors —
// that is the job.
func translatedCall(arg ast.Expr) bool {
	call, ok := arg.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		return translators[fn.Sel.Name]
	case *ast.Ident:
		return translators[fn.Name]
	default:
		return false
	}
}

// mentionsError reports whether an expression names anything error-shaped
// anywhere inside it — the identifier, the field, or a call to Error().
func mentionsError(arg ast.Expr) bool {
	found := false
	ast.Inspect(arg, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.Ident:
			if looksLikeError(e.Name) {
				found = true
			}
		case *ast.SelectorExpr:
			if looksLikeError(e.Sel.Name) {
				found = true
			}
		}
		return !found
	})
	return found
}

func looksLikeError(name string) bool {
	return strings.Contains(strings.ToLower(name), "err")
}

// refusalClauses returns every clause SessionChart.UnusableReason can return,
// read out of its source rather than transcribed. Totality is the point: the
// width contract has to cover the clauses that EXIST, and a list maintained by
// hand covers the ones somebody remembered.
//
// Three things keep the scan from reporting a clean sweep of a set it never
// read. It pins the RECEIVER, so another type's UnusableReason cannot be
// collected in its place. It refuses any returned expression that is not a
// string literal — a clause behind a const, a concatenation or a helper is not
// missed quietly, it stops the test — because "I could not read it" and "there
// was nothing to read" are the same result to a scanner and opposite results to
// a reader. And it requires exactly one such method, so a moved or duplicated
// predicate is a failure rather than a silent half-answer.
func refusalClauses(t *testing.T) []string {
	t.Helper()
	const src = "../store/session.go"
	f, err := parser.ParseFile(token.NewFileSet(), src, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", src, err)
	}
	var out []string
	found := 0
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "UnusableReason" || !hasReceiver(fn, "SessionChart") {
			continue
		}
		found++
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			ret, ok := n.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			for _, r := range ret.Results {
				lit, ok := r.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Errorf("%s: UnusableReason returns a clause this scan cannot read (%T); "+
						"keep every clause a plain string literal, or the width contract stops covering it", src, r)
					continue
				}
				if clause, err := strconv.Unquote(lit.Value); err == nil && clause != "" {
					out = append(out, clause)
				}
			}
			return true
		})
	}
	if found != 1 {
		t.Fatalf("found %d SessionChart.UnusableReason methods in %s, want exactly 1", found, src)
	}
	if len(out) < 5 {
		t.Fatalf("read %d refusal clauses out of %s, want at least the five that exist", len(out), src)
	}
	return out
}

// hasReceiver reports whether fn is a method on the named type, by value or by
// pointer.
func hasReceiver(fn *ast.FuncDecl, typeName string) bool {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return false
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == typeName
}
