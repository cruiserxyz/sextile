# Contributing

Sextile is small and opinionated: Astrolog computes every astrological value, Sextile only arranges its output.
Bug reports and pull requests are welcome on GitHub.

## How a change gets in

GitHub is a mirror. Sextile is made in a workshop elsewhere. Each release comes to GitHub as two commits: the
program, then its changelog. So a pull request is not merged. It is read, and the change is made by hand in the
workshop.

The changelog of that release thanks you by name. The pull request is closed with a note that names the release
the change lands in. This keeps one history, and every line in it passes the same checks.

## Before you open a pull request

- `./check` must pass. It runs gofmt, go vet, the tests and the repository's content rules. It needs Astrolog 8.00
  on PATH with its complete data. That includes the 1200-1800 and the dwarf-planet ephemeris files.
  `scripts/build-astrolog` installs them, and `./check` refuses an Astrolog without them. `./check --release` also
  needs `shellcheck`.
- Commit messages: `area: imperative subject` of at most 60 characters, a blank line, then one paragraph that
  says why. No trailers.
- Never compute a position, aspect, dignity or house in Go. Never add a cache in the Astrolog path. Never let a
  random or probabilistic decision into it.
- Test charts are events, or public figures whose birth data is published, with the source named in the file. Never
  add a client's or a private person's birth data.
- Sextile writes `name.as`, Astrolog's own chart file. It recognises a chart by its first bytes, never by its name.
  No code, test, fixture, golden or document here names a chart with the retired extension. `./check` refuses that
  token, the old chart-kind identifier and the old session `kind` word everywhere.

The changelog thanks each contributor in the item of their change.
