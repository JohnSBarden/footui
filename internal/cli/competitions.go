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
			fmt.Fprintf(out, "%-29s %-26s %-8s %-4s %-6s %-10s %s\n", "ID", "NAME", "COUNTRY", "TIER", "", "SOURCE", "SEASONS")
			for _, c := range sync.Coverages {
				comp := c.Competition
				// Which provider decides what's available: understat has
				// expected goals, ESPN has possession and corners.
				source := "espn"
				if c.Provider == sync.ProviderUnderstat {
					source = "understat"
				}
				fmt.Fprintf(out, "%-29s %-26s %-8s %-4d %-6s %-10s from %s\n", comp.ID, comp.Name, comp.Country,
					comp.Tier, comp.Gender, source, components.SeasonLabelFor(comp.CalendarYear, strconv.Itoa(c.FirstSeason)))
			}
			fmt.Fprintln(out, "\nunderstat competitions include expected goals (xG); ESPN ones include possession, corners, and fouls instead.")
			return nil
		},
	}
}
