package ui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
)

// keyMsg turns the key name the resolver sees into the message bubbletea
// would deliver for it. TestKeyMsgNamesRoundTrip is what keeps the two in
// step: a mapper that drifted would make every test here pass against the
// wrong key.
func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "ctrl+pgup":
		return tea.KeyMsg{Type: tea.KeyCtrlPgUp}
	case "ctrl+pgdown":
		return tea.KeyMsg{Type: tea.KeyCtrlPgDown}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestKeyMsgNamesRoundTrip(t *testing.T) {
	for _, k := range []string{"esc", "enter", "up", "down", "tab", "pgup", "pgdown", "home", "end", "ctrl+pgup", "ctrl+pgdown", ",", "l", "q", "y", " ", "?"} {
		if got := keyMsg(k).String(); got != k {
			t.Errorf("keyMsg(%q).String() = %q", k, got)
		}
	}
}

// press sends one key through the whole Update path, exactly as the terminal
// would — no test reaches into a mode's handler directly, so the routing is
// under test too.
func press(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()
	var tm tea.Model = m
	var cmd tea.Cmd
	for _, k := range keys {
		tm, cmd = tm.Update(keyMsg(k))
	}
	return tm.(Model), cmd
}

// leaderModel is a ready dual-column workspace with the shipped leader map.
func leaderModel(t *testing.T) Model {
	t.Helper()
	return detailsTestModel(t)
}

// threeDeepMap is a test-only map proving the trie is a trie: `g s x` is three
// keys deep, and `g` and `g s` are prefixes of it, not bindings.
func threeDeepMap(t *testing.T) *leaderNode {
	t.Helper()
	root, err := newLeaderMap([]leaderBinding{
		{Seq: []string{"l"}, ID: BindingLogConsole},
		{Seq: []string{"g", "s", "x"}, ID: BindingHelp},
	})
	if err != nil {
		t.Fatalf("newLeaderMap: %v", err)
	}
	return root
}

// C1: resolve returns exactly one of the three outcomes, and an action
// outcome carries the id of what to run.
func TestLeaderResolveOutcomes(t *testing.T) {
	root := threeDeepMap(t)
	cases := []struct {
		seq  []string
		want leaderOutcome
		id   BindingID
	}{
		{nil, resolvePrefix, 0},
		{[]string{"l"}, resolveAction, BindingLogConsole},
		{[]string{"g"}, resolvePrefix, 0},
		{[]string{"g", "s"}, resolvePrefix, 0},
		{[]string{"g", "s", "x"}, resolveAction, BindingHelp},
		{[]string{"z"}, resolveNone, 0},
		{[]string{"l", "z"}, resolveNone, 0},
		{[]string{"g", "z"}, resolveNone, 0},
		{[]string{"g", "s", "z"}, resolveNone, 0},
	}
	for _, tc := range cases {
		got, id := root.resolve(tc.seq)
		switch got {
		case resolveNone, resolvePrefix, resolveAction:
		default:
			t.Fatalf("resolve(%v) returned %d, which is none of the three outcomes", tc.seq, got)
		}
		if got != tc.want {
			t.Errorf("resolve(%v) = %v, want %v", tc.seq, got, tc.want)
		}
		if got == resolveAction && id != tc.id {
			t.Errorf("resolve(%v) carried id %v, want %v", tc.seq, id, tc.id)
		}
	}
}

// A nil tree resolves nothing rather than taking the process down.
func TestLeaderResolveNilMapIsInert(t *testing.T) {
	var root *leaderNode
	if got, _ := root.resolve([]string{"l"}); got != resolveNone {
		t.Errorf("nil map resolved %v", got)
	}
}

// The shipped map must build, since mustDefaultLeaderMap falls back to an
// empty tree rather than panicking.
func TestDefaultLeaderMapBuilds(t *testing.T) {
	if _, err := newLeaderMap(defaultLeaderMap()); err != nil {
		t.Fatalf("the shipped leader map does not build: %v", err)
	}
	if got, id := mustDefaultLeaderMap().resolve([]string{"l"}); got != resolveAction || id != BindingLogConsole {
		t.Errorf("shipped map resolves l to (%v, %v)", got, id)
	}
}

