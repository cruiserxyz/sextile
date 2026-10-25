package ui

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/templates"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The floating chart follows the clock until the operator moves it
// (specs/sextile-release/academic/live-now-and-quiet-log.md, L1). A1 and A2
// are here: the attached flag and the title it drives.

// stepKeys are the six time-step keys, forward and back: each moves the
// floating chart, so each detaches it.
var stepKeys = []string{"y", "Y", "o", "O", "w", "W", "d", "D", "h", "H", "m", "M"}

// A1: the floating slot starts attached, also after a session restore; each
// time-step key, `N` and Enter on a timeline row detach it; `n` attaches it
// again; nothing else changes it.
func TestFloatingAttachedFlag(t *testing.T) {
	t.Run("a new workspace starts attached", func(t *testing.T) {
		m := readyModelWithSlots(t, berlinP)
		if m.floatingDetached || m.floatingTitle() != "~ Now" {
			t.Fatalf("a new workspace: detached=%v title=%q, want attached `~ Now`", m.floatingDetached, m.floatingTitle())
		}
	})
	t.Run("a restored session comes back attached", func(t *testing.T) {
		snap := snapshotChart()
		m := startedModel(t, withSession(perimeter(), v1(&snap, true, false, nil, nil)))
		if m.slots == nil || firstAnchor(m.slots) == nil {
			t.Fatal("fixture: the session did not restore an anchored chart")
		}
		if m.floatingDetached || m.floatingTitle() != "~ Now" {
			t.Errorf("after a restore: detached=%v title=%q, want attached `~ Now`", m.floatingDetached, m.floatingTitle())
		}
	})
	for _, k := range stepKeys {
		t.Run("key "+k+" detaches", func(t *testing.T) {
			m := readyModelWithSlots(t, berlinP)
			m, _ = tpress(t, m, runes(k))
			if !m.floatingDetached {
				t.Errorf("after %q the floating chart is still attached", k)
			}
		})
	}
	t.Run("N detaches", func(t *testing.T) {
		m := readyModelWithSlots(t, berlinP)
		m, _ = tpress(t, m, runes("d"))
		m, _ = tpress(t, m, runes("n")) // attached again, with the stepped moment to go back to
		if m.floatingDetached {
			t.Fatal("fixture: n did not attach")
		}
		m, _ = tpress(t, m, runes("N"))
		if !m.floatingDetached {
			t.Error("after N the floating chart is still attached")
		}
	})
	t.Run("n attaches again", func(t *testing.T) {
		m := readyModelWithSlots(t, berlinP)
		m, _ = tpress(t, m, runes("d"))
		if !m.floatingDetached || m.floatingTitle() != "~" {
			t.Fatalf("fixture: after d detached=%v title=%q", m.floatingDetached, m.floatingTitle())
		}
		m, _ = tpress(t, m, runes("n"))
		if m.floatingDetached || m.floatingTitle() != "~ Now" {
			t.Errorf("after n: detached=%v title=%q, want attached `~ Now`", m.floatingDetached, m.floatingTitle())
		}
	})
	t.Run("Enter on a timeline row detaches", func(t *testing.T) {
		stubTextCompute(t)
		m := onTimeline(t, Config{Accessible: true})
		if m.floatingDetached {
			t.Fatal("fixture: the timeline fixture starts detached")
		}
		m, _ = tpress(t, m, keyEnter)
		if !m.floatingDetached {
			t.Error("after Enter on a timeline row the floating chart is still attached")
		}
	})
	t.Run("nothing else changes it", func(t *testing.T) {
		stubTextCompute(t)
		m := onTimeline(t, Config{Accessible: true})
		// The cursor between slots, the panels' digits, Details, a selection.
		keys := []tea.KeyMsg{keyDown, keyUp, keyTab, runes("1"), runes("2"), runes("3"), runes("0"), keyEnter, keyEsc, {Type: tea.KeySpace}}
		for _, k := range keys {
			m, _ = tpress(t, m, k)
			if m.floatingDetached {
				t.Fatalf("key %v detached the floating chart", k)
			}
		}
		// Detached stays detached through the same keys.
		d := readyModelWithSlots(t, berlinP)
		d, _ = tpress(t, d, runes("d"))
		for _, k := range keys {
			d, _ = tpress(t, d, k)
			if !d.floatingDetached {
				t.Fatalf("key %v re-attached the floating chart", k)
			}
		}
	})
}

