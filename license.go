// Package sextile holds the files of the module root that the program embeds.
package sextile

import (
	_ "embed"
	"regexp"
)

// License is the LICENSE file as it stands.
//
//go:embed LICENSE
var License string

var copyrightRE = regexp.MustCompile(`(?m)^Copyright \(c\) \d{4} .+$`)

// Copyright is the copyright line of LICENSE, verbatim, or "" when it has none.
func Copyright() string { return copyrightRE.FindString(License) }
