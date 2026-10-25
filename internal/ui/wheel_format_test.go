package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// wheel_format (the specification W1 part 5, A5): which image the wheel server
// hosts. The hosting itself is pinned in internal/preview and the render
// choice in internal/lookup; these pin that the key reaches both doors, at
// startup and at every Ctrl+X.

// recordWheelFormat stands in for the render's setting and records every value
// the ui hands it.
func recordWheelFormat(t *testing.T) *[]recipe.ImageFormat {
	t.Helper()
	var got []recipe.ImageFormat
	prev := setWheelFormat
	setWheelFormat = func(f recipe.ImageFormat) { got = append(got, f) }
	t.Cleanup(func() { setWheelFormat = prev })
	return &got
}

func lastFormat(t *testing.T, got *[]recipe.ImageFormat) recipe.ImageFormat {
	t.Helper()
	if len(*got) == 0 {
		t.Fatal("the render format was never set")
	}
	return (*got)[len(*got)-1]
}

// png in preferences.yaml reaches the render at startup and at Ctrl+X; svg and
// an absent key are the SVG default; an edit of the file takes effect at the
// next Ctrl+X with no restart.
func TestWheelFormatPreference(t *testing.T) {
	got := recordWheelFormat(t)
	path := writeClockPrefs(t, "wheel_format: png\n")

	m := coldStart(t)
	if f := lastFormat(t, got); f != recipe.ImagePNG {
		t.Errorf("startup with wheel_format: png set the render to %d, want PNG (%d)", f, recipe.ImagePNG)
	}
	if lines := lanesText(m, laneNote); len(lines) != 0 {
		t.Errorf("a valid wheel_format wrote %q", lines)
	}

	// The operator edits the key and presses Ctrl+X: no restart.
	if err := os.WriteFile(path, []byte("version: 1\nwheel_format: svg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m = modelWithSlot(t)
	m, _ = m.dispatchRenderWheel()
	if f := lastFormat(t, got); f != recipe.ImageSVG {
		t.Errorf("Ctrl+X after editing to svg set the render to %d, want SVG", f)
	}
	if err := os.WriteFile(path, []byte("wheel_format: png\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m = modelWithSlot(t)
	m, _ = m.dispatchRenderWheel()
	if f := lastFormat(t, got); f != recipe.ImagePNG {
		t.Errorf("Ctrl+X after editing to png set the render to %d, want PNG", f)
	}

	// Absent is svg.
	if err := os.WriteFile(path, []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m = modelWithSlot(t)
	_, _ = m.dispatchRenderWheel()
	if f := lastFormat(t, got); f != recipe.ImageSVG {
		t.Errorf("Ctrl+X with no wheel_format set the render to %d, want SVG", f)
	}
}

// An unknown word is refused at startup with a note that names both words that
// are allowed, and svg stands; the same word is not refused again at every
// Ctrl+X.
func TestWheelFormatRefusesUnknown(t *testing.T) {
	got := recordWheelFormat(t)
	writeClockPrefs(t, "wheel_format: gif\n")

	m := coldStart(t)
	notes := lanesText(m, laneNote)
	if len(notes) != 1 {
		t.Fatalf("startup with wheel_format: gif wrote %q, want one note", notes)
	}
	for _, want := range []string{"wheel_format", `"gif"`, "svg", "png"} {
		if !strings.Contains(notes[0], want) {
			t.Errorf("the note %q does not name %s", notes[0], want)
		}
	}
	if f := lastFormat(t, got); f != recipe.ImageSVG {
		t.Errorf("a refused wheel_format set the render to %d, want SVG", f)
	}

	m, _ = m.dispatchRenderWheel()
	if again := lanesText(m, laneNote); len(again) != 1 {
		t.Errorf("Ctrl+X refused the same word again: %q", again)
	}

	// A valid word and then a new bad one is refused anew.
	m = m.applyWheelFormat(store.WheelFormatConfig{Value: "png", Present: true})
	m = m.applyWheelFormat(store.WheelFormatConfig{Value: "gif", Present: true})
	if n := len(lanesText(m, laneNote)); n != 2 {
		t.Errorf("notes = %d, want the second gif refused after a valid word", n)
	}
}
