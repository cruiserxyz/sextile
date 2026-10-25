package ui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// house_frames.go is the per-chart house system (the specification): which
// system each chart is cast in, where that answer came from, and what a choice
// does. The answer is resolved here, per slot, and handed to lookup and
// Astrolog as data on every call; m.lens stays the lens for everything else.

// houseSource says which rule of C1 produced a chart's own house system.
type houseSource int

const (
	houseFallback houseSource = iota // nothing named a system: Whole Sign
	houseDefault                     // preferences.yaml `frame: houses:`
	houseFile                        // the chart file's own line, as Astrolog reads it
	houseChosen                      // a session record (chosen, or preserved at a rename)
	houseSession                     // chosen for a chart with no file: until quit
)

// fileFrame is what a chart file said about itself when it was last read.
type fileFrame struct {
	when   time.Time
	houses recipe.HouseSystem
	has    bool
}

// castKey names one chart cast in one asked-for system, for the polar
// substitutions Astrolog reported (C7).
type castKey struct {
	floating bool
	when     int64
	lat, lon float64
	asked    recipe.HouseSystem
}

// castFact is one cast a compute made: the chart, the system asked for, and the
// system Astrolog's own header says it used ("" when it printed none).
type castFact struct {
	key   chartKey
	asked recipe.HouseSystem
	used  string
}

func (f castFact) mapKey() castKey { return makeCastKey(f.key, f.asked) }

func makeCastKey(k chartKey, asked recipe.HouseSystem) castKey {
	return castKey{floating: k.floating, when: k.when.UnixNano(), lat: k.place.Lat, lon: k.place.Lon, asked: asked}
}

// frameState is everything the model knows about house systems. Maps and slices
// are only ever replaced, never edited in place, so a Model value held elsewhere
// keeps its own view (the bodyRows rule).
type frameState struct {
	// records are the session's `frames:` entries, first-wins, in file order
	// then in the order they were made; the floating slot's has Floating set.
	records []store.FrameRecord
	// files is what each open chart file said about itself, by base name.
	files map[string]fileFrame
	// failed names the restored chart files whose startup read failed and has
	// not been redone, with the reason. Their chart has no house system yet.
	failed map[string]string
	// volatile holds a choice for a chart with no file: it applies until quit.
	volatile map[string]recipe.HouseSystem
	// volatileZ is volatile for the zodiac: a choice for a chart with no file.
	volatileZ map[string]recipe.Zodiac
	// def is the preferences default, when it named a usable system.
	def    recipe.HouseSystem
	defSet bool
	// defZ is the preferences default zodiac, when it named a usable one.
	defZ    recipe.Zodiac
	defZSet bool
	// logged holds the once-per-launch Log keys.
	logged map[string]bool
	// casts remembers the system Astrolog said it used for a cast (C7).
	casts map[castKey]string
	// renames are the preservation records written for rewrites in flight,
	// by write generation, so a failure can take them back.
	renames map[int]renameUndo
	// pruneOK is set by a chart-library listing that succeeded and listed at
	// least one chart: only then may a session write drop a record (C10).
	pruneOK bool
}

func (f frameState) find(file string, floating bool) (store.FrameRecord, bool) {
	for _, r := range f.records {
		if r.Floating == floating && (floating || r.File == file) {
			return r, true
		}
	}
	return store.FrameRecord{}, false
}

// with returns f with rec set (replacing a record of the same chart).
func (f frameState) with(rec store.FrameRecord) frameState {
	out := make([]store.FrameRecord, 0, len(f.records)+1)
	replaced := false
	for _, r := range f.records {
		if r.Floating == rec.Floating && (rec.Floating || r.File == rec.File) {
			out = append(out, rec)
			replaced = true
			continue
		}
		out = append(out, r)
	}
	if !replaced {
		out = append(out, rec)
	}
	f.records = out
	return f
}

// without returns f with the record of file removed.
func (f frameState) without(file string) frameState {
	out := make([]store.FrameRecord, 0, len(f.records))
	for _, r := range f.records {
		if !r.Floating && r.File == file {
			continue
		}
		out = append(out, r)
	}
	f.records = out
	return f
}

func (f frameState) withFile(file string, ff fileFrame) frameState {
	files := make(map[string]fileFrame, len(f.files)+1)
	for k, v := range f.files {
		files[k] = v
	}
	files[file] = ff
	f.files = files
	if _, bad := f.failed[file]; bad {
		failed := make(map[string]string, len(f.failed))
		for k, v := range f.failed {
			if k != file {
				failed[k] = v
			}
		}
		f.failed = failed
	}
	return f
}

