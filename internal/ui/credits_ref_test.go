package ui

import "testing"

func TestReleaseRefIsATagOnlyForAnExactRelease(t *testing.T) {
	for version, want := range map[string]string{
		"v0.1.4": "v0.1.4", "0.1.4": "v0.1.4",
		"v0.1.3-15-g1b28d0f": "main", "local": "main", "docker": "main", "devel": "main", "": "main", "v0.1.4-dirty": "main",
	} {
		ref, release := releaseRef(version)
		if ref != want || release != (want != "main") {
			t.Errorf("releaseRef(%q) = %q, %v, want %q", version, ref, release, want)
		}
	}
}
