// All chart values in this file are constructed synthetic test inputs.
package store

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// A save that fails partway through the record leaves the library as it was:
// nothing under the chart's name, and no temp file holding half a record.
func TestPlaceChartFileFailingMidwayLeavesNothing(t *testing.T) {
	for name, write := range map[string]func(io.Writer, []byte) (int, error){
		"write error": func(w io.Writer, b []byte) (int, error) {
			n, _ := w.Write(b[:len(b)/2])
			return n, errors.New("disk full")
		},
		"short write": func(w io.Writer, b []byte) (int, error) {
			return w.Write(b[:len(b)/2])
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			prev := writeChartRecord
			writeChartRecord = write
			t.Cleanup(func() { writeChartRecord = prev })

			_, err := PlaceChartFile(dir, []byte(astrologRecord), "client")
			if err == nil {
				t.Fatal("PlaceChartFile succeeded with a write that failed midway")
			}
			if errors.Is(err, ErrChartExists) {
				t.Fatalf("err = %v, want a write failure, not a taken name", err)
			}
			entries, rerr := os.ReadDir(dir)
			if rerr != nil {
				t.Fatal(rerr)
			}
			for _, e := range entries {
				t.Errorf("a failed save left %s in the charts folder", e.Name())
			}
		})
	}
}

// The record is on disk whole before the chart's name exists: the temp file
// is synced while the name is still free.
func TestPlaceChartFileSyncsTheWholeRecordBeforeItTakesTheName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client"+ChartFileExt)
	synced := 0
	prev := syncTemp
	syncTemp = func(f *os.File) error {
		synced++
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the chart's name existed before the record was synced (lstat: %v)", err)
		}
		if got, err := os.ReadFile(f.Name()); err != nil || string(got) != astrologRecord {
			t.Errorf("synced temp holds %q (%v), want the whole record", got, err)
		}
		return prev(f)
	}
	t.Cleanup(func() { syncTemp = prev })

	if _, err := PlaceChartFile(dir, []byte(astrologRecord), "client"); err != nil {
		t.Fatalf("PlaceChartFile: %v", err)
	}
	if synced != 1 {
		t.Errorf("%d syncs of the temp file, want 1", synced)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != astrologRecord {
		t.Errorf("the chart holds %q (%v), want the record", got, err)
	}
}
