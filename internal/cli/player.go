package cli

import (
	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/tui"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

func newPlayerCmd(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "player <name>",
		Short: "Open a player's career dashboard",
		Long: "player opens a player's season-by-season dashboard, sliced by competition. " +
			"The name is matched against cached players; a name matching several players opens " +
			"the browse screen pre-filtered rather than guessing.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			return g.launchTUI(ctx, func(deps core.Deps) (core.Screen, error) {
				matches, err := deps.Repos.FindPlayersByName(ctx, args[0])
				if err != nil {
					return nil, err
				}
				// Same rule as `footui team`: one match opens directly,
				// anything else hands off to Browse.
				if len(matches) == 1 {
					return tui.NewPlayerScreen(deps, matches[0]), nil
				}
				return tui.NewBrowseScreen(deps, args[0]), nil
			})
		},
	}
}
