package ui

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// refresh-descriptor-details: the Details surface's refresh policy is one row
// value, and the coordinator reaches it only by asking
// (specs/refresh-descriptor-details.md).

// surfaceBranches names every `case <surface>` and `== <surface>` (or `!=`)
// in src, as "func:line", except inside the functions in allowed. surface is
// the constant's name: surfaceDetails, surfaceTimeline.
func surfaceBranches(t *testing.T, name, src, surface string, allowed ...string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		t.Fatal(err)
	}
	isDetails := func(e ast.Expr) bool {
		id, ok := e.(*ast.Ident)
		return ok && id.Name == surface
	}
	var found []string
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		skip := false
		for _, a := range allowed {
			skip = skip || fn.Name.Name == a
		}
		if skip {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			hit := false
			switch x := n.(type) {
			case *ast.CaseClause:
				for _, e := range x.List {
					hit = hit || isDetails(e)
				}
			case *ast.BinaryExpr:
				hit = (x.Op == token.EQL || x.Op == token.NEQ) && (isDetails(x.X) || isDetails(x.Y))
			}
			if hit {
				found = append(found, fn.Name.Name+":"+strconv.Itoa(fset.Position(n.Pos()).Line))
			}
			return true
		})
	}
	return found
}

// The central test. refresh.go holds no Details branch: the coordinator asks
// the row. rowFor alone names the surface.
func TestSurfaceDetailsIsOnlyReachedThroughItsRow(t *testing.T) {
	src, err := os.ReadFile("refresh.go")
	if err != nil {
		t.Fatal(err)
	}
	if got := surfaceBranches(t, "refresh.go", string(src), "surfaceDetails", "rowFor"); len(got) > 0 {
		t.Errorf("refresh.go branches on surfaceDetails outside the row (%d): %s", len(got), strings.Join(got, ", "))
	}
}

// Wrong-name control: the same scanner over a fixture with one planted branch
// names it, and leaves the allowed function alone.
func TestSurfaceBranchScannerNamesAPlantedBranch(t *testing.T) {
	const fixture = `package ui
func rowFor(s surface) bool { return s == surfaceDetails }
func planted(s surface) int {
	switch s {
	case surfaceText:
		return 1
	case surfaceDetails:
		return 2
	}
	return 0
}
func other(s surface) bool { return s == surfaceImage }
`
	got := surfaceBranches(t, "fixture.go", fixture, "surfaceDetails", "rowFor")
	if len(got) != 1 || got[0] != "planted:7" {
		t.Fatalf("scanner reported %v, want [planted:7]", got)
	}
	if all := surfaceBranches(t, "fixture.go", fixture, "surfaceDetails"); len(all) != 2 {
		t.Fatalf("without the allowance the scanner reported %v, want 2", all)
	}
}

// Acceptance 5 (transcript). Dump of one Details run's Log, success and error.
func detailsRunLog(t *testing.T, fail bool) []string {
	t.Helper()
	freshModels(t)
	stubSpawns(t, func(s *rowSpawns) {
		s.reports.raw = func(_ context.Context, _ time.Time, _ location.PlaceInfo, _ lookup.Lens, _ lookup.RawReportRequest) (string, string, error) {
			if fail {
				return "", "astrolog -x", errors.New("astrolog failed")
			}
			return "report 1", "astrolog -x", nil
		}
	})
	m := detailsTestModel(t)
	m.cfg = Config{Color: ColorNever}
	m.focused = PanelBodies
	m.chart.positions = []aspect.Position{{Object: objMercury}, {Object: objVenus}}
	m.bodyCursorIdx = 0
	m, cmd := m.requestDetails()
	m = pump(t, m, cmd)
	var out []string
	for _, l := range m.logLines {
		out = append(out, l.text)
	}
	return out
}

func TestDetailsRunLogTranscript(t *testing.T) {
	cases := []struct {
		fail bool
		want []string
	}{
		{false, []string{"astrolog -x"}},
		{true, []string{"astrolog -x", "details: report not read, Astrolog returned nothing usable"}},
	}
	for _, c := range cases {
		got := detailsRunLog(t, c.fail)
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("fail=%v: Log %q, want %q", c.fail, got, c.want)
		}
	}
}

// Acceptance 6 (all five surfaces). Every surface has a row; the two debounces
// are the old constants.
func TestRowForAnswersForDetailsAndTimeline(t *testing.T) {
	for _, s := range []surface{surfaceImage, surfaceDetails, surfaceTimeline, surfaceTemplate, surfaceText} {
		if _, ok := defaultRowOf(s); !ok {
			t.Errorf("surface %d has no row", s)
		}
	}
	if debounceOf(surfaceImage) != 150*time.Millisecond || debounceOf(surfaceText) != 50*time.Millisecond {
		t.Errorf("image %v, text %v", debounceOf(surfaceImage), debounceOf(surfaceText))
	}
}

// Acceptance 4. The row reads the old constant, and the coordinator reads the row.
func TestDetailsRowKeepsTheDebounce(t *testing.T) {
	r, _ := defaultRowOf(surfaceDetails)
	if r.debounce != 150*time.Millisecond {
		t.Errorf("row debounce %v, want 150ms", r.debounce)
	}
	if debounceOf(surfaceDetails) != r.debounce {
		t.Errorf("debounceFor(Details) %v, row %v", debounceOf(surfaceDetails), r.debounce)
	}
	if r.trigger != triggerEvent || r.supersede != supersedeHard {
		t.Errorf("trigger %v supersede %v", r.trigger, r.supersede)
	}
}

// The row's Inputs column is the key the commit gate compares.
func TestDetailsRowInputsIsTheKeyGate(t *testing.T) {
	m, held := heldMercuryReport(t)
	r, _ := defaultRowOf(surfaceDetails)
	k1, ok1 := r.inputs(m)
	k2, ok2 := m.detailsKeyFor()
	if !ok1 || !ok2 || k1 != k2 {
		t.Fatalf("row inputs %v/%v, detailsKeyFor %v/%v", k1, ok1, k2, ok2)
	}
	m = asCurrent(m, held)
	if !r.accept(m, held) {
		t.Error("accept refused the current question")
	}
	m, _ = tpress(t, m, keyDown)
	m = asCurrent(m, held)
	if r.accept(m, held) {
		t.Error("accept took a result for an older question")
	}
}
