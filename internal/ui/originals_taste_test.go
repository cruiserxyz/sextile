// All chart values reachable from this file are constructed synthetic test inputs.
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
)

// The frames of the charts folder's door as the operator picked them
// (specs/originals-door.md, the taste lane): the refusal, the first-write
// question, the change question, and the Log lines of `, w` and of the start.

var originalsTasteCfg = Config{Accessible: true, Color: ColorNever}

func tasteSaveState(dest, typed string) saveModalState {
	st := saveModalState{Form: popup.Form{Fields: newSaveForm()}, dest: dest}
	st.Fields.Fields[0].Value = typed
	return st
}

func tasteTitleState(file, was, placeWas, title, place string, rename bool) titleFormState {
	c := newNamedFileChart(file, was, placeWas)
	st := titleFormState{Form: popup.Form{Fields: newRenameForm(c)}, chart: c, rename: rename,
		stamp: time.Date(2026, 11, 13, 9, 18, 0, 0, time.UTC)}
	st.Fields.Fields[0].Value, st.Fields.Fields[1].Value = title, place
	return st
}

func newNamedFileChart(file, title, place string) store.NamedChart {
	return store.NamedChart{ID: strings.ToLower(file), Name: title, PlaceLabel: place, Untitled: title == "",
		Source: store.ChartSource{Kind: store.ChartKindFile, File: file}}
}

func TestOriginalsTasteFrames(t *testing.T) {
	m := newTestModel(t, originalsTasteCfg, FrozenClock(time.Date(2026, 11, 13, 9, 18, 0, 0, time.UTC)))
	sizes := [][2]int{{80, 24}, {132, 40}}
	for _, s := range sizes {
		save := tasteSaveState("/data/op/Documents/Sextile", "anna return")
		save.Refusal = m.protectedRefusal(refusalSaving, t.TempDir())
		checkGolden(t, fmt.Sprintf("originals_refusal_save_%dx%d", s[0], s[1]), renderSaveModal(save, originalsTasteCfg, s[0], s[1]))

		rename := tasteTitleState("GoodFella.as", "good Fella", "Munich, Germany", "Good Fella", "Munich, Germany", true)
		rename.Refusal = m.protectedRefusal(refusalChanges, t.TempDir())
		checkGolden(t, fmt.Sprintf("originals_refusal_rename_%dx%d", s[0], s[1]), renderTitleForm(rename, originalsTasteCfg, s[0], s[1]))

		ask := tasteSaveState("/data/op/Documents/Sextile", "anna return 2026")
		ask.Confirming, ask.Lit = true, len(firstWriteButtons)-1
		checkGolden(t, fmt.Sprintf("originals_first_write_%dx%d", s[0], s[1]), dialog.RenderChoice(firstWriteChoice(ask), originalsTasteCfg, s[0], s[1], popupChrome(originalsTasteCfg)))

		change := tasteTitleState("GoodFella.as", "good Fella", "Munich, Germany", "Good Fella", "Munich, Germany", true)
		change.Confirming, change.Lit = true, len(changeButtons)-1
		checkGolden(t, fmt.Sprintf("originals_change_%dx%d", s[0], s[1]), dialog.RenderChoice(changeChoice(change, originalsTasteCfg), originalsTasteCfg, s[0], s[1], popupChrome(originalsTasteCfg)))
	}

	long := tasteSaveState("/data/op/Documents/Astrology-Consulting/Client-Charts/2026-Autumn-Sessions/Sextile", "anna return 2026")
	long.Confirming, long.Lit = true, len(firstWriteButtons)-1
	checkGolden(t, "originals_first_write_long", dialog.RenderChoice(firstWriteChoice(long), originalsTasteCfg, 80, 24, popupChrome(originalsTasteCfg)))

	longChange := tasteTitleState("Constructed Client With A Long Name.as", "Constructed Client", "Munich, Germany",
		"Constructed Client With A Much Longer Title Than Before", "Garmisch-Partenkirchen, Germany", true)
	longChange.Confirming, longChange.Lit = true, len(changeButtons)-1
	checkGolden(t, "originals_change_long", dialog.RenderChoice(changeChoice(longChange, originalsTasteCfg), originalsTasteCfg, 80, 24, popupChrome(originalsTasteCfg)))
}

// The five lines of `, w` and the two of the start, each within the Log's 76
// cells, are frozen as the operator picked them.
func TestOriginalsLogLinesGolden(t *testing.T) {
	var lines []string
	last := func(m Model) string {
		for i := len(m.logLines) - 1; i >= 0; i-- {
			if m.logLines[i].lane == laneNote {
				return m.logLines[i].text
			}
		}
		t.Fatal("no Log line")
		return ""
	}
	twice := func(t *testing.T, p store.Position, setup func(dir string)) Model {
		m, dir := keyWorld(t, p)
		if setup != nil {
			setup(dir)
		}
		m = pressFolder(t, m)
		return m
	}
	lines = append(lines, last(twice(t, store.PositionProtected, nil)))
	lines = append(lines, last(twice(t, store.PositionAsk, nil)))
	lines = append(lines, last(twice(t, store.PositionProtected, func(string) { t.Setenv("SEXTILE_CHARTS_READONLY", "1") })))
	lines = append(lines, last(twice(t, store.PositionAllow, nil)))
	lines = append(lines, last(twice(t, "", func(dir string) {
		id := folderIdentityFor(t, dir)
		writeFile(t, store.PositionFile(), id+"=ask\n"+id+"=add\n")
	})))
	checkGolden(t, "originals_key_lines", checkedLines(t, lines))

	var start []string
	for _, names := range [][]string{{".GoodFella.4183920.sextile-tmp"}, {".a.1.sextile-tmp", ".b.2.sextile-tmp", ".c.3.sextile-tmp"}} {
		dir := originalsWorld(t)
		for _, n := range names {
			writeFile(t, filepath.Join(dir, n), "x")
		}
		m, _ := applyStartupPerimeter(newTestModel(t, Config{}, RealClock{}), perimeter())
		start = append(start, last(m))
	}
	checkGolden(t, "originals_leftover_lines", checkedLines(t, start))
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// checkedLines is lines, one per row, after refusing any wider than the Log.
func checkedLines(t *testing.T, lines []string) string {
	t.Helper()
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > logLineCells {
			t.Errorf("%q is %d cells, the Log holds %d", l, w, logLineCells)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}
