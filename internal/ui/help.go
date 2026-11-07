package ui

import (
	"fmt"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// The Keys popup is drawn by the keys package from the context ui resolves
// for it (keys_context.go); this file holds what only ui can say about it.

// limitedViewNotice says what size shows Details and the Log: the layout's own
// dual-column threshold for the charts shown, at the height the design treats
// as comfortable.
func limitedViewNotice(shown int) string {
	return fmt.Sprintf("Limited view: %dx30 shows Details and the Log", dualColumnMinWidth(shown))
}

// helpLines is the Keys popup's list for this model.
func (m Model) helpLines() []keys.HelpLine {
	return keys.HelpLines(m.keysContextFor(ownerHelp))
}
