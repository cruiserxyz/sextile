package astrolog

import (
	"strings"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// A chart file's Node and Lilith switches (`Yn`, `Ynn`) are read by Sextile and
// never handed to Astrolog. The form they ask for is walked once over the
// file (FileNodeForm); the guard cuts every one of them out of the record that
// reaches Astrolog (takeNodeSwitches), so each call casts the form the chart
// resolved to and no file turns a call's pins the other way.

// nodeSwitch says which of Astrolog's two Node switches tok is, as
// NProcessSwitchesRare reads the letters after `Y` (astrolog.cpp:538-545):
// after one leading `-`, `/`, `=` or `_`, `Yn0…` is nutation and no Node
// switch, `Ynn…` is the natural Lilith, and every other `Yn…` is the true Node
// and Lilith.
func nodeSwitch(tok string) (natural, ok bool) {
	if len(tok) < 3 || strings.IndexByte("-/=_", tok[0]) < 0 {
		return false, false
	}
	w := tok[1:]
	switch {
	case strings.HasPrefix(w, "Ynn"):
		return true, true
	case strings.HasPrefix(w, "Yn0"):
		return false, false
	case strings.HasPrefix(w, "Yn"):
		return false, true
	}
	return false, false
}

// nodeSwitchAt lists the indexes of the Node switches among the arguments of
// one line, with the arguments of every switch that takes any passed over: a
// label named `-Yn` is a title, not a switch. A comment ends the line, `;` or
// Astrolog's `-;` switch alike (astrolog.cpp, case ';').
//
// After a switch whose arguments Sextile does not know (any switch but the
// chart's own and the Node switches), a word that reads as a Node switch may
// be that switch's argument to Astrolog. It is then returned as unsure, and the
// guard refuses the file rather than read a form Astrolog would not cast.
func nodeSwitchAt(toks []string) (at []int, unsure string) {
	unknownArgs := false
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t != "" && t[0] == ';' || len(t) >= 2 && strings.IndexByte("-/=_", t[0]) >= 0 && t[1] == ';' {
			// After a switch of unknown arguments, the `;` may be its argument
			// and the rest of the line still read.
			if unknownArgs {
				for _, r := range toks[i+1:] {
					if _, ok := nodeSwitch(r); ok {
						return at, r
					}
				}
			}
			break
		}
		if len(t) < 2 || !startsSwitchOrComment(t) {
			continue
		}
		// A signed number after a switch is its value, as in leftOutLine.
		if i > 0 && isSwitchValue(t[1:]) {
			continue
		}
		w := t[1:]
		if _, ok := nodeSwitch(t); ok {
			if unknownArgs {
				return at, t
			}
			// A Node switch takes no argument: a word after it that is not a
			// switch would join the switch before once this one is cut out.
			if i+1 < len(toks) {
				if n := toks[i+1]; n == "" || n[0] != ';' && (!startsSwitchOrComment(n) || len(n) < 2 || isSwitchValue(n[1:])) {
					return at, t
				}
			}
			at = append(at, i)
			continue
		}
		if n, known := allowedArgs[w]; known && (t[0] == '-' || t[0] == '/') {
			i += n
		} else if n := textArgs[w]; n > 0 && (t[0] == '-' || t[0] == '/') {
			i += n
		} else if (w == "s" || w == "c3") && i+1 < len(toks) && (isOffset(toks[i+1]) || toks[i+1] != "" && toks[i+1][0] >= '0' && toks[i+1][0] <= '9') {
			i++
		} else if !allowedSwitch(t) && !strings.HasPrefix(w, "Yn0") {
			unknownArgs = true
		}
	}
	return at, ""
}

// takeNodeSwitches is the line without its Node switches, every other
// argument as written, with how many it took out. unsure is a word that may be a
// Node switch or another switch's argument (nodeSwitchAt): the file is refused.
func takeNodeSwitches(line string) (cut string, taken int, unsure string) {
	for {
		spans := astrologSpans(line)
		toks := make([]string, len(spans))
		for i, sp := range spans {
			toks[i] = sp.text
		}
		at, doubt := nodeSwitchAt(toks)
		if doubt != "" {
			return line, taken, doubt
		}
		if len(at) == 0 {
			return line, taken, ""
		}
		sp := spans[at[0]]
		line = line[:sp.start] + line[sp.end:]
		taken++
	}
}

// FileNodeForm is the form of the Nodes and Lilith a chart file states, walked
// as Astrolog reads it: from Astrolog's own default (the mean Node and the
// mean Lilith, the natural one off), each Node switch in file order, `=` sets,
// `_` clears, `-` and `/` toggle. stated is false when the file holds none.
//
// The walk is over two flags, the true Node and the natural Lilith, so it ends
// on one of the four castable pairs. The pair Astrolog cannot cast (the true
// Node with the mean Lilith) is no state of it: `Yn` moves the Node and
// Lilith together, and only `Ynn` sets the natural one.
func FileNodeForm(text string) (form recipe.NodeForm, stated bool) {
	var isTrue, isNatural bool
	lines := chartFileLines(text)
	if len(lines) > 0 {
		lines = lines[1:]
	}
	for _, line := range lines {
		toks := astrologTokens(line)
		at, _ := nodeSwitchAt(toks)
		for _, i := range at {
			stated = true
			flag := &isTrue
			if natural, _ := nodeSwitch(toks[i]); natural {
				flag = &isNatural
			}
			switch toks[i][0] {
			case '=':
				*flag = true
			case '_':
				*flag = false
			default:
				*flag = !*flag
			}
		}
	}
	switch {
	case isTrue && isNatural:
		return recipe.NodesTrueNatural, stated
	case isTrue:
		return recipe.NodesTrue, stated
	case isNatural:
		return recipe.NodesMeanNatural, stated
	}
	return recipe.NodesMean, stated
}

// CommandNodeForm is the form of the Nodes and Lilith a command line as the
// console shows it casts (argvNodeForm over its words).
func CommandNodeForm(command string) recipe.NodeForm {
	words := strings.Fields(command)
	for i, w := range words {
		// formatCommand quotes `=` words: '=Yn'.
		if len(w) >= 2 && w[0] == '\'' && w[len(w)-1] == '\'' {
			words[i] = w[1 : len(w)-1]
		}
	}
	return argvNodeForm(words)
}

// argvNodeForm is the form of the Nodes and Lilith an argv casts: the last of
// its explicit `=Yn` and `_Yn`, and of its `=Ynn` and `_Ynn`. A call the
// compiler built carries both (the explicit pair of emitNodeMode), so what it
// names is what Astrolog drew, whatever a host file set.
func argvNodeForm(argv []string) recipe.NodeForm {
	trueNode, natural := false, false
	for _, a := range argv {
		switch a {
		case "=Yn":
			trueNode = true
		case "_Yn":
			trueNode = false
		case "=Ynn":
			natural = true
		case "_Ynn":
			natural = false
		}
	}
	switch {
	case trueNode && natural:
		return recipe.NodesTrueNatural
	case natural:
		return recipe.NodesMeanNatural
	case trueNode:
		return recipe.NodesTrue
	}
	return recipe.NodesMean
}
