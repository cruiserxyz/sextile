package location

import (
	"math"
	"strings"
)

// NearestMaxDeg bounds how far (equirectangular degrees) a coordinate may be
// from the closest atlas city and still match. Beyond it, Nearest reports no
// match — a coordinate in the ocean or a data gap should not borrow a distant
// city's country/timezone. ~1° ≈ 111 km N/S: generous enough for any real
// birthplace to hit a nearby city, tight enough to reject bogus (0,0) coords.
const NearestMaxDeg = 1.0

// Nearest returns the atlas entry closest to (lon, lat) by equirectangular
// distance (longitude scaled by cos(latitude) so the metric is roughly uniform
// across latitudes), and whether one lies within NearestMaxDeg. East and North
// are positive — the same convention as PlaceInfo and astrolog.ChartFile.
//
// It is the seam an earlier change uses to borrow a real Country code + IANA timezone +
// canonical city name for a chart whose precise lat/lon came from a chart file.
func Nearest(entries []PlaceInfo, lon, lat float64) (PlaceInfo, bool) {
	best := closest(entries, lon, lat, NearestMaxDeg, func(PlaceInfo) bool { return true })
	if best < 0 {
		return PlaceInfo{}, false
	}
	return entries[best], true
}

// NamedMaxDeg is how far (one arc-minute, in Nearest's metric) a coordinate may
// sit from an atlas city that the file's own place label names and still
// count as that city: the width of the rounding a saved chart's coordinates
// went through (astrolog records whole arc-minutes).
const NamedMaxDeg = 1.0 / 60.0

// NearestNamed is Nearest for a chart file that says where it was cast. When
// label (the file's -zi place label, e.g. "Berlin, DE") names an atlas city
// by Display(), or by bare Name when the label carries no country, and that
// city lies within NamedMaxDeg of (lon, lat), it wins over a closer neighbour
// ("Mitte" beside "Berlin"). Otherwise, and for an empty label, it is exactly
// Nearest.
//
// The label is what a save wrote, so it is compared as a save writes one: a
// double quote is a single quote there, and a Display over 54 bytes is cut and
// ends in an ellipsis, which then names every city whose Display starts with
// what is left.
func NearestNamed(entries []PlaceInfo, lon, lat float64, label string) (PlaceInfo, bool) {
	if label != "" {
		bare := !strings.Contains(label, ", ")
		prefix, cut := strings.CutSuffix(label, savedEllipsis)
		best := closest(entries, lon, lat, NamedMaxDeg, func(p PlaceInfo) bool {
			display := strings.ReplaceAll(p.Display(), `"`, "'")
			if cut {
				return strings.HasPrefix(display, prefix)
			}
			return display == label || (bare && p.Name == label)
		})
		if best >= 0 {
			return entries[best], true
		}
	}
	return Nearest(entries, lon, lat)
}

// savedEllipsis ends a place label that a save cut to fit Astrolog's labels
// (astrolog.CutLabel, which this package may not import).
const savedEllipsis = "…"

// closest returns the index of the entry accepted by ok that is nearest to
// (lon, lat) and no farther than maxDeg, or -1 when none is. The metric is
// Nearest's: longitude scaled by cos(latitude). ok is asked only of entries
// inside maxDeg, so a label comparison costs nothing for the rest of the atlas.
func closest(entries []PlaceInfo, lon, lat, maxDeg float64, ok func(PlaceInfo) bool) int {
	best := -1
	bestD := maxDeg * maxDeg
	cosLat := math.Cos(lat * math.Pi / 180.0)
	for i := range entries {
		dx := (entries[i].Lon - lon) * cosLat
		dy := entries[i].Lat - lat
		d := dx*dx + dy*dy
		if d > bestD || best >= 0 && d == bestD {
			continue
		}
		if ok(entries[i]) {
			bestD = d
			best = i
		}
	}
	return best
}
