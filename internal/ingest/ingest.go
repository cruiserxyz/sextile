// Package ingest is the only perimeter-inward intake point.
// Receives natal data and user interaction; never interprets.
// The contract with the IR package lands when first real use surfaces.
package ingest

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// StartupPerimeterReadMsg carries the cold-start perimeter read as a
// resolver-ready set of candidates plus the IANA preseed. The UI defers
// resolution until the atlas index is also loaded, then walks the chain
// once and dispatches.
//
// LocationCandidate is the location: block (or zero if no block).
// IANAPreseed is the system-derived city name used as a fallback candidate
// when nothing else resolves (and as the modal pre-fill on cold-start).
//
// Delivered exactly once per session.
type StartupPerimeterReadMsg struct {
	LocationCandidate location.Candidate
	IANAPreseed       string

	// Session is the last quit's workspace state, HasSession is whether there
	// was one to read, and SessionErr is why a file that exists must not be
	// acted on. Whether the session anchors the workspace is the UI's
	// decision, and this package does not make it (self-check 7).
	Session    store.Session
	HasSession bool
	SessionErr error

	// SessionKeep is what the same read did with a file it refused: the copy
	// it was moved to, or why it was left alone. Zero for an accepted file.
	SessionKeep store.SessionKeep

	// FolderID is the current charts folder's identity (store.FolderID). The
	// UI restores a session only when the session names this folder.
	FolderID string

	// Keys is the keys: block exactly as the store read it — values with
	// presence markers and the entries it refused. ingest does not
	// interpret it: the UI resolves defaults, decides refusals, and writes
	// any Log line.
	Keys store.KeysConfig

	// Display is the display: block exactly as the store read it. Like Keys,
	// the UI decides what a value means and whether it is usable.
	Display store.DisplayConfig

	// WheelFormat is the top-level wheel_format: key exactly as the store read
	// it: which image the wheel server hosts. The UI decides whether the word is
	// one of the two formats.
	WheelFormat store.WheelFormatConfig

	// HouseDisplay is the top-level house_display: key exactly as the store
	// read it: how the Bodies house column is drawn. The UI decides whether
	// the word is one of the forms.
	HouseDisplay store.HouseDisplayConfig

	// Frame is the frame: block exactly as the store read it: the default
	// house system a chart falls back to (the specification C1).
	Frame store.FrameConfig

	// Clock is the clock: block as the store resolved it (hours and date
	// order, defaults applied, refused entries listed). It is read once, here,
	// and held for the session (the specification C12).
	Clock store.ClockConfig

	// FileReads is what a restored chart file said about itself, by file base
	// name, filled by the UI's own startup read (ingest cannot spawn Astrolog).
	// Absent for a hand-built message and for a file that could not be read.
	FileReads map[string]ChartFileRead
	// FileReadFailures names the restored chart files whose startup read
	// failed, with a one-line reason. Such a chart's house system is unresolved
	// until a later read succeeds; it is never guessed.
	FileReadFailures map[string]string
}

// ChartFileRead is one chart file's own record as Astrolog read it: its birth
// moment and the house system it names, read before the first frame so the
// first cast is already in the right system.
type ChartFileRead struct {
	When      time.Time
	Houses    recipe.HouseSystem
	HasHouses bool
}

// HasLocationCandidate reports whether the location: block carried any
// usable signal (name, tz, or coords).
func (m StartupPerimeterReadMsg) HasLocationCandidate() bool {
	return !m.LocationCandidate.Place.Empty()
}

// ReadStartupPerimeter returns the tea.Cmd that performs the cold-start
// perimeter read (preferences.yaml + host-IANA cascade) and emits a
// StartupPerimeterReadMsg. Wired from ui.Model.Init.
//
// Returns *raw* candidates — Place may be partial (city only, missing
// coords, missing tz). The UI's deferred-resolution stage runs
// location.Resolve once the atlas index also lands.
func ReadStartupPerimeter() tea.Cmd {
	return readStartupPerimeter(store.LoadSessionKeepingRefused)
}

// ReadStartupPerimeterReadOnly is ReadStartupPerimeter for a second window:
// the general session belongs to the window holding the instance lock, so a
// second window does not read it at all and starts empty. Its file, refused
// or not, is left exactly as it is.
func ReadStartupPerimeterReadOnly() tea.Cmd {
	return readStartupPerimeter(func() (store.Session, bool, store.SessionKeep, error) {
		return store.Session{}, false, store.SessionKeep{}, nil
	})
}

// ReadStartupPerimeterNamed is ReadStartupPerimeter for a launch that runs the
// named session kept in file (--session): that session is read instead of the
// general one.
func ReadStartupPerimeterNamed(file string) tea.Cmd {
	return readStartupPerimeter(func() (store.Session, bool, store.SessionKeep, error) {
		s, _, err := store.LoadNamedSession(file)
		return s, err == nil, store.SessionKeep{}, err
	})
}

func readStartupPerimeter(loadSession func() (store.Session, bool, store.SessionKeep, error)) tea.Cmd {
	return func() tea.Msg {
		preseed := location.PreseedFromIANA(location.HostIANA())
		msg := StartupPerimeterReadMsg{IANAPreseed: preseed, Keys: store.Keys(), Display: store.Display(), WheelFormat: store.WheelFormat(), HouseDisplay: store.HouseDisplay(), Frame: store.Frame(), Clock: store.Clock()}
		if cand, ok := store.LocationCandidate(); ok {
			msg.LocationCandidate = cand
		}
		msg.Session, msg.HasSession, msg.SessionKeep, msg.SessionErr = loadSession()
		msg.FolderID = store.FolderID(store.ChartsDir())
		return msg
	}
}
