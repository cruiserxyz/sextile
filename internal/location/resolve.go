package location

import "fmt"

// Resolve walks the candidate chain and returns the first one that yields a
// usable PlaceInfo. Resolution rules per slice:
//
//  1. Coords present + tz present: use as-is. Atlas is NOT consulted —
//     coords are authoritative, no city validation.
//  2. Coords present + tz absent: use coords + name as-is, returning an
//     empty TZ. Downstream compute will refuse; the warning surfaces the
//     "no tz" gap to the user.
//  3. Coords absent + name present: atlas lookup by "<name> <country>"
//     against the supplied index, narrowed by what the candidate already
//     knows (see narrowMatches). Exactly one survivor resolves; more than
//     one is REFUSED rather than guessed, and falls through. Resolved
//     PlaceInfo comes from the atlas row (name, country, lat, lon, tz).
//  4. Single-of-two coords (lat present XOR lon present): treated as
//     invalid partial. Warn and fall through to the next candidate.
//  5. Empty candidate (no name, no tz, no coords): silently skipped.
//
// On full chain miss, returns a non-nil error and accumulated warnings.
// The caller (typically the UI startup handler) routes warnings to the
// log panel and falls back to the atlas modal for explicit picker.
//
// Per the 2026-04-29 design review: there is intentionally no hardcoded
// "Berlin" technical fallback. A silently-cast Berlin chart is worse than
// refusing — refusal opens the picker, the user sees what's happening.
//
// That rule had a hole until 2026-09-20: it governed an atlas MISS but not an
// ambiguous HIT, which took matches[0] while warning "first of N matches". A
// host in Europe/Amsterdam therefore opened every chart in New Amsterdam,
// Guyana — six hours of offset and the wrong hemisphere, silently, because
// the atlas lists Guyana first and the tie broke on file order. Guessing a
// city is the same sin as defaulting to Berlin, so it now refuses too.
func Resolve(candidates []Candidate, atlas []Indexed) (Resolution, error) {
	var warnings []string
	for _, c := range candidates {
		if c.Place.Empty() {
			continue
		}
		if c.Place.LatPresent != c.Place.LonPresent {
			warnings = append(warnings,
				fmt.Sprintf("candidate %q: only one of lat/lon supplied; falling through", c.Source))
			continue
		}
		if c.Place.HasCoords() {
			if c.Place.TZ == "" {
				warnings = append(warnings,
					fmt.Sprintf("candidate %q: coords present but no tz; downstream compute will need one",
						c.Source))
			}
			return Resolution{
				Place: PlaceInfo{
					Name:    c.Place.Name,
					Country: c.Place.Country,
					Lat:     c.Place.Lat,
					Lon:     c.Place.Lon,
					TZ:      c.Place.TZ,
				},
				Source:   c.Source,
				Warnings: warnings,
			}, nil
		}
		// No coords. Atlas lookup is the only path.
		if c.Place.Name == "" {
			warnings = append(warnings,
				fmt.Sprintf("candidate %q: no coords and no name; falling through", c.Source))
			continue
		}
		if atlas == nil {
			warnings = append(warnings,
				fmt.Sprintf("candidate %q: name-only and atlas not loaded; falling through", c.Source))
			continue
		}
		query := c.Place.Name
		if c.Place.Country != "" {
			query = c.Place.Name + " " + c.Place.Country
		}
		// cap 0 = every match, deliberately. Ambiguity is a judgement about
		// the WHOLE match set: a set truncated to five can look unambiguous,
		// or can hide the very row the timezone would have chosen.
		matches, _ := Filter(atlas, query, 0)
		if len(matches) == 0 {
			warnings = append(warnings,
				fmt.Sprintf("candidate %q: %q not found in atlas; falling through", c.Source, c.Place.Name))
			continue
		}
		narrowed, how := narrowMatches(matches, c.Place)
		if len(narrowed) > 1 {
			warnings = append(warnings, fmt.Sprintf(
				"candidate %q: %q is ambiguous — %d atlas entries match%s (e.g. %s and %s); "+
					"refusing to guess, falling through",
				c.Source, c.Place.Name, len(narrowed), how,
				narrowed[0].Display(), narrowed[1].Display()))
			continue
		}
		hit := narrowed[0]
		if len(matches) > 1 {
			warnings = append(warnings, fmt.Sprintf(
				"candidate %q: %q resolved to %q (%d atlas entries matched%s)",
				c.Source, c.Place.Name, hit.Display(), len(matches), how))
		}
		return Resolution{
			Place:     hit,
			Source:    c.Source,
			Warnings:  warnings,
			UsedAtlas: true,
		}, nil
	}
	return Resolution{Warnings: warnings},
		fmt.Errorf("location: Resolve: no candidate yielded a usable place")
}

// narrowMatches reduces an atlas match set using what the candidate already
// knows, so that "refuse when ambiguous" fires on real ambiguity rather than
// on every city whose name is a substring of another.
//
// Two narrowings, in this order, each skipped when it would empty the set:
//
//  1. THE TIMEZONE, when the candidate carries one. For the host preseed this
//     is not a heuristic: the zone is the very datum the city name was
//     extracted from, so "Europe/Amsterdam" naming an Amsterdam in Guyana is
//     a contradiction in terms. A zone that matches nothing is treated as no
//     information rather than as a refusal, because atlas zone data is
//     carried forward row to row and can be sparse.
//  2. THE EXACT NAME. Filter's own "exact" tier cannot do this: its search key
//     is "<name> <country>", so a bare "amsterdam" never equals "amsterdam nl"
//     and every city merely CONTAINING the query lands in the same tier, where
//     ties break on atlas file order. Here the city whose name IS the query
//     beats the city that merely contains it, which is what separates
//     Amsterdam from Amsterdam-Zuidoost.
//
// Returns the narrowed set and a short phrase naming what narrowed it, for the
// warning — a resolution the operator cannot account for is one they cannot
// correct.
func narrowMatches(matches []PlaceInfo, want RawPlace) ([]PlaceInfo, string) {
	how := ""
	if want.TZ != "" {
		zoned := make([]PlaceInfo, 0, len(matches))
		for _, p := range matches {
			if p.TZ == want.TZ {
				zoned = append(zoned, p)
			}
		}
		if len(zoned) > 0 {
			matches, how = zoned, " in "+want.TZ
		}
	}
	wantName := lowerASCII(want.Name)
	exact := make([]PlaceInfo, 0, len(matches))
	for _, p := range matches {
		if lowerASCII(p.Name) == wantName {
			exact = append(exact, p)
		}
	}
	if len(exact) > 0 {
		matches = exact
	}
	return matches, how
}