func (f frameState) withLogged(key string) frameState {
	logged := make(map[string]bool, len(f.logged)+1)
	for k := range f.logged {
		logged[k] = true
	}
	logged[key] = true
	f.logged = logged
	return f
}

func (f frameState) withCast(k castKey, used string) frameState {
	casts := make(map[castKey]string, len(f.casts)+1)
	for key, v := range f.casts {
		casts[key] = v
	}
	casts[k] = used
	f.casts = casts
	return f
}

// slotFile is the chart file a slot was read from, as a base name; "" for the
// floating slot and for a chart with no file.
func slotFile(s *store.Slot) string {
	if s == nil || s.Floating || s.Chart.Source.Kind != store.ChartKindFile || s.Chart.Source.File == "" {
		return ""
	}
	return filepath.Base(s.Chart.Source.File)
}

// volatileKey identifies a fileless chart across a rebuild of the slot objects.
func volatileKey(s *store.Slot) string {
	return fmt.Sprintf("%s\x00%s\x00%d", s.Chart.ID, s.Chart.Name, s.Time.UnixNano())
}

// ownHouse is a chart's own resolution under C1.
type ownHouse struct {
	hs  recipe.HouseSystem
	src houseSource
	// rejected is set when a record exists for the chart's file but was made
	// for a different chart (C11a).
	rejected bool
	// unresolved is set when the chart's file could not be read at startup and
	// no record answers for it: the chart has no house system yet, and
	// nothing may be cast for it.
	unresolved bool
	why        string
}

// chartMoment is the birth moment the chart in s's file has: what the file's
// last read said, else the slot's own.
func (m Model) chartMoment(s *store.Slot) time.Time {
	if f := slotFile(s); f != "" {
		if ff, ok := m.frames.files[f]; ok && !ff.when.IsZero() {
			return ff.when
		}
	}
	return s.Time
}

// ownHouses resolves a chart's own system: its session record, else the system
// its chart file names, else the preferences default, else Whole Sign (C1).
func (m Model) ownHouses(s *store.Slot) ownHouse {
	var out ownHouse
	if s != nil {
		if hs, ok := m.frames.volatile[volatileKey(s)]; ok && s.Chart.Source.File == "" {
			return ownHouse{hs: hs, src: houseSession}
		}
		var rec store.FrameRecord
		var found bool
		if s.Floating {
			rec, found = m.frames.find("", true)
		} else if f := slotFile(s); f != "" {
			rec, found = m.frames.find(f, false)
		}
		if found && rec.HasHouses {
			if s.Floating || rec.When.Equal(m.chartMoment(s)) {
				return ownHouse{hs: rec.Houses, src: houseChosen}
			}
			out.rejected = true
		}
		if f := slotFile(s); f != "" {
			if ff, ok := m.frames.files[f]; ok && ff.has {
				out.hs, out.src = ff.houses, houseFile
				return out
			}
			if why, bad := m.frames.failed[f]; bad {
				out.hs, out.src, out.unresolved, out.why = recipe.WholeSign, houseFallback, true, why
				return out
			}
		}
	}
	if m.frames.defSet {
		out.hs, out.src = m.frames.def, houseDefault
		return out
	}
	out.hs, out.src = recipe.WholeSign, houseFallback
	return out
}

// ownZodiac is a chart's own zodiac (the specification C1): its session
// record, else the preferences default, else tropical. The chart file never
// names one: Astrolog's own writer puts no zodiac in a file, and a `_s` or
// `=s` a file does carry is not read.
func (m Model) ownZodiac(s *store.Slot) recipe.Zodiac {
	if s != nil {
		if z, ok := m.frames.volatileZ[volatileKey(s)]; ok && s.Chart.Source.File == "" {
			return z
		}
		var rec store.FrameRecord
		var found bool
		if s.Floating {
			rec, found = m.frames.find("", true)
		} else if f := slotFile(s); f != "" {
			rec, found = m.frames.find(f, false)
		}
		// A record made for another chart (a different birth moment in the same
		// file name) is no record, for the zodiac as for the houses (C11a).
		if found && rec.HasZodiac && (s.Floating || rec.When.Equal(m.chartMoment(s))) {
			if z, ok := recipe.ParseZodiacRecord(rec.Zodiac); ok {
				return z
			}
		}
	}
	if m.frames.defZSet {
		return m.frames.defZ
	}
	return recipe.Tropical
}

