package astrolog

import (
	"bytes"
	"errors"
	"io"
	"os"
)

// FileKind is what a file's first bytes say it is. Sextile knows an Astrolog
// chart by those bytes and never by its name: a charts folder can hold `a.as`,
// `b.as` and `c.txt`, and all three are charts when they open with
// Astrolog's chart header.
type FileKind uint8

const (
	// FileOther opens with no Astrolog header. It is not ours, and nothing
	// about it is reported.
	FileOther FileKind = iota
	// FileChart opens with `@AI`, Astrolog's chart-info header.
	FileChart
	// FileSettings opens with one of Astrolog's other three headers (`@AD`
	// defaults, `@AC` colours, `@AZ` zones). They are Astrolog's, they are not
	// charts, and `astrolog -id` skips them just the same.
	FileSettings
)

// sniffLen is how many bytes of a file are ever read to decide its kind. The
// header is three; the rest is slack for the day Astrolog grows one.
const sniffLen = 64

// KindOfPrefix is the kind a file's leading bytes name. It reads the START of
// the file, not a substring: a note that merely mentions `@AI` is not a chart.
func KindOfPrefix(b []byte) FileKind {
	switch {
	case bytes.HasPrefix(b, []byte("@AI")):
		return FileChart
	case bytes.HasPrefix(b, []byte("@AD")),
		bytes.HasPrefix(b, []byte("@AC")),
		bytes.HasPrefix(b, []byte("@AZ")):
		return FileSettings
	}
	return FileOther
}

// SniffFile reads at most the first sniffLen bytes of the file at path and
// returns its kind. A file shorter than the header is FileOther; an open or
// read failure is returned as the error, with FileOther.
func SniffFile(path string) (FileKind, error) {
	f, err := os.Open(path)
	if err != nil {
		return FileOther, err
	}
	defer f.Close()
	buf := make([]byte, sniffLen)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return FileOther, err
	}
	return KindOfPrefix(buf[:n]), nil
}
