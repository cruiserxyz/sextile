package testenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Redirecting HOME and the XDG bases is not isolation on its own:
// ChartsDir gives SEXTILE_CHARTS_DIR the TOP precedence and consults
// SEXTILE_DEV_CHARTS_DIR before falling back to the XDG default. An operator
// with either exported — and ./test_charts, the gitignored dev library the
// repo's own .env names, is a real one holding real chart files — kept a test
// pointed at their data no matter what HOME said.
func TestRunIsolatesTheChartLibraryOverrides(t *testing.T) {
	for k, v := range map[string]string{
		"HOME":                   "/operator/home",
		"XDG_DATA_HOME":          "/operator/home/.local/share",
		"XDG_CONFIG_HOME":        "/operator/home/.config",
		"XDG_STATE_HOME":         "/operator/home/.local/state",
		"XDG_CACHE_HOME":         "/operator/home/.cache",
		"SEXTILE_CHARTS_DIR":     "/operator/real/client-charts",
		"SEXTILE_DEV_CHARTS_DIR": "/repo/test_charts",
		"SEXTILE_ATLAS_PATH":     "/operator/atlas/atlas.as",
	} {
		t.Setenv(k, v) // recorded so the harness restores it after this test
	}

	var inside map[string]string
	var sandbox string
	code := Run(func() int {
		inside = map[string]string{}
		for _, k := range []string{"HOME", "XDG_DATA_HOME", "XDG_CONFIG_HOME",
			"XDG_STATE_HOME", "XDG_CACHE_HOME", "SEXTILE_CHARTS_DIR", "SEXTILE_DEV_CHARTS_DIR",
			"SEXTILE_ATLAS_PATH"} {
			inside[k] = os.Getenv(k)
		}
		sandbox = os.Getenv("HOME")
		return 0
	})
	if code != 0 {
		t.Fatalf("Run returned %d", code)
	}

	if inside["SEXTILE_DEV_CHARTS_DIR"] != "" {
		t.Errorf("SEXTILE_DEV_CHARTS_DIR = %q inside the sandbox, want it cleared — "+
			"the dev tier is the repo's own ./test_charts, which is the operator's "+
			"local chart library", inside["SEXTILE_DEV_CHARTS_DIR"])
	}
	if got := inside["SEXTILE_CHARTS_DIR"]; got == "" || !strings.HasPrefix(got, sandbox) {
		t.Errorf("SEXTILE_CHARTS_DIR = %q, want a path inside the sandbox %q — "+
			"it is the TOP precedence tier, so leaving the operator's value "+
			"standing defeats every other redirect", got, sandbox)
	}
	for _, k := range []string{"XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		if !strings.HasPrefix(inside[k], sandbox) {
			t.Errorf("%s = %q, want it under the sandbox %q", k, inside[k], sandbox)
		}
	}
	if sandbox == "/operator/home" || sandbox == "" {
		t.Errorf("HOME = %q; the sandbox was never applied", sandbox)
	}

	// The sandbox is removed when the tests it wrapped finish.
	if _, err := os.Stat(sandbox); !os.IsNotExist(err) {
		t.Errorf("sandbox %s survives Run: stat err = %v", sandbox, err)
	}
	// The atlas is deliberately NOT redirected: it is read-only reference data
	// the .as translation genuinely needs, and there is no copy in a sandbox.
	if got := inside["SEXTILE_ATLAS_PATH"]; got != "/operator/atlas/atlas.as" {
		t.Errorf("SEXTILE_ATLAS_PATH = %q inside the sandbox, want it passed through: "+
			"tests need the real atlas", got)
	}
	_ = filepath.Join // keep the import honest if the assertions above change
}

// The launcher's contract variables steer which folder's backups and session a
// run uses (SEXTILE_CHARTS_HOST_PATH, under SEXTILE_LAUNCH) and what path the
// operator is shown (SEXTILE_BACKUP_HOST_PATH). A test run from inside a
// launched shell must not inherit them, or a backup test would key on the
// operator's real charts folder.
func TestRunClearsTheLauncherContract(t *testing.T) {
	for k, v := range map[string]string{
		"SEXTILE_LAUNCH":           "docker sextile:0.1.2",
		"SEXTILE_CHARTS_HOST_PATH": "/operator/real/client-charts",
		"SEXTILE_BACKUP_HOST_PATH": "/operator/home/.local/share/sextile/backup",
	} {
		t.Setenv(k, v)
	}
	inside := map[string]string{}
	Run(func() int {
		for _, k := range []string{"SEXTILE_LAUNCH", "SEXTILE_CHARTS_HOST_PATH", "SEXTILE_BACKUP_HOST_PATH"} {
			inside[k] = os.Getenv(k)
		}
		return 0
	})
	for k, v := range inside {
		if v != "" {
			t.Errorf("%s = %q inside the sandbox, want it cleared", k, v)
		}
	}
}
