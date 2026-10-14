package core

import (
	"fmt"
	"strings"
)

// validateOverrides checks the whole blueprintOverrides block of the platform
// standards, not only the part the current spec selects, so a typo is caught
// even when nobody uses that tier yet. overrides may be nil.
func validateOverrides(overrides map[string]BlueprintOverrides, catalog catalogFile) []Diagnostic {
	var diagnostics []Diagnostic
	add := func(field, message string) {
		diagnostics = append(diagnostics, diagnostic(standardsSchema.name, field, message)...)
	}

	for _, id := range sortedKeys(overrides) {
		idField := "spec.blueprintOverrides." + id
		if _, supported := supportedBlueprints[id]; !supported {
			add(idField, fmt.Sprintf("%q is not a supported blueprint; use one of: %s.", id, strings.Join(sortedKeys(supportedBlueprints), ", ")))
			continue
		}
		entry := catalog.Blueprints[id]
		for _, tier := range sortedKeys(overrides[id].Tiers) {
			tierField := idField + ".tiers." + tier
			if _, ok := entry.Tiers[tier]; !ok {
				add(tierField, fmt.Sprintf("tier %q is not defined for blueprint %q; use one of: %s.", tier, id, strings.Join(sortedKeys(entry.Tiers), ", ")))
				continue
			}
			parameters := overrides[id].Tiers[tier].Parameters
			for _, name := range sortedKeys(parameters) {
				parameterField := tierField + ".parameters." + name
				schema, known := networkParameters[name]
				if !known {
					add(parameterField, fmt.Sprintf("unknown parameter; use one of: %s.", strings.Join(sortedKeys(networkParameters), ", ")))
					continue
				}
				validatePolicy(parameterField, schema, parameters[name], add)
			}
		}
	}
	return diagnostics
}

// validatePolicy checks one parameter policy against the built-in schema and
// reports each problem through add. Bounds are inclusive.
func validatePolicy(field string, schema parameterSchema, policy ParameterPolicy, add func(field, message string)) {
	bounds := map[string]*int{"minimum": policy.Minimum, "maximum": policy.Maximum}
	for _, name := range []string{"minimum", "maximum"} {
		bound := bounds[name]
		switch {
		case bound == nil:
		case schema.kind != integerParameter:
			add(field+"."+name, "is valid only for integer parameters such as az_count.")
		case *bound < schema.min || *bound > schema.max:
			add(field+"."+name, fmt.Sprintf("must be between %d and %d, the module's intrinsic bounds.", schema.min, schema.max))
		}
	}
	if policy.Minimum != nil && policy.Maximum != nil && *policy.Minimum > *policy.Maximum {
		add(field+".minimum", fmt.Sprintf("%d is greater than maximum %d.", *policy.Minimum, *policy.Maximum))
	}

	if policy.Allowed == nil {
		return
	}
	allowedField := field + ".allowed"
	if len(*policy.Allowed) == 0 {
		add(allowedField, "must list at least one value; remove it to allow any value.")
	}
	if policy.Minimum != nil || policy.Maximum != nil {
		add(allowedField, "cannot be combined with minimum or maximum; use one or the other.")
	}
	seen := make(map[string]struct{}, len(*policy.Allowed))
	for i, value := range *policy.Allowed {
		entryField := fmt.Sprintf("%s[%d]", allowedField, i)
		if problem := schema.check(value); problem != "" {
			add(entryField, problem)
		}
		if _, duplicate := seen[value]; duplicate {
			add(entryField, "duplicates an earlier allowed value; list each value once.")
		}
		seen[value] = struct{}{}
	}
}
