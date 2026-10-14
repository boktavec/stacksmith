package core

import (
	"reflect"
	"strings"
	"testing"
)

const policyStandardsYAML = validStandardsYAML + `  blueprintOverrides:
    network:
      tiers:
        dev:
          parameters:
            az_count:
              minimum: 2
              maximum: 3
            single_nat: {}
            cidr:
              allowed: ["10.10.0.0/16", "10.20.0.0/16"]
`

const policyStandardsJSON = `{"apiVersion":"stacksmith/v2alpha1","kind":"PlatformStandards","spec":{"aws":{"accountId":"123456789012","region":"us-east-2"},"environments":["dev","prod"],"blueprintOverrides":{"network":{"tiers":{"dev":{"parameters":{"az_count":{"minimum":2,"maximum":3},"single_nat":{},"cidr":{"allowed":["10.10.0.0/16","10.20.0.0/16"]}}}}}}}}`

func intPtr(n int) *int { return &n }

func listPtr(values ...string) *[]string { return &values }

// devPolicies returns standards whose dev tier carries exactly the given policies.
func devPolicies(t *testing.T, policies map[string]ParameterPolicy) PlatformStandards {
	t.Helper()
	_, standards := validSpecAndStandards(t)
	standards.Spec.BlueprintOverrides = map[string]BlueprintOverrides{
		"network": {Tiers: map[string]TierOverrides{"dev": {Parameters: policies}}},
	}
	return standards
}

// anyValue permits any canonical in-bounds value for every network parameter.
func anyValue() map[string]ParameterPolicy {
	return map[string]ParameterPolicy{"az_count": {}, "cidr": {}, "single_nat": {}}
}

func specWithParameters(t *testing.T, tier string, parameters map[string]string) Spec {
	t.Helper()
	spec, _ := validSpecAndStandards(t)
	spec.Spec.Tier = tier
	spec.Spec.Parameters = parameters
	return spec
}

func networkEntry(t *testing.T) catalogEntry {
	t.Helper()
	return mustLoadCatalog().Blueprints["network"]
}

func onlyDiagnostic(t *testing.T, diagnostics []Diagnostic, document, field, message string) {
	t.Helper()
	if len(diagnostics) != 1 || diagnostics[0].Document != document || diagnostics[0].Field != field || !strings.Contains(diagnostics[0].Message, message) {
		t.Fatalf("diagnostics = %+v; want exactly one on %s %q containing %q", diagnostics, document, field, message)
	}
}

func TestPolicyYAMLAndJSONDecodeIdentically(t *testing.T) {
	fromYAML, diagnostics := ParseStandards([]byte(policyStandardsYAML), FormatYAML)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	fromJSON, diagnostics := ParseStandards([]byte(policyStandardsJSON), FormatJSON)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if !reflect.DeepEqual(fromYAML, fromJSON) {
		t.Fatalf("YAML and JSON differ:\nYAML: %+v\nJSON: %+v", fromYAML, fromJSON)
	}
	az := fromYAML.Spec.BlueprintOverrides["network"].Tiers["dev"].Parameters["az_count"]
	if az.Minimum == nil || *az.Minimum != 2 || az.Maximum == nil || *az.Maximum != 3 || az.Allowed != nil {
		t.Fatalf("az_count policy = %+v", az)
	}
}

