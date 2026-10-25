package lookup

// the specification N16: a slot's instant is made a whole second ONCE, at the
// source, by truncation (the second Sextile shows), before any Julian Day
// exists. Every call and staged file one slot produces names that second, and
// each builder names the second of ITS OWN slot, primary and secondary alike.

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/payload"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// spawnRecord is one logged Astrolog process: its argv and the staged chart
// files it was handed (path base name to content).
type spawnRecord struct {
	argv  []string
	files map[string]string
}

// slotRecordingAstrolog puts an `astrolog` on PATH that logs every argv and
// every staged .as file, and answers with the repository's recorded Astrolog
// output (positions for a chart or probe call, the relationship list for -rt and
// -r0, the aspect list for -aa), so each builder runs to the end and stages the
// files it would stage. A wheel call gets a placeholder image at its -Xo path.
// The returned function gives the spawns logged since it was last called.
func slotRecordingAstrolog(t *testing.T) func() []spawnRecord {
	t.Helper()
	fix, err := filepath.Abs(filepath.Join("..", "astrolog", "testdata", "provenance"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "log.txt")
	script := "#!/bin/sh\n" +
		"{ echo SPAWN; printf '%s\\n' \"$@\"; } >> " + strconv.Quote(log) + "\n" +
		"prev=\n" +
		"for a in \"$@\"; do\n" +
		"  case \"$a\" in *.as) { echo \"FILE $a\"; /bin/cat \"$a\"; } >> " + strconv.Quote(log) + " ;; esac\n" +
		"  if [ \"$prev\" = \"-Xo\" ]; then printf 'IMG' > \"$a\"; fi\n" +
		"  prev=\"$a\"\n" +
		"done\n" +
		"case \" $* \" in\n" +
		"  *\" -Xo \"*) exit 0 ;;\n" +
		"  *\" -rt \"*|*\" -r0 \"*) exec /bin/cat " + strconv.Quote(filepath.Join(fix, "cross-k.txt")) + " ;;\n" +
		"  *\" -aa \"*) exec /bin/cat " + strconv.Quote(filepath.Join(fix, "aspects-k.txt")) + " ;;\n" +
		"  *) exec /bin/cat " + strconv.Quote(filepath.Join(fix, "positions-k.txt")) + " ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	seen := 0
	return func() []spawnRecord {
		b, _ := os.ReadFile(log)
		lines := strings.Split(string(b), "\n")
		lines = lines[seen:]
		seen += len(lines) - 1 // the trailing empty element is not a line
		var out []spawnRecord
		var cur *spawnRecord
		curFile := ""
		for _, l := range lines {
			switch {
			case l == "SPAWN":
				out = append(out, spawnRecord{files: map[string]string{}})
				cur, curFile = &out[len(out)-1], ""
			case cur == nil:
			case strings.HasPrefix(l, "FILE "):
				curFile = filepath.Base(strings.TrimPrefix(l, "FILE "))
				cur.files[curFile] = ""
			case curFile != "":
				cur.files[curFile] += l + "\n"
			default:
				cur.argv = append(cur.argv, l)
			}
		}
		return out
	}
}

func jdWord(second time.Time) string {
	return strconv.FormatFloat(math.Nextafter(astrolog.GregorianToJD(second), math.Inf(1)), 'f', -1, 64)
}

// fileNamesSecond says whether a staged chart file names the second as the very
// `-qj` Julian Day every other call for the slot sends (the specification C1:
// no staged file carries calendar fields any more).
func fileNamesSecond(content string, s time.Time) bool {
	return strings.Contains(content, "\n-qj "+jdWord(s)+"\n") && !strings.Contains(content, "-qb")
}

func TestSlotInstantIsOneWholeSecond(t *testing.T) {
	place := location.PlaceInfo{Name: "Greenwich", Country: "GB", Lat: 51.48, Lon: 0, TZ: "UTC"}
	cases := []struct {
		name        string
		p, s, third time.Time // the primary slot, the secondary, a third ring
	}{
		{"23:59:59.9995 stays on the day",
			time.Date(2026, 10, 5, 23, 59, 59, 999_500_000, time.UTC),
			time.Date(2026, 10, 6, 11, 34, 56, 999_500_000, time.UTC),
			time.Date(2026, 10, 7, 3, 4, 5, 999_500_000, time.UTC)},
		{"x.5 s",
			time.Date(2026, 10, 5, 12, 0, 7, 500_000_000, time.UTC),
			time.Date(2026, 10, 6, 9, 8, 9, 500_000_000, time.UTC),
			time.Date(2026, 10, 7, 1, 2, 3, 500_000_000, time.UTC)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ps, ss, ts := c.p.Truncate(time.Second), c.s.Truncate(time.Second), c.third.Truncate(time.Second)
			if ps.Equal(ss) || ss.Equal(ts) {
				t.Fatal("the slots must be distinct seconds")
			}
			natal := store.AnchorChart{When: c.p, Place: place}
			ctx := context.Background()

			// The chart computes: the spec lookup hands the compiler.
			var specs []recipe.ChartSpec
			stub := ChartComputer(func(_ context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
				specs = append(specs, spec)
				return payload.RawOutput{}, nil
			})
			probers := &Probers{Full: stub, PositionsOnly: stub}
			if _, err := FirstCallTransits(ctx, probers, c.p, place, Hellenistic); err != nil {
				t.Fatal(err)
			}
			if _, err := FirstCallNatalPositions(ctx, probers, store.AnchorChart{When: c.s, Place: place}, Hellenistic); err != nil {
				t.Fatal(err)
			}
			for i, want := range []time.Time{ps, ss} {
				plan, err := astrolog.Compile(specs[i], recipe.Positions{})
				if err != nil {
					t.Fatal(err)
				}
				if got := wordAfter(plan.Invocations[0].Argv, "-qj"); got != jdWord(want) {
					t.Errorf("chart compute %d sends -qj %s, want the second of its slot %s (%s)", i, got, want, jdWord(want))
				}
			}

			delta := slotRecordingAstrolog(t)
			// pair builders: staged files chart0 = the primary's second, chart1 = the secondary's
			pairs := []struct {
				name string
				run  func() error
				// the second each staged file must name; a probe -qj, if any, is the primary's
				chart0, chart1 time.Time
				probe          bool
			}{
				{"CrossBodyRawReport", func() error {
					_, _, err := CrossBodyRawReport(ctx, Hellenistic, natal, c.s, place, aspect.Sun)
					return err
				}, ps, ss, false},
				{"SynastryCrossBodyRawReport", func() error {
					_, _, err := SynastryCrossBodyRawReport(ctx, Hellenistic, natal, c.s, place, store.ReportLabel{}, aspect.Sun)
					return err
				}, ps, ss, false},
				{"OverlayCrossAspects", func() error {
					_, err := OverlayCrossAspects(ctx, Hellenistic, natal, c.s, place)
					return err
				}, ps, ss, true},
				{"SynastryCrossAspects", func() error {
					_, err := SynastryCrossAspects(ctx, Hellenistic, natal, c.s, place, store.ReportLabel{})
					return err
				}, ps, ss, true},
				{"RenderOverlayWheelImage", func() error {
					_, err := RenderOverlayWheelImage(ctx, natal, c.s, place, Hellenistic)
					return err
				}, ps, ss, false},
			}
			for _, k := range pairs {
				t.Run(k.name, func(t *testing.T) {
					delta() // drop earlier spawns
					if err := k.run(); err != nil {
						t.Fatalf("the call failed: %v", err)
					}
					spawns := delta()
					if len(spawns) == 0 {
						t.Fatal("no spawn logged")
					}
					staged, probes := 0, 0
					for _, sp := range spawns {
						if len(sp.files) == 0 {
							probes++
							if got := wordAfter(sp.argv, "-qj"); got != jdWord(k.chart0) {
								t.Errorf("the probe sends -qj %s, want the primary's second %s", got, jdWord(k.chart0))
							}
						}
						if len(sp.files) > 0 {
							staged++
							for name, content := range sp.files {
								want := k.chart0
								if strings.Contains(name, "chart1") {
									want = k.chart1
								}
								if !fileNamesSecond(content, want) {
									t.Errorf("%s names the wrong second, want %s:\n%s", name, want, content)
								}
							}
							if len(sp.files) != 2 {
								t.Errorf("staged %d charts, want both of the pair", len(sp.files))
							}
						}
					}
					if staged == 0 {
						t.Error("no comparison file was staged: the call stopped before it")
					}
					// A cross-aspect list runs exactly one ephemeris probe (the
					// primary's UT positions) before the relationship; a report or
					// a wheel runs none.
					wantProbes := 0
					if k.probe {
						wantProbes = 1
					}
					if probes != wantProbes {
						t.Errorf("%d probe spawns, want exactly %d", probes, wantProbes)
					}
				})
			}
			t.Run("ComputeRawReport", func(t *testing.T) {
				delta()
				if _, _, err := ComputeRawReport(ctx, c.p, place, Hellenistic, RawReportRequest{Kind: RawReportChart}); err != nil {
					t.Fatal(err)
				}
				sp := delta()
				if len(sp) != 1 {
					t.Fatalf("spawns = %d", len(sp))
				}
				if got := wordAfter(sp[0].argv, "-qj"); got != jdWord(ps) || indexOf(sp[0].argv, "-qb") >= 0 {
					t.Errorf("the local report asks %v, want -qj %s and no -qb", sp[0].argv, jdWord(ps))
				}
			})
			t.Run("RenderWheelImage", func(t *testing.T) {
				delta()
				if _, err := RenderWheelImage(ctx, c.p, place, Hellenistic, WheelHighlight{}); err != nil {
					t.Fatal(err)
				}
				sp := delta()
				if len(sp) != 1 || wordAfter(sp[0].argv, "-qj") != jdWord(ps) {
					t.Errorf("the wheel asks %v, want -qj %s", sp, jdWord(ps))
				}
			})
			t.Run("RenderMultiWheelImage", func(t *testing.T) {
				delta()
				rings := []WheelRing{{When: c.p, Place: place}, {When: c.s, Place: place}, {When: c.third, Place: place}}
				if _, _, err := RenderMultiWheelImageCommands(ctx, rings, Hellenistic); err != nil {
					t.Fatal(err)
				}
				sp := delta()
				if len(sp) != 1 || len(sp[0].files) != 3 {
					t.Fatalf("spawns = %+v", sp)
				}
				for i, want := range []time.Time{ps, ss, ts} {
					if content := sp[0].files[fmt.Sprintf("chart%d.as", i)]; !fileNamesSecond(content, want) {
						t.Errorf("ring %d names the wrong second, want %s:\n%s", i+1, want, content)
					}
				}
			})
		})
	}
}

func indexOf(argv []string, key string) int {
	for i, a := range argv {
		if a == key {
			return i
		}
	}
	return -1
}

func wordAfter(argv []string, key string) string {
	if i := indexOf(argv, key); i >= 0 && i+1 < len(argv) {
		return argv[i+1]
	}
	return ""
}
