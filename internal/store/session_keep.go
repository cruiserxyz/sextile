package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// SessionKeep is what the launch's session read did with a file it refused.
// The zero value means the file was accepted, absent, or never judged.
type SessionKeep struct {
	// Kept is the base name of the copy the refused file was moved to.
	Kept string
	// Problem is why a refused file could not be kept, in lower case, naming
	// no path and no content. When it is set the file was left exactly as it
	// was, and the caller must not write the session for the rest of the
	// launch (the specification C4).
	Problem string
}

// Test seams: the clock that stamps the copy's name, and a hook that runs
// after the refused bytes were judged and before the rename re-checks them.
var (
	keepClock = time.Now
	keepHook  = func() {}
	// keepAfterLink runs after the kept name exists and before the original
	// path is moved off; keepRename is the move itself.
	keepAfterLink = func() {}
	keepRename    = os.Rename
)

// refusedStamp is the UTC form in a kept copy's name: session.yaml.refused-<stamp>.
const refusedStamp = "20060102T150405Z"

// LoadSessionKeepingRefused is the launch's one read of the session file.
//
// It reads the bytes once and returns exactly what LoadSession would for them.
// When the file is REFUSED -- LoadSession's error, or any entry the strict
// reader dropped (a slot that is not Usable) -- it also moves the file aside to
// session.yaml.refused-<stamp>, in the same directory and with the same mode,
// before anything else in the launch can write the session. Without that the
// next quit replaces the operator's own bytes with a file this build wrote.
//
// The move never replaces an existing name: it hard-links the file to the new
// name (which fails if the name exists, a dangling symlink included) and then
// removes the old one. Before that it checks, with Lstat, that the path is
// still the same regular file it read and that the bytes are still the ones it
// judged. A symlink, a directory, or a file that changed is left alone and
// reported in SessionKeep.Problem.
func LoadSessionKeepingRefused() (Session, bool, SessionKeep, error) {
	path := SessionPath()
	before, lerr := os.Lstat(path)
	if lerr != nil {
		s, ok, err := LoadSession()
		if os.IsNotExist(lerr) {
			return s, ok, SessionKeep{}, err
		}
		// Not "absent": the path could not be checked, so it cannot be judged
		// or moved, and nothing may be written over what is there.
		return s, ok, SessionKeep{Problem: "it could not be checked"}, err
	}
	// A symlink, a directory or any other non-regular file is left alone and
	// reported whether or not its target would be accepted, or exists
	// (the specification C1).
	if p := notRegular(before.Mode()); p != "" {
		s, ok, err := LoadSession()
		return s, ok, SessionKeep{Problem: p}, err
	}
	b, rerr := os.ReadFile(path)
	var (
		s   Session
		ok  bool
		err error
	)
	switch {
	case rerr != nil && os.IsNotExist(rerr):
		return Session{}, false, SessionKeep{}, nil
	case rerr != nil:
		err = fmt.Errorf("store: LoadSession: %w", rerr)
	default:
		s, ok, err = parseSessionFile(path, b)
	}
	refused := err != nil || s.DroppedFrames > 0
	for _, sl := range s.Slots {
		if !sl.Usable {
			refused = true
		}
	}
	if !refused {
		return s, ok, SessionKeep{}, err
	}
	return s, ok, keepRefused(path, before, b, rerr == nil), err
}

// keepRefused moves the refused file aside, or says why it could not.
func keepRefused(path string, before os.FileInfo, judged []byte, haveBytes bool) SessionKeep {
	keepHook()
	if !stillThere(path, before, judged, haveBytes) {
		return SessionKeep{Problem: "it changed at launch"}
	}
	stamp := keepClock().UTC().Format(refusedStamp)
	dir := filepath.Dir(path)
	for n := 1; n < 1000; n++ {
		name := "session.yaml.refused-" + stamp
		if n > 1 {
			name = fmt.Sprintf("%s-%d", name, n)
		}
		kept := filepath.Join(dir, name)
		err := os.Link(path, kept)
		if errors.Is(err, syscall.EEXIST) {
			continue
		}
		if err != nil {
			return SessionKeep{Problem: "it could not be moved"}
		}
		// The name is ours; if it landed on a file other than the one judged
		// (swapped in since the check), drop the name and leave the path alone.
		if got, err := os.Lstat(kept); err != nil || !os.SameFile(before, got) {
			os.Remove(kept)
			return SessionKeep{Problem: "it changed at launch"}
		}
		return moveOff(path, dir, name, before)
	}
	return SessionKeep{Problem: "it could not be moved"}
}

// moveOff takes the original path away after the kept name exists, in a way
// that can never delete data it did not keep: the path is renamed onto a fresh
// placeholder of ours, and only then is the result examined. If it is the
// judged inode it is just a second name for the kept copy and goes. If it is
// anything else it is put back without replacing whatever is there now.
func moveOff(path, dir, kept string, before os.FileInfo) SessionKeep {
	keepAfterLink()
	ph, err := os.CreateTemp(dir, "session.yaml.keep-*")
	if err != nil {
		return SessionKeep{Kept: kept, Problem: "it could not be moved"}
	}
	aside := ph.Name()
	ph.Close()
	if err := keepRename(path, aside); err != nil {
		os.Remove(aside) // our own empty placeholder
		return SessionKeep{Kept: kept, Problem: "it could not be moved"}
	}
	if now, err := os.Lstat(aside); err == nil && os.SameFile(before, now) {
		if err := os.Remove(aside); err != nil {
			return SessionKeep{Kept: kept, Problem: "it could not be moved"}
		}
		return SessionKeep{Kept: kept}
	}
	// Not the file that was judged: give it back, never over anything.
	if err := os.Link(aside, path); err == nil {
		os.Remove(aside)
		return SessionKeep{Kept: kept, Problem: "it changed at launch"}
	}
	return SessionKeep{Kept: kept, Problem: "it changed at launch; it is kept as " + filepath.Base(aside)}
}

// notRegular names why a path cannot be moved aside, or "" for a plain file.
func notRegular(m os.FileMode) string {
	switch {
	case m&os.ModeSymlink != 0:
		return "it is a symlink"
	case m.IsDir():
		return "it is a directory"
	case !m.IsRegular():
		return "it is not a plain file"
	}
	return ""
}

// stillThere reports whether path is still the regular file that was judged:
// same file, same size and time, and the same bytes where bytes were read.
func stillThere(path string, before os.FileInfo, judged []byte, haveBytes bool) bool {
	now, err := os.Lstat(path)
	if err != nil || !now.Mode().IsRegular() || !os.SameFile(before, now) ||
		now.Size() != before.Size() || !now.ModTime().Equal(before.ModTime()) {
		return false
	}
	if !haveBytes {
		return true
	}
	again, err := os.ReadFile(path)
	return err == nil && bytes.Equal(again, judged)
}
