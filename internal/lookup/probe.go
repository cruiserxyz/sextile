package lookup

import (
	"context"
	"fmt"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// BodyProbe is what Astrolog said about one body asked for alone
// (the specification C1).
type BodyProbe struct {
	// Drawable is true when Astrolog gave the body a position.
	Drawable bool
	// MissingFiles names the ephemeris files Astrolog reported absent for this
	// call, exactly as it printed them. Empty when Drawable.
	MissingFiles []string
}

// ProbeBody asks Astrolog about ONE object, by registry number, in the chart's
// own moment, place and frame (the lens's houses, zodiac and node mode), and
// reports whether it can draw it. The active object selection is replaced by
// the single object, so no other body can answer for it; the node mode is kept
// because it decides which variant objects 16 and 18 are. The moment carries an
// empty label, so no client text reaches the argv, and a zone that does not
// load is refused here exactly as the chart call refuses it.
//
// The file names are Astrolog's own, read from the call's stderr; Sextile keeps
// no table of which file belongs to which body. A call that gives the body no
// position while naming no missing file is an error, not a verdict: nothing is
// known about why.
func ProbeBody(ctx context.Context, probers *Probers, when time.Time, place location.PlaceInfo, lens Lens, object int) (BodyProbe, error) {
	if !place.HasTimezone() {
		return BodyProbe{}, fmt.Errorf("lookup: ProbeBody: place %q has no IANA timezone", place.Display())
	}
	lens.Objects = lens.Objects.OnlyObject(object)
	spec, err := lens.chartSpecAt(astrolog.SlotJD(when), place, store.ReportLabel{})
	if err != nil {
		return BodyProbe{}, fmt.Errorf("lookup: ProbeBody: %w", err)
	}
	raw, err := probers.positionsOnly()(ctx, spec)
	if err != nil {
		return BodyProbe{}, fmt.Errorf("lookup: ProbeBody: %w", err)
	}
	switch {
	case len(raw.Positions) > 0:
		return BodyProbe{Drawable: true}, nil
	case len(raw.MissingEphemeris) > 0:
		return BodyProbe{MissingFiles: raw.MissingEphemeris}, nil
	}
	return BodyProbe{}, fmt.Errorf("lookup: ProbeBody: object %d has no position and Astrolog named no missing file", object)
}
