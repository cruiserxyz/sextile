package tour

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// csiKeys are the keys a terminal sends as an escape sequence.
var csiKeys = map[tea.KeyType]string{
	tea.KeyUp: "\x1b[A", tea.KeyDown: "\x1b[B", tea.KeyRight: "\x1b[C", tea.KeyLeft: "\x1b[D",
	tea.KeyHome: "\x1b[H", tea.KeyEnd: "\x1b[F", tea.KeyPgUp: "\x1b[5~", tea.KeyPgDown: "\x1b[6~",
	tea.KeyDelete: "\x1b[3~", tea.KeyInsert: "\x1b[2~", tea.KeyShiftTab: "\x1b[Z",
}

// KeyBytes is what a terminal sends for a key: the bytes `tour play` writes
// to the program. Decode reads them back as the same key.
func KeyBytes(k tea.KeyMsg) ([]byte, error) {
	var b []byte
	switch {
	case k.Type == tea.KeyRunes:
		b = []byte(string(k.Runes))
	case k.Type == tea.KeySpace:
		b = []byte{' '}
	case csiKeys[k.Type] != "":
		b = []byte(csiKeys[k.Type])
	case k.Type >= 0 && k.Type <= 31 || k.Type == 127:
		b = []byte{byte(k.Type)}
	default:
		return nil, fmt.Errorf("the key %s cannot be sent to a terminal by this tool", SpellKey(k))
	}
	if k.Alt {
		b = append([]byte{0x1b}, b...)
	}
	return b, nil
}