// A2: the Chart row of the floating slot reads `~ Now` while attached and `~`
// while detached; the rest of the row is the row it was. Frames at 132x40 and
// 80x24, colour and accessible.
func TestFloatingTitleFollowsFlag(t *testing.T) {
	forceColor(t)
	for _, size := range []struct {
		name string
		w, h int
	}{{"132x40", 132, 40}, {"80x24", 80, 24}} {
		for _, mode := range []struct {
			name string
			cfg  Config
		}{{"color", Config{Color: ColorAlways}}, {"accessible", Config{Accessible: true}}} {
			for _, detached := range []bool{false, true} {
				state := "now"
				if detached {
					state = "moved"
				}
				t.Run(fmt.Sprintf("%s/%s/%s", size.name, mode.name, state), func(t *testing.T) {
					m := readyModelWithSlots(t, berlinP)
					m.cfg = mode.cfg
					m.width, m.height = size.w, size.h
					m.floatingDetached = detached
					view := m.View()
					row := floatingRow(t, view)
					// The title opens the name column: itself, two spaces, the
					// moment. Attached it is `~ Now`, or a shorter form when the
					// whole one would cost the row its place (operator
					// 2026-10-24, header.go floatTitleShorts); never `~`.
					wants := []string{"~ Now", "~Now", "Now"}
					if detached {
						wants = []string{"~"}
					}
					opened := ""
					for _, want := range wants {
						if strings.Contains(row, want+"  Sat Apr 25, 2026") {
							opened = want
							break
						}
					}
					if opened == "" {
						t.Errorf("the floating row does not open with one of %q then the moment: %q", wants, row)
					}
					if opened != "" && opened != "~ Now" && !detached && !strings.Contains(row, berlinP.Display()) {
						t.Errorf("the title shortened to %q but the row still lost its place: %q", opened, row)
					}
					checkGolden(t, fmt.Sprintf("floating-title-%s-%s-%s", state, size.name, mode.name), view)
				})
			}
		}
	}
}

// floatingRow is the first screen row that carries the floating slot's moment,
// ANSI stripped.
func floatingRow(t *testing.T, view string) string {
	t.Helper()
	for _, l := range strings.Split(ansi.Strip(view), "\n") {
		if strings.Contains(l, "Sat Apr 25, 2026") {
			return l
		}
	}
	t.Fatalf("no floating row in the frame:\n%s", ansi.Strip(view))
	return ""
}

// --- the minute tick (A3, A4) ---

// fakeClock is a Clock the test moves by hand: the only source of time the
// model reads, so a tick is driven by advancing it, never by waiting.
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }

func (c *fakeClock) advanceTo(t time.Time) { c.t = t }

// tickStart is the fake clock's first reading: twenty seconds into a minute.
func tickStart() time.Time { return time.Date(2026, 4, 25, 12, 0, 20, 0, time.UTC) }

// recordTimer replaces the minute timer with one that notes how long each
// timer was asked to run and passes the request to timer.
func recordTimer(t *testing.T, timer func(time.Duration, func(time.Time) tea.Msg) tea.Cmd) *[]time.Duration {
	t.Helper()
	var asked []time.Duration
	prev := minuteTimer
	minuteTimer = func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		asked = append(asked, d)
		return timer(d, fn)
	}
	t.Cleanup(func() { minuteTimer = prev })
	return &asked
}

// liveModel is a ready workspace on a fake clock, attached, with the first
// minute timer armed as startup arms it. The timer's command is returned so a
// test can tell one was started; it is never run, so nothing waits a minute.
func liveModel(t *testing.T, clock *fakeClock) (Model, tea.Cmd) {
	t.Helper()
	m := NewModelWithClock(Config{Accessible: true}, clock)
	m.startup = startupReady
	m.place = berlinP
	slots, err := store.NewSlots(clock.Now(), berlinP)
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.targetFloatingTime = slots.Floating().Time
	m.width, m.height = 144, 40
	return m.armNowTick()
}

