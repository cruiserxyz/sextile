package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readWorkflow(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// steps cuts a workflow into its steps, comment lines dropped.
func steps(wf string) []string {
	var kept []string
	for _, l := range strings.Split(wf, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			kept = append(kept, l)
		}
	}
	return strings.Split(strings.Join(kept, "\n"), "\n      - ")
}

func stepWith(t *testing.T, all []string, marker string) string {
	t.Helper()
	for _, s := range all {
		if strings.Contains(s, marker) {
			return s
		}
	}
	t.Fatalf("no step holds %q", marker)
	return ""
}

// The release workflow's wiring, read as text (no workflow runs here): the
// check step runs unless the deciding step said exactly green=true; the
// deciding step starts at green=false, asks about the tag's own commit,
// cannot fail the job, and writes its one line once, at the end; the notes,
// the Docker build and the release page always run.
func TestReleaseWorkflowSkipsOnlyOnGreen(t *testing.T) {
	wf := readWorkflow(t, "release.yml")
	if !strings.Contains(wf, "permissions:\n  contents: write\n  checks: read\njobs:\n") {
		t.Error("the permissions are not contents: write and checks: read")
	}
	all := steps(wf)
	green := stepWith(t, all, "id: green")
	check := stepWith(t, all, "./check --release")
	if !strings.Contains(check, "\n        if: steps.green.outputs.green != 'true'\n") {
		t.Error("the check step does not run whenever green is not exactly 'true'")
	}
	if strings.Index(wf, "id: green") > strings.Index(wf, "./check --release") {
		t.Error("the deciding step runs after the check")
	}
	for _, want := range []string{
		"run: |\n          green=false\n",
		`if sha=$(git rev-parse "refs/tags/$GITHUB_REF_NAME^{commit}") &&`,
		"check_name=check&filter=all",
		`line=$(go run ./tools/checkgreen "$sha" < "$RUNNER_TEMP/check-runs.json") &&`,
		`[ "$line" = "green=true" ]; then`,
		`echo "green=$green" >> "$GITHUB_OUTPUT"`,
	} {
		if !strings.Contains(green, want) {
			t.Errorf("the deciding step lacks %q", want)
		}
	}
	if n := strings.Count(green, "GITHUB_OUTPUT"); n != 1 {
		t.Errorf("the deciding step writes the step outputs %d times, want once, after the decision", n)
	}
	// Each command of the decision is a link of the if's && chain, where its
	// failure only leaves green=false.
	for _, l := range strings.Split(green, "\n") {
		l = strings.TrimSpace(l)
		if (strings.Contains(l, "gh api") || strings.Contains(l, "go run")) && !strings.HasSuffix(l, "&&") {
			t.Errorf("%q is not a link of the if's && chain, so its failure fails the release", l)
		}
	}
	for _, kept := range []string{"go run ./tools/releasenotes", "docker build", "gh release create"} {
		if s := stepWith(t, all, kept); strings.Contains(s, "\n        if:") {
			t.Errorf("the step running %q became conditional", kept)
		}
	}
}

// ci is the only workflow with a job named check, by its id or its name, so
// checkgreen may read a GitHub Actions `check` run as ci's.
func TestOnlyCIHasACheckJob(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.y*ml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no workflows found: %v", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		n := checkJobs(string(b))
		if filepath.Base(f) == "ci.yml" {
			if n != 1 {
				t.Errorf("ci.yml defines %d jobs named check, want 1", n)
			}
			continue
		}
		if n != 0 {
			t.Errorf("%s defines a job named check: a green run of it would skip the release check", filepath.Base(f))
		}
	}
}

var (
	jobID   = regexp.MustCompile(`^  ([^\s#:]+):`)
	jobName = regexp.MustCompile(`^    name:\s*['"]?([^'"#]*?)['"]?\s*(#.*)?$`)
)

// checkJobs counts the jobs of a workflow whose id or name is check.
func checkJobs(wf string) int {
	n, inJobs, id, named := 0, false, "", false
	flush := func() {
		if id == "check" || named {
			n++
		}
		id, named = "", false
	}
	for _, l := range strings.Split(wf, "\n") {
		switch {
		case strings.HasPrefix(l, "jobs:"):
			inJobs = true
		case inJobs && len(l) > 0 && l[0] != ' ' && l[0] != '#':
			flush()
			inJobs = false
		case inJobs && jobID.MatchString(l):
			flush()
			id = jobID.FindStringSubmatch(l)[1]
		case inJobs && id != "" && jobName.MatchString(l):
			named = strings.TrimSpace(jobName.FindStringSubmatch(l)[1]) == "check"
		}
	}
	flush()
	return n
}

func TestCheckJobs(t *testing.T) {
	for wf, want := range map[string]int{
		"jobs:\n  check:\n    name: check\n":                       1,
		"jobs:\n  check:\n    runs-on: x\n":                        1,
		"jobs:\n  aux:\n    name: check # public status\n":         1,
		"jobs:\n  aux:\n    name: 'check'\n  release:\n    x: 1\n": 1,
		"jobs:\n  release:\n    name: release\n":                   0,
		"jobs:\n  release:\n    steps:\n      - name: check\n":     0,
	} {
		if got := checkJobs(wf); got != want {
			t.Errorf("checkJobs(%q) = %d, want %d", wf, got, want)
		}
	}
}
