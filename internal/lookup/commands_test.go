package lookup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/payload"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The verifiers C11-C14: lookup carries every spawn's command through, on
// the error path too, and composes none of its own.

func commandsTestPlace() location.PlaceInfo {
	return location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"}
}

func commandsStub(commands []string, err error) *Probers {
	stub := ChartComputer(func(context.Context, recipe.ChartSpec) (payload.RawOutput, error) {
		return payload.RawOutput{
			Positions: []aspect.Position{{Body: aspect.Sun, Longitude: 35.5}},
			Commands:  commands,
		}, err
	})
	return &Probers{Full: stub, PositionsOnly: stub}
}

// C11: Result carries the commands exactly as the compute completed them.
func TestResultCarriesCommandsInCompletionOrder(t *testing.T) {
	want := []string{"first spawn", "second spawn"}
	probers := commandsStub(want, nil)
	when := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	transit, err := FirstCallTransits(context.Background(), probers, when, commandsTestPlace(), Hellenistic)
	if err != nil {
		t.Fatalf("FirstCallTransits: %v", err)
	}
	if !slices.Equal(transit.Commands, want) {
		t.Errorf("FirstCallTransits Commands = %q, want %q", transit.Commands, want)
	}
	natal, err := FirstCallNatalPositions(context.Background(), probers, store.AnchorChart{When: when, Place: commandsTestPlace()}, Hellenistic)
	if err != nil {
		t.Fatalf("FirstCallNatalPositions: %v", err)
	}
	if !slices.Equal(natal.Commands, want) {
		t.Errorf("FirstCallNatalPositions Commands = %q, want %q", natal.Commands, want)
	}
}

// C12: both raw-report wrappers return the executed command beside the
// report. The single-chart command is the acceptance evidence of the ticket:
// pasted into a shell, Astrolog prints the same report byte for byte. The
// comparison command names staged inputs that are gone by the time it is
// read — accurate, not replayable (documented non-goal).
func TestRawReportReturnsCommand(t *testing.T) {
	resolved, err := exec.LookPath("astrolog")
	if err != nil {
		t.Skip("astrolog not on PATH; skipping live raw-report test")
	}
	place := commandsTestPlace()
	when := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)

	report, command, err := ComputeRawReport(context.Background(), when, place, Hellenistic, RawReportRequest{Kind: RawReportChart})
	if err != nil {
		t.Fatalf("ComputeRawReport: %v", err)
	}
	if !strings.HasPrefix(command, resolved+" ") {
		t.Fatalf("command %q does not start with the resolved binary %q", command, resolved)
	}
	pasted, err := exec.Command("sh", "-c", command).CombinedOutput()
	if err != nil {
		t.Fatalf("pasting the recorded command failed: %v\n%s", err, pasted)
	}
	// The compute returns reportText's one translation of the same bytes: the
	// degree character (0xF8) as U+00B0 (the specification C8).
	if strings.ReplaceAll(string(pasted), "\xf8", "°") != report {
		t.Errorf("pasted command output differs from the report the compute captured\ncommand: %s", command)
	}

	natal := store.AnchorChart{When: time.Date(1990, 5, 15, 12, 0, 0, 0, time.UTC), Place: place}
	_, crossCommand, err := CrossBodyRawReport(context.Background(), Hellenistic, natal, when, place, aspect.Moon)
	if err != nil {
		t.Fatalf("CrossBodyRawReport: %v", err)
	}
	if !strings.HasPrefix(crossCommand, resolved+" ") || !strings.Contains(crossCommand, " -rt ") || !strings.Contains(crossCommand, " -RO ") {
		t.Errorf("cross body command = %q; want the resolved binary running -rt … -RO", crossCommand)
	}
}

// C13: the comparison path has a carrier at every layer — payload and lookup
// alike — and the overlay compute fills it with probe then relationship.
func TestOverlayAspectsCarryCommands(t *testing.T) {
	var _ = payload.CrossAspectOutput{Commands: nil}
	var _ = OverlayAspects{Commands: nil}
	resolved, err := exec.LookPath("astrolog")
	if err != nil {
		t.Skip("astrolog not on PATH; skipping live overlay test")
	}
	place := commandsTestPlace()
	natal := store.AnchorChart{When: time.Date(1990, 5, 15, 12, 0, 0, 0, time.UTC), Place: place}
	got, err := OverlayCrossAspects(context.Background(), Hellenistic, natal, time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC), place)
	if err != nil {
		t.Fatalf("OverlayCrossAspects: %v", err)
	}
	if len(got.Commands) != 2 {
		t.Fatalf("Commands = %q, want the probe and the relationship run", got.Commands)
	}
	if !strings.HasPrefix(got.Commands[0], resolved+" ") || !strings.Contains(got.Commands[0], " '=v0' -Yw 0 ") {
		t.Errorf("first command is not the ephemeris probe: %q", got.Commands[0])
	}
	if !strings.HasPrefix(got.Commands[1], resolved+" ") || !strings.Contains(got.Commands[1], " -rt ") {
		t.Errorf("second command is not the relationship run: %q", got.Commands[1])
	}
}

// C14: a wrapper handed commands together with an error returns them beside
// the wrapped error instead of a zero value.
func TestWrappersPropagateCommandsOnError(t *testing.T) {
	boom := errors.New("boom")
	ran := []string{"the spawn that ran"}
	probers := commandsStub(ran, boom)
	when := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	transit, err := FirstCallTransits(context.Background(), probers, when, commandsTestPlace(), Hellenistic)
	if !errors.Is(err, boom) {
		t.Fatalf("FirstCallTransits err = %v, want boom", err)
	}
	if !slices.Equal(transit.Commands, ran) {
		t.Errorf("FirstCallTransits dropped the commands on error: %q", transit.Commands)
	}
	natal, err := FirstCallNatalPositions(context.Background(), probers, store.AnchorChart{When: when, Place: commandsTestPlace()}, Hellenistic)
	if !errors.Is(err, boom) {
		t.Fatalf("FirstCallNatalPositions err = %v, want boom", err)
	}
	if !slices.Equal(natal.Commands, ran) {
		t.Errorf("FirstCallNatalPositions dropped the commands on error: %q", natal.Commands)
	}

	// OverlayCrossAspects reaches astrolog directly, so the failure comes
	// from a real spawn: an `astrolog` first on PATH that starts and exits 3.
	// The probe runs and fails; its command must survive the wrapping.
	dir := t.TempDir()
	fake := filepath.Join(dir, "astrolog")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	chart := store.AnchorChart{When: when.AddDate(-30, 0, 0), Place: commandsTestPlace()}
	overlay, err := OverlayCrossAspects(context.Background(), Hellenistic, chart, when, commandsTestPlace())
	if err == nil {
		t.Fatal("a failing astrolog must fail the overlay compute")
	}
	if len(overlay.Commands) != 1 || !strings.HasPrefix(overlay.Commands[0], fake+" ") {
		t.Errorf("OverlayCrossAspects dropped the probe command on error: %q", overlay.Commands)
	}
}
