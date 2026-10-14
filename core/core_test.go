package core

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

const validSpecYAML = `apiVersion: stacksmith/v2alpha1
kind: Blueprint
metadata:
  name: shared-network
spec:
  blueprint: network
  environment: dev
  tier: dev
`

const validSpecJSON = `{"apiVersion":"stacksmith/v2alpha1","kind":"Blueprint","metadata":{"name":"shared-network"},"spec":{"blueprint":"network","environment":"dev","tier":"dev"}}`

const validStandardsYAML = `apiVersion: stacksmith/v2alpha1
kind: PlatformStandards
spec:
  aws:
    accountId: "123456789012"
    region: us-east-2
  environments:
    - dev
    - prod
`

const validStandardsJSON = `{"apiVersion":"stacksmith/v2alpha1","kind":"PlatformStandards","spec":{"aws":{"accountId":"123456789012","region":"us-east-2"},"environments":["dev","prod"]}}`

func readExample(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("../examples/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExamplesParseAndValidate(t *testing.T) {
	specYAML, diagnostics := ParseSpec(readExample(t, "network.yaml"), FormatYAML)
	if len(diagnostics) != 0 {
		t.Fatalf("ParseSpec(YAML) diagnostics = %+v", diagnostics)
	}
	specJSON, diagnostics := ParseSpec(readExample(t, "network.json"), FormatJSON)
	if len(diagnostics) != 0 {
		t.Fatalf("ParseSpec(JSON) diagnostics = %+v", diagnostics)
	}
	if !reflect.DeepEqual(specYAML, specJSON) {
		t.Fatalf("YAML and JSON specs differ:\nYAML: %#v\nJSON: %#v", specYAML, specJSON)
	}
	standards, diagnostics := ParseStandards(readExample(t, "standards.yaml"), FormatYAML)
	if len(diagnostics) != 0 {
		t.Fatalf("ParseStandards(YAML) diagnostics = %+v", diagnostics)
	}
	for name, spec := range map[string]Spec{"yaml": specYAML, "json": specJSON} {
		if diagnostics := Validate(spec, standards); len(diagnostics) != 0 {
			t.Fatalf("Validate(%s example) diagnostics = %+v", name, diagnostics)
		}
	}
}

// parseCase pairs the same logical mistake in YAML and JSON so one table
// proves both formats report the same (field, message).
type parseCase struct {
	name        string
	yaml, json  string
	field       string
	message     string
	wantLineTag bool // YAML diagnostics carry a "line N:" prefix; JSON never does
}

func TestParseSpecRejectsInvalidDocuments(t *testing.T) {
	y := func(old, new string) string { return strings.Replace(validSpecYAML, old, new, 1) }
	j := func(old, new string) string { return strings.Replace(validSpecJSON, old, new, 1) }
	cases := []parseCase{
		{name: "unknown field", yaml: y("  tier: dev", "  tier: dev\n  owner: x"), json: j(`"tier":"dev"`, `"tier":"dev","owner":"x"`), field: "spec.owner", message: "unknown field", wantLineTag: true},
		{name: "duplicate key", yaml: y("  tier: dev", "  tier: dev\n  tier: prod"), json: j(`"tier":"dev"`, `"tier":"dev","tier":"prod"`), field: "spec.tier", message: "duplicate key", wantLineTag: true},
		{name: "escaped duplicate key", yaml: y("  tier: dev", "  tier: dev\n  \"ti\\u0065r\": prod"), json: j(`"tier":"dev"`, `"tier":"dev","tier":"prod"`), field: "spec.tier", message: "duplicate key", wantLineTag: true},
		{name: "wrong type", yaml: y("tier: dev", "tier: 3"), json: j(`"tier":"dev"`, `"tier":3`), field: "spec.tier", message: "expected a string", wantLineTag: true},
		{name: "null required field", yaml: y("tier: dev", "tier: null"), json: j(`"tier":"dev"`, `"tier":null`), field: "spec.tier", message: "null is not supported", wantLineTag: true},
		{name: "missing required field", yaml: y("  tier: dev\n", ""), json: j(`,"tier":"dev"`, ""), field: "spec.tier", message: "required field is missing"},
		{name: "missing apiVersion", yaml: y("apiVersion: stacksmith/v2alpha1\n", ""), json: j(`"apiVersion":"stacksmith/v2alpha1",`, ""), field: "apiVersion", message: "required field is missing"},
		{name: "apiVersion wrong type", yaml: y("apiVersion: stacksmith/v2alpha1", "apiVersion: 42"), json: j(`"apiVersion":"stacksmith/v2alpha1"`, `"apiVersion":42`), field: "apiVersion", message: "expected a string", wantLineTag: true},
		{name: "apiVersion wrong value", yaml: y("stacksmith/v2alpha1", "stacksmith/v9"), json: j("stacksmith/v2alpha1", "stacksmith/v9"), field: "apiVersion", message: `unsupported apiVersion "stacksmith/v9"; use "stacksmith/v2alpha1".`},
		{name: "kind wrong", yaml: y("kind: Blueprint", "kind: Application"), json: j(`"kind":"Blueprint"`, `"kind":"Application"`), field: "kind", message: `unsupported kind "Application"; use "Blueprint".`},
		{name: "unquoted number parameter", yaml: y("  tier: dev", "  tier: dev\n  parameters:\n    az_count: 3"), json: j(`"tier":"dev"`, `"tier":"dev","parameters":{"az_count":3}`), field: "spec.parameters.az_count", message: "expected a string", wantLineTag: true},
		{name: "unquoted boolean parameter", yaml: y("  tier: dev", "  tier: dev\n  parameters:\n    single_nat: true"), json: j(`"tier":"dev"`, `"tier":"dev","parameters":{"single_nat":true}`), field: "spec.parameters.single_nat", message: "expected a string", wantLineTag: true},
		{name: "null parameter", yaml: y("  tier: dev", "  tier: dev\n  parameters:\n    az_count: null"), json: j(`"tier":"dev"`, `"tier":"dev","parameters":{"az_count":null}`), field: "spec.parameters.az_count", message: "null is not supported", wantLineTag: true},
		{name: "duplicate parameter", yaml: y("  tier: dev", "  tier: dev\n  parameters:\n    az_count: \"2\"\n    az_count: \"3\""), json: j(`"tier":"dev"`, `"tier":"dev","parameters":{"az_count":"2","az_count":"3"}`), field: "spec.parameters.az_count", message: "duplicate key", wantLineTag: true},
		{name: "parameters not an object", yaml: y("  tier: dev", "  tier: dev\n  parameters: [a]"), json: j(`"tier":"dev"`, `"tier":"dev","parameters":["a"]`), field: "spec.parameters", message: "expected an object", wantLineTag: true},
		{name: "old bucket document", yaml: "apiVersion: stacksmith/v1alpha1\nkind: Bucket\nmetadata:\n  name: orders-assets\nspec:\n  environment: dev\n  owner: orders-team\n", json: `{"apiVersion":"stacksmith/v1alpha1","kind":"Bucket","metadata":{"name":"orders-assets"},"spec":{"environment":"dev","owner":"orders-team"}}`, field: "apiVersion", message: `unsupported apiVersion "stacksmith/v1alpha1"; use "stacksmith/v2alpha1".`},
		{name: "version reported before unknown fields", yaml: y("stacksmith/v2alpha1", "stacksmith/v1alpha1") + "extra: 1\n", json: j("stacksmith/v2alpha1", "stacksmith/v1alpha1"), field: "apiVersion", message: "unsupported apiVersion"},
	}
	for _, tc := range cases {
		for _, format := range []Format{FormatYAML, FormatJSON} {
			input, prefixed := tc.yaml, tc.wantLineTag
			if format == FormatJSON {
				input, prefixed = tc.json, false
			}
			t.Run(tc.name+"/"+formatName(format), func(t *testing.T) {
				got, diagnostics := ParseSpec([]byte(input), format)
				if len(diagnostics) == 0 {
					t.Fatal("expected a diagnostic")
				}
				if !reflect.DeepEqual(got, Spec{}) {
					t.Fatalf("invalid document returned non-zero spec: %#v", got)
				}
				d := diagnostics[0]
				if d.Document != "blueprint spec" || d.Field != tc.field || !strings.Contains(d.Message, tc.message) {
					t.Fatalf("diagnostic = %+v; want field %q and message containing %q", d, tc.field, tc.message)
				}
				if prefixed != strings.HasPrefix(d.Message, "line ") {
					t.Fatalf("line prefix mismatch (want prefix %v): %q", prefixed, d.Message)
				}
			})
		}
	}
}

func formatName(f Format) string {
	if f == FormatJSON {
		return "json"
	}
	return "yaml"
}

func TestParseSpecYAMLSyntaxRejections(t *testing.T) {
	y := func(old, new string) string { return strings.Replace(validSpecYAML, old, new, 1) }
	cases := []struct{ name, input, field, message string }{
		{"invalid YAML", "apiVersion: [", "", "invalid YAML"},
		{"custom tag", y("name: shared-network", "name: !custom shared-network"), "metadata.name", "custom or non-JSON YAML tags"},
		{"anchor", y("name: shared-network", "name: &n shared-network"), "metadata.name", "anchors are not supported"},
		{"merge key", y("  name: shared-network", "  <<: {name: ignored}\n  name: shared-network"), "metadata.<<", "merge keys are not supported"},
		{"non-string key", y("  name: shared-network", "  1: shared-network"), "metadata", "mapping keys must be strings"},
		{"trailing document", validSpecYAML + "---\nkind: Blueprint\n", "", "multiple YAML documents"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diagnostics := ParseSpec([]byte(tc.input), FormatYAML)
			if len(diagnostics) == 0 || diagnostics[0].Field != tc.field || !strings.Contains(diagnostics[0].Message, tc.message) {
				t.Fatalf("diagnostics = %+v; want field %q and message containing %q", diagnostics, tc.field, tc.message)
			}
		})
	}
}

