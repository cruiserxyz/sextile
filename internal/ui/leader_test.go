package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"reflect"
	"regexp"
	"strings"
	"testing"

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
	case "ctrl+l":
		return tea.KeyMsg{Type: tea.KeyCtrlL}
	case "ctrl+o":
		return tea.KeyMsg{Type: tea.KeyCtrlO}
	case "ctrl+x":
		return tea.KeyMsg{Type: tea.KeyCtrlX}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestKeyMsgNamesRoundTrip(t *testing.T) {
	for _, k := range []string{"esc", "enter", "up", "down", "tab", "pgup", "pgdown", "home", "end", "ctrl+pgup", "ctrl+pgdown", "ctrl+l", "ctrl+o", "ctrl+x", ",", "l", "q", "y", " ", "?"} {
		if got := keyMsg(k).String(); got != k {
			t.Errorf("keyMsg(%q).String() = %q", k, got)
		}
	}
}

// press sends one key through the whole Update path, exactly as the terminal
// would — no test reaches into a mode's handler directly, so the routing is
// under test too.
func press(t *testing.T, m Model, kmap ...string) (Model, tea.Cmd) {
	t.Helper()
	var tm tea.Model = m
	var cmd tea.Cmd
	for _, k := range kmap {
		tm, cmd = tm.Update(keyMsg(k))
	}
	return tm.(Model), cmd
}

