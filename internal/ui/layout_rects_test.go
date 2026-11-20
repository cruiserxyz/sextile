package ui

// rectsAt is the panels' rectangles for a window with the Chart panel in
// focus and Details closed, as the layout draws them: nil below the minimum.
func rectsAt(width, height, charts, shown int) map[PanelID]panelRect {
	l := layoutFor(layoutInput{width: width, height: height, focus: PanelChart, charts: charts, shown: shown})
	if l == nil {
		return nil
	}
	return l.rects
}

// consoleRectAt is the console's rectangle for the same layout.
func consoleRectAt(width, height, charts, shown int) (panelRect, bool) {
	return consoleRectOf(layoutFor(layoutInput{width: width, height: height, focus: PanelChart, charts: charts, shown: shown}), height)
}

// panelAtCell is the panel that holds the cell (x, y) in that layout.
func panelAtCell(width, height, charts, shown, x, y int) PanelID {
	for id, r := range rectsAt(width, height, charts, shown) {
		if x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h {
			return id
		}
	}
	return PanelNone
}
