package astrolog

import (
	"context"
	"reflect"
	"testing"
)

// A host astrolog.as that adds hours to the moment (`-YzO`) or the cusps
// (`-YzC`) cannot move a chart file's moment: the box pins zero both after the
// file is read.
func TestHostTimeAdditionCannotMoveAFileChart(t *testing.T) {
	needAstrolog(t)
	read := func() ChartFile {
		cf, err := ResolveChartFile(context.Background(), writeChartFile(t, "@AI800\n"+readerQB+readerZI))
		if err != nil {
			t.Fatal(err)
		}
		return cf
	}
	useDataDir(t, dataDirWith(t))
	plain := read()
	useDataDir(t, dataDirWith(t, "-YzO 1", "-YzC 1"))
	hostile := read()
	if !reflect.DeepEqual(hostile, plain) {
		t.Errorf("a host -YzO 1 moved the chart:\n host: %+v\n plain: %+v", hostile, plain)
	}
}
