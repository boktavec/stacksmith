// Package cli is the command-line adapter around the pure core library: it
// owns flags, file reading, output formatting and exit codes, while core stays
// free of I/O. Run takes its writers as parameters and returns the exit code
// instead of calling os.Exit, so tests can run the whole CLI in-process.
package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/boktavec/stacksmith/core"
	"github.com/spf13/cobra"
)

// Exit codes: 0 valid; 1 invalid input or I/O failure; 2 usage error.
const (
	exitOK           = 0
	exitInvalidInput = 1
	exitUsage        = 2
)

// Run executes the CLI with args (without the program name) and returns the
// process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	// cobra falls back to os.Args when given nil args, which would make tests
	// (and callers passing nil) read the real command line.
	if args == nil {
		args = []string{}
	}

	exitCode := exitOK
	root := newRootCmd(stdout, stderr, &exitCode)
	root.SetArgs(args)

	err := root.Execute()
	var runErr *runError
	switch {
	case err == nil:
		return exitCode
	case errors.As(err, &runErr):
		fmt.Fprintln(stderr, escapeControl(err.Error()))
		return exitInvalidInput
	default:
		// Every other Execute error (unknown or missing flag, unknown
		// subcommand, extra args, no subcommand) is a usage error.
		fmt.Fprintln(stderr, "error: "+escapeControl(err.Error()))
		return exitUsage
	}
}

// newRootCmd builds a fresh command tree per call (no package-level command
// variables), so tests cannot leak flag state into each other. A command that
// finds invalid documents reports through exitCode; only failures to run at
// all are returned as errors.
func newRootCmd(stdout, stderr io.Writer, exitCode *int) *cobra.Command {
	root := &cobra.Command{
		Use:           "stacksmith",
		Short:         "Compile a small spec into reviewed Terraform",
		SilenceUsage:  true, // Run prints errors itself, exactly once
		SilenceErrors: true,
		// Cobra's default for a root without Run is "print help, exit 0",
		// which would break the exit-code contract; make it a usage error.
		RunE: func(*cobra.Command, []string) error {
			return errors.New(`a command is required; use "validate"`)
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.CompletionOptions.DisableDefaultCmd = true
	root.DisableSuggestions = true // keeps usage errors to one line
	// A hidden placeholder replaces the auto-generated "help" subcommand;
	// --help and -h still work. Invoking the placeholder is a usage error.
	root.SetHelpCommand(&cobra.Command{
		Use:    "no-help",
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		RunE: func(*cobra.Command, []string) error {
			return errors.New(`unknown command "no-help"`)
		},
	})
	root.AddCommand(newValidateCmd(stdout, stderr, exitCode))
	return root
}

func newValidateCmd(stdout, stderr io.Writer, exitCode *int) *cobra.Command {
	var specPath, standardsPath, format string
	cmd := &cobra.Command{
		Use:   "validate --spec <file> --standards <file>",
		Short: "Check a spec against platform standards without generating anything",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			// Check --format first so a bad format is a usage error even
			// when a file is also missing.
			if format != "text" && format != "json" {
				return fmt.Errorf("invalid --format %q; use text or json", format)
			}
			diagnostics, err := validateFiles(specPath, standardsPath)
			if err != nil {
				return err
			}
			if len(diagnostics) != 0 {
				*exitCode = exitInvalidInput
			}
			if format == "json" {
				err = presentJSON(stdout, diagnostics)
			} else {
				err = presentText(stdout, stderr, diagnostics)
			}
			if err != nil {
				// An output write failure is an I/O failure (exit 1), not usage.
				return &runError{message: fmt.Sprintf("cannot write output: %v", err)}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&specPath, "spec", "", "path to the bucket spec (.yaml, .yml or .json)")
	cmd.Flags().StringVar(&standardsPath, "standards", "", "path to the platform standards (.yaml, .yml or .json)")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	_ = cmd.MarkFlagRequired("spec")      // only errors if the flag is undefined
	_ = cmd.MarkFlagRequired("standards") // only errors if the flag is undefined
	return cmd
}

// validateFiles loads and checks both documents. A non-nil error is a
// runError (unusable file); otherwise the diagnostics describe the documents.
func validateFiles(specPath, standardsPath string) ([]core.Diagnostic, error) {
	// Load both before parsing either; the first load failure wins, spec first.
	specData, specFormat, err := loadInput("--spec", specPath)
	if err != nil {
		return nil, err
	}
	standardsData, standardsFormat, err := loadInput("--standards", standardsPath)
	if err != nil {
		return nil, err
	}

	spec, specDiags := core.ParseSpec(specData, specFormat)
	standards, standardsDiags := core.ParseStandards(standardsData, standardsFormat)
	if parseDiags := append(specDiags, standardsDiags...); len(parseDiags) != 0 {
		// Semantic validation of an unusable document would only add noise.
		return parseDiags, nil
	}
	return core.Validate(spec, standards), nil
}
