package astrolog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// hostSettings are lines a user's astrolog.as may hold that change what a call
// prints: a chart the call did not ask for, a toggle turned the other way, the
// form of the text, a position, the moment, or the ephemeris.
var hostSettings = []string{
	"=v", "=a", ":gs 1", "=ga", "=w", ":w 3", "=g", "=m", "=Z", "=S", "=l", "=j", "=7", "=L", "=K",
	"=t 6 2000", "=tr 6 2000", "=T 6 1 2000", "=V 6 2000", "=Zd", "=N", "=Ux", "=H", "=HX", "=HC", "=HF", "=HS", "=H7", "=HI", "=Y",
	"=d", ":d 4", "=dm", "=dy", "=D", "=E", "=P", "=B", "=8", "=v0", "=HO", "=HA", "=Hc",
	"=k", "=k0", "=b0", "=sr", "=sr0", "=c3",
	"=YT", "=YV", "=Yf", "=Yh", "=Yn0", "=Ynn", "-Yz0 1",
	"_b", "=bs", "=bj", "=bp", "=bm",
	"-z0 1", "-YzO 1", "-YzC 1",
	"=X", "=Xi",
	// Astrolog's own astrolog.as sets each of these; the user's file may not.
	"=b1", "=b2", "_0b", "=Yr", "=Y8", "_YC", "_Yu0", "=Yn", "=I0", "=5", ":I 40", "-YP 1", "-Ya2",
	"=Yz1", "-Yw 0.5", "=s 5", ":sd", "=sr0", "=Ys",
	"=aa", "=v3 5", "=Xr", "=c3 2",
}

// hostPictureSettings each turn one of the wheel's options the other way from
// what the wheel asks for, so a toggle in the wheel call would undo it. The
// wheel call sets each option on or off itself (emitRender), so none moves it.
var hostPictureSettings = []string{
	"=Xi", "=XA", "=Xx", "=Xx0", "=Xr", "_Xu", "_Xt", "=XL",
	// What Astrolog's own astrolog.as sets for a picture (wheelBand).
	"=XJ", "_Xm", ":Xv 0", ":Xv 2", "_XQ", "_XQ0", "_Xv0", ":Xs 300", ":XS 150", ":XI0 60 0", ":Xkv Red",
	":YXG 113221", ":YXg 8", ":YXx 3", ":YXf #112233",
	// What the wheel call or the kind of picture makes moot (hostFollowed).
	":Xw 300 300", ":Xbw", ":YXS 5.0", ":YXj 3", ":YX7 300", ":YXp 1", ":YXp0 1in 1in",
}

// unseenOnStock are the settings that change nothing these calls print on a
// stock installation, so the control below cannot show them acting: Astrolog's
// own astrolog.as says `=0b`, which keeps `-bp` and `-bm` from taking effect;
// the data folder has no JPL file for `-bj`; `-YzC` moves the cusps by a time
// the listing does not print; `_0b` (old calculations) changes nothing the
// current binary prints; Sabian symbols (`=I0`) and the chart list (`=5`)
// act in chart modes these calls do not run; the screen width (`:I`) wraps no
// line these calls print; and the solar-system plane (`=Ys`) acts on a
// heliocentric chart none of them casts. The band still sets each of them.
var unseenOnStock = map[string]bool{"=bj": true, "=bp": true, "=bm": true, "-YzC 1": true,
	"_0b": true, "=I0": true, "=5": true, ":I 40": true, "=Ys": true}

