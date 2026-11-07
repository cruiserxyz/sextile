package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// applyStartupPerimeter stores the perimeter read and tries to resolve.
// If the atlas has not yet loaded, the perimeter sits pending until
// AtlasLoadedMsg arrives; resolution runs once both signals are in.
func applyStartupPerimeter(m Model, msg ingest.StartupPerimeterReadMsg) (Model, tea.Cmd) {
	foreign := false
	if msg.HasSession && msg.SessionErr == nil && !sessionBelongsHere(msg) {
		// One session is shared by every charts folder, but it is only ever
		// opened in the folder it was saved in: a same-named chart in another
		// folder may be another client's copy. Nothing of it is restored.
		//
		// A general session that names another folder is that folder's: this
		// run never writes it, for the whole launch (sessionForeign). One that
		// names no folder is a 0.1.1 leftover, replaced at quit as before, a
		// named session (--session) is not the general one, and a file the read
		// refused is neither marked nor written over: it was moved aside, so B
		// writes its own as before, or it stayed in place and sessionKept
		// (reportSessionKeep) writes nothing.
		foreign = msg.Session.Folder != "" && !m.named.running() && msg.SessionKeep == (store.SessionKeep{})
		msg.HasSession, msg.Session = false, store.Session{}
		m.sessionForeign = foreign
		m = m.logNote("session", "the saved session is another folder's and was not opened")
	}
	m.pendingPerimeter = &msg
	// The keyboard configuration needs neither the atlas nor a resolved
	// location, so it applies here rather than waiting on resolution. It
	// starts nothing and is silent unless something was refused.
	m = m.applyKeyConfig(msg.Keys)
	if foreign {
		// After the key configuration, so the key it names is the configured one.
		// Each line fits the Log (76 cells with its scope), so the key is never cut.
		m = m.logNote("session", "this run is saved only by name (%s)", keys.LeaderSpelling(m.leaderKey, []string{"o"}))
	}
	m = m.applyDisplayConfig(msg.Display)
	m = m.applyWheelFormat(msg.WheelFormat)
	m = m.applyHouseDisplay(msg.HouseDisplay)
	m = m.applyExperimental(msg.Experimental)
	m = m.applyFrameConfig(msg.Frame)
	m = m.applyClockConfig(msg.Clock)
	// A refused session file was moved aside (or not) by the read itself; say so
	// now, so a quit from the city modal still reports a failed keep (C4).
	m = m.reportSessionKeep(msg.SessionKeep)
	if !m.cfg.SecondWindow || m.named.running() {
		return maybeResolveStartup(m)
	}
	// A second window says once, on the Log and in the key bar, that what is
	// done here is not kept: the session it restored is the other window's.
	m = m.logNote("second window", "%s", secondWindowNote)
	m, notice := m.showNotice(noticeSecondWindow)
	m, cmd := maybeResolveStartup(m)
	return m, tea.Batch(notice, cmd)
}

// applyAtlasLoaded folds the atlas index into the model and tries to
// resolve. If the perimeter has not yet been read, the atlas index is
// stored and resolution waits for StartupPerimeterReadMsg.
func applyAtlasLoaded(m Model, msg AtlasLoadedMsg) (Model, tea.Cmd) {
	m.atlas.Index = msg.Index
	m.atlas.LoadErr = msg.Err
	m.cantStart.atlas = m.cantStart.atlas || msg.Err != nil
	m.atlas.Refilter()
	return maybeResolveStartup(m)
}

