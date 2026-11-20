package lookup

import (
	"context"
	"sync"

	"github.com/cruiserxyz/sextile/internal/astrolog"
)

// ephemeris.go carries the availability fact across the perimeter: which
// objects this Astrolog has the ephemeris file for. internal/ui may not import
// internal/astrolog, so the value and its groups are re-exported here.

// EphemerisAvailability is what the probe learned about one data folder: the
// objects with no ephemeris file, the file each needed, the folder asked about.
type EphemerisAvailability = astrolog.Availability

// EphemerisGroup is the absent objects one missing file would bring back.
type EphemerisGroup = astrolog.AbsentGroup

// ephemerisProbe asks Astrolog (a seam for tests).
var ephemerisProbe = astrolog.ProbeAvailability

// ephemerisFolder names the data folder the answer belongs to (a seam).
var ephemerisFolder = astrolog.AvailabilityFolder

var (
	ephemerisMu    sync.Mutex
	ephemerisCache = map[string]astrolog.Availability{}
)

// ProbeEphemeris reads the availability of the current data folder, once per
// folder per process, and installs it so every argv Sextile builds from here
// on leaves an absent object out. A failed probe installs nothing and is not
// cached.
func ProbeEphemeris(ctx context.Context) (EphemerisAvailability, error) {
	key := ephemerisFolder()
	ephemerisMu.Lock()
	defer ephemerisMu.Unlock()
	if av, ok := ephemerisCache[key]; ok {
		astrolog.SetAvailability(av)
		return av, nil
	}
	av, err := ephemerisProbe(ctx)
	if err != nil {
		return EphemerisAvailability{}, err
	}
	av.Folder = key
	ephemerisCache[key] = av
	astrolog.SetAvailability(av)
	return av, nil
}

// OnScreenObjects is the objects the lens draws or lists, before the cut for a
// missing ephemeris file: the one rule for what a chart draws
// (astrolog.WantedObjectNumbers) resolved against av, so under a delta baseline
// it holds the host's own default set, where an asteroid the operator never
// ticked comes from, and an Only narrowing leaves out the rest. The account of
// what cannot be drawn names the ones av lacks a file for. It fails when av
// carries no default set.
func OnScreenObjects(av EphemerisAvailability, l Lens) ([]int, error) {
	return astrolog.WantedObjectNumbers(l.Objects, av)
}

// ForgetEphemeris drops every cached answer and installs the empty one, so the
// next ProbeEphemeris asks Astrolog again. A test that pointed Astrolog at a
// different data folder calls it when it is done.
func ForgetEphemeris() {
	ephemerisMu.Lock()
	ephemerisCache = map[string]astrolog.Availability{}
	ephemerisMu.Unlock()
	astrolog.SetAvailability(astrolog.Availability{})
}

// NoDataBody is the body an Astrolog warning line says has no ephemeris data
// at the chart's moment, or "".
var NoDataBody = astrolog.NoDataBody
