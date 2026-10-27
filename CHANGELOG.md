# Changelog

Each version opens with its most important practical change, then lists the
rest one outcome per bullet, then what still limits use and what an upgrade
changes for you. Astrolog computes every astrological value Sextile shows.
The format follows Keep a Changelog; versions are semantic, tagged vX.Y.Z.

## [0.1.2] - 2026-10-27

Your charts folder now holds only your charts. Sextile 0.1.1 kept the session,
the preferences and the backups of retitled charts in a `.sextile` folder inside
it, so syncing or sharing the folder shared them too. They now live in your home
folder, under `~/.local/state`, `~/.config` and `~/.local/share` (`XDG_STATE_HOME`,
`XDG_CONFIG_HOME` and `XDG_DATA_HOME` move them), and `sextile doctor` names them.

- Upgrading from 0.1.1 copies the session, preferences and backups out of
  `<charts>/.sextile` the first time you start on that folder, only where nothing
  is there yet, and the Log says what was copied. The old `.sextile` folder is
  left as it was, for you to remove. With several charts folders, only the first
  one you open brings its session along.
- A session now opens only in the charts folder it was made in. Started on
  another folder, Sextile opens with no charts and the Log says the saved session
  belongs to another folder. A 0.1.1 session that names no folder is not restored.
- Docker mounts the new folders beside the charts folder, creates the missing
  ones private to you, and refuses to start when one is a symlink or a file. A
  container you start by hand with only `-v ~/charts:/charts` mounts nothing
  else, so its state stays in `.sextile` as before.
- Sessions can be named: `,` `o` saves the open charts, selection and frames
  under a name, opens, renames or moves to the trash the ones saved for this charts
  folder, and `sextile -- --session NAME` starts in one. Sextile removes nothing. A named session runs in one window at a
  time.
- A second window now starts empty and keeps nothing until it opens or saves a
  named session. In 0.1.1 it opened with the first window's charts. The first
  window's session is left alone.
- Opening a chart selects nothing. In 0.1.1 the first chart and the floating
  chart came up selected. Now `Space` is the only way a chart becomes selected,
  so comparison and body limits begin from what you chose.
- A chart saved with `Ctrl+S` opens at once with the cursor on it.
- `scripts/install` clones Sextile under `~/.local/share/sextile/app` and writes
  a `sextile` command into `~/.local/bin`, so you start it with `sextile` from
  anywhere. It stops before changing anything if a different file is already
  that command, prints the line to add to your PATH when `~/.local/bin` is not on
  it, edits no startup file, and warns instead of stopping when Docker is
  missing. A clone an earlier install left in `~/sextile` is no longer used or
  updated. The script says so and leaves it for you to remove.
- On the first run the launcher proposes `~/.local/share/sextile/charts` as the
  charts folder, then `Sextile` in your documents folder. It no longer offers
  `~/.charts` or `~/Documents/charts`. A folder you already chose is remembered.
- Popups are as large as their content and no larger, and wide symbols such as
  ① and ● keep a space of their own colour, so they never run into the next
  character in terminals that draw them wider.

## [0.1.1] - 2026-10-25

A chart saved with `Ctrl+S` reopens under the city it was saved in. In 0.1.0 a
city with a district listed close by in Astrolog's atlas could come back under
the district's name: a chart saved in Berlin reopened as Mitte, DE. Now the
place name stored in the chart file decides among the atlas places within one
arc-minute of its coordinates, and a chart file without a place name opens as
before. Charts already saved with 0.1.0 reopen under their own city too.

Save defaults (`,` `c` `d`) no longer changes the charts you have open. In
0.1.0 every open chart whose file names no house system came back after a
restart in the saved house system and zodiac; now each keeps the frame it
showed, and only the floating chart on a fresh start and charts opened
afterwards take the saved default. In a second window Save defaults refuses,
because the first window keeps the session.

0.1.0 was cut but never tagged: its public check failed on GitHub's runner,
and the fixes are in 0.1.1. Upgrading needs nothing else.

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
