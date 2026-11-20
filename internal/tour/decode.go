package tour

import (
	"io"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Press is one key read from the bytes a terminal sent, and when it arrived.
type Press struct {
	Key tea.KeyMsg
	At  time.Time
}

// Decode reads terminal input from r with Bubble Tea's own parser, the one the
// program runs, and sends each key to out as it is read. A pasted run of text
// is sent as one press per character. The returned stop ends the reading.
func Decode(r io.Reader, out chan<- Press) (stop func()) {
	p := tea.NewProgram(decoder{out: out},
		tea.WithInput(r), tea.WithOutput(io.Discard),
		tea.WithoutRenderer(), tea.WithoutSignalHandler(),
	)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = p.Run()
	}()
	return func() {
		p.Kill()
		<-done
	}
}

type decoder struct{ out chan<- Press }

func (d decoder) Init() tea.Cmd { return nil }

func (d decoder) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return d, nil
	}
	at := time.Now()
	if k.Type == tea.KeyRunes && len(k.Runes) > 1 {
		for _, r := range k.Runes {
			d.out <- Press{Key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: k.Alt}, At: at}
		}
		return d, nil
	}
	d.out <- Press{Key: tea.KeyMsg{Type: k.Type, Runes: k.Runes, Alt: k.Alt}, At: at}
	return d, nil
}

func (d decoder) View() string { return "" }
