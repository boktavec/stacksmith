package core

// ParseSpec strictly decodes data as a developer Spec in the given format.
// A non-empty []Diagnostic means the returned Spec is not usable.
func ParseSpec(data []byte, format Format) (Spec, []Diagnostic) {
	panic("not implemented")
}

// ParseStandards strictly decodes data as PlatformStandards in the given
// format. A non-empty []Diagnostic means the returned PlatformStandards is
// not usable.
func ParseStandards(data []byte, format Format) (PlatformStandards, []Diagnostic) {
	panic("not implemented")
}
