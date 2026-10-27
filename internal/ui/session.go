package ui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/preview"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// session.go is both ends of the relaunch: what the model writes down when the
// operator quits, and what it puts back the next time it starts.
//
// The two halves are deliberately asymmetric. The write is one file, at one
// moment, on the way out. The read is a preference, not an instruction: the
// library chart the session names outranks the snapshot it carries, the
// snapshot outranks nothing at all, and every one of them yields to a
// workspace that could not be built. A relaunch that restores less than it
// saved says so on a Log line and carries on; it never refuses to start.

// quit ends the session, writing it down first.
//
// Both places that quit come through here — main scope and the console — so
// the operator's marked bodies do not depend on which one they happened to be
// looking at when they pressed the key. A modal is the exception, and it never
// reaches this: help, atlas and charts each swallow the key themselves.
func (m Model) quit() (tea.Model, tea.Cmd) {
	// A launch the instance lock refused owns nothing: the session on disk is
	// the other window's, and this one must not write it, whatever the model
	// holds (the specification C2).
	if m.cantStart.locked() {
		return m, tea.Quit
	}
	// Before the workspace exists there is nothing to remember, and writing an
	// empty session would erase a real one for a launch the operator quit out
	// of before it finished starting. A second window writes none at all.
	if m.slots != nil && (m.named.running() || !m.sessionKept) && m.ownsSession() {
		// No frame after this one could show a failure, so it is kept on the
		// model and printed on standard error once the screen is restored
		// (the specification A7). The write leaves the previous
		// session intact on its own.
		if err := m.saveRunningSession(); err != nil {
			m.sessionWriteErr = &SessionWriteError{Path: m.runningSessionPath(), Err: err}
		}
	}
	// The rendered wheel is a named client's chart held by a localhost server.
	// The bytes and server go when the session does. Like the session write, a
	// failure has no frame left to report it in.
	_ = preview.Cleanup()
	return m, tea.Quit
}

// sessionBelongsHere reports whether the session read at startup was saved in
// the charts folder this launch uses. A session that names no folder belongs
// nowhere: the one-time copy of an old session stamps the folder it came from.
func sessionBelongsHere(p ingest.StartupPerimeterReadMsg) bool {
	return p.Session.Folder != "" && p.Session.Folder == p.FolderID
}

// ownsSession reports whether this window may write the session it runs. A
// launch the instance lock refused owns nothing. A second window owns nothing
// either -- the general session belongs to the window that holds its lock --
// until it saves or opens a named session, whose lock it then holds.
func (m Model) ownsSession() bool {
	if m.cantStart.locked() || m.named.kept {
		return false
	}
	return m.named.running() || !m.cfg.SecondWindow
}

// errSecondWindow is a session write asked of a second window.
var errSecondWindow = errors.New("another Sextile window keeps the session")

// saveRunningSession writes the session this window runs: the named one when
// it runs one, else the general one. A window that owns no session writes
// nothing, and so does one whose chosen session is still being opened.
func (m Model) saveRunningSession() error {
	if m.slots == nil || !m.ownsSession() || m.named.pending {
		return nil
	}
	if m.named.running() {
		return store.SaveNamedSession(m.named.lock, m.named.name, m.sessionSnapshot())
	}
	if m.sessionKept {
		return nil
	}
	return store.SaveSession(m.sessionSnapshot())
}

// runningSessionPath is the file saveRunningSession writes.
func (m Model) runningSessionPath() string {
	if m.named.running() {
		return store.NamedSessionPath(m.named.file)
	}
	return store.SessionPath()
}

// releaseRunningLock lets go of the lock of the session this window runs, as
// it goes on to run another: a window holds exactly one.
func (m Model) releaseRunningLock() {
	switch {
	case m.named.running():
		m.named.lock.Release()
	case !m.cfg.SecondWindow:
		store.ReleaseInstanceLock()
	}
}

// SessionWriteError is a session that could not be written at quit: the
// file, and why.
type SessionWriteError struct {
	Path string
	Err  error
}

// Error is the one line standard error carries: the file and the cause in
// the operating system's words, without the write path's own prefixes.
func (e *SessionWriteError) Error() string {
	cause := e.Err
	var pe *fs.PathError
	if errors.As(e.Err, &pe) {
		cause = pe.Err
	} else {
		for next := errors.Unwrap(cause); next != nil; next = errors.Unwrap(cause) {
			cause = next
		}
	}
	// One line, whatever the path or the cause holds (A7).
	return oneLine(fmt.Sprintf("couldn't save the session to %s: %v", e.Path, cause))
}