// slotIndexOf is s's index in the slot list, the focused slot's when s is not
// found (a finder then edits what the cursor is on).
func (m Model) slotIndexOf(s *store.Slot) int {
	if m.slots != nil {
		for i, x := range m.slots.All() {
			if x == s {
				return i
			}
		}
	}
	return m.cursorIdx
}

// lensFor is m.lens with s's own house system and zodiac. Every chart is shown
// in its own, selected or not, in a group or not (operator, 2026-10-24: "the
// other slots need to keep 'their' house system all the time ... same for
// lahiri"); only a comparison borrows another chart's, the frame's
// (comparisonLens).
func (m Model) lensFor(s *store.Slot) lookup.Lens {
	l := m.lens
	l.Houses = m.ownHouses(s).hs
	l.Zodiac = m.ownZodiac(s)
	return l
}

// comparisonLens is the lens a comparison is computed in: the frame's own
// house system and zodiac. The frame is the active pair's left chart, the base
// the cursor's chart is read in ("② in ①" on ② is ②'s bodies in ①'s houses),
// and the floating slot is never a base. A pair's cross aspects, its Details
// report, the bi-wheel and the Bodies house overlay take it; every chart's own rows keep lensFor. Outside a pair it is the cursor's
// own lens.
func (m Model) comparisonLens() lookup.Lens {
	if sc := m.deriveScene(); sc.mode == sceneComparing && sc.frame != nil {
		return m.lensFor(sc.frame)
	}
	return m.viewLens()
}

// showLens is lensFor(s) with the slot's selected bodies as Objects.Only: the
// lens of the chart's OWN wheel, Aspects list and Details report, where a
// selected body shows only itself and its fellows. A slot with no selected
// body gets lensFor(s) unchanged, so nothing selected shows all, as before.
//
// Not for the Bodies table (it stays complete so the selection can grow), the
// timeline, the body chooser's probes, the template tab or any comparison
// surface: those keep viewLens or lensFor.
func (m Model) showLens(s *store.Slot) lookup.Lens {
	l := m.lensFor(s)
	if m.slots == nil || s == nil {
		return l
	}
	if sel := m.slots.SelectedBodies(m.slotIndexOf(s)); len(sel) > 0 {
		l.Objects.Only = recipe.NewBodySet(sel...)
	}
	return l
}

// viewLens is the lens of the screen: the cursor's chart in its own house
// system and zodiac.
func (m Model) viewLens() lookup.Lens { return m.lensFor(m.focusedSlot()) }

// parseHouseName reads a preferences value: Astrolog's header name or the
// approved display name.
func parseHouseName(v string) (recipe.HouseSystem, bool) {
	v = strings.TrimSpace(v)
	if hs, ok := recipe.ParseHouseSystem(v); ok {
		return hs, true
	}
	for n := 0; n < recipe.HouseSystemCount; n++ {
		if strings.EqualFold(lookup.HouseName(recipe.HouseSystem(n)), v) {
			return recipe.HouseSystem(n), true
		}
	}
	return 0, false
}

// applyFrameConfig reads the preferences defaults (C1, C11d). A value that is
// not a house system, or not a zodiac in the form a record carries
// (the specification C6), leaves Whole Sign, or tropical, in force and is
// said once. A `base:` key left over from the retired base switch is ignored.
func (m Model) applyFrameConfig(cfg store.FrameConfig) Model {
	if cfg.HousesPresent {
		if hs, ok := parseHouseName(cfg.Houses); ok {
			m.frames.def, m.frames.defSet = hs, true
		} else if !m.frames.logged["bad default"] {
			m.frames = m.frames.withLogged("bad default")
			m = m.logNote("houses", "preferences: %s is not a house system, using Whole Sign", logIdent(cfg.Houses, m.cfg))
		}
	}
	if cfg.ZodiacPresent {
		if z, ok := recipe.ParseZodiacRecord(strings.TrimSpace(cfg.Zodiac)); ok {
			m.frames.defZ, m.frames.defZSet = z, true
		} else if !m.frames.logged["bad zodiac default"] {
			m.frames = m.frames.withLogged("bad zodiac default")
			m = m.logNote("zodiac", "preferences: %s is not a zodiac, using tropical", logIdent(cfg.Zodiac, m.cfg))
		}
	}
	return m
}

