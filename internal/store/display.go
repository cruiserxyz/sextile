package store

import (
	"os"
	"strings"
)

// DisplayConfig is the `display:` block exactly as it was read.
//
// Like KeysConfig it holds no defaults and no judgement: it reports what the
// file said. Whether a value is usable, and what happens when it is not, is
// internal/ui's to decide.
type DisplayConfig struct {
	// FocusColour and CursorRowColour are the two colours the operator may
	// override (the specification C9): the frame that has the
	// keyboard, and the cursor row's background. Empty means the entry is
	// missing or empty, and the default stands.
	FocusColour     string
	CursorRowColour string

	// RetiredStripePresent reports a `row_stripe` entry, with or without a
	// value. The stripe is retired; ui says so once (C17).
	RetiredStripePresent bool
}

// Display reads the `display:` block from preferences.yaml:
//
//	display:
//	  focus_color: 10
//	  cursor_row_color: "#1f6f6f"
//
// An absent or unreadable file reports every value absent — the ordinary case.
func Display() DisplayConfig {
	return readDisplay(PreferencesPath())
}

// readDisplay follows readKeys exactly: block boundaries come from
// topLevelKey, only the FIRST block of the name is read, and an entry this
// version does not know is ignored rather than reported.
func readDisplay(path string) DisplayConfig {
	var cfg DisplayConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	inBlock := false
	for _, raw := range splitAfterNewlines(string(data)) {
		line := strings.TrimRight(raw, "\r\n")
		if key, ok := topLevelKey(line); ok {
			if inBlock {
				break
			}
			inBlock = key == "display"
			continue
		}
		if !inBlock {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, val, ok := splitKV(stripInlineComment(trimmed))
		if !ok {
			continue
		}
		switch key {
		case "focus_color":
			cfg.FocusColour = val
		case "cursor_row_color":
			cfg.CursorRowColour = val
		case "row_stripe":
			cfg.RetiredStripePresent = true
		}
	}
	return cfg
}

// stripInlineComment drops a YAML inline comment (` # ...`) from an entry, so
// `focus_color: 10  # green` reads as 10. A `#` inside a quoted value — the
// hex form, `"#302d1c"` — is the value, not a comment.
func stripInlineComment(line string) string {
	quote := rune(0)
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '#' && i > 0 && (line[i-1] == ' ' || line[i-1] == '\t'):
			return strings.TrimSpace(line[:i])
		}
	}
	return line
}
