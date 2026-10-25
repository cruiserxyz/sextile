package ui

import (
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// cant_start.go is the whole-screen answer to "Astrolog is unusable"
// (the specification). Three signals feed it — the startup
// probe (binary), a compute that came back with every body at 0° Aries
// (ephemeris), and a failed atlas read (atlas) — and each only ever turns on.
// A fix needs a restart, so nothing turns them off.
//
// A fourth reason, a launch the instance lock refused (the specification:
// anything but another window holding it, which starts a second window),
// uses the same box but is never mixed with those three: it is set before the
// first frame by NewLockRefusedModel, that model starts none of the work that
// could raise the others, and it shows its own remedy instead of the Astrolog
// ones.

// cantStartState records which of the three failures have been seen.
type cantStartState struct {
	binary, ephemeris, atlas bool

	// lockReason and lockRemedy are set together, and only, by
	// NewLockRefusedModel. Non-empty means this launch was refused by the
	// instance lock and nothing else about it is true: nothing was read.
	lockReason, lockRemedy string
}

func (s cantStartState) any() bool {
	return s.binary || s.ephemeris || s.atlas || s.locked()
}

// locked reports a launch the instance lock refused.
func (s cantStartState) locked() bool { return s.lockReason != "" }

// NewLockRefusedModel is the whole program of a launch that must not go on
// because the instance lock was refused (err is what store.AcquireInstanceLock
// returned). It shows the can't-start screen with the lock's reason ALONE and
// does nothing else: Init starts no command (no Astrolog probe, no perimeter
// read, no atlas or registry load) and quitting writes no session.
func NewLockRefusedModel(cfg Config, err error) Model {
	m := NewModel(cfg)
	m.cantStart.lockReason, m.cantStart.lockRemedy = lockRefusalText(err)
	return m
}

// lockRefusalText is the reason and the remedy for each way the lock can
// refuse, in the words of the specification: what could not be done, to
// which file, and why, then the one thing to do.
func lockRefusalText(err error) (reason, remedy string) {
	var lerr *store.LockError
	if errors.As(err, &lerr) {
		switch lerr.Kind {
		case store.LockUnsafe:
			return "Couldn't use " + lerr.Path + ": it is not a plain file of its own.", "Move or remove that file, then start Sextile again."
		case store.LockOpen:
			return "Couldn't open " + lerr.Path + oneCause(lerr.Err), "Make that folder writable, then start Sextile again."
		case store.LockRecord:
			return "Couldn't record this window in " + lerr.Path + oneCause(lerr.Err), "Fix that, then start Sextile again."
		}
		return "Couldn't lock " + lerr.Path + oneCause(lerr.Err), "Fix that, then start Sextile again."
	}
	return "Couldn't check for another Sextile window" + oneCause(err), "Fix that, then start Sextile again."
}

// oneCause is ": <cause>." on one line, or just "." when there is no cause.
func oneCause(err error) string {
	if err == nil {
		return "."
	}
	return ": " + closeLine(oneLine(err.Error()), ".")
}

// The three reasons, in the order they are listed.
const (
	cantStartBinary    = "the astrolog binary was not found."
	cantStartEphemeris = "Astrolog's ephemeris is not being read: every body came back at 0° Aries."
	cantStartAtlas     = "atlas.as, Astrolog's city list, could not be read."
)

// noteEphemeris marks the ephemeris piece missing when err is the cause
// lookup.ErrEphemerisMissing, wrapped or not. It is the one seam every handler
// that reports a failure through failureReason calls first: failureReason has
// a value receiver and a string result, so it cannot record anything itself.
func (m Model) noteEphemeris(err error) Model {
	if errors.Is(err, lookup.ErrEphemerisMissing) {
		m.cantStart.ephemeris = true
	}
	return m
}

// cantStartKey is the keyboard while the box is shown: q and Ctrl+C quit,
// every other key (a burst included) does nothing at all.
func (m Model) cantStartKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.ForceQuit) || (msg.Type == tea.KeyRunes && !msg.Paste && string(msg.Runes) == "q") {
		return m.quit()
	}
	return m, nil
}

