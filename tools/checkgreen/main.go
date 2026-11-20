// Command checkgreen decides whether the release workflow may skip its own
// check because the public `check` run of `ci` already passed on the tagged
// commit. `ci` is the only workflow with a job named `check`
// (TestOnlyCIHasACheckJob), so a GitHub Actions run of that name is ci's.
//
//	checkgreen <commit sha>  < check-runs.json
//
// It reads the answer of GitHub's API
// `GET /repos/{owner}/{repo}/commits/{sha}/check-runs?check_name=check&filter=all`
// (every run, re-runs included) and prints one line: `green=true` only when
// every `check` run GitHub Actions lists for that commit completed with
// success, and there is at least one. Anything else prints `green=false`: an
// error answer, no run, a run still going, any other conclusion, runs that
// disagree (a failed run and its green re-run), or more runs than the answer
// lists. The reason goes to stderr. The workflow skips the check only on the
// exact line `green=true` from a run that exited 0.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
)

var shaRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

type checkRuns struct {
	TotalCount *int `json:"total_count"`
	CheckRuns  *[]struct {
		Name       string  `json:"name"`
		HeadSHA    string  `json:"head_sha"`
		Status     string  `json:"status"`
		Conclusion *string `json:"conclusion"`
		App        struct {
			Slug string `json:"slug"`
		} `json:"app"`
	} `json:"check_runs"`
}

// decide reports whether the answer body shows the check green on sha, and why.
func decide(body []byte, sha string) (bool, string) {
	if !shaRE.MatchString(sha) {
		return false, fmt.Sprintf("%q is not a full commit sha", sha)
	}
	var a checkRuns
	if err := json.Unmarshal(body, &a); err != nil {
		return false, "the API answer is not the check-runs list: " + err.Error()
	}
	if a.TotalCount == nil || a.CheckRuns == nil {
		return false, "the API answer is not the check-runs list"
	}
	if *a.TotalCount != len(*a.CheckRuns) {
		return false, fmt.Sprintf("the answer lists %d of %d runs", len(*a.CheckRuns), *a.TotalCount)
	}
	n := 0
	for _, r := range *a.CheckRuns {
		if r.Name != "check" || r.HeadSHA != sha || r.App.Slug != "github-actions" {
			continue
		}
		n++
		if r.Status != "completed" {
			return false, "a check run on this commit is " + r.Status
		}
		if r.Conclusion == nil || *r.Conclusion != "success" {
			c := "none"
			if r.Conclusion != nil {
				c = *r.Conclusion
			}
			return false, "a check run on this commit concluded " + c
		}
	}
	if n == 0 {
		return false, "no check run on this commit"
	}
	return true, fmt.Sprintf("%d check run(s) on this commit passed", n)
}

func outputLine(green bool) string {
	return fmt.Sprintf("green=%v\n", green)
}

func main() {
	sha := ""
	if len(os.Args) == 2 {
		sha = os.Args[1]
	}
	body, err := io.ReadAll(os.Stdin)
	green, why := false, ""
	if err != nil {
		why = "reading the API answer: " + err.Error()
	} else {
		green, why = decide(body, sha)
	}
	fmt.Fprintln(os.Stderr, "checkgreen:", why)
	fmt.Print(outputLine(green))
}
