# Changelog

Each version opens with its main things. Its changes follow in five groups: New features, Extended and improved,
Program changes, Bug fixes, and For Go importers. Versions are semantic and tagged vX.Y.Z.

## [0.1.4] - 2026-11-20

The main things in this update are charts as their files state them, the Moon, and one letter per command. A chart
now shows the date, the clock and the zone of its file, to the second. Update from 0.1.3.

### New features

- A Moon tab beside the Timeline lists the Moon's aspects, sign changes and phases of the month. `p` and `P` step
  from phase to phase.
- In Bodies, the Moon shows `v/c` when the moment falls in a void of course.
- `,` `t` chooses a chart's Nodes and Lilith: the mean or true Node, and the mean, true or natural Lilith.
- A Status panel above the Chart shows your session and your charts folder. Its Details holds the credits and your
  saved sessions.
- `Enter` opens Details on every panel, in a small window too.
- In the chart list, `Space` ticks several charts, and `Enter` opens them all.
- `s` and `S` step one second.

### Extended and improved

- Every chart row shows the date, the clock to the second, the zone and the place, in one table.
- Selecting a second chart opens the comparison at once, in Aspects, Bodies and the wheel.
- Stepping in Details moves the list's cursor along. `Enter` on a Timeline or Moon row moves Now to its moment.
- `?` and `Ctrl+P` open Keybindings: every key and command, with a search. It replaces Help and Actions.
- After `,`, the key bar lists the letters of the place you are in, a popup's own included.
- The key bar at the foot is in the terminal's blue, with keys written short, such as `<enter>`.
- Each line of the Log names its group, such as Chart or Session.
- `,` `y` in the Chart panel copies the selected charts, or all of them.
- A list's cursor shows only in the panel that has the focus.
- In a filter line, the first `Enter` closes the line, and the second acts on the row.
- Deleting a session asks first. Every question takes `Enter`, `y`, `n` and `Esc` the same way.
- The Timeline also marks a void of course that runs into the next month.
- The Aspects panel shows every aspect of Astrolog's own list.
- Old dates read right: local mean time before standard time, and the Julian calendar before 1582.
- Plainer words on screen, in the Log and in `--help`. The terminal's title comes back when Sextile ends.
- The README leads to a first chart. There is a cheat sheet to print, and the reference is ordered by task.

### Program changes

- Every command has one letter after `,`, and most also work with `Ctrl`. `,` `o` opens a chart, `,` `O` the
  sessions, `,` `l` the city and `,` `L` the Log.
- `,` `u` and `Ctrl+U` choose the houses. `Ctrl+H` is gone. How houses are shown is part of the houses chooser.
- After `,`, Sextile waits for the letter as long as you like. `keys.timeoutlen` does nothing now.
- In a charts folder that Sextile did not create, each new chart asks first, until you answer Always. A change to a
  chart asks too, and a dated copy keeps the old file.
- The next start opens the session you used last.
- A new session is the first row of the sessions list, and it saves the named session you leave. `,` `n` is gone.
- The Nodes and Lilith are now the mean ones, unless you or a chart's file choose otherwise. The Node moves by up to
  about 2°, Lilith by up to about 30°. To keep the true ones, choose them with `,` `t`. Then save
  them as your default with `,` `c` `d` or from Keybindings.
- A chart from a file shows the clock that its file states, and is cast to the second. Its angles and Moon can move
  by a few minutes of arc.
- A chart file's own zodiac and house system start the chart.
- `K` and `J` move a chart. `Ctrl+K` is gone. `Ctrl+J` is gone. `N` is gone.
- `←` leaves Details.
- In the bodies chooser, the row "+ Restore the default bodies" replaces `Ctrl+D`.
- The panels are now `1` Status, `2` Chart, `3` Bodies and `4` Aspects.

### Bug fixes

- A chart file from someone else can no longer make Astrolog write a file on your computer. Such a file does not
  open, and the Log says why.
- A rename changes the title and nothing else. In 0.1.3 it could delete the notes in a chart file. The dated copies
  hold the old file.
- No copy of a chart goes to a shared temporary folder.
- Your own Astrolog settings file no longer changes what Sextile shows.
- Every chart file in your folder is in the list, also one at sea or far from a town.
- A save keeps the second and the local clock of the place.
- A chart before 1200 opens. The bodies that Astrolog cannot place that early are left out.
- A chart that cannot be cast no longer empties the panels.
- A list that you wrote into the settings file by hand survives a save.
- The Timeline names a station of the Nodes the right way round.

### For Go importers

- In `pkg/aspect`, an `AspectKind` is a row of Astrolog's aspect list, with `Index`, `Name` and `Abbrev`, and no
  longer one of eleven constants. The constants, `AspectKind.Angle`, `KindForAngle` and `AllKinds` are gone.
- `RegistryAspect` holds its kind as `Kind`. `AspectRegistry` is the one list of aspects.

## [0.1.3] - 2026-11-07

The main things in this update are every body alike and results you can trust. Fortune, an asteroid, a fixed star or
a cusp point now work the way the Sun does.

### New features

- `Space` in Bodies marks any object, such as Fortune or a fixed star. Marks limit Aspects, Details and the wheel.
- The Timeline searches every body you tick, on your chart and in transit. Ceres, Sedna and the cusps have rows.
- `Ctrl+P` opens Actions: every command with its key, in one list you can search.
- `,` `n` starts a new, unnamed session with your defaults.
- `sextile --moons` adds the planetary moons to the Docker image, about 240 MB.
- `Enter` on a Timeline row puts the Bodies cursor on its body.