// yaml.v3 and encoding/json must both leave an absent `allowed` nil but give an
// explicit empty list a non-nil pointer, so the empty list can be rejected.
func TestPolicyEmptyAllowedDiffersFromAbsent(t *testing.T) {
	const yamlPrefix = validStandardsYAML + "  blueprintOverrides:\n    network:\n      tiers:\n        dev:\n          parameters:\n            cidr:\n"
	jsonPrefix := strings.TrimSuffix(validStandardsJSON, `}}`) + `,"blueprintOverrides":{"network":{"tiers":{"dev":{"parameters":{"cidr":`
	cases := map[string]struct {
		yaml, json string
		wantNil    bool
	}{
		"absent": {yamlPrefix + "              {}\n", jsonPrefix + `{}}}}}}}}`, true},
		"empty":  {yamlPrefix + "              allowed: []\n", jsonPrefix + `{"allowed":[]}}}}}}}}`, false},
	}
	for name, tc := range cases {
		for format, input := range map[Format]string{FormatYAML: tc.yaml, FormatJSON: tc.json} {
			t.Run(name+"/"+formatName(format), func(t *testing.T) {
				standards, diagnostics := ParseStandards([]byte(input), format)
				if len(diagnostics) != 0 {
					t.Fatal(diagnostics)
				}
				allowed := standards.Spec.BlueprintOverrides["network"].Tiers["dev"].Parameters["cidr"].Allowed
				if (allowed == nil) != tc.wantNil || (allowed != nil && len(*allowed) != 0) {
					t.Fatalf("Allowed = %v, want nil=%v", allowed, tc.wantNil)
				}
			})
		}
	}
}

func TestParseStandardsPolicyIntegerFormats(t *testing.T) {
	yamlWith := func(bound string) string {
		return validStandardsYAML + "  blueprintOverrides:\n    network:\n      tiers:\n        dev:\n          parameters:\n            az_count:\n              minimum: " + bound + "\n"
	}
	jsonWith := func(bound string) string {
		return strings.TrimSuffix(validStandardsJSON, `}}`) + `,"blueprintOverrides":{"network":{"tiers":{"dev":{"parameters":{"az_count":{"minimum":` + bound + `}}}}}}}}`
	}
	const field = "spec.blueprintOverrides.network.tiers.dev.parameters.az_count.minimum"

	for _, bound := range []string{"2", "0", "-1"} {
		for format, input := range map[Format]string{FormatYAML: yamlWith(bound), FormatJSON: jsonWith(bound)} {
			if _, diagnostics := ParseStandards([]byte(input), format); len(diagnostics) != 0 {
				t.Errorf("%s %s: diagnostics = %+v", formatName(format), bound, diagnostics)
			}
		}
	}

	// yaml.v3 resolves several of these as !!int (0x2, 1_0, +2, and 010 as octal
	// 8) or !!float, so the parser must refuse them by spelling.
	rejected := map[Format][]string{
		FormatYAML: {"2.0", "0x2", "0o7", "010", "+2", "1_0", `"2"`, "true", "99999999999999999999", "[2]"},
		FormatJSON: {"2.0", "2e0", "2E0", "-0.0", `"2"`, "true", "99999999999999999999", "[2]"},
	}
	for format, bounds := range rejected {
		for _, bound := range bounds {
			input := yamlWith(bound)
			if format == FormatJSON {
				input = jsonWith(bound)
			}
			t.Run(formatName(format)+"/"+bound, func(t *testing.T) {
				_, diagnostics := ParseStandards([]byte(input), format)
				if len(diagnostics) != 1 || diagnostics[0].Field != field || !strings.Contains(diagnostics[0].Message, "expected") {
					t.Fatalf("diagnostics = %+v; want one \"expected ...\" on %s", diagnostics, field)
				}
			})
		}
	}
}