func TestParseSpecJSONSyntaxRejections(t *testing.T) {
	cases := []struct{ name, input, field, message string }{
		{"wrong-case field", strings.Replace(validSpecJSON, `"tier"`, `"Tier"`, 1), "spec.Tier", "unknown field"},
		{"trailing object", validSpecJSON + ` {"kind":"Blueprint"}`, "", "trailing JSON data"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diagnostics := ParseSpec([]byte(tc.input), FormatJSON)
			if len(diagnostics) == 0 || diagnostics[0].Field != tc.field || !strings.Contains(diagnostics[0].Message, tc.message) {
				t.Fatalf("diagnostics = %+v; want field %q and message containing %q", diagnostics, tc.field, tc.message)
			}
		})
	}
}

func TestParseSpecParametersAbsentOrEmpty(t *testing.T) {
	inputs := map[string]struct {
		data   string
		format Format
	}{
		"absent yaml": {validSpecYAML, FormatYAML},
		"absent json": {validSpecJSON, FormatJSON},
		"empty yaml":  {validSpecYAML + "  parameters: {}\n", FormatYAML},
		"empty json":  {strings.Replace(validSpecJSON, `"tier":"dev"`, `"tier":"dev","parameters":{}`, 1), FormatJSON},
	}
	for name, in := range inputs {
		t.Run(name, func(t *testing.T) {
			got, diagnostics := ParseSpec([]byte(in.data), in.format)
			if len(diagnostics) != 0 || len(got.Spec.Parameters) != 0 {
				t.Fatalf("got spec=%#v diagnostics=%+v", got, diagnostics)
			}
		})
	}
}

