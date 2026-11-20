package store

import (
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
)

// AnchorChart is the natal chart a slot is anchored to: the moment and place a
// chart was cast for. Minimal cp3 shape — house code + sidereal flag arrive once
// internal/lookup grows the natal-set lens registry.
type AnchorChart struct {
	When  time.Time
	Place location.PlaceInfo
	// Label is the name and place text Astrolog prints in this chart's report
	// header (the specification). The zero value is the unnamed
	// chart. It is part of the chart's identity wherever charts are cached or
	// compared: two library charts with one moment and place are two charts.
	Label ReportLabel
	// Record is the chart file's text from the read that resolved When: what
	// Astrolog casts for this chart. Empty for the floating slot and an atlas
	// pick, which Astrolog gets as a moment at a place. Like Label, never
	// stored: rebuilt from the file at every pick and restore.
	Record string
}

// ReportLabel is the name and place text a slot's reports carry in Astrolog's
// own header. Both are empty for the floating slot, an atlas pick, an untitled
// chart and a restored chart whose file could not be read; a library chart
// carries them from ONE read of its file, the read that resolved its moment.
// Never stored: it is rebuilt from the file at every pick and restore.
type ReportLabel struct {
	Name  string
	Place string
}

// IsZero reports whether the label is empty (the unnamed chart).
func (l ReportLabel) IsZero() bool { return l == ReportLabel{} }

// IsZero reports whether the record carries no birth moment or no place.
// Callers treat zero as "no natal chart stored". A place may carry no zone: a
// chart file's does not, because its record casts it, and before its file is
// read it is cast at its instant in UT.
func (c AnchorChart) IsZero() bool {
	return c.When.IsZero() || c.Place.Name == ""
}

// ChartClock is how a chart read from its file shows its moment: Astrolog's own
// reading of the file (its centre box), never computed by Sextile. The date is
// in Astrolog's calendar, Julian before 15 October 1582; Zone is the zone in
// Astrolog's words ("LMT", "DT 1E", "ST 5:30E"); Place is the file's own place,
// or its coordinates when the file names none. The zero value means the chart
// has no file reading, and its moment is shown from its Time and place.
type ChartClock struct {
	Year, Month, Day     int
	Hour, Minute, Second int
	Weekday              time.Weekday
	Julian               bool
	ShowSeconds          bool // the file states the moment to the second
	// Zone is the clock's offset from Greenwich as the Chart panel shows it
	// ("+1", "+2 DT", "LMT"), from Astrolog's reading of the file.
	Zone string
	// Place is the file's place words, "" when it states none; Coords are
	// its coordinates in words ("3°12'W 55°57'N"), and Lon and Lat the same
	// in degrees, East and North positive.
	Place    string
	Coords   string
	Lon, Lat float64
	// Houses is the house system Astrolog cast the file in at its place, in
	// Astrolog's words; "" when the file names none. ZodiacSet says the file
	// sets a zodiac, Sidereal and Offset which (the offset from
	// Fagan-Bradley, in degrees).
	Houses string
	// HousesAsked is the system the file asks for where Astrolog cast another
	// (Porphyry for a system it cannot cast at the place), by Astrolog's own
	// reading, in its words; "" when Astrolog cast the one asked.
	HousesAsked string
	ZodiacSet   bool
	Sidereal    bool
	Offset      float64
}

// IsZero reports whether the clock holds no file reading.
func (c ChartClock) IsZero() bool { return c == ChartClock{} }
