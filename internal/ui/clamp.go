package ui

// clamp bounds v into [lo, hi]. The popups' list height and the panels both bound a size with it.
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
