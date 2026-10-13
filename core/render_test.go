package core

import (
	"bytes"
	"flag"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata golden files")

var (
	rootFileOrder      = []string{"versions.tf", "providers.tf", "main.tf", "outputs.tf"}
	moduleFileOrder    = []string{"modules/s3/main.tf", "modules/s3/variables.tf", "modules/s3/outputs.tf", "modules/s3/versions.tf"}
	generatedFileOrder = append(append([]string{}, rootFileOrder...), moduleFileOrder...)
)

// isModulePath reports whether a generated path is a bundled module file.
// Those are covered by the byte-equality test, not by root goldens.
func isModulePath(path string) bool { return strings.HasPrefix(path, "modules/") }

func exampleResolved() resolved {
	return resolved{
		AccountID:                       "123456789012",
		Region:                          "us-east-2",
		Module:                          "s3-private/v1",
		LogicalName:                     "orders-assets",
		Environment:                     "dev",
		Owner:                           "orders-team",
		NoncurrentVersionExpirationDays: 30,
		BucketName:                      "acme-orders-assets-dev-123456789012-us-east-2",
	}
}

// parseHCL parses generated output with the real HCL parser and fails on any
// syntax error, so tests assert on structure rather than on text.
func parseHCL(t *testing.T, file File) *hclsyntax.Body {
	t.Helper()
	parsed, diags := hclsyntax.ParseConfig(file.Content, file.Path, hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("%s does not parse: %v", file.Path, diags)
	}
	return parsed.Body.(*hclsyntax.Body)
}

func stringAttr(t *testing.T, body *hclsyntax.Body, name string) string {
	t.Helper()
	attr, ok := body.Attributes[name]
	if !ok {
		t.Fatalf("attribute %q missing", name)
	}
	value, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		t.Fatalf("attribute %q: %v", name, diags)
	}
	return value.AsString()
}

func findBlock(t *testing.T, body *hclsyntax.Body, typ string, labels ...string) *hclsyntax.Block {
	t.Helper()
	for _, block := range body.Blocks {
		if block.Type == typ && reflect.DeepEqual(block.Labels, labels) {
			return block
		}
	}
	t.Fatalf("block %s %v missing", typ, labels)
	return nil
}

func generateExample(t *testing.T, format Format) []File {
	t.Helper()
	spec, standards := parseValidInputs(t, format)
	files, diagnostics := Generate(spec, standards)
	if len(diagnostics) != 0 {
		t.Fatalf("Generate() diagnostics = %+v", diagnostics)
	}
	return files
}