// deliverTick delivers the tick for the timer most recently armed.
func deliverTick(m Model) (Model, tea.Cmd) {
	u, cmd := m.Update(nowTickMsg{gen: m.nowTickGen})
	return u.(Model), cmd
}

// frameFor answers the text run that is in flight, as the compute would.
func frameFor(m Model, at time.Time, commands ...string) FirstFrameReadyMsg {
	st := m.refresh[surfaceText]
	return FirstFrameReadyMsg{
		RunID: st.runningRunID, WantID: st.runningWantID, TransitTime: at,
		Positions: fullChartPositions(), Aspects: tasteOwnAspects(), Commands: commands,
	}
}

func commit(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	u, _ := m.Update(msg)
	return u.(Model)
}

// A3: with the fake clock past a minute boundary while attached there is one
// recompute, the floating time becomes the clock's, and Bodies, Aspects and the
// timeline's now row follow from the committed frame; the timer for the next
// boundary is armed.
func TestMinuteTickFollowsClock(t *testing.T) {
	asked := recordTimer(t, func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return func() tea.Msg { return nil } })
	clock := &fakeClock{t: tickStart()}
	m, arm := liveModel(t, clock)
	if arm == nil || m.nowTickGen != 1 {
		t.Fatalf("an attached chart arms no minute timer (gen %d, cmd %v)", m.nowTickGen, arm != nil)
	}
	if len(*asked) != 1 || (*asked)[0] != 40*time.Second {
		t.Fatalf("the timer was asked to run %v, want 40s to the boundary", *asked)
	}
	runs := m.refresh[surfaceText].runningRunID

	boundary := time.Date(2026, 4, 25, 12, 1, 0, 0, time.UTC)
	clock.advanceTo(boundary)
	m, cmd := deliverTick(m)
	if cmd == nil {
		t.Fatal("the tick returned no command")
	}
	if got := m.refresh[surfaceText].runningRunID; got != runs+1 {
		t.Errorf("the tick started %d text runs, want exactly 1", got-runs)
	}
	if !m.targetFloatingTime.Equal(boundary) {
		t.Errorf("target = %v, want the clock %v", m.targetFloatingTime, boundary)
	}
	if m.nowTickGen != 2 || len(*asked) != 2 || (*asked)[1] != time.Minute {
		t.Errorf("the next timer: gen %d, asked %v; want gen 2 and a full minute", m.nowTickGen, *asked)
	}

	// The committed frame moves everything that follows the floating time.
	m = commit(t, m, frameFor(m, boundary))
	if got := m.slots.Floating().Time; !got.Equal(boundary) {
		t.Errorf("the floating slot reads %v after the frame, want %v", got, boundary)
	}
	if len(m.chart.positions) == 0 || len(m.chart.aspects) == 0 {
		t.Error("Bodies and Aspects did not take the frame")
	}
	if row := floatingRow(t, m.View()); !strings.Contains(row, "14:01") {
		t.Errorf("the Chart row still reads the old minute: %q", row)
	}
	if !m.timelineNowFollows(boundary) {
		t.Error("the timeline's now row is not at the clock")
	}
}

// timelineNowFollows reports that the timeline's inputs read the moment given:
// its now row is placed from the floating slot's committed time.
func (m Model) timelineNowFollows(at time.Time) bool {
	req, _, ok := m.timelineInputs()
	return ok && req.When.Equal(at)
}

// The minute timer itself, unstubbed: armed at 11:59:59.99 it ends 10ms later
// and delivers the tick for its own number.
func TestMinuteTimerDeliversItsTick(t *testing.T) {
	asked := recordTimer(t, longTimer)
	clock := &fakeClock{t: time.Date(2026, 4, 25, 11, 59, 59, 990_000_000, time.UTC)}
	m, cmd := liveModel(t, clock)
	if cmd == nil {
		t.Fatal("no timer was armed")
	}
	if len(*asked) != 1 || (*asked)[0] != 10*time.Millisecond {
		t.Fatalf("the timer was asked to run %v, want 10ms to the boundary", *asked)
	}
	if msg, ok := cmd().(nowTickMsg); !ok || msg.gen != m.nowTickGen {
		t.Errorf("the timer delivered %#v, want a tick for timer %d", msg, m.nowTickGen)
	}
}

