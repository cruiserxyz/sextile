package lookup

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// RawReportKind is the report context lookup exposes to the UI. lookup maps it
// to recipe's sealed projection while retaining ownership of ChartSpec
// construction; the UI never assembles Astrolog switches or recipe sources.
type RawReportKind uint8

const (
	RawReportChart RawReportKind = iota + 1
	RawReportBody
	// RawReportAspects is the chart's -aa list. Since an earlier change the Details panel
	// takes that list from the frame it was parsed from (Result.AspectsReport)
	// instead of spawning this; the kind stays for callers that want the
	// report on its own (a CLI, a debug dump), never for the Aspects panel.
	RawReportAspects
)

// RawReportRequest selects a single-chart raw report. Object is required only
// for RawReportBody and rejected for the other kinds.
type RawReportRequest struct {
	Kind   RawReportKind
	Object aspect.Object
	// Label is the chart's name and place as its report header prints them
	// (the specification C1); the zero value is the unnamed chart.
	Label store.ReportLabel
	// Record is the chart file's text for a chart cast from its file
	// (store.AnchorChart.Record); empty for Now.
	Record string
}

func (r RawReportRequest) projection() (recipe.RawReport, error) {
	var p recipe.RawReport
	switch r.Kind {
	case RawReportChart:
		if r.Object != (aspect.Object{}) {
			return p, fmt.Errorf("an object is only valid for a body report")
		}
		p = recipe.ChartRawReport()
	case RawReportBody:
		p = recipe.BodyRawReport(r.Object)
	case RawReportAspects:
		if r.Object != (aspect.Object{}) {
			return p, fmt.Errorf("an object is only valid for a body report")
		}
		p = recipe.AspectsRawReport()
	default:
		return p, fmt.Errorf("unknown report kind %d", int(r.Kind))
	}
	if err := p.Validate(); err != nil {
		return recipe.RawReport{}, err
	}
	return p, nil
}

// ComputeRawReport returns an unparsed, uncolored Astrolog report for one
// moment at one place under lens, together with the command that produced
// it — carried from astrolog unchanged, and present beside the
// error whenever the spawn happened.
func ComputeRawReport(ctx context.Context, when time.Time, place location.PlaceInfo, lens Lens, request RawReportRequest) (report string, command string, err error) {
	if request.Record == "" && !place.HasTimezone() {
		return "", "", fmt.Errorf("lookup: ComputeRawReport: place %q has no IANA timezone", place.Display())
	}
	projection, err := request.projection()
	if err != nil {
		return "", "", fmt.Errorf("lookup: ComputeRawReport: %w", err)
	}
	if request.Kind == RawReportChart {
		// Fenced: Astrolog 8.00's influence chart (-j) reads past its arrays
		// for every fixed star, so it dies or prints power in a wrong sign.
		// See Astrolog NEXT.md BUG 7, and docs/REFERENCE.md, Limits.
		// TestInfluenceChartBugSeven records which stars still end the run on
		// the Astrolog in use; no check for "fixed" holds on every build, so
		// retiring this line is a decision made against that entry, not a test
		// turning red.
		lens = lens.withoutStars()
	}
	spec, err := lens.localSpecAt(At{JD: astrolog.SlotJD(when), Place: place, Label: request.Label, Record: request.Record})
	if err != nil {
		return "", "", fmt.Errorf("lookup: ComputeRawReport: %w", err)
	}
	return astrolog.ComputeRawReport(ctx, spec, projection)
}

// InfluenceLeavesOut are the fixed stars the lens would draw that the chart
// report (Astrolog's influence chart) leaves out, as Astrolog's numbers,
// ascending; nil when none. The large log notes it once.
func (l Lens) InfluenceLeavesOut() []int {
	if !slices.ContainsFunc(l.Registry, func(o aspect.RegistryObject) bool { return o.Star }) {
		return nil
	}
	drawn, err := l.VisibleObjectNumbers()
	if err != nil {
		return nil
	}
	var out []int
	for _, n := range drawn {
		if l.Registry.IsStar(n) {
			out = append(out, n)
		}
	}
	return out
}

// withoutStars is the lens with no fixed star in its object policy, in any of
// the policy's forms. Under the host's default with deltas, a star is taken out
// of Add and put in Remove (a set operation that holds whatever the host's
// default lists). An explicit pick loses its stars; a pick of stars alone falls
// back to the lens's default set, as an empty pick does.
func (l Lens) withoutStars() Lens {
	var stars []int
	for _, o := range l.Registry {
		if o.Star {
			stars = append(stars, o.Number)
		}
	}
	if len(stars) == 0 {
		return l
	}
	p := l.Objects
	if p.Baseline == recipe.Explicit {
		if len(p.Selection) == 0 {
			return l
		}
		kept := make([]int, 0, len(p.Selection))
		for _, n := range p.Selection {
			if !l.Registry.IsStar(n) {
				kept = append(kept, n)
			}
		}
		if len(kept) > 0 {
			p.Selection = kept
			l.Objects = p
			return l
		}
		fallback := Hellenistic.Objects
		fallback.Nodes, fallback.Only = p.Nodes, p.Only
		p = fallback
	}
	add := make([]int, 0, len(p.Add))
	for _, n := range p.Add.Members() {
		if !l.Registry.IsStar(n) {
			add = append(add, n)
		}
	}
	p.Add = recipe.NewObjectSet(add...)
	p.Remove = recipe.NewObjectSet(append(p.Remove.Members(), stars...)...)
	l.Objects = p
	return l
}