func TestParseAcceptsDottedDynamicKeys(t *testing.T) {
	spec, diagnostics := ParseSpec([]byte(validSpecYAML+"  parameters:\n    a.b: \"1\"\n"), FormatYAML)
	if len(diagnostics) != 0 || spec.Spec.Parameters["a.b"] != "1" {
		t.Fatalf("spec = %+v diagnostics = %+v", spec, diagnostics)
	}
	// The key contains dots, yet the nested value is still checked as a string.
	_, diagnostics = ParseSpec([]byte(validSpecYAML+"  parameters:\n    a.b: 1\n"), FormatYAML)
	if len(diagnostics) != 1 || diagnostics[0].Field != "spec.parameters.a.b" || !strings.Contains(diagnostics[0].Message, "expected a string") {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestResolveParametersWithoutOverridesEqualsPreset(t *testing.T) {
	want := map[string]map[string]string{
		"dev":  {"cidr": "10.10.0.0/16", "az_count": "2", "single_nat": "true"},
		"prod": {"cidr": "10.0.0.0/16", "az_count": "3", "single_nat": "false"},
	}
	_, standards := validSpecAndStandards(t) // no policies at all
	for tier, preset := range want {
		for name, parameters := range map[string]map[string]string{"absent": nil, "empty": {}} {
			t.Run(tier+"/"+name, func(t *testing.T) {
				got, diagnostics := resolveParameters(specWithParameters(t, tier, parameters), standards, networkEntry(t))
				if len(diagnostics) != 0 || !reflect.DeepEqual(got, preset) {
					t.Fatalf("got %v diagnostics %+v; want %v", got, diagnostics, preset)
				}
			})
		}
	}
}

func TestResolveParametersMergesOverridesOntoPreset(t *testing.T) {
	standards := devPolicies(t, map[string]ParameterPolicy{"az_count": {Minimum: intPtr(2), Maximum: intPtr(3)}})
	entry := networkEntry(t)
	got, diagnostics := resolveParameters(specWithParameters(t, "dev", map[string]string{"az_count": "3"}), standards, entry)
	want := map[string]string{"cidr": "10.10.0.0/16", "az_count": "3", "single_nat": "true"}
	if len(diagnostics) != 0 || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v diagnostics %+v; want %v", got, diagnostics, want)
	}
	if entry.Tiers["dev"]["az_count"] != "2" {
		t.Fatal("resolveParameters mutated the catalog preset")
	}
}

func TestOverrideAcceptsBoundaryValues(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"az_count", "1"}, {"az_count", "8"},
		{"cidr", "10.0.0.0/16"}, {"cidr", "10.0.0.0/24"}, {"cidr", "192.168.4.0/24"},
		{"single_nat", "true"}, {"single_nat", "false"},
	} {
		spec := specWithParameters(t, "dev", map[string]string{tc.name: tc.value})
		if diagnostics := Validate(spec, devPolicies(t, anyValue())); len(diagnostics) != 0 {
			t.Errorf("%s=%s rejected: %+v", tc.name, tc.value, diagnostics)
		}
	}
}

func TestOverrideRejectsInvalidValues(t *testing.T) {
	cases := []struct{ parameter, value, message string }{
		{"az_count", "", "must not be empty"},
		{"cidr", "", "must not be empty"},
		{"single_nat", "", "must not be empty"},
		{"az_count", "03", "canonical decimal integer"},
		{"az_count", "+3", "canonical decimal integer"},
		{"az_count", "-1", "canonical decimal integer"},
		{"az_count", "3.0", "canonical decimal integer"},
		{"az_count", "٣", "canonical decimal integer"}, // Arabic-Indic digit three
		{"az_count", "99999999999999999999", "canonical decimal integer"},
		{"az_count", "0", "between 1 and 8"},
		{"az_count", "9", "between 1 and 8"},
		{"cidr", "010.0.0.0/16", "canonical IPv4 CIDR"},
		{"cidr", "10.0.0.1/16", "canonical IPv4 CIDR"},
		{"cidr", "10.0.0.0", "canonical IPv4 CIDR"},
		{"cidr", "10.0.0.0/016", "canonical IPv4 CIDR"},
		{"cidr", "fd00::/16", "canonical IPv4 CIDR"},
		{"cidr", "::ffff:10.0.0.0/112", "canonical IPv4 CIDR"},
		{"cidr", "10.0.0.0/15", "prefix length must be between /16 and /24"},
		{"cidr", "10.0.0.0/25", "prefix length must be between /16 and /24"},
		{"single_nat", "True", "exactly"},
		{"single_nat", "1", "exactly"},
		{"single_nat", "yes", "exactly"},
	}
	for _, tc := range cases {
		t.Run(tc.parameter+"="+tc.value, func(t *testing.T) {
			spec := specWithParameters(t, "dev", map[string]string{tc.parameter: tc.value})
			diagnostics := Validate(spec, devPolicies(t, anyValue()))
			onlyDiagnostic(t, diagnostics, "blueprint spec", "spec.parameters."+tc.parameter, tc.message)
		})
	}
}

