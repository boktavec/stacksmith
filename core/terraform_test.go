package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	// pluginDirEnv names an unpacked provider mirror, for example
	// <dir>/registry.terraform.io/hashicorp/aws/5.100.0/<os_arch>/.
	pluginDirEnv = "STACKSMITH_TF_PLUGIN_DIR"
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

// terraform runs a command and returns its combined output.
func terraform(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	out, err := terraformCmd(t, dir, args...).CombinedOutput()
	return string(out), err
}

func mustTerraform(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := terraform(t, dir, args...); err != nil {
		t.Fatalf("terraform %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// requireTerraform skips unless a terraform binary of at least 1.7.0 (the
// generated root's required_version) is on PATH.
func requireTerraform(t *testing.T, notVerified string) {
	t.Helper()
	if _, err := exec.LookPath("terraform"); err != nil {
		skipOrFail(t, notVerified, "terraform not found on PATH")
	}
	// Stdout only: the missing CLI config file makes Terraform print a
	// warning on stderr, which is not JSON.
	out, err := terraformCmd(t, t.TempDir(), "version", "-json").Output()
	if err != nil {
		t.Fatalf("terraform version: %v\n%s", err, out)
	}
	var v struct {
		Version string `json:"terraform_version"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("parse terraform version output %q: %v", out, err)
	}
	parts := strings.SplitN(v.Version, ".", 3)
	if len(parts) < 2 {
		t.Fatalf("unexpected terraform version %q", v.Version)
	}
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	if major < 1 || (major == 1 && minor < 7) {
		skipOrFail(t, notVerified, "terraform "+v.Version+" is older than 1.7.0")
	}
}

// requireProviderMirror returns the plugin mirror directory, skipping unless
// it is set and holds the pinned aws provider for this platform.
func requireProviderMirror(t *testing.T, notVerified string) string {
	t.Helper()
	dir := os.Getenv(pluginDirEnv)
	if dir == "" {
		skipOrFail(t, notVerified, pluginDirEnv+" is not set")
	}
	// Terraform runs with cmd.Dir set to a temp dir, so a relative path
	// would pass the check below but fail in init.
	dir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("%s: %v", pluginDirEnv, err)
	}
	osArch := filepath.Join("registry.terraform.io", "hashicorp", "aws", awsProviderVersion, runtime.GOOS+"_"+runtime.GOARCH)
	if info, err := os.Stat(filepath.Join(dir, osArch)); err != nil || !info.IsDir() {
		skipOrFail(t, notVerified, fmt.Sprintf("%s lacks %s", pluginDirEnv, osArch))
	}
	return dir
}

var (
	runBlockStart = regexp.MustCompile(`(?m)^run\s+"[^"]*"\s*\{`)
	commandApply  = regexp.MustCompile(`command\s*=\s*apply`)
	commandPlan   = regexp.MustCompile(`command\s*=\s*plan\b`)
	mockProvider  = regexp.MustCompile(`(?m)^mock_provider\s+"aws"`)
)

// requireMockedPlanOnly fails unless the test file mocks the aws provider and
// every run block declares command = plan (never apply).
func requireMockedPlanOnly(t *testing.T, name string, content []byte) {
	t.Helper()
	if !mockProvider.Match(content) {
		t.Fatalf("%s must declare mock_provider \"aws\"", name)
	}
	if commandApply.Match(content) {
		t.Fatalf("%s must not use command = apply", name)
	}
	starts := runBlockStart.FindAllIndex(content, -1)
	if len(starts) == 0 {
		t.Fatalf("%s has no run blocks", name)
	}
	for i, loc := range starts {
		end := len(content)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		if !commandPlan.Match(content[loc[0]:end]) {
			t.Fatalf("%s: run block at byte %d must declare command = plan", name, loc[0])
		}
	}
}

func writeFiles(t *testing.T, dir string, files []File) {
	t.Helper()
	for _, f := range files {
		path := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, f.Content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// generatedExampleDir writes the generated YAML-example output (exactly the
// 8 files) to a fresh temp dir. JSON is covered by the parity test.
func generatedExampleDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, generateExample(t, FormatYAML))
	return dir
}

func TestGeneratedOutputPassesTerraformFmt(t *testing.T) {
	requireTerraform(t, "terraform fmt -check on generated output")
	dir := generatedExampleDir(t)
	mustTerraform(t, dir, "fmt", "-check", "-recursive", "-diff")

	// The module's native tests are not generated, so check their source
	// directly. This needs only the binary, not the provider mirror.
	testsDir, err := filepath.Abs(filepath.Join("modules", "s3", "tests"))
	if err != nil {
		t.Fatal(err)
	}
	mustTerraform(t, testsDir, "fmt", "-check", "-diff")
}

func TestGeneratedOutputPassesTerraformValidate(t *testing.T) {
	requireTerraform(t, "terraform validate on generated output")
	mirror := requireProviderMirror(t, "terraform init/validate on generated output")
	dir := generatedExampleDir(t)
	mustTerraform(t, dir, "init", "-backend=false", "-input=false", "-plugin-dir="+mirror)
	mustTerraform(t, dir, "validate")
}

// TestModuleTerraformTest runs the module's native tests (mock_provider and
// command = plan only, so no AWS access) against a temp copy of the module
// made of the generated module files plus the canonical tests/ directory,
// which Generate deliberately does not bundle.
func TestModuleTerraformTest(t *testing.T) {
	requireTerraform(t, "terraform test on the s3 module")
	mirror := requireProviderMirror(t, "terraform test on the s3 module")

	moduleDir := t.TempDir()
	var moduleFiles []File
	for _, f := range generateExample(t, FormatYAML) {
		if isModulePath(f.Path) {
			f.Path = strings.TrimPrefix(f.Path, "modules/s3/")
			moduleFiles = append(moduleFiles, f)
		}
	}
	writeFiles(t, moduleDir, moduleFiles)

	canonicalTests, err := filepath.Glob(filepath.Join("modules", "s3", "tests", "*.tftest.hcl"))
	if err != nil || len(canonicalTests) == 0 {
		t.Fatalf("no canonical module tests found: %v", err)
	}
	var testFiles []File
	for _, p := range canonicalTests {
		content, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		requireMockedPlanOnly(t, p, content)
		testFiles = append(testFiles, File{Path: "tests/" + filepath.Base(p), Content: content})
	}
	writeFiles(t, moduleDir, testFiles)

	mustTerraform(t, moduleDir, "init", "-backend=false", "-input=false", "-plugin-dir="+mirror)
	mustTerraform(t, moduleDir, "test")
}
