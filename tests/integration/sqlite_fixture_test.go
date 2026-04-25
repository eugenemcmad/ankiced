package integration

import (
	"os"
	"path/filepath"
	"testing"
	sqliteinfra "ankiced/internal/infrastructure/sqlite"
	"ankiced/tests/testkit"
)

func createFixtureDB(t *testing.T) (string, *sqliteinfra.DB) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "collection.anki2")
	db, err := sqliteinfra.Open(path, sqliteinfra.Pragmas{BusyTimeoutMS: 5000})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testkit.InitSchema(t, db.SQL)
	testkit.SeedData(t, db.SQL)
	return path, db
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
