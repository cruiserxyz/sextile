package astrolog

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// bodyAbbrev4 maps astrolog's 3- or 4-letter body abbreviation (the row label
// in the default chart) to the pkg/aspect Body enum. The default chart emits
// Sun–Pluto, Nort, Asce and Midh; the remaining angles + Chiron/SouthNode/
// Lilith appear only once the body-restriction switches enable them (Slice B2),
// so those keys are harmless until then. astrolog 7.80 spells Imum Coeli "Nadi".
var bodyAbbrev4 = map[string]aspect.Body{
	"Sun":  aspect.Sun,
	"Moon": aspect.Moon,
	"Merc": aspect.Mercury,
	"Venu": aspect.Venus,
	"Mars": aspect.Mars,
	"Jupi": aspect.Jupiter,
	"Satu": aspect.Saturn,
	"Uran": aspect.Uranus,
	"Nept": aspect.Neptune,
	"Plut": aspect.Pluto,
	"Nort": aspect.NorthNode,
	"Sout": aspect.SouthNode,
	"Chir": aspect.Chiron,
	"Lili": aspect.Lilith,
	"Asce": aspect.Ascendant,
	"Midh": aspect.Midheaven,
	"Desc": aspect.Descendant,
	"Nadi": aspect.ImumCoeli,
}

// bodyNames is the one table of the names Astrolog prints for the bodies the
// aspect.Body enum can name, in the long form the aspect lists use (astrolog
// 8 spells Imum Coeli "Nadir"). bodyFullName is derived from it, and both
// aspect parsers resolve through that map.
var bodyNames = []struct {
	name string
	body aspect.Body
}{
	{"Sun", aspect.Sun}, {"Moon", aspect.Moon}, {"Mercury", aspect.Mercury},
	{"Venus", aspect.Venus}, {"Mars", aspect.Mars}, {"Jupiter", aspect.Jupiter},
	{"Saturn", aspect.Saturn}, {"Uranus", aspect.Uranus}, {"Neptune", aspect.Neptune},
	{"Pluto", aspect.Pluto}, {"North Node", aspect.NorthNode}, {"South Node", aspect.SouthNode},
	{"Chiron", aspect.Chiron}, {"Lilith", aspect.Lilith}, {"Ascendant", aspect.Ascendant},
	{"Midheaven", aspect.Midheaven}, {"Descendant", aspect.Descendant}, {"Nadir", aspect.ImumCoeli},
}

// bodyFullName maps a body name as printed in an aspect list to the pkg/aspect
// Body enum: every long name in bodyNames, plus its first seven characters when
// it is longer, because the left column of a list is seven cells wide
// ("Ascenda", "Midheav", "Descend", "North N", "South N"). No two names share a
// seven-character prefix, so the lookup is unambiguous. A name absent from the
// map is a body the enum cannot name; the parsers keep it by its printed name.
var bodyFullName = func() map[string]aspect.Body {
	m := make(map[string]aspect.Body, 2*len(bodyNames))
	for _, e := range bodyNames {
		m[e.name] = e.body
		if len(e.name) > 7 {
			m[e.name[:7]] = e.body
		}
	}
	return m
}()

// signAbbrev maps astrolog's 3-letter sign abbreviation to the pkg/aspect
// Sign enum.
var signAbbrev = map[string]aspect.Sign{
	"Ari": aspect.Aries,
	"Tau": aspect.Taurus,
	"Gem": aspect.Gemini,
	"Can": aspect.Cancer,
	"Leo": aspect.Leo,
	"Vir": aspect.Virgo,
	"Lib": aspect.Libra,
	"Sco": aspect.Scorpio,
	"Sag": aspect.Sagittarius,
	"Cap": aspect.Capricorn,
	"Aqu": aspect.Aquarius,
	"Pis": aspect.Pisces,
}

