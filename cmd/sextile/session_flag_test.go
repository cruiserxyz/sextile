package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// All chart values in this file are constructed synthetic test inputs.

// flagSandbox gives one launch its own XDG bases and two charts folders, and
// makes A the folder the launch uses.
func flagSandbox(t *testing.T) (a, b string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	a, b = filepath.Join(root, "A"), filepath.Join(root, "B")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SEXTILE_CHARTS_DIR", a)
	return a, b
}

func savedSession(t *testing.T, name, folder string) {
	t.Helper()
	s := store.Session{
		Folder: store.FolderID(folder),
		Slots: []store.SessionSlot{{
			Chart: store.SessionChart{ID: "c", Name: "Client C", When: time.Date(1980, 1, 2, 3, 4, 0, 0, time.UTC),
				Place: location.PlaceInfo{Name: "Testford", Country: "TS", Lat: 41, Lon: -71, TZ: "America/New_York"}},
			Usable: true,
		}},
	}
	l, err := store.SaveNamedSessionAs(name, s)
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
}

// `sextile -- --session NAME` starts running that named session; missing,
// locked or from another folder, the launch starts as usual and one Log line
// says why.
func TestSessionFlag(t *testing.T) {
	fs := flag.NewFlagSet("sextile", flag.ContinueOnError)
	opts := registerFlags(fs)
	if err := fs.Parse([]string{"--session", "Client Work"}); err != nil || opts.session != "Client Work" {
		t.Fatalf("--session is not a flag: %v (%q)", err, opts.session)
	}

	t.Run("found", func(t *testing.T) {
		a, _ := flagSandbox(t)
		savedSession(t, "Client Work", a)
		cfg, _, refused, err := prepareLaunch(&options{session: "client work"})
		if refused != nil || err != nil {
			t.Fatalf("refused %v err %v", refused, err)
		}
		defer cfg.NamedSession.Release()
		if cfg.NamedSession.File() != "client-work.yaml" || cfg.NamedSessionName != "Client Work" || cfg.SessionNote != "" {
			t.Errorf("cfg runs %q named %q, note %q", cfg.NamedSession.File(), cfg.NamedSessionName, cfg.SessionNote)
		}
		// The general lock was let go: a window holds exactly one.
		lock, lerr := store.AcquireInstanceLock()
		if lerr != nil {
			t.Errorf("the general session's lock is still held: %v", lerr)
		}
		_ = lock
		store.ReleaseInstanceLock()
	})

	for _, c := range []struct {
		name, want string
		setup      func(t *testing.T, a, b string) func()
	}{
		{"missing", "no saved session", func(t *testing.T, a, b string) func() { return func() {} }},
		{"locked", "open in another Sextile window", func(t *testing.T, a, b string) func() {
			savedSession(t, "Client Work", a)
			_, _, l, err := store.OpenNamedSession("client-work.yaml", store.FolderID(a))
			if err != nil {
				t.Fatal(err)
			}
			return l.Release
		}},
		{"other folder", "another charts folder", func(t *testing.T, a, b string) func() {
			savedSession(t, "Client Work", b)
			return func() {}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, b := flagSandbox(t)
			defer c.setup(t, a, b)()
			cfg, _, refused, err := prepareLaunch(&options{session: "Client Work"})
			if refused != nil || err != nil {
				t.Fatalf("refused %v err %v", refused, err)
			}
			defer store.ReleaseInstanceLock()
			if cfg.NamedSession != nil {
				t.Errorf("the launch runs %q", cfg.NamedSession.File())
			}
			if cfg.SecondWindow {
				t.Error("premise: this launch holds the general lock")
			}
			if !strings.Contains(cfg.SessionNote, "Client Work") || !strings.Contains(cfg.SessionNote, c.want) {
				t.Errorf("Log line = %q, want it to name the session and say %q", cfg.SessionNote, c.want)
			}
		})
	}
}

// A start without --session reopens the session this charts folder's window
// last switched to; --session by hand still wins.
func TestStartReopensLastSession(t *testing.T) {
	t.Run("reopened", func(t *testing.T) {
		a, _ := flagSandbox(t)
		savedSession(t, "Client Work", a)
		if err := store.RecordLastSession(a, "client-work.yaml"); err != nil {
			t.Fatal(err)
		}
		cfg, _, refused, err := prepareLaunch(&options{})
		if refused != nil || err != nil {
			t.Fatalf("refused %v err %v", refused, err)
		}
		defer cfg.NamedSession.Release()
		if cfg.NamedSession.File() != "client-work.yaml" || cfg.NamedSessionName != "Client Work" || cfg.SessionNote != "" {
			t.Errorf("cfg runs %q named %q, note %q", cfg.NamedSession.File(), cfg.NamedSessionName, cfg.SessionNote)
		}
		if _, lerr := store.AcquireInstanceLock(); lerr != nil {
			t.Errorf("the general session's lock is still held: %v", lerr)
		}
		store.ReleaseInstanceLock()
	})
	t.Run("flag wins", func(t *testing.T) {
		a, _ := flagSandbox(t)
		savedSession(t, "Client Work", a)
		savedSession(t, "Other", a)
		if err := store.RecordLastSession(a, "client-work.yaml"); err != nil {
			t.Fatal(err)
		}
		cfg, _, _, _ := prepareLaunch(&options{session: "Other"})
		defer cfg.NamedSession.Release()
		if cfg.NamedSession.File() != "other.yaml" {
			t.Errorf("--session Other ran %q", cfg.NamedSession.File())
		}
	})
	t.Run("no record", func(t *testing.T) {
		a, _ := flagSandbox(t)
		savedSession(t, "Client Work", a)
		cfg, _, _, _ := prepareLaunch(&options{})
		defer store.ReleaseInstanceLock()
		if cfg.NamedSession != nil || cfg.SessionNote != "" {
			t.Errorf("a start with no record ran %q, note %q", cfg.NamedSession.File(), cfg.SessionNote)
		}
	})
}

