package location

import "math"

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
	best := -1
	bestD := math.Inf(1)
	cosLat := math.Cos(lat * math.Pi / 180.0)
	for i := range entries {
		dx := (entries[i].Lon - lon) * cosLat
		dy := entries[i].Lat - lat
		d := dx*dx + dy*dy
		if d < bestD {
			bestD = d
			best = i
		}
	}
	if best < 0 || math.Sqrt(bestD) > NearestMaxDeg {
		return PlaceInfo{}, false
	}
	return entries[best], true
}
