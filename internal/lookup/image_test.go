package lookup

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

// TestRenderWheelImageAgainstLiveAstrolog exercises the full lookup→astrolog
// image path: a moment + place + lens through to a real wheel SVG. Skipped
// when astrolog is absent.
func TestRenderWheelImageAgainstLiveAstrolog(t *testing.T) {
	t.Setenv("SEXTILE_PREVIEW_MODE", "http")
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary image test")
	}
	place := location.PlaceInfo{Name: "London", Country: "GB", Lat: 51.5074, Lon: -0.1278, TZ: "Europe/London"}
	when := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)

	data, err := RenderWheelImage(context.Background(), when, place, Hellenistic, WheelHighlight{})
	if err != nil {
		t.Fatalf("RenderWheelImage: %v", err)
	}
	if len(data) < 1024 {
		t.Fatalf("image suspiciously small: %d bytes", len(data))
	}
	if !bytes.HasPrefix(data, []byte("<?xml ")) || !bytes.Contains(data, []byte("<svg ")) {
		t.Errorf("image is not SVG; starts %q", data[:min(80, len(data))])
	}
}

func TestPreviewImageFormatMatchesBackend(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want recipe.ImageFormat
	}{
		{mode: "", want: recipe.ImagePNG},
		{mode: "xdg", want: recipe.ImagePNG},
		{mode: "terminal-xdg", want: recipe.ImagePNG},
		{mode: "http", want: recipe.ImageSVG},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Setenv("SEXTILE_PREVIEW_MODE", tc.mode)
			if got := previewImageFormat(); got != tc.want {
				t.Fatalf("previewImageFormat() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestRenderWheelImage_RefusesUntimedPlace is hermetic: a place without a
// timezone is rejected before any astrolog spawn.
func TestRenderWheelImage_RefusesUntimedPlace(t *testing.T) {
	_, err := RenderWheelImage(context.Background(), time.Now(), location.PlaceInfo{Name: "Nowhere"}, Hellenistic, WheelHighlight{})
	if err == nil {
		t.Error("RenderWheelImage must refuse a place with no timezone")
	}
}

func TestServeModeRendersSVG(t *testing.T) {
	t.Setenv("SEXTILE_PREVIEW_MODE", "serve")
	if got := previewImageFormat(); got != recipe.ImageSVG {
		t.Fatalf("previewImageFormat() in serve mode = %d, want SVG (%d)", got, recipe.ImageSVG)
	}
}

// wheel_format reaches the render: in the HTTP modes the operator's choice
// decides SVG or PNG; the desktop backends stay PNG whatever it says.
func TestWheelFormatSelectsTheRender(t *testing.T) {
	t.Cleanup(func() { SetWheelFormat(recipe.ImageSVG) })
	for _, tc := range []struct {
		mode   string
		chosen recipe.ImageFormat
		want   recipe.ImageFormat
	}{
		{"serve", recipe.ImagePNG, recipe.ImagePNG},
		{"http", recipe.ImagePNG, recipe.ImagePNG},
		{"serve", recipe.ImageSVG, recipe.ImageSVG},
		{"xdg", recipe.ImageSVG, recipe.ImagePNG},
		{"", recipe.ImageSVG, recipe.ImagePNG},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Setenv("SEXTILE_PREVIEW_MODE", tc.mode)
			SetWheelFormat(tc.chosen)
			if got := previewImageFormat(); got != tc.want {
				t.Fatalf("mode %q with wheel_format %d renders %d, want %d", tc.mode, tc.chosen, got, tc.want)
			}
		})
	}
}

// The same choice against the real Astrolog: PNG bytes, not SVG.
func TestWheelFormatPNGAgainstLiveAstrolog(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary image test")
	}
	t.Setenv("SEXTILE_PREVIEW_MODE", "serve")
	SetWheelFormat(recipe.ImagePNG)
	t.Cleanup(func() { SetWheelFormat(recipe.ImageSVG) })
	place := location.PlaceInfo{Name: "London", Country: "GB", Lat: 51.5074, Lon: -0.1278, TZ: "Europe/London"}
	data, err := RenderWheelImage(context.Background(), time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC), place, Hellenistic, WheelHighlight{})
	if err != nil {
		t.Fatalf("RenderWheelImage: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Errorf("wheel_format png rendered %q, want a PNG", data[:min(16, len(data))])
	}
}
