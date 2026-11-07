# Sextile

**Astrology at your fingertips.**

Open a chart.
Move through time.
Keep a moment worth returning to.

Sextile is a keyboard-operated workspace for [Astrolog](https://www.astrolog.org/). Hold a chart still, move the
sky against it, and watch the aspects between them form and pass. Charts, positions, aspects and a timeline sit on
one screen, so the whole picture moves at once. A moment worth keeping becomes a chart file. A set of open charts
becomes a session you can come back to.

![Sextile at 144 columns. The solar eclipse of 12 August 2026 is chart 1, with Bodies, Aspects and the month's Timeline beside it.](docs/screenshot.svg)

Astrolog, created by Walter D. Pullen, computes every astrological value you see. Sextile arranges what it says and
keeps the commands in reach. Fuller credits are at the end.

[Install and run](#install-and-run) · [Try one question](#try-one-question) · [Every key and setting](docs/REFERENCE.md) · [What changed](CHANGELOG.md)

## Why try it

- **Two moments at once.** One chart holds a moment you opened. The floating chart holds now, or any moment you step
  to. Select both, and the Aspects panel reads one in the other's houses and zodiac, saying whose body is whose.
- **Time under your fingers.** Step the sky by a year, a month, a day or a minute. It moves as fast as you hold the
  key. The Timeline lists the month's events. One key brings you back to now.
- **Nothing hidden.** Every number comes from Astrolog. The log console lists the commands behind the screen, and
  `Enter` copies one, so the same numbers come back wherever Astrolog is installed.
- **Your judgement, your files.** Sextile adds no interpretation. Charts are Astrolog's own files, and a view you
  change on screen never changes the file.
- **Every key in sight.** `?` lists the keys that apply right there. `Ctrl+P` lists every command with its key.

*Why a terminal?* Astrolog is a command-line tool. The quickest way to learn what it can do is to watch its commands
go by while you work.

### Before you install

Sextile is operated from the keyboard, with no mouse. It opens Astrolog chart files you already have. New ones come
from the floating chart: step it to a moment, choose a city, save. There is no birth-data form.

## Install and run

Docker is the supported route. It is tested on Linux and on Windows from a WSL terminal. macOS is expected to work
but is untested, so feedback is welcome. The [native route](#without-docker) needs Linux.

You need `git`, `curl` and Docker 28 or newer, running. First, fetch Sextile:

```bash
curl -fsSL https://raw.githubusercontent.com/cruiserxyz/sextile/v0.1.3/scripts/install | sh
```

The [install script](scripts/install) is short and worth a read. It clones Sextile into
`~/.local/share/sextile/app` and writes a small `sextile` command into `~/.local/bin`. It starts nothing and never
edits your shell's settings. If `~/.local/bin` is not on your PATH, it prints the line to add. Other places and
options are in the [reference](docs/REFERENCE.md#install-options).

Then check the machine and start:

```bash
sextile doctor
sextile
```

`sextile doctor` checks Docker, the port and the charts folder, and says what is wrong without changing anything.

`sextile` asks which folder holds your charts the first time. It proposes `~/.local/share/sextile/charts` and offers
your documents folder too. That is your charts folder from then on. The first start then builds a Docker image with
Astrolog 8.00 and its Swiss Ephemeris files. That takes a few minutes. Later starts skip it.

Sextile opens on the floating chart, called Now. It shows the sky at this minute, in a city proposed from your time
zone. `Ctrl+L` chooses another city. At 132 columns the Details panel and the Log sit beside the charts.

**To update**, run the install line of the newer release again. It moves your copy to that release, and the next
start builds its image. The planetary moons' files (about 240 MB) are left out unless you ask. Run `sextile --moons`
once, and every later start remembers it.

### Without Docker

```bash
sextile --no-docker
```

This runs Sextile on your machine. It needs Go and Astrolog 8.00, and `scripts/build-astrolog` expects Linux. If
either is absent, Sextile says so and names the script. It never falls back to Docker on its own. The steps are in
[Without Docker](docs/REFERENCE.md#without-docker).

## Try one question

> What is this minute's sky doing to a fixed moment?

The example is an event chart, Hamburg at noon on 24 October 2026, 12:00 CEST. The install script left a copy
under `~/.local/share/sextile/app`. Copy it into your charts folder:

```bash
cp ~/.local/share/sextile/app/examples/Demo-Hamburg-2026-10-24.as ~/.local/share/sextile/charts/
```

If you chose another charts folder, change the second path. The file is also
[on GitHub](https://raw.githubusercontent.com/cruiserxyz/sextile/v0.1.3/examples/Demo-Hamburg-2026-10-24.as).

1. Press `Ctrl+O`, type `Hamburg`, press `Enter`. "Hamburg noon" joins the Chart panel above the floating chart.
   Nothing is selected yet, and opening a chart never moves the floating chart.
2. With the cursor on "Hamburg noon", press `Space`. Press `↓` to put the cursor on the floating chart and press
   `Space` again. Both are selected.
3. With the cursor still on the floating chart, press `]`. The Aspects tab reads `② in ①` (`2 in 1` without colour):
   this minute's bodies, read in Hamburg noon's houses and zodiac.
4. Press `h` a few times. Each press moves the floating chart an hour on. Hamburg noon stays, and the aspects re-sort
   as orbs close and open.
5. Press `n`. The floating chart returns to now.

Two more things, when you want them. Press `,` then `l` then `l` to see the Astrolog commands behind what you just
saw. `Enter` copies one, `Esc` closes. To keep a stepped moment, choose its city with `Ctrl+L`. Then press `Ctrl+S`
with the cursor on the floating chart and type a title. The moment is saved as a new chart file in your charts folder.

`?` lists every key, including each popup's.

## What you can do

Each row of the Chart panel is a chart: one you opened, or the floating chart. The Chart, Bodies and Aspects panels
move together. Change something in one, and the others follow.

### Compare

Open a chart with `Ctrl+O`. Select it and the floating chart with `Space`, and the Aspects panel reads one in the
other's houses and zodiac. `[` and `]` choose which. Now is never the chart others are read against. The rules are
in [Comparison](docs/REFERENCE.md#comparison).

### Move through time

Step the floating chart by any unit from a year to a minute. `n` brings it back to now. Its Timeline tab lists the
events of the month. `Enter` on a row moves the floating chart to that moment.

### Change the view, not the file

`Ctrl+H` chooses the house system and `Ctrl+Z` the zodiac for each chart. Every chart's row ends in a four-letter
code for the pair. `WhTr` is Whole Sign houses in the tropical zodiac. All 22 house systems and 11 zodiacs are in
[Frame codes](docs/REFERENCE.md#frame-codes). `Ctrl+A` chooses the aspects shown and `Ctrl+B` the bodies. The Help
(`?`) lists the keys of every popup too.

Aspects are sorted by Astrolog's strength and marked applying or separating. Which ones matter is yours to judge.

### Keep a moment

`Ctrl+S` on the floating chart saves it, with its city, as a new Astrolog chart file. It never writes over an
existing one.

### Keep a session

A session remembers the charts you have open, their selection, their houses and zodiac, and the bodies you marked.
Sextile updates it as you work and restores it when you start again. `,` `o` saves the open charts under a
name and lists the sessions you saved. `,` `n` closes every chart and starts a new, unnamed session, after a second
press. The details are in [Sessions](docs/REFERENCE.md#sessions).

### See the calculation

`,` `l` `l` opens the log console with the Astrolog commands behind the screen. More ways to get a result out are
in [Getting a result out](docs/REFERENCE.md#getting-a-result-out).

### Draw the wheel

`Ctrl+X` draws the wheel as an image in your browser and redraws it as you move. It is served only on this machine,
at 127.0.0.1. `Ctrl+X` again closes it.

Every key, panel and setting is in the [reference](docs/REFERENCE.md).

## Practical boundaries

- **Install.** Docker is the supported route. There are no prebuilt binaries, and the native build expects Linux.
- **What it writes.** Your charts folder holds only your chart files. Sextile saves a new chart there and never over
  an existing one. Everything else stays in your home folder: the session, named sessions, backups of retitled
  charts, and your preferences. The session holds the titles, dates, times and places of your open charts, so it is
  chart data too. Sharing the charts folder shares your charts, not your sessions. The paths are in
  [What Sextile keeps on disk](docs/REFERENCE.md#what-sextile-keeps-on-disk).
- **No birth-data form.** A chart for another moment is the floating chart stepped there and saved, or an Astrolog
  chart file you copy in.
- **No interpretation.** Details shows Astrolog's own reports. Sextile adds none of its own.
- **Bodies.** A body is drawn only within the dates its ephemeris file covers, and never without the file. The
  planetary moons are off unless you run `sextile --moons`.
- **Several windows.** A second window starts with no charts and keeps nothing until you save it as a named session.
- **Keyboard only.** No mouse.

The full list: [Limits](docs/REFERENCE.md#limits).

## What comes next

Everything above is released. What follows is direction, and none of it has a date. Sextile casts nothing. Astrolog
does, and every idea below keeps that promise.

- **Richer chart reading.** The everyday reads one key away, each as a tab: aspect patterns, dignities, midpoints,
  the lunar phase.
- **Searching through time.** When is an aspect exact, last and next, and a timeline over any span.
- **Charts that belong together.** Progressions, returns and relocations linked to the chart they came from, and
  sessions you can replay.

The ideas in detail are in the [roadmap](docs/ROADMAP.md). Through 0.1.x the work goes mostly into how Sextile
feels to use.

## Feedback and contribution

Most useful is a short account of your question, and where Sextile helped or got in the way. Open it as a
[GitHub issue](https://github.com/cruiserxyz/sextile/issues). A chart that breaks something is welcome, but never a
real person's birth data. [CONTRIBUTING.md](CONTRIBUTING.md) has the commit format and the check.
[SECURITY.md](SECURITY.md) says how to report a vulnerability.

## Credits and licence

Sextile has two inspirations. [Astrolog](https://www.astrolog.org/), Walter D. Pullen's free astrology program,
computing charts since 1991, is the only thing here that computes the sky.
Jesse Duffield's [lazygit](https://github.com/jesseduffield/lazygit) showed how a terminal program makes a tool
visible, not replaced. Panels for the nouns, one key per verb, `?` for what applies here, a log of the
commands it ran. Sextile makes that move for Astrolog.

Astrolog calculates with the Swiss Ephemeris by Astrodienst AG. Its city atlas draws on GeoNames data. The interface is
built on Charm's [Bubble Tea](https://github.com/charmbracelet/bubbletea). Sextile is MIT-licensed (see [LICENSE](LICENSE)
and [CONTRIBUTORS](CONTRIBUTORS)). Astrolog, the Swiss Ephemeris and the atlas data keep their own licences.

*Sextile: the aspect of ease, when two planets sit 60° apart and work flows. Also the tool.*
