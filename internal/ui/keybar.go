package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// renderKeyBar draws the key bar: the given owner's bar entries, in table
// order, as `Action: key` pairs joined by ` | `, in the muted colour
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
func renderKeyBar(width int, entries []BindingEntry, cfg Config, indicator string) string {
	// The error mark does not yield its space: it stands for something the
	// operator cannot see anywhere else, and unlike a leader or a copy receipt
	// it does not expire. The bar drops unpinned entries to make room for it.
	avail := width - 1
	note := isNotice(indicator)
	if note {
		// A notice keeps its space too, but the pinned entries and the `…`
		// outrank its tail: cut it from the right when they would not fit.
		room := width - 3 - ansi.StringWidth(keyBarLine(0, entries, cfg))
		tail := "…"
		if cfg.useASCII() {
			tail = "..."
		}
		if ansi.StringWidth(indicator) > room {
			if room > ansi.StringWidth(tail) {
				indicator = ansi.Truncate(indicator, room, tail)
			} else {
				indicator = ""
			}
		}
	}
	if isErrorMark(indicator) || note {
		avail -= lipgloss.Width(indicator) + 2
	}
	line := " " + keyBarLine(avail, entries, cfg)

	style := lipgloss.NewStyle().Width(width)
	if cfg.useColor() {
		style = keyBarStyle.Width(width)
	}
	if indicator != "" {
		right := indicator + " "
		if pad := width - lipgloss.Width(line) - lipgloss.Width(right); pad > 0 {
			return style.Render(line + strings.Repeat(" ", pad) + right)
		}
	}
	return style.Render(line)
}

// keyBarSep joins the bar's pairs.
const keyBarSep = " | "

// keyBarLine is the bar's text for this width: every bar entry if they fit,
// otherwise the longest run of unpinned entries from the start of the table
// that fits beside the pinned ones and the `…`.
func keyBarLine(width int, entries []BindingEntry, cfg Config) string {
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
	if cfg.useASCII() {
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
			parts = append(parts, e.Action+": "+e.BarKey)
		}
		line = strings.Join(parts, keyBarSep)
		if ansi.StringWidth(line) <= width {
			break
		}
	}
	return line
}

// keyBarEntries is what the bar shows this frame: the current owner's table,
// as far as it is available now.
func (m Model) keyBarEntries() []BindingEntry {
	entries := m.available(m.ownerBindings(m.currentOwner()))
	for i := range entries {
		if entries[i].ID == BindingAttach {
			entries[i].Action = m.attachLabel()
		}
	}
	return entries
}

// The footer's right-hand slot carries two things, and only ever one at a
// time: a pending leader, or a copy confirmation.

// leaderPendingStyleMode is the V1 taste switch for how an armed leader shows
// itself. Set once here, the way tabChromeDim and scrollThumbGlyph are, so a
// contact sheet can render every treatment.
type leaderPendingStyleMode int

const (
	// leaderPendingBare prints the leader alone: `,`
	leaderPendingBare leaderPendingStyleMode = iota
	// leaderPendingEllipsis prints it waiting: `, …`
	leaderPendingEllipsis
	// leaderPendingBracketed prints it held: `[,]`
	leaderPendingBracketed
)

var leaderPendingStyle = leaderPendingEllipsis

// yankFlashStyleMode is the V4 taste switch for the copy confirmation.
type yankFlashStyleMode int

const (
	// yankFlashPlain: `yanked`
	yankFlashPlain yankFlashStyleMode = iota
	// yankFlashCounted: `yanked 78 chars`
	yankFlashCounted
	// yankFlashArrow: `→ clipboard`
	yankFlashArrow
)

var yankFlashStyle = yankFlashPlain

// leaderPendingText renders an armed leader and whatever has been typed after
// it, in the configured treatment.
func leaderPendingText(leader string, seq []string, cfg Config) string {
	typed := leader
	if len(seq) > 0 {
		typed += " " + showSeq(seq)
	}
	switch leaderPendingStyle {
	case leaderPendingBracketed:
		return "[" + typed + "]"
	case leaderPendingEllipsis:
		waiting := " …"
		if cfg.useASCII() {
			waiting = " ..."
		}
		return typed + waiting
	default:
		return typed
	}
}

// yankFlashText renders the copy confirmation in the configured treatment.
func yankFlashText(chars int, cfg Config) string {
	switch yankFlashStyle {
	case yankFlashCounted:
		return fmt.Sprintf("Copied %d chars", chars)
	case yankFlashArrow:
		arrow := "→"
		if cfg.useASCII() {
			arrow = "->"
		}
		return arrow + " clipboard"
	default:
		return "Copied"
	}
}

// footerIndicator resolves the footer's right-hand slot for one frame. A
// pending leader outranks a copy confirmation: the leader is a live state the
// operator is in the middle of, the confirmation is a receipt for something
// already finished. They can overlap — copy in the console, Esc out, arm a
// leader, all inside yankFlashDuration — and arming also drops the receipt, so
// this ordering is the second of two guards rather than the only one.
func (m Model) footerIndicator() string {
	switch {
	case m.leader.pending:
		return leaderPendingText(m.leaderKey, m.leader.seq, m.cfg)
	case m.yank.active && m.yank.notice != "":
		return m.yank.notice
	case m.yank.active:
		return yankFlashText(m.yank.chars, m.cfg)
	case m.errorUnseen && !m.logOnScreen():
		return errorMarkTextView(m.leaderKey, m.cfg, m.errorKeptView)
	}
	return ""
}

// logOnScreen reports whether this frame's layout has a Log panel.
func (m Model) logOnScreen() bool {
	_, ok := m.geometry()[PanelLog]
	return ok
}

// isErrorMark reports whether a footer indicator is the error mark.
func isErrorMark(indicator string) bool {
	return strings.HasPrefix(ansi.Strip(indicator), "! error ")
}

// errorMarkText is the key bar's mark for an error the operator has not been
// shown: the `!` in the error colour when colours are on, the words muted.
func errorMarkText(leader string, cfg Config) string {
	return errorMarkTextView(leader, cfg, false)
}

// errorMarkTextView is errorMarkText, saying the previous view is still
// valid when kept is set.
func errorMarkTextView(leader string, cfg Config, kept bool) string {
	sep := "·"
	if cfg.useASCII() {
		sep = "-"
	}
	rest := " error " + sep + " "
	if kept {
		rest += "view unchanged " + sep + " "
	}
	rest += leader + "l"
	if !cfg.useColor() {
		return "!" + rest
	}
	return lipgloss.NewStyle().Foreground(errorColour).Render("!") + keyBarStyle.Render(rest)
}