// A3: detached, no timer runs. A timer armed before the chart was moved fires
// into nothing, arms nothing and starts no run.
func TestNoTickWhenDetached(t *testing.T) {
	clock := &fakeClock{t: tickStart()}
	m, _ := liveModel(t, clock)
	armedGen := m.nowTickGen

	m, _ = tpress(t, m, runes("d"))
	if !m.floatingDetached {
		t.Fatal("fixture: d did not detach")
	}
	if _, cmd := m.armNowTick(); cmd != nil {
		t.Error("a detached chart arms a minute timer")
	}
	runs, target, gen := m.refresh[surfaceText].runningRunID, m.targetFloatingTime, m.nowTickGen

	clock.advanceTo(time.Date(2026, 4, 25, 12, 1, 0, 0, time.UTC))
	u, cmd := m.Update(nowTickMsg{gen: armedGen})
	m = u.(Model)
	if cmd != nil {
		t.Error("the late tick returned a command: it armed a timer or started work")
	}
	if got := m.refresh[surfaceText].runningRunID; got != runs {
		t.Errorf("the late tick started a run (%d, was %d)", got, runs)
	}
	if !m.targetFloatingTime.Equal(target) {
		t.Errorf("the late tick moved the target to %v", m.targetFloatingTime)
	}
	if m.nowTickGen != gen {
		t.Errorf("the late tick armed timer %d", m.nowTickGen)
	}
}

// A3: a tick and a held key never race. The tick's run is in flight when a key
// arrives: no second spawn starts under it, the key's want is the newest, its
// target is the one that paints at last, and the tick's own frame commits
// without a word in the Log.
func TestTickCoalescesWithKeys(t *testing.T) {
	clock := &fakeClock{t: tickStart()}
	m, _ := liveModel(t, clock)
	clock.advanceTo(time.Date(2026, 4, 25, 12, 1, 0, 0, time.UTC))
	m, _ = deliverTick(m)
	tickRun, tickWant := m.refresh[surfaceText].runningRunID, m.refresh[surfaceText].wantID
	if tickRun != 1 || tickWant != 1 {
		t.Fatalf("fixture: the tick's run %d for want %d, want 1 and 1", tickRun, tickWant)
	}

	// A key while the tick's run is in flight.
	m, _ = tpress(t, m, runes("h"))
	st := m.refresh[surfaceText]
	if st.runningRunID != tickRun {
		t.Fatalf("the key started run %d under the tick's run", st.runningRunID)
	}
	if st.wantID != tickWant+1 {
		t.Fatalf("the key's want is %d, want %d", st.wantID, tickWant+1)
	}
	keyTarget := m.targetFloatingTime
	if !keyTarget.Equal(clock.Now().Add(time.Hour)) {
		t.Fatalf("target = %v, want the clock plus the key's hour", keyTarget)
	}

	// The tick's frame lands: it paints (the leading frame) and logs nothing.
	m = commit(t, m, frameFor(m, clock.Now(), "/bin/astrolog tick-command"))
	if n := countLane(m, laneArgv); n != 0 {
		t.Errorf("the tick's frame wrote %d argv lines", n)
	}
	// The trailing debounce starts the one run for the key's want.
	m, cmd := m.onRefreshTick(refreshTickMsg{surface: surfaceText, timerID: m.refresh[surfaceText].timerID})
	if cmd == nil || m.refresh[surfaceText].runningRunID != tickRun+1 {
		t.Fatalf("the key's want started run %d, want run %d", m.refresh[surfaceText].runningRunID, tickRun+1)
	}
	m = commit(t, m, frameFor(m, keyTarget, "/bin/astrolog key-command"))
	if got := m.slots.Floating().Time; !got.Equal(keyTarget) {
		t.Errorf("the floating slot reads %v at last, want the key's %v", got, keyTarget)
	}
	if got := argvTexts(m); len(got) != 1 || got[0] != "/bin/astrolog key-command" {
		t.Errorf("argv lines = %q, want the key's command alone", got)
	}
}

