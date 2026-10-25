package astrolog

import (
	"context"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// cancelOwner names the package that defines cmd.Cancel. exec.CommandContext
// installs its own, so "unset" means "still os/exec's".
func cancelOwner(cmd *exec.Cmd) string {
	if cmd.Cancel == nil {
		return "nil"
	}
	return runtime.FuncForPC(reflect.ValueOf(cmd.Cancel).Pointer()).Name()
}

// Only a bounded exempt call may carry a process group, a group kill and a
// WaitDelay. Every other call's exec.Cmd stays as exec.CommandContext made it:
// a wrapper whose child holds the pipe a moment longer is a successful run
// there, and a WaitDelay would fail it.
func TestOnlyBoundedCallsGetAProcessGroupAndWaitDelay(t *testing.T) {
	ctx := context.Background()
	body := BodyCall{argv: []string{"-qm", "-R0", "1"}, band: newBand([]string{"-R0", "1"})}
	for _, c := range []struct {
		name string
		call spawnCall
	}{
		{"body call", body},
		{"exempt call", newExempt("test: no chart", "-HO")},
		{"exempt call in a dir", newExempt("test: no chart", "-HO").inDir("/tmp")},
	} {
		cmd, _, err := buildCmd(ctx, c.call)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if cmd.SysProcAttr != nil || cmd.WaitDelay != 0 {
			t.Errorf("%s: SysProcAttr=%v WaitDelay=%v, want neither set", c.name, cmd.SysProcAttr, cmd.WaitDelay)
		}
		if owner := cancelOwner(cmd); !strings.HasPrefix(owner, "os/exec.") {
			t.Errorf("%s: Cancel is %s, want os/exec's default", c.name, owner)
		}
	}

	cmd, _, err := buildCmd(ctx, newExempt("test: no chart", "-Hc").bounded())
	if err != nil {
		t.Fatal(err)
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid || strings.HasPrefix(cancelOwner(cmd), "os/exec.") || cmd.WaitDelay == 0 {
		t.Errorf("a bounded call must set Setpgid, Cancel and WaitDelay: %+v cancel=%v delay=%v", cmd.SysProcAttr, cancelOwner(cmd), cmd.WaitDelay)
	}
}
