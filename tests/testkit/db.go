package testkit

import (
	"database/sql"
	"testing"
)

func InitSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE decks (id INTEGER PRIMARY KEY, name TEXT NOT NULL);`,
		`CREATE TABLE notetypes (id INTEGER NOT NULL PRIMARY KEY, name TEXT NOT NULL, mtime_secs INTEGER NOT NULL, usn INTEGER NOT NULL, config BLOB NOT NULL);`,
		`CREATE TABLE fields (ntid INTEGER NOT NULL, ord INTEGER NOT NULL, name TEXT NOT NULL, config BLOB NOT NULL, PRIMARY KEY (ntid, ord));`,
		`CREATE TABLE models (id INTEGER PRIMARY KEY, flds TEXT NOT NULL);`,
		`CREATE TABLE notes (id INTEGER PRIMARY KEY, guid TEXT, mid INTEGER, flds TEXT, mod INTEGER, usn INTEGER);`,
		`CREATE TABLE cards (id INTEGER PRIMARY KEY, nid INTEGER, did INTEGER);`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("init schema: %v", err)
		}
	}
}

func SeedData(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO decks(id, name) VALUES (1, 'Default')`); err != nil {
		t.Fatalf("seed decks: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO notetypes(id, name, mtime_secs, usn, config) VALUES (10, 'Basic', 0, 0, x'00')`); err != nil {
		t.Fatalf("seed notetypes: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO fields(ntid, ord, name, config) VALUES (10, 0, 'Front', x'00'), (10, 1, 'Back', x'00')`); err != nil {
		t.Fatalf("seed fields: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO models(id, flds) VALUES (10, '[{"name":"Front"},{"name":"Back"}]')`); err != nil {
		t.Fatalf("seed models: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO notes(id,guid,mid,flds,mod,usn) VALUES
		(100,'g1',10,'hello <script>world</script>' || char(31) || 'back field',1,0),
		(101,'g2',10,'foo bar' || char(31) || 'baz',1,0)`); err != nil {
		t.Fatalf("seed notes: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO cards(id,nid,did) VALUES (1,100,1),(2,101,1)`); err != nil {
		t.Fatalf("seed cards: %v", err)
	}
}
