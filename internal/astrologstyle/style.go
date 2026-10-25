package astrologstyle

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

type SGR string

const Reset SGR = "\x1b[0m"

type BodyRow struct{ Body, Position, House SGR }
type Rows map[string]BodyRow
type Bodies map[aspect.Body]BodyRow

// AspectIdentity keys the Astrolog-owned presentation spans for one typed
// relationship. First/Second preserve the order chosen by the producer: single
// chart callers canonicalize them to aspect.Aspect A/B; comparison callers keep
// Astrolog's primary/secondary chart roles.
type AspectIdentity struct {
	First, Second aspect.Body
	Kind          aspect.AspectKind
}

// AspectStyle carries the three Astrolog spans that have equivalents in
// Sextile's compact waterfall cell: first body, aspect kind, and second body.
// Sign and orb spans are deliberately absent because the compact cell does not
// render Astrolog's sign fields or its signed-orb presentation.
type AspectStyle struct{ First, Kind, Second SGR }

type Aspects map[AspectIdentity]AspectStyle

// AspectRow is one syntax-level row parsed from colored Astrolog output. The
// astrolog package resolves the text tokens through its canonical body/aspect
// tables before the styles enter payload.
type AspectRow struct {
	FirstName, KindName, SecondName string
	Style                           AspectStyle
}

var sgrRE = regexp.MustCompile("\\x1b\\[[0-9;]*m")

// 1 to 4, not 3 to 4: the name column is four cells wide and right-padded, and
// Astrolog prints `Io  :`. See the same bound in internal/astrolog/parse.go —
// the two patterns read the same column and must agree, or the styles and the
// positions disagree about which rows exist.
var rowRE = regexp.MustCompile(`^(\w{1,4}\s*:)(\s+\d+[A-Z][a-z]{2}\d+)(.*?\[\s*\d+(?:st|nd|rd|th)\s+house\])`)
var positionRE = regexp.MustCompile(`\d+[A-Z][a-z]{2}\d+`)
var houseRE = regexp.MustCompile(`\[\s*\d+(?:st|nd|rd|th)\s+house\]`)
var aspectRowRE = regexp.MustCompile(`^\s*\d+:\s+(.+?)\s+[\(\[][A-Z][a-z]{2}[\)\]]\s+([A-Z][A-Za-z]{2})\s+[\(\[][A-Z][a-z]{2}[\)\]]\s+(.+?)\s*-\s+orb:`)
var crossAspectRowRE = regexp.MustCompile(`^\s*\d+:\s+(.+?)\s+[\(\[][A-Z][a-z]{2}[\)\]]\s+([A-Z][A-Za-z]{2})\s+with\s+[\(\[][A-Z][a-z]{2}[\)\]]\s*(.+?)\s*-\s+orb:`)

func ParseBodies(stdout string) (Rows, error) {
	out := make(Rows)
	for _, line := range strings.Split(stdout, "\n") {
		plain, runs := styledLine(line)
		m := rowRE.FindStringSubmatch(plain)
		if m == nil {
			continue
		}
		body := strings.TrimSpace(strings.TrimSuffix(m[1], ":"))
		bodyStyle, okBody := styleFor(runs, 0, len(m[1]))
		pos, house := positionRE.FindStringIndex(plain), houseRE.FindStringIndex(plain)
		var posStyle, houseStyle SGR
		var okPos, okHouse bool
		if pos != nil {
			posStyle, okPos = styleFor(runs, pos[0], pos[1])
		}
		if house != nil {
			houseStyle, okHouse = styleFor(runs, house[0], house[1])
		}
		if !okBody || !okPos || !okHouse {
			return nil, fmt.Errorf("astrologstyle: uncolored body row %q", plain)
		}
		out[body] = BodyRow{Body: bodyStyle, Position: posStyle, House: houseStyle}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("astrologstyle: no colored body rows found")
	}
	return out, nil
}

// ParseAspectRows extracts the exact body/aspect/body SGR spans from a colored
// single-chart `-a -k` report. It fails closed when a recognized row is only
// partially colored; silently inventing a fallback would violate Astrolog's
// presentation authority.
func ParseAspectRows(stdout string) ([]AspectRow, error) {
	return parseAspectRows(stdout, aspectRowRE)
}

// ParseCrossAspectRows is the role-preserving `-r0 -a -k` analogue. The
// returned FirstName is Astrolog's primary (chart 1) body and SecondName its
// secondary (chart 2) body.
func ParseCrossAspectRows(stdout string) ([]AspectRow, error) {
	return parseAspectRows(stdout, crossAspectRowRE)
}

func parseAspectRows(stdout string, rowPattern *regexp.Regexp) ([]AspectRow, error) {
	var out []AspectRow
	for _, line := range strings.Split(stdout, "\n") {
		plain, runs := styledLine(line)
		m := rowPattern.FindStringSubmatchIndex(plain)
		if m == nil {
			continue
		}
		firstStyle, okFirst := styleFor(runs, m[2], m[3])
		kindStyle, okKind := styleFor(runs, m[4], m[5])
		secondStyle, okSecond := styleFor(runs, m[6], m[7])
		if !okFirst || !okKind || !okSecond {
			return nil, fmt.Errorf("astrologstyle: partially uncolored aspect row %q", plain)
		}
		out = append(out, AspectRow{
			FirstName:  strings.TrimSpace(plain[m[2]:m[3]]),
			KindName:   strings.TrimSpace(plain[m[4]:m[5]]),
			SecondName: strings.TrimSpace(plain[m[6]:m[7]]),
			Style:      AspectStyle{First: firstStyle, Kind: kindStyle, Second: secondStyle},
		})
	}
	return out, nil
}

func StripSGR(s string) string { return sgrRE.ReplaceAllString(s, "") }

type styledRun struct {
	start, end int
	style      SGR
}

func styledLine(line string) (string, []styledRun) {
	var plain strings.Builder
	var runs []styledRun
	style := SGR("")
	visible := 0
	for len(line) > 0 {
		loc := sgrRE.FindStringIndex(line)
		if loc == nil {
			text := line
			plain.WriteString(text)
			if style != "" && text != "" {
				runs = append(runs, styledRun{visible, visible + len(text), style})
			}
			break
		}
		text := line[:loc[0]]
		plain.WriteString(text)
		if style != "" && text != "" {
			runs = append(runs, styledRun{visible, visible + len(text), style})
		}
		visible += len(text)
		code := line[loc[0]:loc[1]]
		if code == string(Reset) {
			style = ""
		} else {
			style = SGR(code)
		}
		line = line[loc[1]:]
	}
	return plain.String(), runs
}

func styleFor(runs []styledRun, start, end int) (SGR, bool) {
	for _, run := range runs {
		if run.start <= start && end <= run.end {
			return run.style, true
		}
	}
	return "", false
}
