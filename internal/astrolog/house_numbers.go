package astrolog

import (
	"context"
	"strconv"
	"sync"
)

// houseNumbers maps the name Astrolog gives each of its house systems in a
// chart header to its `-c` number, as this binary reports them: Astrolog's own
// help lists only 0 to 22, so the experimental ones (23 to 39) are read by
// casting a chart in each once. Like the aspect registry, it is a fact of the
// binary in use, read once per run.
var (
	houseNumbersOnce sync.Once
	houseNumbersRead map[string]int
)

// HouseSystemNumber is the `-c` number of the house system Astrolog names name
// in a chart header ("Whole (MC)" is 23), false when no system of this
// Astrolog has that name.
func HouseSystemNumber(name string) (int, bool) {
	houseNumbersOnce.Do(func() {
		houseNumbersRead = map[string]int{}
		for n := 0; n <= 39; n++ {
			argv := append([]string{"-c", strconv.Itoa(n)}, reformPin...)
			argv = append(argv, "-qj", "2451545", "-zl", "0:00E", "45:00N", flagMainShow, "-w")
			argv = append(argv, boxPins...)
			out, _, err := spawn(context.Background(), newExempt("house numbers: the name Astrolog gives each -c system, read from one chart's centre box, -R0 prints no bodies", argv...))
			if err != nil {
				continue
			}
			if box, err := readCentreBox(out, false); err == nil {
				if _, seen := houseNumbersRead[box.houses]; !seen {
					houseNumbersRead[box.houses] = n
				}
			}
		}
	})
	n, ok := houseNumbersRead[name]
	return n, ok
}