// A recorded session that is missing, unusable or open in another window is
// not reopened: the start runs the unnamed session, one Log line names the
// session and why, and no session file changes. Only a missing one clears the record.
func TestStartLastSessionRefused(t *testing.T) {
	for _, c := range []struct {
		name, want string
		cleared    bool
		setup      func(t *testing.T, a string) func()
	}{
		{"missing", "no longer saved", true, func(t *testing.T, a string) func() { return func() {} }},
		{"open elsewhere", "open in another window", false, func(t *testing.T, a string) func() {
			savedSession(t, "Client Work", a)
			_, _, l, err := store.OpenNamedSession("client-work.yaml", store.FolderID(a))
			if err != nil {
				t.Fatal(err)
			}
			return l.Release
		}},
		{"unreadable", "its file cannot be used", false, func(t *testing.T, a string) func() {
			savedSession(t, "Client Work", a)
			p := store.NamedSessionPath("client-work.yaml")
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, append(b, "unknown: kept\n"...), 0o600); err != nil {
				t.Fatal(err)
			}
			return func() {}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, _ := flagSandbox(t)
			defer c.setup(t, a)()
			if err := store.RecordLastSession(a, "client-work.yaml"); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(store.NamedSessionPath("client-work.yaml"))
			cfg, _, refused, err := prepareLaunch(&options{})
			if refused != nil || err != nil {
				t.Fatalf("refused %v err %v", refused, err)
			}
			defer store.ReleaseInstanceLock()
			if cfg.NamedSession != nil {
				t.Errorf("the start runs %q", cfg.NamedSession.File())
			}
			if cfg.SecondWindow {
				t.Error("premise: this start holds the general lock")
			}
			want := `the last session "Client Work" did not open, it`
			if c.name == "missing" {
				want = `the last session "client-work" did not open, it`
			}
			if !strings.HasPrefix(cfg.SessionNote, want) || !strings.Contains(cfg.SessionNote, c.want) {
				t.Errorf("Log line = %q, want %q ... %q", cfg.SessionNote, want, c.want)
			}
			if after, _ := os.ReadFile(store.NamedSessionPath("client-work.yaml")); string(after) != string(before) {
				t.Error("the session's file changed")
			}
			rec, _ := store.LastSession(a)
			if cleared := rec == ""; cleared != c.cleared {
				t.Errorf("record after the start = %q, want cleared %v", rec, c.cleared)
			}
		})
	}
}

// A recorded session whose file the system will not let Sextile read is said
// in the user's words, and its file is not touched.
func TestStartLastSessionUnreadableFile(t *testing.T) {
	a, _ := flagSandbox(t)
	savedSession(t, "Client Work", a)
	if err := store.RecordLastSession(a, "client-work.yaml"); err != nil {
		t.Fatal(err)
	}
	p := store.NamedSessionPath("client-work.yaml")
	before, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(p, 0o600)
	if _, err := os.ReadFile(p); err == nil {
		t.Skip("this user can read a mode-000 file")
	}
	cfg, _, _, _ := prepareLaunch(&options{})
	defer store.ReleaseInstanceLock()
	if cfg.NamedSession != nil {
		t.Fatalf("the start runs %q", cfg.NamedSession.File())
	}
	if cfg.SessionNote != `the last session "client-work" did not open, its file cannot be read` {
		t.Errorf("Log line = %q", cfg.SessionNote)
	}
	after, err := os.Stat(p)
	if err != nil || !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("the session's file was replaced or written (%v)", err)
	}
}

// A missing session whose record cannot be removed still starts as usual and
// says so; the large log says the record stays.
func TestStartLastSessionRecordNotCleared(t *testing.T) {
	a, _ := flagSandbox(t)
	if err := store.RecordLastSession(a, "gone.yaml"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(os.Getenv("XDG_STATE_HOME"), "sextile", "last-session")
	if err := os.Remove(filepath.Join(dir, ".lock")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	cfg, _, _, _ := prepareLaunch(&options{})
	defer store.ReleaseInstanceLock()
	if cfg.SessionNote != `the last session "gone" did not open, it is no longer saved` {
		t.Errorf("Log line = %q", cfg.SessionNote)
	}
	if !strings.Contains(cfg.LastSessionTrace, "could not be removed") {
		t.Errorf("large-log line = %q, want it to say the record stays", cfg.LastSessionTrace)
	}
}
