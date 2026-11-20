//go:build linux

package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The launcher reads launcher-originals.conf and never writes it: Sextile is
// its one writer. These tests run the script against the stand-in docker and
// read what it was told to start Sextile with.

func (r *rig) originalsPath() string {
	return filepath.Join(r.xdg, "sextile", "launcher-originals.conf")
}

func (r *rig) writeOriginals(content string) {
	r.t.Helper()
	mustWrite(r.t, r.originalsPath(), content, 0o600)
}

func hasEnv(call []string, kv string) bool {
	for _, e := range envs(call) {
		if e == kv {
			return true
		}
	}
	return false
}

func hasEnvName(call []string, name string) bool {
	for _, e := range envs(call) {
		if strings.HasPrefix(e, name+"=") {
			return true
		}
	}
	return false
}

// chartsMount is the --mount value that carries the charts folder.
func chartsMount(t *testing.T, call []string) string {
	t.Helper()
	for _, m := range mounts(call) {
		if strings.Contains(m, ",target=/charts") {
			return m
		}
	}
	t.Fatalf("no charts mount in %q", call)
	return ""
}

func readonlyMount(t *testing.T, call []string) bool {
	t.Helper()
	m := chartsMount(t, call)
	return strings.HasSuffix(m, ",target=/charts,readonly")
}

// noPositionFile fails when the launcher left anything named for the position
// file beside the config.
func (r *rig) noPositionFile() {
	r.t.Helper()
	ents, _ := os.ReadDir(filepath.Join(r.xdg, "sextile"))
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "launcher-originals") || strings.HasPrefix(e.Name(), ".launcher-originals") {
			r.t.Fatalf("the launcher made %s", e.Name())
		}
	}
}

// startsFresh makes a rig whose person is asked for a charts folder and
// answers with path.
func (r *rig) startsFresh(path string, args ...string) (string, int) {
	r.t.Helper()
	prepareHome(r.t, r, true)
	return r.answer(args, [2]string{choose, "3\n"}, [2]string{"Folder: ", path + "\n"})
}

func TestLauncherRecordsItsOwnFolder(t *testing.T) {
	r := newRig(t)
	target := filepath.Join(r.root, "made", "charts")
	out, code := r.startsFresh(target)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	run := r.find("run")
	if !hasEnv(run, "SEXTILE_ORIGINALS_CREATED=1") {
		t.Fatalf("a folder the launcher made is told to Sextile: %q", run)
	}
	if hasEnvName(run, "SEXTILE_ORIGINALS_SET") {
		t.Fatalf("no word was given: %q", run)
	}
	r.noPositionFile()
}

func TestLauncherLeavesAFoundFolderUnrecorded(t *testing.T) {
	r := newRig(t)
	found := r.mkdir("found")
	out, code := r.startsFresh(found)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	run := r.find("run")
	if hasEnvName(run, "SEXTILE_ORIGINALS_CREATED") || hasEnvName(run, "SEXTILE_ORIGINALS_SET") {
		t.Fatalf("a folder that existed is not recorded: %q", run)
	}

	r2 := newRig(t)
	r2.mustSucceed(r2.run("--charts-dir", r2.mkdir("c")))
	if run := r2.find("run"); hasEnvName(run, "SEXTILE_ORIGINALS_CREATED") {
		t.Fatalf("--charts-dir names a folder that exists: %q", run)
	}
}

