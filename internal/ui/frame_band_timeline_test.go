package ui

// the specification, build note pass 7: the Timeline the operator sees is
// searched through the pinned frame band. The request is built by the model
// from its committed lens, run by the real lookup.Timeline against the real
// binary, under a data directory whose astrolog.as tries to move everything.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
)

func copyAstrologDir(t *testing.T, extra ...string) string {
	t.Helper()
	src := astrodir.Need(t)
	dst := t.TempDir()
	ents, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if len(extra) > 0 {
		as := filepath.Join(dst, "astrolog.as")
		b, err := os.ReadFile(as)
		if err != nil {
			t.Fatal(err)
		}
		b = append(b, []byte("\n"+strings.Join(extra, "\n")+"\n")...)
		if err := os.WriteFile(as, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

func TestDisplayedTimelineUsesPinnedFrame(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	old := computeTimeline
	computeTimeline = lookup.Timeline
	t.Cleanup(func() { computeTimeline = old })

	clean, hostile := copyAstrologDir(t), copyAstrologDir(t, "=s", ":s Lahiri", "=Ys", "=sr0", ":sd", "-c Koch", "=Yp")
	// display drives the model's own path end to end: syncTimeline keys the
	// list, buildTimelineCmd is the command the refresh coordinator runs, and
	// commitTimeline is what puts rows on screen. Nothing here builds the
	// request by hand, so a UI that stopped running or showing the search
	// fails it.
	display := func(t *testing.T, dir string, m Model) (rows []string, commands []string) {
		t.Helper()
		t.Setenv("ASTROLOG", dir)
		t.Setenv("ASTR800", os.Getenv("ASTR800"))
		t.Setenv("ASTR", os.Getenv("ASTR"))
		os.Unsetenv("ASTR800")
		os.Unsetenv("ASTR")
		t.Chdir(t.TempDir())
		m, _ = syncTimelineVisited(m)
		cmd := buildTimelineCmd(m, context.Background(), 1, 1)
		if cmd == nil {
			t.Fatal("the model started no timeline search")
		}
		msg, ok := cmd().(refreshResultMsg)
		if !ok || msg.err != nil {
			t.Fatalf("timeline search: %#v", msg.err)
		}
		m, _ = m.commitTimeline(msg)
		for _, row := range m.timeline.rows {
			rows = append(rows, row.Line)
		}
		p, _ := msg.payload.(timelinePayload)
		return rows, p.commands
	}
	// The sky alone, then a selected set whose anchored charts are natals
	// (transit and return rows): the displayed list is one or the other
	// depending on the scene.
	scenes := []struct {
		name string
		m    func(t *testing.T) Model
	}{
		{"sky", func(t *testing.T) Model { return timelineModel(t, Config{Accessible: true}) }},
		{"with natal", func(t *testing.T) Model { return octoberSet(t, 0) }},
	}
	for _, sc := range scenes {
		t.Run(sc.name, func(t *testing.T) {
			cr, cc := display(t, clean, sc.m(t))
			hr, hc := display(t, hostile, sc.m(t))
			if len(cr) == 0 || len(hr) == 0 {
				t.Fatalf("a displayed search showed no rows (clean %d, hostile %d)", len(cr), len(hr))
			}
			if !slices.Equal(cr, hr) {
				t.Errorf("the hostile astrolog.as changed the displayed timeline rows")
			}
			if len(hc) == 0 {
				t.Fatal("no commands carried")
			}
			// The committed lens is whole-sign tropical: -c 14, never the zero value's -c 0.
			want := "-c 14 _s _Ys _sr0 :sz"
			for _, cmd := range hc {
				if !strings.Contains(cmd, want) {
					t.Errorf("a displayed search ran without the pinned frame %q:\n%s", want, cmd)
				}
			}
			if sc.name == "with natal" && !slices.ContainsFunc(hc, func(c string) bool { return strings.Contains(c, " -t ") }) {
				t.Errorf("the natal scene ran no transit search: %q", hc)
			}
			if !reflect.DeepEqual(cc, hc) {
				t.Errorf("commands differ between data directories")
			}
		})
	}
}
