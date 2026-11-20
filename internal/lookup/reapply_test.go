// All chart values reachable from this file are constructed synthetic test inputs.
package lookup

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// Verifiers for the re-apply step of transfer specs/sextile-release/variants/
// chart-rewrite-reapplies-lines.md (K7 to K10). None needs Astrolog.

const reapplySource = "@AI780  ; Astrolog chart info.\n" +
	"; a note\n" +
	"-qb Jan 3 1890 14:00 ST -0:19:32 4:53:28E 52:22N\n" +
	"-zi \"Old\" \"Amsterdam\"\n" +
	"-c 1\n"

const lmtLine = "-qb Jan 3 1890 14:00 ST -0:19:32 4:53:28E 52:22N\n"

const reapplyWritten = "@AI800  ; Astrolog chart info.\n" +
	"-qb Jan 3 1890  2:00pm ST 0:19W   4:53E 52:22N\n" +
	"-zi \"New\" \"Amsterdam, NL\"\n"

// K7: a -qb rewrite with the place unchanged keeps Astrolog's date and time
// and the source's daylight, zone, longitude and latitude as written.
func TestReapplyKeepsSourcePlaceFields(t *testing.T) {
	written := "-qb Feb 4 1890  3:30pm DT 0:19W   4:53E 52:22N\n-zi \"Old\" \"Amsterdam\"\n"
	got, err := Reapply("Anna.as", []byte(reapplySource), []byte(written), OwnsMoment, false)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(reapplySource, "-qb Jan 3 1890 14:00 ST -0:19:32 4:53:28E 52:22N",
		"-qb Feb 4 1890  3:30pm ST -0:19:32 4:53:28E 52:22N", 1)
	if string(got) != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	t.Run("place changed", func(t *testing.T) {
		got, err := Reapply("Anna.as", []byte(reapplySource), []byte(written), OwnsMoment, true)
		if err != nil {
			t.Fatal(err)
		}
		want := strings.Replace(reapplySource, "-qb Jan 3 1890 14:00 ST -0:19:32 4:53:28E 52:22N",
			"-qb Feb 4 1890  3:30pm DT 0:19W   4:53E 52:22N", 1)
		if string(got) != want {
			t.Errorf("got\n%s\nwant\n%s", got, want)
		}
	})
}

// K8: an owned switch the source repeats is ambiguous. The last subtest runs
// a real rename and needs Astrolog.
func TestReapplyRefusesRepeatedSwitch(t *testing.T) {
	t.Run("through the rename", renameRefusesRepeatedNameLine)
	t.Run("a title goes after the last of repeated -qb lines", func(t *testing.T) {
		src := strings.Replace(reapplySource, "-zi \"Old\" \"Amsterdam\"\n", "", 1) + lmtLine
		got, err := Reapply("Anna.as", []byte(src), []byte(reapplyWritten), OwnsName, false)
		if want := src + "-zi \"New\" \"Amsterdam, NL\"\n"; err != nil || string(got) != want {
			t.Errorf("got %q (%v), want %q", got, err, want)
		}
	})
	for _, owned := range []string{OwnsName, OwnsMoment} {
		src := reapplySource + "  " + owned + " again\n"
		_, err := Reapply("Anna.as", []byte(src), []byte(reapplyWritten), owned, true)
		want := "Anna.as has more than one " + owned + " line. Nothing was written."
		if !errors.Is(err, ErrReapply) || err.Error() != want {
			t.Errorf("%s: err = %v, want %q", owned, err, want)
		}
	}
}

