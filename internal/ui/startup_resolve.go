package ui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// AtlasPickedMsg is emitted by the atlas modal on Enter against a real
// match. It carries the picked PlaceInfo all downstream handlers need.
type AtlasPickedMsg struct {
	Place location.PlaceInfo
}

// ChartsLoadedMsg carries the saved-chart library read triggered when the
// chart picker opens (Ctrl+O). Charts is nil when Err is set. Drops names every
// file in the library directory that did not become a chart, so the picker can
// say what it could not read instead of showing a quietly short list. Gen is the picker generation the read was dispatched
// under; applyChartsLoaded drops any message whose Gen != m.pickerGen so a late
// response from a superseded open cannot clobber a newer one.
type ChartsLoadedMsg struct {
	Charts []store.NamedChart
	Drops  []store.ChartDrop
	// Refused is the chart guard's reason for each listed file it refuses,
	// by file name (lookup.Library.Refused).
	Refused map[string]error
	Err     error
	Gen     int
	// Seen is the listing's fingerprint of each chart file (lookup.Library),
	// handed to a retitle so a file changed since listing is refused.
	Seen map[string]string
	// Dir is the charts folder the listing read: a rename of one of its rows
	// asks about, and writes into, this folder.
	Dir string
}

// ChartPickedMsg is emitted by the chart picker on Enter against a real match.
// It carries the chosen library row, which becomes the natal anchor once its
// birth moment has been resolved.
type ChartPickedMsg struct {
	Chart store.NamedChart
}

// ChartResolvedMsg carries the authoritative birth moment of a picked chart,
// read from the file its row named. A listing row holds no moment, so this is
// what turns a pick into an anchor.
//
// Gen is the pick generation the resolve was dispatched under. applyChartResolved
// drops any message whose Gen != m.pickGen, so a resolve superseded by a later
// pick — or cancelled by Esc — changes no state and writes no error line.
type ChartResolvedMsg struct {
	Chart store.NamedChart
	When  time.Time
	Err   error
	Gen   int
	// Houses is the system the chart file names, as Astrolog read it in the
	// same read that gave When; HasHouses is false when it names none
	// (the specification C2).
	Houses    recipe.HouseSystem
	HasHouses bool
	// Nodes is the form of the Nodes and Lilith the file states, from the same
	// read; HasNodes is false when it states none.
	Nodes    recipe.NodeForm
	HasNodes bool
	// Label is the name and place the chart's reports print, and Place the
	// casting place, both from the same read as When (the specification C0).
	Label store.ReportLabel
	Place location.PlaceInfo
	// Record is the file's text from the same read: what Astrolog casts.
	Record string
	// Notes are the file's lines Astrolog does not get, for the Log.
	Notes []lookup.FileNote
	// Clock is the chart's moment as Astrolog read it, for its row.
	Clock store.ChartClock
}

// resolveChartCmd reads the picked chart's authoritative moment off the Update
// loop: one astrolog spawn. The command it ran is deliberately not carried: a
// library resolution is not part of the chart the operator is looking at, and
// the console's Enter copies a command straight to a system clipboard, so the
// argv lane stays reserved for spawns that produced the frame.
func resolveChartCmd(c store.NamedChart, gen int) tea.Cmd {
	return resolveChartCmdIn("", c, gen)
}

// resolveChartCmdIn is resolveChartCmd reading from the directory dir names
// (empty: the resolver's answer now).
func resolveChartCmdIn(dir string, c store.NamedChart, gen int) tea.Cmd {
	return func() tea.Msg {
		p, err := lookup.ResolvePickedChartIn(context.Background(), dir, c)
		return ChartResolvedMsg{Chart: c, When: p.When, Err: err, Gen: gen, Houses: p.Houses, HasHouses: p.HasHouses, Nodes: p.Nodes, HasNodes: p.NodesStated, Label: p.Label, Place: p.Place, Record: p.Record, Notes: p.Notes, Clock: p.Clock}
	}
}

// loadAtlasCmd reads astrolog's atlas at the resolved path and emits
// AtlasLoadedMsg. Always asynchronous (filesystem read in a goroutine).
func loadAtlasCmd() tea.Cmd {
	return func() tea.Msg {
		path, err := location.AtlasPath()
		if err != nil {
			return AtlasLoadedMsg{Err: err}
		}
		entries, err := location.LoadAtlas(path)
		if err != nil {
			return AtlasLoadedMsg{Err: err}
		}
		return AtlasLoadedMsg{Index: location.NewIndex(entries)}
	}
}

// loadChartsCmd reads the chart library (lookup.LoadChartLibrary) off the Update
// loop and emits ChartsLoadedMsg. Unlike the atlas (loaded once at Init), the
// library is loaded lazily each time the picker opens, so a chart saved out of
// band shows up on the next open.
func loadChartsCmd(gen int) tea.Cmd {
	return func() tea.Msg {
		dir := store.ChartsDir()
		lib, err := lookup.LoadChartLibraryIn(dir)
		return ChartsLoadedMsg{
			Dir:     dir,
			Charts:  lib.Charts,
			Drops:   lib.Drops,
			Err:     err,
			Gen:     gen,
			Seen:    lib.Seen,
			Refused: lib.Refused,
		}
	}
}

