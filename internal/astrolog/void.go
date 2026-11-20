package astrolog

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The void search is ONE Astrolog year search (`-dY 2`, two years) over the
// Moon's events: every row with the Moon, filtered by Astrolog's own
// AstroExpression event filter (`-~d`), over the objects up to Pluto and the
// five aspects Astrolog's void-of-course rule counts (FAspectVoid, extern.h:
// Con, Opp, Squ, Tri, Sex). It exists because the month search misses a void
// whose sign change falls in the next month (Astrolog_8.0 NEXT.md BUG 9) and
// overflows once a month holds too many events (BUG 8). The two years put the
// moment at least six months from either end. Spec: specs/moon-observatory.md,
// slice L.

// ErrVoidRegistry means the registry has no unique Pluto or no unique Moon, so
// the void search cannot say which objects and which Moon to ask for.
var ErrVoidRegistry = errors.New("astrolog: void search: the registry has no unique Moon and Pluto")

// VoidRequest is everything a void search depends on: the moment it must
// cover, the fixed offset the rows are printed in, the zodiac, and the
// registry that names the objects.
type VoidRequest struct {
	Moment   time.Time
	Offset   int
	Houses   recipe.HouseSystem
	Zodiac   recipe.Zodiac
	Registry aspect.Registry
	// AspectRegistry is the binary's aspect list, which reads the rows' aspects.
	AspectRegistry aspect.AspectRegistry
	// LeftOut, when set, collects what the search left out (see LeftOut).
	LeftOut *LeftOut
}

// VoidFirstYear is the first of the search's two years: the moment's year Y-1
// when it falls in January to June, else Y.
func VoidFirstYear(moment time.Time, offset int) int {
	y, m, _ := AstrologCalendar{}.DateOf(moment, offset)
	if m <= time.June {
		return y - 1
	}
	return y
}

// voidAspects are Astrolog's Con, Opp, Squ, Tri and Sex: the aspects
// FAspectVoid counts.
var voidAspects = []int{1, 2, 3, 4, 5}

// voidObjects is every registry number from 1 up to Pluto's, ascending, and the
// Moon's number.
func voidObjects(reg aspect.Registry) (objects []int, moon int, err error) {
	moonObj, okMoon := reg.ByName("Moon")
	pluto, okPluto := reg.ByName("Pluto")
	if !okMoon || !okPluto {
		return nil, 0, ErrVoidRegistry
	}
	for _, o := range reg {
		if o.Number >= 1 && o.Number <= pluto.Number && !slices.Contains(objects, o.Number) {
			objects = append(objects, o.Number)
		}
	}
	slices.Sort(objects)
	return objects, moonObj.Number, nil
}

// VoidCall is the void search as a body call. Order is the timeline's: zone
// first, the mode switch, the frame, the pins, then the band.
func VoidCall(req VoidRequest) (BodyCall, error) {
	objects, moon, err := voidObjects(req.Registry)
	if err != nil {
		return BodyCall{}, err
	}
	argv := []string{"-z", formatSearchZone(req.Offset), "-qy", fmt.Sprint(VoidFirstYear(req.Moment, req.Offset)), "-dY", "2"}
	frame, err := frameBand(req.Houses, req.Zodiac)
	if err != nil {
		return BodyCall{}, err
	}
	argv = append(argv, frame...)
	argv = append(argv, reformPin...)
	argv = append(argv, CurrentClock().Pins()...)
	band := newBand(emitShowList(flagMainShow, objects), emitShowList(flagAspectShow, voidAspects), moonEventFilter(moon))
	call := newBodyCall(append(argv, band.tokens...), band).asSearch()
	call.Registry, call.Aspects, call.Objects = req.Registry, req.AspectRegistry, objects
	if req.LeftOut != nil {
		call = call.reportingLeftOut(&req.LeftOut.Bodies).reportingLeftOutLines(&req.LeftOut.Lines)
	}
	return call, nil
}

// moonEventFilter is Astrolog's `-~d` event filter: the first object is the
// Moon, or the event is an aspect whose second object is the Moon (`@v > 0`
// leaves out another body's sign change whose sign number equals the Moon's
// number). It is the one hook a call sets; ReadVoids refuses output it did not
// apply to, because an AstroExpression that fails does so silently.
func moonEventFilter(moon int) []string {
	return []string{"-~d", fmt.Sprintf("Or Equ @u %d And Gt @v 0 Equ @w %d", moon, moon)}
}

// VoidPeriod is one void of course: the instant of the row Astrolog marks
// `(v/c …)` to the instant of the Moon's next sign change.
type VoidPeriod struct {
	Start, End time.Time
	Row        TimelineRow
}

// MoonPhase is one Sun-Moon row Astrolog labels `(New Moon)`, `(Half Moon)` or
// `(Full Moon)`.
type MoonPhase struct {
	Instant time.Time
	Label   string
	Row     TimelineRow
}

// VoidResult is what one void search yields: the voids, the phases, every
// parsed row (the sky rows' marks come from these) and the command that ran.
type VoidResult struct {
	Voids   []VoidPeriod
	Phases  []MoonPhase
	Rows    []TimelineRow
	Command string
}

var phaseLabel = regexp.MustCompile(`\((New Moon|Half Moon|Full Moon)\)`)

