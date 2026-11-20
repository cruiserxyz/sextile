package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// ephemeris.go: the startup account of objects Astrolog has no ephemeris file
// for. Such an object is never in an argv (astrolog.Availability), so nothing
// is drawn at a wrong position. A body without data is left out quietly: the
// account is written for the large log, never for the Log panel or the footer.

// EphemerisProbedMsg carries the availability probe's answer.
type EphemerisProbedMsg struct {
	Availability lookup.EphemerisAvailability
	Err          error
}

// ephemerisProbe is indirected so a test can supply an answer without spawning
// astrolog, the same idiom as objectRegistryRead.
var ephemerisProbe = lookup.ProbeEphemeris

// ephemerisProbeLimit is how long the startup check of the ephemeris files may
// take before the start gives up and says so (slice 8c). The check reads the
// object list and both default sets, casts every object once, and casts each
// one that came back without a position alone. Measured on this host
// (Astrolog 8.00, lookup.ProbeEphemeris, three runs each, 2026-11-04): 0.213 s
// with the complete share folder (32 objects without a file), 0.245 s with a
// share folder that lacks seas_18.se1 (a folder of links to every file of the
// complete one but that: 38 without), the slower. The limit is the larger of
// 30 s and ten times the slower measurement (2.45 s), so 30 s: a check that
// takes a hundred times its usual time is stuck, and waiting longer would leave
// the operator at an empty screen with nothing to read.
const ephemerisProbeLimit = 30 * time.Second

// ephemerisProbeDeadline is the limit in force; a test shortens it.
var ephemerisProbeDeadline = ephemerisProbeLimit

// loadEphemerisCmd runs the check under the deadline. The answer is waited for
// in a goroutine of its own, so a check that does not return when its context
// ends (a child process that ignores the kill) still ends the wait: the start
// then stops on the can't-start screen instead of hanging.
func loadEphemerisCmd() tea.Cmd {
	return func() tea.Msg {
		limit, probe := ephemerisProbeDeadline, ephemerisProbe
		ctx, cancel := context.WithTimeout(context.Background(), limit)
		defer cancel()
		done := make(chan EphemerisProbedMsg, 1)
		go func() {
			av, err := probe(ctx)
			done <- EphemerisProbedMsg{Availability: av, Err: err}
		}()
		select {
		case msg := <-done:
			if msg.Err != nil && ctx.Err() != nil {
				return EphemerisProbedMsg{Err: fmt.Errorf("no answer within %s", limit)}
			}
			return msg
		case <-ctx.Done():
			return EphemerisProbedMsg{Err: fmt.Errorf("no answer within %s", limit)}
		}
	}
}

// ephemerisAccount is the one line (absentLines) for the objects the operator's
// view would draw that lack a file, and the extra objects the saved session
// names, which the lenses do not draw. Empty when nothing is absent.
func (m Model) ephemerisAccount(extra []int, lenses ...lookup.Lens) ([]string, error) {
	av := m.ephemeris
	// With every file there is nothing to name, and nothing to resolve.
	if len(av.Absent) == 0 {
		return nil, nil
	}
	seen := map[int]bool{}
	var nums []int
	add := func(n int) {
		if !seen[n] {
			seen[n] = true
			nums = append(nums, n)
		}
	}
	for _, l := range lenses {
		on, err := lookup.OnScreenObjects(av, l)
		if err != nil {
			return nil, err
		}
		for _, n := range on {
			add(n)
		}
	}
	for _, n := range extra {
		add(n)
	}
	if nums == nil {
		nums = []int{}
	}
	return m.absentLines(nums), nil
}

// absentWhat is the missing file as the Log words it.
func absentWhat(file string) string {
	if file == "" {
		return "no data for this date"
	}
	return "no file " + file
}

