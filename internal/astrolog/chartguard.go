package astrolog

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// chartguard.go decides what of a chart file Astrolog may read. Astrolog applies
// every switch a file holds, and some act outside the chart: `-o` writes a file,
// `-i` reads another, an AstroExpression or a macro runs any command line, and a
// bare word is a switch too. So the copy Astrolog reads is built from an
// allow-list, line by line, and anything this walker cannot place refuses the
// chart. The class of every Astrolog switch is in docs/05-workbench/
// release-0.1.4/moment-design.md ("The file's own settings"), taken from the
// switch reader in Astrolog's astrolog.cpp.

// allowedArgs is the exact argument count of each switch a chart file may hand
// to Astrolog: the moment, the place, the labels and the house system. The
// zodiac (`s`) and the 3D house method (`c3`) take an optional argument and are
// walked apart. The moment switches' counts are the chart reader's own
// (momentArgs), so the guard and the reader never count a line apart.
var allowedArgs = func() map[string]int {
	m := map[string]int{"zZ": 2, "zi": 2, "zj": textArgs["zj"], "c": 1}
	for w, n := range momentArgs {
		m[w] = n
	}
	return m
}()

// reOffset is a number Astrolog's `-s` takes as its zodiac offset (io.cpp
// RParseSz, pmOffset): signed, with a decimal part or `:` minutes and seconds.
var reOffset = regexp.MustCompile(`^[-+]?([0-9]+(\.[0-9]*)?|\.[0-9]+)(:[0-9]+(:[0-9]+)?)?$`)

// ayanamsaNames are the names Astrolog's `-s` takes for an offset (data.cpp
// rgZodiacOffset). It matches one case-insensitively, whole or by a prefix of
// three letters or more (general.cpp FMatchSz).
// Its offset is the one table's (ayanamsaOffsets).
var ayanamsaNames = []struct {
	name string
	z    recipe.Zodiac
}{
	{"Fagan-Bradley", recipe.FaganBradley}, {"Lahiri", recipe.Lahiri}, {"Deluce", recipe.Deluce},
	{"Raman", recipe.Raman}, {"Usha-Shasi", recipe.UshaShasi}, {"Krishnamurti", recipe.Krishnamurti},
	{"Djwhal Khul", recipe.DjwhalKhul}, {"Yukteshwar", recipe.Yukteshwar}, {"J.N. Bhasin", recipe.JNBhasin},
	{"Galactic Center", recipe.GalacticCenter},
}

// namedAyanamsa is the ayanamsa tok names, as Astrolog's `-s` matches a name.
func namedAyanamsa(tok string) (recipe.Zodiac, bool) {
	t := strings.ToLower(tok)
	for _, a := range ayanamsaNames {
		n := strings.ToLower(a.name)
		if t == n || len(t) >= 3 && strings.HasPrefix(n, t) {
			return a.z, true
		}
	}
	return 0, false
}

// isOffset says whether Astrolog's `-s` takes tok as its offset.
func isOffset(tok string) bool {
	if len(tok) > maxArgumentBytes {
		return false
	}
	if reOffset.MatchString(tok) {
		return true
	}
	_, named := namedAyanamsa(tok)
	return named
}

// FileNote is one line of a chart file that Astrolog does not get, and why.
type FileNote struct {
	Switch string // the line's first switch, as written
	Class  FileSwitchClass
}

// ChartRefused is a chart file this walker will not hand to Astrolog.
type ChartRefused struct {
	Switch string // the switch, or the word, that refused it
	// Asks is what the switch asks for, in the operator's words, for a centre
	// or a derived chart Sextile does not show ("a heliocentric chart"); ""
	// for every other refusal.
	Asks string
	// Long says the refused switch is a title or place longer than Astrolog
	// reads back: past MaxLabelBytes it crashes reading the file.
	Long bool
}