// maybeResolveStartup runs the candidate-chain resolution once both the
// perimeter read and the atlas index are in. No-op while either is
// missing. Per the reviewer 2026-04-29 design review.
//
// Resolution policy:
//   - Location chain: stored-location → IANA preseed. On full miss, open
//     the atlas modal so the user picks explicitly.
//   - Anchor: the last quit's session only. Without one, transit-only.
//   - Resolver warnings land in the log panel.
//   - No hardcoded "Berlin" technical fallback — refusing is better than
//     silently casting the wrong chart.
func maybeResolveStartup(m Model) (Model, tea.Cmd) {
	if m.pendingPerimeter == nil {
		return m, nil
	}
	// Wait for atlas: either the index loaded, or load errored. Nil index
	// with nil err means we haven't heard back yet.
	if m.atlas.Index == nil && m.atlas.LoadErr == nil {
		return m, nil
	}
	// Every printed row is named through the object list, which loads beside
	// the perimeter: no chart is cast before it is in (its arrival resumes
	// this). A list that cannot be read is a can't-start, never a start
	// without it.
	if m.objectRegErr != nil {
		return m, nil
	}
	// The same for the check of the ephemeris files: which objects a chart
	// draws is the host's default set, and without that answer there is no
	// chart to cast (slice 8c). It lands before this is reached, because Init
	// starts the reads after it; this only makes a failed one stop the start.
	if m.cantStart.ephemerisProbe != "" {
		return m, nil
	}
	if m.objectRegistry == nil {
		m.awaitingRegistry = true
		return m, nil
	}
	pending := *m.pendingPerimeter
	m.pendingPerimeter = nil

	locCands := []location.Candidate{}
	if pending.HasLocationCandidate() {
		locCands = append(locCands, pending.LocationCandidate)
	}
	if pending.IANAPreseed != "" {
		// Carry the ZONE, not just the city name the zone was cut from. City
		// names repeat across the world — the atlas holds an Amsterdam in NL,
		// one in New York state and a New Amsterdam in Guyana — and the
		// resolver now refuses an ambiguous name rather than taking the first
		// atlas row. The zone is what tells it which one was meant, and it is
		// the same host fact that produced the name, so the two cannot drift.
		locCands = append(locCands, location.Candidate{
			Source: "iana-preseed",
			Place:  location.RawPlace{Name: pending.IANAPreseed, TZ: location.HostIANA()},
		})
	}

	// The resolver's per-candidate notes stay in locRes.Warnings and out of
	// the Log (operator, 2026-10-25): a guessed city is said once, by the
	// city line below, and an unresolved one opens the picker.
	locRes, locErr := location.Resolve(locCands, m.atlas.Index)
	if locErr != nil {
		m.startup = startupAtlasOpen
		m.atlas.Preseed = pending.IANAPreseed
		m.atlas.FilterText = pending.IANAPreseed
		m.atlas.Refilter()
		// Re-stash the pending perimeter so applyAtlasPicked can restore
		// the session's chart once the user disambiguates the city. Without
		// this the session's anchor silently disappears on the
		// unresolved-location startup path.
		stashed := pending
		m.pendingPerimeter = &stashed
		return m, nil
	}
	m.place = locRes.Place
	m.placeGuessed = locRes.Source == "iana-preseed"
	if m.placeGuessed {
		m = m.logNote("city", "%s, from the time zone — , c d saves it", locRes.Place.Display())
	}

	// Construct the workspace: one floating slot at (now, resolved place).
	// Refusal of an untimed place is impossible here — location.Resolve
	// already gates on HasTimezone — but we surface the error defensively.
	slots, err := store.NewSlots(m.clock.Now(), locRes.Place)
	if err != nil {
		m = m.logTrace("slots", "%v — proceeding without workspace", err)
	} else {
		m.slots = slots
		m.cursorIdx = 0 // floating is index 0 when there's no anchor
		m.targetFloatingTime = slots.Floating().Time
	}

	// The last quit's workspace comes back. It is the only thing that anchors
	// at startup: without a readable session the workspace is floating-only
	// (C9).
	m, restored := m.restoreSession(pending)
	// A chart-file anchor owes its re-read once the first frame is out.
	recheck := m.sessionRecheckCmd(pending, restored)
	// Last, so the remembered marks land on the restored workspace.
	// The restored cursor and selections are an arrival: the tab starts on
	// the chart alone (S4a A27).
	m = m.applySessionSelections(pending).forgetTabMemory()
	// The restored selection is in place: say again what it names that this
	// Astrolog cannot draw (once per distinct account).
	m, say := m.sayEphemeris()

	m.startup = startupComputing
	// Cold start births the first workspace identity — route through
	// bumpIdentity (immediate compute, identity floor, no debounce).
	var recompute tea.Cmd
	m, recompute = m.bumpIdentity(surfaceText)
	// Batched, not sequenced: the re-read must not hold up the first frame,
	// which is the whole reason it runs after it. The floating chart starts
	// attached to the clock, so its first minute tick is armed with the
	// workspace.
	m, tick := m.armNowTick()
	return m, tea.Batch(recompute, recheck, tick, say)
}