func TestOverrideRejectsHostileStringsForEveryParameter(t *testing.T) {
	hostile := []string{"${aws_iam_role.x.arn}", "%{if true}x%{endif}", `"quoted"`, "line1\nline2", "nul\x00byte", "a\\b", "10.0.0.0/16\n"}
	for parameter := range networkParameters {
		for _, value := range hostile {
			spec := specWithParameters(t, "dev", map[string]string{parameter: value})
			diagnostics := Validate(spec, devPolicies(t, anyValue()))
			if len(diagnostics) != 1 || diagnostics[0].Field != "spec.parameters."+parameter {
				t.Errorf("%s=%q: diagnostics = %+v", parameter, value, diagnostics)
				continue
			}
			if strings.Contains(diagnostics[0].Message, value) {
				t.Errorf("%s=%q: diagnostic echoes the hostile value: %q", parameter, value, diagnostics[0].Message)
			}
		}
	}
}

func TestOverrideRespectsPolicy(t *testing.T) {
	cases := []struct {
		name       string
		policies   map[string]ParameterPolicy
		tier       string
		parameters map[string]string
		field      string
		message    string
	}{
		{"unknown parameter", anyValue(), "dev", map[string]string{"instance_class": "x"}, "spec.parameters.instance_class", "unknown parameter; use one of: az_count, cidr, single_nat."},
		{"no policy at all for parameter", map[string]ParameterPolicy{"cidr": {}}, "dev", map[string]string{"az_count": "2"}, "spec.parameters.az_count", "no override policy"},
		{"value equal to preset still needs policy", map[string]ParameterPolicy{}, "dev", map[string]string{"az_count": "2"}, "spec.parameters.az_count", "no override policy"},
		{"no policy for tier", anyValue(), "prod", map[string]string{"az_count": "2"}, "spec.parameters.az_count", "no override policy"},
		{"below minimum", map[string]ParameterPolicy{"az_count": {Minimum: intPtr(2)}}, "dev", map[string]string{"az_count": "1"}, "spec.parameters.az_count", "below the platform minimum of 2"},
		{"above maximum", map[string]ParameterPolicy{"az_count": {Maximum: intPtr(3)}}, "dev", map[string]string{"az_count": "4"}, "spec.parameters.az_count", "above the platform maximum of 3"},
		{"not in allowed", map[string]ParameterPolicy{"cidr": {Allowed: listPtr("10.10.0.0/16")}}, "dev", map[string]string{"cidr": "10.20.0.0/16"}, "spec.parameters.cidr", "use one of: 10.10.0.0/16"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diagnostics := Validate(specWithParameters(t, tc.tier, tc.parameters), devPolicies(t, tc.policies))
			onlyDiagnostic(t, diagnostics, "blueprint spec", tc.field, tc.message)
		})
	}

	// Inclusive bounds: the policy limits themselves are accepted.
	for _, value := range []string{"2", "3"} {
		standards := devPolicies(t, map[string]ParameterPolicy{"az_count": {Minimum: intPtr(2), Maximum: intPtr(3)}})
		if diagnostics := Validate(specWithParameters(t, "dev", map[string]string{"az_count": value}), standards); len(diagnostics) != 0 {
			t.Errorf("az_count=%s rejected: %+v", value, diagnostics)
		}
	}
}