func TestParseSpecRejectsOversizedDocuments(t *testing.T) {
	input := []byte(strings.Repeat(" ", maxDocumentBytes+1))
	for _, format := range []Format{FormatYAML, FormatJSON} {
		got, diagnostics := ParseSpec(input, format)
		if !reflect.DeepEqual(got, Spec{}) || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "1 MiB") {
			t.Fatalf("format %v: got spec=%#v diagnostics=%+v", format, got, diagnostics)
		}
	}
}

func TestParseRejectsInvalidUTF8(t *testing.T) {
	for _, format := range []Format{FormatYAML, FormatJSON} {
		got, diagnostics := ParseSpec([]byte{0xff}, format)
		if !reflect.DeepEqual(got, Spec{}) || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "UTF-8") {
			t.Fatalf("format %v: got spec=%#v diagnostics=%+v", format, got, diagnostics)
		}
	}
}

func TestParseSpecRejectsUnsupportedFormat(t *testing.T) {
	got, diagnostics := ParseSpec([]byte(validSpecJSON), Format(99))
	if !reflect.DeepEqual(got, Spec{}) || len(diagnostics) != 1 || diagnostics[0].Message == "" {
		t.Fatalf("got spec=%#v diagnostics=%+v", got, diagnostics)
	}
}

func TestStandardsAccountIDPreservesLeadingZeros(t *testing.T) {
	input := strings.Replace(validStandardsJSON, `"123456789012"`, `"012345678901"`, 1)
	got, diagnostics := ParseStandards([]byte(input), FormatJSON)
	if len(diagnostics) != 0 || got.Spec.AWS.AccountID != "012345678901" {
		t.Fatalf("got accountID=%q diagnostics=%+v", got.Spec.AWS.AccountID, diagnostics)
	}
}

