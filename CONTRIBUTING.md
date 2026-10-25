# Contributing

Sextile is small and opinionated: Astrolog computes every astrological value, Sextile only arranges its output.
Bug reports and pull requests are welcome on GitHub.

## Before you open a pull request

- `./check` must pass: it runs gofmt, go vet, the tests and the repository's content rules. It needs Astrolog 8.00
  on PATH with its complete data, including the 1200-1800 and dwarf-planet ephemeris files (`scripts/build-astrolog`
  installs it, and `./check` refuses an Astrolog without them). `./check --release` also needs `shellcheck`.
- Commit messages: `area: imperative subject` of at most 60 characters, a blank line, then one paragraph that
  says why. No trailers.
- Never compute a position, aspect, dignity or house in Go; never add a cache in the Astrolog path; never let a
  random or probabilistic decision into it.
- Test fixtures are synthetic. Do not add a real person's birth data.
- Sextile writes `name.as`, Astrolog's own chart file, and recognises a chart by its first bytes, never by its name.
  No code, test, fixture, golden or document in this repository names a chart with the retired extension, and
  `./check` refuses that token, the old chart-kind identifier and the old session `kind` word everywhere.

Contributors are credited in CONTRIBUTORS.