// applyAtlasPicked transitions out of atlas-open and queues the compute. A pick
// lasts for the session and writes nothing; `<leader> c d` is what keeps a city.
//
// The live chart is cleared on transition, but Bodies and Aspects keep
// painting the previous committed frame until the picked place's frame
// commits (the specification), the way a cursor move does. Anchor
// state and store-owned selection survive the clear so buildTextCmd keeps its
// comparison route when a selected natal pair is present.
func applyAtlasPicked(m Model, msg AtlasPickedMsg) (Model, tea.Cmd) {
	var recheck, say tea.Cmd // set only on the cold start that had to ask for a city
	m.place = msg.Place
	m.placeGuessed = false
	m = m.clearChartKeepingHold()
	m = m.suspendDetails()
	m = m.rememberBodyRow()
	m.bodyCursorIdx = 0
	m = m.resetBodyTop()
	// Refresh the slot workspace for the newly picked place. Both flows
	// land here: cold-start-via-atlas (no prior slots) and post-startup
	// re-pick (slots exist; every anchored slot is preserved — the natal
	// charts do not move when the user changes their current city).
	m = m.refreshSlotsForPlace(msg.Place)

	// Cold-start path: if the unresolved-location branch in
	// maybeResolveStartup stashed pendingPerimeter, the session is still
	// waiting to be restored. Restore it now, so a cold start that had to ask
	// for the city does not silently lose the session's chart.
	if m.pendingPerimeter != nil {
		pending := *m.pendingPerimeter
		m.pendingPerimeter = nil
		// A cold start that had to ask for the city reaches restore HERE
		// instead, once there is a workspace to restore into (C13).
		var restored bool
		m, restored = m.restoreSession(pending)
		recheck = m.sessionRecheckCmd(pending, restored)
		// The restored cursor and selections are an arrival: the tab starts on
		// the chart alone (S4a A27).
		m = m.applySessionSelections(pending).forgetTabMemory()
		m, say = m.sayEphemeris()
	}

	m.startup = startupComputing
	// bumpIdentity hard-cancels any in-flight compute for the PRIOR city
	// (held-key sweep, initial startup dispatch) and fences off its already-
	// queued results via the commitFloor, so the previous city's frame can
	// never paint over the re-picked one. buildTextCmd reads the rebuilt
	// slots to choose overlay vs transit-only.
	var recompute tea.Cmd
	m, recompute = m.bumpIdentity(surfaceText)
	// A chart attached to the clock keeps its minute tick through a city
	// change; the timer armed for the old workspace retires by number.
	m, tick := m.armNowTick()
	return m, tea.Batch(recompute, recheck, tick, say)
}

// applyChartsLoaded stores the loaded chart library into the open picker (or
// records a load error) and refilters against whatever the user has typed so
// far. The picker renders a loading line until this arrives.
//
// A missing atlas is the one library failure that is not about any single file:
// it takes every chart down at once, and the fix is outside Sextile. It goes on
// the error lane so it survives in the Log and the console, beside the named
// rows the picker shows for the files themselves.
func applyChartsLoaded(m Model, msg ChartsLoadedMsg) (Model, tea.Cmd) {
	if msg.Gen != m.pickerGen {
		// Stale response from a superseded open — dropping it keeps a late
		// failure from latching over a newer successful load (the reviewer an earlier change MED).
		return m, nil
	}
	m.picker.loaded = true
	if msg.Err != nil {
		m.picker.loadErr = msg.Err
		m.picker.charts = nil
		m.picker.drops = nil
	} else {
		m.picker.loadErr = nil // a successful load clears any prior error
		m.picker.charts = msg.Charts
		m.picker.drops = msg.Drops
		m.picker.seen = msg.Seen
		// A listing that succeeded and listed a chart is evidence the folder is
		// readable: only then may a session write drop a deleted chart's record
		// (the specification C10).
		if len(msg.Charts) > 0 && msg.AtlasErr == nil {
			m.frames.pruneOK = true
		}
	}
	if msg.AtlasErr != nil {
		m = m.logError("Couldn't find atlas.as. Put it beside Astrolog and restart.")
	}
	m.picker.refilter()
	if m.picker.fit.Width == 0 {
		m.picker.fit = chartFit(m.picker, m.cfg)
	}
	if m.picker.land != "" {
		// A rename reloads the library and puts the cursor back on the chart
		// it renamed (the specification C4.3).
		for i, c := range m.picker.Matches {
			if c.Source.File == m.picker.land {
				m.picker.Cursor = i
				m.picker.ScrollTo(i, chartBlockHeight(m.picker, dialog.ListHeight(m.height)))
			}
		}
		m.picker.land = ""
	}
	return m, nil
}

