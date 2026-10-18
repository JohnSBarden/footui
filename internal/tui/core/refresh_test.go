package core

import (
	"strings"
	"testing"
)

func TestPlayerRequestKeyAndLabel(t *testing.T) {
	r := RefreshRequest{PlayerID: "understat-5613", Name: "Gabriel"}
	if r.Key() != "player:understat-5613" {
		t.Errorf("key = %q", r.Key())
	}
	if r.Label() != "Gabriel" {
		t.Errorf("label = %q", r.Label())
	}
	if (RefreshRequest{PlayerID: "understat-5613"}).Label() != "understat-5613" {
		t.Error("a nameless player request falls back to its ID")
	}
}

// The TUI never names where its data comes from.
func TestRateLimitErrorNamesNoSource(t *testing.T) {
	msg := ErrRateLimited.Error()
	for _, name := range []string{"ESPN", "espn", "understat"} {
		if strings.Contains(msg, name) {
			t.Errorf("%q names a source", msg)
		}
	}
}
