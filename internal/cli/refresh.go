package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/config"
	"github.com/JohnSBarden/footui/internal/data/provider/understat"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage/sqlite"
)

func newRefreshCmd() *cobra.Command {
	var dbPath, competitionID, season string

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Fetch and cache the latest stats from the data provider",
		Long: "refresh pulls team, standings, and player data from the understat.com provider " +
			"and stores it in the local SQLite cache. See `footy competitions` for valid " +
			"--competition values (currently the six leagues understat.com covers).",
		RunE: func(cmd *cobra.Command, args []string) error {
			comp, ok := sync.FindCompetition(competitionID)
			if !ok {
				var ids []string
				for _, c := range sync.Scope {
					ids = append(ids, c.ID)
				}
				return fmt.Errorf("unknown --competition %q; supported: %s", competitionID, strings.Join(ids, ", "))
			}

			path := dbPath
			if path == "" {
				path = config.DefaultDBPath()
			}
			db, err := sqlite.Open(path)
			if err != nil {
				return err
			}
			defer db.Close()

			store := sqlite.NewStore(db)
			src := understat.New()
			syncer := sync.New(src, store)

			seasonModel := sync.NewSeason(comp.ID, season)

			ctx := context.Background()
			fmt.Fprintf(cmd.OutOrStdout(), "Refreshing %s (%s season)...\n", comp.Name, season)

			res, err := syncer.SyncCompetitionSeason(ctx, comp, seasonModel)
			if err != nil {
				return fmt.Errorf("refresh failed: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Done. Teams: %d, team-season rows: %d, players: %d, player-season rows: %d\n",
				res.Teams, res.TeamStats, res.Players, res.PlayerStats)
			fmt.Fprintf(cmd.OutOrStdout(), "Cached at %s\n", path)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "override the SQLite database path (default: $XDG_DATA_HOME/footui/footy.db)")
	cmd.Flags().StringVar(&competitionID, "competition", "eng-premier-league", "competition ID to refresh (see Scope in internal/data/sync/scope.go)")
	cmd.Flags().StringVar(&season, "season", "2025", "season start year to refresh, e.g. 2025 for 2025-26")

	return cmd
}
