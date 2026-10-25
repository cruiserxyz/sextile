//go:build !dev

package env

// devBuild is false in a release build: no .env is read, no repo root is
// consulted, and the dev-only chart folder is ignored.
const devBuild = false