// C2: a sequence that is both a binding and the start of a longer one is
// refused at construction, in either declaration order, naming both.
func TestLeaderMapRejectsPrefixAmbiguity(t *testing.T) {
	cases := map[string][]leaderBinding{
		"shorter first": {
			{Seq: []string{"g"}, ID: BindingHelp},
			{Seq: []string{"g", "s"}, ID: BindingLogConsole},
		},
		"longer first": {
			{Seq: []string{"g", "s"}, ID: BindingLogConsole},
			{Seq: []string{"g"}, ID: BindingHelp},
		},
	}
	for name, bindings := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := newLeaderMap(bindings)
			if err == nil {
				t.Fatal("an ambiguous map was accepted")
			}
			for _, want := range []string{`"g"`, `"g s"`} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %s", err, want)
				}
			}
		})
	}
}

// C3: the leader arms pending with nothing typed, and the keystroke is
// consumed rather than also reaching main mode.
func TestLeaderArmsPending(t *testing.T) {
	m := leaderModel(t)
	before := m.refresh
	m, cmd := press(t, m, ",")
	if !m.leader.pending {
		t.Fatal("the leader did not arm")
	}
	if len(m.leader.seq) != 0 {
		t.Errorf("pending sequence = %v, want empty", m.leader.seq)
	}
	if cmd == nil {
		t.Error("no timeout was armed with the pending leader")
	}
	if !reflect.DeepEqual(before, m.refresh) {
		t.Error("arming the leader touched a refresh surface")
	}
}

// C4: a key completing a sequence runs the action and clears pending in the
// same update.
func TestLeaderCompletesAction(t *testing.T) {
	m := leaderModel(t)
	m, _ = press(t, m, ",", "l")
	if m.leader.pending {
		t.Error("pending survived a completed sequence")
	}
	if !m.console.open {
		t.Error("the action did not run")
	}
}

// C5: a key extending the sequence keeps pending, for a map three keys deep.
func TestLeaderHoldsPrefix(t *testing.T) {
	m := leaderModel(t)
	m.leaderMap = threeDeepMap(t)
	m, _ = press(t, m, ",", "g")
	if !m.leader.pending || !reflect.DeepEqual(m.leader.seq, []string{"g"}) {
		t.Fatalf("after g: pending=%v seq=%v", m.leader.pending, m.leader.seq)
	}
	gen := m.leader.gen
	m, cmd := press(t, m, "s")
	if !m.leader.pending || !reflect.DeepEqual(m.leader.seq, []string{"g", "s"}) {
		t.Fatalf("after g s: pending=%v seq=%v", m.leader.pending, m.leader.seq)
	}
	if m.leader.gen == gen {
		t.Error("extending the sequence did not re-arm the window")
	}
	if cmd == nil {
		t.Error("no timeout was armed for the longer sequence")
	}
	m, _ = press(t, m, "x")
	if m.leader.pending {
		t.Error("pending survived the third key completing the sequence")
	}
}

// C6: a key that resolves to nothing is swallowed — a mistyped `,y` must not
// reach the time-step binding and move the chart a year.
func TestLeaderUnmappedKeySwallowed(t *testing.T) {
	m := leaderModel(t)
	wantTime := m.targetFloatingTime
	beforeRefresh := m.refresh
	beforeLog := len(m.logLines)

	m, cmd := press(t, m, ",", "y")
	if m.leader.pending {
		t.Error("pending survived an unmapped key")
	}
	if !m.targetFloatingTime.Equal(wantTime) {
		t.Errorf("the chart moved: %v -> %v", wantTime, m.targetFloatingTime)
	}
	if !reflect.DeepEqual(beforeRefresh, m.refresh) {
		t.Error("an unmapped key after the leader reached a refresh surface")
	}
	if cmd != nil {
		t.Error("an unmapped key after the leader returned a command")
	}
	if len(m.logLines) != beforeLog {
		t.Error("a swallowed key wrote a Log line")
	}
}

// C7: Esc cancels a pending leader and is swallowed.
func TestLeaderEscCancels(t *testing.T) {
	m := leaderModel(t)
	m, _ = press(t, m, ",")
	m, cmd := press(t, m, "esc")
	if m.leader.pending {
		t.Error("esc did not clear pending")
	}
	if cmd != nil {
		t.Error("esc while pending returned a command")
	}
}

// C8: a leader that waits out its window retires in silence.
func TestLeaderTimesOutSilently(t *testing.T) {
	m := leaderModel(t)
	m, _ = press(t, m, ",")
	beforeLog := len(m.logLines)
	beforeConsole := m.console

	var tm tea.Model = m
	tm, cmd := tm.Update(leaderTimeoutMsg{gen: m.leader.gen})
	m = tm.(Model)
	if m.leader.pending {
		t.Error("the timeout did not clear pending")
	}
	if len(m.logLines) != beforeLog {
		t.Error("the timeout wrote a Log line")
	}
	if !reflect.DeepEqual(m.console, beforeConsole) {
		t.Error("the timeout ran an action")
	}
	if cmd != nil {
		t.Error("the timeout returned a command")
	}
}

