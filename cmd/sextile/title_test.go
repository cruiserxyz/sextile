package main

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// quitModel sets a title and quits at once.
type quitModel struct{}

func (quitModel) Init() tea.Cmd                         { return tea.Sequence(tea.SetWindowTitle("Sextile"), tea.Quit) }
func (m quitModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (quitModel) View() string                          { return "" }

// The terminal's own title is pushed before the program's first title and
// popped last, once each.
func TestTheTerminalTitleIsPushedBeforeTheFirstTitleAndPoppedAtTheEnd(t *testing.T) {
	var out bytes.Buffer
	runProgramOn(quitModel{}, &out, strings.NewReader(""))
	got := out.String()
	push, title, pop := strings.Index(got, titlePush), strings.Index(got, "Sextile\x07"), strings.LastIndex(got, titlePop)
	if strings.Count(got, titlePush) != 1 || strings.Count(got, titlePop) != 1 {
		t.Fatalf("pushes %d, pops %d, want one each: %q", strings.Count(got, titlePush), strings.Count(got, titlePop), got)
	}
	if push != 0 || title < 0 || !(push < title && title < pop) || !strings.HasSuffix(got, titlePop) {
		t.Errorf("order wrong (push %d, title %d, pop %d): %q", push, title, pop, got)
	}
}
