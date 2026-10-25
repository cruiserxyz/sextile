package ui

import (
	"context"
	"reflect"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// chartKey names one chart a Bodies column can show: an anchored chart by its
// moment and place, the floating chart by the transit moment and place the
// operator is casting for.
type chartKey struct {
	floating bool
	when     time.Time
	place    location.PlaceInfo
	// label is the name and place its reports print. It is part of the key: two
	// library charts with one moment and place are two charts, and a renamed
	// chart is not the chart a cached report was made for
	// (the specification N10, N21).
	label store.ReportLabel
}

func anchoredKey(a store.AnchorChart) chartKey {
	return chartKey{when: a.When, place: a.Place, label: a.Label}
}

func floatingKey(when time.Time, place location.PlaceInfo) chartKey {
	return chartKey{floating: true, when: when, place: place}
}

func (k chartKey) anchor() store.AnchorChart {
	return store.AnchorChart{When: k.when, Place: k.place, Label: k.label}
}

func (k chartKey) equal(o chartKey) bool {
	return k.floating == o.floating && k.when.Equal(o.when) && k.place == o.place && k.label == o.label
}

// columnID is one shown slot as a column's identity: the slot and the chart
// it is cast as.
type columnID struct {
	slot *store.Slot
	key  chartKey
	// label names the chart on a failure line (its numeral and name).
	label string
	// houses is the system the column is cast in: a house choice changes it,
	// and columns cast in another system are not the columns for this scene.
	houses recipe.HouseSystem
	// zodiac is the zodiac the column is cast in, for the same reason.
	zodiac recipe.Zodiac
}

// columnsFor is the shown set a frame's columns were computed for, in numeral
// order.
type columnsFor []columnID

func (c columnsFor) equal(o columnsFor) bool {
	if len(c) != len(o) {
		return false
	}
	for i := range c {
		if c[i].slot != o[i].slot || !c[i].key.equal(o[i].key) || c[i].houses != o[i].houses || c[i].zodiac != o[i].zodiac {
			return false
		}
	}
	return true
}

// bodyColumn is one shown chart's Bodies column: its positions as that chart
// shows them alone, or the error that left the column blank.
type bodyColumn struct {
	key       chartKey
	label     string
	positions []aspect.Position
	// cusps is the chart's own house frame, from the same lookup.
	cusps aspect.Cusps
	err   error
}

// shownColumns is the shown set of a scene as columns would name them.
// floatTime is the floating chart's moment: the compute's target when
// building a frame, the committed slot time when judging one on screen.
//
// Each column is cast in its chart's own house system. Its zodiac is its own on
// the own tab and the frame's while a pair is on screen: in an "X in Y" view
// every degree Bodies shows is in Y's zodiac, as the wheel draws it (operator,
// 2026-10-24, amendment (b) of the view rule).
func (m Model) shownColumns(sc scene, floatTime time.Time) columnsFor {
	out := make(columnsFor, 0, len(sc.shown))
	for i, s := range sc.shown {
		key := anchoredKey(anchorOf(s))
		if s.Floating {
			key = floatingKey(floatTime, m.place)
		}
		zodiac := m.ownZodiac(s)
		if sc.pair != nil {
			zodiac = m.ownZodiac(sc.frame)
		}
		out = append(out, columnID{slot: s, key: key, label: slotLabel(i+1, s, m.cfg), houses: m.ownHouses(s).hs, zodiac: zodiac})
	}
	return out
}

// bodyColumnsForScene returns the committed columns only when they were
// computed for the scene on screen now: the same slots, each cast as it is
// now. Otherwise the panel is waiting for its frame and shows no columns.
func (m Model) bodyColumnsForScene() ([]bodyColumn, bool) {
	// Between a scene change and its frame the panels show the scene they last
	// swapped to, so its columns are the ones that match (specs/scene-swap-
	// together.md rule 8): never another scene than Aspects shows.
	if m.holding() {
		held := m.shown
		if !held.comparing || len(held.chart.columns) == 0 || !held.chart.columnsFor.equal(held.columns) {
			return nil, false
		}
		return held.chart.columns, true
	}
	sc := m.deriveScene()
	if sc.mode != sceneComparing || m.slots == nil || len(m.chart.columns) == 0 {
		return nil, false
	}
	if !m.chart.columnsFor.equal(m.shownColumns(sc, m.slots.Floating().Time)) {
		return nil, false
	}
	return m.chart.columns, true
}

// resultEntry is one chart's lookup result, valid for one lens.
type resultEntry struct {
	key  chartKey
	lens lookup.Lens
	res  lookup.Result
}

// resultCache keeps the position lookups of the charts on screen so a frame
// recomputes only the charts that moved. Ephemeral UI state: never stored. It
// is only ever replaced, never mutated in place, so a copy handed to a compute
// stays valid.
type resultCache []resultEntry

func (c resultCache) get(key chartKey, lens lookup.Lens) (lookup.Result, bool) {
	for _, e := range c {
		if e.key.equal(key) && reflect.DeepEqual(e.lens, lens) {
			return e.res, true
		}
	}
	return lookup.Result{}, false
}

// merged returns c with the entries of fresh added, one entry per chart.
func (c resultCache) merged(fresh resultCache) resultCache {
	out := make(resultCache, 0, len(c)+len(fresh))
	for _, e := range c {
		replaced := false
		for _, f := range fresh {
			if f.key.equal(e.key) {
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, e)
		}
	}
	return append(out, fresh...)
}

// pruned returns the entries whose chart is in the set.
func (c resultCache) pruned(set columnsFor) resultCache {
	out := make(resultCache, 0, len(set))
	for _, e := range c {
		for _, id := range set {
			if id.key.equal(e.key) {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

// resultFetcher answers a compute's position lookups from the cache when the
// chart and lens match, and from Astrolog otherwise. A hit spawns nothing, so
// it carries no Commands; it keeps its Unavailable.
type resultFetcher struct {
	cache resultCache
	fresh resultCache
	// casts is the house system Astrolog reported for each chart looked up.
	casts []castFact
}

func (f *resultFetcher) hit(key chartKey, lens lookup.Lens) (lookup.Result, bool) {
	if r, ok := f.fresh.get(key, lens); ok {
		return r, true
	}
	return f.cache.get(key, lens)
}

func (f *resultFetcher) store(key chartKey, lens lookup.Lens, r lookup.Result) {
	kept := r
	kept.Commands = nil
	f.fresh = append(f.fresh, resultEntry{key: key, lens: lens, res: kept})
}

// natal is firstCallNatalPositions through the cache.
func (f *resultFetcher) natal(ctx context.Context, probers *lookup.Probers, a store.AnchorChart, lens lookup.Lens) (lookup.Result, error) {
	key := anchoredKey(a)
	if r, ok := f.hit(key, lens); ok {
		f.casts = append(f.casts, castsOf(key, lens, r, nil)...)
		return r, nil
	}
	r, err := firstCallNatalPositions(ctx, probers, a, lens)
	if err == nil {
		f.store(key, lens, r)
	}
	f.casts = append(f.casts, castsOf(key, lens, r, err)...)
	return r, err
}

// transit is firstCallTransits through the cache.
func (f *resultFetcher) transit(ctx context.Context, probers *lookup.Probers, when time.Time, place location.PlaceInfo, lens lookup.Lens) (lookup.Result, error) {
	key := floatingKey(when, place)
	if r, ok := f.hit(key, lens); ok {
		f.casts = append(f.casts, castsOf(key, lens, r, nil)...)
		return r, nil
	}
	r, err := firstCallTransits(ctx, probers, when, place, lens)
	if err == nil {
		f.store(key, lens, r)
	}
	f.casts = append(f.casts, castsOf(key, lens, r, err)...)
	return r, err
}

// columns computes one column per shown chart through the same two calls the
// alone frames use, each chart in the house system and zodiac its columnID
// names (the rest of lens is the screen's). A chart whose lookup fails leaves a blank column; the
// failed lookup's spawns are returned so the Log still lists them.
func (f *resultFetcher) columns(ctx context.Context, probers *lookup.Probers, shown columnsFor, place location.PlaceInfo, lens lookup.Lens) ([]bodyColumn, []string) {
	out := make([]bodyColumn, 0, len(shown))
	var spawned []string
	for _, id := range shown {
		own := lens
		own.Houses, own.Zodiac = id.houses, id.zodiac
		var r lookup.Result
		var err error
		if id.key.floating {
			r, err = f.transit(ctx, probers, id.key.when, place, own)
		} else {
			r, err = f.natal(ctx, probers, id.key.anchor(), own)
		}
		col := bodyColumn{key: id.key, label: id.label}
		spawned = append(spawned, r.Commands...)
		if err != nil {
			col.err = err
		} else {
			col.positions = r.Positions
			col.cusps = r.Cusps
		}
		out = append(out, col)
	}
	return out, spawned
}

// commitColumns stores a frame's columns and folds its lookups into the
// result cache, pruned to the charts of the frame just committed. A chart
// whose lookup failed gets one Log error line naming it (C8). A frame with no
// columns (alone, or one that failed early) clears them.
func (m Model) commitColumns(msg FirstFrameReadyMsg) Model {
	m.chart.columns = msg.Columns
	m.chart.columnsFor = msg.ColumnsFor
	m.chart.results = m.chart.results.merged(msg.fresh)
	if len(msg.ColumnsFor) > 0 {
		m.chart.results = m.chart.results.pruned(msg.ColumnsFor)
	}
	for _, c := range msg.Columns {
		if c.err != nil {
			m = m.noteEphemeris(c.err).logError("Couldn't cast %s. %s", c.label, m.failureReason(c.err, m.consolePointer(len(msg.Commands) > 0))).logWarnings(c.err)
		}
	}
	return m
}
