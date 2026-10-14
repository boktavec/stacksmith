package cli

import (
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/boktavec/stacksmith/core"
	"github.com/spf13/cobra"
)

func newCatalogCmd(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Browse the embedded blueprint catalog",
		Args:  cobra.NoArgs,
		// Like the root command, a group without a subcommand is a usage
		// error rather than cobra's default "print help, exit 0".
		RunE: func(*cobra.Command, []string) error {
			return errors.New(`a subcommand is required; use "catalog list"`)
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List blueprints and whether StackSmith can generate them yet",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			entries, err := core.Catalog()
			if err != nil {
				return runErrorf("cannot load catalog: %v", err)
			}
			if err := writeCatalog(stdout, entries); err != nil {
				return runErrorf("cannot write output: %v", err)
			}
			return nil
		},
	})
	return cmd
}

// writeCatalog prints the entries as an aligned table. IDs and summaries come
// from vendored data, but are escaped like every other printed string.
func writeCatalog(w io.Writer, entries []core.CatalogEntry) error {
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "BLUEPRINT\tSTATUS\tSUMMARY")
	for _, entry := range entries {
		status := "not yet supported"
		if entry.Supported {
			status = "supported"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\n", escapeControl(entry.ID), status, escapeControl(entry.Summary))
	}
	// Writes to tabwriter are buffered; Flush reports the underlying error.
	return table.Flush()
}