// TestGenerateGolden compares output for the YAML and JSON inputs with the
// reviewable files in testdata/bucket. Run `go test ./core -update` to rewrite
// them after an intentional change, then review the diff.
func TestGenerateGolden(t *testing.T) {
	for _, format := range []Format{FormatYAML, FormatJSON} {
		for _, file := range generateExample(t, format) {
			if isModulePath(file.Path) {
				continue
			}
			golden := filepath.Join("testdata", "bucket", file.Path)
			if *updateGolden && format == FormatYAML {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, file.Content, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(file.Content, want) {
				t.Errorf("%s (%v input) differs from %s:\n--- got ---\n%s\n--- want ---\n%s",
					file.Path, format, golden, file.Content, want)
			}
		}
	}
}

// TestGenerateExamplesMatchGolden runs the checked-in examples/ files through
// the real entry point and compares them with the same golden files.
func TestGenerateExamplesMatchGolden(t *testing.T) {
	specData, err := os.ReadFile("../examples/bucket.yaml")
	if err != nil {
		t.Fatal(err)
	}
	standardsData, err := os.ReadFile("../examples/standards.yaml")
	if err != nil {
		t.Fatal(err)
	}
	spec, diagnostics := ParseSpec(specData, FormatYAML)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	standards, diagnostics := ParseStandards(standardsData, FormatYAML)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	files, diagnostics := Generate(spec, standards)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if len(files) != len(generatedFileOrder) {
		t.Fatalf("got %d files; want %d", len(files), len(generatedFileOrder))
	}
	for _, file := range files {
		if isModulePath(file.Path) {
			continue
		}
		want, err := os.ReadFile(filepath.Join("testdata", "bucket", file.Path))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(file.Content, want) {
			t.Errorf("%s from examples/ differs from golden", file.Path)
		}
	}
}

func TestGenerateFilesAndOrder(t *testing.T) {
	var paths []string
	for _, file := range generateExample(t, FormatYAML) {
		paths = append(paths, file.Path)
		if isModulePath(file.Path) {
			continue // module bytes are copied unchanged, with no header
		}
		header := "# Generated by StackSmith " + generatorVersion + " (module s3-private/v1). Review before use.\n" +
			"# Do not apply a regenerated copy as a second state for the same bucket.\n"
		if !bytes.HasPrefix(file.Content, []byte(header)) {
			t.Errorf("%s does not start with the standard header:\n%s", file.Path, file.Content)
		}
	}
	if !reflect.DeepEqual(paths, generatedFileOrder) {
		t.Fatalf("paths = %v; want %v", paths, generatedFileOrder)
	}
}

// TestGenerateModuleMatchesCanonicalSource proves each bundled module file is
// byte-for-byte the canonical file, for both input formats.
func TestGenerateModuleMatchesCanonicalSource(t *testing.T) {
	for _, format := range []Format{FormatYAML, FormatJSON} {
		var n int
		for _, file := range generateExample(t, format) {
			if !isModulePath(file.Path) {
				continue
			}
			n++
			want, err := os.ReadFile(filepath.FromSlash(file.Path))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(file.Content, want) {
				t.Errorf("%s (%v input) differs from canonical source", file.Path, format)
			}
		}
		if n != len(moduleFileOrder) {
			t.Errorf("%v input: %d module files; want %d", format, n, len(moduleFileOrder))
		}
	}
}

func TestGenerateNoExcludedModuleFiles(t *testing.T) {
	for _, format := range []Format{FormatYAML, FormatJSON} {
		seen := map[string]bool{}
		for _, file := range generateExample(t, format) {
			if seen[file.Path] {
				t.Errorf("%v input: duplicate path %s", format, file.Path)
			}
			seen[file.Path] = true
			if strings.Contains(file.Path, "tests/") || strings.HasSuffix(file.Path, ".terraform.lock.hcl") {
				t.Errorf("%v input: excluded file generated: %s", format, file.Path)
			}
		}
	}
}

// TestEmbeddedModuleMatchesModuleFiles keeps the go:embed list and moduleFiles
// in step in both directions: every embedded file is emitted and vice versa.
func TestEmbeddedModuleMatchesModuleFiles(t *testing.T) {
	var embedded []string
	err := fs.WalkDir(moduleFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			embedded = append(embedded, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	want := append([]string{}, moduleFiles...)
	slices.Sort(want)
	slices.Sort(embedded)
	if !slices.Equal(embedded, want) {
		t.Fatalf("embedded = %v; moduleFiles = %v", embedded, want)
	}
}

// TestModuleDirHasNoUnreviewedFiles fails when core/modules/s3 gains a file
// that is neither bundled nor explicitly excluded, so adding a module file is
// a deliberate decision (extend the go:embed list and moduleFiles, or exclude
// it here).
func TestModuleDirHasNoUnreviewedFiles(t *testing.T) {
	bundled := map[string]bool{}
	for _, p := range moduleFileOrder {
		bundled[strings.TrimPrefix(p, "modules/s3/")] = true
	}
	excluded := map[string]bool{".terraform.lock.hcl": true}
	entries, err := os.ReadDir("modules/s3")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		switch {
		case e.IsDir() && e.Name() == "tests":
		case e.IsDir(), !bundled[e.Name()] && !excluded[e.Name()]:
			t.Errorf("modules/s3/%s is neither bundled nor excluded; decide explicitly", e.Name())
		}
		delete(bundled, e.Name())
	}
	for name := range bundled {
		t.Errorf("bundled file modules/s3/%s does not exist", name)
	}
	if !reflect.DeepEqual(moduleFiles, moduleFileOrder) {
		t.Errorf("moduleFiles = %v; want %v", moduleFiles, moduleFileOrder)
	}
}

func TestGenerateModuleIsDeterministicAndIsolated(t *testing.T) {
	first := generateExample(t, FormatYAML)
	for _, file := range first {
		if isModulePath(file.Path) {
			clear(file.Content) // a caller scribbling on its copy
		}
	}
	second := generateExample(t, FormatYAML)
	for _, file := range second {
		if !isModulePath(file.Path) {
			continue
		}
		want, err := os.ReadFile(filepath.FromSlash(file.Path))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(file.Content, want) {
			t.Errorf("%s changed after a caller mutated an earlier result", file.Path)
		}
	}
	if again := generateExample(t, FormatYAML); !reflect.DeepEqual(second, again) {
		t.Error("Generate is not deterministic")
	}
}

func TestGenerateInvalidInputYieldsNoFiles(t *testing.T) {
	spec, standards := parseValidInputs(t, FormatYAML)
	spec.Spec.Environment = "staging" // not in standards
	want := Validate(spec, standards)
	if len(want) == 0 {
		t.Fatal("test setup: expected Validate diagnostics")
	}
	files, got := Generate(spec, standards)
	if files != nil {
		t.Fatalf("files = %+v; want nil", files)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diagnostics = %+v; want %+v", got, want)
	}
}

func TestRenderRootDeterministic(t *testing.T) {
	first, second := renderRoot(exampleResolved()), renderRoot(exampleResolved())
	if !reflect.DeepEqual(first, second) {
		t.Fatal("renderRoot output differs between runs")
	}
}

func TestRenderRootStructure(t *testing.T) {
	r := exampleResolved()
	byPath := map[string]*hclsyntax.Body{}
	for _, file := range renderRoot(r) {
		byPath[file.Path] = parseHCL(t, file)
	}

	tf := findBlock(t, byPath["versions.tf"], "terraform")
	if got := stringAttr(t, tf.Body, "required_version"); got != ">= 1.7.0" {
		t.Errorf("required_version = %q", got)
	}
	providers := findBlock(t, tf.Body, "required_providers").Body.Attributes["aws"]
	awsValue, diags := providers.Expr.Value(nil)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if got := awsValue.GetAttr("version").AsString(); got != "5.100.0" {
		t.Errorf("aws version = %q; want exact pin 5.100.0", got)
	}
	if got := awsValue.GetAttr("source").AsString(); got != "hashicorp/aws" {
		t.Errorf("aws source = %q", got)
	}

	provider := findBlock(t, byPath["providers.tf"], "provider", "aws")
	if got := stringAttr(t, provider.Body, "region"); got != r.Region {
		t.Errorf("region = %q", got)
	}
	accounts, diags := provider.Body.Attributes["allowed_account_ids"].Expr.Value(nil)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if got := accounts.AsValueSlice(); len(got) != 1 || got[0].AsString() != r.AccountID {
		t.Errorf("allowed_account_ids = %v", accounts)
	}
	findBlock(t, byPath["providers.tf"], "data", "aws_caller_identity", "current")

	module := findBlock(t, byPath["main.tf"], "module", "bucket")
	if len(module.Body.Attributes) != 6 {
		t.Errorf("module has %d attributes; want source + 5 inputs", len(module.Body.Attributes))
	}
	for name, want := range map[string]string{
		"source":      "./modules/s3",
		"bucket_name": r.BucketName,
		"name":        r.LogicalName,
		"environment": r.Environment,
		"owner":       r.Owner,
	} {
		if got := stringAttr(t, module.Body, name); got != want {
			t.Errorf("module %s = %q; want %q", name, got, want)
		}
	}
	days, diags := module.Body.Attributes["noncurrent_version_expiration_days"].Expr.Value(nil)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if got, _ := days.AsBigFloat().Int64(); got != 30 {
		t.Errorf("noncurrent_version_expiration_days = %v", days)
	}

	outputs := byPath["outputs.tf"]
	findBlock(t, outputs, "output", "bucket_arn")
	bucketName := findBlock(t, outputs, "output", "bucket_name")
	precondition := findBlock(t, bucketName.Body, "precondition")
	binary, ok := precondition.Body.Attributes["condition"].Expr.(*hclsyntax.BinaryOpExpr)
	if !ok || binary.Op != hclsyntax.OpEqual {
		t.Fatalf("precondition condition is %T; want == expression", precondition.Body.Attributes["condition"].Expr)
	}
	lhs, ok := binary.LHS.(*hclsyntax.ScopeTraversalExpr)
	if !ok {
		t.Fatalf("condition lhs is %T; want a reference", binary.LHS)
	}
	wantRef := []string{"data", "aws_caller_identity", "current", "account_id"}
	var gotRef []string
	for _, step := range lhs.Traversal {
		switch s := step.(type) {
		case hcl.TraverseRoot:
			gotRef = append(gotRef, s.Name)
		case hcl.TraverseAttr:
			gotRef = append(gotRef, s.Name)
		}
	}
	if !reflect.DeepEqual(gotRef, wantRef) {
		t.Errorf("condition lhs = %v; want %v", gotRef, wantRef)
	}
	rhs, diags := binary.RHS.Value(nil)
	if diags.HasErrors() || rhs.AsString() != r.AccountID {
		t.Errorf("condition rhs = %v (%v)", rhs, diags)
	}
}

func TestRenderRootEscapesHostileValues(t *testing.T) {
	hostile := []string{
		`quo"te`,
		`back\slash`,
		"${var.x}",
		"%{if true}x%{endif}",
		"line1\nline2",
		"\"${x}\\%{y}\n",
	}
	for _, value := range hostile {
		r := exampleResolved()
		r.Owner, r.LogicalName, r.Environment, r.BucketName = value, value, value, value
		r.AccountID, r.Region, r.Module = value, value, value
		bodies := map[string]*hclsyntax.Body{}
		for _, file := range renderRoot(r) {
			bodies[file.Path] = parseHCL(t, file) // must still parse as valid HCL
		}

		module := findBlock(t, bodies["main.tf"], "module", "bucket")
		for _, name := range []string{"bucket_name", "name", "environment", "owner"} {
			if got := stringAttr(t, module.Body, name); got != value {
				t.Errorf("%s round-trips as %q; want %q", name, got, value)
			}
		}

		provider := findBlock(t, bodies["providers.tf"], "provider", "aws")
		if got := stringAttr(t, provider.Body, "region"); got != value {
			t.Errorf("region round-trips as %q; want %q", got, value)
		}
		accounts, diags := provider.Body.Attributes["allowed_account_ids"].Expr.Value(nil)
		if diags.HasErrors() {
			t.Fatal(diags)
		}
		if got := accounts.AsValueSlice(); len(got) != 1 || got[0].AsString() != value {
			t.Errorf("allowed_account_ids = %v; want [%q]", accounts, value)
		}

		output := findBlock(t, bodies["outputs.tf"], "output", "bucket_name")
		precondition := findBlock(t, output.Body, "precondition")
		binary, ok := precondition.Body.Attributes["condition"].Expr.(*hclsyntax.BinaryOpExpr)
		if !ok {
			t.Fatalf("condition is %T", precondition.Body.Attributes["condition"].Expr)
		}
		rhs, diags := binary.RHS.Value(nil)
		if diags.HasErrors() || rhs.AsString() != value {
			t.Errorf("precondition rhs = %v (%v); want %q", rhs, diags, value)
		}
	}
}

// TestCoreNonTestFilesStayPure enforces the offline boundary: no non-test file
// in core may import os, net or os/exec.
func TestCoreNonTestFilesStayPure(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, imp := range file.Imports {
			switch name := strings.Trim(imp.Path.Value, `"`); name {
			case "os", "net", "os/exec":
				t.Errorf("%s imports %q; core must stay free of I/O", path, name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no non-test files found")
	}
}
