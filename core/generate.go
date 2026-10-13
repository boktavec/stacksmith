package core

// generatorVersion identifies the generator in the header of every generated
// file. It is bumped by hand at release.
const generatorVersion = "0.1.0-dev"

// File is one generated output file. Path is relative to the generated root
// directory; Content is the exact bytes to write. The core never touches the
// filesystem, so a caller (a future CLI adapter) decides where to write it.
type File struct {
	Path    string
	Content []byte
}

// Generate validates spec against standards and returns the generated
// Terraform root files (versions.tf, providers.tf, main.tf, outputs.tf, in
// that order). Validation always runs (via resolve), so library callers
// cannot bypass it; invalid input returns diagnostics and no files.
func Generate(spec Spec, standards PlatformStandards) ([]File, []Diagnostic) {
	r, diagnostics := resolve(spec, standards)
	if len(diagnostics) != 0 {
		return nil, diagnostics
	}
	return renderRoot(r), nil
}
