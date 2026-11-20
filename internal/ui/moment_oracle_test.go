package ui

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// momentDir holds the oracle's chart files, each written by Astrolog 8.00.
const momentDir = "../astrolog/testdata/moments"

var oracleAnchor = time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)

// openMoment opens one chart file from momentDir through the real model at
// cols x rows and returns the model with the cursor on that chart.
func openMoment(t *testing.T, file string, cols, rows int) Model {
	t.Helper()
	needLiveAstrolog(t)
	dir := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")
	b, err := os.ReadFile(filepath.Join(momentDir, file))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), b, 0o600); err != nil {
		t.Fatal(err)
	}
	place := location.PlaceInfo{Name: "Hamburg", Country: "DE", Lat: 53.5507, Lon: 9.993, TZ: "Europe/Berlin"}
	m := newTestModel(t, Config{}, FrozenClock(oracleAnchor))
	m.startup = startupReady
	m.place = place
	slots, err := store.NewSlots(oracleAnchor, place)
	if err != nil {
		t.Fatal(err)
	}
	m.slots = slots
	m.targetFloatingTime = slots.Floating().Time
	m.width, m.height = cols, rows
	return pickLive(t, m, file)
}

// The oracle: Astrolog reading the file itself is the truth. For every file in
// momentDir it compares what Sextile shows (the clock and date of the Chart row,
// its calendar mark and zone label, the instant it stores, the Details header,
// each body's position, the Ascendant and the MC) with `astrolog -i FILE -v`,
// to Astrolog's printed precision. The instant must lie within half a second
// of the Julian Day Astrolog casts, which `=b0` prints to seven decimals.

var (
	reAstroHead = regexp.MustCompile(`^(\w{3} \w{3} [ 0-9]?[0-9], -?[0-9]+) +([0-9]{1,2}:[0-9]{2}) \((\S+) Zone ([^)]*)\)`)
	reAstroPos  = regexp.MustCompile(`^(Sun |Moon|Merc|Venu|Mars|Jupi|Satu|Uran|Nept|Plut|Asce|Midh): *([0-9]{1,2}[A-Z][a-z]{2}[0-9]{2})`)
	reZoneCell  = regexp.MustCompile(`^([+-])([0-9]+)(?::([0-9]{2}))?(?::([0-9]{2}))?( DT)?$`)
	reBodyRow   = regexp.MustCompile(`^.? ?(Sun|Moon|Mercury|Venus|Mars|Jupiter|Saturn|Uranus|Neptune|Pluto|Asc|MC) +([A-Z][a-z]{2}) +([0-9]{1,2})°([0-9]{2})'`)
	bodyKey     = map[string]string{"Sun": "Sun ", "Moon": "Moon", "Mercury": "Merc", "Venus": "Venu", "Mars": "Mars", "Jupiter": "Jupi", "Saturn": "Satu", "Uranus": "Uran", "Neptune": "Nept", "Pluto": "Plut", "Asc": "Asce", "MC": "Midh"}
)

type oracleRead struct {
	date, clock, dst, zone string
	pos                    map[string]string
	// jd is the Julian Day Astrolog casts, from its wheel's centre box.
	jd float64
}

var reAstroJD = regexp.MustCompile(`Julian Day: +(-?[0-9]+\.[0-9]+)`)

// julianCalendar says whether Astrolog prints date ("Thu Dec 27, 1571") in
// the Julian calendar: before its reform of October 15, 1582.
func julianCalendar(t *testing.T, date string) bool {
	t.Helper()
	f := strings.Fields(strings.ReplaceAll(date, ",", ""))
	if len(f) != 4 {
		t.Fatalf("date %q", date)
	}
	mon, err := time.Parse("Jan", f[1])
	if err != nil {
		t.Fatalf("date %q: %v", date, err)
	}
	d, y := atoiOr(f[2]), atoiOr(f[3])
	return y < 1582 || y == 1582 && (mon.Month() < time.October || mon.Month() == time.October && d < 15)
}

