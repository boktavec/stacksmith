package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/boktavec/stacksmith/core"
)

const (
	exampleSpec      = "../../examples/bucket.yaml"
	exampleSpecJSON  = "../../examples/bucket.json"
	exampleStandards = "../../examples/standards.yaml"
)

// run calls Run in-process with in-memory writers: the real code path, no
// subprocess and no mocks.
func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func validate(t *testing.T, extra ...string) (int, string, string) {
	t.Helper()
	return run(t, append([]string{"validate"}, extra...)...)
}

const specYAML = `apiVersion: stacksmith/v1alpha1
kind: Bucket
metadata:
  name: orders-assets
spec:
  environment: dev
  owner: orders-team
`

func TestValidateExamplesSucceed(t *testing.T) { // AC1
	code, stdout, stderr := validate(t, "--spec", exampleSpec, "--standards", exampleStandards)
	if code != 0 || stdout != "spec and standards are valid\n" || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestValidateAcceptsAllExtensionsAndMixedFormats(t *testing.T) { // AC2
	standardsYML := writeFile(t, "standards.yml", mustRead(t, exampleStandards))
	specYML := writeFile(t, "bucket.yml", specYAML)
	for name, args := range map[string][]string{
		"json spec, yaml standards": {"--spec", exampleSpecJSON, "--standards", exampleStandards},
		"yml spec, yml standards":   {"--spec", specYML, "--standards", standardsYML},
		"yaml spec, yml standards":  {"--spec", exampleSpec, "--standards", standardsYML},
	} {
		t.Run(name, func(t *testing.T) {
			if code, _, stderr := validate(t, args...); code != 0 {
				t.Fatalf("code=%d stderr=%q", code, stderr)
			}
		})
	}
}

func TestValidateRejectsBadExtensions(t *testing.T) { // AC2
	for _, name := range []string{"spec.txt", "spec", "SPEC.YAML"} {
		t.Run(name, func(t *testing.T) {
			path := writeFile(t, name, specYAML)
			code, stdout, stderr := validate(t, "--spec", path, "--standards", exampleStandards)
			if code != 1 || stdout != "" {
				t.Fatalf("code=%d stdout=%q", code, stdout)
			}
			for _, want := range []string{"--spec", ".yaml, .yml or .json"} {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr %q missing %q", stderr, want)
				}
			}
		})
	}
	t.Run("standards flag is named", func(t *testing.T) {
		path := writeFile(t, "standards.txt", "x")
		code, _, stderr := validate(t, "--spec", exampleSpec, "--standards", path)
		if code != 1 || !strings.Contains(stderr, "--standards") {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
	})
}

func TestValidateReportsParseDiagnosticsForBothDocuments(t *testing.T) { // AC3
	spec := writeFile(t, "spec.yaml", "apiVersion: stacksmith/v1alpha1\nkind: Bucket\nmetadata:\n  name: a\nspec:\n  environment: dev\n  owner: o\n  bogus: 1\n")
	standards := writeFile(t, "standards.json", `{"apiVersion":"stacksmith/v1alpha1","kind":"PlatformStandards","spec":{"aws":{"accountId":"123456789012","region":"us-east-2","extra":1}}}`)
	code, stdout, stderr := validate(t, "--spec", spec, "--standards", standards)
	if code != 1 || stdout != "" {
		t.Fatalf("code=%d stdout=%q", code, stdout)
	}
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if len(lines) != 2 ||
		!strings.HasPrefix(lines[0], "bucket spec: spec.bogus: ") ||
		!strings.HasPrefix(lines[1], "platform standards: spec.aws.extra: ") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestValidateReportsSemanticDiagnostics(t *testing.T) { // AC4
	spec := writeFile(t, "spec.yaml", strings.Replace(specYAML, "environment: dev", "environment: staging", 1))
	code, stdout, stderr := validate(t, "--spec", spec, "--standards", exampleStandards)
	want := "bucket spec: spec.environment: environment \"staging\" is not allowed; choose one listed in platform standards.\n"
	if code != 1 || stdout != "" || stderr != want {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestDiagnosticWithoutFieldHasNoEmptyFieldSegment(t *testing.T) { // D8
	spec := writeFile(t, "spec.yaml", "{not yaml")
	_, _, stderr := validate(t, "--spec", spec, "--standards", exampleStandards)
	if !strings.HasPrefix(stderr, "bucket spec: ") || strings.HasPrefix(stderr, "bucket spec: : ") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestRunUsageErrorsExitTwoWithNothingOnStdout(t *testing.T) { // AC5
	for name, args := range map[string][]string{
		"missing spec":        {"validate", "--standards", exampleStandards},
		"missing standards":   {"validate", "--spec", exampleSpec},
		"unknown flag":        {"validate", "--bogus", "--spec", exampleSpec, "--standards", exampleStandards},
		"unknown command":     {"nope"},
		"help command":        {"help"},
		"no-help placeholder": {"no-help", "x"},
		"completion":          {"completion"},
		"extra arg":           {"validate", "--spec", exampleSpec, "--standards", exampleStandards, "extra"},
		"no subcommand":       {},
		"catalog alone":       {"catalog"},
		"catalog extra arg":   {"catalog", "list", "extra"},
		"catalog unknown sub": {"catalog", "foo"},
		"catalog list flag":   {"catalog", "list", "--format", "json"},
		"bad format":          {"validate", "--spec", exampleSpec, "--standards", exampleStandards, "--format", "yaml"},
		"bad format first":    {"validate", "--spec", "missing.yaml", "--standards", exampleStandards, "--format", "yaml"},
	} {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := run(t, args...)
			if code != 2 || stdout != "" || stderr == "" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestRunHelpExitsZero(t *testing.T) { // AC5
	for _, args := range [][]string{{"--help"}, {"-h"}, {"validate", "--help"}} {
		code, stdout, _ := run(t, args...)
		if code != 0 || !strings.Contains(stdout, "stacksmith") {
			t.Fatalf("args=%v code=%d stdout=%q", args, code, stdout)
		}
	}
}

func TestValidateMissingFileAndDirectoryExitOne(t *testing.T) { // AC6
	dir := t.TempDir()
	for name, spec := range map[string]string{
		"missing":   filepath.Join(dir, "absent.yaml"),
		"directory": mustMkdir(t, filepath.Join(dir, "dir.yaml")),
	} {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := validate(t, "--spec", spec, "--standards", exampleStandards)
			if code != 1 || stdout != "" || !strings.Contains(stderr, "--spec") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestValidateFollowsSymlinkedInput(t *testing.T) { // D6
	link := filepath.Join(t.TempDir(), "link.yaml")
	target, err := filepath.Abs(exampleSpec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if code, _, stderr := validate(t, "--spec", link, "--standards", exampleStandards); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestValidateLoadErrorStopsAtFirstFailingFileSpecFirst(t *testing.T) { // D7
	code, _, stderr := validate(t, "--spec", "missing.txt", "--standards", "also-missing.txt")
	if code != 1 || !strings.Contains(stderr, "--spec") || strings.Contains(stderr, "--standards") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestValidateOversizeFileGetsCoreDiagnostic(t *testing.T) { // AC6
	spec := writeFile(t, "spec.yaml", paddedSpec(maxInputBytes+1))
	code, stdout, stderr := validate(t, "--spec", spec, "--standards", exampleStandards)
	want := "bucket spec: document exceeds the 1 MiB limit; reduce its size.\n"
	if code != 1 || stdout != "" || stderr != want {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestValidateAcceptsFileOfExactlyOneMiB(t *testing.T) { // AC6
	spec := writeFile(t, "spec.yaml", paddedSpec(maxInputBytes))
	if code, _, stderr := validate(t, "--spec", spec, "--standards", exampleStandards); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestValidateJSONFormat(t *testing.T) { // AC7
	t.Run("valid", func(t *testing.T) {
		code, stdout, stderr := validate(t, "--spec", exampleSpec, "--standards", exampleStandards, "--format", "json")
		if code != 0 || stderr != "" || stdout != "{\"valid\":true,\"diagnostics\":[]}\n" {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		spec := writeFile(t, "spec.yaml", strings.Replace(specYAML, "environment: dev", "environment: staging", 1))
		code, stdout, stderr := validate(t, "--spec", spec, "--standards", exampleStandards, "--format", "json")
		if code != 1 || stderr != "" {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		got := decodeResult(t, stdout)
		if got.Valid || len(got.Diagnostics) != 1 {
			t.Fatalf("result=%+v", got)
		}
		d := got.Diagnostics[0]
		if d.Document != "bucket spec" || d.Field != "spec.environment" || !strings.Contains(d.Message, "staging") {
			t.Fatalf("diagnostic=%+v", d)
		}
		if !strings.Contains(stdout, `"document"`) || !strings.Contains(stdout, `"field"`) || !strings.Contains(stdout, `"message"`) {
			t.Fatalf("missing keys in %q", stdout)
		}
	})
	t.Run("load errors stay plain text", func(t *testing.T) {
		code, stdout, stderr := validate(t, "--spec", "missing.yaml", "--standards", exampleStandards, "--format", "json")
		if code != 1 || stdout != "" || !strings.Contains(stderr, "--spec") {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
	})
}

func TestValidateEscapesControlCharactersInDiagnostics(t *testing.T) { // AC8
	const ansiKey = "x\x1b[31mred"
	yamlSpec := specYAML + "  \"" + `x\e[31mred` + "\": 1\n"
	jsonSpec := `{"apiVersion":"stacksmith/v1alpha1","kind":"Bucket","metadata":{"name":"a"},"spec":{"environment":"dev","owner":"o","x\u001b[31mred":1}}`
	for name, spec := range map[string]string{
		"spec.yaml": yamlSpec,
		"spec.json": jsonSpec,
	} {
		t.Run(name, func(t *testing.T) {
			path := writeFile(t, name, spec)
			code, _, stderr := validate(t, "--spec", path, "--standards", exampleStandards)
			if code != 1 {
				t.Fatalf("code=%d stderr=%q", code, stderr)
			}
			for _, b := range []byte(strings.TrimSuffix(stderr, "\n")) {
				if b < 0x20 || b == 0x7f {
					t.Fatalf("raw control byte %#x in %q", b, stderr)
				}
			}
			if !strings.Contains(stderr, `spec.x\x1b[31mred`) {
				t.Fatalf("escaped key missing: %q", stderr)
			}

			code, stdout, _ := validate(t, "--spec", path, "--standards", exampleStandards, "--format", "json")
			got := decodeResult(t, stdout)
			if code != 1 || len(got.Diagnostics) != 1 || got.Diagnostics[0].Field != "spec."+ansiKey {
				t.Fatalf("code=%d result=%+v", code, got)
			}
		})
	}
}

func TestEscapeControlLeavesPrintableTextUnchanged(t *testing.T) { // AC8
	for _, s := range []string{`unsupported kind "x"; use "Bucket".`, "café 日本語", ""} {
		if got := escapeControl(s); got != s {
			t.Errorf("escapeControl(%q) = %q", s, got)
		}
	}
	if got := escapeControl("a\x1b\n\xffb\u0085"); got != `a\x1b\n\xffb\u0085` {
		t.Errorf("got %q", got)
	}
}

func TestOrdinaryDiagnosticPrintedByteForByte(t *testing.T) { // AC8
	spec := writeFile(t, "spec.yaml", strings.Replace(specYAML, "kind: Bucket", "kind: x", 1))
	_, _, stderr := validate(t, "--spec", spec, "--standards", exampleStandards)
	want := "bucket spec: kind: unsupported kind \"x\"; use \"Bucket\".\n"
	if stderr != want {
		t.Fatalf("stderr=%q want %q", stderr, want)
	}
}

func TestValidateWritesNoFiles(t *testing.T) { // AC9
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.yaml")
	if err := os.WriteFile(spec, []byte(specYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	before := listDir(t, dir)
	validate(t, "--spec", spec, "--standards", exampleStandards)
	validate(t, "--spec", spec, "--standards", exampleStandards, "--format", "json")
	if after := listDir(t, dir); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Fatalf("directory changed: %v -> %v", before, after)
	}
}

func TestRunNilArgsDoesNotReadOSArgs(t *testing.T) { // AC9
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	os.Args = []string{"stacksmith", "validate", "--spec", exampleSpec, "--standards", exampleStandards}
	// With nil args cobra would run the validate command above and exit 0.
	if code, stdout, _ := run(t); code != 2 || stdout != "" {
		t.Fatalf("code=%d stdout=%q", code, stdout)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mustMkdir(t *testing.T, path string) string {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// paddedSpec returns a valid spec of exactly size bytes using a trailing
// YAML comment as padding.
func paddedSpec(size int) string {
	padding := size - len(specYAML) - len("#\n")
	return specYAML + "#" + strings.Repeat("a", padding) + "\n"
}

type jsonOutput struct {
	Valid       bool `json:"valid"`
	Diagnostics []struct {
		Document, Field, Message string
	} `json:"diagnostics"`
}

func decodeResult(t *testing.T, stdout string) jsonOutput {
	t.Helper()
	var got jsonOutput
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%q", err, stdout)
	}
	return got
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// Output write failures are I/O failures (exit 1), not usage errors, and must
// not turn an invalid-input exit 1 into anything else.
func TestValidateWriteFailureExitsOne(t *testing.T) { // R1
	invalidSpec := writeFile(t, "spec.yaml", strings.Replace(specYAML, "environment: dev", "environment: staging", 1))
	for _, format := range []string{"text", "json"} {
		for name, spec := range map[string]string{"valid": exampleSpec, "invalid": invalidSpec} {
			if format == "text" && name == "invalid" {
				continue // text diagnostics go to stderr, covered below
			}
			t.Run(format+" "+name, func(t *testing.T) {
				var errOut bytes.Buffer
				code := Run([]string{"validate", "--spec", spec, "--standards", exampleStandards, "--format", format}, failingWriter{}, &errOut)
				if code != 1 || !strings.Contains(errOut.String(), "cannot write output") {
					t.Fatalf("code=%d stderr=%q", code, errOut.String())
				}
			})
		}
	}
	t.Run("text diagnostics to failing stderr", func(t *testing.T) {
		var out bytes.Buffer
		code := Run([]string{"validate", "--spec", invalidSpec, "--standards", exampleStandards}, &out, failingWriter{})
		if code != 1 || out.Len() != 0 {
			t.Fatalf("code=%d stdout=%q", code, out.String())
		}
	})
}

// Escaping must also cover load errors (echo the user's path) and usage errors
// (echo an unknown flag).
func TestErrorsEscapeControlCharacters(t *testing.T) { // R4
	cases := map[string]struct {
		args []string
		code int
	}{
		"missing file path": {[]string{"validate", "--spec", "no\x1bsuch.yaml", "--standards", exampleStandards}, 1},
		"unknown flag":      {[]string{"validate", "--bad\x1bflag"}, 2},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := run(t, tc.args...)
			if code != tc.code || stdout != "" {
				t.Fatalf("code=%d stdout=%q", code, stdout)
			}
			for _, b := range []byte(strings.TrimSuffix(stderr, "\n")) {
				if b < 0x20 {
					t.Fatalf("raw byte %#x in %q", b, stderr)
				}
			}
			if !strings.Contains(stderr, `\x1b`) {
				t.Fatalf("escape missing: %q", stderr)
			}
		})
	}
}

const catalogHeader = "BLUEPRINT"

func TestCatalogListShowsSnapshotBlueprints(t *testing.T) {
	code, stdout, stderr := run(t, "catalog", "list")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != 8 {
		t.Fatalf("want header + 7 rows, got %d lines:\n%s", len(lines), stdout)
	}
	if fields := strings.Fields(lines[0]); !slices.Equal(fields, []string{catalogHeader, "STATUS", "SUMMARY"}) {
		t.Fatalf("header = %q", lines[0])
	}
	wantIDs := []string{"argocd", "bastion", "dns", "eks", "network", "rds", "s3-event-worker"}
	for i, id := range wantIDs {
		row := lines[i+1]
		if !strings.HasPrefix(row, id+" ") {
			t.Errorf("row %d = %q, want ID %q", i, row, id)
		}
		wantStatus := "not yet supported"
		if id == "network" {
			wantStatus = "supported"
		}
		if rest := strings.TrimSpace(strings.TrimPrefix(row, id)); !strings.HasPrefix(rest, wantStatus+"  ") {
			t.Errorf("row %d = %q, want status %q", i, row, wantStatus)
		}
	}
}

func TestCatalogBareCommandNamesSubcommand(t *testing.T) {
	_, _, stderr := run(t, "catalog")
	if want := "error: a subcommand is required; use \"catalog list\"\n"; stderr != want {
		t.Fatalf("stderr=%q want %q", stderr, want)
	}
	_, _, stderr = run(t)
	if want := "error: a command is required; use \"validate\" or \"catalog list\"\n"; stderr != want {
		t.Fatalf("stderr=%q want %q", stderr, want)
	}
}

func TestCatalogListWriteFailureExitsOne(t *testing.T) {
	var errOut bytes.Buffer
	code := Run([]string{"catalog", "list"}, failingWriter{}, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "cannot write output") {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
}

func TestWriteCatalogEscapesControlCharacters(t *testing.T) {
	var out bytes.Buffer
	err := writeCatalog(&out, []core.CatalogEntry{{ID: "x\x1b[31m", Summary: "evil\x1b[2J\nsecond line", Supported: true}})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("hostile newline split the row: %q", out.String())
	}
	for _, b := range []byte(strings.TrimSuffix(out.String(), "\n")) {
		if (b < 0x20 && b != '\n') || b == 0x7f {
			t.Fatalf("raw control byte %#x in %q", b, out.String())
		}
	}
	if !strings.Contains(lines[1], `x\x1b[31m`) || !strings.Contains(lines[1], `evil\x1b[2J\nsecond line`) {
		t.Fatalf("escaped text missing: %q", lines[1])
	}
}