// C9: a tick from a pending state that is already gone changes nothing.
func TestLeaderStaleTickDropped(t *testing.T) {
	m := leaderModel(t)
	m, _ = press(t, m, ",")
	stale := m.leader.gen
	m, _ = press(t, m, "esc")
	m, _ = press(t, m, ",") // a newer pending, newer generation

	before := m
	var tm tea.Model = m
	tm, cmd := tm.Update(leaderTimeoutMsg{gen: stale})
	if cmd != nil {
		t.Error("a stale tick returned a command")
	}
	if !reflect.DeepEqual(before, tm.(Model)) {
		t.Error("a stale tick changed the Model")
	}
}

// C10: no modal ever sees a leader; the key reaches the modal's own handler.
func TestLeaderInertInModal(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		m := leaderModel(t)
		m.helpOpen = true
		m, _ = press(t, m, ",")
		if m.leader.pending {
			t.Error("the leader armed under the help modal")
		}
		if !m.helpOpen {
			t.Error("the key closed the help modal")
		}
	})
	t.Run("atlas", func(t *testing.T) {
		m := leaderModel(t)
		m.startup = startupAtlasOpen
		m, _ = press(t, m, ",")
		if m.leader.pending {
			t.Error("the leader armed under the atlas modal")
		}
		if m.atlas.filterText != "," {
			t.Errorf("the atlas filter did not receive the key, it reads %q", m.atlas.filterText)
		}
	})
	t.Run("charts", func(t *testing.T) {
		m := leaderModel(t)
		m.startup = startupChartOpen
		m, _ = press(t, m, ",")
		if m.leader.pending {
			t.Error("the leader armed under the chart picker")
		}
		if m.picker.filterText != "," {
			t.Errorf("the chart picker did not receive the key, it reads %q", m.picker.filterText)
		}
	})
}

// C11: the console owns the keyboard, so no leader arms inside it.
func TestLeaderInertInConsole(t *testing.T) {
	m := leaderModel(t)
	m = m.testNote("a line to look at")
	m, _ = press(t, m, ",", "l")
	if !m.console.open {
		t.Fatal("the console did not open")
	}
	m, _ = press(t, m, ",")
	if m.leader.pending {
		t.Error("the leader armed inside the console")
	}
	if !m.console.open {
		t.Error("the leader key closed the console")
	}
}

// C12: nothing the leader does starts a chart compute.
func TestLeaderStartsNoCompute(t *testing.T) {
	m := leaderModel(t)
	before := m.refresh

	m, _ = press(t, m, ",")
	if !reflect.DeepEqual(before, m.refresh) {
		t.Error("arming touched a refresh surface")
	}
	m.leaderMap = threeDeepMap(t)
	m, _ = press(t, m, "esc")
	m, _ = press(t, m, ",", "g")
	if !reflect.DeepEqual(before, m.refresh) {
		t.Error("holding a prefix touched a refresh surface")
	}
	m, _ = press(t, m, "esc")
	if !reflect.DeepEqual(before, m.refresh) {
		t.Error("cancelling touched a refresh surface")
	}
	m, _ = press(t, m, ",")
	var tm tea.Model = m
	tm, _ = tm.Update(leaderTimeoutMsg{gen: m.leader.gen})
	if !reflect.DeepEqual(before, tm.(Model).refresh) {
		t.Error("timing out touched a refresh surface")
	}
}

// C18: a valid configured leader is applied without a word.
func TestValidLeaderIsApplied(t *testing.T) {
	m := leaderModel(t)
	m = m.applyKeyConfig(store.KeysConfig{Leader: ";", LeaderPresent: true})
	if m.leaderKey != ";" {
		t.Errorf("leader = %q, want %q", m.leaderKey, ";")
	}
	if len(m.logLines) != 0 {
		t.Errorf("a valid leader wrote %v", m.logLines)
	}
	m, _ = press(t, m, ";")
	if !m.leader.pending {
		t.Error("the configured leader does not arm")
	}
}

