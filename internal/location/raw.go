package location

// RawPlace is the storage-layer view of a partial PlaceInfo. Carries the
// same fields as PlaceInfo plus explicit presence markers for Lat and Lon —
// since (0, 0) is a legitimate equator-prime-meridian point, the zero
// default cannot double as "absent". Readers (preferences.yaml, .env if it
// ever lands) populate RawPlace; Resolve consumes it and returns a
// fully-populated PlaceInfo.
type RawPlace struct {
	Name       string
	Country    string
	TZ         string
	Lat        float64
	Lon        float64
	LatPresent bool
	LonPresent bool
}

// HasCoords reports whether both lat and lon were explicitly supplied.
// One-of-two is treated as invalid by the resolver (warn + fallback).
func (r RawPlace) HasCoords() bool { return r.LatPresent && r.LonPresent }

// Empty reports whether the raw-place carries nothing useful — no name,
// no tz, no coords. Resolver skips empty candidates without warning.
func (r RawPlace) Empty() bool {
	return r.Name == "" && r.TZ == "" && !r.HasCoords()
}

// Candidate names a possible source for resolving a chart's casting place.
// Source is a stable string used in resolution warnings ("stored-location",
// "iana-preseed", etc.) so the log surfaces *why* a particular candidate won.
type Candidate struct {
	Source string
	Place  RawPlace
}

// Resolution is the resolver's verdict for one chain of candidates. Place
// is fully populated (coords + tz + name) when err is nil. Source names
// the candidate that produced the resolved place. Warnings collect every
// transition (candidate skipped, atlas miss, etc.) so the caller can
// surface them to the user. UsedAtlas reports whether atlas lookup ran.
type Resolution struct {
	Place     PlaceInfo
	Source    string
	Warnings  []string
	UsedAtlas bool
}