func TestOriginalsFlagIsRemembered(t *testing.T) {
	for _, w := range []string{"protected", "ask", "add", "allow"} {
		r := newRig(t)
		r.mustSucceed(r.run("--originals", w, "--charts-dir", r.mkdir("c")))
		run := r.find("run")
		if !hasEnv(run, "SEXTILE_ORIGINALS_SET="+w) {
			t.Fatalf("%s: Sextile is handed the word: %q", w, run)
		}
		if hasEnvName(run, "SEXTILE_ORIGINALS_CREATED") {
			t.Fatalf("%s: the folder existed: %q", w, run)
		}
		if readonlyMount(t, run) != (w == "protected") {
			t.Fatalf("%s: read-only exactly at protected: %q", w, chartsMount(t, run))
		}
		r.noPositionFile()
	}

	// =WORD, and the same through --no-docker.
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	res := r.run("--no-docker", "--no-open", "--originals=add", "--charts-dir", r.mkdir("c"))
	r.mustSucceed(res)
	env, _ := marker(t, res.stdout)
	if env["SEXTILE_ORIGINALS_SET"] != "add" || env["SEXTILE_CHARTS_READONLY"] != "<unset>" {
		t.Fatalf("native run: %v", env)
	}

	// The word wins over a folder the launcher made in this run.
	r3 := newRig(t)
	out, code := r3.startsFresh(filepath.Join(r3.root, "made"), "--originals", "allow")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	run := r3.find("run")
	if !hasEnv(run, "SEXTILE_ORIGINALS_SET=allow") || hasEnvName(run, "SEXTILE_ORIGINALS_CREATED") {
		t.Fatalf("the word outranks the made folder: %q", run)
	}
}

func TestOriginalsFlagSaysWhatItMeans(t *testing.T) {
	var lines []string
	for _, w := range []string{"protected", "ask", "add", "allow"} {
		r := newRig(t)
		res := r.run("--originals", w, "--charts-dir", r.mkdir("c"))
		r.mustSucceed(res)
		var said []string
		for _, l := range strings.Split(res.stderr, "\n") {
			if strings.HasPrefix(l, "charts folder: ") {
				said = append(said, l)
			}
		}
		if len(said) != 1 {
			t.Fatalf("%s: one line says what it means, got %q", w, said)
		}
		lines = append(lines, said[0])
	}
	golden(t, "originals_flag_lines", strings.Join(lines, "\n")+"\n")
}

func TestOriginalsFlagRefusesUnknownWord(t *testing.T) {
	for _, args := range [][]string{
		{"--originals", "bogus"},
		{"--originals"},
		{"--originals", "--moons"},
		{"--originals=Protected"},
		{"--originals", "ask", "--originals", "add"},
	} {
		r := newRig(t)
		before := snapshot(t, r.xdg)
		res := r.run(append(args, "--charts-dir", r.mkdir("c"))...)
		r.mustExit2(res)
		if strings.Contains(strings.Join(args, " "), "ask --originals") {
			contains(t, res.stderr, "--originals was given twice")
		} else {
			contains(t, res.stderr, "protected, ask, add or allow")
		}
		r.noBuildOrRun()
		if n := len(r.calls()); n != 0 {
			t.Fatalf("%q: docker was called: %q", args, r.verbs())
		}
		if d := diffSnapshots(before, snapshot(t, r.xdg)); len(d) != 0 {
			t.Fatalf("%q: something was written: %v", args, d)
		}
	}
}

