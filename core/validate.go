package core

// Validate checks spec against standards using the semantic rules from
// docs/stacksmith-core-plan.md section 4. It assumes both inputs already
// parsed cleanly (see ParseSpec, ParseStandards). An empty result means spec
// is valid against standards.
func Validate(spec Spec, standards PlatformStandards) []Diagnostic {
	panic("not implemented")
}
