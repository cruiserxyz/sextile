package astrolog

import (
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The objects these tests name most, as the registry the fixtures were
// captured from (testdata/ho-all.txt) numbers and names them. They live in the
// tests: the production code reads numbers and holds no such table.
var (
	objSun        = aspect.Object{Number: 1, Name: "Sun"}
	objMoon       = aspect.Object{Number: 2, Name: "Moon"}
	objMercury    = aspect.Object{Number: 3, Name: "Mercury"}
	objVenus      = aspect.Object{Number: 4, Name: "Venus"}
	objMars       = aspect.Object{Number: 5, Name: "Mars"}
	objJupiter    = aspect.Object{Number: 6, Name: "Jupiter"}
	objSaturn     = aspect.Object{Number: 7, Name: "Saturn"}
	objUranus     = aspect.Object{Number: 8, Name: "Uranus"}
	objNeptune    = aspect.Object{Number: 9, Name: "Neptune"}
	objPluto      = aspect.Object{Number: 10, Name: "Pluto"}
	objChiron     = aspect.Object{Number: 11, Name: "Chiron"}
	objNorthNode  = aspect.Object{Number: 16, Name: "North Node"}
	objSouthNode  = aspect.Object{Number: 17, Name: "South Node"}
	objLilith     = aspect.Object{Number: 18, Name: "Lilith"}
	objAscendant  = aspect.Object{Number: 22, Name: "Ascendant"}
	objNadir      = aspect.Object{Number: 25, Name: "Nadir"}
	objDescendant = aspect.Object{Number: 28, Name: "Descendant"}
	objMidheaven  = aspect.Object{Number: 31, Name: "Midheaven"}
)

// objectNums is the objects' Astrolog numbers, in the given order.
func objectNums(objects ...aspect.Object) []int {
	nums := make([]int, 0, len(objects))
	for _, o := range objects {
		nums = append(nums, o.Number)
	}
	return nums
}

// objectSet is the Add/Remove set of the objects, by Astrolog's numbers.
func objectSet(objects ...aspect.Object) recipe.ObjectSet {
	return recipe.NewObjectSet(objectNums(objects...)...)
}
