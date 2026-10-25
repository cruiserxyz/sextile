package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/env"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui"
)

func main() {
	// Flags first: the flag package answers --help and -h itself and exits,
	// before anything else runs. registerFlags reads no environment, so parsing
	// ahead of env.Load is safe.
	opts := registerFlags(flag.CommandLine)
	flag.Parse()
	if opts.version { // before anything touches the screen, a chart folder or a session
		env.Load() // --version may read .env, as it always has; it takes no lock
		os.Exit(runVersion(os.Stdout))
	}

	// The lock's holder owns the session (the specification): the lock is
	// taken before the chart folder, the preferences or the session are read,
	// and after the environment is loaded, because a dev .env can set
	// XDG_DATA_HOME and the lock file lives in SessionDir(): a lock taken
	// first would sit in a different folder from the session every later read
	// uses, and two windows would each hold "the" lock on the same session. A
	// copy that finds it held by another window runs as a second window, which
	// never writes the session; any other failure refuses having touched
	// nothing but the environment (a dev build may have read, or reported, its
	// .env first).
	lock, lockErr := acquireLockAfterEnv()
	lockCfg, refused := lockOutcome(lock, lockErr)
	if refused != nil {
		os.Exit(runProgram(ui.NewLockRefusedModel(ui.Config{
			Accessible: opts.accessible,
			Color:      resolveColorMode(opts.color),
		}, refused)))
	}

	if opts.chartsDir != "" {
		if err := setChartsDir(opts.chartsDir); err != nil {
			fmt.Fprintf(os.Stderr, "sextile: invalid --charts-dir %q: %v\n", opts.chartsDir, err)
			os.Exit(2)
		}
	}
	cfg := ui.Config{
		Accessible:              opts.accessible,
		Color:                   resolveColorMode(opts.color),
		InstanceLockUnsupported: lockCfg.InstanceLockUnsupported,
		SecondWindow:            lockCfg.SecondWindow,
		Launch:                  os.Getenv("SEXTILE_LAUNCH"),
	}
	status := runProgram(ui.NewModel(cfg))
	runtime.KeepAlive(lock) // the descriptor holding the lock lives as long as the program
	os.Exit(status)
}

// loadEnv is env.Load; a test swaps it to stand in for a .env, which an
// untagged build never reads.
var loadEnv = env.Load

// acquireLockAfterEnv seeds the process environment from the repo-root .env
// (a no-op in a release build) and then takes the instance lock, so the lock
// and every later SessionDir() read resolve XDG_DATA_HOME from the same
// environment.
func acquireLockAfterEnv() (*store.InstanceLock, error) {
	loadEnv()
	return store.AcquireInstanceLock()
}

// lockOutcome is what the launch does with AcquireInstanceLock's answer: the
// lock fields of the Config for a window that runs, or the error that refuses
// the launch. A lock held by another window is a second window, not a refusal.
func lockOutcome(lock *store.InstanceLock, err error) (ui.Config, error) {
	var held *store.LockHeldError
	switch {
	case errors.As(err, &held):
		return ui.Config{SecondWindow: true}, nil
	case err != nil:
		return ui.Config{}, err
	}
	return ui.Config{InstanceLockUnsupported: lock.Unsupported}, nil
}

// runProgram runs one model on the alternate screen and returns the exit status.
func runProgram(m tea.Model) int {
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	final, err := p.Run()
	return finish(final, err, os.Stderr)
}

// finish reports what the program left unsaid once the screen is restored,
// and returns the exit status: 1 if the program itself failed, else 0. A
// session that could not be written at quit is one line on stderr and does
// not change the status (the specification A7). A write to stderr that
// fails is ignored; there is nowhere left to report it.
func finish(final tea.Model, runErr error, stderr io.Writer) int {
	status := 0
	if runErr != nil {
		fmt.Fprintf(stderr, "sextile: %v\n", runErr)
		status = 1
	}
	if m, ok := final.(sessionReporter); ok {
		if err := m.SessionWriteError(); err != nil {
			fmt.Fprintf(stderr, "sextile: %v\n", err)
		}
	}
	return status
}

// sessionReporter is the final model's one question after quit. ui.Model
// answers it; the assertion below keeps that true at compile time.
type sessionReporter interface {
	SessionWriteError() error
}

var _ sessionReporter = ui.Model{}

type options struct {
	chartsDir  string
	accessible bool
	color      string
	version    bool
}

// readBuildInfo is the toolchain's build record; a test swaps it.
var readBuildInfo = debug.ReadBuildInfo

