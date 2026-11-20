package keys

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// RenderBar draws the key bar: the context's bar entries, in table
// order, as `Word: key` pairs joined by ` | `, the keys spelled as lazygit
// does (FooterKey), the whole row in the terminal's blue. `indicator` is the right-hand slot: a copy
// confirmation, a notice or the error mark, empty at rest. While a leader
// waits the bar is the comma menu instead (MenuLine).
//
// When the pairs do not fit, unpinned entries go from the end of the table
// order, every pinned entry stays, and `…` stands as one pair where the
// dropped ones were (C4).
//
// The bindings win the space. A transient indicator (a copy confirmation
// lasts yankFlashDuration) shows whole, and the bar yields to it.
func RenderBar(c Context, width int) string {
	if c.Leader.Pending {
		return MenuLine(c, width)
	}
	entries, cfg, indicator := BarEntries(c), c.Style, c.Footer
	// The error mark does not yield its space: it stands for something the
	// operator cannot see anywhere else, and unlike a leader or a copy receipt
	// it does not expire. The bar drops unpinned entries to make room for it.
	avail := width - 1
	tail := "…"
	if cfg.UseASCII() {
		tail = "..."
	}
	if indicator != "" {
		// A transient message (a copy receipt, a notice) shows whole, in the notice colour, the one colour for what is on
		// its way or about to expire; the bar yields, its unpinned entries
		// first and then its pinned ones, cut from the right (operator,
		// 2026-11-07: the message "needs to either overlay or modify the
		// footer content so it has enough space", "a definitive coloring").
		// Only a message wider than the row itself is cut. The error mark
		// keeps its own colour.
		if ansi.StringWidth(indicator) > width-3 {
			indicator = ansi.Truncate(indicator, width-3, tail)
		}
		if cfg.UseColor() && !IsErrorMark(indicator) {
			indicator = lipgloss.NewStyle().Foreground(term.NoticeColour).Render(indicator)
		}
		avail -= lipgloss.Width(indicator) + 2
	}
	line := " " + barLine(avail, entries, cfg)
	if indicator != "" && lipgloss.Width(line) > avail+1 {
		line = ansi.Truncate(line, avail+1, tail)
	}

	style := lipgloss.NewStyle().Width(width)
	if cfg.UseColor() {
		style = style.Foreground(term.FooterColour)
	}
	if indicator != "" {
		right := indicator + " "
		if pad := width - lipgloss.Width(line) - lipgloss.Width(right); pad > 0 {
			return style.Render(line + strings.Repeat(" ", pad) + right)
		}
	}
	return style.Render(line)
}

// BarSep joins the bar's pairs.
const BarSep = " | "

// BarLine is the bar's text for this width of the context's entries; barLine
// is the same for entries already resolved.
func BarLine(c Context, width int) string { return barLine(width, BarEntries(c), c.Style) }

// barLine is the bar's text for this width: every bar entry if they fit,
// otherwise the longest run of unpinned entries from the start of the table
// that fits beside the kept and the pinned ones and the `…`.
func barLine(width int, entries []BindingEntry, cfg term.Style) string {
	return fitLine(width, entries, cfg, func(e BindingEntry) string { return e.Word + ": " + FooterKey(e.BarKey) }, BarSep)
}

// fitLine is barLine's one mechanism, with the spelling of an entry and the
// joiner as parameters: the resting bar spells `Word: key` joined by ` | `, the
// comma menu `key word` joined by two spaces.
func fitLine(width int, entries []BindingEntry, cfg term.Style, spell func(BindingEntry) string, sep string) string {
	var bar []BindingEntry
	unpinned := 0
	for _, e := range entries {
		if e.Bar {
			bar = append(bar, e)
			if !e.Pinned && !e.Kept {
				unpinned++
			}
		}
	}
	more := "…"
	if cfg.UseASCII() {
		more = "..."
	}
	// The plain entries give way from the end of the table first; the kept ones
	// only when no plain entry is left. The `…` stands before the pinned ones.
	line := func(keep int, kept bool) string {
		parts := make([]string, 0, len(bar)+1)
		seen, dropped, marked := 0, false, false
		for _, e := range bar {
			switch {
			case e.Pinned:
				if dropped && !marked {
					parts = append(parts, more)
					marked = true
				}
			case e.Kept:
				if !kept {
					dropped = true
					continue
				}
			default:
				seen++
				if seen > keep {
					dropped = true
					continue
				}
			}
			parts = append(parts, spell(e))
		}
		if dropped && !marked {
			parts = append(parts, more)
		}
		return strings.Join(parts, sep)
	}
	var out string
	for _, kept := range []bool{true, false} {
		for keep := unpinned; keep >= 0; keep-- {
			out = line(keep, kept)
			if ansi.StringWidth(out) <= width {
				return out
			}
		}
	}
	return out
}

// BarEntries is what the bar shows this frame: the context's table, as far as
// it is available now, each entry with its bar word.
func BarEntries(c Context) []BindingEntry {
	return WithWords(c, c.Owner, Available(c, c.Table))
}

// ImpliedWords are the words the owner already says, which its bar drops from
// a title: the main screen's rows are charts; a popup, the console and
// Details act on a row of a list, and Details is also what Details is.
func ImpliedWords(o Owner) []string {
	switch o {
	case OwnerMain:
		return []string{"chart"}
	case OwnerDetails:
		return []string{"row", "list", "Details"}
	}
	return []string{"row", "list"}
}

// BarWord is the one shortening rule: the title without its articles (a, an,
// the) and the owner's implied words, in order, never another word.
func BarWord(title string, implied []string) string {
	var kept []string
	for _, w := range strings.Fields(title) {
		if w == "a" || w == "an" || w == "the" || slices.Contains(implied, w) {
			continue
		}
		kept = append(kept, w)
	}
	if len(kept) == 0 {
		return title
	}
	return strings.Join(kept, " ")
}

