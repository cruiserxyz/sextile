package ui

import "github.com/cruiserxyz/sextile/internal/ui/panel"

// resolveReportBag lifts the Details frame's report side into the one value
// panel.RenderDetailsReport reads. The Template tab shares the branch: its
// lines are laid out here for the panel's width, it has no query line, and it
// dims only once it has lines.
func resolveReportBag(frame detailsFrame, innerW, innerH int, cfg Config) panel.ReportBag {
	bag := panel.ReportBag{
		ScrollOffset: frame.state.scrollOffset,
		Dim:          frame.waiting,
		InnerW:       innerW,
		InnerH:       innerH,
		Style:        cfg,
	}
	if innerH <= 0 {
		return bag
	}
	bag.Lines = panel.ReportLines(frame.state.body)
	if frame.notice != "" {
		// The pane has no content of its subject: one faint line, nothing of
		// another subject under the caption (C5, C6).
		bag.Lines, bag.Dim, bag.ScrollOffset = []string{frame.notice}, true, 0
		return bag
	}
	if isTemplateTab(frame.strip.activeID()) {
		bag.Lines = panel.TemplateBodyLines(frame.template, innerW, cfg)
		bag.Dim = frame.waiting && len(frame.template) > 0
		return bag
	}
	if frame.report != nil && frame.report.Line {
		bag.Lines = frame.report.Lines
		bag.Search = panel.ReportSearch{Show: true, Open: frame.search.Open, Query: frame.search.Query, Shown: frame.report.Shown, Total: frame.search.Total}
	}
	return bag
}
