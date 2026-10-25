// Package location owns the cold-start location-acquisition cascade and the
// city-pick atlas surface. It carries Sextile's PlaceInfo type (lat/lon/tz)
// and reads astrolog's bundled atlas.as at runtime — never bakes the data
// into Sextile, never ships a copy.
//
// Atlas data origin (AT RUNTIME, not embedded):
//
//	/usr/local/share/astrolog/atlas.as
//
// That file ships with astrolog and is composed by the Astrolog project from
// GeoNames "Free Gazetteer Data" (CC BY 4.0). Sextile reads it in-process
// the same way astrolog itself does. Anti-corruption §6's prior ban on
// direct atlas.as parsing is lifted for cp2 — the cleaner trade is to share
// astrolog's atlas verbatim rather than ship a duplicate.
//
// This package is a perimeter package: it is read by ingest at startup and
// by ui at modal-Enter. It does not import internal runtime packages and is
// never consulted from inside a chart-compute path.
package location
