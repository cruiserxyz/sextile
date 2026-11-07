package ui

// adoptWantedReport lands a report the way a commit does, under the key the
// Model asks for now: for a test that needs report text on screen and is not
// about which report it is.
func (m Model) adoptWantedReport(title, body string) Model {
	return m.adoptDetailsReport(title, body, m.detailsWanted().key)
}

// withReportWaiting stamps the report on screen as the wanted subject's: the
// wanted key itself, or, when waiting, an older moment of it, so the view rule
// finds the same subject at another version (faint, owed a replacement).
func (m Model) withReportWaiting(waiting bool) Model {
	k := m.detailsWanted().key
	if waiting {
		switch k.kind {
		case reportFrame:
			k.frame.text.when++
		case reportRaw:
			k.raw.slotWhen++
		}
	}
	m.details.key, m.details.keyed = k, true
	return m
}

// restampFrame takes the frame on screen as the one the Model asks for now: for
// a fixture that changes the scene by hand after committing a hand-built frame
// (or stops before the recompute its key started) and is about something other
// than the view rule. The pixels are the frame's own; only the key moves.
func (m Model) restampFrame() Model {
	k, ok := m.textKeyFor()
	m.shown.key, m.shown.keyed = k, ok
	return m
}
