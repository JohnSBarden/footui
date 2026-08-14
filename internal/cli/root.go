// Package cli implements the footy command-line interface.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set via -ldflags at build time; defaults to "dev" for local builds.
var Version = "dev"

// NewRootCmd constructs the root `footy` command and wires its subcommands.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "footy",
		Short: "Interactive terminal dashboard for football/soccer statistics",
		Long: "footy is a terminal tool for drilling into football/soccer stats: " +
			"leagues, teams, and players across multiple competitions and seasons.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// No-arg invocation launches the Browse screen; wired in a later phase.
			fmt.Fprintln(cmd.OutOrStdout(), "footy: interactive browse screen not implemented yet")
			return nil
		},
	}

	root.AddCommand(newVersionCmd())
	root.AddCommand(newRefreshCmd())
	root.AddCommand(newCompetitionsCmd())

	return root
}

// Execute runs the root command, exiting the process on error.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
