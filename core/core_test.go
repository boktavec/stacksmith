package core

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

const validSpecYAML = `apiVersion: stacksmith/v1alpha1
kind: Bucket
metadata:
  name: orders-assets
spec:
  environment: dev
  owner: orders-team
`

const validSpecJSON = `{"apiVersion":"stacksmith/v1alpha1","kind":"Bucket","metadata":{"name":"orders-assets"},"spec":{"environment":"dev","owner":"orders-team"}}`

const validStandardsYAML = `apiVersion: stacksmith/v1alpha1
kind: PlatformStandards
spec:
  aws:
    accountId: "123456789012"
    region: us-east-2
  environments:
    - dev
    - prod
  bucket:
    module: s3-private/v1
    bucketNamePrefix: acme
    noncurrentVersionExpirationDays: 30
`

const validStandardsJSON = `{"apiVersion":"stacksmith/v1alpha1","kind":"PlatformStandards","spec":{"aws":{"accountId":"123456789012","region":"us-east-2"},"environments":["dev","prod"],"bucket":{"module":"s3-private/v1","bucketNamePrefix":"acme","noncurrentVersionExpirationDays":30}}}`

func TestExamplesParseAndValidate(t *testing.T) {
	yamlSpec, err := os.ReadFile("../examples/bucket.yaml")
	if err != nil {
		t.Fatal(err)
	}
	jsonSpec, err := os.ReadFile("../examples/bucket.json")
	if err != nil {
		t.Fatal(err)
	}
	standardsData, err := os.ReadFile("../examples/standards.yaml")
	if err != nil {
		t.Fatal(err)
	}

	specYAML, diagnostics := ParseSpec(yamlSpec, FormatYAML)
	if len(diagnostics) != 0 {
		t.Fatalf("ParseSpec(YAML) diagnostics = %+v", diagnostics)
	}
	specJSON, diagnostics := ParseSpec(jsonSpec, FormatJSON)
	if len(diagnostics) != 0 {
		t.Fatalf("ParseSpec(JSON) diagnostics = %+v", diagnostics)
	}
	if !reflect.DeepEqual(specYAML, specJSON) {
		t.Fatalf("YAML and JSON specs differ:\nYAML: %#v\nJSON: %#v", specYAML, specJSON)
	}
	standards, diagnostics := ParseStandards(standardsData, FormatYAML)
	if len(diagnostics) != 0 {
		t.Fatalf("ParseStandards(YAML) diagnostics = %+v", diagnostics)
	}
	if diagnostics := Validate(specYAML, standards); len(diagnostics) != 0 {
		t.Fatalf("Validate(example) diagnostics = %+v", diagnostics)
	}
}

