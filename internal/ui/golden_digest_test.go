package ui

import (
	"strings"
	"testing"
)

func TestBuildDigestOrdersOneOffsThenRepeatsByCount(t *testing.T) {
	chg := []goldenChange{
		{name: "a", old: "\x1b[1mKEYS q\x1b[0m\nsame\nonly-a-old\n", new: "KEYS x\nsame\nonly-a-new\n"},
		{name: "b", old: "KEYS q\nsame\n", new: "KEYS x\nsame\n"},
		{name: "c", old: "KEYS q\n", new: "KEYS x\n"},
		{name: "d", old: "same\n", new: "same\n"},
		{name: "e", new: "brand new\n", isNew: true},
		{name: "f", old: "BAR y\n", new: "BAR z\n"},
		{name: "g", old: "BAR y\n", new: "BAR z\n"},
	}
	got := buildDigest(chg)
	if !strings.HasPrefix(got, "goldens written: 7  changed: 5  unchanged: 1  new: 1\n") {
		t.Fatalf("header wrong:\n%s", got)
	}
	oneOffs := strings.Index(got, "== one-offs")
	repeated := strings.Index(got, "== repeated")
	if oneOffs < 0 || repeated < oneOffs {
		t.Fatalf("sections out of order:\n%s", got)
	}
	if !strings.Contains(got[oneOffs:repeated], "- only-a-old   [a]") ||
		!strings.Contains(got[oneOffs:repeated], "+ only-a-new   [a]") {
		t.Errorf("one-offs missing:\n%s", got)
	}
	rep := got[repeated:]
	if !strings.Contains(rep, "   3  - KEYS q   [a, b, c]") || !strings.Contains(rep, "   3  + KEYS x   [a, b, c]") {
		t.Errorf("repeated group (ANSI-stripped, counted per file) missing:\n%s", got)
	}
	if three, two := strings.Index(rep, "KEYS q"), strings.Index(rep, "BAR y"); two < 0 || three > two {
		t.Errorf("repeats not ordered by file count (3 before 2):\n%s", got)
	}
	if strings.Contains(rep, "same") {
		t.Errorf("unchanged line leaked into the digest:\n%s", got)
	}
}

func TestBuildDigestNothingChanged(t *testing.T) {
	got := buildDigest([]goldenChange{{name: "a", old: "x\n", new: "x\n"}})
	if got != "no golden changed\n" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildDigestCapsFileNames(t *testing.T) {
	var chg []goldenChange
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		chg = append(chg, goldenChange{name: n, old: "o\n", new: "n\n"})
	}
	if got := buildDigest(chg); !strings.Contains(got, "[a, b, c, d, e +2 more]") {
		t.Fatalf("no cap:\n%s", got)
	}
}