// reasons lists the failures in the fixed order binary, ephemeris, atlas.
func (s cantStartState) reasons() []string {
	var out []string
	if s.binary {
		out = append(out, cantStartBinary)
	}
	if s.ephemeris {
		out = append(out, cantStartEphemeris)
	}
	if s.atlas {
		out = append(out, cantStartAtlas)
	}
	return out
}

// cantStartView draws the box. The too-small screen wins: below the minimum
// terminal there is no room for the words, and resizing back brings the box
// again.
func (m Model) cantStartView() string {
	if layoutGeometry(m.width, m.height, chartContentLines(m.slots), m.bodiesView().count()) == nil {
		return tooSmallView(m.width, m.height, chartContentLines(m.slots), m.cfg)
	}
	// Roomy padding first; at the minimum terminal it shrinks until the words
	// fit (C8). Words may wrap, padding may not hold the frame hostage.
	pads := [][2]int{{1, 2}, {0, 2}, {0, 1}}
	var box string
	for i, p := range pads {
		box = m.renderCantStart(p[0], p[1])
		if i == len(pads)-1 || (lipgloss.Height(box) <= m.height && lipgloss.Width(box) <= m.width) {
			break
		}
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderCantStart(padV, padH int) string {
	inner := m.width - 2 - 2*padH
	if inner > 76 {
		inner = 76
	}
	const bullet = "  · "
	if m.cantStart.locked() {
		return m.renderLockRefused(padV, padH, inner)
	}
	reasons := m.cantStart.reasons()
	var lines []string
	if len(reasons) == 1 {
		lines = append(lines, wrapHanging("Sextile can't start: "+reasons[0], inner, "")...)
	} else {
		lines = append(lines, "Sextile can't start:")
		for _, r := range reasons {
			lines = append(lines, wrapHanging(bullet+r, inner, strings.Repeat(" ", 4))...)
		}
	}
	lines = append(lines, "", "To fix it, either")
	lines = append(lines, wrapHanging(bullet+"run Sextile from its Docker image, which builds Astrolog for you, or", inner, strings.Repeat(" ", 4))...)
	lines = append(lines, wrapHanging(bullet+"install Astrolog 8.00 as README, Install and run says.", inner, strings.Repeat(" ", 4))...)
	lines = append(lines, "")
	last := "Then start Sextile again."
	const quits = "q quits"
	if gap := inner - lipgloss.Width(last) - lipgloss.Width(quits); gap >= 2 {
		last += strings.Repeat(" ", gap) + quits
	} else {
		last += "  " + quits
	}
	lines = append(lines, last)
	return noticeBoxStyle(m.cfg).Padding(padV, padH).Render(strings.Join(lines, "\n"))
}

// renderLockRefused is the box for a refused lock: the reason alone, then its
// remedy and the quit key. The Astrolog installation lines are not here.
func (m Model) renderLockRefused(padV, padH, inner int) string {
	lines := []string{"Sextile can't start:"}
	lines = append(lines, wrapHanging(m.cantStart.lockReason, inner, "")...)
	lines = append(lines, "")
	remedy := wrapHanging(m.cantStart.lockRemedy, inner, "")
	const quits = "q quits"
	last := remedy[len(remedy)-1]
	if gap := inner - lipgloss.Width(last) - lipgloss.Width(quits); gap >= 2 {
		remedy[len(remedy)-1] = last + strings.Repeat(" ", gap) + quits
		lines = append(lines, remedy...)
	} else {
		lines = append(lines, remedy...)
		lines = append(lines, strings.Repeat(" ", max(0, inner-lipgloss.Width(quits)))+quits)
	}
	return noticeBoxStyle(m.cfg).Padding(padV, padH).Render(strings.Join(lines, "\n"))
}

// wrapHanging breaks text at spaces to at most width cells; continuation lines
// start with indent; leading spaces of text are kept on the first line. A word wider than the line stays whole.
func wrapHanging(text string, width int, indent string) []string {
	var out []string
	cur := ""
	lead := text[:len(text)-len(strings.TrimLeft(text, " "))]
	for _, w := range strings.Fields(text) {
		switch {
		case cur == "":
			cur = lead + w
		case lipgloss.Width(cur)+1+lipgloss.Width(w) <= width:
			cur += " " + w
		default:
			out = append(out, cur)
			cur = indent + w
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
