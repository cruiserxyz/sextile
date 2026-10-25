package astrolog

// the specification: the frame band pins its own zodiac, plane,
// latitude/declination column and zodiac display format, so a host astrolog.as
// cannot move what Sextile labels `Tr`. The live tests copy the Astrolog data
// directory twice - once untouched, once with a hostile astrolog.as - and
// require every Sextile call to answer the same under both.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// frameTokens is the band the spec fixes after `-c <code>`.
func frameTokens(z recipe.Zodiac) []string {
	if z == recipe.Sidereal {
		return []string{"=s", "0", "_Ys", "_sr0", ":sz"}
	}
	return []string{"_s", "_Ys", "_sr0", ":sz"}
}

func wantFrame(code string, z recipe.Zodiac) []string {
	return append([]string{"-c", code}, frameTokens(z)...)
}

// hostileLines is every astrolog.as line the spec names (C3), appended last so
// each overrides whatever the stock file sets. `=Yp` is the polar-flip switch
// that is measured, not assumed, to be irrelevant to Sextile's ephemeris path.
var hostileLines = []string{"=s", ":s Lahiri", "=Ys", "=sr0", ":sd", "-c Koch", "=Yp"}

// originalDataDir is the data directory as the process started, read once at
// package init: a test that has already pointed ASTROLOG at a copy must not
// make the next "unmodified" copy a copy of that one.
var originalDataDir = astrodir.Dir()

// dataDirWith copies the Astrolog data directory into a temp dir and appends
// the given lines to its astrolog.as. No lines gives an unmodified copy.
func dataDirWith(t *testing.T, lines ...string) string {
	t.Helper()
	astrodir.Need(t)
	dir := t.TempDir()
	copyDir(t, originalDataDir, dir)
	if len(lines) == 0 {
		return dir
	}
	as := filepath.Join(dir, "astrolog.as")
	b, err := os.ReadFile(as)
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, []byte("\n"+strings.Join(lines, "\n")+"\n")...)
	if err := os.WriteFile(as, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// useDataDir points Astrolog at dir for the rest of the test. ASTR800 and
// ASTR would outrank ASTROLOG, and a cwd astrolog.as would shadow both, so the
// first is unset and the working directory is an empty temp dir.
func useDataDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("ASTROLOG", dir)
	t.Setenv("ASTR800", os.Getenv("ASTR800"))
	t.Setenv("ASTR", os.Getenv("ASTR"))
	os.Unsetenv("ASTR800")
	os.Unsetenv("ASTR")
	t.Chdir(t.TempDir())
}

func needAstrolog(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
}

// underBoth runs f once against the unmodified data directory and once against
// the hostile one, and returns both answers.
func underBoth[T any](t *testing.T, f func() T) (clean, hostile T) {
	t.Helper()
	cleanDir, hostileDir := dataDirWith(t), dataDirWith(t, hostileLines...)
	useDataDir(t, cleanDir)
	clean = f()
	useDataDir(t, hostileDir)
	hostile = f()
	return clean, hostile
}

// castResult is what Sextile parses from a chart: every Position field and the
// twelve cusps.
type castResult struct {
	Positions []aspect.Position
	Cusps     aspect.Cusps
}

func (c castResult) valid(t *testing.T, label string) {
	t.Helper()
	if len(c.Positions) < 10 {
		t.Fatalf("%s: parsed %d positions, want at least 10 (two empty parses must never compare equal)", label, len(c.Positions))
	}
	zero := 0
	for _, v := range c.Cusps {
		if v < 0 || v >= 360 {
			t.Fatalf("%s: cusp %v out of range: %v", label, v, c.Cusps)
		}
		if v == 0 {
			zero++
		}
	}
	if zero > 1 {
		t.Fatalf("%s: cusps not parsed (%v)", label, c.Cusps)
	}
}

func fixtureSpec(t *testing.T, src recipe.Source, houses recipe.HouseSystem, z recipe.Zodiac) recipe.ChartSpec {
	t.Helper()
	spec, err := recipe.NewChartSpec(src, houses, z, recipe.ObjectPolicy{Baseline: recipe.AstrologDefault})
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	return spec
}

var (
	fixLondon = recipe.MomentAtPlace{JD: 2450000.5, Lon: 0, Lat: 51.5}
	fixTromso = recipe.MomentAtPlace{JD: 2451545.25, Lon: 18.95, Lat: 69.6}
	fixOslo   = recipe.MomentAtPlace{JD: 2447000.75, Lon: 10.75, Lat: 59.91}
)

