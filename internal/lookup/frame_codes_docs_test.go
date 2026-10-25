package lookup

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// The reference page's Frame codes section lists every house and zodiac code
// this package knows, with its name, and no other: a code added to or dropped
// from lens.go without the page following fails here.
func TestReferenceFrameCodesListEveryCode(t *testing.T) {
	b, err := os.ReadFile("../../docs/REFERENCE.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	i := strings.Index(text, "\n### Frame codes\n")
	if i < 0 {
		t.Fatal("the reference page has no Frame codes section")
	}
	sec := text[i+1:]
	if j := strings.Index(sec[len("### Frame codes"):], "\n#"); j >= 0 {
		sec = sec[:len("### Frame codes")+j]
	}
	var want []string
	for h := recipe.HouseSystem(0); int(h) < recipe.HouseSystemCount; h++ {
		want = append(want, fmt.Sprintf("| `%s` | %s |", HouseCode(h), HouseName(h)))
	}
	for z := recipe.Zodiac(0); int(z) < recipe.ZodiacCount; z++ {
		want = append(want, fmt.Sprintf("| `%s` | %s |", ZodiacCode(z), z.Name()))
	}
	row := regexp.MustCompile("(?m)^\\| `[A-Za-z]{2}` \\| .+ \\|$")
	have := row.FindAllString(sec, -1)
	if len(have) != len(want) {
		t.Errorf("the Frame codes section has %d code rows, lens.go knows %d", len(have), len(want))
	}
	for _, w := range want {
		if strings.Count(sec, w+"\n") != 1 {
			t.Errorf("the Frame codes section does not list %q exactly once", w)
		}
	}
}