// aspectAbbrev maps astrolog's 3-letter aspect abbreviation to the
// pkg/aspect AspectKind. Astrolog calls the 150° aspect "Inconjunct" (Inc);
// the system uses the traditional name Quincunx for the same angle.
var aspectAbbrev = map[string]aspect.AspectKind{
	"Con": aspect.Conjunction,
	"Opp": aspect.Opposition,
	"Squ": aspect.Square,
	"Tri": aspect.Trine,
	"Sex": aspect.Sextile,
	"Inc": aspect.Quincunx,
	"SSx": aspect.Semisextile,
	"SSq": aspect.Semisquare,
	"Ses": aspect.Sesquisquare,
	"Qui": aspect.Quintile,
	"BQn": aspect.Biquintile,
}

// bodyPositionPat matches one body-position line from astrolog's default
// chart output. Representative line:
//
//	Sun:  0Ari55   - 0:00' (X) [ 8th house] [-] +0.993  -  House cusp ...
//
// Capture groups, in order: body abbreviation, degree, sign abbreviation,
// minute, retrograde flag ("R" or empty), latitude sign ("+" or "-"),
// latitude degree, latitude minute, house number (1..12, may have leading
// space), ordinal suffix ("st"/"nd"/"rd"/"th"; ignored at extract time),
// velocity (signed).
// The trailing "- House cusp …" segment is optional: the last body row astrolog
// emits (Midheaven in the default chart) has no cusp after its velocity, so the
// velocity group anchors the match without requiring it.
// The name column is FOUR cells wide and right-padded, so a name is 1 to 4
// characters — not 3 to 4. Astrolog prints `Io  :` for Jupiter's moon, and a
// {3,4} bound matched no row at all for a chart containing it: zero positions
// parsed, and the panels drew nothing. Every other object in the shipped
// registry happens to be three or four, which is why the bound survived until a
// selector could put Io in a chart on its own.
var bodyPositionPat = regexp.MustCompile(
	`^(\w{1,4})\s*:\s+(\d+)([A-Z][a-z]{2})(\d+)\s+(R?)\s*([+-])\s*(\d+).(\d+)'.*?\[\s*(\d+)(st|nd|rd|th)\s+house\].*?\s([+-]\d+(?:\.\d+)?)`,
)

// aspectLinePat matches one aspect line from astrolog's -a output.
// Representative line:
//
//	1:     Sun (Ari) Con (Ari) Moon       - orb: +1:08' - power: 23.99
//
// The right name is a fixed ten-cell field that can abut or nearly abut the
// dash ("South Node - orb", "Descendant- orb"), so it is matched lazily and the
// space before the dash is optional.
//
// Body A is Astrolog's FIRST name, printed right-aligned in a seven-cell column
// and cut to fit (charts2.cpp:750, "%7.7s"): "North N", "East Po", "6th Cus",
// "11th Cu", "Kaus Au". It is read as whatever fills that column -- a word, a
// space, anything -- up to seven cells, so no cut of any object's name can drop
// the row (the specification C1). The sign is in (parens) or [brackets];
// <angle brackets> mean the body has no speed at all, which in practice is a
// body Astrolog could not compute (its ephemeris file is missing, so it sits
// at 0Ari00), and such a row is deliberately not read.
//
// Capture groups, in order: body A name,
// sign A, aspect abbreviation, sign B, body B name, orb marker, orb degrees,
// orb minutes, power (aspect strength). Power anchors the waterfall sort.
//
// The orb marker is `+`/`-` (offset from exact) under plain -a, or `a`/`s`
// (applying/separating, Astrolog's own velocity-based phase) under -aa, the
// mode the compiler emits:
//
//	1:   Venus (Tau) Squ (Leo) Midheaven  - orb: a0:20' - power:  9.51
var aspectLinePat = regexp.MustCompile(
	`^\s*\d+:\s+(\S(?:.{0,5}\S)?)\s+[\(\[]([A-Z][a-z]{2})[\)\]]\s+([A-Z][A-Za-z]{2})\s+[\(\[]([A-Z][a-z]{2})[\)\]]\s+(\S+?(?:\s+\S+?)?)\s*-\s+orb:\s+([+-as])\s*(\d+)[^0-9](\d+)'\s+-\s+power:\s+([0-9.]+)`,
)

// applyingMarker is the -aa orb marker for an applying aspect; `s` marks a
// separating one and `+`/`-` (plain -a) carry no phase at all.
const applyingMarker = "a"

