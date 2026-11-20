package store

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// ExperimentalConfig is the `experimental:` block of preferences.yaml: the
// switches the operator turned on in the Experiments popup, remembered across
// restarts (the control repository's specs/experiments-block.md).
//
// Like KeysConfig it holds no judgement: every key of the block is reported,
// and which ids are switches at all is internal/ui's table to decide.
type ExperimentalConfig struct {
	// On maps each direct child's id to whether its value reads on. Never nil.
	On map[string]bool
}

// Experimental reads the `experimental:` block from preferences.yaml:
//
//	experimental:
//	  enter-colour: on
//
// A value is on when it is `on`, `true` or `yes` in any case, quoted or not;
// any other value is off. An absent, unreadable or later-generation file, a
// block that holds a value instead of children and a malformed block read as
// nothing on.
func Experimental() ExperimentalConfig {
	return readExperimental(PreferencesPath())
}

// readExperimental reads only the FIRST `experimental:` block, and in it only
// the first occurrence of each direct child; later blocks, deeper lines and
// repeated keys are ignored, as readFrame does.
func readExperimental(path string) ExperimentalConfig {
	cfg := ExperimentalConfig{On: map[string]bool{}}
	if _, err := readableSchema(path); err != nil {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	lines := splitAfterNewlines(string(data))
	b, ok := findBlock(lines, experimentalBlock)
	if !ok || b.scalar || b.stray >= 0 {
		return cfg
	}
	for key, i := range b.at {
		_, val, _ := splitKV(stripInlineComment(strings.TrimSpace(lines[i])))
		cfg.On[key] = switchOn(val)
	}
	return cfg
}

// experimentalBlock is the block's top-level key.
const experimentalBlock = "experimental"

// switchOn is the reader's one judgement of a value, after splitKV has
// unquoted it.
func switchOn(val string) bool {
	for _, w := range []string{"on", "true", "yes"} {
		if strings.EqualFold(val, w) {
			return true
		}
	}
	return false
}

// experimentID is the shape of an id the writer will put in the file: a bare
// lowercase word with dashes, which YAML reads back as itself.
var experimentID = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// SetExperimental writes one switch to preferences.yaml, atomically, through
// the door every preferences write takes.
//
// The write owns ONE value token of ONE line: the first direct child `<id>:` of
// the first `experimental:` block. Every other byte of that line (its indent,
// the key's own spelling up to the value, the whitespace and comment after
// it, its line ending) and of the file is carried through as written; a line
// is never deleted, so switching off leaves `<id>: off`. A missing key is
// added when turning on and not at all when turning off, and a change that
// leaves the text as it was writes nothing. An id outside experimentID, a
// block that holds a value, and a block YAML would not read as one are
// refused, and nothing is written.
func SetExperimental(id string, on bool) error {
	if !experimentID.MatchString(id) {
		return fmt.Errorf("store: experimental: %q is not a switch id", id)
	}
	return writePreferencesChecked(PreferencesPath(), func(existing string) (string, error) {
		return mergeExperimental(existing, id, on)
	})
}

// mergeExperimental is SetExperimental's text edit.
func mergeExperimental(existing, id string, on bool) (string, error) {
	word := "off"
	if on {
		word = "on"
	}
	lines := splitAfterNewlines(existing)
	b, found := findBlock(lines, experimentalBlock)
	if !found {
		if !on {
			return existing, nil
		}
		if existing != "" && !strings.HasSuffix(existing, "\n") {
			existing += "\n"
		}
		return existing + experimentalBlock + ":\n  " + id + ": on\n", nil
	}
	if b.scalar {
		return "", fmt.Errorf("store: The settings file has a line '%s' where Sextile expects a list. Nothing was saved",
			strings.TrimSpace(lines[b.start]))
	}
	if b.stray >= 0 {
		return "", fmt.Errorf("store: preferences.yaml's experimental: block holds '%s', which YAML does not read as part of it. The switch was not saved",
			strings.TrimSpace(lines[b.stray]))
	}
	if i, ok := b.at[id]; ok {
		lines[i] = setValueToken(lines[i], word)
		return strings.Join(lines, ""), nil
	}
	if !on {
		return existing, nil
	}
	if !strings.HasSuffix(lines[b.last], "\n") {
		lines[b.last] += "\n"
	}
	added := b.childIndent + id + ": on\n"
	lines = append(lines[:b.last+1], append([]string{added}, lines[b.last+1:]...)...)
	return strings.Join(lines, ""), nil
}

// setValueToken replaces the value of a `key: value` line with word and keeps
// every other byte: what stands before the value (indent, key, colon and the
// whitespace after it) and what follows it (whitespace, an inline comment, the
// line ending). An empty value gets word after one space, right after the
// colon, and so does a value written with no space after the colon, so that
// the line reads back as `key: word`.
func setValueToken(line, word string) string {
	body := strings.TrimRight(line, "\r\n")
	ending := line[len(body):]
	colon := strings.Index(body, ":")
	after := body[colon+1:]
	value := strings.TrimLeft(after, " \t")
	gap := after[:len(after)-len(value)]
	n := valueTokenLen(value, gap != "")
	if n == 0 {
		return body[:colon+1] + " " + word + after + ending
	}
	if gap == "" {
		gap = " "
	}
	return body[:colon+1] + gap + word + value[n:] + ending
}

// valueTokenLen is the length of the scalar value at the start of value (what
// follows the colon and its whitespace), so that whatever comes after it, an
// inline comment above all, is kept as written. A quoted scalar ends at its
// closing quote: a backslash escapes in double quotes, a doubled quote in
// single quotes, as YAML has it. One that never closes is read as a plain
// scalar. A plain scalar ends where a `#` follows whitespace, its trailing
// whitespace not counted. A `#` right at the start is a comment only when
// whitespace separates it from the colon (spaced).
func valueTokenLen(value string, spaced bool) int {
	if value == "" || (value[0] == '#' && spaced) {
		return 0
	}
	switch value[0] {
	case '"':
		for i := 1; i < len(value); i++ {
			switch value[i] {
			case '\\':
				i++
			case '"':
				return i + 1
			}
		}
	case '\'':
		for i := 1; i < len(value); i++ {
			if value[i] == '\'' {
				if i+1 < len(value) && value[i+1] == '\'' {
					i++
					continue
				}
				return i + 1
			}
		}
	}
	end := len(value)
	for i := 1; i < len(value); i++ {
		if value[i] == '#' && (value[i-1] == ' ' || value[i-1] == '\t') {
			end = i
			break
		}
	}
	return len(strings.TrimRight(value[:end], " \t"))
}
