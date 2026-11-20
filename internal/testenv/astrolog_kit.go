package testenv

import (
	"context"
	"sync"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The kit below lets a test in a package that must not import astrolog (the
// perimeter: internal/ui) build an INDEPENDENT oracle from Astrolog itself.
// Like the rest of this package it is imported only from _test.go files.

// JD is the Julian Day of an instant.
func JD(t time.Time) float64 { return astrolog.GregorianToJD(t) }

// LocalChartFile is the chart-info file a pair report stages for one chart: the
// instant as the place's local clock and zone.
func LocalChartFile(t time.Time, lon, lat float64, tz string, houses recipe.HouseSystem, zodiac recipe.Zodiac) (string, error) {
	return astrolog.FormatLocalChartFile(recipe.LocalMomentAtPlace{JD: JD(t), Lon: lon, Lat: lat, TZ: tz}, houses, zodiac)
}

// Place is a longitude and latitude, east and north positive.
type Place struct{ Lon, Lat float64 }

// UTTransitCross compiles a transit-over-natal pair with both charts staged in
// UT, the form every pair report took before the clock pins, and returns
// Astrolog's report and parsed cross aspects.
func UTTransitCross(ctx context.Context, natal time.Time, natalAt Place, transit time.Time, transitAt Place,
	houses recipe.HouseSystem, zodiac recipe.Zodiac, objects recipe.ObjectPolicy, aspects recipe.AspectPolicy, registry aspect.Registry, aspectList aspect.AspectRegistry) (string, []aspect.CrossAspect, error) {
	spec, err := recipe.NewChartSpec(recipe.Comparison{
		Primary:   recipe.MomentAtPlace{JD: JD(natal), Lon: natalAt.Lon, Lat: natalAt.Lat},
		Secondary: recipe.MomentAtPlace{JD: JD(transit), Lon: transitAt.Lon, Lat: transitAt.Lat},
		Relation:  recipe.TransitToNatal,
	}, houses, zodiac, objects)
	if err != nil {
		return "", nil, err
	}
	spec.Aspects = aspects
	spec.Registry = registry
	spec.AspectRegistry = aspectList
	out, err := astrolog.ComputeCrossAspects(ctx, spec)
	if err != nil {
		return "", nil, err
	}
	return out.Report, out.Aspects, nil
}

var (
	hostDefaultOnce sync.Once
	hostDefault     astrolog.Availability
)

// HostDefault is the availability a started app holds before it casts anything
// (the startup probe has answered): the host's own default object set, read
// once from the binary (`astrolog -HO`), and no absent object. The compiler has
// no other baseline (slice 8c), so a test that compiles a chart installs this
// first, in its TestMain and again after anything that resets the availability.
// With no Astrolog on PATH it is the thirteen objects the shipped binary lists
// (the classical ten, the North Node, the Ascendant, the Midheaven).
func HostDefault() astrolog.Availability {
	hostDefaultOnce.Do(func() {
		hostDefault = astrolog.Availability{Default: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 16, 22, 31}}
		if sets, err := astrolog.ReadDefaultSets(context.Background()); err == nil && len(sets.Main) > 0 {
			hostDefault.Default = sets.Main
		}
	})
	return hostDefault
}

// InstallHostDefault installs HostDefault for the compiler.
func InstallHostDefault() { astrolog.SetAvailability(HostDefault()) }