// SessionWriteError is the session write that failed when Sextile quit, or
// nil. main reads it from the final model after the program returns.
func (m Model) SessionWriteError() error {
	if m.sessionWriteErr == nil {
		return nil
	}
	return m.sessionWriteErr
}

// sessionSnapshot is the workspace as the session file remembers it: every
// anchored slot in list order, then the floating slot's selection and marks.
//
// The moment and place come from the SLOT rather than from its chart record,
// because the slot is what the operator has been looking at — it is the copy a
// city re-pick carried forward. The record supplies only what the slot's
// moment and place do not say: which library chart this is, and what it is
// called.
//
// A slot whose chart has no name writes no entry. Every path that anchors also
// names the chart, so this is a guard, not a case: it keeps a nameless chart
// record from ever reaching the session file.
func (m Model) sessionSnapshot() store.Session {
	all := m.slots.All()
	floating := len(all) - 1
	s := store.Session{
		Folder:           store.FolderID(store.ChartsDir()),
		FloatingSelected: m.slots.SlotSelected(floating),
		FloatingBodies:   m.slots.SelectedBodies(floating),
		Frames:           m.frameRecordsToSave(),
	}
	for i := 0; i < m.slots.AnchoredCount(); i++ {
		slot := all[i]
		if slot.Chart.Name == "" {
			continue
		}
		rec := slot.Chart
		rec.When = slot.Time
		rec.Place = slot.Location
		s.Slots = append(s.Slots, store.SessionSlot{
			Chart:    rec,
			Usable:   true,
			Selected: m.slots.SlotSelected(i),
			Bodies:   m.slots.SelectedBodies(i),
		})
	}
	return s
}

// restoreSession puts the last quit's workspace back and reports whether it
// anchored anything, which decides whether a chart-file anchor owes a re-read.
//
// It adds the slots and reports; the marks are applied afterwards by
// applySessionSelections, once the list is settled. A chart that cannot stand
// on its own is dropped alone and the rest come back in order (C17); a library
// file that has since vanished is no reason to drop anything, because the
// entry carries its own snapshot (C17b), so restore says nothing for it.

// reportSessionKeep says what the launch's read did with a refused file
// (the specification C1-C4, C6). A file that was moved aside is named on
// one note-lane line, by its base name only, and the launch then writes
// sessions normally. A file that could not be moved stays exactly where it is:
// no session is written for the rest of the launch, the error lane says so, and
// the failure is kept for standard error once the screen is restored. It runs
// when the perimeter arrives, before any workspace exists (applyStartupPerimeter).
func (m Model) reportSessionKeep(k store.SessionKeep) Model {
	if k.Kept != "" {
		m.sessionKept = false
		m = m.logNote("session", "refused, kept %s", k.Kept)
	}
	if k.Problem != "" {
		m.sessionKept = true
		m = m.logError("Couldn't keep the refused session file: %s", k.Problem)
		m.sessionWriteErr = &SessionWriteError{Path: store.SessionPath(), Err: errors.New(k.Problem)}
	}
	return m
}

