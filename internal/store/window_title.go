package store

import (
	"os"
	"strings"
)

// WindowTitleConfig is the top-level `window_title:` key of preferences.yaml
// exactly as it was read: what the terminal window's title says.
//
// Like WheelFormatConfig it holds no judgement. Whether the value is one the
// program knows is internal/ui's to decide.
type WindowTitleConfig struct {
	// Value is the first `window_title:` entry, unquoted and without a trailing
	// comment. Present is false when there is none, or when it is empty.
	Value   string
	Present bool
}

// WindowTitle reads `window_title:` from preferences.yaml:
//
//	window_title: session
//
// An absent or unreadable file reports it absent, the ordinary case.
func WindowTitle() WindowTitleConfig {
	return readWindowTitle(PreferencesPath())
}

// readWindowTitle reads only the first top-level `window_title:` line, as
// readWheelFormat reads its key.
func readWindowTitle(path string) WindowTitleConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return WindowTitleConfig{}
	}
	for _, raw := range splitAfterNewlines(string(data)) {
		line := strings.TrimRight(raw, "\r\n")
		if key, ok := topLevelKey(line); !ok || key != "window_title" {
			continue
		}
		_, val, _ := splitKV(stripInlineComment(strings.TrimSpace(line)))
		return WindowTitleConfig{Value: val, Present: val != ""}
	}
	return WindowTitleConfig{}
}
