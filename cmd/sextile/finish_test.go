package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// finalModel stands in for the model tea.Program.Run returns.
type finalModel struct{ sessionErr error }

func (finalModel) Init() tea.Cmd                         { return nil }
func (m finalModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (finalModel) View() string                          { return "" }
func (m finalModel) SessionWriteError() error            { return m.sessionErr }

// the specification A7: after the screen is restored, a session that
// could not be written is one line on stderr, and the exit status is the
// program's own: 0, or 1 when the program itself failed.
func TestFinishReportsSessionFailure(t *testing.T) {
	sessionErr := errors.New("couldn't save the session to /x/session.yaml: permission denied")
	for _, c := range []struct {
		name       string
		final      tea.Model
		runErr     error
		wantStatus int
		wantLines  []string
	}{
		{"clean quit", finalModel{}, nil, 0, nil},
		{"session not written", finalModel{sessionErr}, nil, 0, []string{"sextile: " + sessionErr.Error()}},
		{"program failed too", finalModel{sessionErr}, errors.New("boom"), 1, []string{"sextile: boom", "sextile: " + sessionErr.Error()}},
		{"program failed, no model", nil, errors.New("boom"), 1, []string{"sextile: boom"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			if got := finish(c.final, c.runErr, &out); got != c.wantStatus {
				t.Errorf("status = %d, want %d", got, c.wantStatus)
			}
			var lines []string
			if s := strings.TrimRight(out.String(), "\n"); s != "" {
				lines = strings.Split(s, "\n")
			}
			if strings.Join(lines, "|") != strings.Join(c.wantLines, "|") {
				t.Errorf("stderr = %q, want %q", lines, c.wantLines)
			}
		})
	}
	t.Run("stderr closed", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		r.Close()
		w.Close()
		if got := finish(finalModel{sessionErr}, nil, w); got != 0 {
			t.Errorf("status = %d with a closed stderr, want 0", got)
		}
	})
}