func TestProtectedFolderIsMountedReadOnly(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		line  string // the word on the folder's line; "" for no line
		wantR bool
	}{
		{"the flag", []string{"--originals", "protected"}, "", true},
		{"its line", nil, "protected", true},
		{"no line", nil, "", false},
		{"ask", nil, "ask", false},
		{"add", nil, "add", false},
		{"allow", nil, "allow", false},
		{"the flag outranks its line", []string{"--originals", "allow"}, "protected", false},
		{"the flag locks over an open line", []string{"--originals", "protected"}, "allow", true},
	}
	for _, c := range cases {
		r := newRig(t)
		charts := r.mkdir("c")
		if c.line != "" {
			r.writeOriginals("# kept\n" + charts + "=" + c.line + "\nother=allow\n")
		}
		r.mustSucceed(r.run(append(c.args, "--charts-dir", charts)...))
		run := r.find("run")
		if readonlyMount(t, run) != c.wantR {
			t.Fatalf("%s: mount %q", c.name, chartsMount(t, run))
		}
		if c.wantR {
			if chartsMount(t, run) != "type=bind,source="+charts+",target=/charts,readonly" {
				t.Fatalf("%s: mount %q", c.name, chartsMount(t, run))
			}
		} else if chartsMount(t, run) != "type=bind,source="+charts+",target=/charts" {
			t.Fatalf("%s: mount %q", c.name, chartsMount(t, run))
		}
		if hasEnv(run, "SEXTILE_CHARTS_READONLY=1") != c.wantR {
			t.Fatalf("%s: environment %q", c.name, envs(run))
		}
	}

	// The mount source is the physical path of a folder reached by a link.
	lr := newRig(t)
	lreal := lr.mkdir("real")
	llink := filepath.Join(lr.root, "link")
	if err := os.Symlink(lreal, llink); err != nil {
		t.Fatal(err)
	}
	lr.mustSucceed(lr.run("--originals", "protected", "--charts-dir", llink))
	if got := chartsMount(t, lr.find("run")); got != "type=bind,source="+lreal+",target=/charts,readonly" {
		t.Fatalf("the mount is %q, want the physical path %q as its source", got, lreal)
	}

	// Without Docker the variable is the lock.
	for _, protected := range []bool{true, false} {
		r := newRig(t)
		r.withGo()
		r.withAstrolog(false)
		charts := r.mkdir("c")
		if protected {
			r.writeOriginals(charts + "=protected\n")
		}
		res := r.run("--no-docker", "--no-open", "--charts-dir", charts)
		r.mustSucceed(res)
		env, _ := marker(t, res.stdout)
		want := "<unset>"
		if protected {
			want = "1"
		}
		if env["SEXTILE_CHARTS_READONLY"] != want {
			t.Fatalf("native protected=%v: %v", protected, env)
		}
	}

	// A caller's own variable never reaches the app unasked.
	r := newRig(t)
	r.withGo()
	r.withAstrolog(false)
	r.setenv("SEXTILE_CHARTS_READONLY", "1")
	r.setenv("SEXTILE_ORIGINALS_SET", "allow")
	res := r.run("--no-docker", "--no-open", "--charts-dir", r.mkdir("c"))
	r.mustSucceed(res)
	env, _ := marker(t, res.stdout)
	if env["SEXTILE_CHARTS_READONLY"] != "<unset>" || env["SEXTILE_ORIGINALS_SET"] != "<unset>" {
		t.Fatalf("the caller's variables leaked: %v", env)
	}
}

