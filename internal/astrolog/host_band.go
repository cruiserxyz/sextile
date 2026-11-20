package astrolog

// hostBand is in every call (CallHead), before the call's own switches. Astrolog
// reads the host's astrolog.as before the command line, and most of its
// switches toggle, so a setting there would turn a call's own switch the other
// way, or add a chart the call did not ask for. Each switch here sets what the
// host file may have changed back to what Astrolog's own astrolog.as sets, so a
// call's toggles start from one known state. On a stock installation the band
// changes nothing Astrolog prints (measured for every kind of call, 2026-11-15).
//
// Arguments matter: `_tr`, `_T` and `_V` read a month and a year even when they
// turn their chart off, so they carry ones that are never used, and they set
// Astrolog's month search on, which `_dm` after them turns off again. The
// picture options are set by the wheel call itself (wheelBand and emitRender):
// any `=X…` here would start a picture in every call.
var hostBand = []string{
	// Text, not a picture: a host `=X…` line turns pictures on, as Astrolog's
	// own file turns them off with `_X`. A wheel call turns them on again.
	"_X",
	// No chart but the one a call asks for, and no help page.
	"_v", "_w", ":w", "0", "_g", "_aa", "_m", "_Z", "_S", "_l", "_j", "_7", "_L", "_K",
	"_tr", "1", "2000", "_T", "1", "1", "2000", "_V", "1", "2000",
	"_dm", ":d", "48", "_D", "_E", "_P", "_Zd", "_B", "_N", "_8", "_Ux", "_v0",
	"_HO", "_HA", "_Hc", "_H", "_Y", "_HX", "_HC", "_HF", "_HS", "_H7", "_HI",
	// The text form: no colour, minutes, longitudes, 2D houses with the 3D method
	// at Astrolog's own 1 (a bare `_c3` keeps the host's method, which a chart
	// file's `=c3` turns on again).
	"_k", "_b0", "_sr", "_c3", "1",
	// The rest of what Astrolog's own astrolog.as sets for the text: seconds and
	// milliseconds, rounding, clipping, the cusp and eclipse rules, the Nodes,
	// Sabian symbols, the chart list, orb type, columns, the Arabic part
	// formula, the input encoding, the time zone form, the stationary
	// threshold, the zodiac offset and form, and the plane.
	"_b1", "_b2", "=0b", "_Yr", "_Y8", "=YC", "=Yu0", "_Yn", "_I0", "_5",
	":gs", "0", ":I", "80", "-YP", "0", "-Ya0", "_Yz1", "-Yw", "0.0",
	"_s", "0", ":sz", "_sr0", "_Ys",
	// The subdivision type: a text listing prints another column under `=v3 5`
	// as well as a wheel drawing another ring.
	"=v3", "0",
	// Positions, as Astrolog's own file leaves them: -YT, -YV, -Yf and -Yh off,
	// the Nodes and Lilith in its default form, and the computed delta-T.
	"_YT", "_YV", "_Yf", "_Yh", "_Yn0", "_Ynn", "_Yz0",
	// The Swiss Ephemeris and its files: each `_b?` also turns the files off,
	// so `=b` comes last.
	"_bs", "_bj", "_bp", "_bm", "=b",
	// Standard time unless a call says otherwise, and no time added to a chart.
	"-z0", "0", "-YzO", "0", "-YzC", "0",
}

// CallHead is what every call starts with, before its own switches. Never
// page: a user's astrolog.as may set -YQ 24, and a pager waiting for a
// keypress on a pipe loops until the process crashes, so -YQ 0 comes first.
// Then the host band, then the hook-clear band (hooks.go), after which the
// command shown to the user begins (argv_display.go). It is a fresh slice on
// every call.
func CallHead() []string {
	head := append([]string{"-YQ", "0"}, hostBand...)
	return append(head, HookClearBand()...)
}

// wheelBand is what the wheel call (emitRender) sets before its own options,
// each at the value Astrolog's own astrolog.as gives it, so a picture setting
// in the user's file moves no wheel. The options emitRender sets after it win.
var wheelBand = []string{
	"_XJ", "=Xm", "=XQ", "=XQ0", "=Xv0", ":Xv", "1", ":YXG", "111221",
	":Xs", "200", ":XS", "100", ":XI0", "25", "1", ":Xkv", "Aut",
	":YXf", "#000000", ":YXg", "0", ":YXx", "0",
}

