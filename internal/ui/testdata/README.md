# Synthetic chart fixtures

Every chart fixture in this directory is
constructed synthetic data for development and CI. Its label, moment, place,
and coordinates are test inputs, not a client's record.

A future fixture may be added only when this declaration remains true: do not
copy, derive from, or verify against an end-user chart library. No automatic
check can determine a chart datum's provenance; the change author and reviewer
make that judgment under the development/CI data boundary in
[`specs/frames/residue.md`](../../../specs/frames/residue.md).