// logLineLimit is the widest line the Log shows uncut at 132x40 with one chart
// shown, the width the one-line accounts are measured against. Zero (no bound)
// when that layout has no Log.
func (m Model) logLineLimit() int {
	l := layoutFor(layoutInput{width: 132, height: 40, focus: PanelChart, charts: 1, shown: 1})
	if l == nil {
		return 0
	}
	r, ok := l.rects[PanelLog]
	if !ok {
		return 0
	}
	return r.w - 2
}

// logMessageLimit is the widest message the Log panel shows uncut in the same
// layout: the panel's width less its group column and the space after it.
func (m Model) logMessageLimit() int {
	l := m.logLineLimit()
	if l <= 0 {
		return 0
	}
	return l - logGroupCells - 1
}

// fits reports whether a sentence is no wider than the limit (0: any).
func fits(sentence string, limit int) bool {
	return limit <= 0 || ansi.StringWidth(sentence) <= limit
}

// nameList is up to three names as a list and whether they were that few.
func nameList(names []string) (string, bool) {
	return strings.Join(names, ", "), len(names) >= 1 && len(names) <= 3
}

// absentLines is the one Log line for the missing files of the objects among
// nums. A single missing file and at most three bodies, when the sentence fits
// the Log, name the bodies and the file; anything else gives the count and says
// where to see which. Nothing when no object among nums is absent.
func (m Model) absentLines(nums []int) []string {
	groups := m.ephemeris.Groups(nums)
	total := 0
	for _, g := range groups {
		total += len(g.Names)
	}
	switch {
	case total == 0:
		return nil
	case len(groups) == 1:
		if list, few := nameList(groups[0].Names); few {
			if line := list + " can't be drawn here: " + absentWhat(groups[0].File); fits(line, m.logLineLimit()) {
				return []string{line}
			}
		}
	}
	if total == 1 {
		return []string{"1 body can't be drawn here. Press Ctrl+B to see which."}
	}
	return []string{fmt.Sprintf("%d bodies can't be drawn here. Press Ctrl+B to see which.", total)}
}

// drainedLine is the one Log line for a selection that draws nothing, so the
// chart shows every drawn body instead: nums are the selected objects that are
// not drawn. Empty when there are none.
func (m Model) drainedLine(nums []int) string {
	if len(nums) == 0 {
		return ""
	}
	var names []string
	every := true
	for _, n := range nums {
		if m.ephemeris.IsAbsent(n) {
			names = append(names, m.ephemeris.Names[n])
			continue
		}
		every = false
		name := fmt.Sprintf("object %d", n)
		if i := slices.IndexFunc(m.objectRegistry, func(r aspect.RegistryObject) bool { return r.Number == n }); i >= 0 {
			name = m.objectRegistry[i].Name
		}
		names = append(names, name)
	}
	// A body the chart has no ephemeris for is selected but cannot be drawn. A
	// body the chart simply lacks is not among its bodies. The tail is the short one, since the Log holds 65 cells.
	form, plain := "%s selected but can't be drawn here", "the selected bodies can't be drawn here"
	if !every {
		form, plain = "%s not among this chart's bodies", "the selected bodies are not drawn"
	}
	const tail = ", all are shown"
	if list, few := nameList(names); few {
		line := fmt.Sprintf(form, list) + tail
		if fits(line, m.logMessageLimit()) {
			return line
		}
	}
	return plain + tail
}

// applyEphemerisProbed folds the check's answer in. A failed or late check is
// a can't-start in its own words, never a start on a guess: which objects a
// chart draws is the host's default set, and the check is what reads it
// (slice 8c; it used to log a line and carry on with a typed list of thirteen).
func applyEphemerisProbed(m Model, msg EphemerisProbedMsg) (Model, tea.Cmd) {
	if msg.Err != nil {
		m.cantStart.ephemerisProbe = oneLine(msg.Err.Error())
		return m, nil
	}
	m.ephemeris = msg.Availability
	return m.sayEphemeris(), nil
}