// The now line renews at the very bottom: user lines keep their order, and
// there is still exactly one now line.
func TestNowLineMovesToTheBottom(t *testing.T) {
	clock := &fakeClock{t: tickStart()}
	m, _ := liveModel(t, clock)
	base := len(m.logLines)
	m = m.logCommands([]string{"/bin/astrolog user-one"})
	for i, cmd := range []string{"user-two", "user-three"} {
		clock.advanceTo(time.Date(2026, 4, 25, 12, i+1, 0, 0, time.UTC))
		m, _ = deliverTick(m)
		m = commit(t, m, frameFor(m, clock.Now(), "/bin/astrolog setup", fmt.Sprintf("/bin/astrolog tick-%d", i+1)))
		m = m.logCommands([]string{"/bin/astrolog " + cmd})
	}
	clock.advanceTo(time.Date(2026, 4, 25, 12, 3, 0, 0, time.UTC))
	m, _ = deliverTick(m)
	m = commit(t, m, frameFor(m, clock.Now(), "/bin/astrolog setup", "/bin/astrolog tick-3"))
	if got := countLane(m, laneNow); got != 1 {
		t.Fatalf("the buffer holds %d now lines, want 1", got)
	}
	last := m.logLines[len(m.logLines)-1]
	if last.lane != laneNow || last.text != "/bin/astrolog tick-3" {
		t.Errorf("the last line is %v %q, want the latest now line", last.lane, last.text)
	}
	want := []string{"/bin/astrolog user-one", "/bin/astrolog user-two", "/bin/astrolog user-three"}
	if got := argvTexts(m); !slices.Equal(got, want) {
		t.Errorf("argv lines = %q, want %q in order", got, want)
	}
	if got := len(m.logLines); got != base+len(want)+1 {
		t.Errorf("the buffer holds %d lines, want %d", got, base+len(want)+1)
	}
}

