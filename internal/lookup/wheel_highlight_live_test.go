package lookup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

var londonPlace = location.PlaceInfo{Name: "London", Country: "GB", Lat: 51.5074, Lon: -0.1278, TZ: "Europe/London"}

// The composite path end to end: an active highlight renders three layers
// through real astrolog and returns one composed SVG (M13's substance at the
// layer that owns it).
func TestRenderWheelImageHighlightComposite(t *testing.T) {
	t.Setenv("SEXTILE_PREVIEW_MODE", "http")
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary image test")
	}
	// The composite is parked behind an opt-in; this test covers the
	// parked path, so it opts in explicitly.
	t.Setenv(wheelHighlightEnvVar, "1")
	when := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
	plain, err := RenderWheelImage(context.Background(), when, londonPlace, Hellenistic, WheelHighlight{})
	if err != nil {
		t.Fatalf("plain render: %v", err)
	}
	composite, err := RenderWheelImage(context.Background(), when, londonPlace, Hellenistic, WheelHighlight{
		Selected: []aspect.Body{aspect.Venus, aspect.Mars},
		Focused:  aspect.Moon,
	})
	if err != nil {
		t.Fatalf("composite render: %v", err)
	}
	if !bytes.HasPrefix(composite, []byte("<?xml ")) || !bytes.Contains(composite, []byte("<svg ")) {
		t.Fatalf("composite is not an SVG")
	}
	if !bytes.Contains(composite, []byte(`opacity="0.35"`)) {
		t.Error("composite does not contain the dimmed vector base")
	}
	if bytes.Equal(plain, composite) {
		t.Error("composite must differ from the plain wheel (dimmed base + highlight)")
	}
}

// The parking contract: with the opt-in unset, a request carrying a
// full highlight renders byte-identical to a request carrying none. The
// composite is unreachable by default no matter what the operator selects,
// and the shipped image is the plain wheel exactly as before an earlier change.
func TestRenderWheelImageHighlightParkedByDefault(t *testing.T) {
	t.Setenv("SEXTILE_PREVIEW_MODE", "http")
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary image test")
	}
	// Force the unset state so an operator .env that opts in cannot mask the
	// default this test exists to pin.
	t.Setenv(wheelHighlightEnvVar, "")
	when := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
	plain, err := RenderWheelImage(context.Background(), when, londonPlace, Hellenistic, WheelHighlight{})
	if err != nil {
		t.Fatalf("plain render: %v", err)
	}
	highlighted, err := RenderWheelImage(context.Background(), when, londonPlace, Hellenistic, WheelHighlight{
		Selected: []aspect.Body{aspect.Venus, aspect.Mars},
		Focused:  aspect.Moon,
	})
	if err != nil {
		t.Fatalf("highlighted render: %v", err)
	}
	if !bytes.Equal(plain, highlighted) {
		t.Errorf("parked highlight changed the image: plain %d bytes, highlighted %d bytes; want byte-identical", len(plain), len(highlighted))
	}
}

// TestRenderTasteFrames is the earlier change taste-lane harness, not a pass/fail
// check: set SEXTILE_TASTE_SHEET_DIR to render the V-frames through the real
// production path into that directory for operator judgement. Skipped
// otherwise.
func TestRenderTasteFrames(t *testing.T) {
	dir := os.Getenv("SEXTILE_TASTE_SHEET_DIR")
	if dir == "" {
		t.Skip("set SEXTILE_TASTE_SHEET_DIR to render taste frames")
	}
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	t.Setenv(wheelHighlightEnvVar, "1") // taste frames render the parked composite
	when := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
	sel := func(bs ...aspect.Body) []aspect.Body { return bs }
	frames := []struct {
		name string
		hl   WheelHighlight
	}{
		{"v2-0-none", WheelHighlight{}},
		{"v2-1-focus-only", WheelHighlight{Focused: aspect.Moon}},
		{"v2-2sel", WheelHighlight{Selected: sel(aspect.Venus, aspect.Mars)}},
		{"v2-3sel", WheelHighlight{Selected: sel(aspect.Venus, aspect.Mars, aspect.Jupiter)}},
		{"v2-4sel-focus", WheelHighlight{Selected: sel(aspect.Venus, aspect.Mars, aspect.Jupiter, aspect.Saturn), Focused: aspect.Moon}},
		{"v2-6sel", WheelHighlight{Selected: sel(aspect.Sun, aspect.Moon, aspect.Venus, aspect.Mars, aspect.Jupiter, aspect.Saturn)}},
		// V3: the J2000 Cap/Aqu cluster with ONLY the Sun selected — the
		// foreground draws the Sun unbumped while the dimmed base keeps the
		// bumped stack, so the faint duplicate is visible for judgement.
		{"v3-sun-split", WheelHighlight{Selected: sel(aspect.Sun)}},
	}
	for _, f := range frames {
		img, err := RenderWheelImage(context.Background(), when, londonPlace, Hellenistic, f.hl)
		if err != nil {
			t.Fatalf("%s: %v", f.name, err)
		}
		path := filepath.Join(dir, f.name+".svg")
		if err := os.WriteFile(path, img, 0o644); err != nil {
			t.Fatal(err)
		}
		fmt.Println("wrote", path)
	}
}