func castChart(t *testing.T, spec recipe.ChartSpec) castResult {
	t.Helper()
	out, err := ComputeChartPositionsOnly(context.Background(), spec)
	if err != nil {
		t.Fatalf("ComputeChartPositionsOnly: %v", err)
	}
	return castResult{out.Positions, out.Cusps}
}

// castStaged casts one staged .as file the way the relationship reads it: the
// file plus the spec's own frame band.
func castStaged(t *testing.T, spec recipe.ChartSpec, content string) castResult {
	t.Helper()
	path := filepath.Join(t.TempDir(), "staged.as")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	frame, err := emitFrame(spec)
	if err != nil {
		t.Fatal(err)
	}
	argv := append(append([]string{"-i", path}, frame...), "-v0", "-k")
	out, _, err := spawn(context.Background(), testCall(argv))
	if err != nil {
		t.Fatalf("spawn: %v (%.300s)", err, out)
	}
	return castResult{parsePositions(out), parseCusps(out)}
}

// runInvocation stages and spawns one compiled invocation and returns the text
// Astrolog printed.
func runInvocation(t *testing.T, inv Invocation) string {
	t.Helper()
	call, _, cleanup, err := stageCall(inv)
	defer cleanup()
	if err != nil {
		t.Fatalf("stageCall: %v", err)
	}
	out, _, err := spawn(context.Background(), call)
	if err != nil {
		t.Fatalf("spawn: %v (%.300s)", err, out)
	}
	return out
}

func compileOne(t *testing.T, spec recipe.ChartSpec, p recipe.Projection) Invocation {
	t.Helper()
	plan, err := Compile(spec, p)
	if err != nil {
		t.Fatalf("Compile %s: %v", p.Label(), err)
	}
	return plan.Invocations[0]
}

type frameCase struct {
	name    string
	houses  recipe.HouseSystem
	zodiac  recipe.Zodiac
	code    string
	zodName string
}

func frameCases() []frameCase {
	var cs []frameCase
	for _, h := range []struct {
		h    recipe.HouseSystem
		code string
		n    string
	}{{recipe.WholeSign, "14", "whole"}, {recipe.Placidus, "0", "placidus"}} {
		for _, z := range []struct {
			z recipe.Zodiac
			n string
		}{{recipe.Tropical, "tropical"}, {recipe.Sidereal, "sidereal"}} {
			cs = append(cs, frameCase{h.n + "/" + z.n, h.h, z.z, h.code, z.n})
		}
	}
	return cs
}

// C1: the band begins -c <code> then the pinned zodiac and plane; it is the
// WHOLE band C3 shows necessary (the plane, the latitude column and the
// display format ride with it) and no compiled argv carries a bare -s.
func TestFrameBandPinsZodiac(t *testing.T) {
	for _, c := range frameCases() {
		t.Run(c.name, func(t *testing.T) {
			spec := fixtureSpec(t, fixLondon, c.houses, c.zodiac)
			got, err := emitFrame(spec)
			if err != nil {
				t.Fatal(err)
			}
			if want := wantFrame(c.code, c.zodiac); !slices.Equal(got, want) {
				t.Fatalf("emitFrame = %q, want %q", got, want)
			}
			// Every compile path carries exactly that band, contiguous.
			sources := map[string]recipe.Source{
				"single":     fixLondon,
				"local":      recipe.LocalMomentAtPlace{JD: 2450000.5, Lon: 0, Lat: 51.5, TZ: "Europe/London"},
				"comparison": recipe.Comparison{Primary: fixLondon, Secondary: fixTromso},
				"transit":    recipe.Comparison{Primary: fixLondon, Secondary: fixTromso, Relation: recipe.TransitToNatal},
				"multiwheel": recipe.MultiWheel{Rings: []recipe.Source{fixLondon, fixTromso, fixOslo}},
			}
			for name, src := range sources {
				s := fixtureSpec(t, src, c.houses, c.zodiac)
				var projs []recipe.Projection
				switch name {
				case "single":
					projs = []recipe.Projection{recipe.Positions{}, recipe.Aspects{}, recipe.ChartRawReport(), recipe.WheelImage{Render: recipe.DefaultWheelRender()}}
				case "local":
					projs = []recipe.Projection{recipe.ChartRawReport()}
				case "multiwheel":
					projs = []recipe.Projection{recipe.WheelImage{Render: recipe.DefaultWheelRender()}}
				default:
					projs = []recipe.Projection{recipe.Aspects{}, recipe.AspectsRawReport(), recipe.WheelImage{Render: recipe.DefaultWheelRender()}}
				}
				for _, p := range projs {
					plan, err := Compile(s, p)
					if err != nil {
						t.Fatalf("%s/%s: %v", name, p.Label(), err)
					}
					for _, inv := range plan.Invocations {
						if !containsRun(inv.Argv, wantFrame(c.code, c.zodiac)) {
							t.Errorf("%s/%s: argv has no contiguous %q: %q", name, p.Label(), wantFrame(c.code, c.zodiac), inv.Argv)
						}
						if slices.Contains(inv.Argv, "-s") {
							t.Errorf("%s/%s: bare -s in argv: %q", name, p.Label(), inv.Argv)
						}
						// Every staged chart carries the spec's own zodiac, not the zero value's.
						for i, ti := range inv.TempInputs {
							if want := zodiacLines(c.zodiac); !strings.Contains(ti.Content, "-c "+c.code+"\n"+want) {
								t.Errorf("%s/%s: staged chart %d lacks the %v pin %q:\n%s", name, p.Label(), i, c.zodiac, want, ti.Content)
							}
						}
					}
				}
			}
		})
	}
}

