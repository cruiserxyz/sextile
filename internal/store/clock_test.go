package store

// the specification C10, C11, N14, N20: the `clock:` preference block.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClockPreferenceReads(t *testing.T) {
	type rej struct{ field, value string }
	cases := []struct {
		name, body    string
		hours, dates  string
		rejections    []rej
		absentFile    bool
		noPreferences bool
	}{
		{name: "absent file", absentFile: true, hours: "24", dates: "month-first"},
		{name: "no clock block", body: "location:\n  name: Berlin\n", hours: "24", dates: "month-first"},
		{name: "empty block", body: "clock:\n", hours: "24", dates: "month-first"},
		{name: "12h month-first", body: "clock:\n  hours: 12\n  dates: month-first\n", hours: "12", dates: "month-first"},
		{name: "24h day-first", body: "clock:\n  hours: 24\n  dates: day-first\n", hours: "24", dates: "day-first"},
		{name: "12h day-first quoted with comments", body: "clock:\n  hours: \"12\"  # am/pm\n  dates: 'day-first'\n", hours: "12", dates: "day-first"},
		{name: "unknown entries ignored", body: "clock:\n  seconds: yes\n  hours: 12\n", hours: "12", dates: "month-first"},
		{name: "empty fields are absent", body: "clock:\n  hours:\n  dates: \"\"\n", hours: "24", dates: "month-first"},
		// C10 + N20: the last entry wins, and an empty last entry means the
		// default: `hours: 12` then an empty `hours:` resolves to 24, with no rejection.
		{name: "empty after 12 resolves to 24", body: "clock:\n  hours: 12\n  hours:\n", hours: "24", dates: "month-first"},
		{name: "empty after day-first resolves to month-first", body: "clock:\n  dates: day-first\n  dates: \"\"\n", hours: "24", dates: "month-first"},
		{name: "empty then 12 is 12", body: "clock:\n  hours:\n  hours: 12\n", hours: "12", dates: "month-first"},
		{name: "last entry wins", body: "clock:\n  hours: 12\n  hours: 24\n  dates: day-first\n  dates: month-first\n", hours: "24", dates: "month-first"},
		{name: "invalid hours", body: "clock:\n  hours: 13\n", hours: "24", dates: "month-first", rejections: []rej{{"clock.hours", "13"}}},
		{name: "invalid dates", body: "clock:\n  dates: iso\n", hours: "24", dates: "month-first", rejections: []rej{{"clock.dates", "iso"}}},
		{name: "invalid then valid wins silently", body: "clock:\n  hours: bogus\n  hours: 12\n", hours: "12", dates: "month-first"},
		{name: "valid then invalid rejects the winner", body: "clock:\n  hours: 12\n  hours: bogus\n", hours: "24", dates: "month-first", rejections: []rej{{"clock.hours", "bogus"}}},
		{name: "two invalid: one line, the winner", body: "clock:\n  hours: x\n  hours: y\n  dates: p\n  dates: q\n", hours: "24", dates: "month-first",
			rejections: []rej{{"clock.hours", "y"}, {"clock.dates", "q"}}},
		{name: "first block only", body: "clock:\n  hours: 12\nother:\n  a: b\nclock:\n  hours: 24\n  dates: day-first\n", hours: "12", dates: "month-first"},
		{name: "block ends at the next top-level key", body: "clock:\n  hours: 12\nkeys:\n  dates: day-first\n", hours: "12", dates: "month-first"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := prefPath(t)
			if !c.absentFile {
				seedPreferences(t, path, c.body)
			}
			got := Clock()
			if got.Hours != c.hours || got.Dates != c.dates {
				t.Errorf("clock = %s / %s, want %s / %s", got.Hours, got.Dates, c.hours, c.dates)
			}
			if len(got.Rejections) != len(c.rejections) {
				t.Fatalf("rejections = %+v, want %v", got.Rejections, c.rejections)
			}
			for i, r := range c.rejections {
				if got.Rejections[i].Field != r.field || got.Rejections[i].Value != r.value {
					t.Errorf("rejection %d = %+v, want %v", i, got.Rejections[i], r)
				}
			}
		})
	}
}

const clockBlock = "clock:\n  # operator's own comment\n  hours: 12\n  dates: day-first\n  extra: kept   # unknown entry\n"

// C11, N14: every Sextile write carries the clock block through byte for byte,
// keeps the file's mode, replaces atomically, and survives a raced edit.
func TestPreferenceWritesKeepTheClockBlock(t *testing.T) {
	t.Run("a plain write", func(t *testing.T) {
		path := prefPath(t)
		seedPreferences(t, path, "keys:\n  leader: \",\"\n"+clockBlock+"frame:\n  houses: Koch\n")
		if err := SetLocation(berlin); err != nil {
			t.Fatal(err)
		}
		if got := readPreferences(t, path); !strings.Contains(got, clockBlock) {
			t.Errorf("the clock block did not survive byte for byte:\n%s", got)
		}
	})

	t.Run("the file's mode is kept", func(t *testing.T) {
		path := prefPath(t)
		seedPreferences(t, path, clockBlock)
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatal(err)
		}
		if err := SetLocation(berlin); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o640 {
			t.Errorf("mode = %v, want 0640", fi.Mode().Perm())
		}
	})

	t.Run("a failed replace leaves the file and no temp file", func(t *testing.T) {
		path := prefPath(t)
		seedPreferences(t, path, clockBlock)
		before := readPreferences(t, path)
		prev := renameFile
		renameFile = func(string, string) error { return errors.New("injected rename failure") }
		t.Cleanup(func() { renameFile = prev })
		if err := SetLocation(berlin); err == nil {
			t.Fatal("a failed rename returned nil")
		}
		if got := readPreferences(t, path); got != before {
			t.Errorf("the file changed on a failed write:\n%s", got)
		}
		entries, _ := os.ReadDir(filepath.Dir(path))
		for _, e := range entries {
			if e.Name() != "preferences.yaml" {
				t.Errorf("left behind %q", e.Name())
			}
		}
	})

	t.Run("a clock edit between the read and the rename is kept", func(t *testing.T) {
		path := prefPath(t)
		seedPreferences(t, path, "clock:\n  hours: 24\n")
		raced := "clock:\n  hours: 12\n  dates: day-first\n"
		fired := 0
		prev := betweenReadAndRename
		betweenReadAndRename = func() {
			if fired++; fired == 1 {
				if err := os.WriteFile(path, []byte(raced), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		t.Cleanup(func() { betweenReadAndRename = prev })
		if err := SetLocation(berlin); err != nil {
			t.Fatal(err)
		}
		got := readPreferences(t, path)
		if !strings.Contains(got, raced) {
			t.Errorf("the raced clock block was lost:\n%s", got)
		}
		if !strings.Contains(got, "name: Berlin") {
			t.Errorf("the location was not written:\n%s", got)
		}
	})
}