// centreWords is what each centre or derived-chart switch asks for, from
// Astrolog's own help (astrolog -H, -Y): the words a refusal names it by, at
// most 25 cells, so the Log's line holds the chart's refusal whole.
var centreWords = map[string]string{
	"h": "a heliocentric chart", "x": "a harmonic chart", "9": "navamsa positions",
	"1": "a body on the Ascendant", "10": "a body on the Ascendant",
	"2": "a body on the Midheaven", "20": "a body on the Midheaven",
	"3": "decan positions", "4": "dwad positions", "F": "a body's forced position",
	"G": "houses from place alone", "f": "flipped houses", "sr": "right ascension positions",
	"YT": "true positions in space", "YV": "topocentric positions", "Yf": "refracted positions",
	"Yh": "barycentric positions", "Ym": "planetary moons", "Yc": "cusps as house positions",
	"Yp": "polar houses by Ascendant", "Yn0": "nutation turned off",
	"Ys": "the solar system's plane", "Y1": "rotated planets",
}

// asksFor is the words of a centre switch tok, or "" when it has none.
func asksFor(tok string) string {
	w := tok[1:]
	if words, ok := centreWords[w]; ok {
		return words
	}
	for _, p := range []string{"h", "sr", "Ys", "x", "F", "Y1"} {
		if strings.HasPrefix(w, p) {
			return centreWords[p]
		}
	}
	return ""
}

func (e *ChartRefused) Error() string {
	return fmt.Sprintf("the chart file holds %q, which Sextile does not hand to Astrolog", e.Switch)
}

// guardChart is the guard every chart read goes through. Production never
// reassigns it; the hostile-file test's control switches it off to prove the
// guard is what stops each file.
var guardChart = GuardChartFile

// GuardChartFile returns the copy of a chart file that Astrolog may read: its
// header line and every line made only of allowed switches, each with exactly
// its arguments. Any other line is decided by every switch on it
// (leftOutLine): a line of display switches is left out silently, because
// Sextile's own pins set the display; one that also sets objects, aspects or
// orbs is left out and noted. The Node and Lilith switches are taken out of
// their lines first and said nothing about (takeNodeSwitches). Anything else refuses the
// chart: a chart switch beside one Sextile does not pass, a centre or derived
// chart, a moment shift, a switch that acts outside the chart, a bare word, a
// second header, or a line the walker cannot read to its end.
func GuardChartFile(text string) (string, []FileNote, error) {
	lines := chartFileLines(text)
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "@AI") {
		return "", nil, &ChartRefused{Switch: firstWord(lines)}
	}
	// Astrolog reads the header line on: only a comment may follow the tag.
	if head := astrologTokens(lines[0]); len(head) > 1 && head[1] != "" && head[1][0] != ';' {
		return "", nil, &ChartRefused{Switch: head[1]}
	}
	kept := []string{lines[0]}
	var notes []FileNote
	for _, line := range lines[1:] {
		toks := astrologTokens(line)
		if len(toks) == 0 {
			continue
		}
		if len(toks) > maxLineWords {
			// Astrolog reads no further on the line, and the rest would cast
			// one moment here and another wherever the line is read whole.
			return "", nil, &ChartRefused{Switch: toks[maxLineWords]}
		}
		// The Node and Lilith switches never reach Astrolog (FileNodeForm
		// reads what they ask for); the rest of the line is judged as before.
		cut, taken, unsure := takeNodeSwitches(line)
		if unsure != "" {
			return "", nil, &ChartRefused{Switch: unsure}
		}
		if taken > 0 {
			line, toks = cut, astrologTokens(cut)
			if len(toks) == 0 {
				continue
			}
		}
		first := toks[0]
		if first == "" {
			return "", nil, &ChartRefused{Switch: `""`} // an empty word read as a switch
		}
		if first[0] == ';' || first == "-"+string(rune(';')) {
			continue // a comment line: Astrolog reads nothing on it
		}
		if strings.HasPrefix(first, "@") {
			return "", nil, &ChartRefused{Switch: first}
		}
		ok, bad, err := allowedLine(toks)
		if err != nil {
			return "", nil, err
		}
		if ok {
			kept = append(kept, line)
			continue
		}
		lineNotes, err := leftOutLine(toks, bad)
		if err != nil {
			return "", nil, err
		}
		notes = append(notes, lineNotes...)
	}
	return strings.Join(kept, "\n") + "\n", notes, nil
}

