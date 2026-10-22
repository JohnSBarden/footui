package components

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

// Clickable regions: screens mark them with Zone, the root app resolves
// clicks with ZoneAt. Until EnableZones, Zone returns its input untouched.
var zones struct {
	mu sync.Mutex
	on bool
	// marked are the IDs marked since the frame began.
	marked []string
}

// EnableZones turns clickable regions on for the rest of the process.
func EnableZones() {
	zone.NewGlobal()
	zones.mu.Lock()
	zones.on = true
	zones.mu.Unlock()
}

// Zone marks s as the clickable region id.
func Zone(id, s string) string {
	zones.mu.Lock()
	defer zones.mu.Unlock()
	if !zones.on {
		return s
	}
	zones.marked = append(zones.marked, id)
	return zone.Mark(id, s)
}

// ScanZones records where frame's regions landed and strips their
// markers. It ends the frame: the next Zone call starts a new one.
func ScanZones(frame string) string {
	zones.mu.Lock()
	defer zones.mu.Unlock()
	if !zones.on {
		return frame
	}
	frame = zone.Scan(frame)
	zones.marked = zones.marked[:0]
	return frame
}

// ZoneAt is the innermost of ids (the last frame's) the event landed in.
func ZoneAt(msg tea.MouseMsg, ids []string) (string, bool) {
	best, area := "", 0
	for _, id := range ids {
		z := zone.Get(id)
		if !z.InBounds(msg) {
			continue
		}
		a := (z.EndX - z.StartX + 1) * (z.EndY - z.StartY + 1)
		if best == "" || a < area {
			best, area = id, a
		}
	}
	return best, best != ""
}

// MarkedZones are the IDs marked in the current frame; read them before
// ScanZones ends it.
func MarkedZones() []string {
	zones.mu.Lock()
	defer zones.mu.Unlock()
	return append([]string(nil), zones.marked...)
}