// A4: three ticks add nothing to the Log. The user lanes hold what they held,
// the 6-row Log shows nothing new, and the console holds exactly one `now:`
// line, moved to the bottom each time.
func TestTickIsQuietInTheLog(t *testing.T) {
	clock := &fakeClock{t: tickStart()}
	m, _ := liveModel(t, clock)
	m = m.logCommands([]string{"/bin/astrolog user-command"})
	m = m.testNote("a note")
	m = m.testError("an error")
	wantPanel := strings.Join(m.logPanelLines(), "\n")
	userLines := len(m.logLines)

	for i := 1; i <= 3; i++ {
		clock.advanceTo(time.Date(2026, 4, 25, 12, i, 0, 0, time.UTC))
		m, _ = deliverTick(m)
		m = commit(t, m, frameFor(m, clock.Now(), "/bin/astrolog setup", fmt.Sprintf("/bin/astrolog tick-%d", i)))
		if got := countLane(m, laneNow); got != 1 {
			t.Fatalf("after tick %d the buffer holds %d now lines, want 1", i, got)
		}
		if got := m.logLines[len(m.logLines)-1].text; got != fmt.Sprintf("/bin/astrolog tick-%d", i) {
			t.Fatalf("after tick %d the now line holds %q", i, got)
		}
	}
	if got := len(m.logLines); got != userLines+1 {
		t.Errorf("the buffer holds %d lines after three ticks, want the %d user lines and one now line", got, userLines)
	}
	if got := argvTexts(m); len(got) != 1 || got[0] != "/bin/astrolog user-command" {
		t.Errorf("argv lines = %q, want the user's command alone", got)
	}
	if got := strings.Join(m.logPanelLines(), "\n"); got != wantPanel {
		t.Errorf("the 6-row Log changed: %q, was %q", got, wantPanel)
	}
	for _, row := range m.logPanelRows() {
		if strings.Contains(ansi.Strip(row), "now:") {
			t.Errorf("the 6-row Log shows the now line: %q", row)
		}
	}

	m, _ = press(t, m, ",", "l")
	if !m.console.open {
		t.Fatal("the console did not open")
	}
	var nows []string
	for _, l := range m.console.snapshot {
		if strings.HasPrefix(renderLaneLine(l, 0, m.cfg), "now: ") {
			nows = append(nows, l.text)
		}
	}
	if len(nows) != 1 || nows[0] != "/bin/astrolog tick-3" {
		t.Errorf("the console's now lines = %q, want the latest tick's alone", nows)
	}

	templateNotesStayOut := func(t *testing.T) {
		stubBodyFacts(t, nil)
		clock := &fakeClock{t: tickStart()}
		m := activeOnTemplate(templateTestModel(t))
		m.clock = clock
		m = runTemplate(t, m)
		if templateNotes(m) == 0 {
			t.Fatal("fixture: the operator's own template run wrote no template lines")
		}
		m, _ = m.armNowTick()
		before := templateNotes(m)
		for i := 1; i <= 3; i++ {
			clock.advanceTo(time.Date(2026, 4, 25, 12, i, 0, 0, time.UTC))
			m, _ = deliverTick(m)
			m = commit(t, m, venusFrame(m, clock.Now()))
			m = runTemplateRun(t, m)
			if got := templateNotes(m); got != before {
				t.Fatalf("after tick %d the Log holds %d template lines, want the %d from before", i, got, before)
			}
			if len(m.tpl.lines) == 0 {
				t.Fatalf("after tick %d the tab shows no lines", i)
			}
		}
		// A run the operator asks for afterwards writes them as ever.
		m, _ = tpress(t, m, runes("h"))
		m = commit(t, m, venusFrame(m, m.targetFloatingTime))
		m = runTemplateRun(t, m)
		if got := templateNotes(m); got <= before {
			t.Errorf("a user-driven run left the template lines at %d, want more than %d", got, before)
		}
	}
	t.Run("the template tab's rule-row notes", templateNotesStayOut)
	t.Run("the template tab's slot-branch notes", func(t *testing.T) {
		slotTemplate(t)
		templateNotesStayOut(t)
	})

	t.Run("the report and the wheel the tick asked for", func(t *testing.T) {
		clock := &fakeClock{t: tickStart()}
		m, _ := liveModel(t, clock)
		clock.advanceTo(time.Date(2026, 4, 25, 12, 1, 0, 0, time.UTC))
		m, _ = deliverTick(m)
		m = commit(t, m, frameFor(m, clock.Now(), "/bin/astrolog tick"))
		st := m.refresh[surfaceDetails]
		if st.runningRunID == 0 {
			t.Fatal("fixture: the tick's frame asked for no Details report")
		}
		m = commit(t, m, refreshResultMsg{
			surface: surfaceDetails, runID: st.runningRunID, wantID: st.runningWantID,
			payload: detailsPayload{title: "[0] Chart", body: "report", commands: []string{"/bin/astrolog report"}},
		})
		if got := argvTexts(m); len(got) != 0 {
			t.Errorf("the report the tick asked for wrote argv lines: %q", got)
		}
		// A report the operator asks for afterwards is logged as ever.
		m = m.requestDetailsForTest()
		st = m.refresh[surfaceDetails]
		if st.wantID == st.runningWantID && st.runningRunID == 1 && m.quiet[surfaceDetails] == st.wantID {
			t.Fatal("fixture: the operator's request registered no new want")
		}
		m = commit(t, m, refreshResultMsg{
			surface: surfaceDetails, runID: st.runningRunID, wantID: st.wantID,
			payload: detailsPayload{title: "[0] Chart", body: "report", commands: []string{"/bin/astrolog user-report"}},
		})
		if got := argvTexts(m); len(got) != 1 || got[0] != "/bin/astrolog user-report" {
			t.Errorf("argv lines = %q, want the operator's report alone", got)
		}
	})
}