// version is set at build time with -ldflags "-X main.version=...".
var version string

// versionString is line 1 of --version's answer, after "sextile ": the -X
// value if non-empty; else the module version the toolchain recorded, unless
// it is "(devel)"; else "devel (<revision[:7]>)", the revision reading "<revision[:7]>+dirty" for a
// modified tree; else "devel".
func versionString(ldflag string, info *debug.BuildInfo, haveInfo bool) string {
	if ldflag != "" {
		return ldflag
	}
	if !haveInfo || info == nil {
		return "devel"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "devel"
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if dirty {
		rev += "+dirty"
	}
	return "devel (" + rev + ")"
}

// astrologLine is line 2 of --version's answer. It only asks Astrolog for its
// version; the exit status of --version does not depend on the answer.
func astrologLine() string {
	v, path, err := lookup.AstrologVersion()
	switch {
	case err == nil:
		return fmt.Sprintf("astrolog %s (%s)", v, path)
	case errors.Is(err, lookup.ErrAstrologNotInstalled):
		return "astrolog: not found"
	default:
		return fmt.Sprintf("astrolog: %s did not report a version", path)
	}
}

// runVersion prints the two lines a bug report needs and returns the exit
// status, which is always 0.
func runVersion(w io.Writer) int {
	info, ok := readBuildInfo()
	fmt.Fprintf(w, "sextile %s\n", versionString(version, info, ok))
	fmt.Fprintln(w, astrologLine())
	return 0
}

func registerFlags(fs *flag.FlagSet) *options {
	opts := &options{}
	fs.StringVar(&opts.chartsDir, "charts-dir", "", "directory containing the chart library (Astrolog chart files)")
	fs.BoolVar(&opts.accessible, "accessible", false, "use ASCII box-drawing glyphs; choose color independently with --color")
	fs.StringVar(&opts.color, "color", "auto", "color policy: auto, always, or never")
	fs.BoolVar(&opts.version, "version", false, "print Sextile's version and the Astrolog it finds, then exit; checks only Astrolog's version answer (no ephemeris or atlas check)")
	fs.Usage = func() { printUsage(fs) }
	return opts
}

// setChartsDir gives an explicit launch flag precedence over environment and
// .env defaults. Relative CLI paths follow normal command-line expectations:
// they are resolved from the directory where Sextile was launched.
func setChartsDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	return os.Setenv("SEXTILE_CHARTS_DIR", abs)
}

func printUsage(fs *flag.FlagSet) {
	out := fs.Output()
	fmt.Fprintln(out, "Sextile — a focused chart workspace for professional astrological practice.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  sextile [options]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Launch with a chart library:")
	fmt.Fprintln(out, "  sextile --charts-dir ~/Clients/charts")
	fmt.Fprintln(out, "  sextile --charts-dir ./charts")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "The directory holds one Astrolog chart per file, found by its first bytes and not")
	fmt.Fprintln(out, "its name. Charts saved during this session are written there as name.as.")
	fmt.Fprintln(out, "Relative paths are resolved from your current working directory. The flag takes")
	fmt.Fprintln(out, "precedence over SEXTILE_CHARTS_DIR.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Options:")
	fs.PrintDefaults()
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Environment:")
	fmt.Fprintln(out, "  SEXTILE_CHARTS_DIR  Default chart-library directory when --charts-dir is omitted.")
	fmt.Fprintln(out, "  SEXTILE_COLOR       Default color policy: auto, always, or never.")
	fmt.Fprintln(out, "  NO_COLOR            Forces color off, following the cross-tool standard.")
}

// resolveColorMode maps the --color flag and the environment to a ui.ColorMode.
// Precedence: NO_COLOR (the cross-tool standard) forces Never; an explicit
// --color=always|never wins next; a flag left at "auto" defers to SEXTILE_COLOR;
// anything unrecognised falls back to Auto (color unless --accessible).
func resolveColorMode(flagVal string) ui.ColorMode {
	if os.Getenv("NO_COLOR") != "" {
		return ui.ColorNever
	}
	mode := strings.ToLower(strings.TrimSpace(flagVal))
	if mode == "" || mode == "auto" {
		mode = strings.ToLower(strings.TrimSpace(os.Getenv("SEXTILE_COLOR")))
	}
	switch mode {
	case "always", "on", "force":
		return ui.ColorAlways
	case "never", "off", "none":
		return ui.ColorNever
	default: // "auto", "", or unrecognised
		return ui.ColorAuto
	}
}