// loadFrames puts the session's records and the startup file reads in place.
func (m Model) loadFrames(p ingest.StartupPerimeterReadMsg) Model {
	if p.SessionErr != nil {
		return m
	}
	if p.HasSession {
		m.frames.records = append([]store.FrameRecord(nil), p.Session.Frames...)
		for _, n := range p.Session.FrameNotes {
			m = m.logNote("session", "%s", n)
		}
	}
	for file, r := range p.FileReads {
		m.frames = m.frames.withFile(file, fileFrame{when: r.When, houses: r.Houses, has: r.HasHouses})
	}
	if len(p.FileReadFailures) > 0 {
		failed := make(map[string]string, len(p.FileReadFailures))
		for k, v := range p.FileReadFailures {
			failed[k] = v
		}
		m.frames.failed = failed
	}
	return m
}

// houseChoiceLabel names a slot on a Log line.
func (m Model) slotLabel(s *store.Slot) string {
	if s == nil || s.Floating || s.Chart.Name == "" {
		return "the floating chart"
	}
	return logIdent(s.Chart.Name, m.cfg)
}

// frameChoice is what one door of the finder changes about a chart: a house
// system or a zodiac. Both go through chooseFrame, so a choice of either is
// recorded, saved and recast the same way and no second write path exists.
type frameChoice struct {
	// noun names the choice on the Log ("houses", "zodiac") and in the failure
	// line ("house", "zodiac").
	lane, noun string
	// set puts the choice into a session record.
	set func(rec *store.FrameRecord)
	// setVolatile remembers the choice until quit, for a chart with no file.
	setVolatile func(f *frameState, key string)
}

// chooseHouseSystem is the one door a house choice comes through (the finder
// calls it): it records the choice for the chart in slot slotIdx, writes the
// session at once (C8), and recasts. The choice applies on screen even when
// the write fails.
func (m Model) chooseHouseSystem(slotIdx int, hs recipe.HouseSystem) (Model, tea.Cmd) {
	if !hs.Valid() {
		return m, nil
	}
	return m.chooseFrame(slotIdx, frameChoice{
		lane: "houses", noun: "house",
		set: func(rec *store.FrameRecord) {
			rec.Houses, rec.HasHouses, rec.From = hs, true, store.FrameFromChosen
		},
		setVolatile: func(f *frameState, key string) {
			vol := make(map[string]recipe.HouseSystem, len(f.volatile)+1)
			for k, v := range f.volatile {
				vol[k] = v
			}
			vol[key] = hs
			f.volatile = vol
		},
	})
}

// chooseZodiac is the zodiac's door (the specification C7): the same path
// as a house choice, writing the record's `zodiac:` and leaving its houses as
// they were.
func (m Model) chooseZodiac(slotIdx int, z recipe.Zodiac) (Model, tea.Cmd) {
	if !z.Valid() {
		return m, nil
	}
	return m.chooseFrame(slotIdx, frameChoice{
		lane: "zodiac", noun: "zodiac",
		set: func(rec *store.FrameRecord) { rec.Zodiac, rec.HasZodiac = z.RecordName(), true },
		setVolatile: func(f *frameState, key string) {
			vol := make(map[string]recipe.Zodiac, len(f.volatileZ)+1)
			for k, v := range f.volatileZ {
				vol[k] = v
			}
			vol[key] = z
			f.volatileZ = vol
		},
	})
}

func (m Model) chooseFrame(slotIdx int, c frameChoice) (Model, tea.Cmd) {
	if m.slots == nil {
		return m, nil
	}
	all := m.slots.All()
	if slotIdx < 0 || slotIdx >= len(all) {
		return m, nil
	}
	slot := all[slotIdx]
	remembered := true
	switch {
	case slot.Floating:
		rec, _ := m.frames.find("", true)
		rec.Floating, rec.File, rec.When = true, "", time.Time{}
		c.set(&rec)
		m.frames = m.frames.with(rec)
	case slotFile(slot) != "":
		file := slotFile(slot)
		rec, had := m.frames.find(file, false)
		moment := m.chartMoment(slot)
		if had && !rec.When.Equal(moment) {
			// A record made for another chart carries nothing over to this one.
			rec = store.FrameRecord{}
		}
		rec.File, rec.Floating, rec.When = file, false, moment
		c.set(&rec)
		m.frames = m.frames.with(rec)
	default:
		remembered = false
		c.setVolatile(&m.frames, volatileKey(slot))
		m = m.logNote(c.lane, "%s: this choice can't be remembered, the chart has no file", m.slotLabel(slot))
	}
	if remembered {
		// A launch whose refused session file could not be kept writes nothing
		// (the specification C4): the launch already said so. Nor does a
		// second window, which said so at start.
		if !m.sessionKept && m.ownsSession() {
			if err := m.writeSessionNow(); err != nil {
				m = m.logError("Couldn't save the %s choice. It will be tried again at quit.", c.noun)
			}
		}
	}
	return m.recastHouses()
}

