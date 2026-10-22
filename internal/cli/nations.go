package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/textfold"
)

// parseNationQuery splits a trailing gender word off ("england
// women"); men's is the default.
func parseNationQuery(query string) (name, gender string) {
	words := strings.Fields(query)
	if n := len(words); n > 1 {
		switch strings.ToLower(strings.Trim(words[n-1], "()'’")) {
		case "women", "womens", "women's", "w":
			return strings.Join(words[:n-1], " "), "women"
		case "men", "mens", "men's":
			return strings.Join(words[:n-1], " "), "men"
		}
	}
	return strings.TrimSpace(query), "men"
}

// exactNation finds the national team a query names exactly, so
// `footui team spain` opens the nation rather than a list.
func exactNation(ctx context.Context, repos storage.Repositories, query string) (models.Team, bool) {
	name, gender := parseNationQuery(query)
	nations, err := repos.NationalTeams(ctx, gender)
	if err != nil {
		return models.Team{}, false
	}
	want := textfold.Fold(name)
	for _, n := range nations {
		if textfold.Fold(n.Name) == want || textfold.Fold(n.ShortName) == want {
			return n, true
		}
	}
	return models.Team{}, false
}

// resolveNation finds one national team for `refresh --team`, seeding
// the directory first when no nations are cached.
func resolveNation(ctx context.Context, repos storage.Repositories, query string, seed func() error) (models.Team, error) {
	name, gender := parseNationQuery(query)
	all, err := repos.NationalTeams(ctx, "")
	if err != nil {
		return models.Team{}, err
	}
	if len(all) == 0 && seed != nil {
		if err := seed(); err != nil {
			return models.Team{}, err
		}
	}
	if team, ok := exactNation(ctx, repos, query); ok {
		return team, nil
	}
	nations, err := repos.NationalTeams(ctx, gender)
	if err != nil {
		return models.Team{}, err
	}
	want := textfold.Fold(name)
	var matches []models.Team
	for _, n := range nations {
		if strings.Contains(textfold.Fold(n.Name), want) {
			matches = append(matches, n)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return models.Team{}, fmt.Errorf("no %s's national team matches %q", gender, name)
	}
	var names []string
	for _, m := range matches {
		names = append(names, m.Name)
	}
	return models.Team{}, fmt.Errorf("%q matches %d national teams: %s", name, len(matches), strings.Join(names, ", "))
}
