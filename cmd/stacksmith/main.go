// Command stacksmith is a thin wrapper around internal/cli. Keeping main this
// small means all behavior can be tested by calling cli.Run directly.
package main

import (
	"os"

	"github.com/boktavec/stacksmith/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
