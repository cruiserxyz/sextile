package astrolog

import (
	"context"
	"fmt"
	"sort"
)

// DefaultSets are the two object sets Astrolog lists when nothing overrides
// them: Main is what a natal or sky search prints, Transit what the transiting
// side of `-t`/`-tr` prints. They are READ from the binary, never written down
// here, so a changed astrolog.as is honoured (C11).
//
// Measured on astrolog 8.00 (the specification): `-HO`
// lists 1-10, 16, 22, 31; `-R2 -HO` lists 1, 3-10, 16 (no Moon). `-RT -HO`
// does NOT list the transit set — `-R2` copies it into the main one.
type DefaultSets struct {
	Main    []int
	Transit []int
}

// ReadDefaultSets reads both default sets through the spawn seam as exempt
// calls (they are metadata lists, no chart) and checks each against the object
// registry `-R1 -HO` returns. It refuses — never guesses — when either read
// fails, parses to nothing, or names an object the registry does not have, so
// a caller can never run a search it could not band.
func ReadDefaultSets(ctx context.Context) (DefaultSets, error) {
	registry, err := ObjectRegistry(ctx)
	if err != nil {
		return DefaultSets{}, fmt.Errorf("astrolog: default object sets: object registry: %w", err)
	}
	known := make(map[int]bool, len(registry))
	for _, o := range registry {
		known[o.Number] = true
	}
	main, err := readMainDefault(ctx)
	if err != nil {
		return DefaultSets{}, fmt.Errorf("astrolog: default object sets: main set (-HO): %w", err)
	}
	transit, err := readTransitDefault(ctx)
	if err != nil {
		return DefaultSets{}, fmt.Errorf("astrolog: default object sets: transit set (-R2 -HO): %w", err)
	}
	for _, side := range []struct {
		name string
		nums []int
	}{{"main", main}, {"transit", transit}} {
		for _, n := range side.nums {
			if !known[n] {
				return DefaultSets{}, fmt.Errorf("astrolog: default object sets: the %s set lists object %d, which the object registry does not have", side.name, n)
			}
		}
	}
	return DefaultSets{Main: main, Transit: transit}, nil
}

// readMainDefault runs `-HO`: the objects a chart lists with no override.
func readMainDefault(ctx context.Context) ([]int, error) {
	out, _, err := spawn(ctx, newExempt("default main object set: -HO lists the objects shown by default, no chart", "-HO"))
	if err != nil {
		return nil, err
	}
	return objectNumbers(out)
}

// readTransitDefault runs `-R2 -HO`: `-R2` copies the transiting set into the
// main one, so the listing that follows is the transiting side's defaults.
func readTransitDefault(ctx context.Context) ([]int, error) {
	out, _, err := spawn(ctx, newExempt("default transit object set: -R2 -HO lists the objects the transiting side shows by default, no chart", "-R2", "-HO"))
	if err != nil {
		return nil, err
	}
	return objectNumbers(out)
}

// objectNumbers parses an object listing with the registry parser, which
// refuses an unparseable or duplicated number and a listing with no rows, and
// returns the numbers ascending.
func objectNumbers(out string) ([]int, error) {
	rows, err := ParseObjectRegistry(out)
	if err != nil {
		return nil, err
	}
	nums := make([]int, 0, len(rows))
	for _, r := range rows {
		nums = append(nums, r.Number)
	}
	sort.Ints(nums)
	return nums, nil
}
