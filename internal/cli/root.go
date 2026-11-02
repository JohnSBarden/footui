// Package cli implements the footui command-line interface.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/config"
	"github.com/JohnSBarden/footui/internal/trace"
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
		// Execute prints errors; without this Cobra prints them twice.
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Config commands must work with a broken config file, to fix it.
			if cmd.Annotations[skipSettings] != "" {
				return nil
			}
			if err := g.resolveSettings(cmd); err != nil {
				return err
			}
			if !g.settings.debug {
				return nil
			}
			if err := trace.Start(config.DebugLogPath()); err != nil {
				return err
			}
			trace.Printf("footui %s: %s", Version, strings.Join(os.Args[1:], " "))
			return nil
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
	root.PersistentFlags().BoolVar(&g.debug, "debug", false,
		"write a debug log for bug reports to $XDG_STATE_HOME/footui/debug.log (also FOOTUI_DEBUG=1; config: debug)")
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
	err := NewRootCmd().Execute()
	if err != nil {
		trace.Line("FAIL", err.Error())
	}
	if path := trace.Path(); path != "" {
		_ = trace.Stop()
		fmt.Fprintln(os.Stderr, "debug log:", path)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
