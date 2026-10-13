package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/boktavec/stacksmith/core"
)

// maxInputBytes bounds how much of an input file the CLI will read. It mirrors
// core's unexported 1 MiB document limit; core stays the single place that
// rejects oversize documents (see loadInput), and cli_test.go pins the
// boundary (TestValidateOversizeFileGetsCoreDiagnostic and
// TestValidateAcceptsFileOfExactlyOneMiB) so the two cannot drift silently.
const maxInputBytes = 1 << 20

// runError marks an input or I/O failure (bad path, extension, file type, or
// a failed output write). Run maps it to exit code 1, as opposed to usage
// errors (exit code 2).
// Go idiom: errors are ordinary values, so callers can tell kinds of failure
// apart with errors.As instead of comparing strings.
type runError struct{ message string }

func (e *runError) Error() string { return e.message }

func runErrorf(format string, args ...any) error {
	return &runError{message: fmt.Sprintf(format, args...)}
}

// loadInput reads one explicit input file and picks its core.Format from the
// extension. flagName ("--spec") names the flag in error messages.
//
// Callers must not use the returned format when err != nil: core.FormatYAML is
// the zero value, so ignoring the error would silently parse as YAML.
//
// Oversize files are not an error here: at most maxInputBytes+1 bytes are read
// and handed to core unchanged, so core reports its own "exceeds the 1 MiB
// limit" diagnostic while the CLI never loads an unbounded file.
func loadInput(flagName, path string) ([]byte, core.Format, error) {
	format, err := formatForPath(flagName, path)
	if err != nil {
		return nil, 0, err
	}

	// Stat before Open: opening a FIFO blocks until a writer appears, so the
	// regular-file check must come first. Stat follows symlinks, so a symlink
	// to a regular file is accepted. A stat-to-open race remains, but the
	// bounded read below limits its impact.
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, runErrorf("%s: cannot read %q: %v", flagName, path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, runErrorf("%s: %q is not a regular file; provide a YAML or JSON file", flagName, path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, 0, runErrorf("%s: cannot open %q: %v", flagName, path, err)
	}
	defer file.Close() // read-only handle: a close error cannot lose data

	// Re-check on the open handle in case the path changed after the Stat.
	handleInfo, err := file.Stat()
	if err != nil {
		return nil, 0, runErrorf("%s: cannot stat %q: %v", flagName, path, err)
	}
	if !handleInfo.Mode().IsRegular() {
		return nil, 0, runErrorf("%s: %q is not a regular file; provide a YAML or JSON file", flagName, path)
	}

	// LimitReader stops after limit bytes. Asking for one byte more than the
	// limit lets core see "too big" without reading the rest of the file.
	data, err := io.ReadAll(io.LimitReader(file, maxInputBytes+1))
	if err != nil {
		return nil, 0, runErrorf("%s: cannot read %q: %v", flagName, path, err)
	}
	return data, format, nil
}

// formatForPath maps a case-sensitive file extension to a core.Format.
func formatForPath(flagName, path string) (core.Format, error) {
	switch filepath.Ext(path) {
	case ".yaml", ".yml":
		return core.FormatYAML, nil
	case ".json":
		return core.FormatJSON, nil
	default:
		return 0, runErrorf("%s: %q has an unsupported extension; use .yaml, .yml or .json", flagName, path)
	}
}
