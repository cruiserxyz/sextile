package recipe

// NodeForm is the form of the Moon's Nodes and of Lilith a chart is cast in:
// the four pairs Astrolog casts with its two switches (`Yn` for the true Node
// and Lilith, `Ynn` for the natural Lilith). The true Node with the mean
// Lilith cannot be cast, so it has no value. The zero value is the mean Node
// and the mean Lilith, Astrolog's own default.
type NodeForm int

const (
	// NodesMean is the mean Node and the mean Lilith (`_Yn _Ynn`).
	NodesMean NodeForm = iota
	// NodesTrue is the true Node and the true Lilith (`=Yn _Ynn`).
	NodesTrue
	// NodesMeanNatural is the mean Node and the natural Lilith (`_Yn =Ynn`).
	NodesMeanNatural
	// NodesTrueNatural is the true Node and the natural Lilith (`=Yn =Ynn`).
	NodesTrueNatural
)

// Switches are the two explicit switches that cast the form, in the order a
// call carries them: the Node's (`Yn`), then Lilith's (`Ynn`). Both always set
// or clear, never toggle, so a host file cannot turn them the other way.
func (f NodeForm) Switches() [2]string {
	yn, ynn := "_Yn", "_Ynn"
	if f == NodesTrue || f == NodesTrueNatural {
		yn = "=Yn"
	}
	if f == NodesMeanNatural || f == NodesTrueNatural {
		ynn = "=Ynn"
	}
	return [2]string{yn, ynn}
}

// Words are the form in the words a file of the user's holds: the Node as
// `mean` or `true`, Lilith as `mean`, `true` or `natural`.
func (f NodeForm) Words() (node, lilith string) {
	switch f {
	case NodesTrue:
		return "true", "true"
	case NodesMeanNatural:
		return "mean", "natural"
	case NodesTrueNatural:
		return "true", "natural"
	}
	return "mean", "mean"
}

// NodeFormFromWords is the form the two words name. It is false for a word it
// does not know and for the pair Astrolog cannot cast (the true Node with the
// mean Lilith).
func NodeFormFromWords(node, lilith string) (NodeForm, bool) {
	for f := NodesMean; f <= NodesTrueNatural; f++ {
		if n, l := f.Words(); n == node && l == lilith {
			return f, true
		}
	}
	return NodesMean, false
}

// Letters are the one-letter marks of the form that Bodies shows beside the
// Node (and the South Node) and beside Lilith: `m` mean, `t` true, `n`
// natural.
func (f NodeForm) Letters() (node, lilith byte) {
	n, l := f.Words()
	return n[0], l[0]
}
