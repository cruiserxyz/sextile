package store

import (
	"os"
	"strings"
)

// WheelFormatConfig is the top-level `wheel_format:` key of preferences.yaml
// exactly as it was read: which image the wheel server hosts.
//
// Like KeysConfig it holds no defaults and no judgement. Whether the value is
// one of the two formats, and what happens when it is not, is internal/ui's to
// decide.
type WheelFormatConfig struct {
	// Value is the first `wheel_format:` entry, unquoted and without a
	// trailing comment. Present is false when there is none, or when it is
	// empty.
	Value   string
	Present bool
}

// WheelFormat reads `wheel_format:` from preferences.yaml:
//
//	wheel_format: png
//
// An absent or unreadable file reports it absent — the ordinary case.
func WheelFormat() WheelFormatConfig {
	return readWheelFormat(PreferencesPath())
}

// readWheelFormat reads only the first top-level `wheel_format:` line, which is
// what makes a hand-edited file with a duplicate resolve to one answer. An
// indented one belongs to some block and is not this key.
func readWheelFormat(path string) WheelFormatConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return WheelFormatConfig{}
	}
	for _, raw := range splitAfterNewlines(string(data)) {
		line := strings.TrimRight(raw, "\r\n")
		if key, ok := topLevelKey(line); !ok || key != "wheel_format" {
			continue
		}
		_, val, _ := splitKV(stripInlineComment(strings.TrimSpace(line)))
		return WheelFormatConfig{Value: val, Present: val != ""}
	}
	return WheelFormatConfig{}
}
