package astrolog

// the specification C2: a host astrolog.as cannot move the clock.

import (
	"context"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// hostileClockLines: each copy's lines differ from the stock `_Yt`, so its
// control can differ. The first uses the bare toggles a host can pre-flip.
var hostileClockLines = map[string][]string{
	"toggles": {"-Yt", "=Yd"},
	"sets":    {"=Yt", "_Yd"},
}

func TestHostAstrologAsCannotMoveTheClock(t *testing.T) {
	needAstrolog(t)
	local := recipe.LocalMomentAtPlace{JD: 2461319.4375, Lon: 4.9, Lat: 52.37, TZ: "Europe/Amsterdam"}
	spec := fixtureSpec(t, local, recipe.Placidus, recipe.Tropical)
	natal := &TimelineNatal{When: time.Date(1990, 5, 1, 12, 0, 0, 0, time.UTC), Lon: 4.9, Lat: 52.37}
	treq := amsterdamRequest(t, 2026, time.October)
	treq.Natal = natal

	observe := func(t *testing.T) (header string, rows []string) {
		t.Helper()
		report, _, err := ComputeRawReport(context.Background(), spec, recipe.ChartRawReport())
		if err != nil {
			t.Fatal(err)
		}
		header = headerOf(t, report)
		got, _, err := ComputeTimeline(context.Background(), treq, []TimelineSource{TimelineSky, TimelineTransit}, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range got {
			rows = append(rows, r.Line)
		}
		if len(rows) == 0 {
			t.Fatal("no timeline rows")
		}
		return header, rows
	}

	for name, lines := range hostileClockLines {
		// The control: with NO pins at all, the hostile copy moves the header.
		t.Run(name+"/control", func(t *testing.T) {
			raw := func() string {
				out, _, err := spawn(context.Background(), testCall([]string{"-qj", "2461319.437500", "-zl", "4:54E", "52:22N", "-v0"}))
				if err != nil {
					t.Fatal(err)
				}
				return headerOf(t, out)
			}
			clean, hostile := func() (string, string) {
				cd, hd := dataDirWith(t), dataDirWith(t, lines...)
				useDataDir(t, cd)
				a := raw()
				useDataDir(t, hd)
				return a, raw()
			}()
			if clean == hostile {
				t.Fatalf("the hostile copy %v did not move an unpinned header (%q): the fixture is not the file Astrolog loaded", lines, clean)
			}
		})
		for _, c := range allClocks {
			t.Run(name+"/"+clockName(c), func(t *testing.T) {
				withClock(t, c)
				useDataDir(t, dataDirWith(t))
				cleanHeader, cleanRows := observe(t)
				useDataDir(t, dataDirWith(t, lines...))
				hostileHeader, hostileRows := observe(t)
				if cleanHeader != hostileHeader {
					t.Errorf("header moved: %q -> %q", cleanHeader, hostileHeader)
				}
				if len(cleanRows) != len(hostileRows) {
					t.Fatalf("rows %d -> %d", len(cleanRows), len(hostileRows))
				}
				for i := range cleanRows {
					if cleanRows[i] != hostileRows[i] {
						t.Errorf("row %d moved: %q -> %q", i, cleanRows[i], hostileRows[i])
					}
				}
			})
		}
	}
}
