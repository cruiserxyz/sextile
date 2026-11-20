package ui

import (
	"context"
	"errors"
	"strconv"
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
	var temp *lookup.PrivateTempError
	var refused *lookup.ChartRefused
	switch {
	case err == nil:
		return "nothing came back"
	case errors.As(err, &unresolved):
		return unresolved.reason()
	case errors.As(err, &refused):
		// A chart file holding a switch that acts outside the chart, or one
		// Sextile cannot vouch for, never reaches Astrolog.
		if refused.Asks != "" {
			// The Log keeps its line short; the chart list's reason names
			// the switch too.
			return "it asks for " + refused.Asks
		}
		if refused.Long {
			return "its title or place is longer than Astrolog can read"
		}
		return "its file holds " + refused.Switch + ", Sextile gives Astrolog only the lines that describe a chart"
	case errors.As(err, &temp):
		// Sextile's own folder, not Astrolog: every chart read, rename and
		// wheel needs it. The system's words go to the large log.
		if errors.Is(err, lookup.ErrHomeNotAbsolute) {
			return "Sextile has no folder for its working files, HOME must be a full path"
		}
		return "Sextile could not make its temp folder in " + temp.Dir
	case errors.Is(err, lookup.ErrAstrologNotInstalled):
		return "Astrolog is not installed"
	case len(warningTexts(err)) > 0 && lookup.NoDataBody(warningTexts(err)[0]) != "":
		// A body whose ephemeris does not reach the date, in the operator's
		// words; Astrolog's own line is in the console (logWarnings).
		return "Astrolog has no data for " + lookup.NoDataBody(warningTexts(err)[0]) + " at this date"
	case len(warningTexts(err)) > 0:
		// A chart call Astrolog warned on is refused, not shown. The reason is
		// Astrolog's own first line; the rest is in the console (logWarnings).
		return "Astrolog warned " + strconv.Quote(warningTexts(err)[0])
	case errors.Is(err, lookup.ErrAstrologCrashed):
		// A named cause, so it carries no console pointer (the pointer is the
		// fallback for a failure that cannot be named, where the command is the
		// reason). The command is in the console all the same.
		return "Astrolog crashed"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// The refresh coordinator drops superseded runs before they reach a
		// log line, so arriving here means a cancellation nobody expected.
		return "it stopped before it finished"
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
		return "Astrolog could not be started"
	}
	return "Astrolog returned nothing usable"
}

// The per-site fallbacks for the surfaces that record no command. Each says
// what its own operation could not do and blames nothing it cannot see: a
// wheel that rendered and then failed to reach a browser is not Astrolog's
// failure, and saying so would send the operator to check the wrong thing.
const (
	pickFallback         = "Astrolog could not read the file"
	wheelRenderFallback  = "Astrolog could not render it"
	wheelPublishFallback = "the picture did not open in an image viewer"
)

// The startup probe's line (applyAstrologProbe) is the only place that says
// how to fix a missing Astrolog, which is what lets every failure above stay
// short enough to be read. It and the missing atlas line are written as
// literals at their call sites, kept by convention (the invariant was retired
// 2026-10-03).

// atlasModalHint is the same fact inside the place picker, where it explains an
// empty list rather than a failed load. It names the FILE, because that is the
// thing to go and find; the paths searched are in internal/location/atlas.go.
const atlasModalHint = "(no list of cities: Astrolog's file atlas.as was not found)"

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

// logFileNotes says once per kind which of a chart file's own lines Astrolog
// does not get, and what Sextile shows instead (moment-design.md, "The file's
// own settings"). A file that asks for what is shown anyway gets no line.
func (m Model) logFileNotes(name string, notes []lookup.FileNote) Model {
	said := map[lookup.FileSwitchClass]bool{}
	for _, n := range notes {
		if said[n.Class] {
			continue
		}
		var line string
		switch n.Class {
		case lookup.SwitchLens:
			line = name + "'s file asks for its own bodies or aspects (" + n.Switch + "), Sextile shows yours"
		default:
			continue
		}
		said[n.Class] = true
		m = m.logNote("chart", "%s", line)
	}
	return m
}

// refusedReason is why a chart file the chart guard refuses does not open, as
// the chart list says it under the cursor (the operator, 2026-11-15: a red !
// and its reason inside the list).
func refusedReason(err error) string {
	var refused *lookup.ChartRefused
	switch {
	case errors.As(err, &refused) && refused.Asks != "":
		return "Its file asks for " + refused.Asks + " (" + refused.Switch + "). Sextile does not show that yet, so it does not open."
	case errors.As(err, &refused) && refused.Long:
		return "Its title or place is longer than Astrolog can read, so it does not open."
	case errors.As(err, &refused):
		return "Its file holds " + refused.Switch + ". Sextile opens only a file that describes a chart and does nothing more."
	}
	return "Its file cannot be read as a chart, so it does not open."
}
