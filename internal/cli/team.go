package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/tui"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

func newTeamCmd(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "team <name>",
		Short: "Open a club's or national team's dashboard",
		Long: "team opens a club's season-by-season dashboard, sliced by competition, or a " +
			"national team's matches with a competition filter (\"england\", \"england women\"). " +
			"The name is matched against cached teams; a name matching several clubs opens " +
			"the browse screen pre-filtered rather than guessing.",
		Args:         cobra.MinimumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			return g.launchTUI(ctx, func(deps core.Deps) (core.Screen, error) {
				// An exact nation name opens it, even if clubs contain the name.
				if nation, ok := exactNation(ctx, deps.Repos, strings.Join(args, " ")); ok {
					return tui.NewTeamScreen(deps, nation), nil
				}
				matches, err := deps.Repos.FindTeamsByName(ctx, strings.Join(args, " "))
				if err != nil {
					return nil, err
				}
				// One match opens the club; otherwise hand off to Browse rather
				// than erroring.
				if len(matches) == 1 {
					return tui.NewTeamScreen(deps, matches[0]), nil
				}
				return tui.NewBrowseScreen(deps, strings.Join(args, " ")), nil
			})
		},
	}
}