func TestParseStandardsStrictness(t *testing.T) {
	y := func(old, new string) string { return strings.Replace(validStandardsYAML, old, new, 1) }
	j := func(old, new string) string { return strings.Replace(validStandardsJSON, old, new, 1) }
	cases := []parseCase{
		{name: "unknown field", yaml: y("    region: us-east-2", "    region: us-east-2\n    role: admin"), json: j(`"region":"us-east-2"`, `"region":"us-east-2","role":"admin"`), field: "spec.aws.role", message: "unknown field", wantLineTag: true},
		{name: "unknown field in overrides", yaml: y("  environments:", "  blueprintOverrides:\n    network:\n      owner: x\n  environments:"), json: j(`"environments"`, `"blueprintOverrides":{"network":{"owner":"x"}},"environments"`), field: "spec.blueprintOverrides.network.owner", message: "unknown field", wantLineTag: true},
		{name: "unknown field in policy", yaml: y("  environments:", "  blueprintOverrides:\n    network:\n      tiers:\n        dev:\n          parameters:\n            az_count:\n              step: 2\n  environments:"), json: j(`"environments"`, `"blueprintOverrides":{"network":{"tiers":{"dev":{"parameters":{"az_count":{"step":2}}}}}},"environments"`), field: "spec.blueprintOverrides.network.tiers.dev.parameters.az_count.step", message: "unknown field", wantLineTag: true},
		{name: "duplicate key", yaml: y("    region: us-east-2", "    region: us-east-2\n    region: us-west-2"), json: j(`"region":"us-east-2"`, `"region":"us-east-2","region":"us-west-2"`), field: "spec.aws.region", message: "duplicate key", wantLineTag: true},
		{name: "missing required field", yaml: y("    accountId: \"123456789012\"\n", ""), json: j(`"accountId":"123456789012",`, ""), field: "spec.aws.accountId", message: "required field is missing"},
		{name: "null required field", yaml: y("\"123456789012\"", "null"), json: j(`"123456789012"`, `null`), field: "spec.aws.accountId", message: "null is not supported", wantLineTag: true},
		{name: "environment item wrong type", yaml: y("    - prod", "    - [prod]"), json: j(`"prod"`, `["prod"]`), field: "spec.environments[1]", message: "expected a string", wantLineTag: true},
		{name: "old version", yaml: y("v2alpha1", "v1alpha1"), json: j("v2alpha1", "v1alpha1"), field: "apiVersion", message: `unsupported apiVersion "stacksmith/v1alpha1"; use "stacksmith/v2alpha1".`},
		{name: "Blueprint kind", yaml: y("kind: PlatformStandards", "kind: Blueprint"), json: j(`"kind":"PlatformStandards"`, `"kind":"Blueprint"`), field: "kind", message: `unsupported kind "Blueprint"; use "PlatformStandards".`},
	}
	for _, tc := range cases {
		for _, format := range []Format{FormatYAML, FormatJSON} {
			input, prefixed := tc.yaml, tc.wantLineTag
			if format == FormatJSON {
				input, prefixed = tc.json, false
			}
			t.Run(tc.name+"/"+formatName(format), func(t *testing.T) {
				got, diagnostics := ParseStandards([]byte(input), format)
				if len(diagnostics) == 0 || !reflect.DeepEqual(got, PlatformStandards{}) {
					t.Fatalf("expected failure with zero value; got standards=%#v diagnostics=%+v", got, diagnostics)
				}
				d := diagnostics[0]
				if d.Document != "platform standards" || d.Field != tc.field || !strings.Contains(d.Message, tc.message) {
					t.Fatalf("diagnostic = %+v; want field %q and message containing %q", d, tc.field, tc.message)
				}
				if prefixed != strings.HasPrefix(d.Message, "line ") {
					t.Fatalf("line prefix mismatch (want prefix %v): %q", prefixed, d.Message)
				}
			})
		}
	}
}

func validSpecAndStandards(t *testing.T) (Spec, PlatformStandards) {
	t.Helper()
	spec, diagnostics := ParseSpec([]byte(validSpecYAML), FormatYAML)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	standards, diagnostics := ParseStandards([]byte(validStandardsYAML), FormatYAML)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	return spec, standards
}

