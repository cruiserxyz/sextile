package tour

import (
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// Session is the workspace a tour starts from: the tour chart open as the only
// chart beside Now, selected nowhere. chartsDir is the charts folder that holds
// ChartFile.
func Session(chartsDir string) store.Session {
	return store.Session{
		Folder: store.FolderID(chartsDir),
		Slots: []store.SessionSlot{{
			Usable: true,
			Chart: store.SessionChart{
				ID:     "sextile-0-1-0",
				Name:   "Sextile 0.1.0",
				When:   time.Date(2026, 10, 25, 9, 21, 54, 0, time.UTC),
				Place:  location.PlaceInfo{Name: "Amsterdam", Lat: 52.0 + 22.0/60, Lon: 4.9},
				Source: store.ChartSource{Kind: store.ChartKindFile, File: ChartFile},
			},
		}},
	}
}
