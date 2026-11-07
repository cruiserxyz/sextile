package astrolog

import (
	"context"
	"errors"
	"testing"

	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
)

// A probing cast does not refuse on a stderr line that names no file.
func TestProbeCastReturnsStrayLinesAsEvidence(t *testing.T) {
	needAstrolog(t)
	astrodir.ReducedWithoutUranians(t)
	out, files, _, err := probeCast(context.Background(), []int{34})
	if err != nil {
		t.Fatalf("probeCast refused: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("files %v, want none (the line names no file)", files)
	}
	// No row, or a row at the no-data sentinel: either way, no position.
	if ps, err := parsePositions(out, []int{34}, testRegistry(t)); err == nil && len(ps) != 0 {
		t.Errorf("Vulcan has positions without its elements: %v", ps)
	}
}

// Collecting file names alone does not make a call forgiving: only the explicit
// probing marker does. An unrelated warning beside the filename collector
// still refuses; the probing call takes it as evidence.
func TestOnlyTheProbingMarkerNeverRefuses(t *testing.T) {
	warnShim(t, "echo 'CHART'; echo 'another warning' >&2")
	var files []string
	if _, _, err := spawn(context.Background(), testCall(singleChartArgv).reportingMissing(&files)); !errors.Is(err, ErrAstrologWarned) {
		t.Fatalf("reportingMissing alone: err = %v, want ErrAstrologWarned", err)
	}
	if out, _, err := spawn(context.Background(), testCall(singleChartArgv).reportingMissing(&files).probingCall()); err != nil || out != "CHART\n" {
		t.Fatalf("probing call: out %q err %v, want the chart and no error", out, err)
	}
}
