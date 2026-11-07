package ui

import "github.com/cruiserxyz/sextile/pkg/aspect"

// The objects these tests name most, as the registry the fixtures were
// captured from (internal/astrolog/testdata/ho-all.txt) numbers and names them.
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
