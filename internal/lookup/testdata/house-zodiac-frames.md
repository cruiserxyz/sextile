# House and zodiac finder — approved frames (operator, 2026-10-08)

Mock frames in the style of the real ⌃a chooser (captured 2026-10-08, same
box, search line, footer counter, key bar). Chart names are made up.
Specs: specs/house-finder.md (V1), specs/house-chooser.md (V2), specs/zodiac-chooser.md (V1).

## F1 — Ctrl+H, empty search (80x24 popup body)

```
+-Houses & zodiac · Constructed One-----------------------+
|                                                          |
|  Search: █                                               |
|  ------------------------------------------------------  |
|> Placidus                                   Pl  ●        |
|  Koch                                       Ko           |
|  Whole Sign                                 Wh           |
|  Equal                                      Eq           |
|  Porphyry                                   Po           |
|  Regiomontanus                              Re           |
|  Campanus                                   Ca           |
|  ------------------------------------------------------  |
|  Alcabitius                                 Al           |
|  APC                                        AP           |
|  Carter P.Equat.                            CP           |
|  Equal (MC)                                 EM           |
|  ...                                                     |
+--------------------------------------------------1 of 33-+
 Move: ↑↓ | Apply: enter | Cancel: esc
```

`●` marks the chart's current system; the cursor opens on it. Below the 15
other systems (alphabetical) comes the Zodiac group:

```
|  -- Zodiac ---------------------------------------------  |
|  Tropical                                   Tr  ●        |
|  Fagan-Bradley    Astrolog's default        Fa           |
|  Lahiri           Fagan-Bradley + 0.883208° La           |
|  Deluce           Fagan-Bradley − 3.075453° De           |
|  ...                                                     |
```

## F2 — `pl` typed (pinned block gone, one list)

```
|  Search: pl█                                             |
|  ------------------------------------------------------  |
|> Placidus                                   Pl  ●        |
|  Pullen (S.Ratio)                           PR           |
|  Pullen (S.Delta)                           PD           |
+---------------------------------------------------1 of 3-+
```

## F3 — finder on a selected chart that is not the first

```
+-Houses & zodiac · Maria — comparison uses Constructed One-+
```

## F4 — Charts panel tags (132x40)

The group is ① Constructed One (Placidus, tropical) and now; Tromsø Client is
selected too; Maria is NOT selected and is set to Koch, Lahiri.

```
+->[1] Chart------------------------------------------------------------+
|> 1 Constructed One  Mon Mar 20, 1995 11:00 CET  Bremen, DE        PlTr|
|  2 Tromsø Client    Fri Jun 01, 1990 14:00 CEST Tromsø, NO       Po*Tr|
|    Maria            Sat Jul 07, 1984 09:15 CEST Graz, AT           KoLa|
|  3                  Thu Oct 08, 2026 13:05 CEST Amsterdam, NL      PlTr|
+--------------------------------------------------------------- 1 of 4-+
```

- Selected rows show the group's frame (Placidus, tropical).
- Tromsø is at 69.6°N: Astrolog cast its Placidus as Porphyry, so its tag
  reads `Po*`; the Log says once "Tromsø Client: Placidus asked, Astrolog
  used Porphyry".
- Maria, unselected, shows her own Koch + Lahiri.

Operator decisions on these frames, 2026-10-08: the polar tag shows the system
actually used (`Po*Tr`, "What was used"); the pinned seven as drafted.

## The 22 house codes and 11 zodiac codes

Pl Placidus · Ko Koch · Eq Equal · Ca Campanus · Me Meridian · Re Regiomontanus
· Po Porphyry · Mo Morinus · To Topocentric · Al Alcabitius · Ks Krusinski ·
EM Equal (MC) · PR Pullen (S.Ratio) · PD Pullen (S.Delta) · Wh Whole Sign ·
Ve Vedic · Sr Sripati · Hz Horizon · AP APC · CP Carter P.Equat. · Su Sunshine
· Sa Savard-A

Tr Tropical · Fa Fagan-Bradley · La Lahiri · De Deluce · Ra Raman · Us
Usha-Shasi · Kr Krishnamurti · Dj Djwhal Khul · Yu Yukteshwar · Bh J.N. Bhasin
· Gc Galactic Center