// openFramesNow is what Save defaults freezes: for every open anchored chart
// that has a file, the houses and zodiac it is shown in at this moment, for each
// of the two it has no record of its own (none, or one made for another chart
// moment). Each entry is a record naming only those fields, with the chart's
// file and moment. The defaults are not applied to the model by a save, so what
// ownHouses and ownZodiac answer is what the row tag shows. The floating slot
// and a chart with no file have no entry.
func (m Model) openFramesNow() []store.FrameRecord {
	if m.slots == nil {
		return nil
	}
	var out []store.FrameRecord
	for _, s := range m.slots.All() {
		file := slotFile(s)
		if file == "" {
			continue
		}
		moment := m.chartMoment(s)
		rec, had := m.frames.find(file, false)
		if had && !rec.When.Equal(moment) {
			// A record made for another chart carries nothing over to this one.
			rec = store.FrameRecord{}
		}
		keep := store.FrameRecord{File: file, When: moment}
		if own := m.ownHouses(s); !rec.HasHouses && !own.unresolved {
			keep.Houses, keep.HasHouses, keep.From = own.hs, true, store.FrameFromChosen
		}
		if !rec.HasZodiac {
			keep.Zodiac, keep.HasZodiac = m.ownZodiac(s).RecordName(), true
		}
		if keep.HasHouses || keep.HasZodiac {
			out = append(out, keep)
		}
	}
	return out
}

// keepFrames writes the entries openFramesNow made when Save defaults was
// pressed, for the charts that are still open with the same file and moment:
// a chart opened since is not one of them and takes the default. An entry
// fills only what the chart still has no record of, with the frame it had at
// the keypress, and a record that exists is never changed. It reports whether a
// record was added.
func (m Model) keepFrames(snapshot []store.FrameRecord) (Model, bool) {
	if m.slots == nil {
		return m, false
	}
	added := false
	for _, e := range snapshot {
		open := false
		for _, s := range m.slots.All() {
			if slotFile(s) == e.File && m.chartMoment(s).Equal(e.When) {
				open = true
				break
			}
		}
		if !open {
			continue
		}
		rec, had := m.frames.find(e.File, false)
		if had && !rec.When.Equal(e.When) {
			rec = store.FrameRecord{}
		}
		changed := false
		if e.HasHouses && !rec.HasHouses {
			rec.Houses, rec.HasHouses, rec.From = e.Houses, true, e.From
			changed = true
		}
		if e.HasZodiac && !rec.HasZodiac {
			rec.Zodiac, rec.HasZodiac = e.Zodiac, true
			changed = true
		}
		if !changed {
			continue
		}
		rec.File, rec.Floating, rec.When = e.File, false, e.When
		m.frames = m.frames.with(rec)
		added = true
	}
	return m, added
}

// recastHouses recasts everything on screen after the house system or zodiac a
// chart is shown in changed: the text frame and its columns, Details, the timeline's
// searches and an open wheel.
func (m Model) recastHouses() (Model, tea.Cmd) {
	m.chart.columns, m.chart.columnsFor = nil, nil
	m = m.waitDetailsForText()
	var textCmd, timelineCmd, wheelCmd tea.Cmd
	m, textCmd = m.bumpIdentity(surfaceText)
	m, timelineCmd = m.syncTimeline()
	m, wheelCmd = m.refreshWheelIfActive()
	return m, tea.Batch(textCmd, timelineCmd, wheelCmd)
}

// writeSessionNow writes the session the way quit does (C8). A launch whose
// refused file could not be kept writes nothing, and says so as an error so a
// caller that depends on the write (a rename's preservation) does not go on.
// The caller puts a failure on the error lane; the quit write retries.
func (m Model) writeSessionNow() error {
	if m.slots == nil || m.cantStart.locked() {
		return errors.New("no workspace to save")
	}
	if m.cfg.SecondWindow {
		return errSecondWindow
	}
	if m.sessionKept {
		return errors.New("the refused session file was left in place")
	}
	return store.SaveSession(m.sessionSnapshot())
}

