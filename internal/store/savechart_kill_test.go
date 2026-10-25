// All chart values in this file are constructed synthetic test inputs.
package store

import (
	"errors"
	"io"
	"os"
	"testing"
)

// killSaveDirEnv names the charts folder TestPlaceChartFileKilledMidWriteHelper
// saves into; it is set only in the process TestSaveKilledMidWriteLeavesTheNameFree
// starts.
const killSaveDirEnv = "SEXTILE_TEST_KILL_SAVE_DIR"

// TestPlaceChartFileKilledMidWriteHelper is the process the kill test SIGKILLs,
// not a test of its own. It writes half the record, says so on stdout, and
// blocks reading stdin, which the parent holds open until it has killed it.
func TestPlaceChartFileKilledMidWriteHelper(t *testing.T) {
	dir := os.Getenv(killSaveDirEnv)
	if dir == "" {
		t.Skip("run by TestSaveKilledMidWriteLeavesTheNameFree")
	}
	writeChartRecord = func(w io.Writer, b []byte) (int, error) {
		n, err := w.Write(b[:len(b)/2])
		if err != nil {
			return n, err
		}
		os.Stdout.WriteString("half written\n")
		io.Copy(io.Discard, os.Stdin)
		return n, errors.New("the parent never killed this process")
	}
	PlaceChartFile(dir, []byte(astrologRecord), "client")
}