func TestValidateSemanticRules(t *testing.T) {
	cases := []struct {
		name, field, message string
		mutate               func(*Spec, *PlatformStandards)
	}{
		{"name uppercase", "metadata.name", "lowercase ASCII slug", func(s *Spec, _ *PlatformStandards) { s.Metadata.Name = "Shared_Net" }},
		{"name leading hyphen", "metadata.name", "lowercase ASCII slug", func(s *Spec, _ *PlatformStandards) { s.Metadata.Name = "-a" }},
		{"name double hyphen", "metadata.name", "lowercase ASCII slug", func(s *Spec, _ *PlatformStandards) { s.Metadata.Name = "a--b" }},
		{"name trailing hyphen", "metadata.name", "lowercase ASCII slug", func(s *Spec, _ *PlatformStandards) { s.Metadata.Name = "a-" }},
		{"name 64 characters", "metadata.name", "63", func(s *Spec, _ *PlatformStandards) { s.Metadata.Name = strings.Repeat("a", 64) }},
		{"environment slug", "spec.environment", "lowercase ASCII slug", func(s *Spec, _ *PlatformStandards) { s.Spec.Environment = "Bad" }},
		{"environment membership", "spec.environment", `environment "staging" is not allowed`, func(s *Spec, _ *PlatformStandards) { s.Spec.Environment = "staging" }},
		{"invalid allowed environment", "spec.environments[1]", "lowercase ASCII slug", func(_ *Spec, p *PlatformStandards) { p.Spec.Environments[1] = "Prod" }},
		{"duplicate environments", "spec.environments[1]", "duplicates", func(_ *Spec, p *PlatformStandards) { p.Spec.Environments[1] = "dev" }},
		{"account too short", "spec.aws.accountId", "12 ASCII digits", func(_ *Spec, p *PlatformStandards) { p.Spec.AWS.AccountID = "12345678901" }},
		{"account letters", "spec.aws.accountId", "12 ASCII digits", func(_ *Spec, p *PlatformStandards) { p.Spec.AWS.AccountID = "12345678901a" }},
		{"region syntax", "spec.aws.region", "region-code syntax", func(_ *Spec, p *PlatformStandards) { p.Spec.AWS.Region = "useast2" }},
		{"unknown blueprint", "spec.blueprint", `unknown blueprint "netwrok"; run "stacksmith catalog list" to see available blueprints.`, func(s *Spec, _ *PlatformStandards) { s.Spec.Blueprint = "netwrok" }},
		{"unsupported blueprint", "spec.blueprint", `blueprint "rds" is in the catalog but not supported by this StackSmith version; supported: network.`, func(s *Spec, _ *PlatformStandards) { s.Spec.Blueprint = "rds" }},
		{"tier not in catalog", "spec.tier", `tier "staging" is not defined for blueprint "network"; use one of: dev, prod.`, func(s *Spec, _ *PlatformStandards) { s.Spec.Tier = "staging" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, standards := validSpecAndStandards(t)
			tc.mutate(&spec, &standards)
			diagnostics := Validate(spec, standards)
			if len(diagnostics) != 1 || diagnostics[0].Field != tc.field || !strings.Contains(diagnostics[0].Message, tc.message) {
				t.Fatalf("Validate() = %+v; want exactly one diagnostic on %q containing %q", diagnostics, tc.field, tc.message)
			}
		})
	}
}

