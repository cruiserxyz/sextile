# Synthetic chart fixtures

Every chart fixture in this directory and its `charts/` descendant is
constructed synthetic data for development and CI. Its label, moment, place,
and coordinates are test inputs, not a client's record.

The chart fixtures are public sky events (two equinoxes, a solstice, a full
moon, a solar eclipse and a great conjunction), chosen so a reader can verify
each with Astrolog itself; the recorded outputs here come from a natal cast the
day after the 1985 vernal equinox at Greenwich, the Sun at 0 Aries 55.

A future fixture may be added only when this declaration remains true: do not
copy, derive from, or verify against an end-user chart library. No automatic
check can determine a chart datum's provenance; the change author and reviewer
make that judgment under the development/CI data boundary in
[`specs/frames/residue.md`](../../../specs/frames/residue.md).
