package astrolog

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"unicode"
)

// ChartToSave is one birth record about to be written: the moment as civil
// time in the zone it was lived in, the coordinates, and the two labels
// astrolog keeps in -zi.
//
// When carries its own zone, and that zone's offset AT THAT INSTANT is what
// reaches the file. This type holds no separate offset field on purpose: two
// fields that can disagree about the same moment is the shape that puts a
// chart an hour out twice a year.
type ChartToSave struct {
	Name  string
	Place string
	When  time.Time
	Lon   float64
	Lat   float64
}

// ChartRecord spawns astrolog to compose a chart-info record for c and returns
// the bytes astrolog wrote. It creates a temporary directory, runs there, and
// removes it before returning whatever the outcome; nothing is left behind and
// nothing is written where the operator's charts live. Placing the record is
// internal/store's job.
//
// Astrolog writes the record rather than Sextile formatting one, which is the
// whole point: the dialect, the field order and the rounding are then astrolog's
// own and cannot drift from what astrolog reads back.
func ChartRecord(ctx context.Context, c ChartToSave) ([]byte, error) {
	// A label carrying a control character is refused before anything is
	// spawned. The record astrolog writes is LINE-BASED: a newline inside the
	// name splits the -zi line in two, and astrolog then refuses to read its
	// own file back --
	//
	//	$ astrolog -i split.as -w
	//	Astrolog: Too few parameters to switch -zi (1 given, 2 required)
	//
	// -- so the chart is written, LISTS in the picker (the header parser reads
	// it with a regex and is untroubled), and cannot be opened, ever. That is
	// worse than a refusal and worse than truncation, because it looks like a
	// saved chart until the moment someone needs it. Verified against astrolog
	// 8.00 on 2026-09-20.
	//
	// It is checked HERE rather than only at the perimeter because both this
	// function and lookup.SaveFloatingChart are exported, and this is the one
	// they both pass through.
	if err := refuseControlChars("name", c.Name); err != nil {
		return nil, err
	}
	if err := refuseControlChars("place", c.Place); err != nil {
		return nil, err
	}
	// The same defence for length: Astrolog writes a name of any size and then
	// segfaults reading it back (the specification). Nothing is spawned.
	if err := CheckLabelLength("name", c.Name); err != nil {
		return nil, err
	}
	if err := CheckLabelLength("place", c.Place); err != nil {
		return nil, err
	}
	if err := Available(); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "sextile-save-*")
	if err != nil {
		return nil, fmt.Errorf("astrolog: ChartRecord: %w", err)
	}
	defer os.RemoveAll(dir)

	// chart.as is named relative to dir, which is also the process working
	// directory, so the path astrolog is handed carries no operator-controlled
	// text at all.
	const name = "chart.as"
	argv := saveArgv(c, name)
	if err := validateArgv(argv); err != nil {
		return nil, err
	}
	out, _, err := spawn(ctx, newExempt("save-chart: writes a chart RECORD (name, moment, place) and prints no positions or aspects", argv...).inDir(dir))
	if err != nil {
		// A binary that vanished between the probe above and this spawn keeps
		// its own identity, because the perimeter answers it with the install
		// sentence rather than with anything about this chart.
		if errors.Is(err, ErrNotInstalled) {
			return nil, err
		}
		// Everything else names the chart that was not saved and carries
		// astrolog's own first line: its usage errors say which parameter it
		// rejected, and that sentence is the whole diagnosis.
		return nil, fmt.Errorf("astrolog could not save %q: %w — %s", c.Name, err, firstLineOf(out))
	}

	// The file is READ BACK rather than the exit code believed. astrolog's
	// -o is a toggle, so an argv that accidentally carries two of them writes
	// nothing and still exits 0 — the save would report success having
	// produced no chart. =o below forces it on; this check is what makes that
	// a fact about the file rather than a fact about the switch we meant to
	// pass.
	record, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("astrolog wrote no chart file for %q: %s", c.Name, firstLineOf(out))
	}
	if len(record) == 0 {
		return nil, fmt.Errorf("astrolog wrote an empty chart file for %q: %s", c.Name, firstLineOf(out))
	}
	return record, nil
}