// An empty allow-list also makes the spec's environment "not allowed", so two
// diagnostics are expected.
func TestValidateEmptyStandardsEnvironments(t *testing.T) {
	spec, standards := validSpecAndStandards(t)
	standards.Spec.Environments = nil
	diagnostics := Validate(spec, standards)
	if len(diagnostics) != 2 || diagnostics[0].Field != "spec.environments" || diagnostics[1].Field != "spec.environment" {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestValidateNameLengthBoundary(t *testing.T) {
	spec, standards := validSpecAndStandards(t)
	spec.Metadata.Name = strings.Repeat("a", 63)
	if diagnostics := Validate(spec, standards); len(diagnostics) != 0 {
		t.Fatalf("63-character name rejected: %+v", diagnostics)
	}
}

func TestValidateAcceptsExtendedRegionSyntax(t *testing.T) {
	spec, standards := validSpecAndStandards(t)
	standards.Spec.AWS.Region = "eusc-de-east-1"
	if diagnostics := Validate(spec, standards); len(diagnostics) != 0 {
		t.Fatalf("valid extended AWS region syntax rejected: %+v", diagnostics)
	}
}

// Unknown and unsupported blueprints must not also produce a tier diagnostic:
// the tier is only meaningful once the blueprint is supported.
func TestValidateSkipsTierForUnresolvedBlueprint(t *testing.T) {
	for _, id := range []string{"netwrok", "rds"} {
		spec, standards := validSpecAndStandards(t)
		spec.Spec.Blueprint, spec.Spec.Tier = id, "staging"
		diagnostics := Validate(spec, standards)
		if len(diagnostics) != 1 || diagnostics[0].Field != "spec.blueprint" {
			t.Fatalf("blueprint %q: diagnostics = %+v; want only spec.blueprint", id, diagnostics)
		}
	}
}

func TestValidateCollectsDiagnosticsInStableOrder(t *testing.T) {
	spec := Spec{Metadata: SpecMetadata{Name: "Not a slug"}, Spec: BlueprintSpec{Blueprint: "rds", Environment: "staging", Tier: "x"}}
	standards := PlatformStandards{Spec: StandardsSpec{
		AWS:          AWSStandards{AccountID: "bad", Region: "invalid"},
		Environments: []string{"dev", "dev"},
	}}
	wantFields := []string{
		"metadata.name", "spec.environments[1]", "spec.environment",
		"spec.aws.accountId", "spec.aws.region", "spec.blueprint",
	}
	for range 2 { // repeated: map iteration is random, the output must not be
		var got []string
		for _, d := range Validate(spec, standards) {
			got = append(got, d.Field)
		}
		if !reflect.DeepEqual(got, wantFields) {
			t.Fatalf("fields = %v, want %v", got, wantFields)
		}
	}
}

func TestMustLoadCatalogSucceedsOnEmbeddedSnapshot(t *testing.T) {
	catalog := mustLoadCatalog()
	if _, ok := catalog.Blueprints["network"]; !ok {
		t.Fatal("embedded catalog has no network blueprint")
	}
}

// Deeply nested input must produce a small diagnostic, not a field path (or
// allocation) proportional to the nesting depth.
func TestParseSpecDeepNestingYieldsSmallDiagnostic(t *testing.T) {
	const depth = 500_000
	inputs := map[Format]string{
		FormatJSON: `{"x":` + strings.Repeat("[", depth),
		FormatYAML: "x: " + strings.Repeat("[", depth),
	}
	for format, input := range inputs {
		t.Run(formatName(format), func(t *testing.T) {
			got, diagnostics := ParseSpec([]byte(input), format)
			if !reflect.DeepEqual(got, Spec{}) || len(diagnostics) != 1 {
				t.Fatalf("got spec=%#v diagnostics=%d", got, len(diagnostics))
			}
			d := diagnostics[0]
			if len(d.Field) > 2*maxFieldPathBytes || len(d.Message) > 1024 {
				t.Fatalf("diagnostic too large: field %d bytes, message %d bytes", len(d.Field), len(d.Message))
			}
		})
	}
}

// The full v1alpha1 standards document, including its removed bucket block, is
// rejected on its version before the bucket block is looked at.
func TestParseStandardsRejectsFullOldV1alpha1Document(t *testing.T) {
	const oldYAML = `apiVersion: stacksmith/v1alpha1
kind: PlatformStandards
spec:
  aws:
    accountId: "123456789012" # Example only; replace before Terraform execution
    region: us-east-2
  environments:
    - dev
    - prod
  bucket:
    module: s3-private/v1
    bucketNamePrefix: acme
    noncurrentVersionExpirationDays: 30
`
	const oldJSON = `{"apiVersion":"stacksmith/v1alpha1","kind":"PlatformStandards","spec":{"aws":{"accountId":"123456789012","region":"us-east-2"},"environments":["dev","prod"],"bucket":{"module":"s3-private/v1","bucketNamePrefix":"acme","noncurrentVersionExpirationDays":30}}}`
	const want = `unsupported apiVersion "stacksmith/v1alpha1"; use "stacksmith/v2alpha1".`
	for format, input := range map[Format]string{FormatYAML: oldYAML, FormatJSON: oldJSON} {
		t.Run(formatName(format), func(t *testing.T) {
			_, diagnostics := ParseStandards([]byte(input), format)
			if len(diagnostics) != 1 || diagnostics[0] != (Diagnostic{Document: "platform standards", Field: "apiVersion", Message: want}) {
				t.Fatalf("diagnostics = %+v", diagnostics)
			}
		})
	}
}
