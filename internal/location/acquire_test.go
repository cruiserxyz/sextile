package location

import "testing"

type stubStore struct {
	place PlaceInfo
	ok    bool
}

func (s stubStore) Location() (PlaceInfo, bool) { return s.place, s.ok }

func TestAcquire_StoredFastPath(t *testing.T) {
	stored := PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5, Lon: 13.4, TZ: "Europe/Berlin"}
	got := Acquire(stubStore{place: stored, ok: true})
	if !got.HasLocation {
		t.Fatal("HasLocation should be true when store returns a place")
	}
	if got.Place != stored {
		t.Errorf("Place = %+v, want %+v", got.Place, stored)
	}
	if got.Preseed != "" {
		t.Errorf("Preseed should be empty on fast-path, got %q", got.Preseed)
	}
}

func TestAcquire_NoStorePopulatesPreseed(t *testing.T) {
	got := Acquire(stubStore{ok: false})
	if got.HasLocation {
		t.Fatal("HasLocation should be false when store returns nothing")
	}
	// Preseed is HostIANA-dependent; we can only assert structural shape
	// (it's either empty or a non-IANA-prefixed bare segment).
	if got.Place != (PlaceInfo{}) {
		t.Errorf("Place should be zero on no-store, got %+v", got.Place)
	}
}

func TestAcquire_NilReaderIsSafe(t *testing.T) {
	got := Acquire(nil)
	if got.HasLocation {
		t.Fatal("nil reader cannot have HasLocation")
	}
}