// saveArgv is the write invocation. The instant crosses into Astrolog as a
// Julian Day; Astrolog, not Go, chooses the calendar label written into the
// record. -zZ then adjusts that instant to the local civil time and effective
// offset before -zi supplies the labels.
func saveArgv(c ChartToSave, dest string) []string {
	// Rounded ONCE, here, and both the JD and zone below read that one value. astrolog
	// records minutes, so a moment carrying seconds has to lose them somewhere;
	// rounding to the nearest minute is at most 30 seconds out where truncating
	// is at most 60. Rounding first also means the zone is the offset in effect
	// at the moment actually written, not at one up to a minute before it.
	when := c.When.Round(time.Minute)
	// A whole minute is usually a repeating binary fraction. If its nearest
	// float lies just below the boundary, Astrolog's minute formatter truncates
	// it into the preceding minute. Move one float toward the chosen minute's
	// interior; this is far below the record's precision and survives decimal
	// serialization without changing the minute or its effective zone.
	jd := math.Nextafter(GregorianToJD(when), math.Inf(1))
	return []string{
		"-qj", strconv.FormatFloat(jd, 'f', -1, 64),
		// -qj starts in zone zero. -zZ, unlike plain -z, both applies the
		// zone and adjusts the civil clock while preserving the instant.
		// Daylight stays 0 and the zone carries the offset IN EFFECT.
		// The alternative — standard offset plus a daylight flag — cannot
		// express a zone whose summer shift is not a whole hour, because
		// astrolog's DT adds exactly one.
		"-zZ", formatSaveZone(when), "0",
		"-zl", formatLon(c.Lon), formatLat(c.Lat),
		"-zi", c.Name, c.Place,
		// =o, never -o: -o is a toggle (ast80src/astrolog.cpp:2214,
		// SwitchF(us.fWriteFile)), so two of them write nothing and exit 0.
		"=o", dest,
	}
}

// formatSaveZone renders the offset in effect at t as astrolog's zone
// parameter, which counts hours WEST of Greenwich as positive where Go counts
// seconds EAST. Probed against astrolog 8.00 on 2026-09-20: "5:00" writes
// "ST 5W", "-2:00" writes "ST 2E", "-5:30" writes "ST 5:30E".
//
// Minutes are the floor: a historical zone whose offset carries seconds is
// written to the minute, which is the precision the rest of the record and
// every fixture in this repository already use.
func formatSaveZone(t time.Time) string {
	_, east := t.Zone()
	west := -east
	sign := ""
	if west < 0 {
		sign, west = "-", -west
	}
	return fmt.Sprintf("%s%d:%02d", sign, west/3600, (west%3600)/60)
}

// ErrLabelHasControlChar means a chart label carried a control character.
// Separate from every other save failure because it is the operator's input
// that is wrong, not the machine's state, and the prompt answers it by asking
// for a different name rather than by reporting a fault.
var ErrLabelHasControlChar = errors.New("astrolog: a chart label may not contain control characters")

// refuseControlChars rejects a label astrolog could not write and read back.
// Tab and newline are the ones a paste actually delivers; the rest are
// refused with them because none of them belongs in a person's name and
// allowing any would need the same argument about the round trip.
func refuseControlChars(what, s string) error {
	for i, r := range s {
		if unicode.IsControl(r) {
			return fmt.Errorf("%w: the %s contains %s at position %d",
				ErrLabelHasControlChar, what, strconv.QuoteRune(r), i)
		}
	}
	return nil
}

// firstLineOf reduces astrolog's combined output to the one line worth putting
// in a refusal. Its usage errors are a single line on stdout; the rest of a
// failed run is noise the operator cannot act on.
func firstLineOf(out string) string {
	for i, r := range out {
		if r == '\n' {
			out = out[:i]
			break
		}
	}
	if out == "" {
		return "it printed nothing"
	}
	return out
}