// logEphemeris appends one account to the large log only: the sentence names
// the bodies and the file and is complete by itself.
func (m Model) logEphemeris(text string) Model { return m.logTrace("bodies", "%s", text) }

// allAbsent reports whether every one of nums has no ephemeris file here.
func (m Model) allAbsent(nums []int) bool {
	for _, n := range nums {
		if !m.ephemeris.IsAbsent(n) {
			return false
		}
	}
	return len(nums) > 0
}

// sayEphemeris writes the account to the large log, unless the same account was
// already given for this data folder.
func (m Model) sayEphemeris() Model {
	// Saved, not drawn: still part of the account, unless the account already
	// named it when the defaults were the view (the startup answer comes first).
	var kept []int
	if m.lens.Objects.Baseline == recipe.Explicit && len(m.ephemeris.Absent) > 0 {
		def := m.lens
		def.Objects = m.defaultObjects()
		onDefault, err := lookup.OnScreenObjects(m.ephemeris, def)
		if err != nil {
			return m.logTrace("bodies", "the ephemeris account cannot be made: %v", err)
		}
		for _, n := range m.keptAbsentNums() {
			if !slices.Contains(onDefault, n) {
				kept = append(kept, n)
			}
		}
	}
	lines, err := m.ephemerisAccount(kept, m.viewedLenses()...)
	if err != nil {
		return m.logTrace("bodies", "the ephemeris account cannot be made: %v", err)
	}
	said := m.ephemeris.Folder + "\n" + strings.Join(lines, "\n")
	if said == m.ephemerisSaid {
		return m
	}
	m.ephemerisSaid = said
	for _, l := range lines {
		m = m.logEphemeris(l)
	}
	return m
}

// viewedLenses are the lenses the screens draw bodies under: the workspace's
// own, and each slot's with the bodies the operator selected on it (a restored
// session brings those back after the probe's answer may already be in).
func (m Model) viewedLenses() []lookup.Lens {
	lenses := []lookup.Lens{m.lens}
	if m.slots == nil {
		return lenses
	}
	for _, s := range m.slots.All() {
		lenses = append(lenses, m.showLens(s))
	}
	return lenses
}

// sayDrainedSelection writes the one Log line for a focused slot whose selected
// bodies are none of them drawn: the chart cannot draw them (no ephemeris file)
// or its set does not, so it draws the default set instead
// (astrolog.emitObjects), and the operator is told once rather than left with a
// chart that does not match their selection.
func (m Model) sayDrainedSelection() Model {
	var lines []string
	traced := false
	if m.slots != nil {
		nums, err := m.showLens(m.focusedSlot()).DrainedObjects()
		if err != nil {
			return m.logTrace("bodies", "the drained selection cannot be told: %v", err)
		}
		if len(nums) > 0 {
			if l := m.drainedLine(nums); l != "" {
				lines, traced = append(lines, l), m.allAbsent(nums)
			}
		}
	}
	line := strings.Join(lines, "\n")
	if line == m.drainSaid {
		return m
	}
	m.drainSaid = line
	for _, l := range lines {
		if traced {
			m = m.logEphemeris(l)
		} else {
			m = m.logBare("bodies", l)
		}
	}
	return m
}

// logLeftOut writes, for the large log only, which bodies a search left out
// because Astrolog has no ephemeris data for the date, and Astrolog's own lines
// about them. The screens say nothing: a body without data is left out quietly.
func (m Model) logLeftOut(what string, lo lookup.LeftOut) Model {
	if len(lo.Bodies) == 0 {
		return m
	}
	var names []string
	for _, n := range lo.Bodies {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	m = m.logTrace("bodies", "the %s search left out %s: no ephemeris data for this date", what, strings.Join(names, ", "))
	var said []string
	for _, l := range lo.Lines {
		if !slices.Contains(said, l) {
			said = append(said, l)
			m = m.logTrace("bodies", "Astrolog said %s", l)
		}
	}
	return m
}