// DefaultCitySavedMsg is the result of keeping the defaults (`<leader> c d`):
// the current city, the house display and the floating chart's frame. Place is
// the city that was written, so a late result names what was saved and not what
// the screen shows by then; House is the form that was written, kept only when
// HouseSaved says the write landed. The house display is written after the
// city, and only once the city was, and the frame after the house display, so
// Err is the city's, HouseErr the house display's and FrameErr the frame's.
// Frame is the "<house name>, <zodiac name>, <form of the Nodes and Lilith>" that was written, kept only when
// FrameSaved says the write landed.
type DefaultCitySavedMsg struct {
	Place      location.PlaceInfo
	Err        error
	House      panel.HouseForm
	HouseSaved bool
	HouseErr   error
	Frame      string
	FrameSaved bool
	FrameErr   error
	// Open is the frame every chart open at the keypress was shown in, for
	// the ones that had no record (openFramesNow). A save that lands keeps them.
	Open []store.FrameRecord
}

// saveDefaultCityCmd writes place, then the house display form, then the frame
// (houses, zodiac and the form of the Nodes and Lilith, in the words the store writes), to preferences.yaml
// through the store. It is the only door to the stored city, the stored house
// display and the stored frame: a start, a pick in the atlas, a choice in the
// popup and a quit write nothing there.
func saveDefaultCityCmd(place location.PlaceInfo, form panel.HouseForm, houses recipe.HouseSystem, zodiac recipe.Zodiac, nodes recipe.NodeForm, open []store.FrameRecord) tea.Cmd {
	return func() tea.Msg {
		msg := DefaultCitySavedMsg{Place: place, Err: store.SetLocation(place), Open: open}
		if msg.Err != nil {
			return msg
		}
		msg.House = form
		msg.HouseErr = store.SetHouseDisplay(houseFormRow(form).value)
		msg.HouseSaved = msg.HouseErr == nil
		if msg.HouseErr != nil {
			return msg
		}
		msg.Frame = lookup.HouseName(houses) + ", " + zodiac.Name() + ", " + nodeFormWords(nodes)
		msg.FrameErr = store.SetFrameNodes(houses.StoredName(), zodiac.RecordName(), nodes)
		msg.FrameSaved = msg.FrameErr == nil
		return msg
	}
}

// saveDefaultsSecondWindow is the Log line for Save defaults in a second window.
const saveDefaultsSecondWindow = "defaults were not kept, only the first Sextile window keeps them"

// saveDefaultCity is `<leader> c d`: it keeps the current city, the house
// display and the floating chart's house system and zodiac as the defaults.
// With no city chosen yet there is nothing to keep, and nothing is written. A
// second window writes nothing either: the session that has to keep the open
// charts' frames (applyDefaultCitySaved) is the first window's.
func (m Model) saveDefaultCity() (tea.Model, tea.Cmd) {
	switch m.blocked(keys.BindingSaveCity) {
	case "":
	case whyNotFirstWindow:
		return m.logErrorLine("settings", saveDefaultsSecondWindow), nil
	default:
		return m.logErrorLine("settings", "the city was not kept as the default, no city is chosen yet"), nil
	}
	var floating *store.Slot
	if m.slots != nil {
		f := m.slots.Floating()
		floating = &f
	}
	nodes, _ := m.ownNodes(floating)
	return m, saveDefaultCityCmd(m.place, m.houseDisplay, m.ownHouses(floating).hs, m.ownZodiac(floating), nodes, m.openFramesNow())
}

// applyDefaultCitySaved logs the outcome of a save, and when everything was
// kept also shows a footer notice so it is seen when the Log is not on screen.
// A failure is one error line naming what was not kept; the city is kept, and
// said so, even when the house display or the frame was not.
func applyDefaultCitySaved(m Model, msg DefaultCitySavedMsg) (Model, tea.Cmd) {
	failed := ""
	switch {
	case msg.Err != nil:
		m = m.logTrace("settings", "%s", saveCause(msg.Err))
		failed = "the city was not kept as the default"
	case msg.HouseErr != nil:
		m = m.logTrace("settings", "%s", saveCause(msg.HouseErr))
		failed = "the house display was not kept as the default"
	case msg.FrameErr != nil:
		m = m.logTrace("settings", "%s", saveCause(msg.FrameErr))
		failed = "the houses, zodiac and Nodes were not kept as the default"
	}
	if msg.Err == nil {
		m.placeFromZone = false
		if msg.HouseSaved {
			m = m.logNote("defaults", "houses shown as %s", houseFormRow(msg.House).title)
		}
		if msg.FrameSaved {
			m = m.logNote("defaults", "%s", msg.Frame)
		}
		m = m.logNote("default city saved", "%s", msg.Place.Display())
	}
	if msg.FrameSaved && failed == "" {
		m, failed = m.keepOpenFrames(msg.Open)
	}
	if failed != "" {
		return m.logErrorLine("settings", failed), nil
	}
	return m.showNotice(noticeDefaultsSaved)
}

// keepOpenFrames is what a saved frame default owes the charts open when it was
// asked for: each keeps the frame it showed (the default is for the floating
// chart on a fresh start and for charts opened later). The default stays saved
// when this write fails. For an ordinary failure a quit retries the frames. A
// run of another charts folder's session keeps nothing of its charts, which the
// start already said, so it says nothing here. failed is the Log line for a
// write that did not land, "" otherwise.
func (m Model) keepOpenFrames(open []store.FrameRecord) (_ Model, failed string) {
	var added bool
	if m, added = m.keepFrames(open); added {
		if err := m.writeSessionNow(); err != nil {
			var outside bool
			if m, outside = m.sessionChangedOutside(err); !outside && !errors.Is(err, errForeignSession) {
				failed = "the houses and zodiac of the open charts were not saved"
			}
		}
	}
	return m, failed
}

// saveCause is the first line of a failed write's error, without the store
// package's own name in front.
func saveCause(err error) string {
	cause, _, _ := strings.Cut(err.Error(), "\n")
	return strings.TrimPrefix(cause, "store: ")
}