// CrossBodyRawReport returns Astrolog's uncolored transit→natal cross-aspect
// list restricted to one object: the same `-rt … -aa` relationship run
// the Aspects panel reads, plus `-RO <n>`, so the report keeps real power,
// real orb and Astrolog's own applying/separating verdict.
//
// The restriction is by OBJECT, not by role: every row where that body is
// either end survives — the natal Sun's transits and the transiting Sun's
// contacts alike. That is what an astrologer asks for when they put the
// cursor on the Sun during a transit reading, and it is Astrolog's own
// semantics for -RO, not a Sextile rule layered on top.
func CrossBodyRawReport(
	ctx context.Context,
	lens Lens,
	natalChart store.AnchorChart,
	transitWhen time.Time,
	transitPlace location.PlaceInfo,
	object aspect.Object,
) (report string, command string, err error) {
	if !transitPlace.HasTimezone() {
		return "", "", fmt.Errorf("lookup: CrossBodyRawReport: place %q has no IANA timezone", transitPlace.Display())
	}
	projection := recipe.BodyRawReport(object)
	if err := projection.Validate(); err != nil {
		return "", "", fmt.Errorf("lookup: CrossBodyRawReport: %w", err)
	}
	spec, err := lens.localComparisonSpecAt(AtChart(natalChart), AtMoment(astrolog.SlotJD(transitWhen), transitPlace, store.ReportLabel{}))
	if err != nil {
		return "", "", fmt.Errorf("lookup: CrossBodyRawReport: %w", err)
	}
	return astrolog.ComputeRawReport(ctx, spec, projection)
}

// SynastryCrossBodyRawReport is CrossBodyRawReport for two ANCHORED charts: the
// same `-RO <n>` restriction over a `-r0 -aa` relationship run (Relation
// Synastry), chart 1 (natalChart, the baseline) first, both staged charts in
// their own local clock.
func SynastryCrossBodyRawReport(
	ctx context.Context,
	lens Lens,
	natalChart store.AnchorChart,
	other store.AnchorChart,
	object aspect.Object,
) (report string, command string, err error) {
	projection := recipe.BodyRawReport(object)
	if err := projection.Validate(); err != nil {
		return "", "", fmt.Errorf("lookup: SynastryCrossBodyRawReport: %w", err)
	}
	spec, err := lens.localSynastrySpecAt(AtChart(natalChart), AtChart(other))
	if err != nil {
		return "", "", fmt.Errorf("lookup: SynastryCrossBodyRawReport: %w", err)
	}
	return astrolog.ComputeRawReport(ctx, spec, projection)
}

// aspectWithRow matches one row of a cross aspect list and captures the name
// Astrolog printed for the SECOND chart's body, the one right of "with". The
// name field is ten cells wide and can abut the dash ("North Node- orb").
var aspectWithRow = regexp.MustCompile(`^\s*\d+:.*\swith\s+[(\[][A-Z][a-z]{2}[)\]]\s+(.+?)\s*-\s+orb:`)

// KeepCursorSide arranges Astrolog's body report; it computes nothing.
// `-RO <n>` keeps every row where the body is either end, so a cross report
// lists the cursor chart's body (chart 2, right of "with") and the base's own
// (chart 1, left) in one power-ordered list. Only the first is what the Aspects
// tab shows for that body, so the rows whose right end is not the body are
// dropped, with their text unchanged. When anything was dropped, the kept rows
// are counted 1..n again (the list's rows are numbered in a row; the tests and
// the Details cursor read it so) and the totals at the end, which described
// the unfiltered list, go with it. The argv, and so the command the Log shows, is untouched.
func KeepCursorSide(report string, o aspect.Object) (shown string, kept, total int) {
	lines := strings.Split(report, "\n")
	keptLines := make([]string, 0, len(lines))
	for _, ln := range lines {
		if m := aspectWithRow.FindStringSubmatch(ln); m != nil {
			total++
			if !strings.EqualFold(m[1], o.Cut(cursorNameWidth)) {
				continue
			}
			kept++
		}
		keptLines = append(keptLines, ln)
	}
	if kept == total {
		return report, kept, total
	}
	// Cut at the totals line ("Sum power: ..."), keeping the blank before it.
	for i, ln := range keptLines {
		if strings.HasPrefix(ln, "Sum power:") {
			keptLines = keptLines[:i]
			break
		}
	}
	n := 0
	for i, ln := range keptLines {
		if loc := aspectRowNumber.FindStringIndex(ln); loc != nil {
			n++
			keptLines[i] = fmt.Sprintf("%3d:", n) + ln[loc[1]:]
		}
	}
	return strings.TrimRight(strings.Join(keptLines, "\n"), "\n") + "\n", kept, total
}

// aspectRowNumber is the "  12:" that opens a numbered row.
var aspectRowNumber = regexp.MustCompile(`^\s*\d+:`)

// cursorNameWidth is the width of the name column right of "with" in an
// aspect list (ten cells; the left column is seven). Object.Cut at this width
// is the name as the list prints the cursor chart's object.
const cursorNameWidth = 10
