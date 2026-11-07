package ui

import "regexp"

// zeroAriesRow matches a position printed at 0 Aries, which is where Astrolog
// puts an object whose ephemeris file is missing.
var zeroAriesRow = regexp.MustCompile(`\b0[°.]?\s?Ari`)
