package astrolog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cruiserxyz/sextile/internal/compose"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

// RenderWheelImage compiles the WheelImage projection for a chart spec and
// runs astrolog to produce a wheel image, returning its bytes. It is the
// image analogue of ComputeChart: a validated recipe.ChartSpec in, a rendered
// chart out, styled by the caller-supplied render (build one via
// DefaultWheelRender and adjust). Sextile targets Astrolog 8.00 and
// explicitly requests its native output format.
//
// A plain render is one astrolog run. An active highlight compiles to the
// an earlier change three-layer plan — base, foreground, furniture — rendered
// sequentially under ctx and assembled by internal/compose into the dimmed
// wheel with the highlight members and their aspect web at full color. Any
// layer failure fails the whole frame: a half-dimmed or undimmed wheel is
// never returned (M14), and a cancellation between layers surfaces as
// context.Canceled so the coordinator drops the frame silently (M15).
func RenderWheelImage(ctx context.Context, spec recipe.ChartSpec, render recipe.RenderSpec) ([]byte, error) {
	img, _, err := RenderWheelImageCommands(ctx, spec, render)
	return img, err
}

// RenderWheelImageCommands is RenderWheelImage that also returns the command
// of every spawn that started, in order, for the console's argv lane (gate
// 12). The commands come back on failure too: a render that went wrong is
// still a command somebody may need to read.
func RenderWheelImageCommands(ctx context.Context, spec recipe.ChartSpec, render recipe.RenderSpec) ([]byte, []string, error) {
	plan, err := Compile(spec, recipe.WheelImage{Render: render})
	if err != nil {
		return nil, nil, fmt.Errorf("astrolog: RenderWheelImage: %w", err)
	}
	var commands []string
	if len(plan.Invocations) == 1 {
		img, command, err := renderImage(ctx, plan.Invocations[0])
		if command != "" {
			commands = append(commands, command)
		}
		return img, commands, err
	}
	layers := make([][]byte, len(plan.Invocations))
	for i, inv := range plan.Invocations {
		if err := ctx.Err(); err != nil {
			return nil, commands, err
		}
		var command string
		layers[i], command, err = renderImage(ctx, inv)
		if command != "" {
			commands = append(commands, command)
		}
		if err != nil {
			return nil, commands, err
		}
	}
	out, err := compose.Composite(layers[0], layers[1], layers[2])
	if err != nil {
		return nil, commands, fmt.Errorf("astrolog: RenderWheelImage: %w", err)
	}
	return out, commands, nil
}

// RenderImage runs a single image Invocation: it appends `-Xo <tmp>` (the sink
// path is an execution concern, not part of the compiled argv), invokes
// astrolog, and returns the written image bytes. Image invocations BYPASS the
// text sanity / parse pipeline (the I5 invariant) — astrolog writes a file, not
// a parseable chart, so checkPositionSanity/parsePositions never run here.
func RenderImage(ctx context.Context, inv Invocation) ([]byte, error) {
	img, _, err := renderImage(ctx, inv)
	return img, err
}

// renderImage is RenderImage that also returns the command that ran ("" when
// no spawn started).
func renderImage(ctx context.Context, inv Invocation) ([]byte, string, error) {
	if inv.Artifact != ImageFile {
		return nil, "", fmt.Errorf("astrolog: RenderImage: invocation is not an image (artifact %d)", inv.Artifact)
	}
	// Stage any comparison chart files (-r0 bi-wheel). For a single chart this
	// is a no-op and the image goes to its own temp; for a comparison the image
	// shares the staged dir so one RemoveAll cleans every artifact.
	staged, dir, cleanup, err := stageCall(inv)
	if err != nil {
		return nil, "", fmt.Errorf("astrolog: RenderImage: %w", err)
	}
	defer cleanup()

	var tmpPath string
	if dir != "" {
		tmpPath = filepath.Join(dir, "wheel"+imageExtension(inv.Argv))
	} else {
		tmp, err := os.CreateTemp("", "sextile-wheel-*"+imageExtension(inv.Argv))
		if err != nil {
			return nil, "", fmt.Errorf("astrolog: RenderImage: temp file: %w", err)
		}
		tmpPath = tmp.Name()
		_ = tmp.Close()
		defer os.Remove(tmpPath)
	}

	// The sink (-Xo) is appended at run time. validateArgv still applies: the
	// argv must carry a chart-entry switch and no naked -b (the compiler
	// guarantees both, but the runner stays defensive).
	call, err := staged.with("-Xo", tmpPath)
	if err != nil {
		return nil, "", fmt.Errorf("astrolog: RenderImage: %w", err)
	}
	if err := validateArgv(call.argv); err != nil {
		return nil, "", err
	}
	out, command, err := spawn(ctx, call)
	if err != nil {
		return nil, command, fmt.Errorf("astrolog image invocation: %w (output: %s)", err, out)
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, command, fmt.Errorf("astrolog: RenderImage: read output: %w", err)
	}
	if len(data) == 0 {
		return nil, command, fmt.Errorf("astrolog: RenderImage: astrolog produced an empty image")
	}
	return data, command, nil
}

func imageExtension(argv []string) string {
	for _, arg := range argv {
		if arg == "-Xbp" {
			return ".png"
		}
	}
	return ".svg"
}
