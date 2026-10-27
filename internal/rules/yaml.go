package rules

import (
	"fmt"
	"strings"
)

// node is one value of the strict YAML subset the rule and term files use:
// a scalar string, a list of nodes, or a map of keys to nodes (in file order).
// The subset is block maps and lists by indentation, `- ` list items, scalars
// to the end of the line, flow lists `[a, b]` of scalars, and whole-line `#`
// comments. Anything else is an error naming its line: a file this parser
// misreads must fail to load, never load as something else.
type node struct {
	line   int
	scalar *string
	list   []*node
	keys   []string
	fields map[string]*node
}

func (n *node) isMap() bool { return n.fields != nil }

type srcLine struct {
	n      int // 1-based
	indent int
	text   string
}

func parseYAML(src string) (*node, error) {
	var lines []srcLine
	for i, raw := range strings.Split(src, "\n") {
		if strings.ContainsRune(raw, '\t') {
			return nil, fmt.Errorf("line %d: a tab. Indent with spaces", i+1)
		}
		trimmed := strings.TrimLeft(raw, " ")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lines = append(lines, srcLine{n: i + 1, indent: len(raw) - len(trimmed), text: strings.TrimRight(trimmed, " ")})
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty file")
	}
	p := &yamlParser{lines: lines}
	n, err := p.block(lines[0].indent)
	if err != nil {
		return nil, err
	}
	if p.i < len(p.lines) {
		return nil, fmt.Errorf("line %d: unexpected indentation", p.lines[p.i].n)
	}
	return n, nil
}

type yamlParser struct {
	lines []srcLine
	i     int
}

// block reads the map or list whose entries sit at exactly indent.
func (p *yamlParser) block(indent int) (*node, error) {
	first := p.lines[p.i]
	if first.indent != indent {
		return nil, fmt.Errorf("line %d: unexpected indentation", first.n)
	}
	if strings.HasPrefix(first.text, "- ") || first.text == "-" {
		return p.list(indent)
	}
	return p.mapping(indent)
}

func (p *yamlParser) list(indent int) (*node, error) {
	out := &node{line: p.lines[p.i].n, list: []*node{}}
	for p.i < len(p.lines) && p.lines[p.i].indent == indent {
		l := p.lines[p.i]
		rest, ok := strings.CutPrefix(l.text, "- ")
		if !ok {
			return nil, fmt.Errorf("line %d: a list item must start with \"- \"", l.n)
		}
		// The item is a map whose first key sits on the dash's line, two
		// columns in; its other keys follow at that column.
		p.lines[p.i] = srcLine{n: l.n, indent: indent + 2, text: rest}
		item, err := p.mapping(indent + 2)
		if err != nil {
			return nil, err
		}
		out.list = append(out.list, item)
	}
	return out, nil
}

func (p *yamlParser) mapping(indent int) (*node, error) {
	out := &node{line: p.lines[p.i].n, fields: map[string]*node{}}
	for p.i < len(p.lines) && p.lines[p.i].indent == indent {
		l := p.lines[p.i]
		if strings.HasPrefix(l.text, "- ") {
			return nil, fmt.Errorf("line %d: a list item where a key belongs", l.n)
		}
		key, val, ok := strings.Cut(l.text, ":")
		key = strings.TrimSpace(key)
		if !ok || key == "" || strings.ContainsAny(key, " \"'{}[]") {
			return nil, fmt.Errorf("line %d: %q is not `key: value`", l.n, l.text)
		}
		if _, dup := out.fields[key]; dup {
			return nil, fmt.Errorf("line %d: %q said twice", l.n, key)
		}
		val = strings.TrimSpace(val)
		p.i++
		var v *node
		switch {
		case val == "":
			if p.i >= len(p.lines) || p.lines[p.i].indent <= indent {
				return nil, fmt.Errorf("line %d: %q has no value", l.n, key)
			}
			var err error
			if v, err = p.block(p.lines[p.i].indent); err != nil {
				return nil, err
			}
		case strings.HasPrefix(val, "["):
			items, err := flowList(val)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", l.n, err)
			}
			v = &node{line: l.n, list: items}
		case strings.HasPrefix(val, "{"), strings.HasPrefix(val, "&"), strings.HasPrefix(val, "*"), strings.HasPrefix(val, "|"), strings.HasPrefix(val, ">"):
			return nil, fmt.Errorf("line %d: %q is outside the YAML subset these files use", l.n, val)
		default:
			s := val
			v = &node{line: l.n, scalar: &s}
		}
		out.keys = append(out.keys, key)
		out.fields[key] = v
	}
	return out, nil
}

func flowList(val string) ([]*node, error) {
	inner, ok := strings.CutSuffix(strings.TrimPrefix(val, "["), "]")
	if !ok {
		return nil, fmt.Errorf("%q: a flow list ends with ]", val)
	}
	var out []*node
	if strings.TrimSpace(inner) == "" {
		return out, nil
	}
	for _, part := range strings.Split(inner, ",") {
		s := strings.TrimSpace(part)
		if s == "" || strings.ContainsAny(s, "[]{}") {
			return nil, fmt.Errorf("%q: a flow list holds plain scalars", val)
		}
		out = append(out, &node{scalar: &s})
	}
	return out, nil
}