// ReadVoids reads the output of one void search printed in offset seconds east
// of UTC whose first year is firstYear. A row without the Moon refuses the whole
// output. A void with no later Moon sign change in the output is dropped, not an
// error.
//
// Astrolog can end a long search with skyOverflowLine, a false alarm printed
// after the last row (Astrolog_8.0 NEXT.md BUG 8). It is read only there: as the
// last non-empty line, after a row on 31 December of the window's second year.
// Anywhere else the line refuses the output.
func ReadVoids(out string, offset, firstYear int, reg aspect.Registry, aspects aspect.AspectRegistry) (VoidResult, error) {
	moon, ok := reg.ByName("Moon")
	if !ok {
		return VoidResult{}, ErrVoidRegistry
	}
	lines := strings.Split(strings.TrimRight(out, " \t\r\n"), "\n")
	falseAlarm := lines[len(lines)-1] == skyOverflowLine
	if falseAlarm {
		out = strings.Join(lines[:len(lines)-1], "\n")
	}
	rows, err := ParseTimeline(out, time.FixedZone("", offset), reg, aspects)
	if err != nil {
		return VoidResult{}, err
	}
	if falseAlarm {
		endsTheWindow := false
		if n := len(rows); n > 0 {
			y, m, d := AstrologCalendar{}.DateOf(rows[n-1].Instant, offset)
			endsTheWindow = y == firstYear+1 && m == time.December && d == 31
		}
		if !endsTheWindow {
			return VoidResult{}, &TimelineLineError{Line: skyOverflowLine, Reason: "not the last day's false alarm"}
		}
	}
	for _, row := range rows {
		if row.Object.Number != moon.Number && row.OtherObject.Number != moon.Number {
			return VoidResult{}, fmt.Errorf("astrolog: void search: the Moon filter did not apply: %q", row.Line)
		}
	}
	res := VoidResult{Rows: rows}
	for i, row := range rows {
		if voidMark.MatchString(row.Line) {
			for _, later := range rows[i+1:] {
				if later.Kind == TimelineKindIngress && later.Object.Number == moon.Number {
					res.Voids = append(res.Voids, VoidPeriod{Start: row.Instant, End: later.Instant, Row: row})
					break
				}
			}
		}
		if row.Kind == TimelineKindAspect {
			if m := phaseLabel.FindStringSubmatch(row.Line); m != nil {
				res.Phases = append(res.Phases, MoonPhase{Instant: row.Instant, Label: m[1], Row: row})
			}
		}
	}
	return res, nil
}

// VoidAt reports whether t falls in a void, start inclusive, end exclusive.
func (r VoidResult) VoidAt(t time.Time) bool {
	for _, v := range r.Voids {
		if !t.Before(v.Start) && t.Before(v.End) {
			return true
		}
	}
	return false
}

// VoidSource answers a void search: the result, whether a process was spawned
// for it (false for a remembered answer), and the error.
type VoidSource func(ctx context.Context, req VoidRequest) (res VoidResult, spawned bool, err error)

// RunVoidSearch runs the void search through search (RunTimelineSearch when
// nil) and reads it. Every row carries the sky source and the command.
func RunVoidSearch(ctx context.Context, req VoidRequest, search TimelineSearcher) (VoidResult, error) {
	if err := requireReadableAspects(req.AspectRegistry); err != nil {
		return VoidResult{}, fmt.Errorf("astrolog: RunVoidSearch: %w", err)
	}
	if search == nil {
		search = RunTimelineSearch
	}
	call, err := VoidCall(req)
	if err != nil {
		return VoidResult{}, err
	}
	out, command, err := search(ctx, call)
	if err != nil {
		return VoidResult{Command: command}, err
	}
	res, err := ReadVoids(out, req.Offset, VoidFirstYear(req.Moment, req.Offset), req.Registry, req.AspectRegistry)
	if err != nil {
		return VoidResult{Command: command}, err
	}
	res.Command = command
	stamp := func(r *TimelineRow) { r.Source, r.Command = TimelineSky, command }
	for i := range res.Rows {
		stamp(&res.Rows[i])
	}
	for i := range res.Voids {
		stamp(&res.Voids[i].Row)
	}
	for i := range res.Phases {
		stamp(&res.Phases[i].Row)
	}
	return res, nil
}

// VoidKey names a void search for the memory that holds its answer: the first
// year, the offset, the zodiac, the clock pins and the registry's identity.
type VoidKey struct {
	FirstYear, Offset int
	Houses            recipe.HouseSystem
	Zodiac            recipe.Zodiac
	Clock             Clock
	Registry          string
}

// VoidKeyFor is the key of a request.
func VoidKeyFor(req VoidRequest) VoidKey {
	entries := slices.Clone(req.Registry)
	slices.SortFunc(entries, func(a, b aspect.RegistryObject) int {
		if a.Number != b.Number {
			return a.Number - b.Number
		}
		return strings.Compare(a.Name, b.Name)
	})
	ids := make([]string, len(entries))
	for i, o := range entries {
		ids[i] = fmt.Sprintf("%d:%s", o.Number, o.Name)
	}
	return VoidKey{
		FirstYear: VoidFirstYear(req.Moment, req.Offset),
		Offset:    req.Offset,
		Houses:    req.Houses,
		Zodiac:    req.Zodiac,
		Clock:     CurrentClock(),
		Registry:  strings.Join(ids, "\n"),
	}
}