// slotTemplate installs a template whose expression slots each write a
// slot-branch line when they run (the shipped one writes rule-row lines).
func slotTemplate(t *testing.T) {
	t.Helper()
	allow, err := os.ReadFile("../templates/allowlist.txt")
	if err != nil {
		t.Fatal(err)
	}
	src := "---\nformat: 2\norigin: shipped\nid: tick-fixture\ntab: Details\nsubject: {kind: body, class: inferior-planet, bodies: [venus]}\n---\n## @now\n" +
		"{term:elongation} {elongation.side == term(\"east\") ? term(\"evening-star\") : term(\"morning-star\")}\n"
	ts, err := templates.LoadFS(fstest.MapFS{"tick-fixture.md": {Data: []byte(src)}, "allowlist.txt": {Data: allow}}, factRegistry{}, shippedRules)
	if err != nil {
		t.Fatal(err)
	}
	old := shippedTemplates
	shippedTemplates = ts
	t.Cleanup(func() { shippedTemplates = old })
}

// venusFrame is frameFor with the positions the template fixture's cursor
// stands on (Sun, Moon, Venus), so the committed frame leaves the tab where it was.
func venusFrame(m Model, at time.Time) FirstFrameReadyMsg {
	f := frameFor(m, at)
	f.Positions = []aspect.Position{{Body: aspect.Sun}, {Body: aspect.Moon}, {Body: aspect.Venus}}
	return f
}

// templateNotes counts the Log's template lines: the verdicts' rule rows and
// the slot branches.
func templateNotes(m Model) int {
	n := 0
	for _, l := range m.logLines {
		if strings.HasPrefix(l.text, "template: ") {
			n++
		}
	}
	return n
}

// runTemplateRun answers the template run that is in flight, as its command
// would, and delivers the result through Update.
func runTemplateRun(t *testing.T, m Model) Model {
	t.Helper()
	st := m.refresh[surfaceTemplate]
	cmd := buildTemplateCmd(m, context.Background(), st.runningRunID, st.runningWantID)
	if cmd == nil {
		t.Fatal("the template tab has no run in flight")
	}
	return commit(t, m, cmd())
}

// requestDetailsForTest asks for the report the way a key would: a new want.
func (m Model) requestDetailsForTest() Model {
	m, _ = m.requestDetails()
	return m
}

// A4: Enter on the now line copies the FULL command, byte for byte.
func TestNowLineCopiesFullCommand(t *testing.T) {
	last, calls, restore := stubYankRecording(t, nil)
	defer restore()
	clock := &fakeClock{t: tickStart()}
	m, _ := liveModel(t, clock)
	full := "/usr/local/bin/astrolog -YQ 0 '-~g' '' '-~5i' '' -qj 2462084.299467593 -zl 13:25E 52:31N '=Yt' -k0"
	clock.advanceTo(time.Date(2026, 4, 25, 12, 1, 0, 0, time.UTC))
	m, _ = deliverTick(m)
	m = commit(t, m, frameFor(m, clock.Now(), full))

	m, _ = press(t, m, ",", "l")
	at := -1
	for i, l := range m.console.snapshot {
		if l.lane == laneNow {
			at = i
		}
	}
	if at < 0 {
		t.Fatal("the console holds no now line")
	}
	if shown := renderLaneLine(m.console.snapshot[at], 0, m.cfg); !strings.HasPrefix(shown, "now: astrolog ") || !strings.HasSuffix(shown, full[len(full)-30:]) {
		t.Errorf("the console shows the now line as %q", shown)
	}
	m.console.cursor = at
	m, _ = tpress(t, m, keyEnter)
	if *calls != 1 || *last != full {
		t.Errorf("Enter copied %q (%d calls), want the full command %q", *last, *calls, full)
	}
}

// tickAt delivers one minute tick and commits its frame with the given command.
func tickAt(t *testing.T, m Model, clock *fakeClock, minute int, cmd string) Model {
	t.Helper()
	clock.advanceTo(time.Date(2026, 4, 25, 12, minute, 0, 0, time.UTC))
	m, _ = deliverTick(m)
	return commit(t, m, frameFor(m, clock.Now(), "/bin/astrolog setup", cmd))
}

