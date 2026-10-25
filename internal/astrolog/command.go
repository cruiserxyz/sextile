package astrolog

import (
	"strings"
	"unicode"
)

// shellUnsafe lists the bytes that make a command element unsafe to paste
// bare into a POSIX shell, alongside whitespace and the empty string. The
// rule is deliberately conservative: an element that merely *looks* like it
// might be interpreted is quoted, so a recorded command pastes back as one
// command with the same argv it ran with.
const shellUnsafe = "|&;<>()$`\\*?[]{}!#~='\""

// shellQuote returns s ready to paste as one shell word: bare when every
// byte is safe, otherwise wrapped in single quotes with each embedded single
// quote closed, backslash-escaped, and reopened (the POSIX idiom).
func shellQuote(s string) string {
	if s == "" || strings.ContainsAny(s, shellUnsafe) || strings.IndexFunc(s, unicode.IsSpace) >= 0 {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}

// formatCommand renders one executed spawn as the command line that would
// re-run it: the resolved binary path first, then every argv element, each
// quoted by shellQuote. It is the only place in the binary a command string
// is composed (invariants: TestCommandsAreCarriedNotComposed); every other
// package carries the result unchanged.
func formatCommand(path string, argv []string) string {
	parts := make([]string, 0, len(argv)+1)
	parts = append(parts, shellQuote(path))
	for _, a := range argv {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// recordCommand appends the command of a spawn that actually started. invoke
// returns an empty command when the process never started (binary missing,
// context already done), and a spawn that never happened is not recorded.
func recordCommand(commands []string, command string) []string {
	if command == "" {
		return commands
	}
	return append(commands, command)
}
