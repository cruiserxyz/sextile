// All chart values in this file are constructed synthetic test inputs.
package lookup

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// Two windows save a new chart under one name at the same moment: both pass
// the collision scan, and the exclusive create is what lets only one of them
// write. The second window's whole save lands between the first one's scan
// and its create.
func TestTwoWindowsSavingOneNameThroughTheSave(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	var other SavedChart
	var otherErr error
	var otherBytes []byte
	prev := beforePlace
	t.Cleanup(func() { beforePlace = prev })
	beforePlace = func() {
		beforePlace = prev // the other window's own save runs straight through
		other, otherErr = SaveFloatingChart(context.Background(), dir, "client a", amsterdamMoment(), amsterdamPlace())
		if otherErr == nil {
			otherBytes, otherErr = os.ReadFile(other.Path)
		}
	}
	_, err := SaveFloatingChart(context.Background(), dir, "Client A", amsterdamMoment(), amsterdamPlace())
	if otherErr != nil {
		t.Fatalf("the other window's save: %v", otherErr)
	}
	if !errors.Is(err, store.ErrChartExists) {
		t.Fatalf("the later create: err = %v, want store.ErrChartExists", err)
	}
	got, rerr := os.ReadFile(other.Path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != string(otherBytes) {
		t.Errorf("the refused save changed the other window's chart:\n got: %q\nwant: %q", got, otherBytes)
	}
}
