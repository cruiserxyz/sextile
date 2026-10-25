package astrolog

import "regexp"

// The two readers below find a value by searching a text wheel. Production no
// longer does (ResolveChartFile reads the centre box by position,
// the specification C1); the house-table tests still use them to
// read a wheel they cast themselves.

// reWheelHouses is the text wheel's "<Name> houses" line. The chart's own name
// and place print above it, so the LAST match is the header.
var reWheelHouses = regexp.MustCompile(`(?m)\|\s+([^|\s][^|]*?) houses\s+\|`)

// wheelHousesHeader returns Astrolog's house-system name from a text wheel, or
// "" when it printed none.
func wheelHousesHeader(out string) string {
	all := reWheelHouses.FindAllStringSubmatch(out, -1)
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1][1]
}

var reJulianDay = regexp.MustCompile(`Julian Day:\s*([0-9]+\.[0-9]+)`)

// lastZI is the labels of a file's text, as ResolveChartFile and the listing
// read them.
func lastZI(text string) (name, place string) { return scanChartSwitches(text).labels() }
