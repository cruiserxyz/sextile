package location

import (
	"os"
	"strings"
	"time"
)

// HostIANA tries to identify the IANA timezone name of the host the
// shipped binary is running on. The result is consumed by Acquire to
// produce an atlas-modal preseed (R8/R10/R11): "Europe/Berlin" → "Berlin".
//
// Linux-only resolution chain (R10 narrows to current Linux):
//
//  1. time.Local.String() — works when Go's tz init resolved a name
//     (often does, but in stripped containers or odd Local setups
//     returns the literal "Local" / "" — review flagged this).
//  2. /etc/timezone — Debian/Ubuntu single-line IANA name file.
//  3. /etc/localtime symlink → "<zoneinfo-prefix>/<IANA>".
//
// Returns empty string when none of the above yields a usable IANA name.
// The empty result triggers the bare-modal hard default per R8 (no
// preseed; user types from scratch).
func HostIANA() string {
	if name := normalisedLocal(); name != "" {
		return name
	}
	if name := readEtcTimezone(); name != "" {
		return name
	}
	if name := readEtcLocaltime(); name != "" {
		return name
	}
	return ""
}

// normalisedLocal returns time.Local.String() when it looks like a real
// IANA name (contains '/'); else empty. "Local" and "UTC" don't qualify
// as preseed candidates because the IANA-last-segment heuristic needs a
// city.
func normalisedLocal() string {
	name := time.Local.String()
	if strings.Contains(name, "/") {
		return name
	}
	return ""
}

func readEtcTimezone() string {
	b, err := os.ReadFile("/etc/timezone")
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(b))
	if !strings.Contains(name, "/") {
		return ""
	}
	return name
}

// readEtcLocaltime resolves /etc/localtime when it's a symlink into the
// system zoneinfo tree. Strips a trailing "<prefix>/zoneinfo/" so the
// returned name is bare IANA (e.g. "Europe/Berlin" not
// "/usr/share/zoneinfo/Europe/Berlin").
func readEtcLocaltime() string {
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		return ""
	}
	const marker = "/zoneinfo/"
	idx := strings.LastIndex(target, marker)
	if idx < 0 {
		return ""
	}
	name := target[idx+len(marker):]
	if !strings.Contains(name, "/") {
		return ""
	}
	return name
}

// PreseedFromIANA extracts the atlas-modal preseed string from an IANA
// name: the last `/`-segment, with `_` → ` ` so "Los_Angeles" surfaces
// as "Los Angeles".
//
// Returns empty for inputs that do not name a real city:
//   - empty string
//   - no `/` (bare names like "UTC" or "Local")
//   - "Etc/*" (IANA's compatibility zone — Etc/UTC, Etc/GMT+5, ... — none
//     of these correspond to atlas cities; the smoke-tested container
//     container exposes Etc/UTC and would have offered "UTC" as preseed,
//     which is a non-match against the atlas)
//   - last segment is "UTC" or "GMT" (defensive: still no atlas city)
func PreseedFromIANA(iana string) string {
	if iana == "" || strings.HasPrefix(iana, "Etc/") {
		return ""
	}
	idx := strings.LastIndex(iana, "/")
	if idx < 0 {
		return ""
	}
	last := iana[idx+1:]
	switch last {
	case "UTC", "GMT", "":
		return ""
	}
	return strings.ReplaceAll(last, "_", " ")
}
