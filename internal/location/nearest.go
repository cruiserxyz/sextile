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
func Nearest(entries []PlaceInfo, lon, lat float64) (PlaceInfo, bool) {
	best := closest(entries, lon, lat, NearestMaxDeg)
	if best < 0 {
		return PlaceInfo{}, false
	}
	return entries[best], true
}

// closest returns the index of the entry nearest to (lon, lat) and no
// farther than maxDeg, or -1 when none is. The metric is Nearest's: longitude
// scaled by cos(latitude).
func closest(entries []PlaceInfo, lon, lat, maxDeg float64) int {
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
		bestD = d
		best = i
	}
	return best
}

// NearestIn is Nearest over the atlas index the program keeps.
func NearestIn(index []Indexed, lon, lat float64) (PlaceInfo, bool) {
	entries := make([]PlaceInfo, len(index))
	for i := range index {
		entries[i] = index[i].Place
	}
	return Nearest(entries, lon, lat)
}

// ConfirmsNear says whether the atlas holds a city that the place words name
// within NearestMaxDeg of (lon, lat). The words' city is what stands before
// their first comma ("Amsterdam, Netherlands", "Edinburgh, GB"), compared with
// the atlas name regardless of case. It is a witness for what the screen
// shows beside the words, never a source of a position or a clock.
func ConfirmsNear(index []Indexed, words string, lon, lat float64) bool {
	city, _, _ := strings.Cut(words, ",")
	city = strings.Join(strings.Fields(city), " ")
	if city == "" {
		return false
	}
	cosLat := math.Cos(lat * math.Pi / 180.0)
	for i := range index {
		p := index[i].Place
		if !strings.EqualFold(p.Name, city) {
			continue
		}
		dx := (p.Lon - lon) * cosLat
		dy := p.Lat - lat
		if dx*dx+dy*dy <= NearestMaxDeg*NearestMaxDeg {
			return true
		}
	}
	return false
}