// C2: a staged chart file carries the same pinned lines, never a bare -s.
func TestStagedChartFilePinsZodiac(t *testing.T) {
	local := recipe.LocalMomentAtPlace{JD: 2450000.5, Lon: 0, Lat: 51.5, TZ: "Europe/London"}
	for _, c := range frameCases() {
		t.Run(c.name, func(t *testing.T) {
			utc, err := FormatChartFile(fixLondon, c.houses, c.zodiac)
			if err != nil {
				t.Fatal(err)
			}
			loc, err := FormatLocalChartFile(local, c.houses, c.zodiac)
			if err != nil {
				t.Fatal(err)
			}
			for name, content := range map[string]string{"FormatChartFile": utc, "FormatLocalChartFile": loc} {
				lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
				var pins []string
				for _, l := range lines {
					switch strings.Fields(l)[0] {
					case "_s", "=s", "_Ys", "=Ys":
						pins = append(pins, l)
					}
					if strings.Fields(l)[0] == "-s" {
						t.Errorf("%s: bare -s line %q", name, l)
					}
				}
				want := []string{"_s", "_Ys"}
				if c.zodiac == recipe.Sidereal {
					want = []string{"=s 0", "_Ys"}
				}
				if !slices.Equal(pins, want) {
					t.Errorf("%s: zodiac/plane lines = %q, want %q\n%s", name, pins, want, content)
				}
				if !strings.Contains(content, "-c "+c.code+"\n") {
					t.Errorf("%s: lost its -c %s line:\n%s", name, c.code, content)
				}
			}
		})
	}
}

