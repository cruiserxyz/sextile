package location

// AcquireResult is the outcome of the cold-start location-acquisition
// cascade per contract R8/R10/R11. The ingest perimeter consults this
// once at startup to decide whether to fast-path past the modal or open
// it (with or without a preseed).
//
// Fields:
//   - HasLocation: true means a stored PlaceInfo was found and Place is
//     populated; the modal does NOT open.
//   - Place: the stored value (only meaningful when HasLocation).
//   - Preseed: the atlas-modal filter prefill (R11 — IANA last segment),
//     used only when HasLocation is false; empty means the modal opens
//     bare.
type AcquireResult struct {
	HasLocation bool
	Place       PlaceInfo
	Preseed     string
}

// StoreReader is the narrow shape Acquire needs from the store package.
// Defined here to invert the dependency: location does not import store.
// ingest passes a closure that wraps store.Location().
type StoreReader interface {
	Location() (PlaceInfo, bool)
}

// Acquire runs the cold-start cascade:
//
//  1. If the store has a saved PlaceInfo → fast-path (HasLocation: true).
//  2. Else compute Preseed = PreseedFromIANA(HostIANA()) — may be empty.
//
// The cascade is deliberately Linux-only (R10) and never falls back to a
// hardcoded "Berlin" or any other city — that anti-pattern is explicit in
// §6 anti-corruption and §1 Explicitly-not-doing.
func Acquire(reader StoreReader) AcquireResult {
	if reader != nil {
		if place, ok := reader.Location(); ok {
			return AcquireResult{HasLocation: true, Place: place}
		}
	}
	return AcquireResult{Preseed: PreseedFromIANA(HostIANA())}
}
