package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRegisterFlagsParsesOperatorOptions(t *testing.T) {
	fs := flag.NewFlagSet("sextile", flag.ContinueOnError)
	fs.SetOutput(&bytes.Buffer{})
	opts := registerFlags(fs)

	if err := fs.Parse([]string{"--charts-dir", "client-charts", "--accessible", "--color=never"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if opts.chartsDir != "client-charts" || !opts.accessible || opts.color != "never" {
		t.Fatalf("parsed options = %+v", *opts)
	}
}

func TestRegisterFlagsRejectsUnknownOption(t *testing.T) {
	var output bytes.Buffer
	fs := flag.NewFlagSet("sextile", flag.ContinueOnError)
	fs.SetOutput(&output)
	registerFlags(fs)

	if err := fs.Parse([]string{"--not-an-option"}); err == nil {
		t.Fatal("Parse accepted an unknown option")
	}
	if !strings.Contains(output.String(), "flag provided but not defined") {
		t.Fatalf("error output omitted the cause:\n%s", output.String())
	}
}

func TestUsageDocumentsOperatorSurface(t *testing.T) {
	var output bytes.Buffer
	fs := flag.NewFlagSet("sextile", flag.ContinueOnError)
	fs.SetOutput(&output)
	registerFlags(fs)
	fs.Usage()

	for _, want := range []string{
		"sextile --charts-dir ~/Clients/charts",
		"-accessible",
		"-charts-dir",
		"-color",
		"SEXTILE_CHARTS_DIR",
		"NO_COLOR",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("usage omitted %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "row-focus") {
		t.Errorf("usage still advertises the retired --row-focus:\n%s", output.String())
	}
}

// the specification C18: --row-focus is gone, and the built
// binary refuses it the way it refuses any flag it does not know. The binary
// is built and run, because a flag can be registered somewhere other than
// registerFlags and still be accepted.
func TestRowFocusRetired(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "sextile")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build sextile: %v\n%s", err, out)
	}
	for _, args := range [][]string{{"--row-focus"}, {"--row-focus=indicator"}, {"-row-focus", "invert"}} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		cmd := exec.CommandContext(ctx, bin, args...)
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		err := cmd.Run()
		cancel()
		exit, ok := err.(*exec.ExitError)
		if !ok || exit.ExitCode() == 0 {
			t.Errorf("sextile %v: want a non-zero exit, got %v:\n%s", args, err, out.String())
			continue
		}
		if !strings.Contains(out.String(), "flag provided but not defined: -row-focus") {
			t.Errorf("sextile %v: want Go's flag diagnostic for -row-focus, got:\n%s", args, out.String())
		}
	}
}

func TestSetChartsDirMakesRelativePathAbsoluteAndOverridesEnvironment(t *testing.T) {
	t.Setenv("SEXTILE_CHARTS_DIR", "/from-environment")
	workdir := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	if err := os.Chdir(workdir); err != nil {
		t.Fatal(err)
	}

	if err := setChartsDir("client-charts"); err != nil {
		t.Fatalf("setChartsDir: %v", err)
	}
	if got, want := os.Getenv("SEXTILE_CHARTS_DIR"), filepath.Join(workdir, "client-charts"); got != want {
		t.Errorf("SEXTILE_CHARTS_DIR = %q, want %q", got, want)
	}
}

func TestSetChartsDirReturnsEnvironmentError(t *testing.T) {
	if err := setChartsDir("invalid\x00path"); err == nil {
		t.Fatal("setChartsDir accepted a path containing NUL")
	}
}
