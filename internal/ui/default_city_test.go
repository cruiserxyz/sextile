package ui

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// fileState is everything about a file a stray write could change.
type fileState struct {
	data  []byte
	mtime time.Time
	mode  os.FileMode
}

// profileSnapshot records every file under the charts directory and the
// session directory, and the preferences file, by path. A path that does not
// exist is recorded as absent, so a file created anywhere in them fails a
// comparison, and so does one rewritten with the same bytes (its mtime moves).
func profileSnapshot(t *testing.T) map[string]fileState {
	t.Helper()
	out := map[string]fileState{}
	add := func(path string) {
		info, err := os.Lstat(path)
		if err != nil {
			return
		}
		st := fileState{mtime: info.ModTime(), mode: info.Mode()}
		if info.Mode().IsRegular() {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("snapshot: %v", err)
			}
			st.data = b
		}
		out[path] = st
	}
	for _, root := range []string{os.Getenv("SEXTILE_CHARTS_DIR"), store.SessionDir(), filepath.Dir(store.PreferencesPath())} {
		if root == "" {
			t.Fatal("snapshot: a profile directory is not isolated")
		}
		_ = filepath.Walk(root, func(p string, _ os.FileInfo, err error) error {
			if err == nil {
				add(p)
			}
			return nil
		})
	}
	add(store.PreferencesPath())
	return out
}

func sameProfile(t *testing.T, before, after map[string]fileState, what string) {
	t.Helper()
	for p, b := range before {
		a, ok := after[p]
		switch {
		case !ok:
			t.Errorf("%s: %s was removed", what, p)
		case !bytes.Equal(a.data, b.data) || !a.mtime.Equal(b.mtime) || a.mode != b.mode:
			t.Errorf("%s: %s changed", what, p)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			t.Errorf("%s: %s appeared", what, p)
		}
	}
}

// runAll executes a command and every command it batches or sequences, and
// returns the messages they produced. A sequence is a slice of commands of an
// unexported type, so it is expanded by shape: a runner that skipped it would
// miss exactly the writes that used to ride in one.
func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if rv := reflect.ValueOf(msg); rv.Kind() == reflect.Slice && rv.Type().Elem() == reflect.TypeOf(cmd) {
		var out []tea.Msg
		for i := 0; i < rv.Len(); i++ {
			out = append(out, runAll(rv.Index(i).Interface().(tea.Cmd))...)
		}
		return out
	}
	return []tea.Msg{msg}
}

var amsterdamNL = location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lat: 52.37, Lon: 4.89, TZ: "Europe/Amsterdam"}

// hostZone makes the host's time zone the named one for the test.
func hostZone(t *testing.T, name string) {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = old })
}

// startFromDisk is a start on a host in Amsterdam: the perimeter is read from
// the sandboxed profile exactly as the program reads it, and the atlas holds
// the given places.
func startFromDisk(t *testing.T, places ...location.PlaceInfo) (Model, tea.Cmd) {
	t.Helper()
	hostZone(t, "Europe/Amsterdam")
	msg, ok := ingest.ReadStartupPerimeter()().(ingest.StartupPerimeterReadMsg)
	if !ok {
		t.Fatal("fixture: the perimeter read returned another message")
	}
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTime()))
	m.width, m.height = 80, 24
	m.atlas.index = location.NewIndex(places)
	m.pendingPerimeter = &msg
	return maybeResolveStartup(m)
}

// guessedStart is a first start in an empty, isolated profile whose only city
// comes from the host's zone.
func guessedStart(t *testing.T) (Model, tea.Cmd) {
	t.Helper()
	sessionSandbox(t)
	return startFromDisk(t, amsterdamNL)
}

// storedStart is the same start with a Berlin on disk.
func storedStart(t *testing.T) (Model, tea.Cmd) {
	t.Helper()
	sessionSandbox(t)
	if err := store.SetLocation(testPlaceBerlin()); err != nil {
		t.Fatal(err)
	}
	return startFromDisk(t, amsterdamNL)
}

func lastLine(m Model) logLine {
	if len(m.logLines) == 0 {
		return logLine{}
	}
	return m.logLines[len(m.logLines)-1]
}

// deliver feeds every message a command produced back into the model.
func deliver(m Model, cmd tea.Cmd) Model {
	for _, msg := range runAll(cmd) {
		if _, ok := msg.(DefaultCitySavedMsg); ok {
			tm, _ := m.Update(msg)
			m = tm.(Model)
		}
	}
	return m
}

// The floating row shows the proposed city and carries no mark about it, at
// both screen sizes.
func TestGuessedCityRowHasNoMark(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {132, 40}} {
		m, _ := guessedStart(t)
		tm, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = tm.(Model)
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "Amsterdam") {
			t.Fatalf("%dx%d: no row names the city:\n%s", size[0], size[1], view)
		}
		if strings.Contains(view, "(guessed)") {
			t.Errorf("%dx%d: the Chart row carries a mark:\n%s", size[0], size[1], view)
		}
	}
}

