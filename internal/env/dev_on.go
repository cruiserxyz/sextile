//go:build dev

package env

// devBuild is true in a development build (-tags dev, which scripts/run.sh
// passes). It is the only line the tag changes.
const devBuild = true
