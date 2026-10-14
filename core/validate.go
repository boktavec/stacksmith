package core

import (
	"fmt"
	"sort"
	"strings"
)

// maxNameBytes is the longest metadata.name accepted. A slug is ASCII, so bytes
// equal characters.
const maxNameBytes = 63

const slugMessage = "must be a lowercase ASCII slug; use letters and digits separated by single hyphens."

// Validate checks spec against standards and the embedded catalog using the
// semantic rules from docs/spec/features/blueprint-catalog/SPEC.md section 4.
// It assumes both inputs already parsed cleanly (see ParseSpec,
// ParseStandards). An empty result means spec is valid against standards.
//
// Diagnostics are collected in a fixed order. Boolean "ok" gates skip only the
// checks that depend on an earlier check passing, so every independent problem
// is still reported in one run.
func Validate(spec Spec, standards PlatformStandards) []Diagnostic {
	var diagnostics []Diagnostic
	add := func(document, field, message string) {
		diagnostics = append(diagnostics, Diagnostic{Document: document, Field: field, Message: message})
	}

	if !validSlug(spec.Metadata.Name) {
		add(specSchema.name, "metadata.name", slugMessage)
	}
	if len(spec.Metadata.Name) > maxNameBytes {
		add(specSchema.name, "metadata.name", fmt.Sprintf("exceeds the %d-character limit; shorten the name.", maxNameBytes))
	}
	environmentOK := validSlug(spec.Spec.Environment)
	if !environmentOK {
		add(specSchema.name, "spec.environment", slugMessage)
	}

	if len(standards.Spec.Environments) == 0 {
		add(standardsSchema.name, "spec.environments", "must contain at least one allowed environment.")
	}
	// A set: map[string]struct{} stores keys only (struct{} takes no space).
	seenEnvironments := make(map[string]struct{}, len(standards.Spec.Environments))
	for i, environment := range standards.Spec.Environments {
		field := fmt.Sprintf("spec.environments[%d]", i)
		if !validSlug(environment) {
			add(standardsSchema.name, field, slugMessage)
		}
		if _, exists := seenEnvironments[environment]; exists {
			add(standardsSchema.name, field, fmt.Sprintf("duplicates environment %q; keep each allowed environment once.", environment))
		} else {
			seenEnvironments[environment] = struct{}{}
		}
	}
	if environmentOK {
		if _, allowed := seenEnvironments[spec.Spec.Environment]; !allowed {
			add(specSchema.name, "spec.environment", fmt.Sprintf("environment %q is not allowed; choose one listed in platform standards.", spec.Spec.Environment))
		}
	}

	if !validAccountID(standards.Spec.AWS.AccountID) {
		add(standardsSchema.name, "spec.aws.accountId", "must be exactly 12 ASCII digits; quote the account ID to preserve leading zeros.")
	}
	if !validRegion(standards.Spec.AWS.Region) {
		add(standardsSchema.name, "spec.aws.region", "must use AWS region-code syntax such as us-east-2; availability is not checked offline.")
	}

	catalog := mustLoadCatalog()
	id := spec.Spec.Blueprint
	entry, inCatalog := catalog.Blueprints[id]
	_, supported := supportedBlueprints[id]
	switch {
	case !inCatalog:
		add(specSchema.name, "spec.blueprint", fmt.Sprintf("unknown blueprint %q; run \"stacksmith catalog list\" to see available blueprints.", id))
	case !supported:
		add(specSchema.name, "spec.blueprint", fmt.Sprintf("blueprint %q is in the catalog but not supported by this StackSmith version; supported: %s.", id, strings.Join(sortedKeys(supportedBlueprints), ", ")))
	default:
		// The tier is meaningful only for a blueprint we can resolve.
		if _, ok := entry.Tiers[spec.Spec.Tier]; !ok {
			add(specSchema.name, "spec.tier", fmt.Sprintf("tier %q is not defined for blueprint %q; use one of: %s.", spec.Spec.Tier, id, strings.Join(sortedKeys(entry.Tiers), ", ")))
		}
	}

	return diagnostics
}

// sortedKeys returns the map's keys in order. Map iteration order is random in
// Go, so anything shown to a user must be sorted first.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func validSlug(value string) bool {
	if value == "" {
		return false
	}
	previousHyphen := false
	for i, r := range value {
		if r == '-' {
			if i == 0 || i == len(value)-1 || previousHyphen {
				return false
			}
			previousHyphen = true
			continue
		}
		if !asciiLowerOrDigit(r) {
			return false
		}
		previousHyphen = false
	}
	return true
}

func asciiLowerOrDigit(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
}

func validAccountID(value string) bool {
	if len(value) != 12 {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func validRegion(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) < 3 || len(parts[0]) < 2 || !asciiLetters(parts[0]) || !asciiDigits(parts[len(parts)-1]) {
		return false
	}
	for _, part := range parts[1 : len(parts)-1] {
		if part == "" {
			return false
		}
		for _, r := range part {
			if !asciiLowerOrDigit(r) {
				return false
			}
		}
	}
	return true
}

func asciiLetters(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < 'a' || value[i] > 'z' {
			return false
		}
	}
	return true
}

func asciiDigits(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}
