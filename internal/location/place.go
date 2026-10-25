package location

// PlaceInfo is the resolved geographic identity of a chart's casting point.
// Lon is east-positive (Greenwich = 0, Berlin ≈ +13.4); Lat is north-positive.
// TZ is an IANA name (e.g. "Europe/Berlin"); empty means the source row had
// no timezone column and no run-length predecessor — chart calc must reject
// such a row.
//
// PlaceInfo is the boundary type between the picker (atlas row → user
// selection) and the chart-compute path. Once an entry is committed, all
// downstream code reads PlaceInfo, not the raw atlas row.
type PlaceInfo struct {
	Name    string  // city name, e.g. "Berlin"
	Country string  // 2-letter code (uppercased ISO; lowercase US-state/CA-province on disk)
	Lat     float64 // decimal degrees, north-positive
	Lon     float64 // decimal degrees, east-positive (note: atlas.as stores east-negative)
	TZ      string  // IANA name, e.g. "Europe/Berlin"
}

// HasTimezone reports whether the entry carries a usable IANA timezone.
// Atlas rows can fall through the run-length-encoding chain to an empty
// string when the file's own first row lacks one; chart compute rejects
// such rows rather than guess.
func (p PlaceInfo) HasTimezone() bool { return p.TZ != "" }

// Display returns "City, CC" for picker rendering. Country is shown after
// the city to match user reading order in fzf-filtered lists ("Berlin, DE"
// scans faster than "DE, Berlin" when the user typed a city name).
func (p PlaceInfo) Display() string {
	if p.Country == "" {
		return p.Name
	}
	return p.Name + ", " + p.Country
}
