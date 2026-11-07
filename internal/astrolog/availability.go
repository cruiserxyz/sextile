package astrolog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// availability.go is the one fact about which objects this Astrolog can draw.
//
// An object whose Swiss Ephemeris file is absent is not an error to Astrolog:
// it prints one `SwissEph file '...' not found` line on stderr, exits 0 and
// places the object at 0 Aries with no motion. Everything downstream (a
// timeline row, an aspect line, a wheel glyph) then looks like data. So the
// answer is read once, from Astrolog itself, and every argv Sextile builds is
// narrowed to it: an object without its file never reaches Astrolog's object
// list. There is no table of which object needs which file here: the probe asks
// Astrolog, object by object, and keeps what it answered.

// Availability is what the probe learned about one data folder.
type Availability struct {
	// Folder identifies the data folder the probe ran under (the environment
	// Astrolog reads its folders from), so a different folder is a different
	// answer.
	Folder string
	// Absent maps an object number to the ephemeris file Astrolog named for it
	// ("" when it placed the object at the no-data position and named no file).
	Absent map[int]string
	// Names maps an object number to the registry's name for it, for every
	// absent object.
	Names map[int]string
	// Said maps an absent object number to the first line Astrolog wrote on
	// stderr when that object was cast alone: its missing-ephemeris line when
	// there is one, else its first other line, control bytes removed. No entry
	// when Astrolog wrote nothing.
	Said map[int]string
	// Default is the main object set the host's own astrolog.as lists, read with
	// no band, so an absent object in it can be switched off by number.
	Default []int
	// DefaultTransit is the same for the transiting side (`-R2 -HO`): the
	// second chart of an `-rt` comparison reads it, and `-R` never reaches it.
	DefaultTransit []int
}

// IsAbsent reports whether object n has no ephemeris file.
func (a Availability) IsAbsent(n int) bool { _, ok := a.Absent[n]; return ok }

