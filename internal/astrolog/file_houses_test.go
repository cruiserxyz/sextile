package astrolog

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var reBoxHouses = regexp.MustCompile(`\|\s*([A-Za-z0-9 .()-]+?) houses\s*\|`)

// A chart file's house system is the one Astrolog's box names at the file's
// place, and where Astrolog cast Porphyry in place of the one asked, the
// asked one is kept apart.
func TestAChartFilesHouseSystemIsTheOneItNames(t *testing.T) {
	needAstrolog(t)
	dir := filepath.Join("testdata", "moments")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range ents {
		path, _ := filepath.Abs(filepath.Join(dir, e.Name()))
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		cf, err := ResolveChartBytes(context.Background(), raw, path)
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if cf.HouseName == "Null" {
			continue
		}
		checked++
		m := reBoxHouses.FindStringSubmatch(runAstrolog(t, "-c", "22", "-i", path, "-R0", "-w"))
		if m == nil || m[1] != cf.HouseName {
			t.Errorf("%s: the cast system is %q, Astrolog's box at its place names %v", e.Name(), cf.HouseName, m)
		}
	}
	if checked < 4 {
		t.Errorf("only %d files name a house system", checked)
	}
}

// polarCase is a file at a place where Astrolog cannot cast what it asks.
func polarCase(t *testing.T, lat, asks string) ChartFile {
	t.Helper()
	raw := "@AI800  ; Astrolog chart info.\n-qb Jun 21 2020 12:00 DT 1E 18:57E " + lat + "\n-zi \"Polar\" \"Far place\"\n-c " + asks + "\n"
	cf, err := ResolveChartBytes(context.Background(), []byte(raw), "Polar.as")
	if err != nil {
		t.Fatalf("%s %s: %v", lat, asks, err)
	}
	return cf
}

// Near either pole Astrolog casts Porphyry for Placidus and Koch. The chart
// keeps both: the cast one, and the asked one by Astrolog's own reading at
// 0°N 0°E. A file that asks for Porphyry itself asks for nothing else.
func TestAPolarChartKeepsTheCastAndTheAskedHouses(t *testing.T) {
	needAstrolog(t)
	for _, c := range []struct{ lat, asks, asked string }{
		{"69:39N", "0", "Placidus"},
		{"69:39S", "0", "Placidus"},
		{"70:00N", "1", "Koch"},
		{"69:39N", "6", ""},
	} {
		cf := polarCase(t, c.lat, c.asks)
		if cf.HouseName != "Porphyry" || cf.HouseNameAsked != c.asked {
			t.Errorf("%s -c %s: cast %q asked %q, want Porphyry and %q", c.lat, c.asks, cf.HouseName, cf.HouseNameAsked, c.asked)
		}
		want := c.asked
		if want == "" {
			want = "Porphyry"
		}
		if got := cf.Houses.StoredName(); !cf.HasHouses || got != want {
			t.Errorf("%s -c %s: the chart starts in %q, want %q", c.lat, c.asks, got, want)
		}
	}
	// In 3D houses the box names the substitute "3D Porphyry", and the asked
	// system is kept the same way.
	cf := polarCase(t, "70:00N", "0\n=c3")
	if cf.HouseName != "3D Porphyry" || cf.HouseNameAsked != "3D Placidus" {
		t.Errorf("70:00N -c 0 =c3: cast %q asked %q, want 3D Porphyry and 3D Placidus", cf.HouseName, cf.HouseNameAsked)
	}
}

// polarStandIn puts a recording stand-in for astrolog in place of the real one for
// the test: it writes each call's arguments to a log, fails the 0°N 0°E read
// when fail is set, and otherwise runs the real program.
func polarStandIn(t *testing.T, fail bool) (log string) {
	t.Helper()
	real, err := exec.LookPath(binary)
	if err != nil {
		t.Skip("no astrolog on PATH")
	}
	dir := t.TempDir()
	log = filepath.Join(dir, "calls.log")
	failLine := ""
	if fail {
		failLine = `case "$*" in *"-zl 0:00E 0:00N"*) exit 1 ;; esac` + "\n"
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + log + "\n" + failLine + "exec " + real + " \"$@\"\n"
	path := filepath.Join(dir, "astrolog")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := binary
	binary = path
	t.Cleanup(func() { binary = old })
	return log
}

// The second read is the first one's guarded record with only the place
// changed, after it; a failed second read costs the mark of a substitution,
// never the chart.
func TestThePolarSecondReadIsTheSameRecordAtZeroZero(t *testing.T) {
	needAstrolog(t)
	log := polarStandIn(t, false)
	polarCase(t, "62:00N", "0")
	if raw, _ := os.ReadFile(log); strings.Contains(string(raw), "-zl 0:00E 0:00N") {
		t.Errorf("Placidus at 62°N, which Astrolog casts, was read a second time")
	}
	polarCase(t, "69:39N", "0")
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	var first, second string
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if !strings.Contains(l, " -i ") {
			continue
		}
		if strings.Contains(l, "-zl 0:00E 0:00N") {
			second = l
		} else {
			first = l
		}
	}
	if first == "" || second == "" {
		t.Fatalf("no first and second read of the file in the calls:\n%s", raw)
	}
	if got := strings.Replace(second, " -zl 0:00E 0:00N", "", 1); got != first {
		t.Errorf("the second read is not the first with only the place added:\n%s\n%s", first, second)
	}
	if i, j := strings.Index(second, " -i "), strings.Index(second, "-zl 0:00E 0:00N"); j < i {
		t.Errorf("the place does not come after the record in %q", second)
	}

	polarStandIn(t, true)
	cf := polarCase(t, "69:39N", "0")
	if cf.HouseName != "Porphyry" || cf.HouseNameAsked != "" {
		t.Errorf("a failed second read gave cast %q asked %q, want Porphyry and nothing", cf.HouseName, cf.HouseNameAsked)
	}
}