// WithWords resolves each entry's title for this context and sets its bar word
// for owner o.
func WithWords(c Context, o Owner, entries []BindingEntry) []BindingEntry {
	out := make([]BindingEntry, len(entries))
	implied := ImpliedWords(o)
	for i, e := range entries {
		e.Title = e.TitleFor(c)
		if e.BarWhen != nil && !e.BarWhen(c) {
			e.Bar = false
		}
		if e.BarKeyFn != nil {
			e.BarKey = e.BarKeyFn(c)
		}
		e.Word = BarWord(cmp.Or(e.BarTitle, e.Title), implied)
		out[i] = e
	}
	return out
}

// The footer's right-hand slot carries one thing at a time: a copy
// confirmation, a notice or the error mark. A pending leader has no slot: the
// comma menu takes the bar.

// YankFlashStyleMode is the V4 taste switch for the copy confirmation.
type YankFlashStyleMode int

const (
	// YankFlashPlain: `yanked`
	YankFlashPlain YankFlashStyleMode = iota
	// YankFlashCounted: `yanked 78 chars`
	YankFlashCounted
	// YankFlashArrow: `→ clipboard`
	YankFlashArrow
)

var YankFlashStyle = YankFlashPlain

// YankFlashText renders the copy confirmation in the configured treatment.
func YankFlashText(chars int, cfg term.Style) string {
	switch YankFlashStyle {
	case YankFlashCounted:
		return fmt.Sprintf("Copied %d chars", chars)
	case YankFlashArrow:
		arrow := "→"
		if cfg.UseASCII() {
			arrow = "->"
		}
		return arrow + " clipboard"
	default:
		return "Copied"
	}
}

// IsErrorMark reports whether a footer indicator is the error mark.
func IsErrorMark(indicator string) bool {
	plain := ansi.Strip(indicator)
	return strings.HasPrefix(plain, "! see the Log ") || strings.HasPrefix(plain, "! that failed, ")
}

// ErrorMarkText is the key bar's mark for an error the operator has not been
// shown: the `!` in the error colour when colours are on, the words muted.
func ErrorMarkText(leader string, cfg term.Style) string {
	return ErrorMarkTextView(leader, cfg, false)
}

// ErrorMarkTextView is ErrorMarkText, saying the previous view is still
// valid when kept is set.
func ErrorMarkTextView(leader string, cfg term.Style, kept bool) string {
	sep := "·"
	if cfg.UseASCII() {
		sep = "-"
	}
	rest := " see the Log " + sep + " "
	if kept {
		rest = " that failed, the screen is as before " + sep + " "
	}
	rest += leader + "L"
	if !cfg.UseColor() {
		return "!" + rest
	}
	return lipgloss.NewStyle().Foreground(term.ErrorColour).Render("!") + term.KeyBarStyle.Render(rest)
}

// menuSep joins the comma menu's entries.
const menuSep = "  "

// menuSpell is a menu entry key first: `o open`, and `esc` alone. The key is
// bold in both looks; the word is not. With colours on both take the menu's
// colour; the outer line carries none, so a style's reset cannot end it early.
func menuSpell(cfg term.Style) func(BindingEntry) string {
	key := lipgloss.NewStyle().Bold(true)
	word := lipgloss.NewStyle()
	if cfg.UseColor() {
		key = key.Foreground(term.FooterColour)
		word = word.Foreground(term.FooterColour)
	}
	return func(e BindingEntry) string {
		if e.Word == "" {
			return key.Render(FooterKey(e.BarKey))
		}
		return key.Render(FooterKey(e.BarKey)) + " " + word.Render(e.Word)
	}
}

// menuEntries are the comma menu's entries as bar entries: the one-key rows of
// the table in its order, then, on the first line, each group of two-key rows
// by its letter and word (`c defaults…`); after the first key of a group, that
// group's second keys, each named in full; `esc` is last and pinned. Over a
// popup only the commands the popup offers are named, then the comma forms of
// the Ctrl keys it binds.
func menuEntries(typed string, lf LeaderFacts) []BindingEntry {
	var out []BindingEntry
	for _, cmd := range commands {
		if lf.Limited && !slices.Contains(lf.Offered, cmd.ID) {
			continue
		}
		switch {
		case typed == "" && cmd.Then == "":
			out = append(out, BindingEntry{Word: cmd.Word, BarKey: cmd.Letter, Bar: true})
		case typed == "" && cmd.PrefixWord != "":
			out = append(out, BindingEntry{Word: cmd.PrefixWord, BarKey: cmd.Letter, Bar: true})
		case typed != "" && cmd.Then != "" && cmd.Letter == typed:
			out = append(out, BindingEntry{Word: cmd.Word, BarKey: cmd.Then, Bar: true})
		}
	}
	if typed == "" {
		for _, tw := range lf.Twins {
			out = append(out, BindingEntry{Word: tw.Word, BarKey: tw.Letter, Bar: true})
		}
	}
	return append(out, BindingEntry{BarKey: "esc", Bar: true, Pinned: true})
}

// MenuLine is the bar while a leader waits: one line in the notice colour, key
// first with no prefix, the table's one-key rows and `esc`; after the first key
// of a two-key row, that row's second keys and `esc`. It is the resting bar's
// own mechanism (fitLine): unpinned entries drop from the end, `…` stands where
// they went, and the line is never wider than the terminal.
func MenuLine(c Context, width int) string {
	cfg := c.Style
	line := " " + fitLine(width-1, menuEntries(ShowSeq(c.Leader.Seq), c.Leader), cfg, menuSpell(cfg), menuSep)
	return lipgloss.NewStyle().Width(width).Render(line)
}
