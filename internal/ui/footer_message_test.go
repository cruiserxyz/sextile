package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// A transient footer message (a notice, the copy receipt, a pending leader)
// shows whole however long the key bar is: the bar yields, its unpinned
// entries first and then its pinned ones, cut from the right. Before, the
// bar won and the message was dropped on a full bar, so `, n again closes 1
// chart` showed only on a wide terminal (operator, 2026-11-07: "needs to
// either overlay or modify the footer content so it has enough space").

// wideFullBar is fullBarModel (every bar entry acting) at the 132 cells the
// operator's try had, where the bar fills the row.
func wideFullBar(t *testing.T) Model {
	t.Helper()
	m := fullBarModel(t)
	m.width, m.height = 132, 40
	return m
}

func TestFooterNoticeShowsWholeOnAFullBar(t *testing.T) {
	// 132 is the operator's terminal; 66 is where even the test model's
	// shorter bar fills the row.
	for _, width := range []int{132, 66} {
		m := wideFullBar(t)
		m.width = width
		text := ", n again closes 1 chart"
		m, _ = m.showNotice(text)
		row := strings.TrimRight(ansi.Strip(lastRow(m.View())), " ")
		if !strings.Contains(row, text) {
			t.Errorf("%d wide: the last row %q lacks the whole notice %q", width, row, text)
		}
		if w := ansi.StringWidth(row); w > width {
			t.Errorf("%d wide: the last row is %d wide", width, w)
		}
	}
}

func TestFooterCopyReceiptKeepsItsSpace(t *testing.T) {
	for _, width := range []int{132, 66} {
		m := wideFullBar(t)
		m.width = width
		m.yank.active = true
		m.yank.chars = 12
		row := strings.TrimRight(ansi.Strip(lastRow(m.View())), " ")
		if !strings.Contains(row, "Copied") {
			t.Errorf("%d wide: the last row %q lost the copy receipt", width, row)
		}
	}
}

// Every transient message carries the loading colour (256-colour 39), the
// one colour Sextile uses for something that is on its way or about to
// expire, so that a message that lasts three seconds is seen (operator,
// 2026-11-07: "a definitive coloring (neon-blueish too, normalize)"). The
// error mark keeps its own colour.
func TestFooterMessageColour(t *testing.T) {
	forceColor(t)
	on := keys.RenderBar(keys.Context{Footer: noticeOnlyNow, Style: Config{Color: ColorAlways}}, 132)
	if !strings.Contains(on, "\x1b[36m") || !strings.Contains(ansi.Strip(on), noticeOnlyNow) {
		t.Errorf("coloured bar = %q", on)
	}
	flash := keys.RenderBar(keys.Context{Footer: "Copied", Style: Config{Color: ColorAlways}}, 132)
	if !strings.Contains(flash, "\x1b[36m") {
		t.Errorf("the copy receipt carries no notice colour (terminal cyan): %q", flash)
	}
	off := keys.RenderBar(keys.Context{Footer: noticeOnlyNow, Style: Config{Color: ColorNever}}, 132)
	if strings.Contains(off, "\x1b[") {
		t.Errorf("colours off, the bar still carries a sequence: %q", off)
	}
	mark := keys.RenderBar(keys.Context{Footer: keys.ErrorMarkText(",", Config{Color: ColorAlways}), Style: Config{Color: ColorAlways}}, 132)
	if strings.Contains(mark, "38;5;39") {
		t.Errorf("the error mark took the loading colour: %q", mark)
	}
}