// runCommand runs a command by its id through the model's action path, the
// way a key that reaches it does, for a command that no longer has a letter.
func runCommand(t *testing.T, m Model, id keys.BindingID) (Model, tea.Cmd) {
	t.Helper()
	tm, cmd, ok := m.runAction(id)
	if !ok {
		t.Fatalf("binding %v is not a command", id)
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
func threeDeepMap(t *testing.T) *keys.LeaderNode {
	t.Helper()
	root, err := keys.NewLeaderMap([]keys.LeaderBinding{
		{Seq: []string{"l"}, ID: keys.BindingLogConsole},
		{Seq: []string{"g", "s", "x"}, ID: keys.BindingHelp},
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
		want keys.LeaderOutcome
		id   keys.BindingID
	}{
		{nil, keys.ResolvePrefix, 0},
		{[]string{"l"}, keys.ResolveAction, keys.BindingLogConsole},
		{[]string{"g"}, keys.ResolvePrefix, 0},
		{[]string{"g", "s"}, keys.ResolvePrefix, 0},
		{[]string{"g", "s", "x"}, keys.ResolveAction, keys.BindingHelp},
		{[]string{"z"}, keys.ResolveNone, 0},
		{[]string{"l", "z"}, keys.ResolveNone, 0},
		{[]string{"g", "z"}, keys.ResolveNone, 0},
		{[]string{"g", "s", "z"}, keys.ResolveNone, 0},
	}
	for _, tc := range cases {
		got, id := root.Resolve(tc.seq)
		switch got {
		case keys.ResolveNone, keys.ResolvePrefix, keys.ResolveAction:
		default:
			t.Fatalf("resolve(%v) returned %d, which is none of the three outcomes", tc.seq, got)
		}
		if got != tc.want {
			t.Errorf("resolve(%v) = %v, want %v", tc.seq, got, tc.want)
		}
		if got == keys.ResolveAction && id != tc.id {
			t.Errorf("resolve(%v) carried id %v, want %v", tc.seq, id, tc.id)
		}
	}
}

// A nil tree resolves nothing rather than taking the process down.
func TestLeaderResolveNilMapIsInert(t *testing.T) {
	var root *keys.LeaderNode
	if got, _ := root.Resolve([]string{"l"}); got != keys.ResolveNone {
		t.Errorf("nil map resolved %v", got)
	}
}

// The shipped map must build, since mustDefaultLeaderMap falls back to an
// empty tree rather than panicking.
func TestDefaultLeaderMapBuilds(t *testing.T) {
	if _, err := keys.NewLeaderMap(defaultLeaderMap()); err != nil {
		t.Fatalf("the shipped leader map does not build: %v", err)
	}
	if got, id := mustDefaultLeaderMap().Resolve([]string{"L"}); got != keys.ResolveAction || id != keys.BindingLogConsole {
		t.Errorf("shipped map resolves L to (%v, %v)", got, id)
	}
}

// C2: a sequence that is both a binding and the start of a longer one is
// refused at construction, in either declaration order, naming both.
func TestLeaderMapRejectsPrefixAmbiguity(t *testing.T) {
	cases := map[string][]keys.LeaderBinding{
		"shorter first": {
			{Seq: []string{"g"}, ID: keys.BindingHelp},
			{Seq: []string{"g", "s"}, ID: keys.BindingLogConsole},
		},
		"longer first": {
			{Seq: []string{"g", "s"}, ID: keys.BindingLogConsole},
			{Seq: []string{"g"}, ID: keys.BindingHelp},
		},
	}
	for name, bindings := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := keys.NewLeaderMap(bindings)
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
	if cmd != nil {
		t.Error("arming the leader started a command")
	}
	if !reflect.DeepEqual(before, m.refresh) {
		t.Error("arming the leader touched a refresh surface")
	}
}

// C4: a key completing a sequence runs the action and clears pending in the
// same update.
func TestLeaderCompletesAction(t *testing.T) {
	m := leaderModel(t)
	m, _ = press(t, m, ",", "L")
	if m.leader.pending {
		t.Error("pending survived a completed sequence")
	}
	if !m.logFocus.active {
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
	m, _ = press(t, m, "s")
	if !m.leader.pending || !reflect.DeepEqual(m.leader.seq, []string{"g", "s"}) {
		t.Fatalf("after g s: pending=%v seq=%v", m.leader.pending, m.leader.seq)
	}
	m, _ = press(t, m, "x")
	if m.leader.pending {
		t.Error("pending survived the third key completing the sequence")
	}
}

// C6: a key that resolves to nothing is swallowed — a mistyped `,x` must not
// reach the time-step binding and move the chart a year.
func TestLeaderUnmappedKeySwallowed(t *testing.T) {
	m := leaderModel(t)
	wantTime := m.targetFloatingTime
	beforeRefresh := m.refresh
	beforeLog := len(m.logLines)

	m, cmd := press(t, m, ",", "d")
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

// C8: a pending leader has no time limit: whatever else arrives, it waits for
// a key or Esc.
func TestLeaderWaitsWithoutALimit(t *testing.T) {
	m := leaderModel(t)
	m, _ = press(t, m, ",")
	var tm tea.Model = m
	for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: 120, Height: 40}, yankFlashMsg{}, nowTickMsg{}} {
		tm, _ = tm.Update(msg)
	}
	if !tm.(Model).leader.pending {
		t.Error("the leader stopped pending without a key")
	}
}

// Over a modal with a filter that binds no chord the leader does nothing and never reaches the filter.
func TestLeaderOverAModalDoesNothing(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		m := leaderModel(t)
		m = openedKeybindings(m)
		m, _ = press(t, m, ",")
		if m.leader.pending || !m.keybindingsOpen {
			t.Errorf("the leader over Keybindings: pending %v, open %v, want nothing", m.leader.pending, m.keybindingsOpen)
		}
	})
	t.Run("atlas", func(t *testing.T) {
		m := leaderModel(t)
		m.startup = startupAtlasOpen
		m, _ = press(t, m, ",")
		if m.leader.pending || m.atlas.FilterText != "" {
			t.Errorf("the leader over the atlas modal: pending %v, filter %q, want nothing", m.leader.pending, m.atlas.FilterText)
		}
	})
	t.Run("charts", func(t *testing.T) {
		m := leaderModel(t)
		m.startup = startupChartOpen
		m, _ = press(t, m, ",")
		if m.leader.pending || m.picker.FilterText != "" {
			t.Errorf("the leader over the chart picker: pending %v, filter %q, want nothing", m.leader.pending, m.picker.FilterText)
		}
	})
}

// C11: the console owns the keyboard, and the leader works in it for one chord,
// `,y`, which copies every line. A sequence that completes any other action is
// swallowed like a mistyped one: it opens nothing and leaves the console open.
func TestLeaderInConsoleTakesOnlyCopy(t *testing.T) {
	copied := captureYank(t)
	m := leaderModel(t)
	m = m.testNote("a line to look at")
	m, _ = press(t, m, ",", "L", "l")
	if !m.console.open {
		t.Fatal("the console did not open")
	}
	m, _ = press(t, m, ",")
	if !m.leader.pending || !m.console.open {
		t.Fatalf("the leader did not arm inside the console: pending %v, open %v", m.leader.pending, m.console.open)
	}
	m, _ = press(t, m, "s")
	if m.leader.pending || !m.console.open || m.startup == startupSaveOpen {
		t.Errorf(",s inside the console: pending %v, open %v, startup %v", m.leader.pending, m.console.open, m.startup)
	}
	m, _ = press(t, m, ",", "L")
	if m.leader.pending || !m.console.open || m.logFocus.active {
		t.Errorf(",l inside the console: pending %v, open %v, log focus %v", m.leader.pending, m.console.open, m.logFocus.active)
	}
	if len(*copied) != 0 {
		t.Fatalf("copied before ,y: %q", *copied)
	}
	m, _ = press(t, m, ",", "y")
	if len(*copied) != 1 || !m.console.open {
		t.Errorf(",y inside the console: copied %d, open %v", len(*copied), m.console.open)
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
			if !strings.Contains(m.logLines[0].text, k) || !strings.Contains(m.logLines[0].text, "cannot start a command") {
				t.Errorf("the line does not name the key and the reason: %q", m.logLines[0].text)
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
// naming field and value, and the default stands.
func TestKeyConfigRejectionsReachTheLog(t *testing.T) {
	m := leaderModel(t)
	m = m.applyKeyConfig(store.KeysConfig{
		Rejections: []store.KeyRejection{
			{Field: "keys.timeoutlen", Value: "soon", Reason: "It must be a whole number above zero"},
			{Field: "keys.timeoutlen", Value: "-4", Reason: "It must be a whole number above zero"},
		},
	})
	if len(m.logLines) != 2 {
		t.Fatalf("log = %v, want one line per rejection", m.logLines)
	}
	for i, want := range []string{"soon", "-4"} {
		line := m.logLines[i].text
		for _, part := range []string{"keys.timeoutlen", want, " not valid, ignored"} {
			if !strings.Contains(line, part) {
				t.Errorf("line %d %q does not carry %q", i, line, part)
			}
		}
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

// C49: Keybindings names the active leader and lists each command with its Ctrl key.
func TestHelpListsLeaderMap(t *testing.T) {
	kmap := keys.DefaultKeys()
	for _, leader := range []string{",", ";"} {
		got := helpListText(kmap, leader, defaultLeaderMap())
		for _, row := range []string{"Log  ", "Choose houses  "} {
			if !strings.Contains(got, row) {
				t.Errorf("help does not carry the description %q", row)
			}
		}
		rowsFound := regexp.MustCompile(`(?m)^(Log|Choose houses) {2,}(.+)$`).FindAllStringSubmatch(got, -1)
		if len(rowsFound) < 2 {
			t.Errorf("help lists %d of the Log and Choose houses rows", len(rowsFound))
		}
		for _, row := range rowsFound {
			if want := map[string]string{"Log": leader + " L", "Choose houses": "<ctrl+u>"}[row[1]]; row[2] != want {
				t.Errorf("help lists %q with %q, want %q", row[1], row[2], want)
			}
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

// Acceptance 6: a letter after the leader that is not in the table cancels the
// menu and does nothing else. `d` is the day step and `q` the quit when no
// leader is pending; after one neither steps, writes or quits.
func TestCommaThenAnUnlistedLetterCancels(t *testing.T) {
	for _, letter := range []string{"d", "q"} {
		t.Run(letter, func(t *testing.T) {
			sessionSandbox(t)
			m := leaderModel(t)
			m.place, m.placeFromZone = amsterdamNL, true
			before := profileSnapshot(t)
			lines := len(m.logLines)
			target := m.targetFloatingTime

			m, cmd := press(t, m, ",", letter)
			if m.leader.pending || cmd != nil || len(m.logLines) != lines {
				t.Errorf("`, %s`: pending=%v cmd=%v log %d -> %d", letter, m.leader.pending, cmd != nil, lines, len(m.logLines))
			}
			if !m.targetFloatingTime.Equal(target) {
				t.Errorf("`, %s` moved the chart: %v -> %v", letter, target, m.targetFloatingTime)
			}
			if !m.placeFromZone {
				t.Errorf("`, %s` kept the city", letter)
			}
			sameProfile(t, before, profileSnapshot(t), "`, "+letter+"`")
		})
	}
}
