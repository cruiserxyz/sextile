package ui

import (
	"context"
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
// say what it could not read instead of showing a quietly short list. AtlasErr
// is the one failure that drops every chart at once, kept apart because it also
// goes on the error lane. Gen is the picker generation the read was dispatched
// under; applyChartsLoaded drops any message whose Gen != m.pickerGen so a late
// response from a superseded open cannot clobber a newer one.
type ChartsLoadedMsg struct {
	Charts   []store.NamedChart
	Drops    []store.ChartDrop
	AtlasErr error
	Err      error
	Gen      int
	// Seen is the listing's fingerprint of each chart file (lookup.Library),
	// handed to a retitle so a file changed since listing is refused.
	Seen map[string]string
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
	// Label is the name and place the chart's reports print, and Place the
	// casting place the chart list would show, both from the same read as When
	// (the specification C0). HasPlace is false when the read
	// could not place the chart; the listing row's place then stands.
	Label    store.ReportLabel
	Place    location.PlaceInfo
	HasPlace bool
	// Unplaced is set by a successful read that could not place the chart (its
	// coordinates are beyond the atlas): the listing row's name and place are
	// then NOT the file's, and the pick is refused rather than mixing them with
	// the read's moment.
	Unplaced bool
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
		return ChartResolvedMsg{Chart: c, When: p.When, Err: err, Gen: gen, Houses: p.Houses, HasHouses: p.HasHouses, Label: p.Label, Place: p.Place, HasPlace: p.HasPlace, Unplaced: err == nil && !p.HasPlace}
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
		lib, err := lookup.LoadChartLibrary()
		return ChartsLoadedMsg{
			Charts:   lib.Charts,
			Drops:    lib.Drops,
			AtlasErr: lib.AtlasErr,
			Err:      err,
			Gen:      gen,
			Seen:     lib.Seen,
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
// Frame is the "<house name>, <zodiac name>" that was written, kept only when
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
// (houses and zodiac, in the words the store writes), to preferences.yaml
// through the store. It is the only door to the stored city, the stored house
// display and the stored frame: a start, a pick in the atlas, a choice in the
// popup and a quit write nothing there.
func saveDefaultCityCmd(place location.PlaceInfo, form panel.HouseForm, houses recipe.HouseSystem, zodiac recipe.Zodiac, open []store.FrameRecord) tea.Cmd {
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
		msg.Frame = lookup.HouseName(houses) + ", " + zodiac.Name()
		msg.FrameErr = store.SetFrame(houses.StoredName(), zodiac.RecordName())
		msg.FrameSaved = msg.FrameErr == nil
		return msg
	}
}

// saveDefaultCity is `<leader> c d`: it keeps the current city, the house
// display and the floating chart's house system and zodiac as the defaults.
// With no city chosen yet there is nothing to keep, and nothing is written. A
// second window writes nothing either: the session that has to keep the open
// charts' frames (applyDefaultCitySaved) is the first window's.
func (m Model) saveDefaultCity() (tea.Model, tea.Cmd) {
	switch m.blocked(keys.BindingSaveCity) {
	case "":
	case whyNotFirstWindow:
		return m.logErrorLine("Save defaults is for the first window, which keeps the session. This window keeps nothing"), nil
	default:
		return m.logErrorLine("default city not saved: no city chosen yet"), nil
	}
	var floating *store.Slot
	if m.slots != nil {
		f := m.slots.Floating()
		floating = &f
	}
	return m, saveDefaultCityCmd(m.place, m.houseDisplay, m.ownHouses(floating).hs, m.ownZodiac(floating), m.openFramesNow())
}

// applyDefaultCitySaved logs the outcome of a save, and when everything was
// kept also shows a footer notice so it is seen when the Log is not on screen.
// A failure is one error line naming what was not kept; the city is kept, and
// said so, even when the house display or the frame was not.
func applyDefaultCitySaved(m Model, msg DefaultCitySavedMsg) (Model, tea.Cmd) {
	failed := ""
	switch {
	case msg.Err != nil:
		cause, _, _ := strings.Cut(msg.Err.Error(), "\n")
		failed = "default city not saved: " + cause
	case msg.HouseErr != nil:
		cause, _, _ := strings.Cut(msg.HouseErr.Error(), "\n")
		failed = "default house display not saved: " + cause
	case msg.FrameErr != nil:
		cause, _, _ := strings.Cut(msg.FrameErr.Error(), "\n")
		failed = "default frame not saved: " + cause
	}
	if msg.Err == nil {
		m.placeGuessed = false
		if msg.HouseSaved {
			m = m.logNote("default house display saved", "%s", houseFormRow(msg.House).title)
		}
		if msg.FrameSaved {
			m = m.logNote("default frame saved", "%s", msg.Frame)
		}
		m = m.logNote("default city saved", "%s", msg.Place.Display())
	}
	if msg.FrameSaved && failed == "" {
		// The default is for the floating chart on a fresh start and for charts
		// opened later: every chart open at the keypress keeps the frame it showed. The
		// default stays saved when this write fails. For an ordinary failure a
		// quit retries the frames, never for errForeignSession, which nothing retries.
		var added bool
		if m, added = m.keepFrames(msg.Open); added {
			if err := m.writeSessionNow(); err != nil {
				var outside bool
				if m, outside = m.sessionChangedOutside(err); !outside {
					cause, _, _ := strings.Cut(err.Error(), "\n")
					failed = "open charts' frames not kept: " + cause
				}
			}
		}
	}
	if failed != "" {
		return m.logErrorLine(failed), nil
	}
	return m.showNotice(noticeDefaultsSaved)
}
