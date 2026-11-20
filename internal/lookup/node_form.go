package lookup

import (
	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

// CommandNodeForm is the form of the Nodes and Lilith that a command Astrolog
// ran cast, read from its own two switches.
func CommandNodeForm(command string) recipe.NodeForm { return astrolog.CommandNodeForm(command) }
