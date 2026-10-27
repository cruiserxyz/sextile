package launcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// fakeGo stands in for the toolchain. `go version` answers; `go build ... -o
// OUT ...` appends the call to $FAKE_GO_LOG (its directory, CGO_ENABLED and
// arguments) and writes OUT, a script that prints what the app would see: its
// arguments and the environment the launcher gave it. FAKE_GO_FAIL=1 writes
// half a binary and fails, as an interrupted build would.
const fakeGo = `#!/usr/bin/env bash
if [ "$1" = version ]; then echo "go version go1.25.1 linux/amd64"; exit 0; fi
out=
prev=
for a in "$@"; do
  [ "$prev" = -o ] && out=$a
  prev=$a
done
{ printf 'build cwd=%s CGO_ENABLED=%s' "$PWD" "${CGO_ENABLED-unset}"; for a in "$@"; do printf ' [%s]' "$a"; done; printf '\n'; } >> "$FAKE_GO_LOG"
if [ "${FAKE_GO_FAIL-}" = 1 ]; then
  echo 'half a binary' > "$out"
  echo 'fake go: build failed' >&2
  exit 3
fi
IFS= read -r -d '' body <<'MARKER'
#!/usr/bin/env bash
echo MARKER-RAN
for v in SEXTILE_CHARTS_DIR SEXTILE_CHARTS_HOST_PATH XDG_DATA_HOME XDG_CONFIG_HOME XDG_STATE_HOME SEXTILE_PREVIEW_MODE SEXTILE_PREVIEW_ADDR SEXTILE_LAUNCH SEXTILE_ATLAS_PATH ASTR800 ASTROLOG ASTR SE_EPHE_PATH; do
  echo "$v=${!v-<unset>}"
done
echo "PATH=$PATH"
for a in "$@"; do echo "ARG=$a"; done
MARKER
printf '%s' "$body" > "$out"
chmod 755 "$out"
`

const fakeAstrolog = "#!/usr/bin/env bash\necho astrolog\n"

// withGo puts the fake go on PATH; FAKE_GO_LOG records its builds.
func (r *rig) withGo() {
	r.t.Helper()
	mustWrite(r.t, filepath.Join(r.fakebin, "go"), fakeGo, 0o755)
	r.setenv("FAKE_GO_LOG", filepath.Join(r.root, "go.log"))
}

// withAstrolog puts a fake astrolog on PATH, or, with home, only in
// ~/.local/bin (where scripts/build-astrolog installs it).
func (r *rig) withAstrolog(home bool) {
	r.t.Helper()
	dir := r.fakebin
	if home {
		dir = filepath.Join(r.home, ".local", "bin")
	}
	mustWrite(r.t, filepath.Join(dir, "astrolog"), fakeAstrolog, 0o755)
}

// withShare fills a share folder with the files doctor looks for, except those named.
func (r *rig) withShare(dir string, except ...string) {
	r.t.Helper()
	for _, f := range []string{"sepl_18.se1", "semo_18.se1", "seas_18.se1", "atlas.as"} {
		skip := false
		for _, e := range except {
			skip = skip || e == f
		}
		if !skip {
			mustWrite(r.t, filepath.Join(dir, f), "x\n", 0o644)
		}
	}
}