// leftOutLine decides a line that is not made only of allowed switches, by
// every switch on it, never by its first alone (`=Yt -s Lahiri` would lose
// the zodiac in silence). A line that also holds an allowed switch is
// refused, naming the first switch that is not allowed: dropping it would
// drop the moment, the place or the zodiac with it. Otherwise each switch is
// classified: display switches are left out silently, objects, aspects or orbs
// are left out and noted once per switch, and a centre, a derived chart or any
// other switch refuses the chart. bad is the first token allowedLine could not place.
func leftOutLine(toks []string, bad string) ([]FileNote, error) {
	if !startsSwitchOrComment(toks[0]) || len(toks[0]) < 2 {
		return nil, &ChartRefused{Switch: toks[0]} // a bare word is a switch to Astrolog
	}
	var notes []FileNote
	for i, t := range toks {
		if t == "" || t[0] == ';' {
			break
		}
		// A line's first word is a switch even when it reads as a number:
		// `-9` and `-1` are charts of their own.
		if !startsSwitchOrComment(t) || i > 0 && isSwitchValue(t[1:]) || len(t) < 2 {
			continue // an argument
		}
		if allowedSwitch(t) {
			return nil, &ChartRefused{Switch: bad}
		}
		switch c := ClassifyFileSwitch(t); c {
		case SwitchDisplay:
		case SwitchLens:
			notes = append(notes, FileNote{Switch: t, Class: c})
		case SwitchCentre:
			// A centre or a derived chart is a different chart: Sextile has
			// no form for it yet, so the file does not open (the operator,
			// 2026-11-15), as a progressed one does not.
			return nil, &ChartRefused{Switch: t, Asks: asksFor(t)}
		default:
			return nil, &ChartRefused{Switch: t}
		}
	}
	return notes, nil
}

// allowedSwitch says whether tok is one of the switches a chart line may hold
// (allowedLine): the moment, the place, the labels, the house system, the
// zodiac and the 3D house method.
func allowedSwitch(tok string) bool {
	prefix, w := tok[0], tok[1:]
	switch w {
	case "s", "c3":
		return strings.ContainsRune("-/=_", rune(prefix))
	}
	_, known := allowedArgs[w]
	return known && (prefix == '-' || prefix == '/')
}

// allowedLine says whether a line is made only of allowed switches, each with
// exactly its arguments. When it is not, bad is the first token that is not.
// An allowed switch whose argument Astrolog must not read refuses the chart
// (err): a label longer than Astrolog reads back, or any other argument that
// is not a plain word (argumentOK).
func allowedLine(toks []string) (ok bool, bad string, err error) {
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t == "" {
			return false, `""`, nil
		}
		if t[0] == ';' {
			return true, "", nil // the rest of the line is a comment
		}
		prefix, w := t[0], t[1:]
		if !strings.ContainsRune("-/=_", rune(prefix)) || w == "" {
			return false, t, nil
		}
		switch {
		case w == "s":
			// `-s` toggles the sidereal zodiac, `=s` and `_s` set it; an offset
			// may follow (astrolog.cpp:2475).
			if i+1 < len(toks) && isOffset(toks[i+1]) {
				i++
			} else if i+1 < len(toks) && !startsSwitchOrComment(toks[i+1]) {
				return false, "", &ChartRefused{Switch: t}
			}
			continue
		case w == "c3":
			// The 3D house method: a digit may follow (astrolog.cpp:2446).
			if i+1 < len(toks) && toks[i+1] != "" && toks[i+1][0] >= '0' && toks[i+1][0] <= '9' {
				if len(toks[i+1]) != 1 || toks[i+1][0] > '3' {
					return false, "", &ChartRefused{Switch: t}
				}
				i++
			} else if i+1 < len(toks) && !startsSwitchOrComment(toks[i+1]) {
				return false, "", &ChartRefused{Switch: t}
			}
			continue
		}
		n, known := allowedArgs[w]
		if !known || prefix != '-' && prefix != '/' {
			return false, t, nil
		}
		if i+n > len(toks)-1 {
			return false, t, nil
		}
		for k, arg := range toks[i+1 : i+1+n] {
			if isLabel(w, k) {
				if len(arg) > MaxLabelBytes {
					return false, "", &ChartRefused{Switch: t, Long: true}
				}
			} else if !argumentOK(arg) {
				return false, "", &ChartRefused{Switch: t}
			}
		}
		i += n
	}
	return true, "", nil
}

