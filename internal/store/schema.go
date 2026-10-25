package store

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// schema.go owns one fact about every file Sextile keeps: which generation of
// the on-disk formats wrote it.
//
// A version line costs nothing to write today and cannot be added retroactively
// once real client libraries exist, because the files already out there would
// have no way to say what they are. Its purpose is not migration by itself —
// there is only one generation so far — but the thing migration needs first: a
// reader that can tell an old file from a new one, and refuse a file it does
// not understand instead of half-reading it.
//
// The convention every Sextile file follows: a TOP-LEVEL `version:` key, beside
// the blocks rather than inside one, so a reader can learn the generation
// without understanding the rest. Any new format — the session file an earlier change will
// write under SessionDir() among them — declares it the same way and gates on
// readableSchema before parsing a byte.

// SchemaVersion is the generation this build writes and is the newest it can
// read. Bump it when a format changes in a way an older Sextile would
// misinterpret — never for an addition an old reader can safely ignore.
const SchemaVersion = 1

// unversionedSchema is the generation of a file carrying no `version:` key at
// all: everything written before versioning existed. It is deliberately its own
// constant rather than SchemaVersion. When SchemaVersion moves on, an absent
// key must STILL mean generation 1 — reading it as "current" would mistake
// every old file on disk for a new one, which is the exact failure this whole
// mechanism exists to prevent.
const unversionedSchema = 1

// schemaVersionOf reports the generation a file's text declares. An absent
// `version:` key is unversionedSchema; a present but unreadable one is an
// error, because a file that tried to say what it is and failed must not be
// guessed at.
func schemaVersionOf(text string) (int, error) {
	for _, line := range strings.Split(text, "\n") {
		key, ok := topLevelKey(line)
		if !ok || key != "version" {
			continue
		}
		_, val, _ := splitKV(strings.TrimSpace(line))
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return 0, fmt.Errorf("unreadable version %q", val)
		}
		return n, nil
	}
	return unversionedSchema, nil
}

// readableSchema reports whether this build can read the file at path, and at
// which generation. A missing file is not a schema problem and is reported as
// such by the os error, so callers keep their own absent-file handling.
func readableSchema(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	v, err := schemaVersionOf(string(b))
	if err != nil {
		return 0, err
	}
	return v, checkSchema(v)
}

// checkSchema is the whole compatibility rule, in one place. A file from the
// FUTURE is refused: this build cannot know what a later generation changed,
// and half-reading a client's birth record is worse than declining to read it.
// A generation below the unversioned floor is malformed, not old.
func checkSchema(v int) error {
	switch {
	case v > SchemaVersion:
		return fmt.Errorf("written by a newer Sextile (generation %d; this build reads %d)", v, SchemaVersion)
	case v < unversionedSchema:
		return fmt.Errorf("unreadable generation %d", v)
	default:
		return nil
	}
}

// declaresVersion reports whether text already carries a top-level `version:`
// key, which decides whether a write rewrites that line in place or prepends
// one.
func declaresVersion(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if key, ok := topLevelKey(line); ok && key == "version" {
			return true
		}
	}
	return false
}

// schemaBlock is the version line as a whole top-level block, for mergeBlock.
func schemaBlock() string {
	return fmt.Sprintf("version: %d\n", SchemaVersion)
}
