package ui

import "github.com/cruiserxyz/sextile/internal/ui/keys"

// Helpers the keys tests share. Each routes through the context the keys
// package is given, so a test reads the table the way a frame does.

// withWords is owner o's entries with their titles and bar words for m.
func (m Model) withWords(o owner, entries []keys.BindingEntry) []keys.BindingEntry {
	return keys.WithWords(m.keysFacts(o), o, entries)
}

// titleOf is entry e's title for m (Enter names the tab it would show).
func (m Model) titleOf(e keys.BindingEntry) string {
	return e.TitleFor(m.keysFacts(ownerMain))
}

// helpPopupBlocks is the Keys popup's per-popup blocks for m.
func (m Model) helpPopupBlocks() []keys.HelpBlock {
	return keys.HelpPopupBlocks(m.keysContextFor(ownerHelp))
}

// withoutConditions is a copy of the table whose entries all stand, the bar of
// every key the table has.
func withoutConditions(t []keys.BindingEntry) []keys.BindingEntry {
	out := append([]keys.BindingEntry(nil), t...)
	for i := range out {
		out[i].When = nil
	}
	return out
}

// barText renders owner o's table t as the key bar at width, under m's
// conditions, in cfg, with footer in the right-hand slot.
func (m Model) barText(o owner, t []keys.BindingEntry, width int, cfg Config, footer string) string {
	c := m.keysFacts(o)
	c.Table, c.Style, c.Footer, c.FooterNotice = t, cfg, footer, isNotice(footer)
	return keys.RenderBar(c, width)
}