// applyChartPicked starts the pick. A listing row carries no birth moment, so
// nothing is anchored here: the pick dispatches a resolve against the file the
// row named, bumps the pick generation, and leaves the picker open showing that
// it is reading. The anchor is set in applyChartResolved when — and only when —
// that resolve lands.
//
// Bumping the generation is what makes a second pick safe: the first resolve
// still returns, and is dropped whole.
func applyChartPicked(m Model, msg ChartPickedMsg) (Model, tea.Cmd) {
	if m.slots == nil {
		// No resolved workspace to anchor onto — restore the view we opened
		// from rather than resolving a chart we could not use. OpenCharts
		// gates on this too; this is defense in depth.
		m.startup = m.picker.prior
		return m, nil
	}
	if needsTitle(msg.Chart) {
		// the specification C3.4: an untitled chart asks for its title
		// first. Skipping the form comes back to resolvePick.
		return openTitleForm(m, msg.Chart), nil
	}
	return resolvePick(m, msg.Chart)
}

// resolvePick dispatches the resolve for a picked row: the pick itself, once
// any title form has been answered or skipped.
func resolvePick(m Model, c store.NamedChart) (Model, tea.Cmd) {
	return resolvePickIn(m, "", c)
}

// resolvePickIn is resolvePick reading the chart from the directory dir names
// (empty: the resolver's answer now). A pick that follows a write passes the
// directory the write went to.
func resolvePickIn(m Model, dir string, c store.NamedChart) (Model, tea.Cmd) {
	if m.slots == nil {
		m.startup = m.picker.prior
		return m, nil
	}
	m.pickGen++
	m.picker.resolving = true
	m.picker.resolveErr = nil
	m.picker.refusal = ""
	m.picker.pending = c
	return m, resolveChartCmdIn(dir, c, m.pickGen)
}

