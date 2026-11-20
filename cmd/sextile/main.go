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

	"github.com/cruiserxyz/sextile"
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

	cfg, lock, refused, err := prepareLaunch(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sextile: invalid --charts-dir %q: %v\n", opts.chartsDir, err)
		os.Exit(2)
	}
	if refused != nil {
		os.Exit(runProgram(ui.NewLockRefusedModel(ui.Config{
			Accessible: opts.accessible,
			Color:      resolveColorMode(opts.color),
		}, refused)))
	}
	if err := recordLauncherWord(store.ChartsDir()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	status := runProgram(ui.NewModel(cfg))
	runtime.KeepAlive(lock) // the descriptor holding the lock lives as long as the program
	os.Exit(status)
}

// prepareLaunch is the launch's order, and the order is the safety:
//
//  1. the environment (a dev .env can set the XDG bases);
//  2. the charts folder, so --charts-dir takes precedence before anything
//     reads it;
//  3. the instance lock, before the preferences or the session are read: the
//     lock lives in SessionDir(), so it must resolve from the same environment
//     every later read uses. A copy that finds it held runs as a second
//     window, which never writes the session; any other failure refuses
//     (refused), having touched nothing but the lock's folder;
//  4. only then, and only for the window that holds the lock (or runs where
//     locking is unsupported), the one-time copy of the charts folder's old
//     state, from the folder the flag chose.
//
// err is an unusable --charts-dir.
func prepareLaunch(opts *options) (cfg ui.Config, lock *store.InstanceLock, refused error, err error) {
	loadEnv()
	if opts.chartsDir != "" {
		if err := setChartsDir(opts.chartsDir); err != nil {
			return ui.Config{}, nil, nil, err
		}
	}
	lock, lockErr := store.AcquireInstanceLock()
	lockCfg, refused := lockOutcome(lock, lockErr)
	if refused != nil {
		return ui.Config{}, nil, refused, nil
	}
	cfg = ui.Config{
		Accessible:              opts.accessible,
		Color:                   resolveColorMode(opts.color),
		InstanceLockUnsupported: lockCfg.InstanceLockUnsupported,
		SecondWindow:            lockCfg.SecondWindow,
		Launch:                  os.Getenv("SEXTILE_LAUNCH"),
		Version:                 runningVersion(),
		Copyright:               sextile.Copyright(),
	}
	if !cfg.SecondWindow {
		cfg.Migration = store.MigrateLegacyState(store.ChartsDir()).Line()
	}
	if opts.session != "" {
		cfg = openSessionFlag(cfg, opts.session)
	} else {
		cfg = reopenLastSession(cfg)
	}
	return cfg, lock, nil, nil
}

// reopenLastSession is a start without --session: it runs the named session
// this charts folder's window last switched to, as --session would. One that
// is no longer saved, cannot be used, or is open in another window is not
// opened: the launch goes on with the unnamed session and one Log line says
// which session and why.
func reopenLastSession(cfg ui.Config) ui.Config {
	file, name, lock, err := store.ReopenLastSession(store.ChartsDir())
	switch {
	case file == "" && err == nil:
		return cfg
	case file == "":
		// The record itself could not be read: nothing was asked for by
		// name, so the start is the usual one and the reason is a trace.
		cfg.LastSessionTrace = "Sextile could not read which session you used last: " + firstLine(err.Error())
		return cfg
	case err != nil:
		cfg.SessionNote = fmt.Sprintf("the last session %q did not open, %s", name, reopenRefusal(err))
		var stale *store.RecordNotClearedError
		if errors.As(err, &stale) {
			cfg.LastSessionTrace = "the record of the last session could not be removed: " + firstLine(stale.Err.Error())
		}
		return cfg
	}
	return runNamedSession(cfg, lock, name)
}

// reopenRefusal is why the last session was not reopened, in the user's
// words and short enough for the Log line to fit beside the session's name.
func reopenRefusal(err error) string {
	switch {
	case errors.Is(err, store.ErrSessionNotFound):
		return "it is no longer saved"
	case errors.Is(err, store.ErrSessionOpenElsewhere):
		return "it is open in another window"
	case errors.Is(err, store.ErrSessionUnreadable), errors.Is(err, store.ErrSessionUnknownContent):
		return "its file cannot be used"
	case errors.Is(err, store.ErrSessionOtherFolder):
		return "it belongs to another charts folder"
	}
	return "its file cannot be read"
}