func (r *rig) goBuilds() []string {
	b, err := os.ReadFile(filepath.Join(r.root, "go.log"))
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

func (r *rig) noDockerCalls() {
	r.t.Helper()
	if c := r.calls(); len(c) != 0 {
		r.t.Fatalf("--no-docker must not run any docker command: %q", r.verbs())
	}
}

func (r *rig) nativeBin(tag string) string {
	return filepath.Join(r.home, ".cache", "sextile", "native", tag, "sextile")
}

// marker parses what the fake binary printed: NAME=value lines and ARG= lines.
func marker(t *testing.T, out string) (env map[string]string, args []string) {
	t.Helper()
	if !strings.HasPrefix(out, "MARKER-RAN\n") {
		t.Fatalf("the built binary did not run:\n%s", out)
	}
	env = map[string]string{}
	for _, l := range strings.Split(strings.TrimSuffix(out, "\n"), "\n")[1:] {
		if a, ok := strings.CutPrefix(l, "ARG="); ok {
			args = append(args, a)
		} else if k, v, ok := strings.Cut(l, "="); ok {
			env[k] = v
		}
	}
	return
}

// N1: --no-docker builds with the host's go once, runs that binary with the
// image's environment, rebuilds on --rebuild or a dirty tree, and never calls
// docker.
func TestNoDockerBuildsOnceAndRuns(t *testing.T) {
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	charts := r.mkdir("charts")
	p := strconv.Itoa(freePort(t))
	args := []string{"--no-docker", "--charts-dir", charts, "--port", p, "--no-open", "--", "--accessible", "a b", "--no-docker"}

	res := r.run(args...)
	r.mustSucceed(res)
	r.noDockerCalls()
	env, got := marker(t, res.stdout)
	for k, want := range map[string]string{
		"SEXTILE_CHARTS_DIR":       charts,
		"SEXTILE_CHARTS_HOST_PATH": charts,
		"XDG_DATA_HOME":            "<unset>",
		"XDG_CONFIG_HOME":          r.xdg,
		"XDG_STATE_HOME":           "<unset>",
		"SEXTILE_PREVIEW_MODE":     "serve",
		"SEXTILE_PREVIEW_ADDR":     "127.0.0.1:" + p,
		"SEXTILE_LAUNCH":           "no-docker v0.1.0-rc.1",
	} {
		if env[k] != want {
			t.Errorf("%s = %q, want %q", k, env[k], want)
		}
	}
	if strings.Join(got, "|") != "--accessible|a b|--no-docker" {
		t.Errorf("forwarded args = %q", got)
	}
	if strings.Contains(env["PATH"], ".local/bin") {
		t.Errorf("astrolog is on PATH, so PATH stays as it was: %q", env["PATH"])
	}
	builds := r.goBuilds()
	if len(builds) != 1 {
		t.Fatalf("the first run builds once: %q", builds)
	}
	want := regexp.MustCompile(`^build cwd=` + regexp.QuoteMeta(r.clone) + ` CGO_ENABLED=0 \[build\] \[-trimpath\] \[-ldflags\] \[-X main\.version=v0\.1\.0-rc\.1\] \[-o\] \[` +
		regexp.QuoteMeta(filepath.Dir(r.nativeBin("v0.1.0-rc.1"))) + `/\.sextile\.\d+\] \[\./cmd/sextile\]$`)
	if !want.MatchString(builds[0]) {
		t.Errorf("go was called as %q", builds[0])
	}
	if _, err := os.Stat(r.nativeBin("v0.1.0-rc.1")); err != nil {
		t.Fatalf("the binary is not in the cache: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Dir(r.nativeBin("v0.1.0-rc.1")))
	if len(entries) != 2 || entries[0].Name() != ".built-by-sextile-launcher" || entries[1].Name() != "sextile" {
		t.Errorf("the temporary build must be moved away, leaving the binary and the launcher's marker: %v", entries)
	}

	// A second run reuses it.
	r.mustSucceed(r.run(args...))
	if n := len(r.goBuilds()); n != 1 {
		t.Fatalf("a second run must not build again, go was called %d times", n)
	}
	// --rebuild builds again.
	r.mustSucceed(r.run(append([]string{"--rebuild"}, args...)...))
	if n := len(r.goBuilds()); n != 2 {
		t.Fatalf("--rebuild builds again, go was called %d times", n)
	}
	r.noDockerCalls()
}

func TestNoDockerDirtyTreeRebuildsAndSaysSo(t *testing.T) {
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	mustWrite(t, filepath.Join(r.clone, "marker.txt"), "edited\n", 0o644)
	args := []string{"--no-docker", "--charts-dir", r.mkdir("c"), "--no-open"}
	for i := 1; i <= 2; i++ {
		res := r.run(args...)
		r.mustSucceed(res)
		contains(t, res.stderr, "native build of v0.1.0-rc.1-dirty, uncommitted edits included\n")
		if n := len(r.goBuilds()); n != i {
			t.Fatalf("run %d: a dirty tree builds every start, go was called %d times", i, n)
		}
		env, _ := marker(t, res.stdout)
		if env["SEXTILE_LAUNCH"] != "no-docker v0.1.0-rc.1-dirty" {
			t.Errorf("SEXTILE_LAUNCH = %q", env["SEXTILE_LAUNCH"])
		}
	}
	r.noDockerCalls()
}

func TestNoDockerWithoutHistoryRebuildsAndSaysSo(t *testing.T) {
	r := newRig(t, plainClone())
	r.withGo()
	r.withAstrolog(false)
	args := []string{"--no-docker", "--charts-dir", r.mkdir("c"), "--no-open"}
	for i := 1; i <= 2; i++ {
		res := r.run(args...)
		r.mustSucceed(res)
		contains(t, res.stderr, "native build of local (no git history, so every start rebuilds)\n")
		if n := len(r.goBuilds()); n != i {
			t.Fatalf("run %d: go was called %d times", i, n)
		}
	}
	if _, err := os.Stat(r.nativeBin("local")); err != nil {
		t.Fatal(err)
	}
}

// astrolog only in ~/.local/bin is found, goes first on the app's PATH, and
// the atlas it installed is named to the app.
func TestNoDockerFindsAstrologInLocalBin(t *testing.T) {
	r := newRig(t)
	r.withGo()
	r.withAstrolog(true)
	r.withShare(filepath.Join(r.home, ".local", "share", "astrolog"))
	res := r.run("--no-docker", "--charts-dir", r.mkdir("c"), "--no-open")
	r.mustSucceed(res)
	env, _ := marker(t, res.stdout)
	if !strings.HasPrefix(env["PATH"], filepath.Join(r.home, ".local", "bin")+":") {
		t.Errorf("~/.local/bin must come first on the app's PATH: %q", env["PATH"])
	}
	if want := filepath.Join(r.home, ".local", "share", "astrolog", "atlas.as"); env["SEXTILE_ATLAS_PATH"] != want {
		t.Errorf("SEXTILE_ATLAS_PATH = %q, want %q", env["SEXTILE_ATLAS_PATH"], want)
	}
	r.noDockerCalls()
}

// The native run no longer points the app's XDG folders into the charts
// folder: the person's own XDG values reach the app unchanged (the app then
// keeps its session, preferences and backups in the XDG places), and the app is
// told the physical charts folder so it can tell one folder from another.
func TestNoDockerLeavesXDGAlone(t *testing.T) {
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	real := r.mkdir("real charts")
	link := filepath.Join(r.root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	st, cf, da := r.mkdir("x/state"), r.mkdir("x/config"), r.mkdir("x/data")
	r.setenv("XDG_STATE_HOME", st)
	r.setenv("XDG_CONFIG_HOME", cf)
	r.setenv("XDG_DATA_HOME", da)
	res := r.run("--no-docker", "--charts-dir", link, "--no-open")
	r.mustSucceed(res)
	env, _ := marker(t, res.stdout)
	for k, want := range map[string]string{
		"XDG_STATE_HOME":           st,
		"XDG_CONFIG_HOME":          cf,
		"XDG_DATA_HOME":            da,
		"SEXTILE_CHARTS_DIR":       real,
		"SEXTILE_CHARTS_HOST_PATH": real,
	} {
		if env[k] != want {
			t.Errorf("%s = %q, want %q", k, env[k], want)
		}
	}
	if _, err := os.Lstat(filepath.Join(real, ".sextile")); err == nil {
		t.Error("the charts folder must not get a .sextile folder from the launcher")
	}
	r.noDockerCalls()
}

// An atlas path left in the caller's environment never reaches the native
// app: the launcher sets it from the share folder it found, or clears it.
func TestNoDockerClearsAnInheritedAtlasPath(t *testing.T) {
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	r.setenv("SEXTILE_ATLAS_PATH", "/old/astrolog/atlas.as")
	res := r.run("--no-docker", "--charts-dir", r.mkdir("c"), "--no-open")
	r.mustSucceed(res)
	env, _ := marker(t, res.stdout)
	if v := env["SEXTILE_ATLAS_PATH"]; v != "<unset>" {
		t.Errorf("with no share folder found, SEXTILE_ATLAS_PATH reached the app as %q; want it unset", v)
	}
	r.withShare(filepath.Join(r.home, ".local", "share", "astrolog"))
	res = r.run("--no-docker", "--charts-dir", r.mkdir("c2"), "--no-open")
	r.mustSucceed(res)
	env, _ = marker(t, res.stdout)
	if want := filepath.Join(r.home, ".local", "share", "astrolog", "atlas.as"); env["SEXTILE_ATLAS_PATH"] != want {
		t.Errorf("SEXTILE_ATLAS_PATH = %q, want the found share's %q", env["SEXTILE_ATLAS_PATH"], want)
	}
	r.noDockerCalls()
}

// An interrupted or failed build never leaves a binary that a later run trusts.
func TestNoDockerFailedBuildLeavesNothingToTrust(t *testing.T) {
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	args := []string{"--no-docker", "--charts-dir", r.mkdir("c"), "--no-open"}
	r.setenv("FAKE_GO_FAIL", "1")
	res := r.run(args...)
	if res.code != 3 {
		t.Fatalf("exit %d, want go's status 3\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, "fake go: build failed")
	contains(t, res.stderr, "the native build failed (exit 3) — Sextile was not started")
	entries, _ := os.ReadDir(filepath.Dir(r.nativeBin("v0.1.0-rc.1")))
	if len(entries) != 0 {
		t.Fatalf("a failed build left %d entries in the cache folder", len(entries))
	}
	r.setenv("FAKE_GO_FAIL", "")
	r.mustSucceed(r.run(args...))
	if n := len(r.goBuilds()); n != 2 {
		t.Fatalf("the next run builds, go was called %d times", n)
	}
	r.noDockerCalls()
}

// N2: a missing piece is one refusal that names it, exit 2, no docker command
// and no fall back, even though a working docker is on PATH.
func TestNoDockerMissingPieceRefused(t *testing.T) {
	const tail = ". It needs go (Go 1.25 or newer) on PATH, and astrolog on PATH or in ~/.local/bin (scripts/build-astrolog installs Astrolog, about two minutes)\n"
	cases := []struct {
		name     string
		goOn     bool
		astroOn  bool
		missing  string
		homeOnly bool
	}{
		{"go absent", false, true, "go", false},
		{"astrolog absent", true, false, "astrolog", false},
		{"both absent", false, false, "go and astrolog", false},
	}
	for _, c := range cases {
		r := newRig(t) // a fake docker that would answer, and record any call
		if c.goOn {
			r.withGo()
		}
		if c.astroOn {
			r.withAstrolog(false)
		}
		res := r.run("--no-docker", "--charts-dir", r.mkdir("c"), "--no-open")
		r.mustExit2(res)
		if want := "--no-docker did not find " + c.missing + tail; res.stderr != want {
			t.Errorf("%s: stderr\n got %q\nwant %q", c.name, res.stderr, want)
		}
		if res.stdout != "" {
			t.Errorf("%s: stdout %q", c.name, res.stdout)
		}
		r.noDockerCalls()
		if _, err := os.Stat(filepath.Join(r.home, ".cache")); err == nil {
			t.Errorf("%s: nothing is built or created when a piece is missing", c.name)
		}
	}
}

func TestNoDockerFlagErrors(t *testing.T) {
	for _, c := range [][]string{
		{"--no-docker", "--no-docker"},
		{"doctor", "--no-docker"},
	} {
		r := newRig(t)
		r.mustExit2(r.run(c...))
		r.noDockerCalls()
	}
	// After `--` it is the app's argument, like any other.
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	res := r.run("--no-docker", "--charts-dir", r.mkdir("c"), "--no-open", "--", "--no-docker")
	r.mustSucceed(res)
	if _, args := marker(t, res.stdout); strings.Join(args, "|") != "--no-docker" {
		t.Errorf("args = %q", args)
	}
}

func TestNoDockerRefusesASymlinkedBuildFolder(t *testing.T) {
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	elsewhere := r.mkdir("elsewhere")
	if err := os.MkdirAll(filepath.Join(r.home, ".cache", "sextile"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(r.home, ".cache", "sextile", "native")
	if err := os.Symlink(elsewhere, link); err != nil {
		t.Fatal(err)
	}
	res := r.run("--no-docker", "--charts-dir", r.mkdir("c"), "--no-open")
	r.mustExit2(res)
	contains(t, res.stderr, link+" is a symlink")
	if n := len(r.goBuilds()); n != 0 {
		t.Fatalf("go ran %d times", n)
	}
	// And a cache folder inside the checkout is refused.
	r2 := newRig(t)
	r2.withGo()
	r2.withAstrolog(false)
	r2.setenv("XDG_CACHE_HOME", filepath.Join(r2.clone, "cache"))
	res = r2.run("--no-docker", "--charts-dir", r2.mkdir("c"), "--no-open")
	r2.mustExit2(res)
	contains(t, res.stderr, "is inside the Sextile checkout")
}

// N3: doctor's --no-docker rows say what is there and what is missing, and
// never change its exit status.
func TestDoctorNativeRows(t *testing.T) {
	rows := func(out string) []string {
		lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
		for i, l := range lines {
			if l == "for --no-docker (optional):" {
				return lines[i+1:]
			}
		}
		t.Fatalf("no --no-docker block in:\n%s", out)
		return nil
	}
	names := []string{"go", "astrolog", "astrolog files"}

	// Everything there.
	r := newRig(t)
	r.withGo()
	r.withAstrolog(true)
	share := filepath.Join(r.home, ".local", "share", "astrolog")
	r.withShare(share)
	r.writeConf("charts-dir=" + r.mkdir("kept") + "\n")
	res := r.run("doctor", "--port", strconv.Itoa(freePort(t)))
	r.mustSucceed(res)
	got := rows(res.stdout)
	if len(got) != 3 {
		t.Fatalf("want 3 rows:\n%s", res.stdout)
	}
	for i, n := range names {
		if !regexp.MustCompile(`^ok +` + n + `: `).MatchString(got[i]) {
			t.Errorf("row %d should be ok for %q: %q", i, n, got[i])
		}
	}
	contains(t, got[0], r.fakebin+"/go (go version go1.25.1 linux/amd64)")
	contains(t, got[1], r.home+"/.local/bin/astrolog (not on PATH")
	contains(t, got[2], "sepl_18.se1, semo_18.se1, seas_18.se1 and atlas.as")

	// Nothing there, and one file missing.
	r = newRig(t)
	r.writeConf("charts-dir=" + r.mkdir("kept") + "\n")
	res = r.run("doctor", "--port", strconv.Itoa(freePort(t)))
	r.mustSucceed(res) // exit 0 with nothing for --no-docker
	got = rows(res.stdout)
	for i, n := range names {
		if !regexp.MustCompile(`^note +` + n + `: missing`).MatchString(got[i]) {
			t.Errorf("row %d should be a missing note for %q: %q", i, n, got[i])
		}
	}
	contains(t, got[1], "scripts/build-astrolog")

	r = newRig(t)
	r.withGo()
	r.withAstrolog(false)
	r.withShare(filepath.Join(r.sys, "usr", "share", "astrolog"), "semo_18.se1")
	r.writeConf("charts-dir=" + r.mkdir("kept") + "\n")
	res = r.run("doctor", "--port", strconv.Itoa(freePort(t)))
	r.mustSucceed(res)
	got = rows(res.stdout)
	if !regexp.MustCompile(`^note +astrolog files: missing semo_18.se1 in `).MatchString(got[2]) {
		t.Errorf("a missing file is named: %q", got[2])
	}
	contains(t, got[1], "(on PATH)")

	// A failing doctor fails the same with or without them.
	for _, with := range []bool{false, true} {
		r = newRig(t)
		if with {
			r.withGo()
			r.withAstrolog(false)
		}
		r.removeDocker()
		r.writeConf("charts-dir=" + r.mkdir("kept") + "\n")
		res = r.run("doctor", "--port", strconv.Itoa(freePort(t)))
		if res.code != 1 {
			t.Errorf("with=%v: doctor without docker exits %d, want 1", with, res.code)
		}
		for _, l := range rows(res.stdout) {
			if strings.HasPrefix(l, "FAIL") {
				t.Errorf("a --no-docker row must never be FAIL: %q", l)
			}
		}
	}
}

// SEXTILE_TEST_SYSROOT is for these tests only: the old name SEXTILE_SYSROOT
// is no longer read, so a fixture root set under it is not used.
func TestNoDockerIgnoresTheOldSysrootName(t *testing.T) {
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	r.withShare(filepath.Join(r.sys, "usr", "share", "astrolog"))
	r.setenv("SEXTILE_TEST_SYSROOT", r.mkdir("empty-root"))
	r.setenv("SEXTILE_SYSROOT", r.sys)
	res := r.run("--no-docker", "--charts-dir", r.mkdir("c"), "--no-open")
	r.mustSucceed(res)
	env, _ := marker(t, res.stdout)
	for _, k := range []string{"SEXTILE_ATLAS_PATH", "ASTROLOG"} {
		if env[k] != "<unset>" {
			t.Errorf("%s = %q: the share under SEXTILE_SYSROOT was used", k, env[k])
		}
	}
}

// Astrolog's own search variables in the caller's environment never reach the
// native app, as the image sets none of them; ASTROLOG is the found share.
func TestNoDockerClearsAstrologsOwnEnvironment(t *testing.T) {
	stale := []string{"ASTR800", "ASTROLOG", "ASTR", "SE_EPHE_PATH"}
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	for _, k := range stale {
		r.setenv(k, "/old/astrolog")
	}
	res := r.run("--no-docker", "--charts-dir", r.mkdir("c"), "--no-open")
	r.mustSucceed(res)
	env, _ := marker(t, res.stdout)
	for _, k := range stale {
		if env[k] != "<unset>" {
			t.Errorf("with no share folder found, %s reached the app as %q; want it unset", k, env[k])
		}
	}
	share := filepath.Join(r.home, ".local", "share", "astrolog")
	r.withShare(share)
	res = r.run("--no-docker", "--charts-dir", r.mkdir("c2"), "--no-open")
	r.mustSucceed(res)
	env, _ = marker(t, res.stdout)
	if env["ASTROLOG"] != share {
		t.Errorf("ASTROLOG = %q, want the found share %q", env["ASTROLOG"], share)
	}
	for _, k := range []string{"ASTR800", "ASTR", "SE_EPHE_PATH"} {
		if env[k] != "<unset>" {
			t.Errorf("%s reached the app as %q; want it unset", k, env[k])
		}
	}
	r.noDockerCalls()
}

const nativeMark = ".built-by-sextile-launcher"

// olderBuild makes a build folder under the native cache as the launcher
// would have, plus any extra files.
func (r *rig) olderBuild(tag string, files ...string) string {
	r.t.Helper()
	dir := filepath.Dir(r.nativeBin(tag))
	for _, f := range files {
		mustWrite(r.t, filepath.Join(dir, f), "old\n", 0o755)
	}
	return dir
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// After a successful build only, the other build folders the launcher made are
// removed; anything else is left alone, and a reused build prunes nothing.
func TestNoDockerPrunesOlderLauncherBuilds(t *testing.T) {
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	args := []string{"--no-docker", "--charts-dir", r.mkdir("c"), "--no-open"}
	made := r.olderBuild("v0.0.9", "sextile", nativeMark)
	noMark := r.olderBuild("v0.0.8", "sextile")
	extra := r.olderBuild("v0.0.7", "sextile", nativeMark, "notes.txt")
	odd := r.olderBuild("v 0.0.6", "sextile", nativeMark)
	elsewhere := r.mkdir("elsewhere") // a launcher-looking folder outside the native cache
	mustWrite(t, filepath.Join(elsewhere, "sextile"), "old\n", 0o755)
	mustWrite(t, filepath.Join(elsewhere, nativeMark), "", 0o644)
	link := filepath.Join(filepath.Dir(made), "linked")
	if err := os.Symlink(elsewhere, link); err != nil {
		t.Fatal(err)
	}
	sub := r.olderBuild("v0.0.5", "sextile", nativeMark)
	mustMkdir(t, filepath.Join(sub, "nested"))

	// A failed build prunes nothing.
	r.setenv("FAKE_GO_FAIL", "1")
	res := r.run(args...)
	if res.code != 3 {
		t.Fatalf("exit %d, want go's status 3\n%s", res.code, res.stderr)
	}
	if strings.Contains(res.stderr, "older native builds") || !exists(made) {
		t.Fatalf("a failed build must not prune: made folder exists=%v\n%s", exists(made), res.stderr)
	}
	r.setenv("FAKE_GO_FAIL", "")

	res = r.run(args...)
	r.mustSucceed(res)
	contains(t, res.stderr, "removed 1 older native builds\n")
	if exists(made) {
		t.Errorf("the launcher-made older build %s was not removed", made)
	}
	for name, dir := range map[string]string{"no marker": noMark, "extra file": extra, "odd name": odd, "symlink target": elsewhere, "nested folder": sub} {
		if !exists(filepath.Join(dir, "sextile")) {
			t.Errorf("%s: %s must be left alone", name, dir)
		}
	}
	if !exists(link) || !exists(filepath.Join(elsewhere, nativeMark)) {
		t.Errorf("the symlink and its target must be left alone")
	}
	cur := r.nativeBin("v0.1.0-rc.1")
	if !exists(cur) || !exists(filepath.Join(filepath.Dir(cur), nativeMark)) {
		t.Errorf("the current build and its marker must exist")
	}

	// A second run reuses the build, and prunes nothing.
	again := r.olderBuild("v0.0.4", "sextile", nativeMark)
	res = r.run(args...)
	r.mustSucceed(res)
	if n := len(r.goBuilds()); n != 2 {
		t.Fatalf("the second run must reuse the build, go was called %d times in all", n)
	}
	if strings.Contains(res.stderr, "older native builds") || !exists(again) {
		t.Errorf("a run that reuses the build must not prune (exists=%v)\n%s", exists(again), res.stderr)
	}
	// A rebuild does.
	res = r.run(append([]string{"--rebuild"}, args...)...)
	r.mustSucceed(res)
	contains(t, res.stderr, "removed 1 older native builds\n")
	if exists(again) {
		t.Errorf("--rebuild must prune the older build %s", again)
	}
	r.noDockerCalls()
}

// A folder that cannot be emptied is one line, not a stop.
func TestNoDockerPruneFailureIsNotFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can remove from a read-only folder")
	}
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	stuck := r.olderBuild("v0.0.9", "sextile", nativeMark)
	if err := os.Chmod(stuck, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(stuck, 0o755) })
	res := r.run("--no-docker", "--charts-dir", r.mkdir("c"), "--no-open")
	r.mustSucceed(res)
	contains(t, res.stderr, "could not remove 1 older native builds. They stay for now\n")
	if !exists(filepath.Join(stuck, "sextile")) {
		t.Errorf("the stuck build is left in place")
	}
	marker(t, res.stdout)
}

// hook writes a test-only SEXTILE_TEST_PRUNE_HOOK script into the rig and sets
// it; body runs with the candidate folder as $1. ln is called by its absolute
// path, as the rig's PATH holds only the tools the launcher itself uses.
func (r *rig) hook(body string) {
	r.t.Helper()
	ln, err := exec.LookPath("ln")
	if err != nil {
		r.t.Skip("no ln")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		r.t.Skip("no bash")
	}
	path := filepath.Join(r.root, "prune-hook")
	mustWrite(r.t, path, "#!"+bash+"\nln() { "+ln+" \"$@\"; }\n"+body, 0o755)
	r.setenv("SEXTILE_TEST_PRUNE_HOOK", path)
}

// A candidate folder swapped for a symlink after the launcher inspected it
// must not redirect the removal: the two files in the symlink's target stay.
func TestNoDockerPruneCannotBeRedirectedByASwappedFolder(t *testing.T) {
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	victim := r.olderBuild("v0.0.9", "sextile", nativeMark)
	outside := r.mkdir("outside")
	mustWrite(t, filepath.Join(outside, "sextile"), "precious\n", 0o755)
	mustWrite(t, filepath.Join(outside, nativeMark), "", 0o644)
	// The hook runs after the checks: it moves the folder away and puts a
	// symlink to the outside folder in its place.
	r.hook(`[ "$1" = ` + victim + ` ] || exit 0
mv "$1" "$1.moved"
ln -s ` + outside + ` "$1"
`)
	res := r.run("--no-docker", "--charts-dir", r.mkdir("c"), "--no-open")
	r.mustSucceed(res)
	for _, f := range []string{"sextile", nativeMark} {
		if !exists(filepath.Join(outside, f)) {
			t.Errorf("%s in the folder the swapped symlink points at was removed", f)
		}
	}
	if !exists(victim) {
		t.Errorf("the swapped-in symlink must stay")
	}
	contains(t, res.stderr, "could not remove 1 older native builds. They stay for now\n")
}

// A folder holding the marker alone, or the binary alone, is not a complete
// launcher build and is left alone.
func TestNoDockerPruneLeavesIncompleteBuilds(t *testing.T) {
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	markerOnly := r.olderBuild("v0.0.9", nativeMark)
	binaryOnly := r.olderBuild("v0.0.8", "sextile")
	empty := r.olderBuild("v0.0.7")
	mustMkdir(t, empty)
	res := r.run("--no-docker", "--charts-dir", r.mkdir("c"), "--no-open")
	r.mustSucceed(res)
	if strings.Contains(res.stderr, "older native builds") {
		t.Errorf("nothing qualified, yet: %s", res.stderr)
	}
	for name, p := range map[string]string{"marker only": filepath.Join(markerOnly, nativeMark), "binary only": filepath.Join(binaryOnly, "sextile")} {
		if !exists(p) {
			t.Errorf("%s: %s was removed", name, p)
		}
	}
	if !exists(empty) {
		t.Errorf("an empty folder is not ours and stays")
	}
}

// If another window's pruning removes the build between this launcher's build
// and its exec, the launcher says so in one line instead of failing in exec.
func TestNoDockerSaysWhenTheBuildVanishedBeforeRunning(t *testing.T) {
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	r.olderBuild("v0.0.9", "sextile", nativeMark)
	cur := r.nativeBin("v0.1.0-rc.1")
	r.hook(`rm -f ` + cur + "\n")
	res := r.run("--no-docker", "--charts-dir", r.mkdir("c"), "--no-open")
	r.mustExit2(res)
	if want := "the native build " + cur + " is gone (another sextile may have removed it). Run sextile --no-docker again\n"; !strings.HasSuffix(res.stderr, want) {
		t.Errorf("stderr %q does not end with %q", res.stderr, want)
	}
	if res.stdout != "" {
		t.Errorf("the app must not have run: %q", res.stdout)
	}
}