// The console always ends on the now line, however many user lines were
// written after the tick that wrote it.
func TestNowLineIsAlwaysLastInTheConsole(t *testing.T) {
	clock := &fakeClock{t: tickStart()}
	m, _ := liveModel(t, clock)
	m = tickAt(t, m, clock, 1, "/bin/astrolog tick-1")
	m = m.logCommands([]string{"/bin/astrolog user-one", "/bin/astrolog user-two"})
	m, _ = press(t, m, ",", "l")
	if !m.console.open {
		t.Fatal("the console did not open")
	}
	snap := m.console.snapshot
	last := snap[len(snap)-1]
	if last.lane != laneNow || last.text != "/bin/astrolog tick-1" {
		t.Errorf("the console's last line is %v %q, want the now line", last.lane, last.text)
	}
	if got := argvTexts(m); !slices.Equal(got, []string{"/bin/astrolog user-one", "/bin/astrolog user-two"}) {
		t.Errorf("argv lines = %q", got)
	}
}

// A tick while the console is open shows: the snapshot's last line is the
// newest command, the length is unchanged, the cursor stays.
func TestOpenConsoleShowsEachTick(t *testing.T) {
	clock := &fakeClock{t: tickStart()}
	m, _ := liveModel(t, clock)
	m = tickAt(t, m, clock, 1, "/bin/astrolog tick-1")
	m = m.logCommands([]string{"/bin/astrolog user-one"})
	m, _ = press(t, m, ",", "l")
	n, cursor := len(m.console.snapshot), m.console.cursor
	for i := 2; i <= 3; i++ {
		m = tickAt(t, m, clock, i, fmt.Sprintf("/bin/astrolog tick-%d", i))
		snap := m.console.snapshot
		if len(snap) != n {
			t.Fatalf("after tick %d the snapshot holds %d lines, want %d", i, len(snap), n)
		}
		if last := snap[len(snap)-1]; last.lane != laneNow || last.text != fmt.Sprintf("/bin/astrolog tick-%d", i) {
			t.Errorf("after tick %d the last line is %v %q", i, last.lane, last.text)
		}
		nows := 0
		for _, l := range snap {
			if l.lane == laneNow {
				nows++
			}
		}
		if nows != 1 {
			t.Errorf("after tick %d the snapshot holds %d now lines, want 1", i, nows)
		}
		if m.console.cursor != cursor {
			t.Errorf("after tick %d the cursor moved to %d, was %d", i, m.console.cursor, cursor)
		}
	}
}

// `, l` opens on the now line only while the floating chart follows the clock
// and the Chart cursor is on it.
func TestConsoleOpensOnTheNowLineWhileFollowing(t *testing.T) {
	setup := func(t *testing.T) (Model, *fakeClock) {
		clock := &fakeClock{t: tickStart()}
		m, _ := liveModel(t, clock)
		m = m.logCommands([]string{"/bin/astrolog user-one"})
		m = tickAt(t, m, clock, 1, "/bin/astrolog tick-1")
		return m, clock
	}
	open := func(t *testing.T, m Model) consoleState {
		m, _ = press(t, m, ",", "l")
		if !m.console.open {
			t.Fatal("the console did not open")
		}
		return m.console
	}
	t.Run("attached, cursor on floating", func(t *testing.T) {
		m, _ := setup(t)
		c := open(t, m)
		if c.snapshot[c.cursor].lane != laneNow {
			t.Errorf("cursor on %v %q, want the now line", c.snapshot[c.cursor].lane, c.snapshot[c.cursor].text)
		}
	})
	t.Run("detached", func(t *testing.T) {
		m, _ := setup(t)
		m.floatingDetached = true
		c := open(t, m)
		if got := c.snapshot[c.cursor]; got.lane != laneArgv || got.text != "/bin/astrolog user-one" {
			t.Errorf("cursor on %v %q, want the newest non-now line", got.lane, got.text)
		}
	})
	t.Run("cursor on another chart", func(t *testing.T) {
		m, _ := setup(t)
		if err := m.slots.AddAnchored(store.SessionChart{ID: "c", Name: "C", When: tickStart(), Place: berlinP}, tickStart(), berlinP); err != nil {
			t.Fatal(err)
		}
		m.cursorIdx = 0
		c := open(t, m)
		if got := c.snapshot[c.cursor]; got.lane != laneArgv {
			t.Errorf("cursor on %v %q, want the newest non-now line", got.lane, got.text)
		}
	})
}
