package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/data/sources"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/tui/components"
)

func newCompetitionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "competitions",
		Short: "List the competitions footui currently knows how to refresh",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%-29s %-32s %-14s %-4s %-6s %-10s %s\n", "ID", "NAME", "COUNTRY", "TIER", "", "SOURCE", "SEASONS")
			for _, c := range sync.Coverages {
				comp := c.Competition
				tier := strconv.Itoa(comp.Tier)
				country := comp.Country
				if comp.International() {
					tier, country = "-", comp.Category
				}
				fmt.Fprintf(out, "%-29s %-32s %-14s %-4s %-6s %-10s from %s\n", comp.ID, comp.Name, country,
					tier, comp.Gender, c.Provider, components.SeasonLabelFor(comp.CalendarYear, strconv.Itoa(c.FirstSeason)))
			}
			// Which source covers a competition decides what's there
			// beyond the table.
			fmt.Fprintln(out)
			seen := map[string]bool{}
			for _, c := range sync.Coverages {
				if about := sources.About(c.Provider); about != "" && !seen[c.Provider] {
					seen[c.Provider] = true
					fmt.Fprintf(out, "%s competitions include %s.\n", c.Provider, about)
				}
			}
			fmt.Fprintln(out, "International competitions are fetched for the national teams you follow (footui refresh --team england),")
			fmt.Fprintln(out, "or one season at a time (footui refresh --competition world-cup --season 2022).")
			return nil
		},
	}
}
