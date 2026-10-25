package compose

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

const (
	baseMark      = `<line id="base"/>`
	furnitureMark = `<line id="furniture"/>`
	highlightMark = `<line id="highlight"/>`
)

func svg(viewBox, content string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="` + viewBox + `">
<!-- Astrolog 8.00 -->
<g stroke-width="16" stroke-linecap="round" fill="none">
` + content + `</g>
</svg>
`)
}

func TestCompositeReturnsVectorHighlightOverDimmedBase(t *testing.T) {
	base := svg("0 0 100 100", baseMark+"\n")
	furniture := svg("0 0 100 100", furnitureMark+"\n")
	foreground := svg("0 0 100 100", furnitureMark+"\n"+highlightMark+"\n")

	out, err := Composite(base, foreground, furniture)
	if err != nil {
		t.Fatalf("Composite: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("<?xml ")) || !bytes.Contains(out, []byte("<svg ")) {
		t.Fatal("output is not SVG")
	}
	text := string(out)
	for _, want := range []string{`fill="white"`, `opacity="0.35"`, baseMark, highlightMark} {
		if !strings.Contains(text, want) {
			t.Errorf("composite missing %q", want)
		}
	}
	if strings.Contains(text, furnitureMark) {
		t.Error("foreground furniture was promoted to full strength")
	}
	if strings.Index(text, baseMark) > strings.Index(text, highlightMark) {
		t.Error("full-strength highlight must follow the dimmed base")
	}
	decoder := xml.NewDecoder(bytes.NewReader(out))
	for {
		if _, err := decoder.Token(); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("composite is not well-formed XML: %v", err)
		}
	}
}

func TestCompositeRequiresFurniturePrefix(t *testing.T) {
	base := svg("0 0 100 100", baseMark+"\n")
	furniture := svg("0 0 100 100", furnitureMark+"\n")
	foreground := svg("0 0 100 100", highlightMark+"\n")
	if out, err := Composite(base, foreground, furniture); err == nil || out != nil ||
		!strings.Contains(err.Error(), "foreground") {
		t.Fatalf("want foreground/furniture error, got (%q, %v)", out, err)
	}
}

func TestCompositeRejectsMalformedAndMismatchedLayers(t *testing.T) {
	good := svg("0 0 100 100", furnitureMark+"\n")
	cases := []struct {
		name                   string
		base, foreground, furn []byte
		want                   string
	}{
		{"bad base", []byte("not svg"), good, good, "base"},
		{"bad foreground", good, []byte("not svg"), good, "foreground"},
		{"bad furniture", good, good, []byte("not svg"), "furniture"},
		{"geometry", svg("0 0 99 99", furnitureMark+"\n"), good, good, "geometry"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Composite(tc.base, tc.foreground, tc.furn)
			if err == nil || out != nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got (%q, %v), want error containing %q", out, err, tc.want)
			}
		})
	}
}

func TestCompositeIsDeterministic(t *testing.T) {
	base := svg("0 0 100 100", baseMark+"\n")
	furniture := svg("0 0 100 100", furnitureMark+"\n")
	foreground := svg("0 0 100 100", furnitureMark+"\n"+highlightMark+"\n")
	a, err := Composite(base, foreground, furniture)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Composite(base, foreground, furniture)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Error("composite is not deterministic")
	}
}
