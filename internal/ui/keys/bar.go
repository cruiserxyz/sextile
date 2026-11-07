package keys

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// RenderBar draws the key bar: the context's bar entries, in table
// order, as `Word: key` pairs joined by ` | `, in the muted colour
// (the specification C3). `indicator` is the right-hand slot: a
// pending leader or a copy confirmation, empty at rest.
//
// When the pairs do not fit, unpinned entries go from the end of the table
// order, every pinned entry stays, and `…` stands as one pair where the
// dropped ones were (C4).
//
// The bindings win the space. Both indicators are transient — a pending leader
// lasts at most timeoutlen, a confirmation yankFlashDuration — so on a
// terminal with no room to spare the bar keeps advertising the keys and the
// indicator simply does not appear, rather than covering `Quit: q`.
func RenderBar(c Context, width int) string {
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
		// A transient message (a pending leader, a copy receipt, a notice)
		// shows whole, in the loading colour, the one colour for what is on
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
			indicator = lipgloss.NewStyle().Foreground(term.LoadingColour).Render(indicator)
		}
		avail -= lipgloss.Width(indicator) + 2
	}
	line := " " + barLine(avail, entries, cfg)
	if indicator != "" && lipgloss.Width(line) > avail+1 {
		line = ansi.Truncate(line, avail+1, tail)
	}

	style := lipgloss.NewStyle().Width(width)
	if cfg.UseColor() {
		style = term.KeyBarStyle.Width(width)
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
// that fits beside the pinned ones and the `…`.
func barLine(width int, entries []BindingEntry, cfg term.Style) string {
	var bar []BindingEntry
	unpinned := 0
	for _, e := range entries {
		if e.Bar {
			bar = append(bar, e)
			if !e.Pinned {
				unpinned++
			}
		}
	}
	more := "…"
	if cfg.UseASCII() {
		more = "..."
	}
	var line string
	for keep := unpinned; keep >= 0; keep-- {
		parts := make([]string, 0, len(bar))
		seen, dropped := 0, false
		for _, e := range bar {
			if !e.Pinned {
				seen++
				if seen > keep {
					if !dropped {
						parts = append(parts, more)
						dropped = true
					}
					continue
				}
			}
			parts = append(parts, e.Word+": "+e.BarKey)
		}
		line = strings.Join(parts, BarSep)
		if ansi.StringWidth(line) <= width {
			break
		}
	}
	return line
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
		e.Word = BarWord(e.Title, implied)
		out[i] = e
	}
	return out
}

// The footer's right-hand slot carries two things, and only ever one at a
// time: a pending leader, or a copy confirmation.

// LeaderPendingStyleMode is the V1 taste switch for how an armed leader shows
// itself. Set once here, the way tabChromeDim and scrollThumbGlyph are, so a
// contact sheet can render every treatment.
type LeaderPendingStyleMode int

const (
	// LeaderPendingBare prints the leader alone: `,`
	LeaderPendingBare LeaderPendingStyleMode = iota
	// LeaderPendingEllipsis prints it waiting: `, …`
	LeaderPendingEllipsis
	// LeaderPendingBracketed prints it held: `[,]`
	LeaderPendingBracketed
)

var LeaderPendingStyle = LeaderPendingEllipsis

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

// LeaderPendingText renders an armed leader and whatever has been typed after
// it, in the configured treatment.
func LeaderPendingText(leader string, seq []string, cfg term.Style) string {
	typed := leader
	if len(seq) > 0 {
		typed += " " + ShowSeq(seq)
	}
	switch LeaderPendingStyle {
	case LeaderPendingBracketed:
		return "[" + typed + "]"
	case LeaderPendingEllipsis:
		waiting := " …"
		if cfg.UseASCII() {
			waiting = " ..."
		}
		return typed + waiting
	default:
		return typed
	}
}

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
	return strings.HasPrefix(ansi.Strip(indicator), "! error ")
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
	rest := " error " + sep + " "
	if kept {
		rest += "view unchanged " + sep + " "
	}
	rest += leader + "l"
	if !cfg.UseColor() {
		return "!" + rest
	}
	return lipgloss.NewStyle().Foreground(term.ErrorColour).Render("!") + term.KeyBarStyle.Render(rest)
}