func TestParseSpecRejectsInvalidDocuments(t *testing.T) {
	cases := []struct {
		name, input string
		format      Format
		field       string
		message     string
	}{
		{"invalid YAML", "apiVersion: [", FormatYAML, "", "invalid YAML"},
		{"unknown YAML field", strings.Replace(validSpecYAML, "  owner: orders-team", "  owner: orders-team\n  public: true", 1), FormatYAML, "spec.public", "unknown field"},
		{"duplicate nested YAML key", strings.Replace(validSpecYAML, "  owner: orders-team", "  owner: orders-team\n  owner: other-team", 1), FormatYAML, "spec.owner", "duplicate key"},
		{"wrong YAML scalar type", strings.Replace(validSpecYAML, "apiVersion: stacksmith/v1alpha1", "apiVersion: 42", 1), FormatYAML, "apiVersion", "expected a string"},
		{"YAML custom tag", strings.Replace(validSpecYAML, "owner: orders-team", "owner: !custom orders-team", 1), FormatYAML, "spec.owner", "custom or non-JSON YAML tags"},
		{"YAML anchor", strings.Replace(validSpecYAML, "  name: orders-assets", "  name: &bucket-name orders-assets", 1), FormatYAML, "metadata.name", "anchors are not supported"},
		{"YAML merge key", strings.Replace(validSpecYAML, "  name: orders-assets", "  <<: {name: ignored}\n  name: orders-assets", 1), FormatYAML, "metadata.<<", "merge keys are not supported"},
		{"non-string YAML key", strings.Replace(validSpecYAML, "  name: orders-assets", "  1: orders-assets", 1), FormatYAML, "metadata", "mapping keys must be strings"},
		{"trailing YAML document", validSpecYAML + "---\nkind: Bucket\n", FormatYAML, "", "multiple YAML documents"},
		{"missing required field", strings.Replace(validSpecYAML, "  owner: orders-team\n", "", 1), FormatYAML, "spec.owner", "required field is missing"},
		{"null required field", strings.Replace(validSpecYAML, "  owner: orders-team", "  owner: null", 1), FormatYAML, "spec.owner", "null is not supported"},
		{"unsupported YAML apiVersion", strings.Replace(validSpecYAML, "stacksmith/v1alpha1", "stacksmith/v9", 1), FormatYAML, "apiVersion", "unsupported apiVersion"},
		{"unsupported YAML kind", strings.Replace(validSpecYAML, "kind: Bucket", "kind: Application", 1), FormatYAML, "kind", "unsupported kind"},
		{"unknown JSON field", strings.Replace(validSpecJSON, `"owner":"orders-team"`, `"owner":"orders-team","public":true`, 1), FormatJSON, "spec.public", "unknown field"},
		{"wrong-case JSON field", strings.Replace(validSpecJSON, `"apiVersion"`, `"ApiVersion"`, 1), FormatJSON, "ApiVersion", "unknown field"},
		{"duplicate nested JSON key", strings.Replace(validSpecJSON, `"owner":"orders-team"`, `"owner":"orders-team","owner":"other-team"`, 1), FormatJSON, "spec.owner", "duplicate key"},
		{"escaped duplicate JSON key", strings.Replace(validSpecJSON, `"owner":"orders-team"`, `"owner":"orders-team","ow\u006eer":"other-team"`, 1), FormatJSON, "spec.owner", "duplicate key"},
		{"wrong JSON scalar type", strings.Replace(validSpecJSON, `"environment":"dev"`, `"environment":true`, 1), FormatJSON, "spec.environment", "expected a string"},
		{"missing JSON field", strings.Replace(validSpecJSON, `,"owner":"orders-team"`, "", 1), FormatJSON, "spec.owner", "required field is missing"},
		{"null JSON field", strings.Replace(validSpecJSON, `"owner":"orders-team"`, `"owner":null`, 1), FormatJSON, "spec.owner", "null is not supported"},
		{"trailing JSON object", validSpecJSON + ` {"kind":"Bucket"}`, FormatJSON, "", "trailing JSON data"},
		{"unsupported JSON apiVersion", strings.Replace(validSpecJSON, "stacksmith/v1alpha1", "stacksmith/v9", 1), FormatJSON, "apiVersion", "unsupported apiVersion"},
		{"unsupported JSON kind", strings.Replace(validSpecJSON, `"kind":"Bucket"`, `"kind":"Application"`, 1), FormatJSON, "kind", "unsupported kind"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, diagnostics := ParseSpec([]byte(tc.input), tc.format)
			if len(diagnostics) == 0 {
				t.Fatal("expected a diagnostic")
			}
			if !reflect.DeepEqual(got, Spec{}) {
				t.Fatalf("invalid document returned non-zero spec: %#v", got)
			}
			if diagnostics[0].Document != "bucket spec" || diagnostics[0].Field != tc.field || !strings.Contains(diagnostics[0].Message, tc.message) {
				t.Fatalf("diagnostic = %+v; want field %q and message containing %q", diagnostics[0], tc.field, tc.message)
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

func TestStandardsOptionalPrefixDefaultsEmpty(t *testing.T) {
	input := strings.Replace(validStandardsJSON, `,"bucketNamePrefix":"acme"`, "", 1)
	got, diagnostics := ParseStandards([]byte(input), FormatJSON)
	if len(diagnostics) != 0 || got.Spec.Bucket.BucketNamePrefix != "" {
		t.Fatalf("got standards=%#v diagnostics=%+v", got, diagnostics)
	}
}

func TestStandardsAccountIDPreservesLeadingZeros(t *testing.T) {
	input := strings.Replace(validStandardsJSON, `"123456789012"`, `"012345678901"`, 1)
	got, diagnostics := ParseStandards([]byte(input), FormatJSON)
	if len(diagnostics) != 0 || got.Spec.AWS.AccountID != "012345678901" {
		t.Fatalf("got accountID=%q diagnostics=%+v", got.Spec.AWS.AccountID, diagnostics)
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

func TestParseStandardsStrictness(t *testing.T) {
	cases := []struct {
		name, input string
		format      Format
		field       string
		message     string
	}{
		{"valid JSON", validStandardsJSON, FormatJSON, "", ""},
		{"unknown field", strings.Replace(validStandardsYAML, "    region: us-east-2", "    region: us-east-2\n    role: admin", 1), FormatYAML, "spec.aws.role", "unknown field"},
		{"duplicate YAML key", strings.Replace(validStandardsYAML, "    region: us-east-2", "    region: us-east-2\n    region: us-west-2", 1), FormatYAML, "spec.aws.region", "duplicate key"},
		{"duplicate JSON key", strings.Replace(validStandardsJSON, `"accountId":"123456789012"`, `"accountId":"123456789012","accountId":"000000000000"`, 1), FormatJSON, "spec.aws.accountId", "duplicate key"},
		{"missing required field", strings.Replace(validStandardsYAML, "    accountId: \"123456789012\"\n", "", 1), FormatYAML, "spec.aws.accountId", "required field is missing"},
		{"null required field", strings.Replace(validStandardsJSON, `"accountId":"123456789012"`, `"accountId":null`, 1), FormatJSON, "spec.aws.accountId", "null is not supported"},
		{"null optional prefix", strings.Replace(validStandardsJSON, `"bucketNamePrefix":"acme",`, `"bucketNamePrefix":null,`, 1), FormatJSON, "spec.bucket.bucketNamePrefix", "null is not supported"},
		{"wrong YAML expiration type", strings.Replace(validStandardsYAML, "noncurrentVersionExpirationDays: 30", "noncurrentVersionExpirationDays: 30.5", 1), FormatYAML, "spec.bucket.noncurrentVersionExpirationDays", "whole number"},
		{"fractional expiration", strings.Replace(validStandardsJSON, `"noncurrentVersionExpirationDays":30`, `"noncurrentVersionExpirationDays":30.5`, 1), FormatJSON, "spec.bucket.noncurrentVersionExpirationDays", "whole number"},
		{"integer overflow", strings.Replace(validStandardsJSON, `"noncurrentVersionExpirationDays":30`, `"noncurrentVersionExpirationDays":999999999999999999999999`, 1), FormatJSON, "spec.bucket.noncurrentVersionExpirationDays", "whole number"},
		{"unsupported module", strings.Replace(validStandardsJSON, "s3-private/v1", "other/v1", 1), FormatJSON, "spec.bucket.module", "unsupported module"},
		{"unsupported version", strings.Replace(validStandardsJSON, "stacksmith/v1alpha1", "stacksmith/v2", 1), FormatJSON, "apiVersion", "unsupported apiVersion"},
		{"unsupported kind", strings.Replace(validStandardsJSON, `"kind":"PlatformStandards"`, `"kind":"Bucket"`, 1), FormatJSON, "kind", "unsupported kind"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, diagnostics := ParseStandards([]byte(tc.input), tc.format)
			if tc.message == "" {
				if len(diagnostics) != 0 || got.Kind != "PlatformStandards" {
					t.Fatalf("got standards=%#v diagnostics=%+v", got, diagnostics)
				}
				return
			}
			if len(diagnostics) == 0 || !reflect.DeepEqual(got, PlatformStandards{}) {
				t.Fatalf("expected failure with zero value; got standards=%#v diagnostics=%+v", got, diagnostics)
			}
			if diagnostics[0].Field != tc.field || !strings.Contains(diagnostics[0].Message, tc.message) {
				t.Fatalf("diagnostic = %+v; want field %q and message containing %q", diagnostics[0], tc.field, tc.message)
			}
		})
	}
}

func TestValidateSemanticRules(t *testing.T) {
	baseSpec := Spec{APIVersion: "stacksmith/v1alpha1", Kind: "Bucket", Metadata: SpecMetadata{Name: "orders-assets"}, Spec: BucketSpec{Environment: "dev", Owner: "orders-team"}}
	baseStandards := PlatformStandards{
		APIVersion: "stacksmith/v1alpha1", Kind: "PlatformStandards",
		Spec: StandardsSpec{
			AWS:          AWSStandards{AccountID: "012345678901", Region: "us-east-2"},
			Environments: []string{"dev", "prod"},
			Bucket:       BucketStandards{Module: "s3-private/v1", BucketNamePrefix: "acme", NoncurrentVersionExpirationDays: 30},
		},
	}
	cases := []struct {
		name, field string
		mutate      func(*Spec, *PlatformStandards)
	}{
		{"name slug", "metadata.name", func(s *Spec, _ *PlatformStandards) { s.Metadata.Name = "Bad_Name" }},
		{"environment slug", "spec.environment", func(s *Spec, _ *PlatformStandards) { s.Spec.Environment = "Bad" }},
		{"prefix slug", "spec.bucket.bucketNamePrefix", func(_ *Spec, p *PlatformStandards) { p.Spec.Bucket.BucketNamePrefix = "Upper" }},
		{"empty environments", "spec.environments", func(_ *Spec, p *PlatformStandards) { p.Spec.Environments = nil }},
		{"invalid allowed environment", "spec.environments[1]", func(_ *Spec, p *PlatformStandards) { p.Spec.Environments[1] = "Prod" }},
		{"duplicate environments", "spec.environments[1]", func(_ *Spec, p *PlatformStandards) { p.Spec.Environments[1] = "dev" }},
		{"environment membership", "spec.environment", func(s *Spec, _ *PlatformStandards) { s.Spec.Environment = "staging" }},
		{"account ID", "spec.aws.accountId", func(_ *Spec, p *PlatformStandards) { p.Spec.AWS.AccountID = "123" }},
		{"region syntax", "spec.aws.region", func(_ *Spec, p *PlatformStandards) { p.Spec.AWS.Region = "US_EAST_2" }},
		{"module ID", "spec.bucket.module", func(_ *Spec, p *PlatformStandards) { p.Spec.Bucket.Module = "other/v1" }},
		{"zero expiration", "spec.bucket.noncurrentVersionExpirationDays", func(_ *Spec, p *PlatformStandards) { p.Spec.Bucket.NoncurrentVersionExpirationDays = 0 }},
		{"negative expiration", "spec.bucket.noncurrentVersionExpirationDays", func(_ *Spec, p *PlatformStandards) { p.Spec.Bucket.NoncurrentVersionExpirationDays = -1 }},
		{"blank owner", "spec.owner", func(s *Spec, _ *PlatformStandards) { s.Spec.Owner = " \t" }},
		{"owner surrounding whitespace", "spec.owner", func(s *Spec, _ *PlatformStandards) { s.Spec.Owner = " team " }},
		{"owner control character", "spec.owner", func(s *Spec, _ *PlatformStandards) { s.Spec.Owner = "team\nname" }},
		{"owner tag length", "spec.owner", func(s *Spec, _ *PlatformStandards) { s.Spec.Owner = strings.Repeat("é", maxTagValueCharacters+1) }},
		{"logical name tag length", "metadata.name", func(s *Spec, _ *PlatformStandards) { s.Metadata.Name = strings.Repeat("a", maxTagValueCharacters+1) }},
		{"reserved bucket prefix", "metadata.name", func(_ *Spec, p *PlatformStandards) { p.Spec.Bucket.BucketNamePrefix = "sthree" }},
		{"composed bucket name too long", "metadata.name", func(s *Spec, p *PlatformStandards) {
			p.Spec.Bucket.BucketNamePrefix = ""
			s.Metadata.Name = strings.Repeat("a", 37)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, standards := baseSpec, baseStandards
			standards.Spec.Environments = append([]string(nil), baseStandards.Spec.Environments...)
			tc.mutate(&spec, &standards)
			diagnostics := Validate(spec, standards)
			if !hasField(diagnostics, tc.field) {
				t.Fatalf("Validate() diagnostics = %+v; want field %q", diagnostics, tc.field)
			}
		})
	}

	spec := baseSpec
	standards := baseStandards
	standards.Spec.AWS.Region = "eusc-de-east-1"
	if diagnostics := Validate(spec, standards); len(diagnostics) != 0 {
		t.Fatalf("valid extended AWS region syntax rejected: %+v", diagnostics)
	}
	standards.Spec.AWS.Region = "us-east-2"
	spec.Metadata.Name = strings.Repeat("a", 36) // composed name is exactly 63 characters
	standards.Spec.Bucket.BucketNamePrefix = ""
	if diagnostics := Validate(spec, standards); len(diagnostics) != 0 {
		t.Fatalf("63-character bucket name rejected: %+v", diagnostics)
	}
}

func TestInvalidBucketNameRejectsS3ReservedForms(t *testing.T) {
	for _, name := range []string{"xn--reserved", "sthree-reserved", "amzn-s3-demo-name", "name-s3alias", "name--ol-s3", "name.mrap", "name--x-s3", "name--table-s3", "192.0.2.1"} {
		t.Run(name, func(t *testing.T) {
			if reason := invalidBucketName(name); reason == "" {
				t.Fatalf("invalidBucketName(%q) accepted", name)
			}
		})
	}
}

func TestValidateCollectsDiagnosticsInStableOrder(t *testing.T) {
	spec := Spec{Metadata: SpecMetadata{Name: "Not a slug"}, Spec: BucketSpec{Environment: "staging", Owner: " team "}}
	standards := PlatformStandards{Spec: StandardsSpec{
		AWS:          AWSStandards{AccountID: "bad", Region: "invalid"},
		Environments: []string{"dev", "dev"},
		Bucket:       BucketStandards{Module: "other", NoncurrentVersionExpirationDays: 0},
	}}
	first := Validate(spec, standards)
	second := Validate(spec, standards)
	if len(first) < 8 {
		t.Fatalf("expected multiple diagnostics, got %+v", first)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("diagnostics are not stable:\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

func hasField(diagnostics []Diagnostic, field string) bool {
	for _, item := range diagnostics {
		if item.Field == field {
			return true
		}
	}
	return false
}
