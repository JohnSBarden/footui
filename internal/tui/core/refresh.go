package core

import "context"

// Refresher fetches a competition's current season into the cache. It is
// how the TUI runs the same work as `footy refresh --competition <id>`
// without importing the provider or sync packages: the root app is handed
// one in Deps and screens never see it.
//
// progress is called with a human-readable line each time something worth
// telling the user happens mid-fetch (a retry, mostly), since a refresh
// against a slow host can take most of a minute and silence for that long
// looks like a hang. The returned summary is one line describing what was
// cached, shown once the refresh succeeds.
type Refresher func(ctx context.Context, competitionID string, progress func(text string)) (summary string, err error)

// Refreshable is implemented by screens that know which competition a
// refresh triggered from them should fetch. A screen with nothing sensible
// to refresh (a player, say) simply doesn't implement it, and the root app
// explains that instead of guessing.
type Refreshable interface {
	RefreshTarget() (competitionID string, ok bool)
}

// RefreshedMsg is broadcast by the root app to every screen on the stack
// after a refresh succeeds, so each can re-query and replace its empty
// state with the data that just arrived — including screens beneath the
// visible one, which the user may pop back to.
type RefreshedMsg struct{ CompetitionID string }
