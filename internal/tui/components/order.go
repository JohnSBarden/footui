package components

import (
	"sort"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// SortCompetitions orders a club's or player's competitions for tabs:
// domestic before European, then by tier (with tier 0, unknown, last),
// then by name. A fixed order keeps tabs stable across refreshes rather
// than following map iteration.
func SortCompetitions(comps []models.Competition) {
	sort.Slice(comps, func(i, j int) bool {
		if ei, ej := comps[i].Country == "Europe", comps[j].Country == "Europe"; ei != ej {
			return ej
		}
		if comps[i].Tier != comps[j].Tier {
			if comps[i].Tier == 0 {
				return false
			}
			if comps[j].Tier == 0 {
				return true
			}
			return comps[i].Tier < comps[j].Tier
		}
		return comps[i].Name < comps[j].Name
	})
}