// C19: a valid timeoutlen is the window a pending leader waits.
func TestValidTimeoutlenIsApplied(t *testing.T) {
	m := leaderModel(t)
	m = m.applyKeyConfig(store.KeysConfig{TimeoutlenMS: 250, TimeoutlenPresent: true})
	if m.leaderTimeout != 250*time.Millisecond {
		t.Errorf("timeout = %v, want 250ms", m.leaderTimeout)
	}
	if len(m.logLines) != 0 {
		t.Errorf("a valid timeoutlen wrote %v", m.logLines)
	}
}

// C20: an absent leader defaults to `,` in silence.
func TestLeaderDefaultsSilently(t *testing.T) {
	m := leaderModel(t)
	m = m.applyKeyConfig(store.KeysConfig{})
	if m.leaderKey != "," {
		t.Errorf("leader = %q, want %q", m.leaderKey, ",")
	}
	if len(m.logLines) != 0 {
		t.Errorf("an absent leader wrote %v", m.logLines)
	}
}

// C21: an absent timeoutlen defaults to 1000ms in silence.
func TestTimeoutlenDefaultsSilently(t *testing.T) {
	m := leaderModel(t)
	m = m.applyKeyConfig(store.KeysConfig{})
	if m.leaderTimeout != 1000*time.Millisecond {
		t.Errorf("timeout = %v, want 1000ms", m.leaderTimeout)
	}
	if len(m.logLines) != 0 {
		t.Errorf("an absent timeoutlen wrote %v", m.logLines)
	}
}

// C22: a leader that would strand the operator is refused with one line, and
// the default stands.
func TestLeaderRejectsStrandingKey(t *testing.T) {
	for _, k := range []string{"q", "esc", "enter", "tab"} {
		t.Run(k, func(t *testing.T) {
			m := leaderModel(t)
			m = m.applyKeyConfig(store.KeysConfig{Leader: k, LeaderPresent: true})
			if m.leaderKey != "," {
				t.Errorf("leader = %q, want the default to stand", m.leaderKey)
			}
			if len(m.logLines) != 1 {
				t.Fatalf("log = %v, want exactly one line", m.logLines)
			}
			if !strings.Contains(m.logLines[0].text, k) || !strings.Contains(m.logLines[0].text, "keys.leader") {
				t.Errorf("the line does not name field and value: %q", m.logLines[0].text)
			}
		})
	}
}

// C23: a leader that is not one rune is refused with one line.
func TestLeaderRejectsMultiRune(t *testing.T) {
	for _, k := range []string{"gg", "ctrl+g", "space", "…,"} {
		t.Run(k, func(t *testing.T) {
			m := leaderModel(t)
			m = m.applyKeyConfig(store.KeysConfig{Leader: k, LeaderPresent: true})
			if m.leaderKey != "," {
				t.Errorf("leader = %q, want the default to stand", m.leaderKey)
			}
			if len(m.logLines) != 1 {
				t.Fatalf("log = %v, want exactly one line", m.logLines)
			}
			if !strings.Contains(m.logLines[0].text, k) {
				t.Errorf("the line does not name the value: %q", m.logLines[0].text)
			}
		})
	}
	t.Run("one multi-byte rune is still one rune", func(t *testing.T) {
		m := leaderModel(t)
		m = m.applyKeyConfig(store.KeysConfig{Leader: "ö", LeaderPresent: true})
		if m.leaderKey != "ö" {
			t.Errorf("leader = %q, want ö", m.leaderKey)
		}
		if len(m.logLines) != 0 {
			t.Errorf("a one-rune leader wrote %v", m.logLines)
		}
	})
}

// C24: every rejection the reader hands over becomes exactly one Log line
// naming field, value and reason, and the default stands.
func TestKeyConfigRejectionsReachTheLog(t *testing.T) {
	m := leaderModel(t)
	m = m.applyKeyConfig(store.KeysConfig{
		Rejections: []store.KeyRejection{
			{Field: "keys.timeoutlen", Value: "soon", Reason: "expected a whole number of milliseconds above zero"},
			{Field: "keys.timeoutlen", Value: "-4", Reason: "expected a whole number of milliseconds above zero"},
		},
	})
	if len(m.logLines) != 2 {
		t.Fatalf("log = %v, want one line per rejection", m.logLines)
	}
	for i, want := range []string{"soon", "-4"} {
		line := m.logLines[i].text
		for _, part := range []string{"keys.timeoutlen", want, "milliseconds"} {
			if !strings.Contains(line, part) {
				t.Errorf("line %d %q does not carry %q", i, line, part)
			}
		}
	}
	if m.leaderTimeout != 1000*time.Millisecond {
		t.Errorf("timeout = %v, want the default to stand", m.leaderTimeout)
	}
}