// K8a: a title is placed after the line that sets the moment, whichever
// switch it is, and a source that sets no moment is refused.
func TestReapplyPlacesATitleAfterTheMoment(t *testing.T) {
	src := "@AI800\n-qa Jun 1 1990 16:30 1E 4:53E 52:22N\n-c 1\n"
	got, err := Reapply("Anna.as", []byte(src), []byte(reapplyWritten), OwnsName, false)
	if want := "@AI800\n-qa Jun 1 1990 16:30 1E 4:53E 52:22N\n-zi \"New\" \"Amsterdam, NL\"\n-c 1\n"; err != nil || string(got) != want {
		t.Errorf("got %q (%v), want %q", got, err, want)
	}
	bare := strings.Replace(strings.Replace(reapplySource, lmtLine, "", 1), "-zi \"Old\" \"Amsterdam\"\n", "", 1)
	_, err = Reapply("Anna.as", []byte(bare), []byte(reapplyWritten), OwnsName, false)
	want := "Anna.as sets no moment. Nothing was written."
	if !errors.Is(err, ErrReapply) || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

// K10: Astrolog's bytes hold none, or more than one, of the owned line. The
// last subtest runs a real rename and needs Astrolog.
func TestReapplyRefusesUnclearAstrologOutput(t *testing.T) {
	t.Run("through the rename", renameRefusesUnclearAstrologOutput)
	for name, written := range map[string]string{
		"none": "@AI800  ; Astrolog chart info.\n-qb Jan 3 1890  2:00pm ST 0:19W   4:53E 52:22N\n",
		"two":  reapplyWritten + "-zi \"Other\" \"Elsewhere\"\n",
	} {
		_, err := Reapply("Anna.as", []byte(reapplySource), []byte(written), OwnsName, false)
		want := "Astrolog did not write the -zi line as expected. Nothing was written."
		if !errors.Is(err, ErrReapply) || err.Error() != want {
			t.Errorf("%s: err = %v, want %q", name, err, want)
		}
	}
}

// K9: Reapply is bytes in, bytes out, with no Astrolog on PATH; it keeps
// CRLF, a missing final newline, and leading blanks; and retitle calls it.
func TestReapplyIsPure(t *testing.T) {
	t.Setenv("PATH", "")
	for _, tc := range []struct{ name, src, want string }{
		{"replace", reapplySource,
			strings.Replace(reapplySource, "-zi \"Old\" \"Amsterdam\"", "-zi \"New\" \"Amsterdam, NL\"", 1)},
		{"crlf", strings.ReplaceAll(reapplySource, "\n", "\r\n"),
			strings.ReplaceAll(strings.Replace(reapplySource, "-zi \"Old\" \"Amsterdam\"", "-zi \"New\" \"Amsterdam, NL\"", 1), "\n", "\r\n")},
		{"no final newline", strings.TrimSuffix(reapplySource, "\n"),
			strings.TrimSuffix(strings.Replace(reapplySource, "-zi \"Old\" \"Amsterdam\"", "-zi \"New\" \"Amsterdam, NL\"", 1), "\n")},
		{"insert", strings.Replace(reapplySource, "-zi \"Old\" \"Amsterdam\"\n", "", 1),
			strings.Replace(reapplySource, "-zi \"Old\" \"Amsterdam\"", "-zi \"New\" \"Amsterdam, NL\"", 1)},
		{"insert after a last -qb without a newline", "@AI780\n-qb Jan 3 1890 14:00 ST -0:19:32 4:53:28E 52:22N",
			"@AI780\n-qb Jan 3 1890 14:00 ST -0:19:32 4:53:28E 52:22N\n-zi \"New\" \"Amsterdam, NL\""},
		{"leading blanks", strings.Replace(reapplySource, "-zi", " \t-zi", 1),
			strings.Replace(reapplySource, "-zi \"Old\" \"Amsterdam\"", "-zi \"New\" \"Amsterdam, NL\"", 1)},
		{"a last owned line without a newline", "@AI780\n-qb Jan 3 1890 14:00 ST -0:19:32 4:53:28E 52:22N\n-zi \"Old\" \"Amsterdam\"",
			"@AI780\n-qb Jan 3 1890 14:00 ST -0:19:32 4:53:28E 52:22N\n-zi \"New\" \"Amsterdam, NL\""},
		{"a last owned line with CRLF", "@AI780\n-qb Jan 3 1890 14:00 ST -0:19:32 4:53:28E 52:22N\n-zi \"Old\" \"Amsterdam\"\r\n",
			"@AI780\n-qb Jan 3 1890 14:00 ST -0:19:32 4:53:28E 52:22N\n-zi \"New\" \"Amsterdam, NL\"\r\n"},
		{"a vertical tab is not a blank", strings.Replace(reapplySource, "-zi", "\v-zi", 1),
			strings.Replace(strings.Replace(reapplySource, "-zi", "\v-zi", 1), lmtLine, lmtLine+"-zi \"New\" \"Amsterdam, NL\"\n", 1)},
		{"a repeated -qb does not stop a rename that has its -zi", reapplySource + lmtLine,
			strings.Replace(reapplySource, "-zi \"Old\" \"Amsterdam\"", "-zi \"New\" \"Amsterdam, NL\"", 1) + lmtLine},
		{"a longer switch is not the owned one", reapplySource + "-zix 1\n",
			strings.Replace(reapplySource, "-zi \"Old\" \"Amsterdam\"", "-zi \"New\" \"Amsterdam, NL\"", 1) + "-zix 1\n"},
	} {
		got, err := Reapply("Anna.as", []byte(tc.src), []byte(reapplyWritten), OwnsName, false)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("%s:\ngot  %q\nwant %q", tc.name, got, tc.want)
		}
	}
	if !callsIn(t, "retitle.go", "retitleChartFile", "Reapply") {
		t.Error("retitleChartFile does not call Reapply")
	}
}

// callsIn reports whether function fn in file calls callee. Comments and
// strings do not count.
func callsIn(t *testing.T, file, fn, callee string) bool {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != fn || fd.Body == nil {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if id, ok := c.Fun.(*ast.Ident); ok && id.Name == callee {
					found = true
				}
			}
			return true
		})
	}
	return found
}