// isLabel says whether argument k (from 0) of switch w is a title or a place,
// which Astrolog keeps as text and never reads as a number.
func isLabel(w string, k int) bool {
	return w == "zi" || w == "zj" || w == "qc" && k >= 8
}

// reArgument is a plain word: a date, a clock, a zone, a coordinate, a
// Julian day or a house system's name, as Astrolog writes them (`77:13'00E`).
// It holds no `~`, which makes Astrolog run the rest as an AstroExpression
// (io.cpp NParseSz, RParseSz, RFromSz), and no leading space, which Astrolog
// strips before it looks for one.
var reArgument = regexp.MustCompile(`^[-+]?[0-9A-Za-z.][0-9A-Za-z .,:'()-]*$`)

// maxLineWords is how many words Astrolog reads from one line of a file
// (astrolog.cpp NParseCommandLine, MAXSWITCHES 100 less the program name and
// the end mark); it warns and ignores the rest.
const maxLineWords = 98

// maxArgumentBytes bounds an argument well below the 255 bytes Astrolog copies
// it into unchecked (io.cpp NParseSz and RParseSz, cchSzMax): past them it
// crashes. Its longest house system name is 19.
const maxArgumentBytes = 32

// argumentOK says whether Astrolog may read arg as a switch's argument.
func argumentOK(arg string) bool {
	return len(arg) <= maxArgumentBytes && reArgument.MatchString(arg)
}

// startsSwitchOrComment says whether tok begins a switch or a comment, so it
// is not an argument of the switch before it.
func startsSwitchOrComment(tok string) bool {
	if tok == "" {
		return false
	}
	switch tok[0] {
	case '-', '/', '=', '_', ';':
		return true
	}
	return false
}

func firstWord(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	if toks := astrologTokens(lines[0]); len(toks) > 0 {
		return toks[0]
	}
	return lines[0]
}

// InlineRecord is a guarded copy (GuardChartFile) as the argv items Astrolog
// would take from it: every switch line after the header, split as Astrolog
// splits a file's line (a quoted label is one item), comments left out.
func InlineRecord(guarded string) []string {
	var argv []string
	for _, line := range chartFileLines(guarded)[1:] {
		for _, tok := range astrologTokens(line) {
			if tok != "" && tok[0] == ';' {
				break
			}
			argv = append(argv, tok)
		}
	}
	return argv
}

// FileZodiac is the zodiac a guarded record sets, walked as Astrolog reads it
// (astrolog.cpp:2475): `-s` toggles the sidereal zodiac, `=s` sets it, `_s`
// clears it, and an offset after any of them sets the ayanamsa offset from
// Fagan-Bradley. set is false when the record holds no zodiac switch. Sextile
// sends `_s 0` before the record, so the walk starts tropical at offset 0.
func FileZodiac(guarded string) (set, sidereal bool, offset float64) {
	toks := InlineRecord(guarded)
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if len(t) != 2 || t[1] != 's' {
			continue
		}
		set = true
		switch t[0] {
		case '-', '/':
			sidereal = !sidereal
		case '=':
			sidereal = true
		case '_':
			sidereal = false
		}
		if i+1 < len(toks) && isOffset(toks[i+1]) {
			offset = offsetValue(toks[i+1])
			i++
		}
	}
	return set, sidereal, offset
}

// offsetValue is the degrees of an offset Astrolog's `-s` takes: a number, with
// `:` minutes and seconds, or an ayanamsa's name (data.cpp rgZodiacOffset).
func offsetValue(tok string) float64 {
	if z, named := namedAyanamsa(tok); named {
		off, _ := AyanamsaOffset(z)
		return off
	}
	neg := strings.HasPrefix(tok, "-")
	parts := strings.Split(strings.TrimLeft(tok, "+-"), ":")
	if len(parts) == 2 {
		if m, sec, ok := compactMMSS(parts[1]); ok {
			parts = []string{parts[0], fmt.Sprint(m), fmt.Sprint(sec)}
		}
	}
	var v float64
	for i, p := range parts {
		var x float64
		fmt.Sscan(p, &x)
		for j := 0; j < i; j++ {
			x /= 60
		}
		v += x
	}
	if neg {
		v = -v
	}
	return v
}