// C25: a valid leader that collides with a main-mode binding wins, that
// binding stops answering to its own key, and one line says what was lost.
func TestLeaderShadowsBinding(t *testing.T) {
	m := leaderModel(t)
	wantTime := m.targetFloatingTime
	m = m.applyKeyConfig(store.KeysConfig{Leader: "y", LeaderPresent: true})
	if m.leaderKey != "y" {
		t.Fatalf("leader = %q, want y to win", m.leaderKey)
	}
	if len(m.logLines) != 1 {
		t.Fatalf("log = %v, want exactly one line", m.logLines)
	}
	if !strings.Contains(m.logLines[0].text, "Step a year") {
		t.Errorf("the line does not name what was shadowed: %q", m.logLines[0].text)
	}
	m, _ = press(t, m, "y")
	if !m.leader.pending {
		t.Error("the shadowing leader did not arm")
	}
	if !m.targetFloatingTime.Equal(wantTime) {
		t.Error("the shadowed binding still acted on its own key")
	}
}

// C49: the help modal names the active leader and lists every leader binding.
func TestHelpListsLeaderMap(t *testing.T) {
	keys := defaultKeys()
	for _, leader := range []string{",", ";"} {
		got := helpListText(keys, leader, defaultLeaderMap())
		if !strings.Contains(got, "leader: "+leader) {
			t.Errorf("help does not name the active leader %q", leader)
		}
		if !strings.Contains(got, leader+"l") {
			t.Errorf("help does not list %ql", leader)
		}
		if !strings.Contains(got, "Log console") {
			t.Error("help does not carry the binding's description")
		}
		if !strings.Contains(got, leader+"c d") || !strings.Contains(got, "Save defaults") {
			t.Errorf("help does not list %qc d with its description", leader)
		}
	}
}

// C57: a resize leaves a pending leader exactly as it was.
func TestLeaderSurvivesResize(t *testing.T) {
	m := leaderModel(t)
	m.leaderMap = threeDeepMap(t)
	m, _ = press(t, m, ",", "g")
	before := m.leader

	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if got := tm.(Model).leader; !reflect.DeepEqual(before, got) {
		t.Errorf("pending leader after resize = %+v, want %+v", got, before)
	}
}

// Acceptance 9: `c` is only a prefix. Both shipped maps build with `c d` in
// them; `,c` alone does nothing when its window runs out, and `,c x` is a
// mistyped sequence that is swallowed. Neither writes a Log line, starts a
// command or touches the profile.
func TestCommaCIsOnlyAPrefix(t *testing.T) {
	for name, tree := range map[string]func() (*leaderNode, error){
		"main":    func() (*leaderNode, error) { return newLeaderMap(defaultLeaderMap()) },
		"details": func() (*leaderNode, error) { return newLeaderMap(detailsLeaderMap()) },
	} {
		root, err := tree()
		if err != nil {
			t.Fatalf("%s: the shipped leader map does not build: %v", name, err)
		}
		if got, _ := root.resolve([]string{"c"}); got != resolvePrefix {
			t.Errorf("%s: `c` resolves to %v, want only a prefix", name, got)
		}
		if got, id := root.resolve([]string{"c", "d"}); got != resolveAction || id != BindingSaveCity {
			t.Errorf("%s: `c d` resolves to (%v, %v)", name, got, id)
		}
	}

	sessionSandbox(t)
	m := leaderModel(t)
	m.place, m.placeGuessed = amsterdamNL, true
	before := profileSnapshot(t)
	lines := len(m.logLines)

	m, _ = press(t, m, ",", "c")
	if !m.leader.pending {
		t.Fatal("`,c` is not waiting for a second key")
	}
	tm, cmd := tea.Model(m).Update(leaderTimeoutMsg{gen: m.leader.gen})
	m = tm.(Model)
	if m.leader.pending || cmd != nil || len(m.logLines) != lines {
		t.Errorf("`,c` then timeout: pending=%v cmd=%v log %d -> %d", m.leader.pending, cmd != nil, lines, len(m.logLines))
	}
	m, cmd = press(t, m, ",", "c", "x")
	if m.leader.pending || cmd != nil || len(m.logLines) != lines {
		t.Errorf("`,c x`: pending=%v cmd=%v log %d -> %d", m.leader.pending, cmd != nil, lines, len(m.logLines))
	}
	if !m.placeGuessed {
		t.Error("a half-typed sequence kept the city")
	}
	sameProfile(t, before, profileSnapshot(t), "a half-typed `,c`")
}
