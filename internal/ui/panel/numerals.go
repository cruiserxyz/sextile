package panel

import (
	"fmt"

	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// CircledNumerals are ①–⑥, indexed by numeral-1: the Unicode numeral glyphs
// the operator asked for (P6).
var CircledNumerals = [6]string{"①", "②", "③", "④", "⑤", "⑥"}

// NumeralForColour is a numeral spelled for the current COLOUR mode: ①–⑥ with
// colours on, `1`–`6` with colours off (C1's tab titles). Out of range spells
// the plain digits, so a stray numeral can never index out of the table.
func NumeralForColour(n int, cfg term.Style) string {
	if n < 1 || n > len(CircledNumerals) || !cfg.UseColor() {
		return fmt.Sprintf("%d", n)
	}
	return CircledNumerals[n-1]
}
