package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/data/sync"
)

func newCompetitionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "competitions",
		Short: "List the competitions footy currently knows how to refresh",
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, c := range sync.Scope {
				fmt.Fprintf(cmd.OutOrStdout(), "%-22s %-28s %-10s tier %d  %s\n", c.ID, c.Name, c.Country, c.Tier, c.Gender)
			}
			return nil
		},
	}
}
