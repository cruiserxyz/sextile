package store

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// O5: every writing call of os, syscall and x/sys/unix in a non-test Go file
// of the module, calls and function values alike, is in this table. The
// charts folder's are all in originals.go (and the platform move it uses).
// Every other row writes somewhere that is not a charts folder: the session,
// preferences, backups, locks, Astrolog's private temp folders, the wheel
// image, and the test environment. A new site, or one more in a listed file,
// fails here and must be placed: in the door, or in this table with the place
// it writes.
//
// A scan by call site cannot tell where a write in an allowed file lands, and
// it does not see writes through an *os.File or a helper such as io.Copy; the
// behaviour tests with the folder snapshot are the other half.
var allowedWriteSites = map[string]int{
	"internal/store/originals.go":           22, // the door
	"internal/store/noreplace_linux.go":     1,  // the door's no-replace move
	"internal/store/chartfiles.go":          1,  // mkdirAllSynced: private folders and preferences
	"internal/store/folder.go":              6,  // private folders, named sessions, migrated backups
	"internal/store/session.go":             3,  // the session file
	"internal/store/session_keep.go":        8,  // a refused session kept aside
	"internal/store/named_session.go":       6,  // named sessions and their trash
	"internal/store/instance_lock.go":       2,  // the lock file
	"internal/store/last_session.go":        1,  // the last session's record in the state folder
	"internal/store/clock.go":               1,  // the preferences file's replace
	"internal/store/store.go":               2,  // preferences
	"internal/astrolog/private_temp.go":     5,  // Sextile's private temp folder, never TMPDIR, and its base narrowed in a shared folder
	"internal/astrolog/savechart.go":        1,  // a save's private temp folder
	"internal/astrolog/retitle.go":          2,  // a rename's private temp folder and the guarded copy written into it
	"internal/astrolog/chartfile.go":        2,  // a read's private copy
	"internal/astrolog/run.go":              2,  // a run's private staging folder
	"internal/astrolog/image.go":            2,  // the wheel image's temp file
	"internal/preview/preview.go":           3,  // the wheel image
	"internal/testenv/testenv.go":           2,  // the test sandbox
	"internal/testenv/astrodir/astrodir.go": 3,  // the test sandbox
	"tools/tour/main.go":                    6,  // the tour tool, never in the program: its own fresh run folder (charts, state) and the pty device
	"tools/tour/record.go":                  1,  // the tour tool's recording file, a new file only, refused inside a charts folder
}

// writingOS are the os functions the scan counts.
var writingOS = map[string]bool{
	"WriteFile": true, "Create": true, "CreateTemp": true, "OpenFile": true, "Rename": true, "Link": true,
	"Symlink": true, "Remove": true, "RemoveAll": true, "Mkdir": true, "MkdirAll": true, "MkdirTemp": true,
	"Chmod": true, "Chtimes": true, "Truncate": true, "Lchown": true, "Chown": true,
}

// writingSys are the name prefixes counted in syscall and x/sys/unix.
var writingSys = []string{"Rename", "Link", "Symlink", "Unlink", "Mkdir", "Rmdir", "Open", "Creat", "Chmod", "Fchmod", "Truncate"}

func TestChartsFolderWritesGoThroughTheDoor(t *testing.T) {
	root := moduleRoot(t)
	got, sites := scanWriteSites(t, root)
	var bad []string
	for file, n := range got {
		want, ok := allowedWriteSites[file]
		switch {
		case !ok:
			bad = append(bad, fmt.Sprintf("%s: %d writing call(s) in a file the door does not know:\n  %s", file, n, strings.Join(sites[file], "\n  ")))
		case n != want:
			bad = append(bad, fmt.Sprintf("%s: %d writing calls, the table says %d:\n  %s", file, n, want, strings.Join(sites[file], "\n  ")))
		}
	}
	for file := range allowedWriteSites {
		if _, ok := got[file]; !ok {
			bad = append(bad, fmt.Sprintf("%s: listed, but holds no writing call", file))
		}
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Error(b)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		if filepath.Dir(dir) == dir {
			t.Fatal("no go.mod above the test")
		}
		dir = filepath.Dir(dir)
	}
}

// scanWriteSites counts, per module-relative file, the writing calls and
// function values of os, syscall and x/sys/unix in non-test Go files.
func scanWriteSites(t *testing.T, root string) (map[string]int, map[string][]string) {
	t.Helper()
	counts, sites := map[string]int{}, map[string][]string{}
	fset := token.NewFileSet()
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			if n := fi.Name(); n != "." && (strings.HasPrefix(n, ".") || n == "testdata" || n == "vendor" || n == "tmp") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			t.Errorf("parse %s: %v", p, err)
			return nil
		}
		names := map[string]string{} // local name -> import path
		for _, im := range f.Imports {
			path, _ := strconv.Unquote(im.Path.Value)
			local := filepath.Base(path)
			if im.Name != nil {
				local = im.Name.Name
			}
			names[local] = path
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if writes(names[pkg.Name], sel.Sel.Name) {
				counts[rel]++
				sites[rel] = append(sites[rel], fmt.Sprintf("%s %s.%s", fset.Position(sel.Pos()), pkg.Name, sel.Sel.Name))
			}
			return true
		})
		return nil
	})
	return counts, sites
}

func writes(importPath, name string) bool {
	switch importPath {
	case "os":
		return writingOS[name]
	case "syscall", "golang.org/x/sys/unix":
		for _, p := range writingSys {
			if strings.HasPrefix(name, p) {
				return true
			}
		}
	}
	return false
}
