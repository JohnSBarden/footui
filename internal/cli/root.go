// Package cli implements the footy command-line interface.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/tui"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// Version is set via -ldflags at build time; defaults to "dev" for local builds.
var Version = "dev"

// NewRootCmd constructs the root `footy` command and wires its subcommands.
func NewRootCmd() *cobra.Command {
	g := &globalFlags{}

	root := &cobra.Command{
		Use:   "footy",
		Short: "Interactive terminal dashboard for football/soccer statistics",
		Long: "footy is a terminal tool for drilling into football/soccer stats: " +
			"leagues, teams, and players across multiple competitions and seasons.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		// Errors are printed once, by Execute below. Without this Cobra
		// prints them too and every failure appears twice.
		SilenceErrors: true,
		// Bare `footy` opens the Browse screen over whatever is cached.
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			return g.launchTUI(ctx, func(deps core.Deps) (core.Screen, error) {
				return tui.NewBrowseScreen(deps, ""), nil
			})
		},
	}

	root.PersistentFlags().StringVar(&g.dbPath, "db", "",
		"override the SQLite database path (default: $XDG_DATA_HOME/footui/footy.db)")
	root.PersistentFlags().BoolVar(&g.noColor, "no-color", false,
		"disable color output (also honours the NO_COLOR environment variable)")
	root.PersistentFlags().StringVar(&g.theme, "theme", os.Getenv("FOOTY_THEME"),
		"club theme: weak (club colors as highlights) or strong (screen painted in club colors); default $FOOTY_THEME or weak")
	root.PersistentFlags().DurationVar(&g.staleAfter, "stale-after", DefaultStaleAfter,
		"when the interface opens, refresh followed leagues whose current season is older than this (0 disables)")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newRefreshCmd(g))
	root.AddCommand(newCompetitionsCmd())
	root.AddCommand(newLeagueCmd(g))
	root.AddCommand(newTeamCmd(g))
	root.AddCommand(newPlayerCmd(g))
	root.AddCommand(newColorsCmd(g))
	root.AddCommand(newStatusCmd(g))

	return root
}

// Execute runs the root command, exiting the process on error.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