// applyChartResolved anchors the picked chart now that its authoritative moment
// is known: it sets the overlay handle and the anchored slot, selects the
// anchored+floating pair, and dispatches the comparison recompute. Picking a
// chart changes the workspace identity (a new anchored slot), so it clears the
// stale chart payload and goes through bumpIdentity — which cancels the prior
// anchor's in-flight compute and floors its queued results so a rapid re-pick
// never paints stale. The transit place and the floating-slot time are
// untouched; only the anchor changes.
//
// A failed resolve changes nothing at all. The anchor the operator was looking
// at stays exactly as it was, the picker stays open, and the reason goes on the
// error lane, where it survives in the Log and the console. The popup itself
// says nothing about it (the specification C5). A
// resolve that SUCCEEDED but produced a record that cannot anchor is refused
// the same way, and for the same reason: an explicit pick that quietly does
// nothing is indistinguishable from a broken key.
func applyChartResolved(m Model, msg ChartResolvedMsg) (Model, tea.Cmd) {
	if msg.Gen != m.pickGen {
		// Superseded by a later pick, or cancelled with Esc. Drop it whole:
		// no anchor, no state, no error line.
		return m, nil
	}
	m.picker.resolving = false
	if msg.Err != nil {
		m.picker.resolveErr = msg.Err
		return m.noteEphemeris(msg.Err).logError("Couldn't open that chart. %s", m.failureReason(msg.Err, pickFallback)), nil
	}
	if m.slots == nil {
		m.startup = m.picker.prior
		return m, nil
	}
	c := msg.Chart
	// The casting place is the one the read found for the file as it is NOW: a
	// file replaced since the listing gives its own place with its own moment.
	if msg.HasPlace {
		c.Place = msg.Place
	}
	if msg.Unplaced {
		return m.refuseAnchor("no known place")
	}
	// What the file says about its own houses is known before anything casts.
	// For a chart that is already open, the system it is shown in is compared
	// before and after, so a changed house line recasts it.
	var openBefore recipe.HouseSystem
	openIdx := m.slots.FindAnchored(store.SessionChart{Source: c.Source})
	if openIdx >= 0 {
		openBefore = m.ownHouses(m.slots.All()[openIdx]).hs
	}
	if f := filepath.Base(c.Source.File); c.Source.Kind == store.ChartKindFile && c.Source.File != "" {
		m.frames = m.frames.withFile(f, fileFrame{when: msg.When, houses: msg.Houses, has: msg.HasHouses})
	}
	// The record the session file will name this chart by, built BEFORE
	// anything moves so it can be checked before anything moves. The slot keeps
	// the moment and the place; only this remembers which library row they are.
	rec := store.SessionChart{ID: c.ID, Name: c.Name, When: msg.When, Place: c.Place, Source: c.Source, Label: msg.Label}
	if msg.HasPlace {
		// The title the read found, with the moment it found it beside: a file
		// replaced since the listing is named by what it holds now, and one
		// that became untitled is shown under its file stem and sends no name.
		if msg.Label.Name != "" {
			rec.Name = msg.Label.Name
		} else {
			base := filepath.Base(c.Source.File)
			rec.Name = strings.TrimSuffix(base, filepath.Ext(base))
		}
	}
	if reason := rec.UnusableReason(); reason != "" {
		// The library lists on a weaker bar than anchoring needs, by design: a
		// listing row carries no moment at all, so a file with no
		// `place:`, a `lat:` that parsed to NaN, or a zone this host cannot
		// load is pickable and still not castable. ingest answers that by
		// falling back to the session snapshot, which carries the same chart;
		// here there is no fallback and the operator chose this row explicitly,
		// so the pick refuses and names what is missing. Anchoring it anyway
		// leaves a second slot on the header that the comparison never
		// computes, which is the whole of an earlier change.
		return m.refuseAnchor(reason)
	}
	m.picker.resolveErr = nil
	m.picker.refusal = ""
	if i := m.slots.FindAnchored(rec); i >= 0 {
		// A chart already in the list: the cursor goes to its row and nothing
		// is added (M3). One file is one row. The pick just read the file, so if
		// its name, id, moment or place is not the slot's the file wins, in place: the slot keeps its
		// position, selection, body marks and Bodies row (C5).
		open := m.slots.All()[i]
		if open.Time.Equal(msg.When) && open.Location == c.Place && open.Chart.Name == rec.Name && open.Chart.ID == rec.ID && open.Chart.Label == rec.Label {
			// Nothing about the chart changed: the pick only moves the cursor.
			// The picker closes back to where it opened from; the settled frame
			// stays, and a recompute happens only if the cursor lands on a
			// different slot, exactly as ↑/↓ would.
			// The Charts panel takes the keyboard either way: the operator
			// just chose this chart, so the next ↑/↓ or Space is about it.
			m.startup = m.picker.prior
			recast := m.ownHouses(open).hs != openBefore
			if i == m.cursorIdx {
				m, cmd := m.setFocusedPanel(PanelChart)
				return recastAfter(m, cmd, recast)
			}
			m = m.focusSlot(i)
			m.focused = PanelChart
			m, cmd := m.afterFocusChange()
			return recastAfter(m, cmd, recast)
		}
		m = m.clearChartKeepingHold()
		m = m.suspendDetails()
		m = m.rememberBodyRow()
		if err := m.slots.ReplaceAnchored(i, rec, msg.When, c.Place); err != nil {
			m = m.logNote("slots", "anchor refused (%v)", err)
		}
		m = m.focusSlot(i)
	} else {
		m = m.clearChartKeepingHold()
		m = m.suspendDetails()
		// The row of the slot being left is remembered BEFORE the list changes:
		// the insert shifts what the cursor index points at.
		m = m.rememberBodyRow()
		if err := m.slots.AddAnchored(rec, msg.When, c.Place); err != nil {
			m = m.logNote("slots", "anchor refused (%v)", err)
		} else {
			// The new slot sits directly above the floating slot and takes
			// focus, leaving every other slot as it was (M2).
			m = m.focusSlot(m.slots.AnchoredCount() - 1)
			m.bodyCursorIdx = 0
			m = m.resetBodyTop()
		}
	}
	// The cursor arrived on a chart (an add, a replace): the Charts panel
	// takes the keyboard on it. Details was suspended above and the wheel repaints below, so
	// the focus change needs neither from setFocusedPanel.
	m.focused = PanelChart
	m = m.pruneTabMemory().noteArrival()
	m.startup = startupComputing
	var recompute, wheel tea.Cmd
	m, recompute = m.bumpIdentity(surfaceText)
	// The focused slot may have changed (an add, or a jump to an open chart):
	// an open wheel follows it the way it follows ↑/↓ (afterFocusChange).
	m, wheel = m.refreshWheelIfActive()
	return m, tea.Batch(recompute, wheel)
}

