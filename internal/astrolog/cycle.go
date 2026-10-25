package astrolog

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The cycle search (the specification P11): Astrolog's own
// event list over whole years (`-dY`) for the Sun and one other body,
// conjunctions only, at UT (`-z 0`) with seconds (`-b0`). A body template
// reads its synodic events from it: the conjunctions with the Sun, the other
// body's stations and ingresses, each as Astrolog printed it.

// CycleRequest is one cycle search: Years whole years from 1 January of
// StartYear, for the Sun and Other, in the lens's reference frame.
type CycleRequest struct {
	StartYear int
	Years     int
	Other     aspect.Body
	Houses    recipe.HouseSystem
	Zodiac    recipe.Zodiac
}

// cycleFirstYear is the first year the search may start in: Astrolog prints
// Julian-calendar dates before the Gregorian reform, which this parser does
// not read.
const cycleFirstYear = 1583

// CycleCall is the search as a body call. `-z 0` comes first, so the list
// is in UT; `-qy` names only the year the search starts, never a moment's
// calendar fields.
func CycleCall(req CycleRequest) (BodyCall, error) {
	if req.StartYear < cycleFirstYear {
		return BodyCall{}, fmt.Errorf("astrolog: CycleCall: a search from %d reaches dates before the Gregorian reform, which the cycle list does not read", req.StartYear)
	}
	if req.Years < 1 {
		return BodyCall{}, fmt.Errorf("astrolog: CycleCall: %d years", req.Years)
	}
	argv := []string{"-z", "0", "-qy", strconv.Itoa(req.StartYear), "-dY", strconv.Itoa(req.Years), "-b0"}
	frame, err := frameBand(req.Houses, req.Zodiac)
	if err != nil {
		return BodyCall{}, err
	}
	argv = append(argv, frame...)
	argv = append(argv, reformPin...)
	// The list is read, never shown, so its clock is fixed (24-hour, month
	// first) whatever the operator chose for what Astrolog prints on screen.
	argv = append(argv, Clock{}.Pins()...)
	band, err := cycleBand(req.Other)
	if err != nil {
		return BodyCall{}, err
	}
	return newBodyCall(append(argv, band.tokens...), band), nil
}

// CycleKind is what a cycle line is.
type CycleKind int

const (
	CycleConjunction CycleKind = iota + 1
	CycleStation
	CycleIngress
)

// CyclePoint is a body's position as the line printed it: degree, minute
// and sign, and the bracket around it — '(' direct, '[' retrograde, '<'
// stationary (Astrolog's own notation).
type CyclePoint struct {
	Deg, Min int
	Sign     string // Astrolog's three letters: Ari, Tau, …
	Bracket  byte
}

// CycleEvent is one line of the list.
type CycleEvent struct {
	Instant time.Time // UT, to Astrolog's printed second
	Line    string
	Kind    CycleKind
	Body    aspect.Body
	At      CyclePoint
	// A conjunction's second body and its position; a station's word
	// ("S/R" or "S/D"); an ingress's sign.
	Other   aspect.Body
	OtherAt CyclePoint
	Station string
	Ingress string
}

var (
	cycleLineRE = regexp.MustCompile(`^(?:Sun|Mon|Tue|Wed|Thu|Fri|Sat) +(\d{1,2})/ ?(\d{1,2})/(\d{4}) +(\d{1,2}):(\d{2}):(\d{2}) +([A-Z][A-Za-z]*(?: [A-Z][A-Za-z]*)?) ([(\[<])([ \d]\d)([A-Z][a-z]{2})(\d{2})([)\]>]) (.*)$`)
	cycleConRE  = regexp.MustCompile(`^Con ([(\[<])([ \d]\d)([A-Z][a-z]{2})(\d{2})([)\]>]) ([A-Z][A-Za-z]*(?: [A-Z][A-Za-z]*)?)(?: \([A-Z][a-z]+(?: [A-Z][a-z]+)* \d{1,3}%\))?$`)
	cycleIngRE  = regexp.MustCompile(`^(?:-->|<--) ([A-Z][a-z]+)(?: \((?:Spring|Summer|Autumn|Winter) (?:Equinox|Solstice)\))?$`)
	cycleStaRE  = regexp.MustCompile(`^S/([RD])$`)
	closers     = map[byte]byte{'(': ')', '[': ']', '<': '>'}
)

// ParseCycleEvents reads every line of a cycle search or refuses the whole
// result: an unknown line is an error naming it, never a skipped row.
func ParseCycleEvents(out string) ([]CycleEvent, error) {
	var events []CycleEvent
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, " \r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		ev, err := parseCycleLine(line)
		if err != nil {
			return nil, fmt.Errorf("astrolog: ParseCycleEvents: %q: %w", line, err)
		}
		events = append(events, ev)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("astrolog: ParseCycleEvents: no events")
	}
	return events, nil
}

