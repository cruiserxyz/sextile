package testenv

import (
	"context"
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
	houses recipe.HouseSystem, zodiac recipe.Zodiac, objects recipe.ObjectPolicy, aspects recipe.AspectPolicy) (string, []aspect.CrossAspect, error) {
	spec, err := recipe.NewChartSpec(recipe.Comparison{
		Primary:   recipe.MomentAtPlace{JD: JD(natal), Lon: natalAt.Lon, Lat: natalAt.Lat},
		Secondary: recipe.MomentAtPlace{JD: JD(transit), Lon: transitAt.Lon, Lat: transitAt.Lat},
		Relation:  recipe.TransitToNatal,
	}, houses, zodiac, objects)
	if err != nil {
		return "", nil, err
	}
	spec.Aspects = aspects
	out, err := astrolog.ComputeCrossAspects(ctx, spec)
	if err != nil {
		return "", nil, err
	}
	return out.Report, out.Aspects, nil
}
