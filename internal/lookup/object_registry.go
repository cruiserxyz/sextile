package lookup

import (
	"context"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// ObjectRegistry reads the object set out of the user's own Astrolog binary.
//
// It exists here rather than being called directly by the UI because `ui` may
// not import `internal/astrolog` — the isolation invariant keeps the active
// packages behind this one. The UI indirects through a package var so a test
// can substitute it without spawning a process, exactly as it does for
// AspectRegistry.
func ObjectRegistry(ctx context.Context) ([]aspect.RegistryObject, error) {
	return astrolog.ObjectRegistry(ctx)
}
