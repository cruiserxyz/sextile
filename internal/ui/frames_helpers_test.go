package ui

// The tab tests drive the tab's memory the way the keys do. These two helpers
// state a pair as a chosen tab: choosePair is `[`/`]` landing on p while the
// cursor stays where it is; showPair also moves the cursor to the pair's right
// chart (the chart you read) first.

// choosePair writes the base the way cycleTab does for the pair p of the
// current cursor: its left chart.
func choosePair(m Model, p scenePair) Model {
	return m.withBase(p.left)
}

// showPair moves the cursor onto the pair's right chart and makes its left
// chart the base.
func showPair(m Model, p scenePair) Model {
	for i, s := range m.slots.All() {
		if s == p.right {
			m.cursorIdx = i
		}
	}
	m.base = p.left
	return m
}