// linkedDataDir is Astrolog's data folder with line added to its astrolog.as:
// the settings file is copied and every other file is linked, so a case costs
// no copy of the ephemeris. No line gives the stock folder.
func linkedDataDir(t *testing.T, line string) string {
	t.Helper()
	astrodir.Need(t)
	dir := t.TempDir()
	ents, err := os.ReadDir(originalDataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		src := filepath.Join(originalDataDir, e.Name())
		if e.Name() == "astrolog.as" {
			b, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			if line != "" {
				b = append(b, []byte("\n"+line+"\n")...)
			}
			if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.Symlink(src, filepath.Join(dir, e.Name())); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// hostProbes are the kinds of call Sextile makes, each through the one spawn
// path every call takes: a listing of every body to the second, an aspect
// list (with the cusps too), the chart box a chart file is read from, a day
// search, a year search, a transit and a return search, a single cast, a
// compiled comparison, a TimelineCall of each source, a CycleCall (a planet,
// the Node, Lilith), a SunPairCall, a chart file read, and a wheel picture.
func hostProbes(t *testing.T) map[string]func() string {
	t.Helper()
	at := []string{"-qj", "2451545.25", "-zl", "18:57E", "69:36N"}
	every := []string{"-R0", "-R"}
	for n := 1; n <= 21; n++ {
		every = append(every, strconv.Itoa(n))
	}
	cusps := []string{"-R0"}
	for n := 1; n <= 33; n++ {
		cusps = append(cusps, strconv.Itoa(n))
	}
	raw := func(argv ...string) func() string {
		return func() string {
			out, _, err := spawn(context.Background(), newExempt("test: a host setting against the band", argv...))
			if err != nil {
				return "error: " + err.Error()
			}
			return out
		}
	}
	search := func(call BodyCall, err error) func() string {
		return func() string {
			if err != nil {
				return "error: " + err.Error()
			}
			out, _, err := RunTimelineSearch(context.Background(), call)
			if err != nil {
				return "error: " + err.Error()
			}
			return out
		}
	}
	kepler, err := os.ReadFile(filepath.Join("testdata", "moments", "Kepler.as"))
	if err != nil {
		t.Fatal(err)
	}
	spec := fixtureSpec(t, fixTromso, recipe.Placidus, recipe.Tropical)
	pair := fixtureSpec(t, recipe.Comparison{Primary: fixLondon, Secondary: fixTromso}, recipe.Placidus, recipe.Tropical)
	probes := map[string]func() string{
		"listing": raw(append(append(append([]string(nil), at...), "-v", "-b0"), every...)...),
		"aspects": raw(append(append([]string(nil), at...), "-aa")...),
		"cusps":   raw(append(append(append([]string(nil), at...), "-a"), cusps...)...),
		"box":     raw(append(append([]string(nil), at...), "-w", "-R0")...),
		// A chart file may turn 3D houses on with a bare `=c3`: the method is then
		// the band's, never the host's.
		"3D houses": raw(append(append(append([]string(nil), at...), "=c3", "-v"), every...)...),
		"day":       raw("-z", "1:00E", "-qd", "6", "1", "1990", "-d"),
		"year":      raw("-z", "0", "-qy", "2000", "-dY", "1", "-R0", "-R", "2"),
		"transits":  raw(append(append([]string(nil), at...), "-t", "6", "2000")...),
		"returns":   raw(append(append([]string(nil), at...), "-tr", "6", "2000")...),
		"cast": func() string {
			out, err := ComputeChartPositionsOnly(context.Background(), spec)
			if err != nil {
				return "error: " + err.Error()
			}
			out.Commands = nil
			return withoutColour(fmt.Sprintf("%+v", out))
		},
		"comparison": func() string {
			out, err := ComputeCrossAspects(context.Background(), pair)
			if err != nil {
				return "error: " + err.Error()
			}
			out.Commands = nil
			return withoutColour(fmt.Sprintf("%+v", out))
		},
		"chart file": func() string {
			cf, err := ResolveChartBytes(context.Background(), kepler, "Kepler.as")
			if err != nil {
				return "error: " + err.Error()
			}
			return fmt.Sprintf("%+v", cf)
		},
		"wheel": func() string {
			svg, err := RenderWheelImage(context.Background(), spec, recipe.DefaultWheelRender())
			if err != nil {
				return "error: " + err.Error()
			}
			return string(svg)
		},
	}
	req := bandReq(t, recipe.ObjectPolicy{Baseline: recipe.AstrologDefault}, recipe.AspectPolicy{})
	for _, src := range []TimelineSource{TimelineSky, TimelineTransit, TimelineReturn} {
		call, err := TimelineCall(src, TimelineSegments(2026, time.October, req.Zone)[0], req)
		probes["timeline "+src.String()] = search(call, err)
	}
	for name, other := range map[string]aspect.Object{"venus": objVenus, "node": objNorthNode, "lilith": objLilith} {
		probes["cycle "+name] = search(CycleCall(CycleRequest{StartYear: 2016, Years: 3, Other: other}))
	}
	probes["sun pair"] = search(SunPairCall(SunPairRequest{When: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), Lat: 51, Other: objVenus, Registry: testRegistry(t)}))
	return probes
}

// ansiColour is one colour escape of Astrolog's `-k` text.
var ansiColour = regexp.MustCompile("\x1b\\[[0-9;]*m")

// withoutColour is text with its colour escapes taken out. A call that asks for
// Astrolog's colours (run.go adds `-k`) shows the colours of the user's
// Astrolog, on purpose (hostFollowed: `Yk*`, `Xr`), and the styles it parses
// from them are compared whole; only the raw text's escapes follow the user.
func withoutColour(s string) string { return ansiColour.ReplaceAllString(s, "") }

// switchKey is a switch's name without its leading `-`, `=` or `_`; a `:`
// setter keeps its colon, because `:s` sets Astrolog's sidereal offset and `_s`
// its zodiac. A value (a number, a name, a colour) is no switch.
func switchKey(tok string) (string, bool) {
	if len(tok) < 2 || !strings.ContainsRune("-=_:", rune(tok[0])) {
		return "", false
	}
	if _, err := strconv.ParseFloat(tok, 64); err == nil {
		return "", false
	}
	if tok[0] == ':' {
		return tok, true
	}
	return tok[1:], true
}

// switchKeys is the switches among argv's words; an element may hold several
// (a settings line is a switch and its values).
func switchKeys(argv []string) map[string]bool {
	keys := map[string]bool{}
	for _, el := range argv {
		for _, tok := range strings.Fields(el) {
			if k, ok := switchKey(tok); ok {
				keys[k] = true
			}
		}
	}
	return keys
}

// settingsFileKeys is what Astrolog writes for its own settings
// (`astrolog -od <file> -n`), as the switch of each line, in file order.
func settingsFileKeys(t *testing.T) []string {
	t.Helper()
	needAstrolog(t)
	dir := linkedDataDir(t, "")
	out := filepath.Join(t.TempDir(), "settings.as")
	cmd := exec.Command("astrolog", "-od", out, "-n")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "ASTROLOG="+dir)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("astrolog -od: %v\n%s", err, b)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], ";") || strings.HasPrefix(f[0], "@") {
			continue
		}
		if k, ok := switchKey(f[0]); ok {
			keys = append(keys, k)
		}
	}
	if len(keys) < 100 {
		t.Fatalf("only %d settings in what Astrolog wrote", len(keys))
	}
	return keys
}

