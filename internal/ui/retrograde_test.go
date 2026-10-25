package ui

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// TestFormatPositionRow_Retrograde asserts a retrograde body (Speed < 0) gets
// the ℞ marker, and a direct body does not.
func TestFormatPositionRow_Retrograde(t *testing.T) {
	retro := aspect.Position{Body: aspect.Mercury, Longitude: 15.3, Speed: -1.1}
	if got := formatPositionRow(retro, false); !strings.Contains(got, "℞") {
		t.Errorf("retrograde row should contain ℞, got %q", got)
	}

	direct := aspect.Position{Body: aspect.Mercury, Longitude: 15.3, Speed: 1.1}
	if got := formatPositionRow(direct, false); strings.Contains(got, "℞") {
		t.Errorf("direct row must not contain ℞, got %q", got)
	}
}
