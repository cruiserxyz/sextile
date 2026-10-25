//go:build !linux

package store

// renameNoReplace moves from onto to, refusing an existing to (os.ErrExist).
func renameNoReplace(from, to string) error { return linkNoReplace(from, to) }