// unclassified is the keys that are neither reset by a call's own band, nor
// pinned by the wheel, nor followed on purpose.
func unclassified(keys []string, classified ...map[string]bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, k := range keys {
		known := false
		for _, c := range classified {
			known = known || c[k]
		}
		if !known && !seen[k] {
			out = append(out, k)
		}
		seen[k] = true
	}
	return out
}

func followedKeys() map[string]bool {
	m := map[string]bool{}
	for k := range hostFollowed {
		m[k] = true
	}
	return m
}

// Every setting Astrolog's own astrolog.as holds is reset by a call or followed
// on purpose (hostFollowed): a setting a later Astrolog adds fails here by name.
func TestEveryAstrologSettingIsResetOrFollowedOnPurpose(t *testing.T) {
	full := recipe.DefaultWheelRender()
	full.SymbolScale, full.TextScale = 150, 150
	got := unclassified(settingsFileKeys(t), switchKeys(CallHead()), switchKeys(wheelBand), switchKeys(emitRender(full)), followedKeys())
	if len(got) > 0 {
		t.Errorf("settings no call resets and no table follows on purpose: %s", strings.Join(got, " "))
	}
}

// The check above is not vacuous: a switch in no table is named.
func TestUnclassifiedNamesASwitchInNoTable(t *testing.T) {
	got := unclassified([]string{"Yr", "Yzz", "Xw"}, map[string]bool{"Yr": true}, map[string]bool{"Xw": true})
	if len(got) != 1 || got[0] != "Yzz" {
		t.Errorf("got %q, want [Yzz]", got)
	}
}

// Every table entry names a switch Astrolog writes, so a table that outlives a
// setting says so, and no setting is both reset and followed.
func TestHostFollowedHoldsOnlyRealSettingsNeverReset(t *testing.T) {
	written := map[string]bool{}
	for _, k := range settingsFileKeys(t) {
		written[k] = true
	}
	reset := switchKeys(CallHead())
	for k := range hostFollowed {
		if !written[k] {
			t.Errorf("%q is followed on purpose but Astrolog writes no such setting", k)
		}
		if reset[k] {
			t.Errorf("%q is both reset by the band and followed on purpose", k)
		}
	}
}