func (m Model) restoreSession(p ingest.StartupPerimeterReadMsg) (Model, bool) {
	m = m.loadFrames(p)
	if p.SessionErr != nil {
		// A named session --session admitted and the launch's read then refused
		// (it changed in between): it is kept untouched, never saved over.
		if m.named.running() {
			m.named.kept = true
		}
		// The file exists and this build must not act on it. Startup proceeds
		// exactly as if there were none. The launch's read already moved the
		// file aside (reportSessionKeep), so the next quit writes a new one.
		// A hand-built message with no keep report keeps the older rule: a line
		// too long to read holds charts this build could not load, so the quit
		// leaves the file alone instead of writing a shorter list over it.
		if p.SessionKeep == (store.SessionKeep{}) {
			m.sessionKept = errors.Is(p.SessionErr, store.ErrSessionLineTooLong)
		}
		return m.logError("Couldn't restore the last session. Its file cannot be used."), false
	}
	if !p.HasSession {
		return m, false
	}
	sess := p.Session

	restored, dropped := false, 0
	for _, entry := range sess.Slots {
		if !addable(entry) {
			dropped++
			continue
		}
		var ok bool
		m, ok = m.anchorFromSession(entry.Chart)
		if !ok {
			dropped++
			continue
		}
		restored = true
		// A chart-file slot owes a re-check, issued against the slot just added;
		// it is tracked through any city change until it lands.
		if entry.Chart.Source.Kind == store.ChartKindFile && entry.Chart.Source.File != "" {
			m = m.trackRecheck(m.slots.All()[m.slots.AnchoredCount()-1])
		}
	}
	switch {
	case dropped == 0:
	case sess.Version == 1:
		// A chart: block that cannot stand on its own. Saying nothing here
		// would leave an astrologer looking at transits over the wrong chart
		// without a word about why.
		m = m.logNote("session", "natal not restored")
	default:
		m = m.logError("Couldn't restore %d of the open charts.", dropped)
	}

	if len(sess.UnknownBodies) > 0 {
		m = m.logNote("session", "skipped unknown bodies: %s", strings.Join(sess.UnknownBodies, ", "))
	}
	return m, restored
}

// addable reports whether a remembered slot can come back: it could stand on its
// own, and its place carries a timezone the workspace will accept. A hand-built
// entry can be Usable and still fail the second test; the store marks that one
// unusable itself.
func addable(e store.SessionSlot) bool {
	return e.Usable && e.Chart.Place.HasTimezone()
}

// anchorFromSession adds one remembered chart to the workspace, as an anchored
// slot carrying the record the next quit writes back, so that this launch's
// identity is the one it reads.
func (m Model) anchorFromSession(rec store.SessionChart) (Model, bool) {
	if m.slots == nil {
		return m, false
	}
	if err := m.slots.AddAnchored(rec, rec.When, rec.Place); err != nil {
		return m.logNote("slots", "anchor refused (%v)", err), false
	}
	m.cursorIdx = m.slots.AnchoredCount() // the floating slot sits behind the list
	return m, true
}

// applySessionSelections marks what the operator had marked, and runs LAST in
// startup — after the session has had its turn at anchoring.
//
// Adding a chart selects nothing; the session is the authority on what was
// selected, so every selection is cleared and then set as it was written,
// at most MaxSelectedSlots of them. The store keeps the first six when it reads
// a file; a hand-built session is held to the same cap here, and either way the
// count that was turned off is one Log error line. Marks of an entry that was
// dropped have nowhere to go and are dropped with it.
func (m Model) applySessionSelections(p ingest.StartupPerimeterReadMsg) Model {
	if !p.HasSession || m.slots == nil {
		return m
	}
	sess := p.Session
	floating := len(m.slots.All()) - 1
	for i := 0; i <= floating; i++ {
		m.slots.SetSlotSelected(i, false)
	}
	refused := sess.DroppedSelections
	apply := func(i int, selected bool, bodies []aspect.Body) {
		if selected && !m.slots.SetSlotSelected(i, true) {
			refused++
		}
		selectBodies(m.slots, i, bodies)
	}
	i := 0
	for _, entry := range sess.Slots {
		if !addable(entry) || i >= m.slots.AnchoredCount() {
			continue
		}
		apply(i, entry.Selected, entry.Bodies)
		i++
	}
	apply(floating, sess.FloatingSelected, sess.FloatingBodies)
	if refused > 0 {
		m = m.logError("Couldn't keep %d of the selections. At most six slots can be selected.", refused)
	}
	return m
}

// selectBodies marks each body once. Selection is a toggle, so a name the file
// happens to carry twice would mark a body and then unmark it — the collapse
// is what keeps a duplicate from meaning the opposite of what it says.
func selectBodies(slots *store.Slots, slotIdx int, bodies []aspect.Body) {
	seen := make(map[aspect.Body]struct{}, len(bodies))
	for _, b := range bodies {
		if _, dup := seen[b]; dup {
			continue
		}
		seen[b] = struct{}{}
		slots.ToggleBodySelection(slotIdx, b)
	}
}

