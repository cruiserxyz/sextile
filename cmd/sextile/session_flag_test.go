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
