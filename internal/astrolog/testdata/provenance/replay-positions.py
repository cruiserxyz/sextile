# Stand-in for the part of `astrolog -v` that a replayed fixture cannot
# answer by itself: WHICH objects it prints. positions-k.txt holds the
# eighteen objects Sextile's lens draws; a call asking for fewer gets only
# those rows, in Astrolog's order, as the binary prints them.
#
#   replay-positions.py <positions-k.txt> <the call's argv...>
#
#   -R0 a b c   an explicit list: only those objects
#   -R n        a toggle against the default set (flips n on or off)
#   neither     the default set: Sun to Pluto, North Node, Ascendant, Midheaven
import os
import re
import sys

NUMBERS = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 16, 17, 18, 22, 25, 28, 31]
DEFAULT = {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 16, 22, 31}
SGR = re.compile(r"\x1b\[[0-9;]*m")
ROW = re.compile(r"^\w{1,4}\s*:\s+\d+[A-Z][a-z]{2}\d+")

path, argv = sys.argv[1], sys.argv[2:]
drawn, explicit = set(DEFAULT), None
i = 0
while i < len(argv):
    if argv[i] == "-R0":
        explicit, i = set(), i + 1
        while i < len(argv) and argv[i].isdigit():
            explicit.add(int(argv[i]))
            i += 1
        continue
    if argv[i] == "-R" and i + 1 < len(argv) and argv[i + 1].isdigit():
        drawn ^= {int(argv[i + 1])}
        i += 2
        continue
    i += 1
if explicit is not None:
    drawn = explicit

# An object the fixture has no row for (an asteroid, say) gets a generic row, so
# a call for it prints as many rows as it asked for. Its name is the registry's.
names = {}
for line in open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "ho-all.txt"), encoding="latin-1"):
    if len(line) > 16 and line[:3].strip().isdigit():
        names[int(line[:3])] = line[4:16].strip()
text = open(path, "rb").read()
extra = sorted(n for n in drawn if n not in NUMBERS and n in names)


def generic(number):
    # The first row of the fixture, cut before its cusp column, under another name.
    sun = next(l for l in text.split(b"\n") if b"Sun :" in l)
    row = sun[: sun.index(b"+0.969") + len(b"+0.969")]
    return row.replace(b"Sun :", names[number][:4].ljust(4).encode("latin-1") + b":", 1)


out = sys.stdout.buffer
row = 0
kept = []
lines = text.removesuffix(b"\n").split(b"\n")
last_row = max(i for i, line in enumerate(lines) if ROW.match(SGR.sub("", line.decode("latin-1"))))
for i, line in enumerate(lines):
    if ROW.match(SGR.sub("", line.decode("latin-1"))):
        number = NUMBERS[row]
        row += 1
        while extra and extra[0] < number:
            kept.append(generic(extra.pop(0)))
        if number in drawn:
            kept.append(line)
        if i == last_row:
            while extra:
                kept.append(generic(extra.pop(0)))
        continue
    kept.append(line)
out.write(b"\n".join(kept) + (b"\n" if text.endswith(b"\n") else b""))