// runNamedSession hands the window the named session whose lock was taken,
// letting go of the general one: a window holds exactly one.
func runNamedSession(cfg ui.Config, lock *store.SessionLock, name string) ui.Config {
	if !cfg.SecondWindow {
		store.ReleaseInstanceLock()
	}
	cfg.NamedSession, cfg.NamedSessionName = lock, name
	return cfg
}

// firstLine is s up to its first line break.
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// openSessionFlag is --session NAME: the launch runs that named session from
// the start, holding its lock and letting go of the general one -- a window
// holds exactly one. A session that is missing, open in another window, or
// saved in another charts folder is not opened: the launch goes on as usual
// and one Log line says why.
func openSessionFlag(cfg ui.Config, name string) ui.Config {
	_, display, lock, err := store.OpenNamedSessionByName(name, store.FolderID(store.ChartsDir()))
	if err != nil {
		why := err.Error()
		if errors.Is(err, store.ErrSessionNotFound) {
			why = "there is no saved session of that name in " + store.SessionsDir()
		}
		cfg.SessionNote = fmt.Sprintf("--session %q did not open, %s, started as usual", name, why)
		return cfg
	}
	return runNamedSession(cfg, lock, display)
}

// recordLauncherWord writes the charts folder's position the launcher handed
// over, before the first frame: SEXTILE_ORIGINALS_SET=<word> is --originals,
// and a word that cannot be recorded stops the launch with the reason;
// SEXTILE_ORIGINALS_CREATED=1 means the launcher made the folder, which is
// recorded as add, and if that fails the folder stays at ask.
func recordLauncherWord(dir string) error {
	if w := os.Getenv("SEXTILE_ORIGINALS_SET"); w != "" {
		if err := store.SetPositionReplacing(dir, store.Position(w)); err != nil {
			if errors.Is(err, store.ErrFolderBusy) {
				return fmt.Errorf("Another Sextile window is open on this charts folder.\nClose it, then start again.")
			}
			return fmt.Errorf("The charts folder could not be set to %s.\n%v", w, err)
		}
		return nil
	}
	if os.Getenv("SEXTILE_ORIGINALS_CREATED") == "1" {
		_ = store.SetPosition(dir, store.PositionAdd)
	}
	return nil
}

// loadEnv is env.Load; a test swaps it to stand in for a .env, which an
// untagged build never reads.
var loadEnv = env.Load

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

// Terminal title stack (xterm): push saves the terminal's own title, pop puts
// it back. One push before the program's first title, one pop on every way out.
var (
	titlePush = "\x1b[22" + semicolon + "0t"
	titlePop  = "\x1b[23" + semicolon + "0t"
)

// semicolon is spelled apart: the terminal's parameter separator is not text
// for a person.
const semicolon = "\x3b"

// runProgram runs one model on the alternate screen and returns the exit status.
func runProgram(m tea.Model) int {
	return runProgramOn(m, os.Stdout, os.Stdin)
}

// runProgramOn is runProgram on the given terminal streams. The terminal's own
// title is pushed before the program can set one, and popped by the deferred
// call, so a quit, Ctrl+C, the can't-start screen and a panic that unwinds
// through here all give it back.
func runProgramOn(m tea.Model, out io.Writer, in io.Reader) int {
	io.WriteString(out, titlePush)
	defer io.WriteString(out, titlePop)
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithOutput(out),
		tea.WithInput(in),
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
	session    string
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

// runningVersion is the version this binary reports, as --version prints it.
func runningVersion() string {
	info, ok := readBuildInfo()
	return versionString(version, info, ok)
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
	fs.StringVar(&opts.chartsDir, "charts-dir", "", "the charts folder (Astrolog chart files)")
	fs.BoolVar(&opts.accessible, "accessible", false, "use ASCII box-drawing glyphs, and choose color independently with --color")
	fs.StringVar(&opts.color, "color", "auto", "color policy: auto, always, or never")
	fs.StringVar(&opts.session, "session", "", "open the session NAME (saved with , O in this charts folder)")
	fs.BoolVar(&opts.version, "version", false, "print Sextile's version and the Astrolog it finds, then exit — checks only Astrolog's version answer (no ephemeris or atlas check)")
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
	fmt.Fprintln(out, "Sextile shows astrology charts in a terminal window.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  sextile [options]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Launch with a chart library:")
	fmt.Fprintln(out, "  sextile --charts-dir ~/Clients/charts")
	fmt.Fprintln(out, "  sextile --charts-dir ./charts")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "The folder holds one Astrolog chart in each file. Sextile knows a chart by its first")
	fmt.Fprintln(out, "characters, not by its name. A chart that you save is written there as name.as.")
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