// AbsentNumbers is the absent object numbers, ascending.
func (a Availability) AbsentNumbers() []int {
	out := make([]int, 0, len(a.Absent))
	for n := range a.Absent {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// AbsentGroup is the absent objects that one file would bring back.
type AbsentGroup struct {
	File  string
	Names []string
}

// Groups is the absent objects among nums (all of them when nums is nil), one
// group per file, in object-number order of each file's first object.
func (a Availability) Groups(nums []int) []AbsentGroup {
	var keep map[int]bool
	if nums != nil {
		keep = map[int]bool{}
		for _, n := range nums {
			keep[n] = true
		}
	}
	var out []AbsentGroup
	for _, n := range a.AbsentNumbers() {
		if keep != nil && !keep[n] {
			continue
		}
		file := a.Absent[n]
		i := slices.IndexFunc(out, func(g AbsentGroup) bool { return g.File == file })
		if i < 0 {
			out = append(out, AbsentGroup{File: file})
			i = len(out) - 1
		}
		out[i].Names = append(out[i].Names, a.Names[n])
	}
	return out
}

// ErrBodyUnavailable is a search refused because its body has no ephemeris
// file; the call never reaches Astrolog.
var ErrBodyUnavailable = errors.New("astrolog: body without an ephemeris file")

var currentAvailability atomic.Pointer[Availability]

// SetAvailability installs the answer every later argv is narrowed to.
func SetAvailability(a Availability) { currentAvailability.Store(&a) }

// CurrentAvailability is the installed answer; the zero value (nothing known)
// until SetAvailability runs.
func CurrentAvailability() Availability {
	if p := currentAvailability.Load(); p != nil {
		return *p
	}
	return Availability{}
}

// AvailabilityFolder names the data folder a probe would run under: the
// environment values Astrolog and the Swiss Ephemeris take their folders from.
func AvailabilityFolder() string {
	var parts []string
	for _, k := range []string{"SE_EPHE_PATH", "ASTROLOG", "ASTR800", "ASTR"} {
		parts = append(parts, k+"="+os.Getenv(k))
	}
	// The Astrolog that would run: a different binary reads different files.
	exe, _ := exec.LookPath(binary)
	parts = append(parts, "exe="+exe)
	return strings.Join(parts, "|")
}

// probeJD is the moment every availability cast uses: 2000-01-01 12:00 UT.
const probeJD = 2451545.0

// ProbeAvailability asks Astrolog which registry objects it can draw.
//
// One cast switches on every object. It reports at most ONE missing file per
// run, so it only tells which objects came back at the no-data position
// (0 Aries, no latitude, no motion). Each of those is then cast alone, which is
// when Astrolog names its file. An object that comes back without a position is
// absent whether or not a file was named.
func ProbeAvailability(ctx context.Context) (Availability, error) {
	registry, err := ObjectRegistry(ctx)
	if err != nil {
		return Availability{}, fmt.Errorf("astrolog: availability: object registry: %w", err)
	}
	def, err := readMainDefault(ctx)
	if err != nil {
		return Availability{}, fmt.Errorf("astrolog: availability: main set: %w", err)
	}
	defT, err := readTransitDefault(ctx)
	if err != nil {
		return Availability{}, fmt.Errorf("astrolog: availability: transit set: %w", err)
	}
	names := map[int]string{}
	var all []int
	for _, o := range registry {
		names[o.Number] = o.Name
		if o.Number != 0 {
			all = append(all, o.Number)
		}
	}
	sort.Ints(all)

	out, _, _, err := probeCast(ctx, all)
	if err != nil {
		return Availability{}, fmt.Errorf("astrolog: availability: %w", err)
	}
	// Absent means asked for and not returned at a real position, whatever
	// Astrolog says or does not say about it (a stderr line is evidence, not a
	// refusal, and is not needed to find the candidates).
	candidates := probeCandidates(registry, out)

	av := Availability{Folder: AvailabilityFolder(), Absent: map[int]string{}, Said: map[int]string{}, Names: map[int]string{}, Default: def, DefaultTransit: defT}
	for _, n := range candidates {
		one, files, said, err := probeCast(ctx, []int{n})
		if err != nil {
			return Availability{}, fmt.Errorf("astrolog: availability: object %d: %w", n, err)
		}
		// An unparseable one-object result stays "absent with no file": the
		// conservative side, never a false position (declined to refuse it).
		// A failed one-object parse is the same conservative side: absent.
		if rows, err := parsePositions(one, []int{n}, registry); err == nil && len(rows) > 0 {
			continue
		}
		file := ""
		if len(files) > 0 {
			file = files[0]
		}
		av.Absent[n] = file
		if said != "" {
			av.Said[n] = said
		}
		av.Names[n] = names[n]
	}
	return av, nil
}

// probeCandidates are the registry objects the all-objects cast was asked for
// and did not return at a real position: those printed at the no-data position
// and those with no row at all. A row's label is Astrolog's four characters, so
// a label shared by several registry names (Titan, Titania) claims none of them:
// every object with a shared label is probed one by one. Asking about one
// object too many costs a spawn; asking about one too few hides an absent body.
func probeCandidates(registry []aspect.RegistryObject, out string) []int {
	rows := map[string]bool{}
	for _, line := range strings.Split(astrologstyle.StripSGR(out), "\n") {
		if m := bodyPositionPat.FindStringSubmatch(line); m != nil && !isNoDataRow(m) {
			rows[strings.ToLower(strings.TrimSpace(m[1]))] = true
		}
	}
	var cands []int
	for _, o := range registry {
		if o.Number == 0 {
			continue
		}
		name := strings.ToLower(o.Name)
		claimed := false
		for label := range rows {
			if label == "" || !strings.HasPrefix(name, label) {
				continue
			}
			sharing := 0
			for _, r := range registry {
				if r.Number != 0 && strings.HasPrefix(strings.ToLower(r.Name), label) {
					sharing++
				}
			}
			if sharing == 1 {
				claimed = true
				break
			}
		}
		if !claimed {
			cands = append(cands, o.Number)
		}
	}
	sort.Ints(cands)
	return cands
}

// probeCast casts the given objects at probeJD and returns the positions text,
// the missing-file names and the first line Astrolog wrote on stderr; no
// stderr line refuses it (probingCall).
func probeCast(ctx context.Context, nums []int) (string, []string, string, error) {
	spec := recipe.ChartSpec{Source: recipe.MomentAtPlace{JD: probeJD, Lon: 0, Lat: 0}}
	frame, err := emitFrame(spec)
	if err != nil {
		return "", nil, "", err
	}
	jd, err := momentArg(probeJD)
	if err != nil {
		return "", nil, "", err
	}
	argv := []string{"-qj", jd, "-zl", formatLon(0), formatLat(0)}
	argv = append(argv, frame...)
	argv = append(argv, reformPin...)
	argv = append(argv, CurrentClock().Pins()...)
	obj := emitShowList(flagMainShow, nums)
	argv = append(argv, obj...)
	argv = append(argv, "=v0", "-Yw", "0")
	call := newBodyCall(argv, newBand(obj, nil))
	call.Objects = nums
	if err := validateArgv(call.argv); err != nil {
		return "", nil, "", err
	}
	positionsCall, err := call.derive(colorPositionsArgv)
	if err != nil {
		return "", nil, "", err
	}
	var files []string
	var said string
	out, _, err := spawn(ctx, positionsCall.reportingMissing(&files).sayingFirst(&said).probingCall())
	if err != nil {
		return "", nil, "", err
	}
	return astrologstyle.StripSGR(out), files, said, nil
}
