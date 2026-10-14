package core

// These helpers are kept for Story 3 (Terraform rendering), which will add
// tests that run Terraform against generated output. No test in this file
// runs Terraform today.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const (
	// requireTerraformEnv turns every skip in this file into a failure.
	requireTerraformEnv = "STACKSMITH_REQUIRE_TERRAFORM"

	terraformTimeout = 5 * time.Minute
)

// skipOrFail skips the test, or fails it when STACKSMITH_REQUIRE_TERRAFORM=1,
// so a skipped check can never be mistaken for a passed one in CI.
func skipOrFail(t *testing.T, notVerified, reason string) {
	t.Helper()
	msg := fmt.Sprintf("NOT VERIFIED: %s (%s)", notVerified, reason)
	if os.Getenv(requireTerraformEnv) == "1" {
		t.Fatalf("%s [%s=1]", msg, requireTerraformEnv)
	}
	t.Skip(msg)
}

// terraformEnv is an allowlist: no AWS_* variable and no user CLI config can
// reach a Terraform subprocess.
func terraformEnv(t *testing.T) []string {
	t.Helper()
	env := []string{
		"CHECKPOINT_DISABLE=1",
		"TF_IN_AUTOMATION=1",
		"TF_CLI_CONFIG_FILE=" + filepath.Join(t.TempDir(), "no-such-terraformrc"),
	}
	for _, name := range []string{"PATH", "HOME", "TMPDIR"} {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return env
}

// terraformCmd builds one fixed-argument Terraform command (no shell) with the
// allowlisted environment and a timeout.
func terraformCmd(t *testing.T, dir string, args ...string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), terraformTimeout)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "terraform", args...)
	cmd.Dir = dir
	cmd.WaitDelay = 10 * time.Second
	cmd.Env = terraformEnv(t)
	return cmd
}