// SessionChartRecheckedMsg carries the re-read of the chart file the session
// named: the row that file yields today, and the authoritative moment resolved
// from it.
//
// Remembered is what the snapshot anchored, carried along so the handler can
// compare without reaching back into startup state that a later pick may have
// moved. Found is false when the library has no row at that file any more —
// deleted, renamed, or dropped for a reason of its own.
//
// Slot is the workspace slot the re-read was issued for. The re-check is stale
// only if THAT slot was replaced or removed since — a pick of some other chart,
// which bumps the shared pick generation, leaves it alone — so an operator who
// re-picks this very chart while the re-read is in flight keeps their choice.
type SessionChartRecheckedMsg struct {
	Remembered store.SessionChart
	Row        store.NamedChart
	Found      bool
	When       time.Time
	Err        error
	Slot       *store.Slot
	// Houses and HasHouses are the system the re-read file names
	// (the specification C2).
	Houses    recipe.HouseSystem
	HasHouses bool
	// Label is the name and place the chart's reports print, from the same
	// read as When. It is empty when that read is not of the file the listing
	// row came from (the file was replaced between the two), so a name is never
	// paired with a chart it does not belong to.
	Label store.ReportLabel
	// Unplaced is set by a successful read that could not place the chart: the
	// row's name and place are then not the file's, and the slot is refused.
	Unplaced bool
}

// sessionRecheckCmd is the re-read a restored chart-file anchor owes, or nil when
// nothing is owed.
//
// Nothing is owed when the session anchored nothing, or when a record names
// no chart file -- a session written while YAML charts existed restores from its
// snapshot and has no file left to re-read. One re-read is owed per chart file
// slot, and each finds its slot again by the file it names.
func (m Model) sessionRecheckCmd(p ingest.StartupPerimeterReadMsg, restored bool) tea.Cmd {
	if !restored || !p.HasSession || p.SessionErr != nil {
		return nil
	}
	var cmds []tea.Cmd
	k := 0 // the restored slots are the addable entries, in order
	for _, entry := range p.Session.Slots {
		if !addable(entry) || m.slots == nil || k >= m.slots.AnchoredCount() {
			continue
		}
		slot := m.slots.All()[k]
		k++
		if entry.Chart.Source.Kind != store.ChartKindFile || entry.Chart.Source.File == "" {
			continue
		}
		cmds = append(cmds, recheckSessionChartCmd(entry.Chart, slot))
	}
	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	}
	return tea.Batch(cmds...)
}

// recheckSessionChartCmd re-reads the chart file the session named and resolves its
// authoritative moment, off the Update loop.
//
// It looks the file up by its FILENAME rather than by the id the session
// recorded, because the id is the half that does not survive the astrologer's
// own tools: astrolog erases a `;sextile-id:` comment every time it rewrites a
// chart file, after which the identity falls back to the filename stem and
// no longer equals what the session wrote down. The
// locator is what stayed put, so the locator is what looks it up — and the
// identifying content is compared afterwards, once there is something to
// compare against.
//
// The cost is one library read, which spawns nothing, plus the single astrolog
// spawn a pick of the same chart already pays. It runs after the first frame,
// never before it: internal/ingest is the perimeter and the isolation invariant
// forbids it importing astrolog or lookup at all, so "no astrolog before the
// first frame" is structural here rather than a promise.
// afterRecheckListing is a test seam between the re-check's library listing
// and its resolve. Production never reassigns it.
var afterRecheckListing = func() {}

func recheckSessionChartCmd(rec store.SessionChart, slot *store.Slot) tea.Cmd {
	return func() tea.Msg {
		lib, err := lookup.LoadChartLibrary()
		if err != nil {
			return SessionChartRecheckedMsg{Remembered: rec, Err: err, Slot: slot}
		}
		if lib.AtlasErr != nil {
			// With no place list no chart can be given a timezone, so every one
			// of them drops. Reporting "no longer in the library" would blame
			// the client's file for a fact about the atlas, which is the
			// wolf-crying that gets a warning ignored.
			return SessionChartRecheckedMsg{Remembered: rec, Err: lib.AtlasErr, Slot: slot}
		}
		afterRecheckListing()
		want := filepath.Base(rec.Source.File)
		for _, c := range lib.Charts {
			if filepath.Base(c.Source.File) != want {
				continue
			}
			p, err := lookup.ResolvePickedChart(context.Background(), c)
			// Name, place and moment of the answer are ONE read: a file replaced
			// since the listing is judged by what it holds now (C0).
			c = rowOfRead(c, p)
			return SessionChartRecheckedMsg{Remembered: rec, Row: c, Found: true, When: p.When, Err: err, Slot: slot, Houses: p.Houses, HasHouses: p.HasHouses, Label: labelOfRead(p), Unplaced: err == nil && !p.HasPlace}
		}
		return SessionChartRecheckedMsg{Remembered: rec, Slot: slot}
	}
}

