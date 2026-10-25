package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWheelFormat(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		want       WheelFormatConfig
	}{
		{"absent key", "version: 1\n", WheelFormatConfig{}},
		{"png", "wheel_format: png\n", WheelFormatConfig{Value: "png", Present: true}},
		{"svg among other blocks", "version: 1\nlocation:\n  name: Berlin\nwheel_format: svg\nkeys:\n  leader: \",\"\n", WheelFormatConfig{Value: "svg", Present: true}},
		{"quoted with a comment", "wheel_format: \"png\"  # for the desktop\n", WheelFormatConfig{Value: "png", Present: true}},
		{"the first one wins", "wheel_format: png\nwheel_format: svg\n", WheelFormatConfig{Value: "png", Present: true}},
		{"an unknown word is reported as written", "wheel_format: gif\n", WheelFormatConfig{Value: "gif", Present: true}},
		{"empty is absent", "wheel_format:\n", WheelFormatConfig{}},
		{"an indented key belongs to a block", "display:\n  wheel_format: png\n", WheelFormatConfig{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "preferences.yaml")
			if err := os.WriteFile(path, []byte(tc.file), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := readWheelFormat(path); got != tc.want {
				t.Errorf("readWheelFormat = %+v, want %+v", got, tc.want)
			}
		})
	}
	if got := readWheelFormat(filepath.Join(t.TempDir(), "missing.yaml")); got != (WheelFormatConfig{}) {
		t.Errorf("a missing file reported %+v", got)
	}
}