// Every switch of the band has a line in the host settings below, so each is
// tried in a user's file.
func TestEveryBandSwitchHasAHostSetting(t *testing.T) {
	have := switchKeys(hostSettings)
	// A setting whose non-stock line is spelled with another switch.
	for line, band := range map[string]string{":sd": ":sz", "Ya2": "Ya0"} {
		if have[line] {
			have[band] = true
		}
	}
	var missing []string
	for k := range switchKeys(hostBand) {
		if !have[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("band switches no host setting tries: %s", strings.Join(missing, " "))
	}
}

// A setting in the user's astrolog.as changes nothing any call prints: every
// call starts with the host band, which sets each one back. On a stock
// installation the band itself changes nothing a call prints.
func TestAHostSettingChangesNoCall(t *testing.T) {
	needAstrolog(t)
	probes := hostProbes(t)
	useDataDir(t, linkedDataDir(t, ""))
	stock := map[string]string{}
	for name, probe := range probes {
		stock[name] = probe()
		if strings.HasPrefix(stock[name], "error: ") || len(stock[name]) < 100 {
			t.Fatalf("%s on a stock installation: %.300s", name, stock[name])
		}
	}
	band := hostBand
	hostBand = nil
	for name, probe := range probes {
		if bare := probe(); bare != stock[name] {
			t.Errorf("%s: the band changes it on a stock installation: %.200s", name, firstDifference(bare, stock[name]))
		}
	}
	hostBand = band
	cases := append([]string(nil), hostSettings...)
	cases = append(cases, allAtOnce(hostSettings, hostPictureSettings))
	for _, line := range cases {
		t.Run(strings.ReplaceAll(line, "\n", " ; "), func(t *testing.T) {
			useDataDir(t, linkedDataDir(t, line))
			for name, probe := range probes {
				if got := probe(); got != stock[name] {
					t.Errorf("%s moved: %.200s", name, firstDifference(stock[name], got))
				}
			}
		})
	}
}

// allAtOnce is one host file holding every line of each list: settings that
// interact (a toggle and a value, two forms of one setting) are tried together.
func allAtOnce(lists ...[]string) string {
	var all []string
	for _, l := range lists {
		all = append(all, l...)
	}
	return strings.Join(all, "\n")
}

// The control: without the band, each setting does change what some call
// prints, so the test above sees the band at work for every setting but the
// ones a stock installation cannot show (unseenOnStock).
func TestAHostSettingChangesNoCallControl(t *testing.T) {
	needAstrolog(t)
	prev := hostBand
	t.Cleanup(func() { hostBand = prev })
	hostBand = nil
	probes := hostProbes(t)
	useDataDir(t, linkedDataDir(t, ""))
	stock := map[string]string{}
	for name, probe := range probes {
		stock[name] = probe()
	}
	cases := append([]string(nil), hostSettings...)
	cases = append(cases, allAtOnce(hostSettings))
	for _, line := range cases {
		t.Run(strings.ReplaceAll(line, "\n", " ; "), func(t *testing.T) {
			useDataDir(t, linkedDataDir(t, line))
			moved := 0
			for name, probe := range probes {
				if probe() != stock[name] {
					moved++
				}
			}
			if moved == 0 && !unseenOnStock[line] {
				t.Errorf("without the band %q moves nothing: the test cannot see the band", line)
			}
			if moved > 0 && unseenOnStock[line] {
				t.Errorf("%q moves a call without the band: take it out of unseenOnStock", line)
			}
		})
	}
}

// A picture option in the user's astrolog.as does not change the wheel. Its
// control is by hand: with emitRender's toggles back (-Xi, -Xu, ...), each
// setting here moves the wheel.
func TestAHostPictureSettingChangesNoWheel(t *testing.T) {
	needAstrolog(t)
	wheel := hostProbes(t)["wheel"]
	useDataDir(t, linkedDataDir(t, ""))
	stock := wheel()
	if !strings.Contains(stock, "<svg") {
		t.Fatalf("no wheel on a stock installation: %.300s", stock)
	}
	for _, line := range append(append([]string(nil), hostPictureSettings...), allAtOnce(hostPictureSettings)) {
		t.Run(strings.ReplaceAll(line, "\n", " ; "), func(t *testing.T) {
			useDataDir(t, linkedDataDir(t, line))
			if got := wheel(); got != stock {
				t.Errorf("the wheel moved (%d bytes, stock %d)", len(got), len(stock))
			}
		})
	}
}
