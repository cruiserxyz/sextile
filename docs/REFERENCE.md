# Sextile reference

The [README](../README.md) gets you to a first session. This page holds the rest: what each panel shows, every
key, the install routes and their options, where Sextile keeps what, and its limits. It describes Sextile 0.1.2.

- [The screen](#the-screen)
- [Keys](#keys)
- [Charts](#charts)
- [Sessions](#sessions)
- [The example chart](#the-example-chart)
- [The wheel](#the-wheel)
- [Getting a result out](#getting-a-result-out)
- [Install options](#install-options)
- [What Sextile keeps on disk](#what-sextile-keeps-on-disk)
- [Limits](#limits)

## The screen

Every astrological value on screen comes from Astrolog 8.00 — Sextile runs it, arranges what it returns and adds no
interpretation of its own.

- **Holds two moments at once.** One chart holds a moment you opened. The floating chart holds now, or any moment
  you step to. Select two charts and the Aspects panel reads across them, saying whose body is whose.
- **Shows the chart as Astrolog casts it.** Bodies with sign, degree, house, speed and station marks. The house
  system the chart file asks for, and the tropical zodiac unless you choose another, with a chooser to try another
  without touching the file. A file that names no house system opens in Whole Sign houses. The Chart row ends in
  the pair it uses (`WhTr`: Whole Sign, tropical).
- **Sorts the aspects.** By Astrolog's strength value, marked applying or separating, filtered by the aspects and
  bodies you choose. Which of them deserve attention is yours to judge.
- **Shows only what you select.** `Space` on a body in Bodies selects it, and that chart's wheel, Aspects and report then show only the selected bodies — the Bodies table keeps every body so you can add to the selection, a body the `Ctrl+B` chooser hides stays hidden, and in a comparison the selection does not yet reach the cross surfaces.
- **Moves time.** A year, a month, a week, a day, an hour or a minute, forwards or back, as fast as you hold the key.
  The step keys always move the floating chart, wherever the cursor is. An opened chart keeps its moment.
- **Follows the clock.** The floating chart starts on the current minute and follows the clock. Its row reads
  `~ Now`. A step key, `N` or Enter on a timeline row stops it where it is (the row reads `~`) — `n` returns to now
  and follows again, `N` goes back to where you were.
- **Answers "what does Astrolog say?"** Details follows the cursor with Astrolog's report for the body under it.
  On a chart it opens on Astrolog's influence chart, a table of each body's positional and aspect strength. A
  timeline tab lists the events of the floating moment's calendar month. The Timeline tab exists for the floating
  chart and for a selected group that contains it: alone, the floating month's sky. In a group, the sky plus each
  selected chart's transits and returns. The bold row is the floating chart's moment, whichever chart the cursor
  is on.
- **Keeps a chart library.** Charts are Astrolog's own chart files (`name.as`) in a folder you own — `Ctrl+O` opens a
  type-to-filter picker over it (each row also shows the chart's age), `Ctrl+S` saves the floating chart into it.
- **Draws the wheel.** `Ctrl+X` has Astrolog draw the wheel. How it reaches you depends on how Sextile was started
  (see [The wheel](#the-wheel)).
- **Picks up where you left off.** Quit and reopen: your charts, your selection and your house and zodiac choices
  come back. The floating chart is freshly now.
- **Shows its working.** The log console (`,` then `l`) lists the Astrolog commands behind the screen, and you can
  copy one out. Each line leaves out the opening every call shares and starts at the switches that name the chart.
  One too wide for the console ends in `…`. Enter copies the whole command. While the chart follows the clock its
  minute updates write nothing to the Log except one `now:` line, always the console's last, which updates with the
  latest command each minute while the console is open, and `,` `l` opens on it. So the console holds the command
  behind the current screen and every step you took, not a line for every minute.

### Comparison

Opening a chart selects nothing. `Space` selects the slot under the cursor. Two selected charts engage comparison.

Comparison reads `X in Y`: the chart under the cursor (X) is the one you read, and the Aspects tab, with `[` and `]`, picks the chart it is read against (Y), whose houses and zodiac frame the comparison — now is never Y. Bodies keeps listing X under the title `X in Y`, each body's house in Y's houses bold beside Y's own house for that body faint.

The tab is labelled with the charts' numbers, like `② in ①` (`2 in 1` without colour). With the cursor on an opened
chart and only the floating chart beside it, there is no other chart to read it against, so its own tab shows.

### Frame codes

Every chart's row ends in a four-letter tag: the house code, then the zodiac code (`WhTr` is Whole Sign houses, tropical zodiac).
A sidereal chart always names its ayanamsa, so no code stands for sidereal alone. These are all of them, in Astrolog's own order.

| House code | House system |
| --- | --- |
| `Pl` | Placidus |
| `Ko` | Koch |
| `Eq` | Equal |
| `Ca` | Campanus |
| `Me` | Meridian |
| `Re` | Regiomontanus |
| `Po` | Porphyry |
| `Mo` | Morinus |
| `To` | Topocentric |
| `Al` | Alcabitius |
| `Ks` | Krusinski |
| `EM` | Equal (MC) |
| `PR` | Pullen (S.Ratio) |
| `PD` | Pullen (S.Delta) |
| `Wh` | Whole Sign |
| `Ve` | Vedic |
| `Sr` | Sripati |
| `Hz` | Horizon |
| `AP` | APC |
| `CP` | Carter P.Equat. |
| `Su` | Sunshine |
| `Sa` | Savard-A |

| Zodiac code | Zodiac |
| --- | --- |
| `Tr` | Tropical |
| `Fa` | Fagan-Bradley |
| `La` | Lahiri |
| `De` | Deluce |
| `Ra` | Raman |
| `Us` | Usha-Shasi |
| `Kr` | Krishnamurti |
| `Dj` | Djwhal Khul |
| `Yu` | Yukteshwar |
| `Bh` | J.N. Bhasin |
| `Gc` | Galactic Center |

### House display

How the house column of the Bodies panel is drawn. `,` `a` opens the House display popup on the form in force — `Space` chooses the form under the cursor and puts it on the panel at once, and `Enter`, `Esc` or `q` close the popup keeping it. The choice is the session's, for every chart. `,` `c` `d` keeps it with your default city, as the `house_display:` line of `preferences.yaml` (`house_display: angularity`, `quadrants`, `number` or `marked`). A word it does not know is refused in the Log and angularity stands.

- **Angularity at a glance** (`angularity`, the default). Houses grouped around the four angles: Ascendant, IC, Descendant, Midheaven. The frame is `12 1 2 | 3 4 5 | 6 7 8 | 9 10 11`. In each bracket the middle cell is angular, the right succedent, the left cadent.
- **True quadrants** (`quadrants`). Houses in the four quadrants, counted from the Ascendant: `1 2 3 | 4 5 6 | 7 8 9 | 10 11 12`.
- **Plain number** (`number`). The house number alone.
- **Number + angularity** (`marked`). The house number with `A` for angular, `s` for succedent, `c` for cadent: `1A`, `2s`, `3c`, `4A`, and so on.

All four fill the same 18 cells, so the Bodies panel never shifts when the form changes. In a comparison a row carries two houses in every form: the chart's own house faint, the house in the compared chart bold (with colour off, position or order carries the difference).

## Keys

Press `?` inside Sextile for the full list.

| Key | What |
|---|---|
| `y/Y` `o/O` `w/W` `d/D` `h/H` `m/M` | Step a year, month, week, day, hour, minute forward / back |
| `n` / `N` | Now (the floating chart follows the clock until you move it) / back to the previous time |
| `Ctrl+O` / `Ctrl+S` | Open a chart from the library / save the floating chart (focus it first) |
| `Ctrl+L` | Choose the city |
| `Ctrl+H` / `Ctrl+Z` | Choose the house system / the zodiac of the focused chart. Every other chart keeps its own, and a comparison is read in the houses and zodiac of the chart the Aspects tab reads it against. The popup's title names the focused chart |
| `Ctrl+A` / `Ctrl+B` | Choose which aspects / which bodies are shown — `Space` ticks at once and the screen updates live, `Esc` or `Enter` closes keeping what is ticked |
| `Space` | Select the focused chart or body. Two selected charts engage comparison. Selected bodies show only themselves on the chart's wheel, aspects and report (nothing selected shows all) |
| `Tab` `1` `2` `3` | Move between the Chart, Bodies and Aspects panels |
| `Enter` `[` `]` `0` | Into Details, and between its tabs. From the Chart or Aspects panel `[` `]` pick the chart the cursor's chart is read against |
| `Enter` `/` | In Details: on a timeline row, move the floating chart to that moment without selecting it (`n` follows the clock again). Filter the rows |
| `Ctrl+X` | Render the wheel. With Docker, again closes it |
| `,` `l` | The log console (from the main screen): the Astrolog commands — `Enter` copies the highlighted line |
| `,` `a` | How the Bodies house column is drawn ([House display](#house-display)): angularity at a glance (the default), true quadrants, plain number, or number + angularity — `,` `c` `d` keeps it with the city, along with the floating chart's house system and zodiac |
| `,` `o` | The Sessions list ([Sessions](#sessions)): save the open charts under a name, or open, rename (`Ctrl+R`) or move to the trash (`Ctrl+D`) a saved session of this charts folder |
| `,` `y` | In Details: copy the timeline rows (marked, else all) or the whole report |
| `,` `h` / `,` `z` / `,` `s` | The same as `Ctrl+H` / `Ctrl+Z` / `Ctrl+S`, for a terminal that keeps those keys for itself |
| `q` | Quit from the main screen. Closes Help, the log console, Details and the house display popup. Inside a search or title field it is a letter. `Ctrl+C` quits from anywhere |

⇞ ⇟ Home End (and g G outside a filter) move the cursor of the focused list: first to the edge of its window, then by a page of the window minus one row. This holds in the Chart and Bodies panels, the timeline, the Log console and every chooser. ⌃⇞ ⌃⇟ scroll the Details report from anywhere, two lines a press. If your leader key is g, the leader wins — Home and End always work.

In Details, `/` filters the timeline rows or the report: type to match anywhere in a row, ignoring case. Start with a second `/` for a loose match (the letters in order), or a third for an exact match that respects case.

Options: `--charts-dir DIR` (the chart library, also `SEXTILE_CHARTS_DIR`), `--accessible` (ASCII borders and markers),
`--color auto|always|never` (also `SEXTILE_COLOR` and `NO_COLOR`), `--session NAME` (start in that named session,
through the launcher, `sextile -- --session NAME`), `--version`.

## Charts

### Where charts live

A chart is one Astrolog chart-info file in the charts folder:

- With Docker or `sextile --no-docker`, that is the charts folder `sextile` asked for (the first suggestion is `~/.local/share/sextile/charts`, the folder `bin/sextile` uses by default, on every system and whether or not it exists yet, the second is `Sextile` in your documents folder, `~/Documents/Sextile` unless `XDG_DOCUMENTS_DIR` names another, `XDG_DATA_HOME` moves the first) — Docker mounts it at `/charts`.
- Started by hand as `bin/sextile`, it is the folder from `--charts-dir DIR` or `SEXTILE_CHARTS_DIR`. With neither,
  `$XDG_DATA_HOME/sextile/charts`, which is `~/.local/share/sextile/charts` when `XDG_DATA_HOME` is unset.
- Copy the file in, press `Ctrl+O`, type to filter, `Enter` to open.

### Chart files

Sextile saves a chart as `name.as`, the file Astrolog itself writes and reads, and lists every Astrolog chart it finds in
the folder, whatever the file is called: it recognises a chart by its first bytes (`@AI`), never by its name. Astrolog's
settings files (`astrolog.as` and its kin) and any other file are left alone. Astrolog's multi-chart list files are
not read. Each chart must be a file of its own.

A file with no title asks for one when you open it: type one, press `Enter`, read which file it will update and the
backup it keeps, and press `Enter` again. When you retitle a chart, the previous file is kept once, outside the
charts folder, in its backup folder under `~/.local/share/sextile/backup/` (see
[What Sextile keeps on disk](#what-sextile-keeps-on-disk)). The confirm step names it in full.

The file is a few lines of text. The [example chart](../examples/Demo-Hamburg-2026-10-24.as) reads:

```
@AI800  ; Astrolog chart info.
-qb Oct 24 2026 12:00pm ST 2E  10:00E 53:33N
-zi "Hamburg noon" "Hamburg, DE"
```

The `-qb` line is the moment: month, day, year, time, `ST` and the offset in effect (two hours east of Greenwich,
summer time in Hamburg on that date), longitude, latitude. `-zi` is the title and the place. Astrolog reads the file
only with its first line as shown. A `-c` line, such as `-c Placidus`, would name a house system. This file has none.

### Saving a chart

`Ctrl+S` (or `,` `s`) saves the floating chart, its moment and its city, as a new chart file under the title you
type. There is no birth-data form and no field for a date: reach the moment with the step keys or Enter on a
timeline row, choose the city with `Ctrl+L`, focus the floating chart, press `Ctrl+S`, and type a title. The file is
named after the title, its letters and digits in lower case and anything else becoming `-`, and a save never
replaces an existing chart. The new chart opens right away with the cursor on it, not selected.
Astrolog writes the file itself, so its clock follows Astrolog's own settings. The stock settings write `12:00pm`.
The example chart was saved this way.

## Sessions

A window always runs one session and keeps it up to date as you work: the general session every window starts
from, or a named one. `,` `o` opens the Sessions list. Its first row, **+ Save this session as…**, saves the open
charts, their selection and their house and zodiac choices under a name: `Enter` opens the name field, filled in
with the title of the first selected chart (else the first chart), and `Enter` saves. A name already used is
refused in the field and nothing is written. From then on the window keeps that named session, and the general
session stays as it was at that moment.

Below it are the sessions saved in this charts folder, newest first, each with when it was last updated. `Enter`
on one saves the window's current session as usual, then opens the chosen one: its charts, selection and frames
replace the open ones, and the window keeps it from then on. `Ctrl+R` renames a session (a name already used is
refused) — `Ctrl+D` moves it to the trash at once, and the Log names where it went. A session open in another window
says so and cannot be opened, renamed or deleted. The session this window runs cannot be deleted (switch to another
first). Each session's file name is made from its name the way a chart's is. To start straight into one, launch
with `--session NAME` (`sextile -- --session NAME`). A session that is missing, open elsewhere or from another
charts folder is not opened, and the Log says why.

## The example chart

[`examples/Demo-Hamburg-2026-10-24.as`](../examples/Demo-Hamburg-2026-10-24.as) is the moment every example here
starts from. It is an event chart: no person and no birth data.

| | |
|---|---|
| Title and place | Hamburg noon — Hamburg, DE |
| Local time | Saturday 24 October 2026, 12:00 CEST (Europe/Berlin, UTC+02:00) |
| Universal time | 2026-10-24 10:00 UTC |
| Coordinates | 10:00E 53:33N, the atlas entry for Hamburg (9.993E 53.5507N) to the minute |
| House system and zodiac | none in the file: Whole Sign and tropical on a fresh install |
| Made with | Sextile 0.1.0 and Astrolog 8.00: the floating chart stepped to 12:00, `Ctrl+L` Hamburg, `Ctrl+S` |

Astrolog reports it as `Sat Oct 24, 2026 12:00pm (ST Zone 2E) Hamburg, DE  10:00E 53:33N` — Sextile's header for it
reads `Sat Oct 24, 2026 12:00 (DT Zone 1E)`, the same instant written as standard time plus daylight saving.

To show the screen the README describes:

1. Start Sextile with a terminal at least 144 columns by 40 rows, a charts folder holding only the example, and no
   `preferences.yaml`.
2. `Ctrl+O`, type `Hamburg`, `Enter`. Hamburg noon joins the Chart panel with the cursor on it and nothing selected.
   With the cursor on Hamburg noon press `Space`, then `↓` and `Space` on the floating chart: Hamburg noon is chart 1,
   the floating chart (now) chart 2.
3. With the cursor on Hamburg noon, Bodies, Aspects and Details show the example moment. `↓` then `]` shows
   `② in ①`, now read against it. The floating chart's row and every value that depends on it show the moment you
   pressed the keys, so a capture of them is a capture of that moment, not of the example.

[`screenshot.svg`](screenshot.svg), the README's picture, is a screen made reproducible: the solar eclipse
fixture opened as chart 1, the clock frozen at Sunday 25 October 2026 09:00 in Hamburg, Details on the Timeline tab and
Enter on Venus's return to Libra, which moves the floating chart to 10:09, rendered by the model itself at 144x40 on
real Astrolog. A test converts the frame to SVG. Regenerate it, never edit it, with:

```bash
go test ./internal/ui -run '^TestReadmeScreenshotScene$' -demo
```

## The wheel

`Ctrl+X` has Astrolog draw the wheel. What happens next depends on how Sextile was started:

- **With Docker or `sextile --no-docker`** (the launcher or the commands under [By hand](#by-hand)), Sextile serves the wheel to a browser on
  this machine and redraws it as you move time. The Log names the address (`Wheel at http://localhost:5307 (Ctrl+X again closes it)`, unless you chose another port), `sextile` prints it at launch, and your browser opens on the first Ctrl+X (`wslview` on WSL, `xdg-open` on Linux, with neither, as on a Mac without them, the printed address is all). Only this machine can reach it.
- A second Ctrl+X closes the wheel (a red Log line says `Wheel closed; Ctrl+X opens it again`), and a third opens it again at the same address. The browser does not open a second time: the tab you left open reconnects by itself, or reload it.
- **Started by hand as `bin/sextile`**, Ctrl+X hands the wheel image to `xdg-open` in a new `$TERMINAL` window, and a second Ctrl+X
  refreshes it. Start Sextile as `SEXTILE_PREVIEW_MODE=http bin/sextile …` to have the wheel served at the address
  the Log names instead, as with Docker.
- The served wheel is an SVG. To host a PNG instead, put `wheel_format: png` (or `svg`) on a line of its own in `preferences.yaml` (its place is under [What Sextile keeps on disk](#what-sextile-keeps-on-disk)). It takes effect the next time you open the wheel, and any other word is refused in the Log.

## Getting a result out

- **Text.** In Details, `,` then `y` copies to the clipboard: on the Timeline tab the marked rows (all rows if none are
  marked. Mark with `Space`), on the report tab the whole report. In the log console, `Enter` copies a command,
  whole, however short its line is shown.
- **Wheel.** `Ctrl+X`, as described under [The wheel](#the-wheel).
- **Clipboard.** Copying sends the terminal's clipboard escape sequence (OSC 52). Sextile shows `Copied` once it has sent the text and cannot tell whether your terminal accepted it. If nothing pastes, read the command in the log console (`,` `l`).

## Install options

The README's [Install and run](../README.md#install-and-run) fetches Sextile with `scripts/install`. It clones into `~/.local/share/sextile/app` (`$XDG_DATA_HOME/sextile/app` when you set `XDG_DATA_HOME`, set `SEXTILE_DIR` to choose another folder). An existing Sextile clone there is moved to the release tag, so files committed after it leave the working tree and stay in its history, and uncommitted changes stop the checkout. Any other existing folder is refused. It then writes a `sextile` command into `~/.local/bin` that runs the clone's launcher: three lines, the second of which marks it as the script's own, so a file already there that the script did not write is never replaced and the install stops before it changes anything. When `~/.local/bin` is not on your PATH (the default on macOS) it prints the line to add to `~/.zshrc`, `~/.bashrc` or `~/.profile`, whichever fits your shell, and says that `~/.local/bin/sextile` works right away. It edits no startup file. A clone that an earlier install left in `~/sextile` is no longer used. The script says so and leaves it as it is, for you to remove by hand. Docker is needed to run Sextile, not to install it: without a running Docker the script warns and goes on. The details:

- You need Docker 28 or newer, running on this machine.
- The first run asks which folder holds your charts (Astrolog chart files), suggesting `~/.local/share/sextile/charts` first and `~/Documents/Sextile` second, remembers the answer in `~/.config/sextile/`, builds the image, and starts Sextile. The first build downloads and compiles Astrolog 8.00 from its checksum-pinned source archive, which carries the Swiss Ephemeris files for 1800–2400 and the files for Hygiea and seven dwarf planets (Eris, Haumea, Makemake, Gonggong, Quaoar, Sedna, Orcus), which cover about 1500 to 2100, and adds the three 1200–1800 files from the aloistr/swisseph GitHub mirror at a pinned commit, every file checked by checksum. It takes a few minutes, and later starts skip it.
- To bring existing Astrolog chart files, copy them into your charts folder, then press Ctrl+O.
- `sextile --charts-dir DIR` uses another charts folder for one run, `sextile --port 5308` another port, `sextile --no-open` leaves the browser closed, and `sextile --rebuild` builds the image again. Anything after `--` goes to Sextile itself. (Run the launcher from a clone you made by hand as `./sextile`.)
- After a launch, `sextile` removes older `localhost/sextile` images that it built itself and that no container uses. Other images are never touched.
- `sextile doctor` says what is wrong (Docker, the port, the charts folder, and for `sextile --no-docker` go and astrolog) without changing anything.
- Your time zone is passed in so the first run can propose your city. A proposed city is named in the Log. Choose another with Ctrl+L or keep it as your default with `, c d`, which keeps your house display (`, a`) with it, and the floating chart's house system and zodiac as the frame a chart that names none opens in (charts already open keep the frame they show, and a second window refuses `, c d`). A city chosen with Ctrl+L lasts for the session only. Chart times always use each chart's own place.
- Times and dates follow a `clock:` block in `preferences.yaml` (where it lives is under [The wheel](#the-wheel)): `hours: 24` (default) or `12`, and `dates: month-first` (default) or `day-first`, each on its own indented line. It is read when Sextile starts.

### By hand

The launcher runs these two commands for you. To run them yourself:

```bash
git clone https://github.com/cruiserxyz/sextile
cd sextile
docker build -t sextile --build-arg SEXTILE_VERSION="$(git describe --tags --always)" .
```

Then run it:

```bash
mkdir -p ~/.charts
docker run -it --rm --user "$(id -u):$(id -g)" -v ~/.charts:/charts --tmpfs /tmp -p 127.0.0.1:5307:5307 sextile
```

- Your charts are read from and saved to `~/.charts`. This run mounts nothing else, so the session, the preferences and the backups are kept in `~/.charts/.sextile`, where `sextile` itself no longer keeps them.
- If port 5307 is taken, change the left-hand number in `-p`.
- Add `-e TZ=<zone>` (for example `-e TZ=Europe/Amsterdam`) to the `docker run` command so the first run proposes your city.

### Without Docker

```bash
git clone https://github.com/cruiserxyz/sextile
cd sextile
scripts/build-astrolog          # Astrolog 8.00 into ~/.local, from the pinned archive
mkdir -p bin
go build -o bin/sextile ./cmd/sextile
bin/sextile --charts-dir ~/.charts
```

Or let the launcher do it: `./sextile --no-docker` (go and astrolog on the host, it builds into `~/.cache/sextile/native/`, removing the older builds there after a new one, and keeps session, preferences and backups in the same XDG folders as Docker, outside the charts folder).

Requires Go 1.25 or newer and a C compiler for Astrolog. `scripts/build-astrolog` expects a Linux system (GNU `sed`, `sha256sum`, `nproc`). On macOS, use Docker. Put `~/.local/bin` on your PATH so Sextile finds the Astrolog that `scripts/build-astrolog` installed there. `bin/sextile --version` prints both versions and where it found Astrolog. The wheel behaves as described under [The wheel](#the-wheel).

## What Sextile keeps on disk

The charts folder holds only your charts. Everything else Sextile keeps lives in your home folder. The session and
backup folders are private to you (`0700`):

- **The session.** `~/.local/state/sextile/session/` (`$XDG_STATE_HOME/sextile/session/` when `XDG_STATE_HOME` is set)
  holds the last-opened charts (titles, dates, times and places) so the session can be restored, and the lock that
  makes a second window a second window. There is one session for all your charts folders, and it records which
  charts folder it was saved in: started on another folder, Sextile opens with no charts and says on the Log that the
  saved session belongs to another charts folder and was not opened, so a chart in one folder is never mistaken for a
  same-named chart in another. Quitting there then keeps that folder's session instead.
- **Named sessions.** `~/.local/share/sextile/sessions/` (`$XDG_DATA_HOME/sextile/sessions/`), one `<name>.yaml`
  per saved session (mode 0600, the folder 0700): the same content as the session, plus its name and the charts
  folder it belongs to, and a `<name>.lock` while a window runs it. `Ctrl+D` moves a session to
  `sessions/.trash/<name>-<UTC time>.yaml`. Nothing there is ever deleted by Sextile.
- **Backups.** A retitled chart's previous file, in `~/.local/share/sextile/backup/<folder code>/`
  (`$XDG_DATA_HOME/sextile/backup/<folder code>/`), where the folder code is a long code made from the charts folder's
  full path, so two folders holding a chart of the same name keep two backups. The confirm step and the Log name the
  full path.
- **With Docker.** `sextile` mounts the session, preferences, backup and named-sessions folders into the container
  beside the charts folder. It makes the ones that are missing private (mode 0700) and refuses to start when one of
  them, or `$XDG_DATA_HOME/sextile`, is a symlink or a file. A relative `XDG_*` value is ignored, and `sextile doctor`
  names the four folders. `sextile --no-docker` leaves your `XDG_*` variables as they are. A hand-written
  `docker run -v ~/charts:/charts` mounts only the charts folder, so there the session, the preferences, the
  backups and the named sessions stay in its `.sextile` folder, as before.
- **Charts you save.** New `name.as` files in the charts folder. Nothing is overwritten.
- **Preferences.** `~/.config/sextile/preferences.yaml` (`$XDG_CONFIG_HOME/sextile/preferences.yaml`): the default city, house display and frame that `, c d` keeps for the floating chart on a fresh start and for charts opened afterwards (the frame is a `frame:` block
  with `houses:`, Astrolog's name for the house system, and `zodiac:`, `tropical` or `sidereal <name>`), the
  `clock:` block and `wheel_format`.
- **The launcher's memory.** `~/.config/sextile/launcher.conf` remembers which charts folder `sextile` uses.
- **Moving from 0.1.1.** Sextile 0.1.1 kept the session, the preferences and the backups in `<charts folder>/.sextile/`
  (and a hand-started one kept its session in `~/.local/share/sextile/session/`). Each launch on that folder
  copies them to the places above, but only where nothing is there yet: an existing session or preferences file, or
  a backup already copied, is never replaced. When a launch copies something, the Log says in one line what was
  copied and what was already there. A launch that finds everything in place says nothing. The old `.sextile`
  folder is left exactly as it was, for you to remove when you are satisfied. With several 0.1.1 charts
  folders, the first one opened brings its session along. The others' sessions stay in their `.sextile` folders.

## Limits

- Several windows at once: the first runs the general session. A later one opens with no charts, says once that it
  keeps nothing until it is saved as a named session, and leaves the general session alone. Once saved with `,` `o`,
  or opened from there, its session is kept like any other. A named session runs in one window at a time. Two
  windows cannot save a chart under the same name: the second is told the name is taken. With `sextile` and no `--port`, a later window
  takes the next free port and prints which. A `--port` that is taken is refused.
- The terminal must be at least 66 columns by 15 rows with one chart, one more row for each further chart. Below
  that Sextile shows "Terminal too small" and the size it needs. Resize and it carries on.
- Details and the Log sit beside the charts when the terminal is at least 132 columns wide plus 12 per extra chart.
  Narrower, the charts take one column without them, and Sextile says once which size would show them. When the
  Bodies columns do not all fit, the focused chart's column stays whole and `‹N` / `N›` count the columns hidden
  to each side.
- It is a terminal application: no mouse, no windows, no phone.
- It needs Astrolog 8.00. The Docker image builds it for you. Otherwise you install it.
- There are no prebuilt binaries: Docker is the install, on Linux or macOS, and on Windows from a WSL terminal. Linux and Windows through WSL are tested — macOS is expected to work but untested.
- It draws no planetary moons or planets' centres of body (their ephemeris files are not included), and none of the asteroid bodies outside the dates their files cover. The Log names a body it leaves out.
- Names and places longer than 54 bytes are cut, ending in `…`. Accented letters count as two. A chart file
  written elsewhere with a longer place may fail to open.
