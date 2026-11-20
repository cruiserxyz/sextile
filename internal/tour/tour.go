// Package tour is the key file of the Sextile Tour: the plain text that says
// which keys are pressed, one step to a line, and how long the picture rests
// after each. The capture lane plays a file into the README's animation, and
// tools/tour records and plays one in a terminal.
//
// The package is test and tool support; the released program does not import it.
package tour

import (
	"bufio"
	"embed"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

//go:embed Sextile-0.1.0.as
var chartFS embed.FS

// ChartFile is the base name of the chart every tour starts with.
const ChartFile = "Sextile-0.1.0.as"

// ChartText is that chart's file: the moment of the first public release.
func ChartText() []byte {
	b, err := chartFS.ReadFile(ChartFile)
	if err != nil {
		panic(err)
	}
	return b
}

// Format describes the key file, for the tool's --help and the tour's own file.
const Format = `A key file is plain text. Each line is one of:

  # a comment            a line that starts with #
  @session NAME          the session the tour runs in (the unnamed one without it)
  @start DURATION        how long the opening picture rests before the first key (default 1s)
  KEY DURATION           press KEY, then rest DURATION on the picture it makes

KEY is written as the Keybindings list writes it: <down>, <enter>, <esc>,
<space>, <ctrl+o>, <alt+x>, or a bare character such as 0, / or u. The one
character that is written in brackets is # (<#>), so it is not read as a comment.
DURATION is Go's: 800ms, 1.5s, 2s.`

// DefaultStart is how long the opening picture rests when the file says nothing.
const DefaultStart = time.Second

// Step is one key and how long the picture rests after it.
type Step struct {
	Key  tea.KeyMsg
	Rest time.Duration
}

// Script is a parsed key file.
type Script struct {
	Session string
	Start   time.Duration
	Steps   []Step
}

// Total is how long the whole tour plays.
func (s Script) Total() time.Duration {
	d := s.Start
	for _, st := range s.Steps {
		d += st.Rest
	}
	return d
}

// Parse reads a key file. An error names the line.
func Parse(text string) (Script, error) {
	s := Script{Start: DefaultStart}
	sc := bufio.NewScanner(strings.NewReader(text))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "@") {
			if err := s.directive(line); err != nil {
				return Script{}, fmt.Errorf("line %d: %w", n, err)
			}
			continue
		}
		f := strings.Fields(line)
		if len(f) != 2 {
			return Script{}, fmt.Errorf("line %d: want a key and a duration, got %q", n, line)
		}
		key, err := ParseKey(f[0])
		if err != nil {
			return Script{}, fmt.Errorf("line %d: %w", n, err)
		}
		rest, err := parseRest(f[1])
		if err != nil {
			return Script{}, fmt.Errorf("line %d: %w", n, err)
		}
		s.Steps = append(s.Steps, Step{Key: key, Rest: rest})
	}
	if err := sc.Err(); err != nil {
		return Script{}, err
	}
	if len(s.Steps) == 0 {
		return Script{}, fmt.Errorf("the file has no steps")
	}
	return s, nil
}

func (s *Script) directive(line string) error {
	name, arg, _ := strings.Cut(line, " ")
	arg = strings.TrimSpace(arg)
	switch name {
	case "@session":
		if arg == "" {
			return fmt.Errorf("@session needs a name")
		}
		s.Session = arg
	case "@start":
		d, err := parseRest(arg)
		if err != nil {
			return err
		}
		s.Start = d
	default:
		return fmt.Errorf("unknown directive %q", name)
	}
	return nil
}

func parseRest(v string) (time.Duration, error) {
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("bad duration %q", v)
	}
	if d < 0 {
		return 0, fmt.Errorf("a duration cannot be negative: %q", v)
	}
	return d, nil
}

// Text is the script as a key file, which Parse reads back as the same script.
func (s Script) Text() string {
	var b strings.Builder
	if s.Session != "" {
		fmt.Fprintf(&b, "@session %s\n", s.Session)
	}
	fmt.Fprintf(&b, "@start %s\n", s.Start)
	for _, st := range s.Steps {
		fmt.Fprintf(&b, "%s %s\n", SpellKey(st.Key), st.Rest)
	}
	return b.String()
}

// keyTypes is every named key type by the name Bubble Tea gives it. The names
// are Bubble Tea's own, as the Keybindings popup spells them; nothing here is a
// second list of keys.
var keyTypes = func() map[string]tea.KeyType {
	m := map[string]tea.KeyType{}
	for t := tea.KeyType(-100); t <= 127; t++ {
		if n := t.String(); n != "" && t != tea.KeyRunes {
			m[n] = t
		}
	}
	return m
}()

// SpellKey is how the Keybindings list writes a key, the pound sign aside.
func SpellKey(k tea.KeyMsg) string {
	name := k.String()
	if k.Type == tea.KeyRunes && !k.Alt {
		name = string(k.Runes)
	}
	if name == "#" {
		return "<#>"
	}
	return keys.Spell(name)
}

// ParseKey is SpellKey read back.
func ParseKey(spelled string) (tea.KeyMsg, error) {
	if spelled == "<#>" {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'#'}}, nil
	}
	if utf8.RuneCountInString(spelled) == 1 {
		r, _ := utf8.DecodeRuneInString(spelled)
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}, nil
	}
	if len(spelled) < 3 || spelled[0] != '<' || spelled[len(spelled)-1] != '>' {
		return tea.KeyMsg{}, fmt.Errorf("unknown key %q", spelled)
	}
	name := spelled[1 : len(spelled)-1]
	alt := strings.HasPrefix(name, "alt+")
	name = strings.TrimPrefix(name, "alt+")
	if name == "space" {
		name = " "
	}
	if t, ok := keyTypes[name]; ok {
		msg := tea.KeyMsg{Type: t, Alt: alt}
		if t == tea.KeySpace {
			msg.Runes = []rune{' '}
		}
		return msg, nil
	}
	if alt && utf8.RuneCountInString(name) == 1 {
		r, _ := utf8.DecodeRuneInString(name)
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: true}, nil
	}
	return tea.KeyMsg{}, fmt.Errorf("unknown key %q", spelled)
}
