package astrolog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// A canceled context must surface as context.Canceled (not a generic exec
// error), so the refresh coordinator can drop a superseded run silently. A
// pre-canceled context is the deterministic case: exec.CommandContext refuses
// to start the process and returns ctx.Err(); the mid-flight SIGKILL path is
// the same stdlib contract, normalized through the same ctx.Err() check.
func TestInvoke_CanceledContextSurfacesContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := invoke(ctx, []string{"-qb", "Jun", "15", "1985", "2:30pm", "DT", "1E", "11:34E", "48:08N"})
	if err == nil {
		t.Fatal("invoke with a canceled context returned nil error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want errors.Is(err, context.Canceled), got %v", err)
	}
}

// The cancellation signal must propagate through the public ComputeChart API
// (Run -> invoke), with the wrapping preserved so callers can still match it.
func TestComputeChart_CanceledContextSurfacesContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ComputeChart(ctx, hellenisticSpecAt(t, 2451545.0, 11.5667, 48.1333))
	if err == nil {
		t.Fatal("ComputeChart with a canceled context returned nil error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want errors.Is(err, context.Canceled), got %v", err)
	}
}

// A normal (uncanceled) compute must NOT be misreported as canceled — the
// ctx.Err() guard only fires when the context is actually done.
func TestComputeChart_UncanceledSucceeds(t *testing.T) {
	if _, err := ComputeChart(context.Background(), hellenisticSpecAt(t, 2451545.0, 11.5667, 48.1333)); err != nil {
		t.Fatalf("uncanceled ComputeChart failed: %v", err)
	}
}

// Every public astrolog entry point funnels through invoke, so each must
// preserve the cancellation contract end-to-end (errors.Is(context.Canceled)).
// Locking it here means a future wrapper-error change at any of them can't
// silently break the contract the refresh coordinator relies on.
func TestPublicEntryPoints_PreserveCancellation(t *testing.T) {
	spec := hellenisticSpecAt(t, 2451545.0, 11.5667, 48.1333)
	canceled := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	cases := []struct {
		name string
		run  func(context.Context) error
	}{
		{"ComputeChartPositionsOnly", func(c context.Context) error { _, e := ComputeChartPositionsOnly(c, spec); return e }},
		{"ComputeRawReport", func(c context.Context) error { _, _, e := ComputeRawReport(c, spec, recipe.ChartRawReport()); return e }},
		{"RenderWheelImage", func(c context.Context) error {
			_, e := RenderWheelImage(c, spec, recipe.DefaultWheelRender())
			return e
		}},
		{"ResolveChartFile", func(c context.Context) error { _, e := ResolveChartFile(c, fixture("Equinox-2026-03.as")); return e }},
	}
	for _, c := range cases {
		err := c.run(canceled())
		if !errors.Is(err, context.Canceled) {
			t.Errorf("%s: want errors.Is(context.Canceled), got %v", c.name, err)
		}
	}
}

// The mid-flight SIGKILL path, deterministically: substitute the binary for a
// long `sleep`, cancel after it has certainly started, and assert invoke both
// aborts PROMPTLY (the subprocess was killed, not awaited) and surfaces the
// context error. Exercises the real CommandContext kill, not just pre-start
// refusal.
func TestInvoke_MidFlightCancelKillsPromptly(t *testing.T) {
	// A script, not sleep itself: every call now starts with -YQ 0 (C1), which
	// sleep would reject. The script ignores its arguments and sleeps 30s.
	script := filepath.Join(t.TempDir(), "slow")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := binary
	binary = script
	t.Cleanup(func() { binary = old })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, _, err := invoke(ctx, []string{"30"}) // would block 30s if not killed
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("invoke did not abort promptly on cancel (%s) — subprocess not killed", elapsed)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want errors.Is(context.Canceled), got %v", err)
	}
}
