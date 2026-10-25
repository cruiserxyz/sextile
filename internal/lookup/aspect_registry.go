package lookup

import (
	"context"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// AspectRegistry reads the aspect set out of the user's own Astrolog binary.
//
// It exists here rather than being called directly by the UI because `ui` may
// not import `internal/astrolog` — the isolation invariant keeps the active
// packages behind this one. The UI indirects through a package var so a test
// can substitute it without spawning a process.
func AspectRegistry(ctx context.Context) ([]aspect.RegistryAspect, error) {
	return astrolog.AspectRegistry(ctx)
}
