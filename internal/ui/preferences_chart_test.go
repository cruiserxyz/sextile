package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// All chart values in this file are constructed synthetic test inputs.

// A2 (the specification item A): a legacy block in the operator's own
// preferences file anchors nothing and says nothing. Startup with the block is
// startup without it — the same floating-only workspace, the same Log, line for
// line — because the operator deletes it by hand and Sextile neither reads it
// nor rewrites the file.
func TestPreferencesChartBlockIsIgnored(t *testing.T) {
	const locationBlock = `location:
  name: Berlin
  country: DE
  lat: 52.5244
  lon: 13.4105
  tz: Europe/Berlin
`
	// The legacy anchor exactly as an older build read it: a moment and a
	// fully resolvable place.
	const legacyBlock = `chart:
  when: "2001-02-03T04:05:00Z"
  name: Berlin
  country: DE
  lat: 52.5244
  lon: 13.4105
  tz: Europe/Berlin
`
	start := func(t *testing.T, prefs string) Model {
		t.Helper()
		sessionSandbox(t)
		path := store.PreferencesPath()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(prefs), 0o644); err != nil {
			t.Fatal(err)
		}
		return startedModel(t, realPerimeter(t))
	}

	with := start(t, locationBlock+legacyBlock)
	without := start(t, locationBlock)

	for name, m := range map[string]Model{"with the block": with, "without it": without} {
		if m.slots == nil {
			t.Fatalf("%s: no workspace was built", name)
		}
		if a := firstAnchor(m.slots); a != nil {
			t.Errorf("%s: anchored slot = %v / %+v, want a floating-only workspace", name, a.Time, a.Location)
		}
	}
	if !reflect.DeepEqual(with.logLines, without.logLines) {
		t.Errorf("the block changed the Log:\nwith:    %v\nwithout: %v", with.logLines, without.logLines)
	}
}