func astrologTruth(t *testing.T, path string) oracleRead {
	t.Helper()
	out, err := exec.Command("astrolog", "=Yt", "_Yd", "-i", path, "-v").CombinedOutput()
	if err != nil {
		t.Fatalf("astrolog -i %s: %v\n%s", path, err, out)
	}
	r := oracleRead{pos: map[string]string{}}
	for _, l := range strings.Split(string(out), "\n") {
		if m := reAstroHead.FindStringSubmatch(l); m != nil && r.date == "" {
			r.date, r.clock, r.dst, r.zone = strings.Join(strings.Fields(m[1]), " "), m[2], m[3], m[4]
		}
		if m := reAstroPos.FindStringSubmatch(l); m != nil {
			r.pos[m[1]] = strings.TrimLeft(m[2], " ")
		}
	}
	if r.date == "" {
		t.Fatalf("astrolog printed no header for %s:\n%s", path, out)
	}
	box, err := exec.Command("astrolog", "-i", path, "=b0", "-w", "-R0").CombinedOutput()
	if err != nil {
		t.Fatalf("astrolog -i %s -w: %v\n%s", path, err, box)
	}
	m := reAstroJD.FindStringSubmatch(string(box))
	if m == nil {
		t.Fatalf("astrolog printed no Julian Day for %s:\n%s", path, box)
	}
	fmt.Sscan(m[1], &r.jd)
	return r
}

type sextileRead struct {
	date, clock, mark, zoneLabel  string
	seconds                       string // the row clock's ":ss"
	headDate, headClock, headZone string
	pos                           map[string]string
	when                          time.Time
}

func sextileShows(t *testing.T, m Model, name string) sextileRead {
	t.Helper()
	r := sextileRead{pos: map[string]string{}}
	frame := ansi.Strip(m.View())
	for _, raw := range strings.Split(frame, "\n") {
		cells := strings.Split(raw, "│")
		for _, c := range cells {
			c = strings.TrimSpace(c)
			if r.headDate == "" {
				if mm := reAstroHead.FindStringSubmatch(c); mm != nil {
					r.headDate, r.headClock, r.headZone = strings.Join(strings.Fields(mm[1]), " "), mm[2], mm[3]+" Zone "+mm[4]
				}
			}
			if mm := reBodyRow.FindStringSubmatch(c); mm != nil {
				if _, seen := r.pos[bodyKey[mm[1]]]; !seen {
					r.pos[bodyKey[mm[1]]] = fmt.Sprintf("%d%s%s", atoiOr(mm[3]), mm[2], mm[4])
				}
			}
		}
	}
	for _, s := range m.slots.All() {
		if !s.Floating && s.Chart.Name == name {
			r.when = s.Time
			// The row's cells, as the table draws them: the weekday may give
			// way to a narrow panel, the cells keep it.
			c := slotCells(s, nil)
			r.zoneLabel = c.zone
			r.date = c.weekday + " " + strings.Join(strings.Fields(c.date), " ")
			if c.julian {
				r.mark = "Jul"
			}
			hm, sec, _ := strings.Cut(strings.TrimSpace(c.clock), ":")
			mm, ss, _ := strings.Cut(sec, ":")
			r.clock, r.seconds = hm+":"+mm, ":"+ss
		}
	}
	return r
}

var reZoneWithSeconds = regexp.MustCompile(`(?m)^-q[a-z]? .* [0-9]+:[0-9]{2}:[0-9]{2}[EW] `)

// zoneHasSeconds says whether the chart file states its zone to the second.
func zoneHasSeconds(t *testing.T, path string) bool {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return reZoneWithSeconds.Match(b)
}

// zoneCellSeconds reads a zone cell ("+2 DT", "-3", "+5:30") as seconds east
// of Greenwich; "LMT", "LAT" and "AT" cells name no offset and give false.
func zoneCellSeconds(cell string) (int, bool) {
	m := reZoneCell.FindStringSubmatch(cell)
	if m == nil {
		return 0, false
	}
	n := func(s string) int { v, _ := strconv.Atoi(s); return v }
	s := (n(m[2])*60+n(m[3]))*60 + n(m[4])
	if m[1] == "-" {
		s = -s
	}
	return s, true
}

// atoiNum reads a decimal number, a leading zero included ("09" is nine).
func atoiNum(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}

func atoiOr(s string) int {
	n := 0
	fmt.Sscan(s, &n)
	return n
}

