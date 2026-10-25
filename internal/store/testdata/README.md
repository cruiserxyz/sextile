# Synthetic session fixtures

Every file in this directory is constructed synthetic data for development and
CI. Its label, moment, place, and coordinates are test inputs, not a client's
record. `session-v1.yaml` is a verbatim version-1 `session.yaml`, written by
the version-1 writer before the writer moved to version 2 (specs/slot-list.md
C15).

A future fixture may be added only when this declaration remains true: do not
copy, derive from, or verify against an end-user chart library. See the
development/CI data boundary in
[`specs/frames/residue.md`](../../../specs/frames/residue.md).
