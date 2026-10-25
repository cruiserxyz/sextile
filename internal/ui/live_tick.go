package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// The minute tick keeps an attached floating chart on the clock. While the
// chart is attached a timer aligned to the next minute boundary writes the
// clock's time into the floating target and asks for the ordinary recompute;
// detached, no timer runs. The Clock on the Model is the only source of time
// (specs/sextile-release/academic/live-now-and-quiet-log.md, L1 item 3).

// nowTickMsg fires at a minute boundary. gen names the timer that was armed
// for it: a tick whose gen is not the latest armed is a timer the operator has
// since detached or re-armed, and is ignored.
type nowTickMsg struct{ gen uint64 }

// untilNextMinute is how long the clock has to run to its next minute boundary,
// always more than zero and at most a minute.
func (m Model) untilNextMinute() time.Duration {
	now := m.clock.Now()
	return now.Truncate(time.Minute).Add(time.Minute).Sub(now)
}

// minuteTimer starts the timer that ends at a minute boundary. It is one
// package-level variable, like yankLine and showWheel, so the test binary can
// put a timer in its place that does not sleep: a test that runs every command
// a flow returns (runAll) would otherwise wait out a wall-clock minute, and
// no test needs to. Production always runs longTimer.
var minuteTimer = longTimer

// armNowTick starts the timer for the next minute boundary, and retires any
// earlier one by number. It starts nothing while the floating chart is
// detached or before a workspace exists. The timer is a long timer, which the
// tests' pump abandons rather than waits out.
func (m Model) armNowTick() (Model, tea.Cmd) {
	if m.slots == nil || m.floatingDetached {
		return m, nil
	}
	m.nowTickGen++
	gen := m.nowTickGen
	return m, minuteTimer(m.untilNextMinute(), func(time.Time) tea.Msg { return nowTickMsg{gen: gen} })
}

// onNowTick moves an attached floating chart to the clock and arms the next
// tick. The move takes the ordinary path of a time change (the target, the
// per-surface debounce, latest wins), so a tick and a key never race: the
// newer want is the one that paints. A tick for a timer that was retired, or
// that fires after the chart was detached, does nothing and arms nothing.
func (m Model) onNowTick(msg nowTickMsg) (Model, tea.Cmd) {
	if msg.gen != m.nowTickGen || m.floatingDetached || m.slots == nil {
		return m, nil
	}
	m, next := m.armNowTick()
	now := m.clock.Now()
	if now.Equal(m.targetFloatingTime) {
		return m, next
	}
	m, cmd := m.setFloatingTarget(now)
	// The want this tick registered: its frame is the tick's, and quiet.
	m.tickWant = m.refresh[surfaceText].wantID
	return m, tea.Batch(next, cmd)
}

// isTickWork reports whether msg is the tick itself or the text frame the tick
// asked for, the two messages whose work is quiet in the Log.
func (m Model) isTickWork(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case nowTickMsg:
		return true
	case FirstFrameReadyMsg:
		return m.isTickFrame(msg.WantID)
	case refreshResultMsg:
		return msg.surface == surfaceText && m.isTickFrame(msg.wantID)
	}
	return false
}

// isTickFrame reports whether a text result carrying wantID answers the want
// the minute tick registered. Wants only grow, so no later want can match.
func (m Model) isTickFrame(wantID uint64) bool {
	return m.tickWant != 0 && wantID == m.tickWant
}

// surfaceWants is every surface's latest want, for hush to compare against.
func (m Model) surfaceWants() [numSurfaces]uint64 {
	var out [numSurfaces]uint64
	for s := range out {
		out[s] = m.refresh[s].wantID
	}
	return out
}

// hush marks as quiet every want a tick's message registered on a surface
// other than the text surface (Details, the wheel, the timeline, a template
// tab): a call made on the tick's account writes no argv line either. A want a
// later input registers is a newer number and logs as usual.
func (m Model) hush(before [numSurfaces]uint64) Model {
	for s := range before {
		if surface(s) != surfaceText && m.refresh[s].wantID != before[s] {
			m.quiet[s] = m.refresh[s].wantID
		}
	}
	return m
}

// logCommandsFor is logCommands for the result of a run on surface s that was
// started for wantID: a quiet want records nothing.
func (m Model) logCommandsFor(s surface, wantID uint64, commands []string) Model {
	if m.isQuiet(s, wantID) {
		return m
	}
	return m.logCommands(commands)
}

// isQuiet reports whether a result for wantID on surface s answers work the
// minute tick started, which writes no Log line of any kind.
func (m Model) isQuiet(s surface, wantID uint64) bool {
	q := m.quiet[s]
	return q != 0 && wantID == q
}

// nowLinePrefix opens the console's one line for the minute tick's calls.
const nowLinePrefix = "now: "

// setNowLine keeps the provenance of a quiet frame: the last command it ran,
// stored whole and shown behind `now: `. There is one such line in the buffer,
// added by the first tick and moved to the bottom by every later one (appendLog),
// so the buffer holds the same number of lines however many ticks have run. A
// frame that ran no command leaves it as it was.
func (m Model) setNowLine(commands []string) Model {
	if len(commands) == 0 {
		return m
	}
	return m.appendLog(laneNow, commands[len(commands)-1])
}