func TestMomentOracle(t *testing.T) {
	needLiveAstrolog(t)
	ents, err := os.ReadDir(momentDir)
	if err != nil {
		t.Fatal(err)
	}
	var table []string
	for _, e := range ents {
		file := e.Name()
		t.Run(file, func(t *testing.T) {
			path, _ := filepath.Abs(filepath.Join(momentDir, file))
			truth := astrologTruth(t, path)
			name, _ := chartFileTitle(t, path)
			m := openMoment(t, file, 200, 60)
			got := sextileShows(t, m, name)
			var bad []string
			if got.date != truth.date || got.clock != truth.clock {
				bad = append(bad, fmt.Sprintf("Chart row %q %s, Astrolog %q %s", got.date, got.clock, truth.date, truth.clock))
			}
			if got.headDate != truth.date || got.headClock != truth.clock || got.headZone != truth.dst+" Zone "+truth.zone {
				bad = append(bad, fmt.Sprintf("Details header %q %s (%s), Astrolog %q %s (%s Zone %s)", got.headDate, got.headClock, got.headZone, truth.date, truth.clock, truth.dst, truth.zone))
			}
			if jul := julianCalendar(t, truth.date); jul != (got.mark == "Jul") {
				bad = append(bad, fmt.Sprintf("Chart row mark %q for %s, Julian calendar %v", got.mark, truth.date, jul))
			}
			secs := (truth.jd - 2440587.5) * 86400
			whole := math.Floor(secs)
			instant := time.Unix(int64(whole), int64((secs-whole)*1e9))
			if d := got.when.Sub(instant); d <= -time.Second/2 || d >= time.Second/2 {
				bad = append(bad, fmt.Sprintf("stored instant %s, Astrolog casts JD %.7f (%s)", got.when.UTC().Format(time.RFC3339), truth.jd, instant.UTC().Format(time.RFC3339)))
			}
			if want := lookup.ZoneCell(truth.dst, truth.zone); got.zoneLabel != want {
				bad = append(bad, fmt.Sprintf("zone label %q, Astrolog says %q", got.zoneLabel, want))
			}
			// The zone cell says "clock minus this number is universal time":
			// Astrolog's own instant at that offset reads the row's clock, so
			// the daylight hour is counted once and only once. The instant is
			// rounded to the second (its Julian Day is printed to 1e-7 days),
			// and Astrolog's header truncates its clock, so a moment a hair
			// below a whole second reads one second earlier; no more.
			// A file whose zone states seconds shows the minutes of Astrolog's
			// header (a stated limit), so only whole-minute zones are held to it.
			if off, ok := zoneCellSeconds(got.zoneLabel); ok && !zoneHasSeconds(t, path) {
				local := instant.Round(time.Second).In(time.FixedZone("", off))
				want := local.Hour()*3600 + local.Minute()*60 + local.Second()
				h, mm, _ := strings.Cut(got.clock, ":")
				shown := atoiNum(h)*3600 + atoiNum(mm)*60 + atoiNum(strings.TrimPrefix(got.seconds, ":"))
				if d := (want - shown + 86400) % 86400; d > 1 {
					bad = append(bad, fmt.Sprintf("row clock %s%s at %s, Astrolog's instant reads %s there", got.clock, got.seconds, got.zoneLabel, local.Format("15:04:05")))
				}
			}
			var keys []string
			for k := range truth.pos {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if got.pos[k] != truth.pos[k] {
					bad = append(bad, fmt.Sprintf("%s %s, Astrolog %s", strings.TrimSpace(k), got.pos[k], truth.pos[k]))
				}
			}
			row := fmt.Sprintf("| %s | %s %s %s (%s %s) | %s %s %s %s | %s | %s |", file, truth.date, truth.clock, "", truth.dst, truth.zone,
				got.date, got.mark, got.clock, got.zoneLabel, got.when.UTC().Format("2006-01-02T15:04:05Z"), strings.Join(bad, "; "))
			table = append(table, row)
			for _, b := range bad {
				t.Errorf("%s: %s", file, b)
			}
		})
	}
	if out := os.Getenv("SEXTILE_ORACLE_OUT"); out != "" {
		head := "| file | Astrolog: date clock (dst zone) | Sextile Chart row: date mark clock zone | stored when (UTC) | mismatches |\n|---|---|---|---|---|\n"
		_ = os.WriteFile(out, []byte(head+strings.Join(table, "\n")+"\n"), 0o644)
	}
}

// chartFileTitle is the file's -zi name, the title the Chart row shows.
func chartFileTitle(t *testing.T, path string) (string, string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mm := regexp.MustCompile(`(?m)^-zi "([^"]*)" "([^"]*)"`).FindStringSubmatch(string(b))
	if mm == nil {
		return "", ""
	}
	return mm[1], mm[2]
}

func TestMomentOracleDump(t *testing.T) {
	if os.Getenv("SEXTILE_ORACLE_DUMP") == "" {
		t.Skip("set SEXTILE_ORACLE_DUMP to print the frames")
	}
	m := openMoment(t, os.Getenv("SEXTILE_ORACLE_DUMP"), 200, 60)
	t.Log("\n" + ansi.Strip(m.View()))
}