func TestLauncherFailsClosed(t *testing.T) {
	notice := func(fault string) string {
		return "charts folder protected: launcher-originals.conf " + fault + " · this run writes nothing in it"
	}
	cases := []struct {
		name  string
		setup func(r *rig, charts string)
		fault string
	}{
		{"two lines", func(r *rig, c string) { r.writeOriginals(c + "=ask\n" + c + "=allow\n") }, "has 2 lines for it"},
		{"an unknown word", func(r *rig, c string) { r.writeOriginals(c + "=open\n") }, `says "open" for it`},
		{"a NUL byte after a word", func(r *rig, c string) { r.writeOriginals(c + "=allow\x00\n") }, "cannot be read"},
		{"a folder, not a file", func(r *rig, c string) { mustMkdir(r.t, r.originalsPath()) }, "is not a file"},
		{"a link", func(r *rig, c string) {
			mustWrite(r.t, filepath.Join(r.root, "elsewhere"), c+"=allow\n", 0o600)
			mustMkdir(r.t, filepath.Dir(r.originalsPath()))
			if err := os.Symlink(filepath.Join(r.root, "elsewhere"), r.originalsPath()); err != nil {
				r.t.Fatal(err)
			}
		}, "is not a file"},
	}
	for _, c := range cases {
		r := newRig(t)
		charts := r.mkdir("c")
		c.setup(r, charts)
		watch := []string{r.xdg}
		if _, err := os.Lstat(filepath.Join(r.root, "elsewhere")); err == nil {
			watch = append(watch, filepath.Join(r.root, "elsewhere"))
		}
		before := snapshot(t, watch...)
		res := r.run("--charts-dir", charts)
		r.mustSucceed(res)
		contains(t, res.stderr, notice(c.fault))
		run := r.find("run")
		if !readonlyMount(t, run) || !hasEnv(run, "SEXTILE_CHARTS_READONLY=1") {
			t.Fatalf("%s: protected means a read-only mount: %q", c.name, run)
		}
		if hasEnvName(run, "SEXTILE_ORIGINALS_SET") || hasEnvName(run, "SEXTILE_ORIGINALS_CREATED") {
			t.Fatalf("%s: nothing is handed over: %q", c.name, run)
		}
		if d := diffSnapshots(before, snapshot(t, watch...)); len(d) != 0 {
			t.Fatalf("%s: the file was touched: %v", c.name, d)
		}
	}

	// Without Docker a faulty line reaches the process as the lock.
	nr := newRig(t)
	nr.withGo()
	nr.withAstrolog(false)
	nc := nr.mkdir("c")
	nr.writeOriginals(nc + "=ask\n" + nc + "=allow\n")
	nres := nr.run("--no-docker", "--no-open", "--charts-dir", nc)
	nr.mustSucceed(nres)
	if env, _ := marker(t, nres.stdout); env["SEXTILE_CHARTS_READONLY"] != "1" {
		t.Fatalf("native fail closed: %v", env)
	}

	// A word given on the command line replaces a faulty line, for Sextile to
	// write; a file that is not a file is never written, whatever was asked.
	r := newRig(t)
	charts := r.mkdir("c")
	r.writeOriginals(charts + "=ask\n" + charts + "=allow\n")
	res := r.run("--originals", "add", "--charts-dir", charts)
	r.mustSucceed(res)
	run := r.find("run")
	if !hasEnv(run, "SEXTILE_ORIGINALS_SET=add") || readonlyMount(t, run) {
		t.Fatalf("the word outranks a faulty line: %q", run)
	}
	if strings.Contains(res.stderr, "launcher-originals.conf") {
		t.Fatalf("no fault is named when the word replaces the line: %s", res.stderr)
	}
	r2 := newRig(t)
	charts = r2.mkdir("c")
	mustMkdir(t, r2.originalsPath())
	res = r2.run("--originals", "add", "--charts-dir", charts)
	r2.mustSucceed(res)
	run = r2.find("run")
	if hasEnvName(run, "SEXTILE_ORIGINALS_SET") || !readonlyMount(t, run) {
		t.Fatalf("a file that is not a file stays protected: %q", run)
	}
	contains(t, res.stderr, notice("is not a file"))
}

func TestLauncherNeverWritesThePositionFile(t *testing.T) {
	// No file: none appears, with a word, with a made folder, in doctor.
	r := newRig(t)
	charts := r.mkdir("c")
	r.mustSucceed(r.run("--originals", "allow", "--charts-dir", charts))
	r.noPositionFile()
	r.mustSucceed(r.run("--originals", "protected", "--charts-dir", charts))
	r.noPositionFile()
	r.mustSucceed(r.run("doctor", "--charts-dir", charts))
	r.noPositionFile()
	if out, code := r.startsFresh(filepath.Join(r.root, "made")); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	r.noPositionFile()

	// A file: its bytes, mode and modification time stay.
	r2 := newRig(t)
	charts = r2.mkdir("c")
	r2.writeOriginals("# mine\n" + charts + "=ask\nzz=allow\n")
	before := snapshot(t, r2.xdg)
	r2.mustSucceed(r2.run("--originals", "allow", "--charts-dir", charts))
	r2.mustSucceed(r2.run("--charts-dir", charts))
	r2.mustSucceed(r2.run("doctor", "--charts-dir", charts))
	if d := diffSnapshots(before, snapshot(t, r2.xdg)); len(d) != 0 {
		t.Fatalf("the file or its folder changed: %v", d)
	}
}

