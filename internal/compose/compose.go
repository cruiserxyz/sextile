// Package compose assembles the highlighted wheel SVG: one plain
// base wheel dimmed toward white, with the selected/focused bodies and their
// aspect web overlaid at full strength.
//
// Astrolog's foreground SVG is its furniture-only SVG plus appended element
// groups for the requested bodies. Composite verifies that prefix relationship,
// extracts only those appended groups, and places them over a dimmed base. The
// result stays vector from Astrolog to the browser; no raster conversion or SVG
// interpretation is introduced here.
package compose

import (
	"bytes"
	"fmt"
)

// DimKeep is the percentage of base ink that survives dimming. Rendering the
// base at this opacity over an opaque white rectangle has the same blend shape
// as the former per-pixel PNG compositor.
const DimKeep = 35

var svgClose = []byte("</g>\n</svg>\n")

type svgParts struct {
	header    []byte
	groupOpen []byte
	content   []byte
}

// Composite accepts Astrolog SVGs in base, foreground and furniture order and
// returns one SVG. Foreground must be furniture plus appended highlight groups;
// this is Astrolog 8.00's deterministic -XV shape and the vector equivalent of
// subtracting the furniture layer in the former bitmap compositor.
func Composite(base, foreground, furniture []byte) ([]byte, error) {
	b, err := splitSVG("base", base)
	if err != nil {
		return nil, err
	}
	f, err := splitSVG("foreground", foreground)
	if err != nil {
		return nil, err
	}
	u, err := splitSVG("furniture", furniture)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(b.header, f.header) || !bytes.Equal(b.header, u.header) ||
		!bytes.Equal(b.groupOpen, f.groupOpen) || !bytes.Equal(b.groupOpen, u.groupOpen) {
		return nil, fmt.Errorf("compose: SVG layer geometry differs")
	}
	if !bytes.HasPrefix(f.content, u.content) {
		return nil, fmt.Errorf("compose: foreground is not furniture plus highlight groups")
	}
	highlight := f.content[len(u.content):]

	var out bytes.Buffer
	out.Grow(len(base) + len(highlight) + 160)
	out.Write(b.header)
	out.WriteString("<rect width=\"100%\" height=\"100%\" fill=\"white\"/>\n")
	fmt.Fprintf(&out, "<g opacity=\"0.%02d\">\n", DimKeep)
	out.Write(b.groupOpen)
	out.Write(b.content)
	out.WriteString("</g>\n</g>\n")
	out.Write(b.groupOpen)
	out.Write(highlight)
	out.Write(svgClose)
	return out.Bytes(), nil
}

func splitSVG(layer string, data []byte) (svgParts, error) {
	if !bytes.HasPrefix(data, []byte("<?xml ")) || !bytes.HasSuffix(data, svgClose) {
		return svgParts{}, fmt.Errorf("compose: %s layer is not an Astrolog SVG", layer)
	}
	start := bytes.Index(data, []byte("<g stroke-width="))
	if start < 0 {
		return svgParts{}, fmt.Errorf("compose: %s layer has no drawing group", layer)
	}
	openEnd := bytes.IndexByte(data[start:], '\n')
	if openEnd < 0 {
		return svgParts{}, fmt.Errorf("compose: %s layer has an unterminated drawing group", layer)
	}
	openEnd += start + 1
	contentEnd := len(data) - len(svgClose)
	if openEnd > contentEnd {
		return svgParts{}, fmt.Errorf("compose: %s layer has no drawing content", layer)
	}
	return svgParts{
		header:    data[:start],
		groupOpen: data[start:openEnd],
		content:   data[openEnd:contentEnd],
	}, nil
}
