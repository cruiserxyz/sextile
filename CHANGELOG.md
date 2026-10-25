# Changelog

Each version opens with its most important practical change, then lists the
rest one outcome per bullet, then what still limits use and what an upgrade
changes for you. Astrolog computes every astrological value Sextile shows.
The format follows Keep a Changelog; versions are semantic, tagged vX.Y.Z.

## [0.1.0] - 2026-10-25

Sextile 0.1.0 opens your Astrolog chart files in the terminal and compares each
with the sky of the current minute, which follows the clock, or of any moment
you step to. Every position, house and aspect is computed by Astrolog 8.00,
Walter D. Pullen's free astrology program, with the Swiss Ephemeris; Sextile
arranges what Astrolog returns, and its log console lists every Astrolog
command it ran.

### Added

- `./sextile` builds a Docker image and starts Sextile in one step, or with
  `./sextile --no-docker` builds and runs the checkout with the host's Go and
  Astrolog instead. It asks once which folder holds your charts, removes older
  images it built, and `./sextile doctor` says what is wrong without changing
  anything. The image
  compiles Astrolog 8.00 from its checksum-pinned source;
  `scripts/build-astrolog` does the same without Docker.
- Both install the same checksum-verified Swiss Ephemeris files: planets, the
  Moon and the main asteroids from 1200 to 2400, and Hygiea, Eris, Haumea,
  Makemake, Gonggong, Quaoar, Sedna and Orcus from 1500 to 2100.
- Charts are Astrolog chart files in a folder you own, listed by their content
  whatever their file name, in a type-to-filter picker. `Ctrl+S` saves the
  floating chart, its moment and city, as a new `name.as` file and never
  replaces a chart; titling an untitled chart rewrites its file only after you
  confirm, keeping the previous one in `.sextile/backup`. One sample chart,
  Hamburg at noon on 24 October 2026, is in `examples/` to open first.
- The floating chart starts on the current minute and follows the clock (its
  row reads `~ Now`). Stepping by a year, month, week, day, hour or minute
  stops it there; `n` returns to now and follows again, `N` goes back.
- Bodies with sign, degree, house, speed and station marks; aspects ranked by
  strength, marked applying or separating, and filtered by the aspects and
  bodies you choose. Selected bodies limit that chart's wheel, aspects and
  report to themselves. Each chart keeps its own house system (the one its file
  names) and zodiac (tropical unless you choose another), changeable without
  touching the file.
- Select two or more charts and the Aspects panel reads across them, naming
  whose body is whose. The chart under the cursor is the one you read; the
  tab, labelled like `② in ①`, picks the chart it is read against, whose
  houses and zodiac frame it.
- Details shows Astrolog's report for the body under the cursor; its Timeline
  tab lists the floating moment's month of events, plus each selected chart's
  transits and returns. Rows or the report copy to the clipboard.
- `Ctrl+X` has Astrolog draw the wheel and serves it to a browser on this
  machine, refreshed as time moves.
- A city picker over Astrolog's atlas, proposing a city from your time zone on
  the first run, which `, c d` keeps as your default along with the floating
  chart's house system and zodiac; one clock setting (24- or 12-hour, month- or day-first) for
  every time shown. Charts, selection and house and zodiac choices come back
  when you reopen; a second window opens with them too, and what you change
  there is not kept.

### Known limitations

- It adds no interpretation: Details shows Astrolog's own reports, and there
  are no delineations.
- There is no birth-data form. A chart for another moment is saved from the
  floating chart after stepping to it and choosing its city, or written as an
  Astrolog chart file; Astrolog's multi-chart list files are not read.
- Bodies outside their ephemeris range, the planetary moons and the planets'
  centres of body are not drawn; the Log names a body left out. A comparison
  does not yet follow the body selection.
- `.sextile/` in the charts folder keeps the names, birth data and places of
  the charts last open; sharing or syncing the folder shares them.
- The terminal needs at least 66x15 for one chart, a row
  more per further chart; Details and the Log sit beside the charts from 132
  columns, plus 12 per extra chart.
- Names and places longer than 54 bytes are cut, ending in `…`, because
  Astrolog's report header holds no more; accented letters count as two. A
  chart file written elsewhere with a longer place may fail to open; Sextile's
  own saves never write one.
- Docker is the install, on Linux or macOS, or from a WSL terminal on Windows;
  there are no prebuilt binaries. Linux and Windows through WSL are tested;
  macOS is expected to work but untested. Without Docker, Astrolog 8.00 must
  be installed, and `scripts/build-astrolog` expects Linux.
