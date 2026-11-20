package lookup

import (
	"context"
	"errors"

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

// NewAspectRegistry is the registry of the rows AspectRegistry read, with the
// aspects whose letters no list can be read with recorded (aspect.AspectRegistry.Refusal).
func NewAspectRegistry(rows []aspect.RegistryAspect) aspect.AspectRegistry {
	return astrolog.NewAspectRegistry(rows)
}

// AspectsRefusal returns the sentence an aspect list shows in place of its rows
// when err is the refusal of a list whose aspects cannot be read (a rename to
// letters the lists do not carry), and whether err is one.
func AspectsRefusal(err error) (string, bool) {
	var refused *astrolog.AspectsRefusedError
	if errors.As(err, &refused) {
		return refused.Text, true
	}
	return "", false
}
