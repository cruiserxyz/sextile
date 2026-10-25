package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cruiserxyz/sextile/internal/location"
)

func setupXDG(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func TestLocation_AbsentFile(t *testing.T) {
	setupXDG(t)
	if _, ok := Location(); ok {
		t.Fatal("expected ok=false when preferences.yaml absent")
	}
}

func TestSetLocation_RoundTrip(t *testing.T) {
	xdg := setupXDG(t)
	want := location.PlaceInfo{
		Name:    "Berlin",
		Country: "DE",
		Lat:     52.5244,
		Lon:     13.4105,
		TZ:      "Europe/Berlin",
	}
	if err := SetLocation(want); err != nil {
		t.Fatalf("SetLocation: %v", err)
	}
	path := filepath.Join(xdg, "sextile", "preferences.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("preferences.yaml not written at %s: %v", path, err)
	}
	got, ok := Location()
	if !ok {
		t.Fatal("Location returned ok=false after SetLocation")
	}
	if got != want {
		t.Errorf("round-trip mismatch:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestSetLocation_OverwriteAtomic(t *testing.T) {
	setupXDG(t)
	first := location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5, Lon: 13.4, TZ: "Europe/Berlin"}
	second := location.PlaceInfo{Name: "Tokyo", Country: "JP", Lat: 35.7, Lon: 139.7, TZ: "Asia/Tokyo"}
	if err := SetLocation(first); err != nil {
		t.Fatalf("SetLocation first: %v", err)
	}
	if err := SetLocation(second); err != nil {
		t.Fatalf("SetLocation second: %v", err)
	}
	got, ok := Location()
	if !ok || got != second {
		t.Errorf("expected second value, got %+v ok=%v", got, ok)
	}
}

func TestLocation_RejectsRowWithoutTimezone(t *testing.T) {
	xdg := setupXDG(t)
	dir := filepath.Join(xdg, "sextile")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "location:\n  name: Atlantis\n  country: ??\n  lat: 0\n  lon: 0\n  tz: \n"
	if err := os.WriteFile(filepath.Join(dir, "preferences.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Location(); ok {
		t.Error("a row with empty tz must not validate as a stored location")
	}
}