// Acceptance 6: `,c d` writes the current city as the `location:` block, leaves
// the other blocks as they were, says so in the Log, and the city is no longer
// a guess. Both leader maps carry it.
func TestCommaCDSavesTheCity(t *testing.T) {
	for _, owner := range []string{"main", "details"} {
		t.Run(owner, func(t *testing.T) {
			var m Model
			if owner == "main" {
				m, _ = guessedStart(t)
			} else {
				stubTextCompute(t)
				sessionSandbox(t)
				m = onTimeline(t, Config{Accessible: true})
				m.place, m.placeGuessed = amsterdamNL, true
			}
			clock := "clock:\n  mode: held\n"
			path := store.PreferencesPath()
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(clock), 0o644); err != nil {
				t.Fatal(err)
			}
			charts := profileSnapshot(t)

			m, cmd := press(t, m, ",", "c", "d")
			if cmd == nil {
				t.Fatal("`,c d` scheduled no save")
			}
			m = deliver(m, cmd)

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(got), clock) {
				t.Errorf("the clock block did not survive byte-for-byte: %q", got)
			}
			saved, ok := store.Location()
			if !ok || saved.Name != "Amsterdam" || saved.Country != "NL" || saved.TZ != "Europe/Amsterdam" {
				t.Errorf("stored location = %+v (%v), want Amsterdam, NL", saved, ok)
			}
			if want := "default city saved: Amsterdam, NL"; lastLine(m).text != want || lastLine(m).lane != laneNote {
				t.Errorf("last Log line = %q (lane %d), want the note %q", lastLine(m).text, lastLine(m).lane, want)
			}
			if got := ansi.Strip(m.View()); !strings.Contains(got, "Defaults saved") {
				t.Errorf("the footer does not show the save notice:\n%s", got)
			}
			after := profileSnapshot(t)
			for p, st := range charts {
				if p == path || strings.HasPrefix(p, "preferences:") || p == filepath.Dir(path) {
					continue
				}
				if a, ok := after[p]; !ok || !bytes.Equal(a.data, st.data) {
					t.Errorf("%s changed: `,c d` writes the preferences and nothing else", p)
				}
			}
		})
	}
}

// Acceptance 7: a save that cannot be made says so on the error lane in the
// words given, leaves the target as it was and keeps the mark.
func TestCommaCDFailureIsLoudAndLeavesTheFile(t *testing.T) {
	m, _ := guessedStart(t)
	path := store.PreferencesPath()
	if err := os.MkdirAll(path, 0o755); err != nil { // a directory where the file belongs
		t.Fatal(err)
	}
	before := profileSnapshot(t)

	m, cmd := press(t, m, ",", "c", "d")
	m = deliver(m, cmd)

	last := lastLine(m)
	if last.lane != laneError {
		t.Errorf("failure lane = %d, want the error lane", last.lane)
	}
	if !strings.HasPrefix(last.text, "default city not saved: ") || strings.Contains(last.text, "\n") || strings.Contains(last.text, "keeps") {
		t.Errorf("failure line = %q", last.text)
	}
	if strings.HasSuffix(last.text, ".") || strings.Contains(last.text, "Couldn't") {
		t.Errorf("the operator's wording was altered: %q", last.text)
	}
	sameProfile(t, before, profileSnapshot(t), "a failed save")
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Errorf("the target is no longer the directory it was: %v", err)
	}
}

// Acceptance 8: with no city chosen there is nothing to keep.
func TestCommaCDWithNoCity(t *testing.T) {
	sessionSandbox(t)
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTime()))
	m.width, m.height = 80, 24
	m.startup = startupReady
	before := profileSnapshot(t)

	m, cmd := press(t, m, ",", "c", "d")
	if cmd != nil {
		t.Error("`,c d` with no city scheduled a command")
	}
	if want := "default city not saved: no city chosen yet"; lastLine(m).text != want || lastLine(m).lane != laneError {
		t.Errorf("last Log line = %q (lane %d), want the error line %q", lastLine(m).text, lastLine(m).lane, want)
	}
	sameProfile(t, before, profileSnapshot(t), "`,c d` with no city")
}

// A late success names the city that was written, not the one on screen now.
func TestDefaultCitySavedNamesTheSavedCity(t *testing.T) {
	m, _ := guessedStart(t)
	m.place = testPlaceBerlin()
	m, _ = applyDefaultCitySaved(m, DefaultCitySavedMsg{Place: amsterdamNL})
	if want := "default city saved: Amsterdam, NL"; lastLine(m).text != want {
		t.Errorf("line = %q, want %q", lastLine(m).text, want)
	}
	m, _ = applyDefaultCitySaved(m, DefaultCitySavedMsg{Place: amsterdamNL, Err: errors.New("disk full\nsecond line")})
	if want := "default city not saved: disk full"; lastLine(m).text != want {
		t.Errorf("line = %q, want %q (first line of the error)", lastLine(m).text, want)
	}
}

// Acceptance 11: store.SetLocation has one caller outside the store, in any
// production package, and it is the command `,c d` runs. A second caller is a
// second way to change the stored city without the operator choosing it.
func TestSetLocationHasOneCaller(t *testing.T) {
	fset, files := productionSourcesOutsideStore(t)
	var in []string
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "SetLocation" {
					in = append(in, fn.Name.Name+" ("+filepath.Base(fset.Position(call.Pos()).Filename)+")")
				}
				return true
			})
		}
	}
	// Anything that names the function without calling it is a call in waiting.
	named := 0
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "SetLocation" {
				named++
			}
			return true
		})
	}
	if want := []string{"saveDefaultCityCmd (startup.go)"}; !reflect.DeepEqual(in, want) || named != 1 || len(files) < 50 {
		t.Errorf("store.SetLocation is called from %v and named %d times in %d files, want only %v", in, named, len(files), want)
	}
}

// productionSourcesOutsideStore parses every non-test Go file under cmd/ and
// internal/ except internal/store, the one package allowed to write
// preferences itself.
func productionSourcesOutsideStore(t *testing.T) (*token.FileSet, []*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	root := filepath.Join("..", "..")
	var files []*ast.File
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if p == filepath.Join(root, "internal", "store") || d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(fset, p, nil, 0)
			if err != nil {
				return err
			}
			files = append(files, f)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return fset, files
}