### Extended and improved

- Details, Bodies and Aspects never show one chart's results under another's name. Stepping through time no longer
  flickers.
- The bodies chooser (`Ctrl+B`) shows exactly the bodies a chart is asked for, in seven groups with counts.
- On a pair tab such as `② in ①`, every aspect names the cursor chart's body first.
- The Log panel speaks only about your charts. Technical notes go to the log console.
- `,` `l` gives the keyboard to the Log panel. `,` `y` copies whatever has the keyboard.
- The key bar and the Keys popup use the same words for each key.
- An object with no short name, such as Fortune, shows its full name.
- `scripts/build-astrolog` keeps an `astrolog.as` that you changed.

### Program changes

- Going back to 0.1.2 refuses the session files of this version. The unnamed one is kept aside.
- `,` `l` focuses the Log panel. The log console is `,` `l` `l`.
- When Astrolog's list of objects cannot be read, Sextile does not start, and says why.
- The Chart report leaves fixed stars out, because Astrolog 8.00 miscounts them there.

### Bug fixes

- Starting in another charts folder no longer overwrites the session saved there.
- A body without its ephemeris file is no longer drawn at 0° Aries. The Log names the missing file.
- A chart that Astrolog cannot compute is refused, instead of showing wrong positions.
- With two or more charts, Fortune, Vertex and the fixed stars each keep their own degree and house.
- The choosers refresh Details, and Details keeps its report behind a popup.
- A held cursor key in Bodies stops when you release it.
- `Enter` in the Timeline's filter line goes to the row.
- A full key bar no longer hides a notice.
- `Space` selects nothing while only the floating chart is open.
- When Astrolog crashes, the Log says so.

### For Go importers

- In `pkg/aspect`, the `Body` enum and `ParseBody` are gone. Every reader takes an `Object` from a `Registry`.

## [0.1.2] - 2026-10-27

The main things in this update are a charts folder that holds only your charts, and named sessions.

### New features

- `,` `o` saves the open charts and their selection as a named session, and opens, renames or trashes the saved
  ones. `sextile -- --session NAME` starts in one.

### Extended and improved

- A chart saved with `Ctrl+S` opens at once.
- Every popup fits its content and keeps its size. Its notes show in one row at its foot.
- Every message is in short sentences.

### Program changes

- Your charts folder holds only your charts. Sextile's session, settings and backups moved to `~/.local/state`,
  `~/.config` and `~/.local/share`. The first start copies them over from 0.1.1.
- A session opens only in the charts folder it was made in.
- A second window starts empty.
- Opening a chart selects nothing. `Space` selects.
- `scripts/install` puts a `sextile` command into `~/.local/bin`.
- The first run proposes `~/.local/share/sextile/charts` as the charts folder.

## [0.1.1] - 2026-10-25

The main things in this update are two fixes. A saved chart reopens under its own city, and Save defaults leaves
your open charts alone.

### Program changes

- 0.1.0 was cut but never tagged. Upgrading from 0.1.0 needs nothing else.

### Bug fixes

- A chart saved with `Ctrl+S` reopens under its own city, not under a district nearby, such as Mitte for Berlin.
- Save defaults (`,` `c` `d`) no longer changes the charts you have open.

## [0.1.0] - 2026-10-25

The main things in this first release are your chart files in the terminal, against the sky of any moment. Astrolog
8.00 computes every position, house and aspect, with the Swiss Ephemeris.

### New features

- `./sextile` builds a Docker image and starts Sextile. `./sextile doctor` says what is wrong. `scripts/install`
  fetches Sextile with one command.
- The image builds Astrolog 8.00 and the Swiss Ephemeris files from pinned and checked sources.
- Your charts are Astrolog chart files in a folder you own. `Ctrl+O` lists and filters them, and `Ctrl+R` renames
  one.
- `Ctrl+S` saves the floating chart as a new file. It never replaces a chart.
- The floating chart follows the clock. The step keys move it by a year down to a minute, and `n` returns to now.
- Bodies show sign, degree, house, speed and stations. Aspects are ranked by strength. `Ctrl+A` and `Ctrl+B` choose
  them.
- `Ctrl+H` chooses the house system and `Ctrl+Z` the zodiac, without touching the file.
- With two or more charts selected, the Aspects panel reads across them.
- Details shows the report on the body under the cursor, and a Timeline of the month's events.
- `Ctrl+X` draws the wheel in your browser, and it follows time.
- `Ctrl+L` picks a city. `,` `c` `d` keeps it as your default.
- Your charts and choices come back when you reopen.
- The key bar shows the keys that matter where you are. `?` opens Help.
- Sextile takes `--charts-dir`, `--accessible`, `--color` and `--version`.
- The log console lists the Astrolog commands behind the screen.

### Program changes

- There is no form for birth data. A chart for another moment is saved from the floating chart.
- Some bodies are not drawn: those outside their ephemeris range, the planetary moons and the planets' centres of
  body.
- `.sextile/` in the charts folder keeps the names and birth data of the charts last open. Sharing the folder shares
  them.
- One chart needs a terminal of at least 66x15.
- Names and places longer than 54 bytes are cut.
- Docker is the install. Linux and Windows through WSL are tested, macOS is not.