// crossAspectLinePat matches one aspect line from astrolog's -r0 relationship
// output. Representative lines (verified, astrolog 7.80):
//
//	1:     Sun (Gem) Squ with (Pis) Jupiter   - orb: +0:22' - power: 15.26
//	22: Mercury (Can) Opp with [Cap] North Node- orb: -0:09' - power:  5.51
//
// The LEFT body (group 1, truncated to 7 chars) is from the first chart, the
// RIGHT body (group 3, full name, after "with") from the second — membership is
// POSITIONAL, not encoded by the (paren)/[bracket] sign tags (those mark direct
// vs retrograde and are discarded). The right body may abut "- orb" with no
// space (a fixed-width field, as in line 22), so its capture is non-greedy up to
// "- orb". Groups: leftBody, aspect, rightBody, orbMarker, orbDeg, orbMin,
// power. The orb marker is `a`/`s` under -aa (see aspectLinePat), `+`/`-`
// under plain -a.
// (?m) so ^ anchors each line: parseCrossAspects matches per split line (where
// it is a no-op), while checkCrossAspectSanity matches against the whole stdout.
var crossAspectLinePat = regexp.MustCompile(
	`(?m)^\s*\d+:\s+(.+?)\s+[\(\[][A-Z][a-z]{2}[\)\]]\s+([A-Z][A-Za-z]{2})\s+with\s+[\(\[][A-Z][a-z]{2}[\)\]]\s*(.+?)\s*-\s+orb:\s+([+-as])\s*(\d+)[^0-9](\d+)'\s+-\s+power:\s+([0-9.]+)`,
)

// resolveCrossBody maps an astrolog -r0 body token to a Body. The right-side
// token is the full name; the left-side token is truncated to 7 chars
// ("Ascenda"→Ascendant, "North N"→North Node). An exact match, or a token that
// is the 7-char prefix of a longer canonical name, resolves unambiguously — no
// two canonical names share a 7-char prefix. It reads bodyFullName, the table
// parseAspects uses too.
func resolveCrossBody(tok string) (aspect.Body, bool) {
	body, ok := bodyFullName[strings.TrimSpace(tok)]
	return body, ok
}

// resolveAspectBody resolves a body token of an aspect line. A body the enum
// cannot name comes back as BodyUnknown with the printed name (trimmed; the left
// column may be a seven-character truncation) as its label; a typed body has no
// label.
func resolveAspectBody(tok string) (aspect.Body, string) {
	tok = strings.TrimSpace(tok)
	if body, ok := bodyFullName[tok]; ok {
		return body, ""
	}
	return aspect.BodyUnknown, tok
}

// parseCrossAspects extracts cross-aspects from astrolog's -r0/-rt -aa output.
// Roles are preserved (Primary = left/first chart, Secondary = right/second
// chart) — NOT canonicalized like parseAspects. Lines that don't match, or
// whose aspect kind isn't in the system enum, are silently skipped. A body the
// Body enum cannot name is KEPT as BodyUnknown carrying the printed name in
// PrimaryLabel/SecondaryLabel.
// Applying is Astrolog's own `a`/`s` marker; its semantics follow the
// relation switch the list ran under (see aspect.CrossAspect).
func parseCrossAspects(stdout string) []aspect.CrossAspect {
	var out []aspect.CrossAspect
	stdout = astrologstyle.StripSGR(stdout)
	for _, line := range strings.Split(stdout, "\n") {
		m := crossAspectLinePat.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		primary, primaryLabel := resolveAspectBody(m[1])
		kind, okK := aspectAbbrev[m[2]]
		secondary, secondaryLabel := resolveAspectBody(m[3])
		if !okK {
			continue
		}
		orbDeg, _ := strconv.Atoi(m[5])
		orbMin, _ := strconv.Atoi(m[6])
		power, _ := strconv.ParseFloat(m[7], 64)
		out = append(out, aspect.CrossAspect{
			Primary:        primary,
			PrimaryLabel:   primaryLabel,
			Secondary:      secondary,
			SecondaryLabel: secondaryLabel,
			Kind:           kind,
			Orb:            aspect.Orb(float64(orbDeg) + float64(orbMin)/60.0),
			Power:          power,
			Applying:       m[4] == applyingMarker,
		})
	}
	return out
}

