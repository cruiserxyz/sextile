package main

import (
	"strings"
	"testing"
)

const sha = "1111111111111111111111111111111111111111"

// run is one check run as the check-runs API lists it.
func run(name, head, slug, status, conclusion string) string {
	c := "null"
	if conclusion != "" {
		c = `"` + conclusion + `"`
	}
	return `{"name":"` + name + `","head_sha":"` + head + `","status":"` + status + `","conclusion":` + c + `,"app":{"slug":"` + slug + `"}}`
}

func answer(total int, runs ...string) string {
	return `{"total_count":` + itoa(total) + `,"check_runs":[` + strings.Join(runs, ",") + `]}`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// Only a completed, successful `check` run of GitHub Actions on this very
// commit, and no other `check` run that says otherwise, skips the check.
// Everything else runs it.
func TestDecide(t *testing.T) {
	green := run("check", sha, "github-actions", "completed", "success")
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"green", answer(1, green), true},
		{"two green runs", answer(2, green, green), true},
		{"red", answer(1, run("check", sha, "github-actions", "completed", "failure")), false},
		{"cancelled", answer(1, run("check", sha, "github-actions", "completed", "cancelled")), false},
		{"skipped", answer(1, run("check", sha, "github-actions", "completed", "skipped")), false},
		{"none", answer(0), false},
		{"in progress", answer(1, run("check", sha, "github-actions", "in_progress", "")), false},
		{"queued", answer(1, run("check", sha, "github-actions", "queued", "")), false},
		{"green and red disagree", answer(2, green, run("check", sha, "github-actions", "completed", "failure")), false},
		{"a failed run and its green re-run", answer(2, run("check", sha, "github-actions", "completed", "failure"), green), false},
		{"green and running", answer(2, green, run("check", sha, "github-actions", "in_progress", "")), false},
		{"another commit's run", answer(1, run("check", strings.Repeat("2", 40), "github-actions", "completed", "success")), false},
		{"another app's run", answer(1, run("check", sha, "someone-else", "completed", "success")), false},
		{"another name", answer(1, run("lint", sha, "github-actions", "completed", "success")), false},
		{"more runs than listed", answer(3, green), false},
		{"an API error body", `{"message":"Not Found","documentation_url":"https://docs.github.com"}`, false},
		{"not JSON", `curl: (6) Could not resolve host`, false},
		{"empty", ``, false},
		{"no check_runs field", `{"total_count":1}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, why := decide([]byte(c.body), sha)
			if got != c.want {
				t.Errorf("decide = %v (%s), want %v", got, why, c.want)
			}
			if why == "" {
				t.Error("no reason given")
			}
		})
	}
}

// The workflow reads one line: green=true only when decide says so.
func TestOutputLine(t *testing.T) {
	if got := outputLine(true); got != "green=true\n" {
		t.Errorf("outputLine(true) = %q", got)
	}
	if got := outputLine(false); got != "green=false\n" {
		t.Errorf("outputLine(false) = %q", got)
	}
}

// A commit that is not a full sha is refused: the decision would compare
// against the wrong string.
func TestDecideRefusesABadSha(t *testing.T) {
	if got, _ := decide([]byte(answer(1, run("check", "abc", "github-actions", "completed", "success"))), "abc"); got {
		t.Error("a short sha skipped the check")
	}
}
