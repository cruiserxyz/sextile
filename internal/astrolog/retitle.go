package astrolog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// retitle.go is the Astrolog half of a chart rename: Astrolog writes a chart's
// record with new labels into a private temp folder, and the bytes come back.
// Reading them back to decide whether they are right is internal/lookup's job,
// and writing them into the charts folder is the store door's.

// RetitleRecord is the chart record of the file src with title and place as
// its labels, as Astrolog writes it: `-i source.as -zi title place =o chart.as`,
// where source.as is the guarded copy of src (GuardChartFile) and both files are
// in a private temp folder outside the charts folder, read back and removed.
// Astrolog never reads src itself, and nothing is written beside it.
func RetitleRecord(ctx context.Context, src, title, place string) ([]byte, error) {
	if err := refuseControlChars("title", title); err != nil {
		return nil, err
	}
	if err := refuseControlChars("place", place); err != nil {
		return nil, err
	}
	if err := CheckLabelLength("title", title); err != nil {
		return nil, err
	}
	if err := CheckLabelLength("place", place); err != nil {
		return nil, err
	}
	dir, err := privateTempDir("sextile-retitle-*")
	if err != nil {
		return nil, fmt.Errorf("astrolog: RetitleRecord: %w", err)
	}
	defer os.RemoveAll(dir)
	if err := Available(); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("astrolog: RetitleRecord: %w", err)
	}
	guarded, _, err := guardChart(string(raw))
	if err != nil {
		return nil, fmt.Errorf("astrolog: RetitleRecord: %w", err)
	}
	const source, name = "source.as", "chart.as"
	if err := os.WriteFile(filepath.Join(dir, source), []byte(guarded), 0o600); err != nil {
		return nil, fmt.Errorf("astrolog: RetitleRecord: %w", err)
	}
	if err := writeRecord(ctx, retitleArgv(source, title, place, name), title, dir, name); err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(dir, name))
}

// retitleArgv is the retitle invocation, kept beside saveArgv.
func retitleArgv(src, title, place, dest string) []string {
	return []string{"-i", src, "-zi", title, place, "=o", dest}
}

// writeRecord runs argv in dir, where Astrolog writes the record into file.
func writeRecord(ctx context.Context, argv []string, name, dir, file string) error {
	if err := Available(); err != nil {
		return err
	}
	out, _, err := spawn(ctx, newExempt("retitle: rewrites a chart RECORD (name, moment, place) and prints no positions or aspects", argv...).inDir(dir))
	if err != nil {
		if errors.Is(err, ErrNotInstalled) {
			return err
		}
		return fmt.Errorf("astrolog could not write %q: %w — %s", name, err, firstLineOf(out))
	}
	fi, err := os.Stat(filepath.Join(dir, file))
	if err != nil || fi.Size() == 0 {
		return fmt.Errorf("astrolog wrote no record for %q: %s", name, firstLineOf(out))
	}
	return nil
}

// RecordLabel is a label as Astrolog writes it into -zi: a double quote
// becomes a single one (measured on 8.00, the specification § Measured
// facts). A read-back is compared against this, not against what was typed.
func RecordLabel(s string) string {
	return strings.ReplaceAll(s, `"`, "'")
}