// C3 (and C4's control): the hostile astrolog.as cannot move a single chart,
// a comparison, or a staged chart of either, in either zodiac.
func TestHostAstrologAsCannotMoveTheFrame(t *testing.T) {
	needAstrolog(t)

	// C4: the fixture is the file Astrolog loaded. With NO frame band at all
	// the hostile copy moves the Sun.
	t.Run("control", func(t *testing.T) {
		raw := func() string {
			out, _, err := spawn(context.Background(), testCall([]string{"-qj", "2450000.500000", "-zl", "0E", "51:30N", "-v0"}))
			if err != nil {
				t.Fatal(err)
			}
			return out
		}
		clean, hostile := underBoth(t, raw)
		sunClean, sunHostile := sunRow(t, clean), sunRow(t, hostile)
		if sunClean == sunHostile {
			t.Fatalf("the hostile astrolog.as did not move the Sun on a raw call (%q): the fixture is not the file Astrolog loaded", sunClean)
		}
	})

	// Pass 6: each neutraliser the band carries is there because the host
	// switch it neutralises has an observable effect on a raw call. A switch
	// with no effect proves nothing, so it is not in the band.
	t.Run("neutralisers", func(t *testing.T) {
		rawCall := func(frame ...string) []string {
			return append([]string{"-qj", "2450000.500000", "-zl", "0E", "51:30N"}, append(frame, "-v0", "-k")...)
		}
		for _, n := range []struct {
			name, line string
			frame      []string
		}{
			{"=s", "=s", nil},
			{":s", ":s Lahiri", []string{"=s"}},
			{"=Ys", "=Ys", []string{"=s", "0"}},
			{"=sr0", "=sr0", nil},
			{":sd", ":sd", nil},
			{"-c", "-c Koch", nil},
		} {
			t.Run(n.name, func(t *testing.T) {
				argv := rawCall(n.frame...)
				// The effect is judged on what Sextile PARSES (every Position
				// field and the cusps), not on the printed text: a switch that
				// only moved a header would prove nothing about the frame.
				run := func() castResult {
					out, _, err := spawn(context.Background(), testCall(argv))
					if err != nil {
						t.Fatal(err)
					}
					return castResult{parsePositions(out), parseCusps(out)}
				}
				cleanDir, oneDir := dataDirWith(t), dataDirWith(t, n.line)
				useDataDir(t, cleanDir)
				clean := run()
				clean.valid(t, "clean raw call")
				useDataDir(t, oneDir)
				if one := run(); reflect.DeepEqual(one, clean) {
					t.Errorf("host line %q changes no parsed value on a raw call %q: its neutraliser is unproven", n.line, argv)
				}
			})
		}
		// =Yp is measured irrelevant: the polar flip never reaches Sextile's
		// ephemeris path, so the band carries no _Yp. If a future Astrolog
		// makes it matter this fails and the band must gain _Yp.
		t.Run("=Yp-irrelevant", func(t *testing.T) {
			cleanDir, ypDir := dataDirWith(t), dataDirWith(t, "=Yp")
			for _, lat := range []string{"51:30N", "69:36N", "80:00N"} {
				for _, jd := range []string{"2450000.5", "2450000.9", "2450001.3"} {
					for _, code := range []string{"0", "14"} {
						argv := []string{"-qj", jd, "-zl", "0E", lat, "-c", code, "-v0"}
						useDataDir(t, cleanDir)
						a, _, _ := spawn(context.Background(), testCall(argv))
						useDataDir(t, ypDir)
						b, _, _ := spawn(context.Background(), testCall(argv))
						if a == "" || a != b {
							t.Errorf("=Yp changes the chart at %s jd %s -c %s: emit _Yp in the frame band", lat, jd, code)
						}
					}
				}
			}
		})
	})

	for _, c := range frameCases() {
		t.Run("single/"+c.name, func(t *testing.T) {
			for _, src := range []recipe.MomentAtPlace{fixLondon, fixTromso} {
				spec := fixtureSpec(t, src, c.houses, c.zodiac)
				clean, hostile := underBoth(t, func() castResult { return castChart(t, spec) })
				clean.valid(t, "clean")
				hostile.valid(t, "hostile")
				if !reflect.DeepEqual(clean, hostile) {
					t.Errorf("lat %v: hostile astrolog.as moved the chart\nclean   %+v\nhostile %+v", src.Lat, clean, hostile)
				}
			}
		})
		t.Run("comparison/"+c.name, func(t *testing.T) {
			// The second chart sits at 69.6N, where the house systems are most
			// fragile.
			spec := fixtureSpec(t, recipe.Comparison{Primary: fixLondon, Secondary: fixTromso}, c.houses, c.zodiac)
			inv := compileOne(t, spec, recipe.Aspects{})
			if !slices.Contains(inv.Argv, "-r0") {
				t.Fatalf("not a -r0 relationship: %q", inv.Argv)
			}
			clean, hostile := underBoth(t, func() string { return runInvocation(t, inv) })
			if !strings.Contains(clean, "Astrolog") || len(clean) < 200 {
				t.Fatalf("relationship printed nothing usable:\n%.300s", clean)
			}
			if clean != hostile {
				t.Errorf("hostile astrolog.as moved the executed -r0 output\nclean:\n%.600s\nhostile:\n%.600s", clean, hostile)
			}
			for i, ti := range inv.TempInputs {
				c0, h0 := underBoth(t, func() castResult { return castStaged(t, spec, ti.Content) })
				c0.valid(t, "clean staged")
				h0.valid(t, "hostile staged")
				if !reflect.DeepEqual(c0, h0) {
					t.Errorf("staged chart %d: hostile astrolog.as moved it\nclean   %+v\nhostile %+v", i, c0, h0)
				}
			}
		})
	}
}

