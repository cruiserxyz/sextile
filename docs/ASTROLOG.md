# For Astrolog users

This page is for you if you know [Astrolog](https://www.astrolog.org/): its switches, its chart files, its
`astrolog.as`. It says in Astrolog's terms what the [reference](REFERENCE.md) says in Sextile's own words.
You can use Sextile without knowing Astrolog.

- [What Sextile is beside Astrolog](#what-sextile-is-beside-astrolog)
- [Your chart files](#your-chart-files)
- [What Sextile writes](#what-sextile-writes)
- [Your settings file](#your-settings-file)
- [The command behind every number](#the-command-behind-every-number)
- [Where Sextile differs on purpose](#where-sextile-differs-on-purpose)
- [The Astrolog that Sextile runs](#the-astrolog-that-sextile-runs)

## What Sextile is beside Astrolog

Sextile is a screen for Astrolog 8.00. It runs Astrolog for every number and calculates nothing itself.

- Your chart files stay Astrolog's files. Astrolog opens every chart that Sextile saves.
- Sextile does not replace Astrolog. It shows a small part of it: positions, houses, aspects, transits, the Moon
  and the wheel.
- You can trace each result to one Astrolog command, which you can copy and run.
- [How the numbers are checked](REFERENCE.md#how-the-numbers-are-checked) says how each release is compared
  with Astrolog's own output.

## Your chart files

Copy your chart files into the charts folder. Sextile knows a chart by its first characters, `@AI`. The name of
the file does not matter.

| In the file | What Sextile does |
| --- | --- |
| `-qb`, `-qa`, `-qj` and the other forms of the moment | the chart's moment and place, as Astrolog reads and states them |
| `-zi` | the title and the name of the place |
| `-c` and the zodiac lines (`-s` and its offset) | the houses and zodiac the chart starts with. The choosers in Sextile change them for the session |
| a line that only changes how Astrolog prints | left out, without a word |
| `-Yn`, `-Ynn`, the form of the Nodes and of Lilith | the chart starts in that form. Sextile reads the line and does not give it to Astrolog |
| a line that chooses bodies, aspects or orbs | left out, and the Log says so once |
| a line that asks for another centre or another kind of chart, such as `-h` or `-x` | the file does not open yet |
| a line that mixes the moment, place, title, houses or zodiac with any other switch | the file does not open. Astrolog's own writer never makes such a line |
| any other switch, such as one that writes a file | the file does not open |
| a title or a place longer than 54 bytes | the file does not open. Astrolog cannot read it |
| a comment line, which starts with `;` | kept in the file, and not shown |

- A file that does not open carries a red `!` in the list of charts. The list says why.
- Each chart must be a file of its own. Sextile does not read a list file with several charts.
- A settings file in the charts folder, such as `astrolog.as`, is not a chart. Sextile leaves it alone.

**Why Sextile is strict.** A chart file is a list of switches. A switch can do more than describe a chart.
A file from someone else could make Astrolog write a file on your computer. So Sextile gives Astrolog only the
lines that describe the chart.

This is the eclipse chart of the README's picture. It has no title, so Sextile shows the name of the file.

```
@AI800  ; Astrolog chart info.
; The solar eclipse: Moon at the Sun, 17:37 UTC per Astrolog 8.00.
-qb Aug 12 2026 19:37 DT 1E   8:43E 52:51N
-zi "" "Bassum, Germany"
```

## What Sextile writes

- Astrolog's own writer writes a saved chart. Sextile adds no line of its own.
- The clock is the place's own clock at that date. Before standard time it is local mean time.
- Astrolog writes a zone to the whole minute. A legal zone with seconds is rounded in the file, and the moment
  stays exact.
- A rename changes the `-zi` line only. Sextile keeps a dated copy of the file first.
- The houses, zodiac, bodies and aspects that you choose in Sextile go into Sextile's session file, not into a
  chart file.

## Your settings file

Astrolog reads `astrolog.as` each time it starts. You may have one of your own. With Docker, Sextile uses
Astrolog's own settings file from the image. Yours counts only when you run
[without Docker](REFERENCE.md#without-docker).

Before every call, Sextile sets Astrolog's settings back to Astrolog's own values, one by one. It checks them
against Astrolog's own list. It sets the moment, the place, the zone, the houses and the zodiac. It sets the form
of dates and of the clock from its own settings. It also sets how Astrolog prints, searches and draws the wheel.
So the positions, the houses and the moment of a chart never depend on a settings file.

On purpose, your settings file still decides these today.

| What your settings file still decides | What it changes |
| --- | --- |
| the aspects and their orbs, until you choose aspects in Sextile | which aspects a chart shows |
| the bodies that show at first | which rows Bodies has, not where a planet stands |
| the colours | the wheel's picture, and Astrolog's colours on screen |
| the weights of the influence report and of the rays | the numbers of that report in Details |
| your folders for ephemeris files and fonts | where Astrolog finds its files |

The first row matters most. Two computers with different settings files can list different aspects for the same
chart. Choose your aspects in Sextile, and that ends.

The [roadmap](ROADMAP.md) plans a way to bring in your own settings, where they are a matter of taste. Until then,
set houses, zodiac, planets and aspects inside Sextile. [Keep your defaults](REFERENCE.md#keep-your-defaults) shows
how. If a setting of yours matters to you, [say so](https://github.com/cruiserxyz/sextile/issues).

## The command behind every number

The log console lists each Astrolog command that Sextile ran. `Enter` on a line copies the whole command.
Run it with Astrolog 8.00, and the same numbers come back.

- The command for one chart holds the chart itself as switches. It needs no file.
- The command is long. It names every setting that could change a result, except those in the table above.
- [The calculation and the Log](REFERENCE.md#the-calculation-and-the-log) shows the keys.

## Where Sextile differs on purpose

| What | Sextile | In Astrolog's terms |
| --- | --- | --- |
| The Nodes and Lilith | your choice for each chart, mean and mean at first. Sextile names the form on every call | `_Yn` / `=Yn` and `_Ynn` / `=Ynn`. The true Node with the mean Lilith has no switch, and Sextile cannot show it yet |
| A Node or Lilith of your own | it moves with the form, and it carries no letter | an object that your settings file defines with `-YeO` |
| The aspects | the first eleven kinds | Astrolog has 24. One of the others that you switch on does not show |
| The Chart report in Details | leaves out the fixed stars | Astrolog 8.00 counts them wrongly in that one report |
| The zone on a chart's row | the clock's real offset, such as `+2 DT` | Astrolog's own report writes the zone, `DT Zone 1E` |
| A daylight shift that is not one hour | shown as `AT` | Astrolog prints no amount for it |
| The ephemeris | always the Swiss Ephemeris files | a body with no file for a date is left out, never placed at 0° Aries |

A few of Astrolog 8.00's printouts are not right in rare cases. Sextile works around each one, and we keep the list
for Astrolog's author. The [Limits](REFERENCE.md#limits) name those that you can meet.

## The Astrolog that Sextile runs

- Sextile needs Astrolog 8.00. Docker brings it, built from Astrolog's source archive. Every file is checked
  against a checksum.
- `sextile --version` prints the version of Sextile and of Astrolog.
- Without Docker, you install Astrolog 8.00 yourself. [Without Docker](REFERENCE.md#without-docker) shows how.
  The script there keeps an `astrolog.as` that you changed.
- Astrolog uses the Swiss Ephemeris by Astrodienst AG. Both keep their own licences.