// frameRecordsToSave is the session's `frames:` as the next write makes it:
// every record, minus those whose chart file is gone once a library listing has
// succeeded (C10). A file that exists but was dropped from a listing keeps its
// record, and so does any file whose existence could not be judged.
func (m Model) frameRecordsToSave() []store.FrameRecord {
	var out []store.FrameRecord
	for _, r := range m.frames.records {
		if !r.Floating && m.frames.pruneOK {
			if _, err := os.Lstat(filepath.Join(store.ChartsDir(), r.File)); errors.Is(err, fs.ErrNotExist) {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// rowTag is the Charts row's tag: the house code of the system the row is cast
// in (the one Astrolog used, with `*`, where it substituted) and the zodiac.
func (m Model) rowTag(s *store.Slot) string {
	lens := m.lensFor(s)
	code := lens.HouseAbbrev()
	if used, ok := m.knownSubstitution(s, lens.Houses); ok {
		code = lookup.HouseCode(used) + "*"
	}
	return code + lens.ZodiacAbbrev()
}

// knownSubstitution reports the system Astrolog used for s's chart when it was
// not the one asked for.
func (m Model) knownSubstitution(s *store.Slot, asked recipe.HouseSystem) (recipe.HouseSystem, bool) {
	if s == nil || len(m.frames.casts) == 0 {
		return 0, false
	}
	key := chartKey{when: s.Time, place: s.Location}
	if s.Floating {
		key = floatingKey(s.Time, s.Location)
	}
	name, ok := m.frames.casts[makeCastKey(key, asked)]
	if !ok {
		return 0, false
	}
	used, ok := recipe.ParseHouseSystem(name)
	if !ok || used == asked {
		return 0, false
	}
	return used, true
}

// commitCasts remembers what each cast of a frame reported and says, once per
// chart and system, where Astrolog used another system than the one asked (C7).
func (m Model) commitCasts(casts []castFact) Model {
	for _, c := range casts {
		if c.used == "" {
			continue
		}
		m.frames = m.frames.withCast(c.mapKey(), c.used)
		used, ok := recipe.ParseHouseSystem(c.used)
		if !ok || used == c.asked {
			continue
		}
		key := fmt.Sprintf("sub|%s", fmt.Sprint(c.mapKey()))
		if m.frames.logged[key] {
			continue
		}
		m.frames = m.frames.withLogged(key)
		label := "the floating chart"
		if m.slots != nil {
			for _, s := range m.slots.All() {
				k := chartKey{when: s.Time, place: s.Location, floating: s.Floating, label: s.Chart.Label}
				if k.equal(c.key) {
					label = m.slotLabel(s)
					break
				}
			}
		}
		m = m.logNote("houses", "%s: %s asked, Astrolog used %s", label,
			lookup.HouseName(c.asked), lookup.HouseName(used))
	}
	return m
}

// noteHouseSources says, once per chart per launch, where the system the
// screen is cast in came from when that was the chart file or the preferences
// default (C12), and names a record that was made for another chart (C11a).
func (m Model) noteHouseSources() Model {
	s := m.focusedSlot()
	if s == nil || m.slots == nil {
		return m
	}
	own := m.ownHouses(s)
	id := slotFile(s)
	if id == "" {
		id = "floating"
	}
	if own.rejected && !m.frames.logged["rejected|"+id] {
		m.frames = m.frames.withLogged("rejected|" + id)
		m = m.logNote("houses", "%s: the saved house choice was made for a different chart and is ignored", m.slotLabel(s))
	}
	var from string
	switch own.src {
	case houseFile:
		from = "its chart file"
	case houseDefault:
		from = "preferences"
	default:
		return m
	}
	if m.frames.logged["source|"+id] {
		return m
	}
	m.frames = m.frames.withLogged("source|" + id)
	return m.logNote("houses", "%s: %s houses, from %s", m.slotLabel(s), lookup.HouseName(own.hs), from)
}

// castsOf is the cast fact of one lookup: nothing for a failed one.
func castsOf(key chartKey, lens lookup.Lens, r lookup.Result, err error) []castFact {
	if err != nil {
		return nil
	}
	return []castFact{{key: key, asked: lens.Houses, used: r.HousesUsed}}
}

// readPerimeterCmd is the cold-start perimeter read plus what each restored
// chart file says about itself. The files are read BEFORE the first frame (in
// parallel, one Astrolog read each) so the first cast is already in the system
// the file names; ingest cannot spawn Astrolog, so the UI does it.
//
// A second window reads the session without moving a refused file aside: the
// file is the other window's.
func readPerimeterCmd(secondWindow bool) tea.Cmd {
	read := ingest.ReadStartupPerimeter()
	if secondWindow {
		read = ingest.ReadStartupPerimeterReadOnly()
	}
	return func() tea.Msg {
		p, ok := read().(ingest.StartupPerimeterReadMsg)
		if !ok {
			return p
		}
		p.FileReads, p.FileReadFailures = readSessionFiles(p)
		return p
	}
}

// readSessionFiles reads every restorable chart file the session names. A file
// that cannot be read is left out: its chart then falls to the next rule of C1
// and the re-check that follows the first frame says what it found.
func readSessionFiles(p ingest.StartupPerimeterReadMsg) (map[string]ingest.ChartFileRead, map[string]string) {
	if !p.HasSession || p.SessionErr != nil {
		return nil, nil
	}
	var files []string
	seen := map[string]bool{}
	for _, e := range p.Session.Slots {
		if !addable(e) || e.Chart.Source.Kind != store.ChartKindFile || e.Chart.Source.File == "" {
			continue
		}
		f := filepath.Base(e.Chart.Source.File)
		if !seen[f] {
			seen[f] = true
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return nil, nil
	}
	out := make(map[string]ingest.ChartFileRead, len(files))
	failed := map[string]string{}
	results := make([]ingest.ChartFileRead, len(files))
	okRead := make([]bool, len(files))
	reasons := make([]string, len(files))
	var wg sync.WaitGroup
	for i, f := range files {
		wg.Add(1)
		go func(i int, f string) {
			defer wg.Done()
			pc, err := readChartFile(context.Background(), f)
			if err != nil {
				reasons[i] = firstLine(err.Error())
				return
			}
			results[i], okRead[i] = ingest.ChartFileRead{When: pc.When, Houses: pc.Houses, HasHouses: pc.HasHouses}, true
		}(i, f)
	}
	wg.Wait()
	for i, f := range files {
		if okRead[i] {
			out[f] = results[i]
		} else {
			failed[f] = reasons[i]
		}
	}
	return out, failed
}

// unresolvedHouses is the failure a cast reports when the chart it would cast
// has no house system yet: its chart file could not be read.
type unresolvedHouses struct {
	file, why string
}

func (e *unresolvedHouses) Error() string {
	return fmt.Sprintf("the house system of %s is not known: %s", e.file, e.why)
}

// reason is the error-lane sentence for the failure.
func (e *unresolvedHouses) reason() string {
	return "The house system in " + e.file + " could not be read."
}

// unresolvedFor returns the failure for a chart the screen is cast for, or nil
// when every house system it needs is known: s's own and, while comparing,
// every shown chart's (each is cast in its own, and the frame's carries the
// comparison).
func (m Model) unresolvedFor(s *store.Slot) *unresolvedHouses {
	need := []*store.Slot{s}
	if sc := m.deriveScene(); sc.mode == sceneComparing {
		need = append(need, sc.shown...)
	}
	for _, src := range need {
		if own := m.ownHouses(src); own.unresolved {
			return &unresolvedHouses{file: slotFile(src), why: own.why}
		}
	}
	return nil
}

// readChartFile is the chart-file read as a seam for tests.
var readChartFile = lookup.ReadChartFile

// readChartFileIn is the same read in a named directory, the one the confirmed
// rename uses so it never asks the resolver again.
var readChartFileIn = lookup.ReadChartFileIn

// renameCmd is the rename's first step (the specification C9). Astrolog
// rewrites the chart file, and a rewrite drops the file's house line, so a
// chart whose file names a system the session does not already hold for it
// needs a record (`from: file`) in the session BEFORE the rewrite.
//
// The record belongs to the model, so a quit at any moment writes a session
// consistent with the file on disk:
//
//	(a) this command reads the file's system (no write);
//	(b) when a record is needed, its result (RenameReadMsg) is handled in Update,
//	    which adds the record to the model and writes the session in the same
//	    step; if that write fails the rename is refused;
//	(c) only then is the rewrite issued (retitleCmd);
//	(d) a failed rewrite removes the record and writes the session back.
//
// A record counts only if its `when` is the fresh read's moment; any other is
// treated as no record. A chart that needs no record goes straight to the
// rewrite inside this command, so its result is the finished write.
//
// A second window counts no record: one it made itself is not on disk, and it
// cannot write one, so a file that names a system is refused at step (b).
func (m Model) renameCmd(gen int, dir string, c store.NamedChart, seen, title, place string) tea.Cmd {
	file := filepath.Base(c.Source.File)
	rec, haveRec := m.frames.find(file, false)
	haveRec = haveRec && !m.cfg.SecondWindow
	// The read is always fresh: nothing cached about the file is trusted, since
	// it may have been replaced since it was last read.
	return func() tea.Msg {
		read, err := readChartFileIn(context.Background(), dir, c.Source.File)
		if err != nil {
			return TitleWrittenMsg{Gen: gen, Chart: c, Err: err}
		}
		if !read.HasHouses || (haveRec && rec.HasHouses && rec.When.Equal(read.When)) {
			return retitleCmd(gen, dir, c, seen, title, place)()
		}
		return RenameReadMsg{Gen: gen, Chart: c, Dir: dir, Seen: seen, Title: title, Place: place, Read: read}
	}
}

// RenameReadMsg is the file read of a rename whose chart needs its house system
// preserved: step (b) of renameCmd.
type RenameReadMsg struct {
	Gen          int
	Chart        store.NamedChart
	Dir, Seen    string
	Title, Place string
	Read         lookup.PickedChart
}

// renameUndo is what step (d) puts back.
type renameUndo struct {
	file  string
	had   bool
	prior store.FrameRecord
}

// applyRenameRead is step (b) and (c): the record goes into the model and the
// session in one synchronous step, then the rewrite is issued.
func applyRenameRead(m Model, msg RenameReadMsg) (Model, tea.Cmd) {
	if msg.Gen != m.titleGen || m.startup != startupTitleOpen || !m.title.saving {
		// Esc walked away before anything was written: nothing to undo.
		m.abandonedTitles, _ = reclaim(m.abandonedTitles, msg.Gen)
		return m, nil
	}
	file := filepath.Base(msg.Chart.Source.File)
	prior, had := m.frames.find(file, false)
	keep := prior
	if had && !prior.When.Equal(msg.Read.When) {
		// A record for another chart is no record: nothing of it carries over.
		keep = store.FrameRecord{}
	}
	keep.File, keep.Floating, keep.When = file, false, msg.Read.When
	keep.Houses, keep.HasHouses, keep.From = msg.Read.Houses, true, store.FrameFromFile
	prev := m.frames
	m.frames = m.frames.with(keep)
	if err := m.writeSessionNow(); err != nil {
		m.frames = prev
		m.title.saving = false
		if errors.Is(err, errSecondWindow) {
			m.title.refusal = "Rename it in the first Sextile window: only that one can keep its house system."
			return m.logError("Couldn't rename that chart. Its house system can only be kept by the first Sextile window."), nil
		}
		m.title.refusal = "The house system could not be saved first."
		return m.logError("Couldn't rename that chart. Its house system could not be saved first."), nil
	}
	undo := make(map[int]renameUndo, len(m.frames.renames)+1)
	for k, v := range m.frames.renames {
		undo[k] = v
	}
	undo[msg.Gen] = renameUndo{file: file, had: had, prior: prior}
	m.frames.renames = undo
	return m, retitleCmd(msg.Gen, msg.Dir, msg.Chart, msg.Seen, msg.Title, msg.Place)
}

// settlePreservation is step (d): a rewrite that failed takes back the record
// written for it and puts the session back at once, so a relaunch still reads
// the chart file's own house line. A finished one keeps the record.
func (m Model) settlePreservation(msg TitleWrittenMsg) Model {
	u, ok := m.frames.renames[msg.Gen]
	if !ok {
		return m
	}
	left := make(map[int]renameUndo, len(m.frames.renames))
	for k, v := range m.frames.renames {
		if k != msg.Gen {
			left[k] = v
		}
	}
	m.frames.renames = left
	if msg.Err == nil {
		return m
	}
	if u.had {
		m.frames = m.frames.with(u.prior)
	} else {
		m.frames = m.frames.without(u.file)
	}
	if err := m.writeSessionNow(); err != nil {
		return m.logError("Couldn't restore the session after the failed rename.")
	}
	return m
}

// recastAfter adds a recast of everything on screen to a command already
// issued, when the house system a chart is shown in changed.
func recastAfter(m Model, cmd tea.Cmd, recast bool) (Model, tea.Cmd) {
	if !recast {
		return m, cmd
	}
	m, more := m.recastHouses()
	return m, tea.Batch(cmd, more)
}
