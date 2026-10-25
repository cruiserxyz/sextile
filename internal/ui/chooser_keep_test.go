package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// chooserKeepFixture is a loaded timeline model with both registries read and
// the timeline tab visited, so a closing key's effect on the chart, the lens
// and the timeline searches can all be observed.
func chooserKeepFixture(t *testing.T) (Model, *[]timelineCall) {
	t.Helper()
	calls := bandAwareTimeline(t)
	stubTextCompute(t)
	m := loadedTimeline(t, Config{Accessible: true})
	m.aspectRegistry = testRegistry()
	m.objectRegistry = testObjectRegistry()
	m, _ = tpress(t, m, runes("0"))
	return m, calls
}

// tickAspect opens the Aspects chooser, ticks the first row and lets the
// preview land; it stops before any closing key.
func tickAspect(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyCtrlA})
	if !m.modalOpen() {
		t.Fatal("ctrl+a did not open the aspect chooser")
	}
	var cmd tea.Cmd
	m, cmd = tpress(t, m, tea.KeyMsg{Type: tea.KeySpace})
	return pump(t, m, cmd)
}

func closeWith(t *testing.T, m Model, k tea.KeyMsg) Model {
	t.Helper()
	m, cmd := tpress(t, m, k)
	m = pump(t, m, cmd)
	if m.modalOpen() {
		t.Fatalf("%v left the chooser open", k.Type)
	}
	return m
}

// Acceptance 1: Space on an aspect row then Esc closes the popup, leaves the
// chart as Space then Enter leaves it, and keeps the ticked policy.
func TestAspectsEscKeepsTheTick(t *testing.T) {
	m, _ := chooserKeepFixture(t)
	entry := m.lens.Aspects
	ticked := tickAspect(t, m)
	if sameSelection(ticked.lens.Aspects, entry) {
		t.Fatal("fixture: the tick did not change the policy")
	}
	byEsc := closeWith(t, ticked, keyEsc)
	byEnter := closeWith(t, ticked, keyEnter)
	if !sameSelection(byEsc.lens.Aspects, ticked.lens.Aspects) {
		t.Errorf("Esc left %v in force, want the ticked %v", byEsc.lens.Aspects.Selected(), ticked.lens.Aspects.Selected())
	}
	if !reflect.DeepEqual(byEsc.chart, byEnter.chart) {
		t.Errorf("the chart after Esc differs from the chart after Enter:\n%+v\n%+v", byEsc.chart, byEnter.chart)
	}
}

// Acceptance 2: the same for a Bodies row.
func TestBodiesEscKeepsTheTick(t *testing.T) {
	m, _ := chooserKeepFixture(t)
	entry := m.lens.Objects
	ticked := untickMoonInBodies(t, m)
	if sameObjectSelection(ticked.lens.Objects, entry) {
		t.Fatal("fixture: the tick did not change the policy")
	}
	byEsc := closeWith(t, ticked, keyEsc)
	byEnter := closeWith(t, ticked, keyEnter)
	if !sameObjectSelection(byEsc.lens.Objects, ticked.lens.Objects) {
		t.Errorf("Esc left %v in force, want the ticked %v", byEsc.lens.Objects.Selected(), ticked.lens.Objects.Selected())
	}
	if !reflect.DeepEqual(byEsc.chart, byEnter.chart) {
		t.Errorf("the chart after Esc differs from the chart after Enter:\n%+v\n%+v", byEsc.chart, byEnter.chart)
	}
}

// Acceptance 3: after Esc the Details timeline recomputes with the kept
// selection exactly as after Enter: one search set, the same request.
func TestEscRefreshesTheTimelineLikeEnter(t *testing.T) {
	for _, c := range []struct {
		name string
		tick func(*testing.T, Model) Model
	}{
		{"aspects", tickAspect},
		{"bodies", untickMoonInBodies},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, calls := chooserKeepFixture(t)
			ticked := c.tick(t, m)
			spawned := len(*calls)
			closeWith(t, ticked, keyEnter)
			enterCalls := append([]timelineCall(nil), (*calls)[spawned:]...)
			if len(enterCalls) != 1 {
				t.Fatalf("fixture: Enter ran the timeline %d times, want 1", len(enterCalls))
			}
			spawned = len(*calls)
			byEsc := closeWith(t, ticked, keyEsc)
			escCalls := (*calls)[spawned:]
			if len(escCalls) != 1 {
				t.Fatalf("Esc ran the timeline %d times, want exactly once", len(escCalls))
			}
			if got, want := escCalls[0].req.Lens, enterCalls[0].req.Lens; !sameSelection(got.Aspects, want.Aspects) || !sameObjectSelection(got.Objects, want.Objects) {
				t.Errorf("Esc searched with %+v, Enter with %+v", got, want)
			}
			if byEsc.timeline.key == m.timeline.key {
				t.Error("the timeline key did not follow the kept selection")
			}
		})
	}
}

// Acceptance 4: Esc with nothing ticked closes and spawns nothing.
func TestEscWithoutATickSpawnsNothing(t *testing.T) {
	for _, open := range []tea.KeyType{tea.KeyCtrlA, tea.KeyCtrlB} {
		m, calls := chooserKeepFixture(t)
		spawned, key := len(*calls), m.timeline.key
		m, _ = tpress(t, m, tea.KeyMsg{Type: open})
		m, cmd := tpress(t, m, keyEsc)
		if m.modalOpen() {
			t.Fatalf("%v: Esc left the chooser open", open)
		}
		if cmd != nil {
			t.Errorf("%v: Esc with no tick scheduled work", open)
		}
		m = pump(t, m, cmd)
		if len(*calls) != spawned || m.timeline.key != key {
			t.Errorf("%v: Esc with no tick ran %d searches", open, len(*calls)-spawned)
		}
	}
}

// Acceptance 5: the Aspects title stays Aspects after a tick.
func TestAspectTitleIgnoresTicks(t *testing.T) {
	m := escBase(t)
	m.cfg = Config{Color: ColorNever}
	m, _ = escSend(t, m, escKey(tea.KeyCtrlA))
	m, _ = escSend(t, m, escKey(tea.KeySpace))
	title, _ := popupFrame(t, m.View(), "Aspects")
	if title != "Aspects" {
		t.Errorf("after one tick the title is %q, want Aspects", title)
	}
	if strings.Contains(ansi.Strip(m.View()), "Preview") {
		t.Error("the frame mentions Preview")
	}
}

// Acceptance 6: both choosers' bars read one Close entry for enter/esc.
func TestChooserBarsSayClose(t *testing.T) {
	for _, c := range []struct {
		name string
		key  tea.KeyType
	}{{"aspects", tea.KeyCtrlA}, {"bodies", tea.KeyCtrlB}} {
		m := escBase(t)
		m.cfg = Config{Color: ColorNever}
		m, _ = escSend(t, m, escKey(c.key))
		bar := ansi.Strip(m.View())
		want := "Move: ↑↓ | Toggle: space | Close: enter/esc"
		if !strings.Contains(bar, want) {
			t.Errorf("%s: the bar does not read %q:\n%s", c.name, want, bar)
		}
		if strings.Contains(bar, "Apply") || strings.Contains(bar, "Cancel") {
			t.Errorf("%s: the bar still says Apply or Cancel", c.name)
		}
	}
}

// oldChooserRule runs the calling test under the previous closing rule (Enter
// keeps, Esc puts back the selection the chooser opened with) and restores the
// default when the test ends.
func oldChooserRule(t *testing.T) {
	t.Helper()
	chooserCloseKeeps = false
	t.Cleanup(func() { chooserCloseKeeps = true })
}
