package core

// Format identifies the input encoding of a spec or standards document.
type Format int

const (
	FormatYAML Format = iota
	FormatJSON
)

// Diagnostic reports one problem found while parsing or validating a
// document. Message includes both the reason and a correction, per
// docs/stacksmith-core-plan.md section 4's example diagnostics.
type Diagnostic struct {
	Document string // e.g. "bucket spec", "platform standards"
	Field    string // dotted path, e.g. "spec.environment"
	Message  string
}
