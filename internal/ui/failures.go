package ui

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// failures.go is where an internal error stops being one.
//
// the design notes asks three things of every line the operator
// reads: no plumbing, a cause in astrology's own words, and something to do
// next. A `%v` of a wrapped Go error gives none of them. It gives
// `chart compute error: lookup: FirstCallTransits: astrolog positio` — cut off
// by the six-row Log at the exact point where the reason would have been.
//
// Two things make the replacement harder than picking better words.
//
// LENGTH is part of the contract, not a nicety: a sentence that does not FIT
// says as little as the dump it replaced. Every line below is written to the
// Log's inner width, and TestFailureLinesFitTheLog renders a real frame to hold
// it there.
//
// And a line may only promise what the log can actually deliver. The wrapped
// cause is not thrown away — the Astrolog command that failed is on the argv
// lane beside the line, in the console where there is room to read
// it — but ONLY two surfaces record one. A wheel render, a wheel publish and a
// chart-file resolve record nothing, so a line that sent the operator to the
// console for them would send them to look at some earlier command and draw the
// wrong conclusion. Each site therefore supplies its own fallback, and the two
// that can name the console ask first whether this particular frame recorded
// anything.

// failureReason is the clause that follows a site's own subject.
//
// The named causes are the ones worth naming everywhere: Astrolog missing,
// which is what a fresh machine hits first, and a cancellation nobody asked
// for. Everything else — a refused argv, an unparsable chart, a file that
// vanished mid-run — is not diagnosable from a sentence, so the site's own
// fallback says the truest thing it can about ITS operation.
func (m Model) failureReason(err error, fallback string) string {
	var unresolved *unresolvedHouses
	switch {
	case err == nil:
		return "Nothing came back."
	case errors.As(err, &unresolved):
		return unresolved.reason()
	case errors.Is(err, lookup.ErrAstrologNotInstalled):
		return "Astrolog is not installed."
	case len(warningTexts(err)) > 0:
		// A chart call Astrolog warned on is refused, not shown. The reason is
		// Astrolog's own first line; the rest is in the console (logWarnings).
		return "Astrolog warned: " + warningTexts(err)[0]
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// The refresh coordinator drops superseded runs before they reach a
		// log line, so arriving here means a cancellation nobody expected.
		return "It stopped before it finished."
	default:
		return fallback
	}
}

// consolePointer is the fallback for the two surfaces that record the command
// they ran — the chart compute and the Details report. It is the best clause in
// the file, because the command IS the reason.
//
// recorded is whether THIS frame carried one. A compute whose very first spawn
// failed to start never got a command to record, and pointing at the console
// then would show the operator the last successful frame's command and let them
// diagnose the wrong thing.
//
// The key comes from the CONFIGURED leader rather than from a literal, because
// the leader is user-writable (preferences.yaml `keys:`) and a line naming the
// wrong key is worse than one naming none.
func (m Model) consolePointer(recorded bool) string {
	if !recorded {
		return "Astrolog could not be started."
	}
	if m.leaderKey == "" {
		return "Astrolog returned nothing usable."
	}
	return "Press " + m.leaderKey + "l for what Astrolog ran."
}

// The per-site fallbacks for the surfaces that record no command. Each says
// what its own operation could not do and blames nothing it cannot see: a
// wheel that rendered and then failed to reach a browser is not Astrolog's
// failure, and saying so would send the operator to check the wrong thing.
const (
	pickFallback         = "Astrolog could not read the file."
	wheelRenderFallback  = "Astrolog could not render it."
	wheelPublishFallback = "The image could not be opened."
)

// The startup probe's line (applyAstrologProbe) is the only place that says
// how to fix a missing Astrolog, which is what lets every failure above stay
// short enough to be read. It and the missing atlas line are written as
// literals at their call sites, kept by convention (the invariant was retired
// 2026-10-03).

// atlasModalHint is the same fact inside the place picker, where it explains an
// empty list rather than a failed load. It names the FILE, because that is the
// thing to go and find; the paths searched are in the README.
const atlasModalHint = "(no places to search — Astrolog's atlas.as was not found)"

// warningTexts are the warning lines of a refused Astrolog call, each with its
// terminal control sequences and control bytes removed and empty results
// dropped. Astrolog's text reaches the Log and the console only through here.
func warningTexts(err error) []string {
	var out []string
	for _, l := range lookup.AstrologWarnings(err) {
		l = strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, ansi.Strip(l))
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