func parseCycleLine(line string) (CycleEvent, error) {
	m := cycleLineRE.FindStringSubmatch(line)
	if m == nil {
		return CycleEvent{}, fmt.Errorf("not a dated event line")
	}
	month, _ := strconv.Atoi(m[1])
	day, _ := strconv.Atoi(m[2])
	year, _ := strconv.Atoi(m[3])
	hour, _ := strconv.Atoi(m[4])
	minute, _ := strconv.Atoi(m[5])
	sec, _ := strconv.Atoi(m[6])
	if hour > 23 {
		return CycleEvent{}, fmt.Errorf("hour %d", hour)
	}
	when := time.Date(year, time.Month(month), day, hour, minute, sec, 0, time.UTC)
	if when.Month() != time.Month(month) || when.Day() != day || minute > 59 || sec > 59 {
		return CycleEvent{}, fmt.Errorf("an impossible date or time")
	}
	ev := CycleEvent{Instant: when, Line: line}
	var ok bool
	if ev.Body, ok = bodyFullName[m[7]]; !ok {
		return CycleEvent{}, fmt.Errorf("unknown body %q", m[7])
	}
	if ev.At, ok = cyclePoint(m[8], m[9], m[10], m[11], m[12]); !ok {
		return CycleEvent{}, fmt.Errorf("an unreadable position")
	}
	rest := m[13]
	switch {
	case cycleConRE.MatchString(rest):
		c := cycleConRE.FindStringSubmatch(rest)
		ev.Kind = CycleConjunction
		if ev.OtherAt, ok = cyclePoint(c[1], c[2], c[3], c[4], c[5]); !ok {
			return CycleEvent{}, fmt.Errorf("an unreadable position")
		}
		if ev.Other, ok = bodyFullName[c[6]]; !ok {
			return CycleEvent{}, fmt.Errorf("unknown body %q", c[6])
		}
	case cycleStaRE.MatchString(rest):
		ev.Kind, ev.Station = CycleStation, rest
	case cycleIngRE.MatchString(rest):
		ev.Kind, ev.Ingress = CycleIngress, cycleIngRE.FindStringSubmatch(rest)[1]
	default:
		return CycleEvent{}, fmt.Errorf("not a conjunction, station or ingress")
	}
	return ev, nil
}

func cyclePoint(open, deg, sign, min, close string) (CyclePoint, bool) {
	if closers[open[0]] != close[0] {
		return CyclePoint{}, false
	}
	d, err1 := strconv.Atoi(strings.TrimSpace(deg))
	mi, err2 := strconv.Atoi(min)
	if err1 != nil || err2 != nil || d > 29 || mi > 59 {
		return CyclePoint{}, false
	}
	return CyclePoint{Deg: d, Min: mi, Sign: sign, Bracket: open[0]}, true
}

// SunPairRequest is the Sun and one other body at a moment, read to the
// second of arc (`-b0`): what a body template's verdicts are judged on
// (the specification P9), never a display-rounded value.
type SunPairRequest struct {
	When     time.Time
	Lon, Lat float64
	Other    aspect.Body
	Houses   recipe.HouseSystem
	Zodiac   recipe.Zodiac
}

// SunPairCall is the request as a body call: the moment as its Julian Day
// (momentArg, the same second every call for the slot names), `-b0`, the
// frame, the reform pin, a fixed clock (the list is read, never shown) and
// the band of the Sun and the body.
func SunPairCall(req SunPairRequest) (BodyCall, error) {
	jd, err := momentArg(SlotJD(req.When))
	if err != nil {
		return BodyCall{}, fmt.Errorf("astrolog: SunPairCall: %w", err)
	}
	// -v prints the positions and the retrograde column, -Z the horizon
	// table with the magnitude and the illuminated fraction: both in one call.
	argv := []string{"-qj", jd, "-zl", formatLon(req.Lon), formatLat(req.Lat), "-b0", "-v", "-Z"}
	frame, err := frameBand(req.Houses, req.Zodiac)
	if err != nil {
		return BodyCall{}, err
	}
	argv = append(argv, frame...)
	argv = append(argv, reformPin...)
	argv = append(argv, Clock{}.Pins()...)
	band, err := cycleBand(req.Other)
	if err != nil {
		return BodyCall{}, err
	}
	return newBodyCall(append(argv, band.tokens...), band), nil
}

