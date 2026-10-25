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

// IsZero reports whether the record carries no birth moment or no IANA place.
// Callers treat zero as "no natal chart stored".
func (c AnchorChart) IsZero() bool {
	return c.When.IsZero() || c.Place.Name == "" || !c.Place.HasTimezone()
}
