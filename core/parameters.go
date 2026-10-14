package core

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// catalogSnapshotDocument names the embedded catalog in diagnostics about a
// defective preset (a build defect, not a user mistake).
const catalogSnapshotDocument = "catalog snapshot"

type parameterKind uint8

const (
	integerParameter parameterKind = iota + 1
	cidrParameter
	booleanParameter
)

// parameterSchema is StackSmith's built-in definition of one blueprint
// parameter. Standards can constrain a parameter but never add or retype one.
type parameterSchema struct {
	kind parameterKind
	// min and max are the module-intrinsic bounds: az_count's range, or the
	// allowed IPv4 prefix length for a CIDR. They apply to every value,
	// including catalog presets and whatever a policy permits.
	min, max int
}

// networkParameters is the schema for the supported "network" blueprint. The
// bounds come from the module's cidrsubnet(var.cidr, 4, i + 8) subnet carving
// and AWS VPC limits.
var networkParameters = map[string]parameterSchema{
	"az_count":   {kind: integerParameter, min: 1, max: 8},
	"cidr":       {kind: cidrParameter, min: 16, max: 24},
	"single_nat": {kind: booleanParameter},
}

// check returns "" when value is in canonical form and within the intrinsic
// bounds, otherwise a message saying how to fix it. Valid but non-canonical
// values (03, True, 10.0.0.1/16) are rejected rather than normalized, which
// also guarantees the value is a safe literal to place in generated Terraform.
// Messages never echo the value, so hostile input cannot reach a diagnostic.
func (p parameterSchema) check(value string) string {
	switch p.kind {
	case integerParameter:
		n, ok := canonicalInteger(value)
		if !ok {
			return `must be a canonical decimal integer such as "3" (digits only, no sign or leading zero).`
		}
		if n < p.min || n > p.max {
			return fmt.Sprintf("must be between %d and %d.", p.min, p.max)
		}
	case cidrParameter:
		prefix, err := netip.ParsePrefix(value)
		// Masked().String() is the canonical spelling; comparing it with the input
		// rejects host bits, leading zeros and any other alternate form at once.
		if err != nil || !prefix.Addr().Is4() || prefix.Masked().String() != value {
			return `must be a canonical IPv4 CIDR such as "10.0.0.0/16" (no leading zeros, host bits zero).`
		}
		if prefix.Bits() < p.min || prefix.Bits() > p.max {
			return fmt.Sprintf("prefix length must be between /%d and /%d.", p.min, p.max)
		}
	case booleanParameter:
		if value != "true" && value != "false" {
			return `must be exactly "true" or "false".`
		}
	}
	return ""
}

// canonicalInteger parses ASCII digits with no sign and no leading zero.
func canonicalInteger(value string) (int, bool) {
	if value == "" || (len(value) > 1 && value[0] == '0') {
		return 0, false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(value) // fails only on overflow here
	return n, err == nil
}

// resolveParameters returns the tier preset merged with the developer's
// validated overrides: the complete parameter map for rendering. Any
// diagnostic means the map is nil. spec.Spec.Tier must exist in entry; Validate
// checks that first.
//
// A preset is checked against the schema only; policy constrains overrides
// only, so a value equal to the preset still needs a policy to be overridden.
func resolveParameters(spec Spec, standards PlatformStandards, entry catalogEntry) (map[string]string, []Diagnostic) {
	id, tier := spec.Spec.Blueprint, spec.Spec.Tier
	var diagnostics []Diagnostic

	preset := entry.Tiers[tier]
	for _, name := range sortedKeys(networkParameters) {
		field := fmt.Sprintf("blueprints.%s.tiers.%s.%s", id, tier, name)
		value, ok := preset[name]
		if !ok {
			diagnostics = append(diagnostics, diagnostic(catalogSnapshotDocument, field, "preset is missing this parameter; refresh or fix the snapshot.")...)
		} else if problem := networkParameters[name].check(value); problem != "" {
			diagnostics = append(diagnostics, diagnostic(catalogSnapshotDocument, field, "preset "+problem)...)
		}
	}
	for _, name := range sortedKeys(preset) {
		if _, ok := networkParameters[name]; !ok {
			field := fmt.Sprintf("blueprints.%s.tiers.%s.%s", id, tier, name)
			diagnostics = append(diagnostics, diagnostic(catalogSnapshotDocument, field, "preset has a parameter StackSmith does not define; refresh or fix the snapshot.")...)
		}
	}

	policies := standards.Spec.BlueprintOverrides[id].Tiers[tier].Parameters // nil maps read as empty
	resolved := make(map[string]string, len(preset))
	for name, value := range preset {
		resolved[name] = value
	}
	for _, name := range sortedKeys(spec.Spec.Parameters) {
		value := spec.Spec.Parameters[name]
		field := "spec.parameters." + name
		if problem := checkOverride(name, value, id, tier, policies); problem != "" {
			diagnostics = append(diagnostics, diagnostic(specSchema.name, field, problem)...)
			continue
		}
		resolved[name] = value
	}

	if len(diagnostics) != 0 {
		return nil, diagnostics
	}
	return resolved, nil
}

// checkOverride returns "" when the developer may set name to value, otherwise
// the reason. There is no fallback to a default when an override is denied.
func checkOverride(name, value, blueprint, tier string, policies map[string]ParameterPolicy) string {
	schema, known := networkParameters[name]
	if !known {
		return fmt.Sprintf("unknown parameter; use one of: %s.", strings.Join(sortedKeys(networkParameters), ", "))
	}
	if value == "" {
		return "must not be empty; remove the entry to use the tier preset."
	}
	policy, allowed := policies[name]
	if !allowed {
		return fmt.Sprintf("cannot be overridden for blueprint %q tier %q; platform standards define no override policy for it.", blueprint, tier)
	}
	if problem := schema.check(value); problem != "" {
		return problem
	}
	if n, err := strconv.Atoi(value); err == nil && schema.kind == integerParameter {
		if policy.Minimum != nil && n < *policy.Minimum {
			return fmt.Sprintf("is below the platform minimum of %d.", *policy.Minimum)
		}
		if policy.Maximum != nil && n > *policy.Maximum {
			return fmt.Sprintf("is above the platform maximum of %d.", *policy.Maximum)
		}
	}
	if policy.Allowed != nil && !contains(*policy.Allowed, value) {
		// Policy entries are user-supplied; list only well-formed ones so a
		// malformed entry (already reported as a policy error) is never echoed.
		var usable []string
		for _, entry := range *policy.Allowed {
			if schema.check(entry) == "" {
				usable = append(usable, entry)
			}
		}
		if len(usable) == 0 {
			return "is not allowed by platform standards."
		}
		return fmt.Sprintf("is not allowed by platform standards; use one of: %s.", strings.Join(usable, ", "))
	}
	return ""
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