func sunRow(t *testing.T, out string) string {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "Sun ") {
			if i := strings.Index(l, "-  House"); i > 0 {
				l = l[:i]
			}
			return l
		}
	}
	t.Fatalf("no Sun row in:\n%.400s", out)
	return ""
}

// Build note pass 5: every ring of a multi-wheel and the single wheel cast
// draw the same picture under both data directories.
func TestHostAstrologAsCannotMoveMultiWheel(t *testing.T) {
	needAstrolog(t)
	for _, c := range frameCases() {
		t.Run(c.name, func(t *testing.T) {
			spec := fixtureSpec(t, recipe.MultiWheel{Rings: []recipe.Source{fixLondon, fixTromso, fixOslo}}, c.houses, c.zodiac)
			clean, hostile := underBoth(t, func() string {
				svg, err := RenderWheelImage(context.Background(), spec, recipe.DefaultWheelRender())
				if err != nil {
					t.Fatalf("RenderWheelImage: %v", err)
				}
				return string(svg)
			})
			if !strings.Contains(clean, "<svg") || len(clean) < 1000 {
				t.Fatalf("no wheel drawn: %.200s", clean)
			}
			if clean != hostile {
				t.Errorf("hostile astrolog.as moved the multi-wheel (%d vs %d bytes)", len(clean), len(hostile))
			}
		})
	}
}

func TestHostAstrologAsCannotMoveWheel(t *testing.T) {
	needAstrolog(t)
	for _, c := range frameCases() {
		t.Run(c.name, func(t *testing.T) {
			spec := fixtureSpec(t, fixTromso, c.houses, c.zodiac)
			clean, hostile := underBoth(t, func() string {
				svg, err := RenderWheelImage(context.Background(), spec, recipe.DefaultWheelRender())
				if err != nil {
					t.Fatalf("RenderWheelImage: %v", err)
				}
				return string(svg)
			})
			if !strings.Contains(clean, "<svg") || len(clean) < 1000 {
				t.Fatalf("no wheel drawn: %.200s", clean)
			}
			if clean != hostile {
				t.Errorf("hostile astrolog.as moved the wheel (%d vs %d bytes)", len(clean), len(hostile))
			}
		})
	}
}

// The local-moment raw report goes through LocalMomentAtPlace's own prefix.
func TestLocalRawReportUsesPinnedFrame(t *testing.T) {
	needAstrolog(t)
	for _, c := range frameCases() {
		t.Run(c.name, func(t *testing.T) {
			src := recipe.LocalMomentAtPlace{JD: 2450000.5, Lon: 0, Lat: 51.5, TZ: "Europe/London"}
			spec := fixtureSpec(t, src, c.houses, c.zodiac)
			inv := compileOne(t, spec, recipe.ChartRawReport())
			if !containsRun(inv.Argv, wantFrame(c.code, c.zodiac)) {
				t.Fatalf("local raw report argv lacks the band: %q", inv.Argv)
			}
			clean, hostile := underBoth(t, func() string {
				out, _, err := ComputeRawReport(context.Background(), spec, recipe.ChartRawReport())
				if err != nil {
					t.Fatalf("ComputeRawReport: %v", err)
				}
				return out
			})
			if !strings.Contains(clean, "Sun") {
				t.Fatalf("report has no Sun row:\n%.300s", clean)
			}
			if clean != hostile {
				t.Errorf("hostile astrolog.as moved the local raw report\nclean:\n%.500s\nhostile:\n%.500s", clean, hostile)
			}
		})
	}
}

