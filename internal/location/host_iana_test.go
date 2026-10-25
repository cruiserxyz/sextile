package location

import "testing"

func TestPreseedFromIANA(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Europe/Berlin", "Berlin"},
		{"America/Los_Angeles", "Los Angeles"},
		{"Asia/Ho_Chi_Minh", "Ho Chi Minh"},
		{"UTC", ""},
		{"", ""},
		{"Local", ""},
		{"Etc/UTC", ""},
		{"Etc/GMT+5", ""},
		{"Atlantic/UTC", ""}, // defensive: bare UTC suffix
	}
	for _, c := range cases {
		if got := PreseedFromIANA(c.in); got != c.want {
			t.Errorf("PreseedFromIANA(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalisedLocal_Smoke just confirms the function never panics; the
// returned value depends on the host environment and is not asserted.
func TestNormalisedLocal_Smoke(t *testing.T) {
	_ = normalisedLocal()
}

// TestHostIANA_Smoke confirms the layered chain runs end-to-end without
// panicking, regardless of which layer answers (or none — empty is valid).
func TestHostIANA_Smoke(t *testing.T) {
	_ = HostIANA()
}
