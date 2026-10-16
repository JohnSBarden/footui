// Package cli implements the footui command-line interface.
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

// NewRootCmd constructs the root `footui` command and wires its subcommands.
func NewRootCmd() *cobra.Command {
	g := &globalFlags{}

	root := &cobra.Command{
		Use:   "footui",
		Short: "Interactive terminal dashboard for football/soccer statistics",
		Long: "footui is a terminal tool for drilling into football/soccer stats: " +
			"leagues, teams, and players across multiple competitions and seasons.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		// Errors are printed once, by Execute below. Without this Cobra
		// prints them too and every failure appears twice.
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// The config commands must work with a broken config file —
			// they are how it gets fixed — so they resolve on their own.
			if cmd.Annotations[skipSettings] != "" {
				return nil
			}
			return g.resolveSettings(cmd)
		},
		// Bare `footui` opens the dashboard of favourites, or Browse over
		// whatever is cached when there are none.
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			return g.launchTUI(ctx, func(deps core.Deps) (core.Screen, error) {
				return tui.NewHomeScreen(deps), nil
			})
		},
	}

	root.PersistentFlags().StringVar(&g.dbPath, "db", "",
		"override the SQLite database path (default: $XDG_DATA_HOME/footui/footui.db)")
	root.PersistentFlags().BoolVar(&g.noColor, "no-color", false,
		"disable color output (also honours the NO_COLOR environment variable)")
	root.PersistentFlags().StringVar(&g.theme, "theme", "",
		"club theme: weak (club colors as highlights) or strong (screen painted in club colors); default $FOOTUI_THEME, then the config file, then weak")
	root.PersistentFlags().DurationVar(&g.staleAfter, "stale-after", DefaultStaleAfter,
		"when the interface opens, refresh followed leagues whose current season is older than this (0 disables; config: stale_after)")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newRefreshCmd(g))
	root.AddCommand(newCompetitionsCmd())
	root.AddCommand(newLeagueCmd(g))
	root.AddCommand(newTeamCmd(g))
	root.AddCommand(newPlayerCmd(g))
	root.AddCommand(newColorsCmd(g))
	root.AddCommand(newStatusCmd(g))
	root.AddCommand(newConfigCmd(g))

	return root
}

// Execute runs the root command, exiting the process on error.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
