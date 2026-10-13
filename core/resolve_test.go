package core

import (
	"reflect"
	"strings"
	"testing"
)

// parseValidInputs parses the shared YAML or JSON fixtures through the real
// parsers, so resolve is tested on production-shaped values.
func parseValidInputs(t *testing.T, format Format) (Spec, PlatformStandards) {
	t.Helper()
	specData, standardsData := validSpecYAML, validStandardsYAML
	if format == FormatJSON {
		specData, standardsData = validSpecJSON, validStandardsJSON
	}
	spec, diagnostics := ParseSpec([]byte(specData), format)
	if len(diagnostics) != 0 {
		t.Fatalf("ParseSpec() diagnostics = %+v", diagnostics)
	}
	standards, diagnostics := ParseStandards([]byte(standardsData), format)
	if len(diagnostics) != 0 {
		t.Fatalf("ParseStandards() diagnostics = %+v", diagnostics)
	}
	return spec, standards
}

func TestResolveCarriesEveryField(t *testing.T) {
	spec, standards := parseValidInputs(t, FormatYAML)
	got, diagnostics := resolve(spec, standards)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve() diagnostics = %+v", diagnostics)
	}
	want := resolved{
		AccountID:                       "123456789012",
		Region:                          "us-east-2",
		Module:                          "s3-private/v1",
		LogicalName:                     "orders-assets",
		Environment:                     "dev",
		Owner:                           "orders-team",
		NoncurrentVersionExpirationDays: 30,
		BucketName:                      "acme-orders-assets-dev-123456789012-us-east-2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolve() = %+v; want %+v", got, want)
	}
}

func TestResolveBucketNamePrefix(t *testing.T) {
	cases := []struct {
		name, prefix, want string
	}{
		{"with prefix", "acme", "acme-orders-assets-dev-123456789012-us-east-2"},
		{"without prefix", "", "orders-assets-dev-123456789012-us-east-2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, standards := parseValidInputs(t, FormatYAML)
			standards.Spec.Bucket.BucketNamePrefix = tc.prefix
			got, diagnostics := resolve(spec, standards)
			if len(diagnostics) != 0 {
				t.Fatalf("resolve() diagnostics = %+v", diagnostics)
			}
			if got.BucketName != tc.want {
				t.Fatalf("BucketName = %q; want %q", got.BucketName, tc.want)
			}
		})
	}
}

func TestResolveYAMLAndJSONAreIdentical(t *testing.T) {
	yamlSpec, yamlStandards := parseValidInputs(t, FormatYAML)
	jsonSpec, jsonStandards := parseValidInputs(t, FormatJSON)
	fromYAML, diagnostics := resolve(yamlSpec, yamlStandards)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve(YAML) diagnostics = %+v", diagnostics)
	}
	fromJSON, diagnostics := resolve(jsonSpec, jsonStandards)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve(JSON) diagnostics = %+v", diagnostics)
	}
	if !reflect.DeepEqual(fromYAML, fromJSON) {
		t.Fatalf("YAML and JSON resolve differently:\nYAML: %+v\nJSON: %+v", fromYAML, fromJSON)
	}
}

func TestGenerateRejectsInvalidInputWithoutFiles(t *testing.T) {
	cases := []struct {
		name, field string
		mutate      func(*Spec, *PlatformStandards)
	}{
		{"invalid spec field", "spec.owner", func(s *Spec, _ *PlatformStandards) { s.Spec.Owner = " " }},
		{"invalid standards field", "spec.aws.accountId", func(_ *Spec, p *PlatformStandards) { p.Spec.AWS.AccountID = "123" }},
		{"unsupported module", "spec.bucket.module", func(_ *Spec, p *PlatformStandards) { p.Spec.Bucket.Module = "s3-public/v1" }},
		{"non-positive expiration", "spec.bucket.noncurrentVersionExpirationDays", func(_ *Spec, p *PlatformStandards) { p.Spec.Bucket.NoncurrentVersionExpirationDays = 0 }},
		{"disallowed environment", "spec.environment", func(s *Spec, _ *PlatformStandards) { s.Spec.Environment = "staging" }},
		{"invalid composed bucket name", "metadata.name", func(s *Spec, p *PlatformStandards) {
			p.Spec.Bucket.BucketNamePrefix = ""
			s.Metadata.Name = strings.Repeat("a", 37)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, standards := parseValidInputs(t, FormatYAML)
			tc.mutate(&spec, &standards)

			files, diagnostics := Generate(spec, standards)
			if files != nil {
				t.Fatalf("Generate() files = %+v; want nil", files)
			}
			if !hasField(diagnostics, tc.field) {
				t.Fatalf("Generate() diagnostics = %+v; want field %q", diagnostics, tc.field)
			}
			if want := Validate(spec, standards); !reflect.DeepEqual(diagnostics, want) {
				t.Fatalf("Generate() diagnostics differ from Validate():\ngot:  %+v\nwant: %+v", diagnostics, want)
			}
			if got, _ := resolve(spec, standards); !reflect.DeepEqual(got, resolved{}) {
				t.Fatalf("resolve() = %+v on invalid input; want zero value", got)
			}
		})
	}
}

func TestGenerateAcceptsValidInput(t *testing.T) {
	for _, format := range []Format{FormatYAML, FormatJSON} {
		spec, standards := parseValidInputs(t, format)
		files, diagnostics := Generate(spec, standards)
		if len(diagnostics) != 0 {
			t.Fatalf("Generate() diagnostics = %+v", diagnostics)
		}
		if len(files) != len(rootFileOrder) {
			t.Fatalf("Generate() returned %d files; want %d", len(files), len(rootFileOrder))
		}
		for i, file := range files {
			if file.Path != rootFileOrder[i] || len(file.Content) == 0 {
				t.Fatalf("Generate() file %d = %q (%d bytes); want %q non-empty", i, file.Path, len(file.Content), rootFileOrder[i])
			}
		}
	}
}
