package lookup

import (
	"bytes"
	"errors"
	"fmt"
)

// A chart rewrite goes through Astrolog's =o writer, which keeps only its own
// -qb and -zi lines. Reapply puts the rest back: the result is the source
// file, line for line and byte for byte, with only the line of the switch the
// action owns taken from Astrolog's bytes. It reads and writes no file.

// Owned switches: the line an action rewrites.
const (
	OwnsName   = "-zi" // a rename
	OwnsMoment = "-qb" // a moment rewrite (variants)
)

// ErrReapply marks a rewrite that Reapply refused: nothing was written.
var ErrReapply = errors.New("lookup: the rewrite was refused")

// reapplyRefusal is a refusal whose text is the sentence the user reads.
type reapplyRefusal string

func (r reapplyRefusal) Error() string        { return string(r) }
func (r reapplyRefusal) Is(target error) bool { return target == ErrReapply }

func refuse(format string, a ...any) error { return reapplyRefusal(fmt.Sprintf(format, a...)) }

// Reapply returns source with its owned line replaced by the one owned line
// in written. file names the chart in a refusal. When owned is -qb and
// placeChanged is false, Astrolog's date and time fields are kept and the
// daylight, zone, longitude and latitude fields come from the source as
// written. A source with no owned line gets Astrolog's line directly after the
// last line that sets its moment (-qb, -qa, -qc or -qj).
func Reapply(file string, source, written []byte, owned string, placeChanged bool) ([]byte, error) {
	src := splitLines(source)
	mine := linesOf(src, owned)
	if len(mine) > 1 {
		return nil, refuse("%s has more than one %s line. Nothing was written.", file, owned)
	}
	// A rename that replaces its -zi line needs no moment line to place it.
	// A moment rewrite needs the file's one -qb line; an inserted line goes
	// after the last line that sets the moment.
	qb := linesOf(src, OwnsMoment)
	if owned == OwnsMoment && len(qb) != 1 {
		if len(qb) == 0 {
			return nil, refuse("%s has no -qb line. Nothing was written.", file)
		}
		return nil, refuse("%s has more than one -qb line. Nothing was written.", file)
	}
	after := -1
	for _, sw := range []string{"-qb", "-qa", "-qc", "-qj"} {
		for _, l := range linesOf(src, sw) {
			after = max(after, l.index)
		}
	}
	if len(mine) == 0 && after < 0 {
		return nil, refuse("%s sets no moment. Nothing was written.", file)
	}
	theirs := linesOf(splitLines(written), owned)
	if len(theirs) != 1 {
		return nil, refuse("Astrolog did not write the %s line as expected. Nothing was written.", owned)
	}
	line := bytes.TrimRight(theirs[0].text, "\r\n")

	if owned == OwnsMoment && !placeChanged {
		var err error
		if line, err = keepPlaceFields(file, line, bytes.TrimRight(src[qb[0].index].text, "\r\n")); err != nil {
			return nil, err
		}
	}

	var out bytes.Buffer
	for i, l := range src {
		switch {
		case len(mine) == 1 && i == mine[0].index:
			out.Write(line)
			out.Write(l.text[len(bytes.TrimRight(l.text, "\r\n")):])
		case len(mine) == 0 && i == after:
			// The inserted line follows the moment. A line that ends the file
			// without a line end gets one, and the inserted line ends the
			// file without one in its place.
			end := terminator(l.text, source)
			out.Write(l.text)
			if bytes.HasSuffix(l.text, end) {
				out.Write(line)
				out.Write(end)
			} else {
				out.Write(end)
				out.Write(line)
			}
		default:
			out.Write(l.text)
		}
	}
	return out.Bytes(), nil
}

type line struct {
	index int
	text  []byte // the line with its own terminator, if it has one
}

// splitLines cuts b after every LF, so each line keeps its CRLF or LF and a
// last line without one stays without one.
func splitLines(b []byte) []line {
	var out []line
	for len(b) > 0 {
		n := bytes.IndexByte(b, '\n') + 1
		if n == 0 {
			n = len(b)
		}
		out = append(out, line{index: len(out), text: b[:n]})
		b = b[n:]
	}
	return out
}

// linesOf returns the lines whose first token, after leading spaces and
// tabs, is exactly sw. Only a space, a tab or the line end ends the token.
func linesOf(ls []line, sw string) []line {
	var out []line
	for _, l := range ls {
		t := bytes.TrimLeft(l.text, " \t")
		if bytes.HasPrefix(t, []byte(sw)) {
			rest := t[len(sw):]
			if len(rest) == 0 || bytes.IndexByte([]byte(" \t\r\n"), rest[0]) >= 0 {
				out = append(out, l)
			}
		}
	}
	return out
}

// terminator is the line end to give a line inserted after l: l's own, or
// else the file's first one, or else LF.
func terminator(l, file []byte) []byte {
	switch {
	case bytes.HasSuffix(l, []byte("\r\n")):
		return []byte("\r\n")
	case bytes.HasSuffix(l, []byte("\n")):
		return []byte("\n")
	}
	if i := bytes.IndexByte(file, '\n'); i > 0 && file[i-1] == '\r' {
		return []byte("\r\n")
	}
	return []byte("\n")
}

// keepPlaceFields is theirs with its last four fields (daylight, zone,
// longitude, latitude) replaced by the source's, as the source wrote them.
// A -qb line is the switch and eight fields.
func keepPlaceFields(file string, theirs, source []byte) ([]byte, error) {
	t, s := fieldSpans(theirs), fieldSpans(source)
	if len(s) != 9 {
		return nil, refuse("%s has a -qb line that is not eight fields. Nothing was written.", file)
	}
	if len(t) != 9 {
		return nil, refuse("Astrolog did not write the -qb line as expected. Nothing was written.")
	}
	var out []byte
	out = append(out, theirs[:t[5][0]]...)
	out = append(out, source[s[5][0]:s[8][1]]...)
	out = append(out, theirs[t[8][1]:]...)
	return out, nil
}

// fieldSpans is the [start, end) of each blank-separated field of b.
func fieldSpans(b []byte) [][2]int {
	var out [][2]int
	start := -1
	for i, c := range b {
		blank := c == ' ' || c == '\t'
		switch {
		case !blank && start < 0:
			start = i
		case blank && start >= 0:
			out = append(out, [2]int{start, i})
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, [2]int{start, len(b)})
	}
	return out
}
