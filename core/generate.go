package core

// File is one generated output file. Path is relative to the generated root
// directory; Content is the exact bytes to write. The core never touches the
// filesystem, so a caller (a future CLI adapter) decides where to write it.
type File struct {
	Path    string
	Content []byte
}

// Generate validates spec against standards and returns the generated
// Terraform root files. Validation always runs (via resolve), so library
// callers cannot bypass it; invalid input returns diagnostics and no files.
//
// This is a stub: STACK-6 will render HCL from the resolved configuration.
// Until then valid input returns (nil, nil), meaning "valid, but no files are
// generated yet", not "generation produced nothing".
func Generate(spec Spec, standards PlatformStandards) ([]File, []Diagnostic) {
	if _, diagnostics := resolve(spec, standards); len(diagnostics) != 0 {
		return nil, diagnostics
	}
	return nil, nil
}
