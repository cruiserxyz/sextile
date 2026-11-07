package ui

import (
	"context"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/preview"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// rowSpawns are the outside calls a row's build or commit makes (an Astrolog
// spawn, the preview server): fields of the row, held by the Model, so a test
// stands in for them on its own Model and no other test sees it. Each row
// fills its own group in defaultRows; the others stay nil.
type rowSpawns struct {
	// Timeline: the search.
	timeline func(ctx context.Context, req lookup.TimelineRequest) (lookup.TimelineResult, error)
	// Template: the body's facts.
	bodyFacts func(ctx context.Context, req lookup.BodyFactsRequest) (lookup.BodyFactsResult, error)
	// Details: the three reports.
	reports detailsSpawns
	// Text: the chart casts.
	cast castSpawns
	// Image: the renders and the preview server.
	wheel wheelSpawns
}

// detailsSpawns are the Details row's three report spawns: the chart's own
// report, the cross body report, and its anchored x anchored form.
type detailsSpawns struct {
	raw      func(ctx context.Context, when time.Time, place location.PlaceInfo, lens lookup.Lens, req lookup.RawReportRequest) (string, string, error)
	cross    func(ctx context.Context, lens lookup.Lens, natal store.AnchorChart, when time.Time, place location.PlaceInfo, object aspect.Object) (string, string, error)
	synastry func(ctx context.Context, lens lookup.Lens, natal store.AnchorChart, when time.Time, place location.PlaceInfo, other store.ReportLabel, object aspect.Object) (string, string, error)
}

// castSpawns are the Text row's lookups. All three frame-producing paths
// (computeChartCmd, computeNatalSingleCmd, computeOverlayCmd) go through them;
// lookup.Probers cannot be the seam, because its func type returns
// payload.RawOutput, which ui may not import.
type castSpawns struct {
	natal    func(ctx context.Context, probers *lookup.Probers, chart store.AnchorChart, lens lookup.Lens) (lookup.Result, error)
	transit  func(ctx context.Context, probers *lookup.Probers, now time.Time, place location.PlaceInfo, lens lookup.Lens) (lookup.Result, error)
	overlay  func(ctx context.Context, lens lookup.Lens, natal store.AnchorChart, when time.Time, place location.PlaceInfo) (lookup.OverlayAspects, error)
	synastry func(ctx context.Context, lens lookup.Lens, natal store.AnchorChart, when time.Time, place location.PlaceInfo, other store.ReportLabel) (lookup.OverlayAspects, error)
	stations func(ctx context.Context, probers *lookup.Probers, positions []aspect.Position, at time.Time, place location.PlaceInfo, lens lookup.Lens) (map[int]lookup.Station, []string, error)
}

// wheelSpawns are the Image row's renders and the browser preview's three
// calls (publish, ask for a viewer, close). Show stands in for the real
// preview.Show, which starts an HTTP server and may launch a browser.
type wheelSpawns struct {
	render        func(ctx context.Context, when time.Time, place location.PlaceInfo, lens lookup.Lens, highlight lookup.WheelHighlight) ([]byte, error)
	renderOverlay func(ctx context.Context, natal store.AnchorChart, when time.Time, place location.PlaceInfo, lens lookup.Lens) ([]byte, error)
	renderRings   func(ctx context.Context, rings []lookup.WheelRing, lens lookup.Lens) ([]byte, []string, error)
	show          func(img []byte) (string, error)
	requestOpen   func()
	closeServer   func() error
}

// defaultRows is the refresh table: the five rows with their real spawns,
// set once in the Model's constructor.
func defaultRows() [numSurfaces]surfaceRow {
	var rows [numSurfaces]surfaceRow
	rows[surfaceDetails] = newDetailsRow()
	rows[surfaceTimeline] = newTimelineRow()
	rows[surfaceTemplate] = newTemplateRow()
	rows[surfaceText] = newTextRow()
	rows[surfaceImage] = newImageRow()
	return rows
}

// rowFor answers the row of surface s; false for a surface that has none.
func (m Model) rowFor(s surface) (surfaceRow, bool) {
	if s < 0 || s >= numSurfaces || m.rows[s].build == nil {
		return surfaceRow{}, false
	}
	return m.rows[s], true
}

func realWheelSpawns() wheelSpawns {
	return wheelSpawns{
		render:        lookup.RenderWheelImage,
		renderOverlay: lookup.RenderOverlayWheelImage,
		renderRings:   lookup.RenderMultiWheelImageCommands,
		show:          preview.Show,
		requestOpen:   preview.RequestOpen,
		closeServer:   preview.Close,
	}
}

func realCastSpawns() castSpawns {
	return castSpawns{
		natal:    lookup.FirstCallNatalPositions,
		transit:  lookup.FirstCallTransits,
		overlay:  lookup.OverlayCrossAspects,
		synastry: lookup.SynastryCrossAspects,
		stations: lookup.StationMarks,
	}
}
