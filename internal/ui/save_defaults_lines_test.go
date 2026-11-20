package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// A failed save of the defaults says what was not kept in one Log line, and the
// whole cause, uncut, in the large Log.
func TestSaveDefaultsFailureLinesAreNeverCut(t *testing.T) {
	longPath := "/data/op/" + strings.Repeat("a-very-long-folder-name/", 4) + "sextile/preferences.yaml"
	cases := map[string]struct {
		msg   DefaultCitySavedMsg
		want  string
		cause string
	}{
		"a frame block Sextile cannot edit": {
			msg:   DefaultCitySavedMsg{Place: amsterdamNL, FrameErr: errors.New("store: preferences.yaml's frame: block holds 'garbage', which YAML does not read as part of it. The frame was not saved")},
			want:  "settings: the houses, zodiac and Nodes were not kept as the default",
			cause: "preferences.yaml's frame: block holds 'garbage', which YAML does not read as part of it. The frame was not saved",
		},
		"a write failure with a long path": {
			msg:   DefaultCitySavedMsg{Place: amsterdamNL, House: panel.HouseNumber, HouseErr: errors.New("store: read: open " + longPath + ": permission denied")},
			want:  "settings: the house display was not kept as the default",
			cause: "read: open " + longPath + ": permission denied",
		},
		"a city the file refuses": {
			msg:   DefaultCitySavedMsg{Place: amsterdamNL, Err: errors.New("store: preferences.yaml was written by a newer Sextile (generation 3, this build reads 1). Nothing was written")},
			want:  "settings: the city was not kept as the default",
			cause: "preferences.yaml was written by a newer Sextile (generation 3, this build reads 1). Nothing was written",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			m, _ := guessedStart(t)
			before := len(m.logLines)
			m, _ = applyDefaultCitySaved(m, c.msg)
			var said, traced []string
			for _, l := range m.logLines[before:] {
				switch l.lane {
				case laneError:
					said = append(said, l.text)
				case laneTrace:
					traced = append(traced, l.text)
				}
			}
			if len(said) != 1 || said[0] != c.want {
				t.Errorf("the Log says %q, want exactly [%q]", said, c.want)
			}
			if len(traced) != 1 || traced[0] != "settings: "+c.cause {
				t.Errorf("the large Log holds %q, want the whole cause %q", traced, c.cause)
			}
		})
	}
}

// The line a second window gives for Save defaults fits the Log.
func TestSaveDefaultsSecondWindowLineFits(t *testing.T) {
	if w := ansi.StringWidth(saveDefaultsSecondWindow); w > logLineCells {
		t.Errorf("%q is %d cells, the Log holds %d", saveDefaultsSecondWindow, w, logLineCells)
	}
}
