package cli

import (
	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/tui"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

func newLeagueCmd(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "league <name>",
		Short: "Open a competition's standings table",
		Long: "league opens the interactive standings table for a competition, showing its " +
			"two most recent cached seasons. The name is matched against cached competitions, " +
			"so a partial name like \"premier\" works.",
		Args: cobra.ExactArgs(1),
		// The resolution error is the user's own typo or an un-refreshed
		// competition, not a bug — don't bury it under Cobra's usage dump.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			return g.launchTUI(ctx, func(deps core.Deps) (core.Screen, error) {
				comp, err := resolveCompetition(ctx, deps.Repos, args[0])
				if err != nil {
					return nil, err
				}
				return tui.NewLeagueScreen(deps, comp.ID), nil
			})
		},
	}
}
