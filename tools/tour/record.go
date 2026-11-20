package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"github.com/cruiserxyz/sextile/internal/tour"
)

// stopKey is Ctrl+], which ends a recording and is not written.
const stopKey = 0x1d

func record(file, bin string, in, out, errOut *os.File) error {
	// The user's charts folder, read once: the run points the program at the
	// tool's own folder, and the check before writing must still mean the user's.
	charts, err := userCharts()
	if err != nil {
		return err
	}
	if err := refuseCharts(file, charts); err != nil {
		return err
	}
	if _, err := os.Stat(file); err == nil {
		return fmt.Errorf("%s exists. A recording never replaces a file", file)
	}
	if !isTerminal(in) {
		return fmt.Errorf("record needs a terminal to read keys from")
	}
	st, err := newStage(bin, "")
	if err != nil {
		return err
	}
	defer st.close()

	fmt.Fprintf(errOut, "tour: every key you type, text in a search included, is written to %s. Ctrl+] ends the recording.\n", file)

	p, _, end, err := st.start(out)
	if err != nil {
		return err
	}
	began := time.Now()
	restore, err := rawMode(in)
	if err != nil {
		end()
		return err
	}

	pr, pw := io.Pipe()
	presses := make(chan tour.Press, 256)
	stop := tour.Decode(pr, presses)
	buf := make([]byte, 256)
	var script tour.Script
	script.Start = tour.DefaultStart
	var lastAt time.Time
	collected := make(chan struct{})
	go func() {
		defer close(collected)
		var prev *tour.Press
		for pr := range presses {
			if prev == nil {
				script.Start = pr.At.Sub(began).Round(10 * time.Millisecond)
			} else {
				script.Steps[len(script.Steps)-1].Rest = pr.At.Sub(prev.At).Round(10 * time.Millisecond)
			}
			script.Steps = append(script.Steps, tour.Step{Key: pr.Key})
			cp := pr
			prev, lastAt = &cp, pr.At
		}
	}()
	var last time.Time
	for {
		n, err := in.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			done := false
			for i, c := range chunk {
				if c == stopKey {
					chunk, done = chunk[:i], true
					break
				}
			}
			if len(chunk) > 0 {
				_, _ = p.master.Write(chunk)
				_, _ = pw.Write(chunk)
			}
			if done {
				last = time.Now()
				break
			}
		}
		if err != nil {
			last = time.Now()
			break
		}
	}
	time.Sleep(200 * time.Millisecond) // the decoder reads the last keys
	stop()
	close(presses)
	<-collected
	restore()
	end()
	if n := len(script.Steps); n > 0 {
		script.Steps[n-1].Rest = last.Sub(lastAt).Round(10 * time.Millisecond)
	}
	if len(script.Steps) == 0 {
		return fmt.Errorf("no keys were pressed, so %s was not written", file)
	}
	// The folder is checked again here: the recording took minutes, and the
	// path may point elsewhere now.
	if err := refuseCharts(file, charts); err != nil {
		return err
	}
	// Only the owner reads it: it holds every key typed, searches included.
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(script.Text()); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintf(errOut, "tour: %d keys written to %s\n", len(script.Steps), file)
	return nil
}

func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS)
	return err == nil
}

// rawMode puts the terminal in raw mode and returns what puts it back.
func rawMode(f *os.File) (func(), error) {
	old, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS)
	if err != nil {
		return nil, err
	}
	raw := *old
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	raw.Cc[unix.VMIN], raw.Cc[unix.VTIME] = 1, 0
	if err := unix.IoctlSetTermios(int(f.Fd()), unix.TCSETS, &raw); err != nil {
		return nil, err
	}
	return func() { _ = unix.IoctlSetTermios(int(f.Fd()), unix.TCSETS, old) }, nil
}
