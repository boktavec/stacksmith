//go:build unix

package cli

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A FIFO has no writer here, so opening it would block forever. loadInput must
// reject it from Stat alone; the timeout turns a regression into a failure
// instead of a hung test run.
func TestValidateRejectsFIFOWithoutHanging(t *testing.T) { // AC6
	fifo := filepath.Join(t.TempDir(), "spec.yaml")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("cannot create FIFO: %v", err)
	}

	type result struct {
		code   int
		stderr string
	}
	done := make(chan result, 1)
	go func() {
		code, _, stderr := validate(t, "--spec", fifo, "--standards", exampleStandards)
		done <- result{code, stderr}
	}()

	select {
	case got := <-done:
		if got.code != 1 || !strings.Contains(got.stderr, "not a regular file") {
			t.Fatalf("code=%d stderr=%q", got.code, got.stderr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("validate hung on a FIFO")
	}
}
