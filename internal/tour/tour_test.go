package tour

import (
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

func TestParseReadsStepsDirectivesAndComments(t *testing.T) {
	s, err := Parse("# a comment\n@session Sextile Tour\n@start 2s\n<down> 800ms\n0 1.5s\n<#> 1s\n<space> 1s\n<ctrl+o> 1s\n<alt+x> 1s\n")
	if err != nil {
		t.Fatal(err)
	}
	if s.Session != "Sextile Tour" || s.Start != 2*time.Second || len(s.Steps) != 6 {
		t.Fatalf("parsed %+v", s)
	}
	if got := s.Steps[0]; got.Key.Type != tea.KeyDown || got.Rest != 800*time.Millisecond {
		t.Errorf("step 1 = %+v", got)
	}
	for i, want := range []string{"<down>", "0", "<#>", "<space>", "<ctrl+o>", "<alt+x>"} {
		if got := SpellKey(s.Steps[i].Key); got != want {
			t.Errorf("step %d spelled %q, want %q", i+1, got, want)
		}
	}
	if s.Total() != 2*time.Second+800*time.Millisecond+1500*time.Millisecond+4*time.Second {
		t.Errorf("total = %s", s.Total())
	}
}

func TestParseRefusesWhatItCannotReadAndNamesTheLine(t *testing.T) {
	for text, want := range map[string]string{
		"<down>\n":             "line 1: want a key and a duration",
		"<down> soon\n":        "line 1: bad duration",
		"<down> -1s\n":         "line 1: a duration cannot be negative",
		"\n<nokey> 1s\n":       "line 2: unknown key",
		"@speed 2\n<down> 1s":  "line 1: unknown directive",
		"@session\n<down> 1s":  "line 1: @session needs a name",
		"# only a comment\n":   "no steps",
		"<down> 1s extra 2s\n": "line 1: want a key and a duration",
	} {
		_, err := Parse(text)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%q) = %v, want %q", text, err, want)
		}
	}
}

// Every key Bubble Tea names is written as the Keybindings list writes it and
// reads back as the same key.
func TestEveryNamedKeyRoundTripsThroughTheKeybindingsSpelling(t *testing.T) {
	if len(keyTypes) < 80 {
		t.Fatalf("only %d named keys found", len(keyTypes))
	}
	for name, typ := range keyTypes {
		spelled := keys.Spell(name)
		got, err := ParseKey(spelled)
		if err != nil || got.Type != typ {
			t.Errorf("%q read back as %v, %v; want type %v", spelled, got, err, typ)
		}
		if typ != tea.KeySpace {
			if back := SpellKey(tea.KeyMsg{Type: typ}); back != spelled {
				t.Errorf("type %v spells %q, want %q", typ, back, spelled)
			}
		}
	}
	for _, r := range "0/u# <>aZ" {
		k := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		if r == ' ' {
			k = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		}
		got, err := ParseKey(SpellKey(k))
		if err != nil || got.String() != k.String() {
			t.Errorf("rune %q: %q read back as %v, %v", r, SpellKey(k), got, err)
		}
	}
}

func TestTextReadsBackAsTheSameScript(t *testing.T) {
	in := Script{Session: "S", Start: 1500 * time.Millisecond, Steps: []Step{
		{Key: tea.KeyMsg{Type: tea.KeyEnter}, Rest: time.Second},
		{Key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'#'}}, Rest: 250 * time.Millisecond},
		{Key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}, Alt: true}, Rest: 2 * time.Second},
	}}
	out, err := Parse(in.Text())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Errorf("read back %+v from\n%s", out, in.Text())
	}
}

// The recorder reads terminal bytes with Bubble Tea's own parser, so a key it
// writes is the key the program saw.
func TestDecodeReadsTheBytesAProgramWouldRead(t *testing.T) {
	pr, pw := io.Pipe()
	out := make(chan Press, 16)
	stop := Decode(pr, out)
	defer stop()
	go func() {
		for _, chunk := range []string{"\x1b[B", "0", "\r", "/", "\x1b", "\x0f", "ura"} {
			_, _ = pw.Write([]byte(chunk))
			time.Sleep(150 * time.Millisecond)
		}
		_ = pw.Close()
	}()
	var got []string
	timeout := time.After(10 * time.Second)
	for len(got) < 9 {
		select {
		case p := <-out:
			got = append(got, SpellKey(p.Key))
		case <-timeout:
			t.Fatalf("read %v", got)
		}
	}
	want := []string{"<down>", "0", "<enter>", "/", "<esc>", "<ctrl+o>", "u", "r", "a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decoded %v, want %v", got, want)
	}
}

func TestChartIsAPublicExampleWithItsSource(t *testing.T) {
	text := string(ChartText())
	for _, want := range []string{"; Source:", "v0.1.0", `-zi "Sextile 0.1.0" "Amsterdam"`, "Oct 25 2026 10:21:54 ST 1E 4:54E 52:22N"} {
		if !strings.Contains(text, want) {
			t.Errorf("the chart file lacks %q", want)
		}
	}
}

// A key the player writes is the key the recorder reads back, through the same
// parser the program runs.
func TestKeyBytesDecodeAsTheSameKey(t *testing.T) {
	var cases []tea.KeyMsg
	for _, typ := range []tea.KeyType{tea.KeyUp, tea.KeyDown, tea.KeyLeft, tea.KeyRight, tea.KeyHome, tea.KeyEnd,
		tea.KeyPgUp, tea.KeyPgDown, tea.KeyDelete, tea.KeyShiftTab, tea.KeyEnter, tea.KeyTab, tea.KeyEsc,
		tea.KeyBackspace, tea.KeyCtrlO, tea.KeyCtrlL, tea.KeyCtrlC} {
		cases = append(cases, tea.KeyMsg{Type: typ})
	}
	cases = append(cases, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}}, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'é'}}, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}, Alt: true})
	pr, pw := io.Pipe()
	out := make(chan Press, 64)
	stop := Decode(pr, out)
	defer stop()
	for _, k := range cases {
		b, err := KeyBytes(k)
		if err != nil {
			t.Errorf("%s: %v", SpellKey(k), err)
			continue
		}
		_, _ = pw.Write(b)
		select {
		case p := <-out:
			if SpellKey(p.Key) != SpellKey(k) {
				t.Errorf("%s was sent as %q and read back as %s", SpellKey(k), b, SpellKey(p.Key))
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s sent as %q was not read back", SpellKey(k), b)
		}
		time.Sleep(60 * time.Millisecond)
	}
	if _, err := KeyBytes(tea.KeyMsg{Type: tea.KeyF1}); err == nil {
		t.Error("an unsupported key must be refused, not sent as nothing")
	}
}