// isNoDataRow reports whether a matched body row is Astrolog's no-data
// sentinel. All three fields must be exactly zero: the angles carry a zero
// latitude but a real velocity, and the nodes carry a zero latitude and a small
// one — so any looser predicate would start dropping bodies that have
// positions. Measured against this Astrolog: 40 of its 134 objects show the
// signature and every one of the other 94 shows a real position.
func isNoDataRow(m []string) bool {
	return m[2] == "0" && m[3] == "Ari" && m[4] == "00" && // 0Ari00
		m[7] == "0" && m[8] == "00" && // 0:00' latitude
		(m[11] == "+0.000" || m[11] == "-0.000") // zero velocity
}

// unavailableObjects names every body Astrolog reported with the no-data
// sentinel, in the order it printed them, deduplicated. It is what lets the UI
// say which objects it dropped instead of leaving the operator to notice a row
// that never appeared.
func unavailableObjects(stdout string) []string {
	stdout = astrologstyle.StripSGR(stdout)
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(stdout, "\n") {
		m := bodyPositionPat.FindStringSubmatch(line)
		if m == nil || !isNoDataRow(m) {
			continue
		}
		name := strings.TrimSpace(m[1])
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// parsePositions extracts body positions from astrolog's default chart output.
//
// Two kinds of row do NOT become a position, and they are different:
//
//   - A row whose body is not in bodyAbbrev4 is KEPT, carrying Astrolog's own
//     label and BodyUnknown. It has a real position and no typed identity.
//     (This used to be skipped, which is what made a selected Ceres compute and
//     then never appear.)
//   - A row at the no-data sentinel is DROPPED. It has a typed identity or a
//     label but no position, because this Astrolog has no ephemeris file for
//     it. unavailableObjects names those so the perimeter can say so.
//
// A line that does not match the body-position pattern at all is not a body row
// and is skipped.
func parsePositions(stdout string) []aspect.Position {
	var positions []aspect.Position
	stdout = astrologstyle.StripSGR(stdout)
	for _, line := range strings.Split(stdout, "\n") {
		m := bodyPositionPat.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// An abbreviation the enum does not name is KEPT, carrying Astrolog's
		// own label instead of a typed identity. Dropping it is what made the
		// body selector able to add Ceres to a chart that then did not show
		// it. Only the sign is still required — a row whose position cannot
		// be read is not a position.
		body := bodyAbbrev4[m[1]]
		label := ""
		if body == aspect.BodyUnknown {
			label = m[1]
		}
		sign, ok := signAbbrev[m[3]]
		if !ok {
			continue
		}
		deg, _ := strconv.Atoi(m[2])
		min, _ := strconv.Atoi(m[4])
		latDeg, _ := strconv.Atoi(m[7])
		latMin, _ := strconv.Atoi(m[8])
		houseNum, _ := strconv.Atoi(m[9])
		veloc, _ := strconv.ParseFloat(m[11], 64)

		longitude := float64(int(sign-1))*30 + float64(deg) + float64(min)/60.0
		latitude := float64(latDeg) + float64(latMin)/60.0
		if m[6] == "-" {
			latitude = -latitude
		}
		var house aspect.House
		if houseNum >= 1 && houseNum <= 12 {
			house = aspect.House(houseNum)
		}
		// An object this Astrolog has no ephemeris file for comes back as the
		// no-data sentinel: 0Ari00, zero latitude, zero velocity, all three
		// exactly. Astrolog says so on stderr too, but names the FILE
		// ('s136199s.se1'), not the object, so the row itself is the only
		// thing that identifies which body is missing.
		//
		// It is DROPPED rather than shown. A row reading `Eris  Ari 0°00'` is
		// not a body Sextile could not name — it is a body with no position,
		// and an astrologer reads it as Eris at 0° Aries. Operator decision,
		// 2026-09-17: drop the row, and say so once. unavailableObjects
		// collects the names so the perimeter can.
		if isNoDataRow(m) {
			continue
		}
		positions = append(positions, aspect.Position{
			Body:      body,
			Label:     label,
			Longitude: longitude,
			Latitude:  latitude,
			Speed:     veloc,
			House:     house,
		})
	}
	return positions
}

// houseCuspPat matches one house-cusp field from Astrolog's default chart.
// The cusps print in the right-hand column of the same body rows
// parsePositions already reads, so recovering the frame costs no extra
// invocation and no extra switch.
//
//	House cusp  1: 22Leo22
//
// Capture groups: house number (1..12), degree, sign abbreviation, minute.
var houseCuspPat = regexp.MustCompile(`House cusp\s+(\d+):\s+(\d+)([A-Z][a-z]{2})(\d+)`)

// reColumnHouses is the "<Name> Houses" label that ends the positions chart's
// column-header row. The row is matched from its own first column, `Body`: the
// chart's name and place print on the two lines above it, and a label reading
// `Veloc. Campanus Houses` must not be taken for the house system
// (the specification C3, N17, N22).
var reColumnHouses = regexp.MustCompile(`(?m)^\s*Body\s+Locat\..*?Veloc\.\s+(\S.*?) Houses\s*$`)

// parseHousesUsed returns the house system Astrolog reports it USED for a
// positions chart (its own header, not a latitude test), or "" if absent.
func parseHousesUsed(stdout string) string {
	m := reColumnHouses.FindStringSubmatch(astrologstyle.StripSGR(stdout))
	if m == nil {
		return ""
	}
	return m[1]
}

// parseCusps extracts the twelve house cusp longitudes — the chart's house
// frame — from the same stdout parsePositions reads.
//
// The frame is what makes a house number a claim about a particular chart,
// and it is what lets one chart's body be located in another chart's houses.
// Astrolog has always printed it; the per-body house was parsed and the cusps
// that produced it discarded.
//
// Returns the zero Cusps when fewer than twelve are found. A partial frame is
// refused whole: it would place some bodies plausibly and others wrongly, with
// nothing on screen to say which.
func parseCusps(stdout string) aspect.Cusps {
	stdout = astrologstyle.StripSGR(stdout)
	var cusps aspect.Cusps
	seen := 0
	for _, m := range houseCuspPat.FindAllStringSubmatch(stdout, -1) {
		num, _ := strconv.Atoi(m[1])
		if num < 1 || num > 12 {
			continue
		}
		sign, ok := signAbbrev[m[3]]
		if !ok {
			continue
		}
		deg, _ := strconv.Atoi(m[2])
		min, _ := strconv.Atoi(m[4])
		if cusps[num-1] == 0 {
			seen++
		}
		cusps[num-1] = float64(int(sign-1))*30 + float64(deg) + float64(min)/60.0
	}
	if seen != 12 {
		return aspect.Cusps{}
	}
	return cusps
}

// parseAspects extracts aspects from astrolog's -aa output. Lines that do
// not match, or whose aspect kind is not in the system enum, are silently
// skipped. A body the Body enum cannot name is kept as BodyUnknown with its
// printed name in ALabel/BLabel. Each aspect is canonicalised through
// aspect.NewAspect so that (Mars, Venus, Con) and (Venus, Mars, Con)
// collapse to a single record. Applying is Astrolog's `a` marker;
// a plain -a list (+/- orbs) parses with Applying false throughout.
func parseAspects(stdout string) []aspect.Aspect {
	var aspects []aspect.Aspect
	stdout = astrologstyle.StripSGR(stdout)
	for _, line := range strings.Split(stdout, "\n") {
		m := aspectLinePat.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		bodyA, labelA := resolveAspectBody(m[1])
		bodyB, labelB := resolveAspectBody(m[5])
		kind, okK := aspectAbbrev[m[3]]
		if !okK {
			continue
		}
		orbDeg, _ := strconv.Atoi(m[7])
		orbMin, _ := strconv.Atoi(m[8])
		orb := aspect.Orb(float64(orbDeg) + float64(orbMin)/60.0)
		power, _ := strconv.ParseFloat(m[9], 64)
		a := aspect.NewLabelledAspect(bodyA, labelA, bodyB, labelB, kind, orb)
		a.Power = power
		a.Applying = m[6] == applyingMarker
		aspects = append(aspects, a)
	}
	return aspects
}
