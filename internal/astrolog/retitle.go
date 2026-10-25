package astrolog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// retitle.go is the Astrolog half of the specification item 3: Astrolog
// writes a chart's record whole, into a temp file internal/store created and
// hands in by path. This file never names a path of its own and never writes a
// byte itself; reading the result back to decide whether it is right is
// internal/lookup's job.

// RetitleTo has Astrolog read the chart at src and write it to dest with a new
// title and place label: `-i <src> -zi <title> <place> =o <dest>`.
//
// dest must already exist (store.CreateChartTemp makes it); Astrolog writes
// over it and keeps its mode. =o, never -o, because -o is a toggle (see
// saveArgv). The record is checked to be non-empty afterwards, since a
// toggled-off write exits 0 having written nothing.
func RetitleTo(ctx context.Context, src, title, place, dest string) error {
	if err := refuseControlChars("title", title); err != nil {
		return err
	}
	if err := refuseControlChars("place", place); err != nil {
		return err
	}
	if err := CheckLabelLength("title", title); err != nil {
		return err
	}
	if err := CheckLabelLength("place", place); err != nil {
		return err
	}
	return writeRecord(ctx, retitleArgv(src, title, place, dest), title, dest)
}

// retitleArgv is the retitle invocation, kept beside saveArgv.
func retitleArgv(src, title, place, dest string) []string {
	return []string{"-i", src, "-zi", title, place, "=o", dest}
}

func writeRecord(ctx context.Context, argv []string, name, dest string) error {
	if err := Available(); err != nil {
		return err
	}
	out, _, err := spawn(ctx, newExempt("retitle: rewrites a chart RECORD (name, moment, place) and prints no positions or aspects", argv...))
	if err != nil {
		if errors.Is(err, ErrNotInstalled) {
			return err
		}
		return fmt.Errorf("astrolog could not write %q: %w — %s", name, err, firstLineOf(out))
	}
	fi, err := os.Stat(dest)
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
