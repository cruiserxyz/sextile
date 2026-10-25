# Sextile

**Astrology at your fingertips.**

Open a chart. Move through time. Keep a moment worth returning to.

Sextile is a keyboard-operated workspace for [Astrolog](https://www.astrolog.org/). It puts charts, planetary positions,
aspects and a timeline on one screen, so you can follow a question as it develops.

![Sextile showing the solar eclipse of 12 August 2026 beside the sky as Venus moves back into Libra: bodies, aspects and the month's timeline](docs/screenshot.svg)

Astrolog, created by Walter D. Pullen, computes every astrological value you see. Sextile arranges what it says and keeps the commands in
view. Why a terminal? Because Astrolog is a command-line program, and the quickest way to learn what it can do is to watch the
commands go by while you work. Press `?` on the main screen and Sextile lists the keys that apply right there.

[Install and run](#install-and-run) · [Try one question](#try-one-question) · [Every key and setting](docs/REFERENCE.md) · [What changed](CHANGELOG.md)

## What you can do

- **Compare a chart with now.** Open a chart and it sits beside the floating chart, which starts on the current minute. Select
  both and the Aspects panel reads across them, naming whose body is whose and whose houses you are looking through.
- **Move through time.** Step the floating chart by a year, a month, a week, a day, an hour or a minute, as fast as you hold the
  key. Its Timeline tab lists the month's events and each selected chart's transits and returns; `Enter` on a row moves the floating chart to that moment.
- **Change the view, not the file.** Choose the house system and zodiac for each chart, and the aspects and bodies for the whole session; the chart file stays as it was.
  Aspects are sorted by Astrolog's strength and marked applying or separating. Which ones matter is yours to judge.
- **Keep a moment.** `Ctrl+S` saves the floating chart, its moment and its city, as a new Astrolog chart file.
- **See the calculation.** The Log lists the Astrolog commands behind the screen. `Enter` copies one; run it wherever Astrolog is
  installed and the same numbers come back. If you are new to Astrolog, this is also the gentlest way to learn its switches.

Each row of the Chart panel is a slot: a chart you opened, or the floating chart.

## Your frame, whatever your school

Every chart's row in the Chart panel ends in a four-letter tag naming its frame: two letters for the houses, two for the
zodiac. `WhTr` is Whole Sign houses in the tropical zodiac, what a chart file that names no house system opens in. A
Hellenistic reader will feel at home there; a modern Western reader may want `PlTr`, Placidus tropical; a Vedic reader `WhLa`,
Whole Sign in the Lahiri zodiac. There is no bare sidereal tag: a sidereal chart always names its ayanamsa.

| House codes | Zodiac codes |
| --- | --- |
| `Pl` Placidus | `Tr` Tropical |
| `Ko` Koch | `La` Lahiri |
| `Wh` Whole Sign | `Fa` Fagan-Bradley |
| `Eq` Equal | `Ra` Raman |
| `Po` Porphyry | `Kr` Krishnamurti |
| `Re` Regiomontanus | |
| `Ca` Campanus | |

All 22 house systems and 11 zodiacs are in the [reference](docs/REFERENCE.md#frame-codes). `Ctrl+H` chooses the houses and
`Ctrl+Z` the zodiac of the focused chart only; type to filter, and the popup names the chart. `Enter` applies
the one under the cursor and `Esc` leaves the chart as it was; the chart file is never written, and your choices are back when you quit and reopen. `,` `c` `d`
makes the floating chart's frame your default, so every chart whose file names no house system opens in it; charts that are open when you press it keep the frame they show. If your terminal
keeps those keys for itself, `,` then `h` or `,` then `z` do the same.

In a comparison, the aspects are read in the houses and zodiac of the chart named on the right of the Aspects title, `② in ①`
style, so two readers from two schools each choose the frame they trust. The Log shows the Astrolog command behind it, the `-c`
house switch and the sidereal switch included.

The house column of the Bodies panel is drawn the way you read houses, `,` then `a`: angularity at a glance (the houses grouped
around the four angles, the default), true quadrants (1-3, 4-6, 7-9, 10-12), the plain number, or the number with A, s or c for
angular, succedent and cadent. `,` `c` `d` keeps this too, with your city and frame. The four, drawn out:
[reference](docs/REFERENCE.md#house-display).

## Install and run

You need Docker 28 or newer, running. Linux and Windows (from a WSL terminal) are tested; macOS is expected to work but untested.

```bash
curl -fsSL https://raw.githubusercontent.com/cruiserxyz/sextile/v0.1.1/scripts/install | sh
```

The [install script](scripts/install) is short and worth a read. It checks that git and Docker are there, clones Sextile 0.1.1
into `~/sextile` (`SEXTILE_DIR` chooses another place), moves an existing Sextile clone to that release, refuses any other
existing folder, prints the start command, and starts nothing.

```bash
cd ~/sextile
./sextile doctor
./sextile
```

The first run asks which folder holds your charts, then builds the image: Astrolog 8.00, compiled from checksum-pinned sources, with its Swiss Ephemeris
files, a few minutes, once. Sextile opens on the floating chart at the current minute, in a city
proposed from your time zone (`Ctrl+L` chooses another).

**Without Docker.** `./sextile --no-docker` builds this checkout and runs it straight on your machine when `go` and `astrolog` are installed;
`scripts/build-astrolog` installs the same Astrolog in about two minutes (Linux). Same charts folder, same port, same wheel in the
browser; the Log's first line says which mode ran. If `go` or `astrolog` is missing, the launcher says so and names the script;
it never falls back to Docker on its own.

Sextile keeps its session in the charts folder, in `.sextile/`: the titles, dates, times and places of the charts you had open.
Sharing or syncing the folder shares that chart data. The terminal needs at least 66 columns by 15 rows with one chart (a row more for each further chart); Details and the Log sit
beside the charts from 132 columns, plus 12 for each chart you open. Other routes, options and settings:
[reference](docs/REFERENCE.md#install-options).

## Try one question

What is this minute's sky doing to a fixed moment? Try it on the included event chart, Hamburg at noon on 24 October 2026,
12:00 CEST (UTC+2): [examples/Demo-Hamburg-2026-10-24.as](examples/Demo-Hamburg-2026-10-24.as).

1. Copy that file into your charts folder.
2. Press `Ctrl+O`, type `Hamburg`, press `Enter`. "Hamburg noon" joins the Chart panel above the floating chart and both are
   selected, so the comparison is on. Opening a chart never moves the floating chart.
3. Press `↓` to put the cursor on the floating chart, then `]`. The Aspects tab reads `② in ①` (`2 in 1` without colour):
   this minute's bodies, read in Hamburg noon's houses and zodiac.
4. Press `h` a few times. Each press moves the floating chart an hour on; Hamburg noon stays, and the aspects re-sort as orbs
   close and open. `n` returns to now.
5. Press `,` then `l`: the Astrolog commands behind what you just saw. `Enter` copies one, `Esc` closes.

To keep a stepped moment, choose its city with `Ctrl+L` and press `Ctrl+S`. `?` lists every key.

## Practical boundaries

- **Install.** Docker is the supported route; there are no prebuilt binaries, and the native build expects Linux.
- **What it writes.** Charts you save (never over an existing one), backups of retitled charts, the session and the settings
  you keep; the launcher remembers your charts folder in `~/.config/sextile/`.
- **No birth-data form.** A chart for another moment is the floating chart stepped there and saved, or an Astrolog chart file
  you copy in.
- **No interpretation.** Details shows Astrolog's own reports; Sextile adds none of its own.
- **Bodies.** Drawn only within their ephemeris dates (about 1500 to 2100 for the eight added bodies); no planetary moons. A
  comparison does not yet follow the body selection.
- **Several windows.** The first keeps your session; later windows open with the same charts and keep nothing between runs.
  Keyboard only.

The full list: [Limits](docs/REFERENCE.md#limits).

## What comes next

Sextile casts nothing; Astrolog does. Everything below keeps that vow: Sextile arranges what Astrolog says and shows the
sentence it sent, so if you delete Sextile tomorrow you have lost no data and learned nothing false. The road from here is
plotted in eight steps. The first is next; none has a date.

1. **The room settles.** A fresh start greets you in two lines, every message is in a person's words, and the Log reads like
   what you did. The dozen reads an astrologer asks for several times an hour, risings and settings, the lunar phase, dignities,
   midpoints, each become a tab, and Astrolog's own interpretation text appears on request, never mixed in.
2. **Lenses.** A chart is a moment and a place; the house system, the zodiac, the centre you count from are ways of looking.
   Each becomes a lens: a chooser, a name on screen, no change to the file, and your choice is back after a relaunch.
3. **The switch line.** Type an Astrolog switch and it opens as a tab; every tab copies out as exactly one Astrolog sentence;
   a recipe you kept in March runs again in June. If you already speak Astrolog's switches, this step is written for you.
4. **The chart has children.** A rectification variant, a relocation, a progression, a return: each a slot bound to its parent,
   indented under it, moved through time with the same keys, so the derivation is visible and never done by hand.
5. **Time, searched.** Stepping is the consulting gesture; searching is the planning one. When was this aspect last exact, when
   is it next, one key each; a timeline over any window, with filters. Sextile never computes a hit; it asks Astrolog and shows
   you the rows.
6. **The technique library.** A technique is a file: its Astrolog expression, its panel, its words. Mundane, horary, time lords
   and dignities, each rebuilt from a cited source and checked against that source's worked example before it ships.
7. **The wheel comes home.** Astrolog's own text wheel in the terminal, refreshing as time moves; `Ctrl+X` comes to mean "show
   me Astrolog's drawing of this".
8. **The research lane, 1.0.** A release you can cite, a session you can replay from the Log, exports that carry their
   settings, and a charts folder from 0.1.0 that opens intact.

## Contribute

Most useful is an account: the question you were pursuing, and where the workflow helped or got in the way. A chart that breaks
something is welcome, but never a real person's birth data. [CONTRIBUTING.md](CONTRIBUTING.md) has the commit format and the
check; [SECURITY.md](SECURITY.md) says how to report a vulnerability.

## Credits and licence

Sextile has two inspirations. [Astrolog](https://www.astrolog.org/), Walter D. Pullen's free astrology program, has computed
charts since 1991 and is the only thing here that computes the sky. [lazygit](https://github.com/jesseduffield/lazygit), by
Jesse Duffield, showed that a terminal program can make a tool's model visible without replacing it: panels for the nouns, one
key per verb, `?` for what applies here, and a log of the real commands it ran. Sextile makes that move for Astrolog.

Astrolog calculates with the Swiss Ephemeris by Astrodienst AG; its city atlas draws on GeoNames data. The interface is built on
Charm's [Bubble Tea](https://github.com/charmbracelet/bubbletea). Sextile is MIT-licensed (see [LICENSE](LICENSE) and
[CONTRIBUTORS](CONTRIBUTORS)); Astrolog, the Swiss Ephemeris and the atlas data keep their own licences.

*Sextile: the aspect of ease, when two planets sit 60° apart and work flows. Also the tool.*