// PrecisePosition is a body's longitude to the second of arc, its velocity,
// its sign and retrograde column, and its brightness, as `-b0 -v -Z` printed
// them.
type PrecisePosition struct {
	Body       aspect.Body
	Longitude  float64 // degrees, 0 to 360
	Speed      float64 // degrees per day, as printed
	Sign       string  // Astrolog's three letters
	Retrograde bool    // Astrolog's R column
	Magnitude  float64 // -Z Magn.
	LitPercent float64 // -Z Full (Area), the illuminated fraction
}

var (
	pairHeadRE  = regexp.MustCompile(`^Astrolog \d+\.\d+: `)
	pairColsRE  = regexp.MustCompile(`^Body +Location +Ret\. +Latitude `)
	pairZColsRE = regexp.MustCompile(`^Body +Altitude +Azimuth +Azi\. Vector +Magn\. +Full\(Area,Time\) +Diameter$`)
	pairRowRE   = regexp.MustCompile(`^([A-Z][a-z]{2,3}) ?: +(\d{1,2})([A-Z][a-z]{2})(\d{2})'(\d{2})" (R| ) +[-+] ?\d{1,2}:\d{2}'\d{2}" +\(.\) +\[[ \d]\d[a-z]{2} house\] +\[.\] +([-+]\d+\.\d+) +[-+]\d+\.\d+$`)
	// The altitude's sign is followed by a space below ten degrees, as the
	// latitude's is: `- 3:03'28"`, `+18:02'22"`.
	pairZRowRE = regexp.MustCompile(`^([A-Z][a-z]{2,3}) ?: +[-+] ?\d{1,2}:\d{2}'\d{2}" +\d{1,3}:\d{2}'\d{2}" +\(\d\.\d{3}[ns] \d\.\d{3}[ew]\) +([-+]?\d+\.\d+) +(\d{1,3}\.\d{2})% +\d{1,3}\.\d{2}% +\d+:\d{2}'\d{2}\.\d{3}"$`)
)

// ParseSunPair reads the Sun's and the other body's rows of a SunPairCall
// output, positions then the horizon table; any line it does not know, a
// missing row or a second one refuses the result.
func ParseSunPair(out string, other aspect.Body) (sun, body PrecisePosition, err error) {
	got := map[aspect.Body]PrecisePosition{}
	bright := map[aspect.Body]bool{}
	inZ := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, " \r")
		if line == "" || pairHeadRE.MatchString(line) || pairColsRE.MatchString(line) {
			continue
		}
		if pairZColsRE.MatchString(line) {
			inZ = true
			continue
		}
		if inZ {
			z := pairZRowRE.FindStringSubmatch(line)
			if z == nil {
				return sun, body, fmt.Errorf("astrolog: ParseSunPair: %q is not a horizon row", line)
			}
			b, ok := bodyAbbrev4[z[1]]
			p, seen := got[b]
			if !ok || !seen || bright[b] {
				return sun, body, fmt.Errorf("astrolog: ParseSunPair: horizon row %q matches no position row once", z[1])
			}
			p.Magnitude, _ = strconv.ParseFloat(z[2], 64)
			p.LitPercent, _ = strconv.ParseFloat(z[3], 64)
			got[b], bright[b] = p, true
			continue
		}
		m := pairRowRE.FindStringSubmatch(line)
		if m == nil {
			return sun, body, fmt.Errorf("astrolog: ParseSunPair: %q is not a position row", line)
		}
		b, ok := bodyAbbrev4[m[1]]
		if !ok || (b != aspect.Sun && b != other) {
			return sun, body, fmt.Errorf("astrolog: ParseSunPair: %q is not the Sun or %s", m[1], other)
		}
		if _, dup := got[b]; dup {
			return sun, body, fmt.Errorf("astrolog: ParseSunPair: %s twice", b)
		}
		sign, ok := signAbbrev[m[3]]
		deg, _ := strconv.Atoi(m[2])
		mi, _ := strconv.Atoi(m[4])
		sec, _ := strconv.Atoi(m[5])
		if !ok || deg > 29 || mi > 59 || sec > 59 {
			return sun, body, fmt.Errorf("astrolog: ParseSunPair: %q: an unreadable position", line)
		}
		speed, _ := strconv.ParseFloat(m[7], 64)
		got[b] = PrecisePosition{Body: b, Longitude: float64(int(sign-1))*30 + float64(deg) + float64(mi)/60 + float64(sec)/3600, Speed: speed,
			Sign: m[3], Retrograde: m[6] == "R"}
	}
	var okSun, okBody bool
	sun, okSun = got[aspect.Sun]
	body, okBody = got[other]
	if !okSun || !okBody || !bright[aspect.Sun] || !bright[other] {
		return sun, body, fmt.Errorf("astrolog: ParseSunPair: the output lacks the Sun's or %s's position or horizon row", other)
	}
	return sun, body, nil
}
