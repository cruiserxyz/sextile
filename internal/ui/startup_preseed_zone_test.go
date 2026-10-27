package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// TestStartupPreseedCarriesTheHostZone binds the half of the Amsterdam fix
// that lives in this package. The resolver refuses an ambiguous city name
// rather than taking the first atlas row (location.Resolve, 2026-09-20), so
// the startup candidate has to carry the ZONE the name was cut from, or every
// repeated city name resolves to nothing and the operator meets a picker they
// did not ask for.
//
// The fixture is built FROM the host's own zone rather than hardcoding
// Europe/Amsterdam, so the test means the same thing on any machine: two atlas
// rows share the preseed name, and only one sits in the host's zone.
func TestStartupPreseedCarriesTheHostZone(t *testing.T) {
	host := location.HostIANA()
	if host == "" {
		t.Skip("host exposes no IANA zone, so there is no preseed to carry")
	}
	name := location.PreseedFromIANA(host)
	if name == "" {
		t.Skipf("host zone %q yields no preseed name", host)
	}

	const decoy, wanted = "ZZ", "YY"
	atlas := location.NewIndex([]location.PlaceInfo{
		// Ordered decoy-first on purpose: file order is exactly what the old
		// matches[0] rule broke on, so a regression picks this row.
		{Name: name, Country: decoy, Lat: 1, Lon: 1, TZ: "Antarctica/Troll"},
		{Name: name, Country: wanted, Lat: 2, Lon: 2, TZ: host},
	})

	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(time.Now().UTC()))
	m.atlas.index = atlas
	stashed := ingest.StartupPerimeterReadMsg{IANAPreseed: name}
	m.pendingPerimeter = &stashed

	got, _ := maybeResolveStartup(m)

	if got.startup == startupAtlasOpen {
		t.Fatalf("startup fell through to the atlas picker: the preseed candidate "+
			"carried no zone, so %q stayed ambiguous between %s and %s", name, decoy, wanted)
	}
	if got.place.Country != wanted {
		t.Errorf("resolved to %q (%s); want the row in the host zone %s (%s)",
			got.place.Name, got.place.Country, host, wanted)
	}
	if got.place.TZ != host {
		t.Errorf("resolved TZ = %q, want the host zone %q", got.place.TZ, host)
	}
}

// TestStartLogsNoResolverCandidates: the resolver's per-candidate notes
// ("candidate \"iana-preseed\": ...") are not written to the Log at startup;
// the city line is the one thing a guessed start says about its city. A
// second Amsterdam, in another zone, makes the resolver narrow by zone, which
// is the case that used to log "resolved to ... (2 atlas entries matched)".
func TestStartLogsNoResolverCandidates(t *testing.T) {
	sessionSandbox(t)
	newYork := location.PlaceInfo{Name: "Amsterdam", Country: "US", Lat: 42.94, Lon: -74.19, TZ: "America/New_York"}
	m, _ := startFromDisk(t, newYork, amsterdamNL)
	for _, text := range logTexts(m) {
		if strings.HasPrefix(text, "location: ") {
			t.Errorf("the start logged a resolver note %q; want none", text)
		}
	}
	if !strings.Contains(strings.Join(logTexts(m), "\n"), "city: Amsterdam, NL, from the time zone") {
		t.Errorf("the city line is gone: %q", logTexts(m))
	}
}

// Acceptance 3: a start whose city came from the host's time zone says so once,
// naming the city and the two keys; a start from a stored city says nothing and
// carries no mark.
func TestGuessedStartLogsOnce(t *testing.T) {
	m, _ := guessedStart(t)
	want := "city: Amsterdam, NL, from the time zone — , c d saves it"
	n := 0
	for _, l := range m.logLines {
		if l.text == want && l.lane == laneNote {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the guess note appears %d times in %q, want once", n, logTexts(m))
	}

	s, _ := storedStart(t)
	for _, text := range logTexts(s) {
		if strings.Contains(text, "from the time zone") {
			t.Errorf("a stored-city start logged %q", text)
		}
	}
}

// Acceptance 4: starting on a guessed city, and running every command the start
// returned (the first compute included), writes nothing: the preferences file
// stays absent, or stays byte-identical when it existed.
func TestGuessedCityIsNeverWritten(t *testing.T) {
	sessionSandbox(t)
	before := profileSnapshot(t)
	m, cmd := startFromDisk(t, amsterdamNL)
	if !m.placeGuessed || cmd == nil {
		t.Fatalf("fixture: guessed=%v cmd=%v", m.placeGuessed, cmd != nil)
	}
	for _, msg := range runAll(cmd) {
		tm, next := m.Update(msg)
		m = tm.(Model)
		runAll(next)
	}
	sameProfile(t, before, profileSnapshot(t), "a guessed start")
	if _, err := os.Stat(store.PreferencesPath()); !os.IsNotExist(err) {
		t.Errorf("preferences.yaml exists after a guessed start: %v", err)
	}

	// With a preferences file already there, it stays byte-identical, mtime too.
	path := store.PreferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("clock:\n  mode: held\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before = profileSnapshot(t)
	m, cmd = startFromDisk(t, amsterdamNL)
	for _, msg := range runAll(cmd) {
		tm, next := m.Update(msg)
		m = tm.(Model)
		runAll(next)
	}
	sameProfile(t, before, profileSnapshot(t), "a guessed start over an existing file")
}