func TestValidateRejectsInvalidPolicyDefinitions(t *testing.T) {
	const prefix = "spec.blueprintOverrides.network.tiers.dev.parameters."
	cases := []struct {
		name     string
		policies map[string]ParameterPolicy
		field    string
		message  string
	}{
		{"unknown parameter", map[string]ParameterPolicy{"instance_class": {}}, prefix + "instance_class", "unknown parameter"},
		{"minimum on cidr", map[string]ParameterPolicy{"cidr": {Minimum: intPtr(16)}}, prefix + "cidr.minimum", "only for integer parameters"},
		{"maximum on boolean", map[string]ParameterPolicy{"single_nat": {Maximum: intPtr(1)}}, prefix + "single_nat.maximum", "only for integer parameters"},
		{"minimum above maximum", map[string]ParameterPolicy{"az_count": {Minimum: intPtr(4), Maximum: intPtr(3)}}, prefix + "az_count.minimum", "4 is greater than maximum 3"},
		{"minimum below bounds", map[string]ParameterPolicy{"az_count": {Minimum: intPtr(0)}}, prefix + "az_count.minimum", "between 1 and 8"},
		{"maximum above bounds", map[string]ParameterPolicy{"az_count": {Maximum: intPtr(9)}}, prefix + "az_count.maximum", "between 1 and 8"},
		{"empty allowed", map[string]ParameterPolicy{"cidr": {Allowed: listPtr()}}, prefix + "cidr.allowed", "at least one value"},
		{"non-canonical allowed entry", map[string]ParameterPolicy{"cidr": {Allowed: listPtr("10.0.0.0/16", "010.0.0.0/16")}}, prefix + "cidr.allowed[1]", "canonical IPv4 CIDR"},
		{"out-of-bounds allowed entry", map[string]ParameterPolicy{"az_count": {Allowed: listPtr("9")}}, prefix + "az_count.allowed[0]", "between 1 and 8"},
		{"duplicate allowed entry", map[string]ParameterPolicy{"single_nat": {Allowed: listPtr("true", "true")}}, prefix + "single_nat.allowed[1]", "duplicates"},
		{"allowed with minimum", map[string]ParameterPolicy{"az_count": {Minimum: intPtr(2), Allowed: listPtr("2")}}, prefix + "az_count.allowed", "cannot be combined"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, _ := validSpecAndStandards(t)
			onlyDiagnostic(t, Validate(spec, devPolicies(t, tc.policies)), "platform standards", tc.field, tc.message)
		})
	}

	structural := []struct {
		name      string
		overrides map[string]BlueprintOverrides
		field     string
		message   string
	}{
		{"unsupported blueprint", map[string]BlueprintOverrides{"rds": {}}, "spec.blueprintOverrides.rds", `"rds" is not a supported blueprint; use one of: network.`},
		{"unknown blueprint", map[string]BlueprintOverrides{"netwrok": {}}, "spec.blueprintOverrides.netwrok", "not a supported blueprint"},
		{"unknown tier", map[string]BlueprintOverrides{"network": {Tiers: map[string]TierOverrides{"staging": {}}}}, "spec.blueprintOverrides.network.tiers.staging", "use one of: dev, prod."},
	}
	for _, tc := range structural {
		t.Run(tc.name, func(t *testing.T) {
			spec, standards := validSpecAndStandards(t)
			standards.Spec.BlueprintOverrides = tc.overrides
			onlyDiagnostic(t, Validate(spec, standards), "platform standards", tc.field, tc.message)
		})
	}
}

// The whole blueprintOverrides block is checked, including tiers the spec does not use.
func TestValidateChecksPolicyForUnselectedTier(t *testing.T) {
	spec, standards := validSpecAndStandards(t)
	standards.Spec.BlueprintOverrides = map[string]BlueprintOverrides{"network": {Tiers: map[string]TierOverrides{
		"prod": {Parameters: map[string]ParameterPolicy{"az_count": {Maximum: intPtr(99)}}},
	}}}
	onlyDiagnostic(t, Validate(spec, standards), "platform standards", "spec.blueprintOverrides.network.tiers.prod.parameters.az_count.maximum", "between 1 and 8")
}

