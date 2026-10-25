package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
)

// Ctrl+L (re-open) must clear filterText so leftover text from a previous
// pick / typing session never pre-fills. The IANA preseed pre-fill is
// reserved for first cold-start without a stored location.
func TestOpenAtlas_ClearsFilterTextOnReopen(t *testing.T) {
	m := NewModel(Config{Accessible: true})
	m.atlas.filterText = "Berlin"
	m.atlas.cursor = 3
	m.startup = startupReady

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	got := tm.(Model)

	if got.startup != startupAtlasOpen {
		t.Fatalf("startup = %q, want %q", got.startup, startupAtlasOpen)
	}
	if got.atlas.filterText != "" {
		t.Errorf("filterText = %q, want empty (sticky text must be cleared on re-open)", got.atlas.filterText)
	}
	if got.atlas.cursor != 0 {
		t.Errorf("cursor = %d, want 0", got.atlas.cursor)
	}
}

// First cold-start without a stored location keeps the IANA preseed pre-fill —
// this is the one and only place filterText is pre-populated with a system
// guess. cp3: resolution defers until both perimeter + atlas msgs are in,
// then on full-chain miss (empty atlas can't resolve preseed) the modal
// opens with the preseed pre-filled.
func TestStartupPerimeter_NoLocation_PreseedFillsFilterText(t *testing.T) {
	m := NewModel(Config{Accessible: true})

	// Inject empty-but-non-nil atlas so resolution actually fires (an
	// atlas-not-loaded state would just wait).
	tm, _ := m.Update(AtlasLoadedMsg{Index: []location.Indexed{}})
	tm, _ = tm.(Model).Update(ingest.StartupPerimeterReadMsg{
		IANAPreseed: "Berlin",
	})
	got := tm.(Model)

	if got.startup != startupAtlasOpen {
		t.Fatalf("startup = %q, want %q", got.startup, startupAtlasOpen)
	}
	if got.atlas.filterText != "Berlin" {
		t.Errorf("filterText = %q, want %q (IANA preseed should pre-fill on full-miss modal open)", got.atlas.filterText, "Berlin")
	}
}

// An empty city chooser says what to type, with an example that the filter
// really ranks first (see internal/location TestFilter_ExampleQueryRanksFirst).
func TestAtlasEmptyFilterSaysWhatToType(t *testing.T) {
	st := liftState("", 0)
	got := strings.TrimSpace(renderResultList(st, Config{}, 54, 5))
	if got != "Type a city, e.g. Amsterdam. Narrow it: Paris FR" {
		t.Errorf("empty city list = %q, want the typing hint", got)
	}
}

// The other list messages are not touched by the typing hint.
func TestAtlasOtherListMessagesUnchanged(t *testing.T) {
	loading := strings.TrimSpace(renderResultList(atlasModalState{}, Config{}, 80, 5))
	if loading != "(loading atlas…)" {
		t.Errorf("loading line = %q", loading)
	}
	none := strings.TrimSpace(renderResultList(liftState("zzz", 0), Config{}, 80, 5))
	if none != "(no matches)" {
		t.Errorf("no-match line = %q", none)
	}
	failed := atlasModalState{loadErr: errors.New("boom")}
	if got := strings.TrimSpace(renderResultList(failed, Config{}, 80, 5)); got != strings.TrimSpace(atlasModalHint) {
		t.Errorf("load-error line = %q, want atlasModalHint %q", got, atlasModalHint)
	}
}
