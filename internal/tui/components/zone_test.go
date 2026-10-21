package components

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

func TestZonesAreInertUntilEnabled(t *testing.T) {
	if got := Zone("row:1", "Arsenal"); got != "Arsenal" {
		t.Errorf("Zone marked %q with zones off", got)
	}
}

func TestAClickFindsTheInnermostRegion(t *testing.T) {
	EnableZones()
	t.Cleanup(func() {
		zones.mu.Lock()
		zones.on = false
		zones.mu.Unlock()
	})

	table := Table(TableOpts{
		Columns: []Column{{Title: "Club", Width: 10}},
		Rows:    [][]Cell{{Text("Arsenal")}, {Text("Chelsea")}},
		Cursor:  -1,
		Theme:   styles.Plain(),
		Zone:    "row",
	})
	frame := Zone("card:0", "Header\n"+table)
	ids := MarkedZones()
	plain := ScanZones(frame)
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("markers left in the frame: %q", plain)
	}

	// Scan hands its results to a worker; give it a moment.
	click := tea.MouseMsg{X: 2, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	deadline := time.Now().Add(time.Second)
	scanned := func() bool {
		for _, id := range ids {
			if zone.Get(id) == nil {
				return false
			}
		}
		return true
	}
	for !scanned() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	// Line 0 is "Header", 1 the table's header, 2 Arsenal, 3 Chelsea.
	if id, ok := ZoneAt(click, ids); !ok || id != "row:1" {
		t.Errorf("click on Chelsea found %q, %v", id, ok)
	}
	click.Y = 0
	if id, ok := ZoneAt(click, ids); !ok || id != "card:0" {
		t.Errorf("click on the header found %q, %v", id, ok)
	}
}
