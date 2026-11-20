package ui

import (
	"context"
	"errors"
	"maps"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// The Moon's lane in Bodies (specs/moon-observatory.md, slice B). Bodies never
// waits for the void search: the lane is derived at paint time from void memory
// alone, and a search that is not in memory is asked for in a command of its
// own, right after the frame commits or in idle time (prefetch.go). A window
// that is asked for, or failed, is remembered here so nothing is asked twice.

// voidLandedMsg is one void search's answer, asked by askVoid or by the idle
// prefetch.
type voidLandedMsg struct {
	key lookup.VoidKey
	res lookup.VoidResult
	err error
}

// voidState is what the Model knows about searches that are not in memory:
// the ones in flight and the ones that failed. The maps are never written in
// place, so a Model copied before an Update does not see what the Update did.
type voidState struct {
	// due says a Bodies frame committed and its window has not been looked for
	// yet (stampShown): the ask follows the commit, not every message.
	due    bool
	asked  map[lookup.VoidKey]bool
	failed map[lookup.VoidKey]bool
}

// withAsked returns the state with k in flight or not.
func (v voidState) withAsked(k lookup.VoidKey, on bool) voidState {
	next := maps.Clone(v.asked)
	if next == nil {
		next = map[lookup.VoidKey]bool{}
	}
	if on {
		next[k] = true
	} else {
		delete(next, k)
	}
	v.asked = next
	return v
}

func (v voidState) withFailed(k lookup.VoidKey) voidState {
	next := maps.Clone(v.failed)
	if next == nil {
		next = map[lookup.VoidKey]bool{}
	}
	next[k] = true
	v.failed = next
	return v
}

// voidRequestFor is the void search that answers a chart read at moment in
// the frame hs and z: universal time, the lens's registry. The houses change no
// void; they are the frame every displayed search carries. ok is false when the registry
// names no Moon, so there is no lane.
func (m Model) voidRequestFor(moment time.Time, hs recipe.HouseSystem, zodiac recipe.Zodiac) (lookup.VoidRequest, lookup.VoidKey, bool) {
	if moment.IsZero() {
		return lookup.VoidRequest{}, lookup.VoidKey{}, false
	}
	if _, ok := m.lens.Registry.ByName("Moon"); !ok {
		return lookup.VoidRequest{}, lookup.VoidKey{}, false
	}
	req := lookup.VoidRequest{Moment: moment, Offset: 0, Houses: hs, Zodiac: zodiac, Registry: m.lens.Registry, AspectRegistry: m.lens.AspectRegistry}
	return req, lookup.VoidKeyFor(req), true
}

// voidRequestForSlot is voidRequestFor for a chart in its own frame.
func (m Model) voidRequestForSlot(s *store.Slot) (lookup.VoidRequest, lookup.VoidKey, bool) {
	if s == nil {
		return lookup.VoidRequest{}, lookup.VoidKey{}, false
	}
	return m.voidRequestFor(s.Time, m.ownHouses(s).hs, m.ownZodiac(s))
}

// shownSlot is the chart Bodies shows: the cursor's, or the held frame's.
func (m Model) shownSlot() *store.Slot {
	if m.slots == nil {
		return nil
	}
	idx := m.cursorIdx
	if m.holding() {
		idx = m.shown.slot
	}
	all := m.slots.All()
	if idx < 0 || idx >= len(all) {
		return nil
	}
	return all[idx]
}

// moonLane is the Moon's speed lane for the chart Bodies shows, read from void
// memory only (a peek, never a spawn). Nil when there is no lane: the registry
// names no Moon, or there is no chart.
func (m Model) moonLane() map[int]string {
	moon, ok := m.lens.Registry.ByName("Moon")
	if !ok {
		return nil
	}
	s := m.shownSlot()
	_, key, ok := m.voidRequestForSlot(s)
	if !ok {
		return nil
	}
	word := panel.MoonLanePending
	if res, ok := m.mem.voids.peek(key); ok {
		word = panel.MoonLaneNone
		if res.VoidAt(s.Time) {
			word = panel.MoonLaneVoid
		}
	} else if m.void.failed[key] {
		word = panel.MoonLaneFailed
	}
	return map[int]string{moon.Number: word}
}

// voidWanted says the search for key is neither remembered, nor in flight, nor
// failed.
func (m Model) voidWanted(key lookup.VoidKey) bool {
	if _, ok := m.mem.voids.peek(key); ok {
		return false
	}
	return !m.void.asked[key] && !m.void.failed[key]
}

// askShownVoid asks for the shown chart's window right after its Bodies frame
// committed, if memory lacks it. Stepping inside a window already known finds
// the same key and spawns nothing.
func (m Model) askShownVoid() (Model, tea.Cmd) {
	if !m.void.due || m.startup != startupReady || m.slots == nil || m.cantStart.any() || m.chart.epoch == 0 || m.holding() {
		return m, nil
	}
	m.void.due = false
	req, key, ok := m.voidRequestForSlot(m.shownSlot())
	if !ok || !m.voidWanted(key) {
		return m, nil
	}
	spawn := m.rows[surfaceTimeline].spawn.voidsAhead
	if spawn == nil {
		return m, nil
	}
	m.void = m.void.withAsked(key, true)
	return m, askVoid(spawn, key, req)
}

// askVoid runs one void search off the Update goroutine.
func askVoid(spawn func(context.Context, lookup.VoidRequest) (lookup.VoidResult, error), key lookup.VoidKey, req lookup.VoidRequest) tea.Cmd {
	return func() tea.Msg {
		res, err := spawn(context.Background(), req)
		return voidLandedMsg{key: key, res: res, err: err}
	}
}

// onVoidLanded files an answer: into memory on success, into the failed set
// (and one trace line, nothing in the Log panel) on failure. Either way the
// key is no longer in flight.
func (m Model) onVoidLanded(msg voidLandedMsg) Model {
	m.void = m.void.withAsked(msg.key, false)
	return m.fileVoid(msg)
}

// fileVoid is the part of a landing the idle prefetch shares.
func (m Model) fileVoid(msg voidLandedMsg) Model {
	switch {
	case msg.err == nil:
		m.mem.voids = m.mem.voids.put(msg.key, msg.res)
	case errors.Is(msg.err, context.Canceled):
		// Stopped, not failed: the key stays open.
	default:
		m.void = m.void.withFailed(msg.key)
		m = m.logTrace("bodies", "the Moon's void search for %d failed: %v", msg.key.FirstYear, msg.err)
	}
	return m
}

// voidInFlight says the idle candidate c is a void search the Model already
// has out.
func (m Model) voidInFlight(c prefetchCand) bool {
	k, ok := c.key.(lookup.VoidKey)
	return ok && c.void != nil && m.void.asked[k]
}

// voidCandidates are the windows to search ahead in idle time, in this order
// and without a key twice: Now's, Now's two months later and earlier, then each
// chart's own.
func (m Model) voidCandidates() []prefetchCand {
	if m.slots == nil {
		return nil
	}
	var out []prefetchCand
	seen := map[lookup.VoidKey]bool{}
	add := func(moment time.Time, hs recipe.HouseSystem, z recipe.Zodiac) {
		req, key, ok := m.voidRequestFor(moment, hs, z)
		if !ok || seen[key] || !m.voidWanted(key) {
			return
		}
		seen[key] = true
		out = append(out, prefetchCand{key: key, void: &req})
	}
	floating := m.slots.Floating()
	nowHS, nowZ := m.ownHouses(&floating).hs, m.ownZodiac(&floating)
	loc, _ := lookup.PlaceZone(floating.Time, floating.Location)
	now := floating.Time.In(loc)
	add(now, nowHS, nowZ)
	add(now.AddDate(0, 2, 0), nowHS, nowZ)
	add(now.AddDate(0, -2, 0), nowHS, nowZ)
	for _, s := range m.slots.All() {
		add(s.Time, m.ownHouses(s).hs, m.ownZodiac(s))
	}
	return out
}
