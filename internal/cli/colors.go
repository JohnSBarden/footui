package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/colors"
)

func newColorsCmd(g *globalFlags) *cobra.Command {
	var showUncolored bool

	cmd := &cobra.Command{
		Use:   "colors",
		Short: "Apply and report on curated club brand colors",
		Long: "colors re-applies footy's hand-curated club colors to every team in the local " +
			"cache and reports coverage.\n\n" +
			"Club colors have no scrapeable source, so they are a curated dataset that only " +
			"covers part of every league. This command is how you find the gaps: any club " +
			"listed as uncolored can be added to internal/colors/seed/teamcolors.yaml.\n\n" +
			"`footy refresh` runs this automatically, so you rarely need it by hand.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, store, err := g.openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			report, err := colors.Seed(cmdContext(cmd), store)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			total := report.Applied + len(report.Uncolored)
			if total == 0 {
				fmt.Fprintln(out, "No teams cached yet — run `footy refresh` first.")
				return nil
			}

			fmt.Fprintf(out, "%d of %d cached teams have brand colors.\n", report.Applied, total)
			if report.Unused > 0 {
				fmt.Fprintf(out, "%d curated entries matched no cached team "+
					"(usually just a competition you haven't refreshed).\n", report.Unused)
			}

			if len(report.Uncolored) == 0 {
				return nil
			}

			fmt.Fprintf(out, "\n%d without colors:\n", len(report.Uncolored))
			if !showUncolored && len(report.Uncolored) > 10 {
				for _, t := range report.Uncolored[:10] {
					fmt.Fprintf(out, "  %-32s %s\n", t.ID, t.Name)
				}
				fmt.Fprintf(out, "  … and %d more (--all to list them)\n", len(report.Uncolored)-10)
				return nil
			}
			for _, t := range report.Uncolored {
				fmt.Fprintf(out, "  %-32s %s\n", t.ID, t.Name)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&showUncolored, "all", false, "list every uncolored team instead of the first ten")

	return cmd
}
