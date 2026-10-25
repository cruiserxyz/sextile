package astrolog

import (
	"bytes"
	"context"
	"os/exec"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// TestRenderWheelImageAgainstLiveAstrolog exercises the image-run path end to
// end: compile the WheelImage projection, run astrolog with -XV plus an
// appended -Xo, and read back the SVG. Skipped when astrolog is
// absent so the suite stays hermetic.
func TestRenderWheelImageAgainstLiveAstrolog(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary image test")
	}
	// Hellenistic-equivalent spec at the J2000 epoch, London.
	data, err := RenderWheelImage(context.Background(), hellenisticSpecAt(t, 2451545.0, -0.1278, 51.5074), recipe.DefaultWheelRender())
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

// TestRenderImage_RejectsNonImage is hermetic: RenderImage must refuse a
// text invocation before ever spawning astrolog (the I5 artifact contract).
func TestRenderImage_RejectsNonImage(t *testing.T) {
	_, err := RenderImage(context.Background(), Invocation{Artifact: TextStdout, Argv: []string{"-qj", "1", "-v0"}})
	if err == nil {
		t.Error("RenderImage must reject a non-image (TextStdout) invocation")
	}
}