func TestLauncherAndAppAgreeOnTheFolder(t *testing.T) {
	r := newRig(t)
	real := r.mkdir("real")
	link := filepath.Join(r.root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(link)
	if err != nil {
		t.Fatal(err)
	}
	phys, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatal(err)
	}
	id := filepath.Clean(phys)
	if id != real {
		t.Fatalf("the app's identity of the link is %q, want %q", id, real)
	}
	r.writeOriginals(id + "=protected\n")
	r.writeConf("charts-dir=" + link + "\n")
	r.mustSucceed(r.run())
	run := r.find("run")
	if !readonlyMount(t, run) {
		t.Fatalf("the launcher did not find the app's line for a symlinked folder: %q", run)
	}
	if !hasEnv(run, "SEXTILE_CHARTS_HOST_PATH="+id) {
		t.Fatalf("the host path is the app's identity: %q", envs(run))
	}
	if got := chartsMount(t, run); got != "type=bind,source="+id+",target=/charts,readonly" {
		t.Fatalf("the mount is %q, want the physical path %q as its source", got, id)
	}
	r.mustSucceed(r.run("doctor"))

	t.Run("a leading // is one /", func(t *testing.T) {
		r := newRig(t)
		real := r.mkdir("real")
		r.writeOriginals(real + "=protected\n")
		r.writeConf("charts-dir=/" + real + "\n")
		r.mustSucceed(r.run())
		run := r.find("run")
		if !readonlyMount(t, run) || !hasEnv(run, "SEXTILE_CHARTS_HOST_PATH="+real) {
			t.Fatalf("a //path missed the app's line: %q", run)
		}
	})
	t.Run("native runs the app on the physical folder", func(t *testing.T) {
		r := newRig(t)
		r.withGo()
		r.withAstrolog(false)
		real := r.mkdir("real")
		link := filepath.Join(r.root, "link")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		r.writeConf("charts-dir=" + link + "\n")
		res := r.run("--no-docker", "--no-open")
		r.mustSucceed(res)
		env, _ := marker(t, res.stdout)
		if env["SEXTILE_CHARTS_DIR"] != real || env["SEXTILE_CHARTS_HOST_PATH"] != real {
			t.Fatalf("native app folder %q (host path %q), want the pinned physical %q", env["SEXTILE_CHARTS_DIR"], env["SEXTILE_CHARTS_HOST_PATH"], real)
		}
	})
}

func TestLauncherRefusesANewlineInThePath(t *testing.T) {
	notice := "charts folder protected: its path holds a line break · this run writes nothing in it"
	for _, args := range [][]string{nil, {"--originals", "allow"}} {
		r := newRig(t)
		charts := mustMkdir(t, filepath.Join(r.root, "a\rb"))
		r.writeOriginals(charts + "=allow\n")
		res := r.run(append(args, "--charts-dir", charts)...)
		r.mustSucceed(res)
		contains(t, res.stderr, notice)
		run := r.find("run")
		if !readonlyMount(t, run) || !hasEnv(run, "SEXTILE_CHARTS_READONLY=1") {
			t.Fatalf("%q: protected means a read-only mount: %q", args, run)
		}
		if hasEnvName(run, "SEXTILE_ORIGINALS_SET") || hasEnvName(run, "SEXTILE_ORIGINALS_CREATED") {
			t.Fatalf("%q: neither variable: %q", args, run)
		}
	}

	// A physical path that ends in a line feed, reached through the link the
	// config remembers, is not another folder's identity.
	lf := newRig(t)
	end := mustMkdir(t, filepath.Join(lf.root, "anna\n"))
	link := filepath.Join(lf.root, "link")
	if err := os.Symlink(end, link); err != nil {
		t.Fatal(err)
	}
	lf.writeOriginals(strings.TrimSuffix(end, "\n") + "=allow\n")
	lf.writeConf("charts-dir=" + link + "\n")
	res := lf.run()
	lf.mustSucceed(res)
	contains(t, res.stderr, notice)
	run := lf.find("run")
	if !readonlyMount(t, run) || !hasEnv(run, "SEXTILE_CHARTS_READONLY=1") {
		t.Fatalf("a trailing line feed: protected means a read-only mount: %q", run)
	}
	if hasEnvName(run, "SEXTILE_ORIGINALS_SET") || hasEnvName(run, "SEXTILE_ORIGINALS_CREATED") {
		t.Fatalf("a trailing line feed: neither variable: %q", run)
	}
}

