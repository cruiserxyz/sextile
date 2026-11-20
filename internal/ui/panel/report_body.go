package panel

import (
	"strings"

	"github.com/cruiserxyz/sextile/internal/templates"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// ViewLines is what the report tab draws: the lines the panel scrolls,
// and, when the query line is there, how many records it keeps of how many.
type ViewLines struct {
	Lines        []string
	Line         bool // the query line takes the panel's last row
	Shown, Total int
}

// ReportSearch is the report's query line: it takes the panel's last row when
// Show is set, with Shown of Total records kept.
type ReportSearch struct {
	Show         bool
	Open         bool
	Query        string
	Shown, Total int
}

// ReportBag is everything RenderDetailsReport draws from, resolved by ui: the
// lines the panel scrolls (a report's, or a body template's laid out for
// InnerW), the query line, the stored scroll offset, and whether the body dims
// while its tab waits. The Template tab shares it.
type ReportBag struct {
	Lines        []string
	Search       ReportSearch
	ScrollOffset int
	Dim          bool
	InnerW       int
	InnerH       int
	Style        term.Style
}

// ReportLines splits a report into the lines the panel scrolls. A
// report's final newline TERMINATES its last line rather than starting an
// empty one — Astrolog ends every report with one — so it is never a row the
// operator can scroll onto. An empty report has no lines at all.
func ReportLines(body string) []string {
	body = strings.TrimSuffix(body, "\n")
	if body == "" {
		return nil
	}
	return strings.Split(body, "\n")
}

// ReportLineCount is the report's line count as the panel renders it.
func ReportLineCount(body string) int {
	return len(ReportLines(body))
}

// TemplateBodyLines lays a body template's lines out in the panel's three
// columns (the specification R1) and dims the aside, the
// body's only second intensity (R8).
func TemplateBodyLines(lines []templates.Line, width int, cfg term.Style) []string {
	laid := templates.Layout(lines, width)
	out := make([]string, 0, len(laid))
	for _, l := range laid {
		if l.Aside != "" && cfg.UseColor() {
			out = append(out, l.Main+term.TemplateAsideStyle.Render(l.Aside))
			continue
		}
		out = append(out, l.Main+l.Aside)
	}
	return out
}

// RenderDetailsReport draws the report tab's body from the panel's scroll
// offset down — the generic panel renderer autoscrolls to the final lines for
// Log, and clipping here keeps Details anchored where the operator scrolled to
// instead; the offset is re-clamped on every render, so a resize that shortens
// the report's tail corrects itself with no keypress (C18).
func RenderDetailsReport(bag ReportBag) (string, pane.Viewport) {
	innerW, innerH, cfg := bag.InnerW, bag.InnerH, bag.Style
	if innerH <= 0 {
		return "", pane.Viewport{}
	}
	lines := bag.Lines
	listH := innerH
	searchRow := ""
	if bag.Search.Show {
		listH = innerH - 1
		searchRow = drawSearchLine(bag.Search.Open, bag.Search.Query, bag.Search.Shown, bag.Search.Total, innerW, cfg)
	}
	scroll := pane.Viewport{
		Total:   len(lines),
		Visible: listH,
		Offset:  pane.ClampOffset(bag.ScrollOffset, len(lines), listH),
	}
	end := scroll.Offset + listH
	if end > len(lines) {
		end = len(lines)
	}
	shownLines := lines[scroll.Offset:end]
	body := strings.Join(shownLines, "\n")
	if bag.Dim {
		// What is on screen is the previous content: faint until the new
		// one commits (the specification rule 6, widened to every
		// Details tab by the specification rule 5).
		body = DimBody(body, cfg)
	}
	if searchRow != "" {
		// The query line is the panel's last row, as on the timeline.
		var pad []string
		if len(shownLines) > 0 {
			pad = strings.Split(body, "\n")
		}
		for len(pad) < listH {
			pad = append(pad, "")
		}
		body = strings.Join(append(pad, searchRow), "\n")
	}
	return body, scroll
}