// The ephemeris probe and the relationship carry the same pinned frame.
func TestComparisonProbeUsesPinnedFrame(t *testing.T) {
	needAstrolog(t)
	for _, c := range frameCases() {
		t.Run(c.name, func(t *testing.T) {
			spec := fixtureSpec(t, recipe.Comparison{Primary: fixLondon, Secondary: fixTromso}, c.houses, c.zodiac)
			want := formatCommandTokens(wantFrame(c.code, c.zodiac))
			type answer struct {
				Aspects  []aspect.CrossAspect
				Commands []string
			}
			run := func() answer {
				out, err := ComputeCrossAspects(context.Background(), spec)
				if err != nil {
					t.Fatalf("ComputeCrossAspects: %v", err)
				}
				return answer{out.Aspects, out.Commands}
			}
			clean, hostile := underBoth(t, run)
			if len(hostile.Commands) != 2 {
				t.Fatalf("want probe + relationship (2 commands), got %q", hostile.Commands)
			}
			for i, cmd := range hostile.Commands {
				if !strings.Contains(cmd, want) {
					t.Errorf("command %d lacks the pinned frame %q:\n%s", i, want, cmd)
				}
			}
			// The aspects themselves, not their count: a probe that let the
			// host move one aspect for another would keep the count.
			if len(clean.Aspects) == 0 || !reflect.DeepEqual(clean.Aspects, hostile.Aspects) {
				t.Errorf("cross-aspects differ (clean %d, hostile %d):\nclean   %+v\nhostile %+v", len(clean.Aspects), len(hostile.Aspects), clean.Aspects, hostile.Aspects)
			}
		})
	}
}

// formatCommandTokens renders tokens the way a recorded command quotes them.
func formatCommandTokens(toks []string) string {
	parts := make([]string, len(toks))
	for i, tk := range toks {
		parts[i] = shellQuote(tk)
	}
	return strings.Join(parts, " ")
}

// The transit-to-natal relationship runs as -rt; its executed output is the
// same under both directories.
func TestHostAstrologAsCannotMoveTransitRelationship(t *testing.T) {
	needAstrolog(t)
	for _, c := range frameCases() {
		t.Run(c.name, func(t *testing.T) {
			spec := fixtureSpec(t, recipe.Comparison{Primary: fixLondon, Secondary: fixTromso, Relation: recipe.TransitToNatal}, c.houses, c.zodiac)
			inv := compileOne(t, spec, recipe.Aspects{})
			if inv.Argv[0] != "-rt" {
				t.Fatalf("transit relationship argv[0] = %q, want -rt", inv.Argv[0])
			}
			clean, hostile := underBoth(t, func() string { return runInvocation(t, inv) })
			if len(clean) < 200 {
				t.Fatalf("-rt printed nothing usable:\n%.300s", clean)
			}
			if clean != hostile {
				t.Errorf("hostile astrolog.as moved the executed -rt output\nclean:\n%.500s\nhostile:\n%.500s", clean, hostile)
			}
		})
	}
}

// The raw cross-chart Details report (aspects and a body over -r0).
func TestCrossDetailsReportUsesPinnedFrame(t *testing.T) {
	needAstrolog(t)
	for _, c := range frameCases() {
		t.Run(c.name, func(t *testing.T) {
			spec := fixtureSpec(t, recipe.Comparison{Primary: fixLondon, Secondary: fixTromso}, c.houses, c.zodiac)
			for _, rep := range []recipe.RawReport{recipe.AspectsRawReport(), recipe.BodyRawReport(aspect.Moon)} {
				inv := compileOne(t, spec, rep)
				if !containsRun(inv.Argv, wantFrame(c.code, c.zodiac)) {
					t.Fatalf("%s argv lacks the band: %q", rep.Label(), inv.Argv)
				}
				clean, hostile := underBoth(t, func() string {
					out, _, err := ComputeRawReport(context.Background(), spec, rep)
					if err != nil {
						t.Fatalf("ComputeRawReport: %v", err)
					}
					return out
				})
				if len(clean) < 100 {
					t.Fatalf("%s printed nothing usable:\n%.300s", rep.Label(), clean)
				}
				if clean != hostile {
					t.Errorf("%s: hostile astrolog.as moved the report\nclean:\n%.500s\nhostile:\n%.500s", rep.Label(), clean, hostile)
				}
			}
		})
	}
}