func TestDoctorNamesThePosition(t *testing.T) {
	cases := []struct {
		name  string
		setup func(r *rig, c string)
		want  string
	}{
		{"no line", func(r *rig, c string) {}, "ok    charts folder position: ask (not set)"},
		{"a line", func(r *rig, c string) { r.writeOriginals("x=ask\n" + c + "=add\n") }, "ok    charts folder position: add"},
		{"two lines", func(r *rig, c string) { r.writeOriginals(c + "=ask\n" + c + "=add\n") },
			"note  charts folder position: protected (launcher-originals.conf has 2 lines for it)"},
		{"a bad word", func(r *rig, c string) { r.writeOriginals(c + "=wide\n") },
			`note  charts folder position: protected (launcher-originals.conf says "wide" for it)`},
		{"not a file", func(r *rig, c string) { mustMkdir(r.t, r.originalsPath()) },
			"note  charts folder position: protected (launcher-originals.conf is not a file)"},
	}
	for _, c := range cases {
		r := newRig(t)
		charts := r.mkdir("c")
		c.setup(r, charts)
		before := snapshot(t, r.xdg)
		res := r.run("doctor", "--charts-dir", charts)
		contains(t, res.stdout, c.want+"\n")
		lines := strings.Split(res.stdout, "\n")
		for i, l := range lines {
			if strings.Contains(l, " charts folder: ") && !strings.Contains(lines[i+1], " charts folder position: ") {
				t.Fatalf("%s: the position follows the charts folder line:\n%s", c.name, res.stdout)
			}
		}
		if d := diffSnapshots(before, snapshot(t, r.xdg)); len(d) != 0 {
			t.Fatalf("%s: doctor writes nothing: %v", c.name, d)
		}
	}
	r := newRig(t)
	res := r.run("doctor", "--originals", "ask")
	r.mustExit2(res)
}

// The three lines a person reads when something is protected for them.
func TestOriginalsNoticeLinesAreFrozen(t *testing.T) {
	var got []string

	r := newRig(t)
	charts := r.mkdir("c")
	r.writeOriginals(charts + "=ask\n" + charts + "=add\n")
	res := r.run("doctor", "--charts-dir", charts)
	for _, l := range strings.Split(res.stdout, "\n") {
		if strings.Contains(l, "charts folder position: ") {
			got = append(got, r.norm(l))
		}
	}
	res = r.run("--charts-dir", charts)
	r.mustSucceed(res)
	for _, l := range strings.Split(res.stderr, "\n") {
		if strings.HasPrefix(l, "charts folder protected: ") {
			got = append(got, r.norm(l))
		}
	}

	r2 := newRig(t)
	res = r2.run("--charts-dir", mustMkdir(t, filepath.Join(r2.root, "a\rb")))
	r2.mustSucceed(res)
	for _, l := range strings.Split(res.stderr, "\n") {
		if strings.HasPrefix(l, "charts folder protected: ") {
			got = append(got, r2.norm(l))
		}
	}
	if len(got) != 3 {
		t.Fatalf("want the doctor line and two notices, got %q", got)
	}
	golden(t, "originals_notice_lines", strings.Join(got, "\n")+"\n")
}

func TestUsageListsOriginals(t *testing.T) {
	r := newRig(t)
	res := r.run("--help")
	contains(t, res.stdout, "[--originals WORD]")
	contains(t, res.stdout, "--originals WORD  what Sextile may do in this charts folder, from now on. WORD is one of")
	for _, w := range []string{"protected  Sextile writes nothing in it", "ask        Sextile asks before each write", "add        new charts are saved freely", "allow      Sextile writes without asking"} {
		contains(t, res.stdout, w)
	}
}

func TestFailedOriginalsWordStopsTheStart(t *testing.T) {
	r := newRig(t)
	r.setenv("FAKE_DOCKER_RUN_EXIT", "7")
	res := r.run("--originals", "allow", "--charts-dir", r.mkdir("c"))
	if res.code != 7 {
		t.Fatalf("the launcher passes Sextile's exit on: %d", res.code)
	}
	var extra []string
	for _, l := range strings.Split(strings.TrimSpace(res.stderr), "\n") {
		if l != "" && !strings.HasPrefix(l, "charts folder: ") {
			extra = append(extra, l)
		}
	}
	if len(extra) != 0 {
		t.Fatalf("the launcher prints nothing over Sextile's own reason: %q", extra)
	}
}
