package sqlite

import (
	"context"
	"database/sql"
	"testing"
)

// The upgrade renames a cached friendlies "FRI" short name, which
// reads as Friday beside a match date.
func TestUpgradeRenamesFriendliesShortName(t *testing.T) {
	path := copyFile(t, "testdata/footui-0011.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO competition (id, name, name_folded, country, tier, gender, format, calendar_year, category, short_name)
		VALUES ('int-friendly', 'International Friendlies', 'international friendlies', 'International', 0, 'men', 'cup', 1, 'friendly', 'FRI')`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	comp, err := NewStore(db).GetCompetition(context.Background(), "int-friendly")
	if err != nil {
		t.Fatal(err)
	}
	if comp.ShortName != "FRND" {
		t.Errorf("short name = %q, want FRND", comp.ShortName)
	}
}
