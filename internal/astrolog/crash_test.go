package astrolog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// standIn points binary at a script for the test.
func standIn(t *testing.T, body string) {
	t.Helper()
	script := filepath.Join(t.TempDir(), "stand-in")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := binary
	binary = script
	t.Cleanup(func() { binary = old })
}

// A call the binary dies on is its own outcome, for both kinds of call, and
// nothing it printed first comes back.
func TestCrashIsItsOwnOutcome(t *testing.T) {
	standIn(t, "echo partial table\nkill -SEGV $$")

	out, cmdText, err := invoke(context.Background(), []string{"-j"})
	if !errors.Is(err, ErrAstrologCrashed) {
		t.Fatalf("exempt call: want ErrAstrologCrashed, got %v", err)
	}
	if !strings.Contains(err.Error(), "segmentation fault") && !strings.Contains(err.Error(), "SIGSEGV") {
		t.Errorf("the error does not name the signal: %v", err)
	}
	if out != "" {
		t.Errorf("exempt call returned stdout of a crashed run: %q", out)
	}
	if cmdText == "" {
		t.Error("a crashed call still ran a command; it must be recorded")
	}

	out, cmdText, err = spawn(context.Background(), testCall([]string{"-qa", "1", "1", "2000", "12:00", "0", "0", "0"}))
	if !errors.Is(err, ErrAstrologCrashed) {
		t.Fatalf("body call: want ErrAstrologCrashed, got %v", err)
	}
	if out != "" || cmdText == "" {
		t.Errorf("body call: out %q command %q", out, cmdText)
	}
}

// A call the caller cancelled is a cancelled one, never a crash, even though
// the process ended on a signal (our own SIGKILL).
func TestCancelledCallIsNotACrash(t *testing.T) {
	standIn(t, "exec sleep 30")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	_, _, err := invoke(ctx, []string{"-j"})
	if errors.Is(err, ErrAstrologCrashed) {
		t.Fatalf("a cancelled call reported as a crash: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want context.Canceled, got %v", err)
	}
}