func TestEmptyOverrideContainersEqualAbsent(t *testing.T) {
	for name, overrides := range map[string]map[string]BlueprintOverrides{
		"empty map":        {},
		"empty blueprint":  {"network": {}},
		"empty tier":       {"network": {Tiers: map[string]TierOverrides{"dev": {}}}},
		"empty parameters": {"network": {Tiers: map[string]TierOverrides{"dev": {Parameters: map[string]ParameterPolicy{}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			spec, standards := validSpecAndStandards(t)
			standards.Spec.BlueprintOverrides = overrides
			if diagnostics := Validate(spec, standards); len(diagnostics) != 0 {
				t.Fatalf("diagnostics = %+v", diagnostics)
			}
			spec.Spec.Parameters = map[string]string{"az_count": "3"}
			onlyDiagnostic(t, Validate(spec, standards), "blueprint spec", "spec.parameters.az_count", "no override policy")
		})
	}
}

func TestPolicyMinimumOrMaximumAloneIsValid(t *testing.T) {
	for _, policy := range []ParameterPolicy{{Minimum: intPtr(2)}, {Maximum: intPtr(3)}, {Minimum: intPtr(3), Maximum: intPtr(3)}} {
		spec, _ := validSpecAndStandards(t)
		if diagnostics := Validate(spec, devPolicies(t, map[string]ParameterPolicy{"az_count": policy})); len(diagnostics) != 0 {
			t.Errorf("policy %+v rejected: %+v", policy, diagnostics)
		}
	}
}

func TestResolveParametersRejectsDefectivePreset(t *testing.T) {
	good := networkEntry(t).Tiers["dev"]
	mutate := func(change func(map[string]string)) catalogEntry {
		preset := map[string]string{}
		for k, v := range good {
			preset[k] = v
		}
		change(preset)
		return catalogEntry{Tiers: map[string]map[string]string{"dev": preset}}
	}
	cases := []struct {
		name    string
		entry   catalogEntry
		field   string
		message string
	}{
		{"non-canonical cidr", mutate(func(p map[string]string) { p["cidr"] = "10.10.0.1/16" }), "blueprints.network.tiers.dev.cidr", "preset must be a canonical IPv4 CIDR"},
		{"out-of-bounds az_count", mutate(func(p map[string]string) { p["az_count"] = "9" }), "blueprints.network.tiers.dev.az_count", "preset must be between 1 and 8"},
		{"bad boolean", mutate(func(p map[string]string) { p["single_nat"] = "True" }), "blueprints.network.tiers.dev.single_nat", "preset must be exactly"},
		{"missing key", mutate(func(p map[string]string) { delete(p, "cidr") }), "blueprints.network.tiers.dev.cidr", "missing this parameter"},
		{"empty value", mutate(func(p map[string]string) { p["cidr"] = "" }), "blueprints.network.tiers.dev.cidr", "preset must be a canonical IPv4 CIDR"},
		{"extra key", mutate(func(p map[string]string) { p["instance_class"] = "x" }), "blueprints.network.tiers.dev.instance_class", "does not define"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, standards := validSpecAndStandards(t)
			got, diagnostics := resolveParameters(spec, standards, tc.entry)
			if got != nil {
				t.Fatalf("defective preset produced a map: %v", got)
			}
			onlyDiagnostic(t, diagnostics, "catalog snapshot", tc.field, tc.message)
		})
	}
}

// A preset is never checked against policy, even a restrictive one.
func TestPresetIsNotCheckedAgainstPolicy(t *testing.T) {
	standards := devPolicies(t, map[string]ParameterPolicy{"az_count": {Minimum: intPtr(3)}}) // dev preset is 2
	if diagnostics := Validate(specWithParameters(t, "dev", nil), standards); len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestParameterDiagnosticsAreSortedAndComplete(t *testing.T) {
	spec := specWithParameters(t, "dev", map[string]string{"single_nat": "x", "az_count": "0", "cidr": "bad", "zzz": "1", "aaa": "1"})
	var want = []string{"spec.parameters.aaa", "spec.parameters.az_count", "spec.parameters.cidr", "spec.parameters.single_nat", "spec.parameters.zzz"}
	for range 3 {
		var got []string
		for _, d := range Validate(spec, devPolicies(t, anyValue())) {
			got = append(got, d.Field)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("fields = %v, want %v", got, want)
		}
	}
}

func TestValidateAcceptsExampleOverridesInBothFormats(t *testing.T) {
	standards, diagnostics := ParseStandards([]byte(policyStandardsYAML), FormatYAML)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	fromYAML, _ := ParseSpec([]byte(validSpecYAML+"  parameters:\n    az_count: \"3\"\n    cidr: \"10.20.0.0/16\"\n"), FormatYAML)
	fromJSON, _ := ParseSpec([]byte(strings.Replace(validSpecJSON, `"tier":"dev"`, `"tier":"dev","parameters":{"az_count":"3","cidr":"10.20.0.0/16"}`, 1)), FormatJSON)
	if !reflect.DeepEqual(fromYAML, fromJSON) {
		t.Fatalf("specs differ: %+v vs %+v", fromYAML, fromJSON)
	}
	a, da := resolveParameters(fromYAML, standards, networkEntry(t))
	b, db := resolveParameters(fromJSON, standards, networkEntry(t))
	want := map[string]string{"cidr": "10.20.0.0/16", "az_count": "3", "single_nat": "true"}
	if len(da)+len(db) != 0 || !reflect.DeepEqual(a, want) || !reflect.DeepEqual(a, b) {
		t.Fatalf("a=%v %+v b=%v %+v", a, da, b, db)
	}
}

func TestOverrideDiagnosticDoesNotEchoHostileAllowedEntry(t *testing.T) {
	const hostile = "${evil}\x1b[31m"
	standards := devPolicies(t, map[string]ParameterPolicy{"cidr": {Allowed: listPtr("10.10.0.0/16", hostile)}})
	diagnostics := Validate(specWithParameters(t, "dev", map[string]string{"cidr": "10.20.0.0/16"}), standards)
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics = %+v; want the policy error and the override error", diagnostics)
	}
	for _, d := range diagnostics {
		if d.Document != "blueprint spec" {
			continue
		}
		if strings.Contains(d.Message, "evil") || strings.Contains(d.Message, "\x1b") {
			t.Fatalf("spec diagnostic echoes the hostile entry: %q", d.Message)
		}
		if !strings.Contains(d.Message, "10.10.0.0/16") {
			t.Fatalf("well-formed entry missing from %q", d.Message)
		}
	}
}

func TestParseSpecNULParameterValueReachesValidationAndIsRejected(t *testing.T) {
	inputs := map[Format]string{
		FormatYAML: validSpecYAML + "  parameters:\n    cidr: \"10.0.0.0/16\\0\"\n",
		FormatJSON: strings.Replace(validSpecJSON, `"tier":"dev"`, `"tier":"dev","parameters":{"cidr":"10.0.0.0/16\u0000"}`, 1),
	}
	for format, input := range inputs {
		t.Run(formatName(format), func(t *testing.T) {
			spec, diagnostics := ParseSpec([]byte(input), format)
			if len(diagnostics) != 0 || !strings.Contains(spec.Spec.Parameters["cidr"], "\x00") {
				t.Fatalf("spec = %+v diagnostics = %+v; want a parsed NUL byte", spec, diagnostics)
			}
			onlyDiagnostic(t, Validate(spec, devPolicies(t, anyValue())), "blueprint spec", "spec.parameters.cidr", "canonical IPv4 CIDR")
		})
	}
}

func TestPolicyAllowedWithMaximumIsRejected(t *testing.T) {
	spec, _ := validSpecAndStandards(t)
	standards := devPolicies(t, map[string]ParameterPolicy{"az_count": {Maximum: intPtr(3), Allowed: listPtr("2")}})
	onlyDiagnostic(t, Validate(spec, standards), "platform standards", "spec.blueprintOverrides.network.tiers.dev.parameters.az_count.allowed", "cannot be combined")
}