// hostFollowed is every setting of Astrolog's own astrolog.as that no call
// resets, each with why following the user's file is right.
// TestEveryAstrologSettingIsResetOrFollowedOnPurpose reads the list Astrolog
// writes (`astrolog -od <file> -n`) and fails naming a
// switch that is in neither the band, the wheel's pins, emitRender's options nor
// this table, so a setting a later Astrolog adds cannot go unseen. The key is
// the switch without its leading `-`, `=` or `_`; a `:` setter keeps its colon.
var hostFollowed = map[string]string{
	// Every call states its own moment and place.
	"z": "every call names its zone, or reads one from the chart it casts", "zl": "every call names its place",
	"zj": "the default name and place label are never printed by a call that names its own",
	"n":  "no call asks for the chart of now, each names its moment", "Yz": "the time added to a chart of now",
	// Elevation and temperature act on topocentric positions only, which `_YT` turns off.
	"zv": "elevation acts only on topocentric positions, and the band turns those off",
	"zf": "temperature acts only on topocentric positions, and the band turns those off",
	// The user's own Astrolog defaults, by design (emitAspects, the object policy).
	"A": "the aspect set is the user's default unless a policy names one", "YAo": "orbs are the user's default unless a policy names them",
	"YAm": "maximum orbs are the user's default", "YAd": "orb additions are the user's default",
	"YR": "the default object set is read from the host by design", "YRT": "the default transit object set is read from the host by design",
	"YR0": "restrictions of the default object set, read from the host by design", "YR1": "same", "YR2": "same",
	"YR7": "same", "YRZ": "same", "YRp": "same",
	// Every casting call names its system; `_c3` is the band's.
	"c": "every casting call names its house system",
	// The clock form: every call that prints a date or a time sets its own
	// (Clock.Pins, checked by TestEveryDisplayCallPinsTheClock).
	"Yd": "every call that prints a date sets its own date form", "Yt": "every call that prints a time sets its own clock form",
	"Yv": "units of elevation only", "YO": "copy and printing only", "0n": "web queries only",
	"Yi1": "the user's ephemeris folder", "Yi2": "the user's font folder", "Yi3": "the user's source folder",
	// Influence and ray weights, which the Details influence report shows as the user set them.
	"Yj": "influence weights, shown as the user set them", "YjC": "same", "YjA": "same", "YjT": "same",
	"Yj0": "same", "Yj7": "same", "YJ": "rulerships for the influence report, as the user set them", "YJ0": "same",
	"Y7C": "ray weights, as the user set them", "Y7O": "same",
	// Colours: the band sets `_k`, but a call that parses Astrolog's colours adds
	// `-k` again (run.go), and shows the user's colours then, as the wheel does.
	// A host `=Xr` (reverse colours) moves only those escapes, and the wheel sets it itself.
	"YkO": "colours", "YkA": "colours", "YkC": "colours", "Yk7": "colours", "Yk0": "colours", "Yk": "colours",
	// Progressions: no call runs one (no `-p` switch is emitted anywhere).
	":pd": "progressions are never asked for", ":pC": "progressions are never asked for",
	":pO": "progressions are never asked for", "pc": "progressions are never asked for",
	// Macros: no call runs one.
	"M0": "no call runs a macro",
	// The wheel call sets its size and format itself; these draw nothing a Sextile wheel shows.
	":Xw": "a wheel's width is always set by emitRender (RenderSpec.Width is required)", ":Xbw": "the wheel names its format (-XV or -Xbp)",
	":YXS": "orbit radius: drawn only by the orbit chart, never a wheel", ":YXj": "orbit trail: drawn only by the orbit chart, never a wheel",
	":YX7": "ray column width: drawn only by the ray chart, never a wheel",
	":YXp": "PostScript paper: a wheel is SVG or PNG", ":YXp0": "PostScript paper: a wheel is SVG or PNG",
	// The user's sidereal offset only acts through a sidereal call, which names its own.
	":s": "the offset a sidereal call uses is the one that call names (frameBand)",
}
