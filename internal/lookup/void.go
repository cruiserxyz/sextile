package lookup

import (
	"context"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
)

// The void search (astrolog/void.go) re-stated for the ui, which reaches
// Astrolog only through this package. It is stateless: the ui's memory holds
// the answers (spec L6), never this package.
type (
	VoidRequest = astrolog.VoidRequest
	VoidResult  = astrolog.VoidResult
	VoidKey     = astrolog.VoidKey
	VoidSource  = astrolog.VoidSource
	VoidPeriod  = astrolog.VoidPeriod
	MoonPhase   = astrolog.MoonPhase
)

// VoidKeyFor is the key that names a void search's answer.
func VoidKeyFor(req VoidRequest) VoidKey { return astrolog.VoidKeyFor(req) }

// VoidFirstYear is the first of the void search's two years.
func VoidFirstYear(moment time.Time, offset int) int { return astrolog.VoidFirstYear(moment, offset) }

// RunVoidSearchWith runs the void search through search: the seam a test uses
// to script Astrolog's output without a binary.
func RunVoidSearchWith(ctx context.Context, req VoidRequest, search TimelineSearcher) (VoidResult, error) {
	return astrolog.RunVoidSearch(ctx, req, astrolog.TimelineSearcher(search))
}

// RunVoidSearch runs the real void search.
func RunVoidSearch(ctx context.Context, req VoidRequest) (VoidResult, error) {
	return astrolog.RunVoidSearch(ctx, req, nil)
}