// refuseAnchor is the pick's one refusal: the picker names what the chart
// lacks, and the error lane says so.
func (m Model) refuseAnchor(reason string) (Model, tea.Cmd) {
	m.picker.refusal = reason
	return m.logError("Couldn't anchor that chart. It has %s.", reason), nil
}

// refreshSlotsForPlace rebuilds m.slots for the newly picked place,
// preserving any operator-controlled state from the prior workspace:
// the natal anchor (anchor doesn't move when the user changes city)
// AND the floating slot's Time (if the user time-stepped to a target
// moment, re-picking the city should keep them at that target rather
// than snap back to wall-clock now). Without preservation, an earlier change
// review round-3 noted that re-pick silently discards step navigation.
//
// Falls back to m.clock.Now() for the floating time when no prior
// workspace exists (true cold-start atlas-pick). Logs and leaves
// m.slots untouched if place lacks an IANA timezone — atlas picks
// always have one, so that branch is defensive.
func (m Model) refreshSlotsForPlace(place location.PlaceInfo) Model {
	previousSlots := m.slots
	var prevAnchored []*store.Slot
	// a review finding: preserve the OPERATOR'S TARGET
	// (m.targetFloatingTime) across re-pick, not just the committed
	// slot.Floating().Time. If the user time-stepped to T+5 and the
	// matching computes haven't returned yet, slot.Time is still T+0
	// — using slot.Time would silently snap back to the unstepped
	// moment when they change city. Use the target when set; fall
	// back to slot.Time (e.g. after a long settle) and finally to
	// clock.Now() for cold-start.
	floatingTime := m.clock.Now()
	if m.slots != nil {
		prevAnchored = m.slots.All()[:m.slots.AnchoredCount()]
		floatingTime = m.slots.Floating().Time
	}
	if !m.targetFloatingTime.IsZero() {
		floatingTime = m.targetFloatingTime
	}
	slots, err := store.NewSlots(floatingTime, place)
	if err != nil {
		m = m.logTrace("slots", "%v — workspace not refreshed", err)
		return m
	}
	// The caller (applyAtlasPicked) has already remembered the focused slot's
	// row; remembering again here would read the reset cursor and overwrite it.
	m.slots = slots
	m.cursorIdx = 0
	m.targetFloatingTime = slots.Floating().Time
	// The whole list comes across, in order, each slot with its own chart. A
	// slot whose place cannot be re-added (never the case for one that was
	// added) is dropped and said so; the rest still carry over.
	oldRows := m.bodyRows
	m.bodyRows = nil
	// The tab's memory names slots, and these are new objects: each old slot
	// maps to its rebuilt counterpart (anchored by its place in the list,
	// floating to floating) so the base survives the new city.
	counterpart := map[*store.Slot]*store.Slot{}
	for _, prev := range prevAnchored {
		if err := m.slots.AddAnchored(prev.Chart, prev.Time, prev.Location); err != nil {
			m = m.logTrace("slots", "anchor refused after refresh (%v)", err)
			continue
		}
		m.cursorIdx = m.slots.AnchoredCount() // floating sits behind the anchored slots
		fresh := m.slots.All()[m.slots.AnchoredCount()-1]
		counterpart[prev] = fresh
		m = m.rehomeRechecks(prev, fresh)
		if row, ok := oldRows[prev]; ok {
			m = m.keepBodyRow(fresh, row)
		}
	}
	m.slots.CopySelectionsFrom(previousSlots)
	// The cursor is on the floating slot, as it has always been after a city
	// re-pick, and now keeps the row it was left on like every other slot.
	if previousSlots != nil {
		old := previousSlots.All()
		counterpart[old[len(old)-1]] = m.slots.All()[len(m.slots.All())-1]
		if row, ok := oldRows[old[len(old)-1]]; ok {
			m = m.keepBodyRow(m.slots.All()[len(m.slots.All())-1], row)
		}
	}
	m.bodyCursorIdx = m.bodyRows[m.focusedSlot()]
	m = m.resetBodyTop()
	// A city change is not a tab choice: the base and the row arrived on go
	// over to the rebuilt slots, and only what has no counterpart is dropped.
	return m.remapTabMemory(counterpart)
}
