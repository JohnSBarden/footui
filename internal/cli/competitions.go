package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/tui/components"
)

func newCompetitionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "competitions",
		Short: "List the competitions footy currently knows how to refresh",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%-29s %-32s %-14s %-4s %-6s %-10s %s\n", "ID", "NAME", "COUNTRY", "TIER", "", "SOURCE", "SEASONS")
			for _, c := range sync.Coverages {
				comp := c.Competition
				// Which provider decides what's available: understat has
				// expected goals, ESPN has possession and corners.
				source := "espn"
				if c.Provider == sync.ProviderUnderstat {
					source = "understat"
				}
				tier := strconv.Itoa(comp.Tier)
				country := comp.Country
				if comp.International() {
					tier, country = "-", comp.Category
				}
				fmt.Fprintf(out, "%-29s %-32s %-14s %-4s %-6s %-10s from %s\n", comp.ID, comp.Name, country,
					tier, comp.Gender, source, components.SeasonLabelFor(comp.CalendarYear, strconv.Itoa(c.FirstSeason)))
			}
			fmt.Fprintln(out, "\nunderstat competitions include expected goals (xG); ESPN ones include possession, corners, and fouls instead.")
			fmt.Fprintln(out, "International competitions are fetched for the national teams you follow (footy refresh --team england),")
			fmt.Fprintln(out, "or one season at a time (footy refresh --competition world-cup --season 2022).")
			return nil
		},
	}
}