// rowOfRead is the listing row corrected to the read made after it: when the
// read could place the chart, its title and casting place replace the row's,
// so the comparison with the snapshot and the moment come from one read. An
// untitled read keeps the file stem the listing showed.
func rowOfRead(row store.NamedChart, p lookup.PickedChart) store.NamedChart {
	if !p.HasPlace {
		return row
	}
	row.Place = p.Place
	if p.Label.Name != "" {
		row.Name, row.Untitled = p.Label.Name, false
		return row
	}
	if !row.Untitled {
		base := filepath.Base(row.Source.File)
		row.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	row.Untitled = true
	return row
}

// labelOfRead is the report label of a re-read: its own, when it could place
// the chart. The row was corrected to the same read (rowOfRead), so the label
// and the row cannot disagree.
func labelOfRead(p lookup.PickedChart) store.ReportLabel {
	if !p.HasPlace {
		return store.ReportLabel{}
	}
	return p.Label
}

// applySessionChartRecheck decides what the re-read means and, when the file
// won, re-anchors onto it.
//
// The rule it implements is the one thing an earlier change was not allowed to flip
// silently in either direction: an edited chart whose identifying content still
// matches is the SAME chart and its edit wins; one whose content no longer
// matches is a DIFFERENT chart at a familiar path, and that is the case an
// astrologer has to be told about rather than shown. Every other outcome keeps
// the snapshot, because the snapshot carries the chart the operator quit
// looking at and nothing here is worth taking that away.
//
// Silence is deliberate in exactly one case: the file re-read to the same
// moment, so nothing happened and there is nothing to say.
//
// Each line names the chart or the file it is about, through logIdent, which
// bounds that identifier rather than trusting it to be short. The Log's inner
// width is 64 cells and it truncates with NO ellipsis
// (`internal/ui/panels.go`), so an unbounded identifier makes a line only ever
// "short enough for the names tried so far" — and the first casualty is the
// substitution refusal, the line here that matters most.
//
// Dropping the identifier entirely was tried and was wrong: nothing in the
// persistent UI renders a chart's name. `chartSlotLine`
// (`internal/ui/header.go`) draws the moment, the zone and the PLACE, and the
// panel title is title-only, so outside the picker modal the name appears only
// on the one `session: restored <name> …` Log line — which the 6-row window
// drops as soon as anything else is written. A substitution warning that
// cannot be tied to a client is an audit trail with a hole in it.
//
// A COORDINATE-ONLY correction — the astrologer tightens a client's birth
// coordinates and leaves the recorded time alone — is reported here as a
// different chart and is NOT applied. That is a stated limit, not an
// oversight: nothing available without a spawn distinguishes it from a
// substitution, which is the hole a place-name-only comparison left open. The
// safe direction is to refuse and say so, and the astrologer's path is the one
// they already have — re-pick the chart with Ctrl+O, which resolves the file
// fresh and anchors it explicitly.
func applySessionChartRecheck(m Model, msg SessionChartRecheckedMsg) (Model, tea.Cmd) {
	// Whatever happens to this result, the re-check is no longer pending.
	target := m.resolveSlot(msg.Slot)
	m = m.dropRechecks(msg.Slot, nil)
	// The slot this re-read is about, found again by the file it was read from:
	// the list may have been reordered or this chart taken out since.
	idx := m.slotHolding(msg.Remembered)
	if idx < 0 {
		return m, nil
	}
	// Stale unless that slot STILL holds exactly the snapshot the re-read was
	// issued against — name, id, source, moment and place. A pick that put
	// anything else into it since (even a retitle) is the operator's choice, and
	// outranks a background re-read of what was there before.
	if slot := m.slots.All()[idx]; msg.Slot != nil && (slot != target || !holdsSnapshot(slot, msg.Remembered)) {
		return m, nil
	}
	switch {
	case msg.Err != nil:
		return m.logNote("session", "couldn't re-read %s", logIdent(msg.Remembered.Name, m.cfg)), nil
	case !msg.Found:
		// "no longer in the library" and not "the file is gone": the row can
		// also vanish with the file untouched on disk -- another chart file whose
		// stem slugifies to the same id took the row (library.go names it as
		// a drop), or the atlas can no longer place it. Claiming a file was
		// deleted when it was not is a false report about a client's data.
		return m.logNote("session", "%s is no longer in the library",
			logIdent(filepath.Base(msg.Remembered.Source.File), m.cfg)), nil
	}

	if msg.Row.Source != msg.Remembered.Source {
		// One file, one row: content read from another file is never this row's.
		return m, nil
	}
	if msg.Unplaced {
		// The re-read found the file but could not place it, so nothing of the
		// listing row (its name, its place) is the file's. Never fall back to it.
		return m.logNote("session", "that chart file can't be cast: %s", "no known place"), nil
	}
	cand := store.SessionChart{
		ID:     msg.Row.ID,
		Name:   msg.Row.Name,
		When:   msg.When,
		Place:  msg.Row.Place,
		Source: msg.Row.Source,
		Label:  msg.Label,
	}
	if reason := cand.UnusableReason(); reason != "" {
		// The library lists on a weaker bar than anchoring needs, so a
		// row can still list and no longer be castable. Same answer as the pick
		// path: refuse and name what is missing.
		//
		// This is checked BEFORE identity, and says nothing about whether the
		// chart was edited or replaced, because it cannot know: the fields
		// sameChart compares are among the ones that can be the missing fact.
		// A row with no place name is not a chart in a different place, and
		// announcing a substitution on that evidence would alarm an astrologer
		// about the wrong thing. The file is named because the file is what is
		// certainly true here.
		return m.logNote("session", "that chart file can't be cast: %s", reason), nil
	}
	if !sameChart(msg.Remembered, cand) {
		// A different person, or the same name somewhere else. Anchoring it
		// would put an astrologer in front of a chart they did not open,
		// wearing the name of one they did.
		return m.logNote("session", "%s now holds a different chart",
			logIdent(filepath.Base(msg.Row.Source.File), m.cfg)), nil
	}
	// The file is the same chart: what it says about its own houses is current.
	// A change of the system shown by this slot recasts it.
	before := m.ownHouses(m.slots.All()[idx]).hs
	wasUnresolved := m.unresolvedFor(m.slots.All()[idx]) != nil
	m.frames = m.frames.withFile(filepath.Base(msg.Row.Source.File), fileFrame{when: msg.When, houses: msg.Houses, has: msg.HasHouses})
	if cand.When.Equal(msg.Remembered.When) {
		// The file says what the snapshot said. Only its id may have moved:
		// a session written while `;sextile-id:` still named charts carries
		// that id, and a chart file's id is now its file stem (C3.20, C3.21).
		// Adopting it quietly is right — same file, same chart, nothing to
		// tell the operator — and it is what makes the next quit write the
		// id the library will resolve.
		if slot := m.slots.All()[idx]; cand.ID != msg.Remembered.ID && slot.Chart.ID == msg.Remembered.ID {
			slot.Chart.ID = cand.ID
		}
		// The read is also where the chart's report name and place come from
		// (a restored chart sends none until its file has been read again); the
		// reports recompute when they change.
		slot := m.slots.All()[idx]
		relabelled := slot.Chart.Label != cand.Label
		slot.Chart.Label = cand.Label
		if wasUnresolved || relabelled || m.ownHouses(m.slots.All()[idx]).hs != before {
			return m.recastHouses()
		}
		return m, nil
	}
	return m.reanchorEditedChart(idx, cand)
}

// sameChart reports whether two records describe the same chart, comparing
// everything a library read knows without spawning anything: the display label
// and the WHOLE casting place — city, country, coordinates and zone.
//
// The coordinates are the load-bearing half and comparing the city name alone
// was not enough. `Place.Name` is the nearest ATLAS city
// (`lookup.translateChartFiles` → `location.Nearest`), so two births thirty
// kilometres apart share it; the file's own precise `Lat`/`Lon` sit in the same
// record at no extra cost. Without them, a chart file overwritten with a different
// client who happens to share a display name and round to the same city read as
// the same chart, and the workspace re-anchored onto the wrong person while the
// Log said the reassuring thing. `PlaceInfo` is comparable, so this is the whole
// of it rather than a field list that can fall behind the struct.
//
// The MOMENT is deliberately not part of it — a changed moment is precisely
// what makes this an edit rather than a substitution, so including it would
// make every edit look like a different chart and the distinction an earlier change exists
// to draw would collapse.
//
// The ID is not part of it either, and that is the earlier change finding made
// load-bearing: one astrolog rewrite drops a `;sextile-id:` and changes the id
// of a file nobody touched otherwise. An identity check that failed on that
// would report "a different chart" for a chart the astrologer merely opened.
func sameChart(a, b store.SessionChart) bool {
	return a.Name == b.Name && a.Place == b.Place
}

// holdsSnapshot reports whether a slot holds exactly the chart record a restore
// made it from.
func holdsSnapshot(slot *store.Slot, rec store.SessionChart) bool {
	c := slot.Chart
	return c.Name == rec.Name && c.ID == rec.ID && c.Source == rec.Source &&
		slot.Time.Equal(rec.When) && slot.Location == rec.Place
}

// slotHolding is the list index of the anchored slot read from the file the
// remembered record names, or -1. The id is compared only when both sides have
// one: astrolog erases it from a chart file it rewrites, so the file is the locator.
func (m Model) slotHolding(remembered store.SessionChart) int {
	if m.slots == nil {
		return -1
	}
	all := m.slots.All()
	for i := 0; i < m.slots.AnchoredCount(); i++ {
		c := all[i].Chart
		if c.Source.Kind == remembered.Source.Kind && c.Source.File == remembered.Source.File {
			return i
		}
	}
	return -1
}

// reanchorEditedChart moves the anchored slot at idx onto the re-read chart,
// keeping every operator-owned mark that belongs to it.
//
// A pick starts a slot fresh — a different chart's marks should not survive
// onto a new one — and this is the same chart with a corrected moment, so the
// slot is replaced in place: its selection and body marks stay exactly as the
// operator left them, each slot's own flag included. Without that, an
// astrologer who fixed a birth minute would silently lose the bodies they had
// marked and have no way to know why.
func (m Model) reanchorEditedChart(idx int, cand store.SessionChart) (Model, tea.Cmd) {
	if m.slots == nil {
		return m, nil
	}
	if err := m.slots.ReplaceAnchored(idx, cand, cand.When, cand.Place); err != nil {
		return m.logNote("slots", "anchor refused (%v)", err), nil
	}

	m.chart = chartViewState{}
	m = m.suspendDetails()
	m = m.logNote("session", "%s was edited — re-anchored", logIdent(cand.Name, m.cfg))

	m.startup = startupComputing
	var recompute, wheel tea.Cmd
	m, recompute = m.bumpIdentity(surfaceText)
	// The corrected slot may be the one on the wheel: an open preview follows it
	// the way it follows a focus change.
	m, wheel = m.refreshWheelIfActive()
	return m, tea.Batch(recompute, wheel)
}

// logIdentWidth is how many cells a chart name or filename may spend on a Log
// line. The Log's inner width is 64; the widest fixed text around an identifier
// here is 37 cells, and the uncastable line — which spends its budget on the
// refusal reason instead — carries no identifier at all. 24 leaves every line
// at 62 or less, and TestSessionRecheckLinesFitTheLog proves it by driving each
// outcome with an absurd name rather than a realistic one.
const logIdentWidth = 24

// logIdent cuts a chart name or filename to logIdentWidth cells, marking the
// cut (text.go) the way the picker does for a long chart name. A marked cut
// beats the Log's own clip: the reader can see that a name was shortened
// rather than misread a truncation as the whole name.
func logIdent(s string, cfg Config) string {
	return cutRight(s, logIdentWidth, cfg)
}

// sessionPrefill is the name Save as offers (sessions_popup.go): the title of
// the first selected anchored chart in the Chart panel's order; else of the
// first anchored chart; else nothing. It reads the selection here, with the
// rest of what a session keeps, to name what is saved, not to display it.
func (m Model) sessionPrefill() string {
	if m.slots == nil {
		return ""
	}
	all := m.slots.All()
	for i := 0; i < m.slots.AnchoredCount(); i++ {
		if m.slots.SlotSelected(i) && all[i].Chart.Name != "" {
			return all[i].Chart.Name
		}
	}
	for i := 0; i < m.slots.AnchoredCount(); i++ {
		if all[i].Chart.Name != "" {
			return all[i].Chart.Name
		}
	}
	return ""
}
