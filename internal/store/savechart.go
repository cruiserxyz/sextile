package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// ErrChartExists means a chart file of that name is already in the library.
// It is a SEPARATE cause because it is the one failure that is not a fault:
// the operator picked a name that is taken, the refusal is the feature, and
// the perimeter answers it by keeping the prompt open rather than by
// reporting an error.
var ErrChartExists = errors.New("store: a chart of that name is already saved")

// ChartFileExt is the ending of the file name Sextile gives a chart it writes:
// `.as`, Astrolog's own convention for a file in its syntax. It is the ending
// of what is WRITTEN, never a test of what is read: a chart is known by its
// first bytes, whatever it is called.
const ChartFileExt = ".as"

// PlaceChartFile writes record into the charts directory as <slug>.as,
// exactly as handed, and returns the path it created. Not one byte is added:
// a chart's library identity is its file stem (the specification C3.19,
// C3.20), so nothing has to travel inside the record.
//
// It REFUSES an existing file rather than replacing it. That refusal is the
// limit the whole save was authorised under: an earlier change ruled that Sextile writes
// no byte into any chart file, and the 2026-09-20 reversal covers a NEW file
// under a name the operator typed and nothing else. The no-replace move below
// is what makes that a property of the syscall rather than of a check somebody
// could reorder.
//
// The record is written whole into a hidden temp file beside the chart,
// synced, and only then given the chart's name, by a move that refuses an
// existing name (renameNoReplace). A plain rename is not used: it REPLACES,
// silently, which is exactly the motion forbidden here (the architecture
// self-check: branch never overwrite). So the name is either absent or holds
// the whole record: a process killed mid-write leaves at most a dot-named
// temp file, which the library never lists and which owns no name, and the
// next save under that name finds it free. Each temp file has its own random
// name, so a leftover never collides with a later one; none is swept.
//
// The name is not SaveChart or formatNamedChart; TestNoYAMLChartWriter holds
// those two retired in this package, and the retired thing was a YAML chart
// writer. This writes astrolog's own record, which astrolog composed.
func PlaceChartFile(dir string, record []byte, slug string) (string, error) {
	if slug == "" {
		return "", fmt.Errorf("store: PlaceChartFile: empty slug")
	}
	if dir == "" {
		return "", fmt.Errorf("store: PlaceChartFile: no charts directory given")
	}
	// The directory is HANDED IN, resolved once by the caller, and this
	// function never calls ChartsDir() itself. ChartsDir()'s second tier turns
	// on the moment the XDG default directory comes into existence, so two
	// calls a subprocess apart can name two different places: a save would
	// then check one directory for a collision, show the operator a second and
	// write into a third. One resolution per save is what makes "where it said
	// it would go" true by construction rather than by timing.
	//
	// It is still never defaultChartsDir(): creating the XDG default when the
	// resolver did not choose it would flip that tier on for every later call
	// and silently hide a configured dev library. C12 pins that.
	if err := mkdirAllSynced(dir, 0o700); err != nil {
		return "", fmt.Errorf("store: PlaceChartFile: %w", err)
	}
	path := filepath.Join(dir, slug+ChartFileExt)
	f, err := os.CreateTemp(dir, "."+slug+".*"+chartTempSuffix)
	if err != nil {
		return "", fmt.Errorf("store: PlaceChartFile: %w", err)
	}
	tmp := f.Name()
	n, err := writeChartRecord(f, record)
	if err == nil && n != len(record) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = syncTemp(f)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = renameNoReplace(tmp, path)
	}
	if errors.Is(err, errNoAtomicMove) {
		os.Remove(tmp)
		return placeExclusive(path, record)
	}
	if err != nil {
		// Nothing took the name; the temp file is ours, and goes.
		os.Remove(tmp)
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("%w: %s", ErrChartExists, path)
		}
		return "", fmt.Errorf("store: PlaceChartFile: %w", err)
	}
	syncDir(dir)
	return path, nil
}

// placeExclusive is the save on a filesystem with neither RENAME_NOREPLACE nor
// hard links (FAT, exFAT, some FUSE and network mounts). No atomic move that
// refuses an existing name exists there, so the chart's own name is created
// exclusively and the record written into it, as every save did before the
// temp file: still never a replace, but a process killed mid-write can leave a
// partial chart under the name. A failed write removes the file this call
// created.
func placeExclusive(path string, record []byte) (string, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("%w: %s", ErrChartExists, path)
		}
		return "", fmt.Errorf("store: PlaceChartFile: %w", err)
	}
	n, err := writeChartRecord(f, record)
	if err == nil && n != len(record) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = syncTemp(f)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return "", fmt.Errorf("store: PlaceChartFile: %w", err)
	}
	syncDir(filepath.Dir(path))
	return path, nil
}

// Fault-injection seams for PlaceChartFile's tests. Production never
// reassigns them.
var (
	// writeChartRecord writes the record into the new file.
	writeChartRecord = func(w io.Writer, b []byte) (int, error) { return w.Write(b) }
	// syncTemp flushes a new file before it is given, or holds, its final name.
	syncTemp = func(f *os.File) error { return f.Sync() }
	// linkFile is the hard link linkNoReplace makes.
	linkFile = os.Link
)

// errNoAtomicMove means the filesystem has no atomic move that refuses an
// existing name: neither RENAME_NOREPLACE nor hard links.
var errNoAtomicMove = errors.New("no atomic no-replace move on this filesystem")

// linkNoReplace gives from the name to by a hard link, which refuses an
// existing to (os.ErrExist), and then removes from. Once the link is made the
// move has happened, so a failed removal only leaves a hidden temp behind. A
// filesystem that cannot link answers errNoAtomicMove.
func linkNoReplace(from, to string) error {
	if err := linkFile(from, to); err != nil {
		if linkUnsupported(err) {
			return fmt.Errorf("%w: %v", errNoAtomicMove, err)
		}
		return err
	}
	os.Remove(from)
	return nil
}

// linkUnsupported reports whether a failed link means the filesystem does not
// do hard links (here), rather than that the link itself failed. EMLINK is not
// in the set: it means this inode has too many links, which a link-capable
// filesystem answers, and must not weaken the save to the in-place path.
func linkUnsupported(err error) bool {
	for _, e := range []syscall.Errno{
		syscall.EPERM, syscall.EOPNOTSUPP, syscall.ENOTSUP, syscall.ENOSYS,
		syscall.EXDEV, syscall.EINVAL,
	} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
