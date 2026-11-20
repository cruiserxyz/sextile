package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWindowTitle(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		want       WindowTitleConfig
	}{
		{"absent key", "version: 1\n", WindowTitleConfig{}},
		{"session", "window_title: session\n", WindowTitleConfig{Value: "session", Present: true}},
		{"quoted with a comment", "window_title: \"session\"  # name\n", WindowTitleConfig{Value: "session", Present: true}},
		{"an unknown word is reported as written", "window_title: chart\n", WindowTitleConfig{Value: "chart", Present: true}},
		{"empty is absent", "window_title:\n", WindowTitleConfig{}},
		{"an indented key belongs to a block", "display:\n  window_title: session\n", WindowTitleConfig{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "preferences.yaml")
			if err := os.WriteFile(path, []byte(tc.file), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := readWindowTitle(path); got != tc.want {
				t.Errorf("readWindowTitle = %+v, want %+v", got, tc.want)
			}
		})
	}
}