// C5: the timeline searches carry the same pinned frame, after the mode
// switch's month and year and before the chooser band, with -z still first.
func TestTimelineCallCarriesFrameBand(t *testing.T) {
	for _, c := range frameCases() {
		for _, src := range []TimelineSource{TimelineSky, TimelineTransit, TimelineReturn} {
			t.Run(c.name+"/"+src.String(), func(t *testing.T) {
				req := bandReq(t, recipe.ObjectPolicy{Baseline: recipe.AstrologDefault}, recipe.AspectPolicy{})
				req.Houses, req.Zodiac = c.houses, c.zodiac
				call, err := TimelineCall(src, TimelineSegments(2026, time.October, req.Zone)[0], req)
				if err != nil {
					t.Fatal(err)
				}
				argv := call.Argv()
				if argv[0] != "-z" {
					t.Errorf("-z must lead: %q", argv)
				}
				frame := wantFrame(c.code, c.zodiac)
				at := -1
				for i := 0; i+len(frame) <= len(argv); i++ {
					if slices.Equal(argv[i:i+len(frame)], frame) {
						at = i
					}
				}
				if at < 0 {
					t.Fatalf("no frame band %q in %q", frame, argv)
				}
				mode := slices.IndexFunc(argv, func(s string) bool { return s == "-dm" || s == "-t" || s == "-tr" })
				if mode < 0 || at < mode {
					t.Errorf("frame band at %d precedes the mode switch at %d: %q", at, mode, argv)
				}
				if slices.Contains(argv, "-s") {
					t.Errorf("bare -s in %q", argv)
				}
			})
		}
	}
	// A zero-value request is Placidus tropical, stated rather than assumed.
	req := bandReq(t, recipe.ObjectPolicy{Baseline: recipe.AstrologDefault}, recipe.AspectPolicy{})
	call, err := TimelineCall(TimelineSky, TimelineSegments(2026, time.October, req.Zone)[0], req)
	if err != nil {
		t.Fatal(err)
	}
	if !containsRun(call.Argv(), wantFrame("0", recipe.Tropical)) {
		t.Errorf("zero-value request: want Placidus tropical band, got %q", call.Argv())
	}
}

// C5 live: -dm (no natal), -t and -tr under the hostile directory answer as
// under the unmodified one; each parses at least one row, and a raw call
// without the pin shows an observable difference.
func TestHostAstrologAsCannotMoveTimeline(t *testing.T) {
	needAstrolog(t)
	zone := time.FixedZone("UTC+1", 3600)
	for _, c := range frameCases() {
		for _, src := range []TimelineSource{TimelineSky, TimelineTransit, TimelineReturn} {
			t.Run(c.name+"/"+src.String(), func(t *testing.T) {
				req := TimelineRequest{Year: 2026, Month: time.October, Zone: zone, Houses: c.houses, Zodiac: c.zodiac, Defaults: measuredSets}
				if src != TimelineSky {
					req.Natal = liveNatal
				}
				type answer struct {
					Lines    []string
					Commands []string
				}
				run := func() answer {
					rows, cmds, err := ComputeTimeline(context.Background(), req, []TimelineSource{src}, nil)
					if err != nil {
						t.Fatalf("ComputeTimeline: %v", err)
					}
					var a answer
					for _, r := range rows {
						a.Lines = append(a.Lines, r.Instant.UTC().Format(time.RFC3339)+" "+r.Line)
					}
					a.Commands = cmds
					return a
				}
				clean, hostile := underBoth(t, run)
				if len(clean.Lines) < 1 || len(hostile.Lines) < 1 {
					t.Fatalf("a search parsed no rows (clean %d, hostile %d): two empty parses must never compare equal", len(clean.Lines), len(hostile.Lines))
				}
				if !reflect.DeepEqual(clean, hostile) {
					t.Errorf("hostile astrolog.as moved the %s search\nclean   %q\nhostile %q", src, clean.Lines[:min(3, len(clean.Lines))], hostile.Lines[:min(3, len(hostile.Lines))])
				}
				// Control: the same call with its frame band cut out is moved by
				// the hostile file.
				seg := TimelineSegments(2026, time.October, zone)[0]
				call, err := TimelineCall(src, seg, req)
				if err != nil {
					t.Fatal(err)
				}
				frame := wantFrame(c.code, c.zodiac)
				argv := call.Argv()
				cut := -1
				for i := 0; i+len(frame) <= len(argv); i++ {
					if slices.Equal(argv[i:i+len(frame)], frame) {
						cut = i
					}
				}
				if cut < 0 {
					t.Fatalf("no frame band in %q", argv)
				}
				rawCall := newBodyCall(slices.Concat(argv[:cut], argv[cut+len(frame):]), call.band)
				rawOut := func() string {
					out, _, err := RunTimelineSearch(context.Background(), rawCall)
					if err != nil {
						t.Fatal(err)
					}
					return out
				}
				rc, rh := underBoth(t, rawOut)
				if rc == rh {
					t.Errorf("a raw %s search without the pin is not moved by the hostile file: the control proves nothing", src)
				}
			})
		}
	}
}
