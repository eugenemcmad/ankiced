package sqlite

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestPragmasNormalize_Defaults(t *testing.T) {
	got, err := Pragmas{}.normalize()
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if got.JournalMode != "WAL" {
		t.Fatalf("expected default journal_mode=WAL, got %q", got.JournalMode)
	}
	if got.Synchronous != "NORMAL" {
		t.Fatalf("expected default synchronous=NORMAL, got %q", got.Synchronous)
	}
	if got.BusyTimeoutMS != 5000 {
		t.Fatalf("expected default busy_timeout=5000, got %d", got.BusyTimeoutMS)
	}
}

func TestPragmasNormalize_Whitelist(t *testing.T) {
	if _, err := (Pragmas{JournalMode: "evil; DROP TABLE"}).normalize(); !errors.Is(err, ErrInvalidJournalMode) {
		t.Fatalf("expected ErrInvalidJournalMode, got %v", err)
	}
	if _, err := (Pragmas{Synchronous: "weird"}).normalize(); !errors.Is(err, ErrInvalidSynchronous) {
		t.Fatalf("expected ErrInvalidSynchronous, got %v", err)
	}
}

func TestPragmasNormalize_AcceptsLowercase(t *testing.T) {
	got, err := (Pragmas{JournalMode: "delete", Synchronous: "full"}).normalize()
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if got.JournalMode != "DELETE" || got.Synchronous != "FULL" {
		t.Fatalf("expected uppercase pragmas, got journal=%q synchronous=%q", got.JournalMode, got.Synchronous)
	}
}

func TestOpen_DefaultsSucceed(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.sqlite3")
	db, err := Open(dbPath, Pragmas{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("close: %v", closeErr)
		}
	}()
	row := db.SQL.QueryRow("PRAGMA journal_mode")
	var mode string
	if err := row.Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode == "" {
		t.Fatalf("journal_mode is empty")
	}
}

func TestOpen_RejectsInvalidJournalMode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.sqlite3")
	if _, err := Open(dbPath, Pragmas{JournalMode: "garbage"}); !errors.Is(err, ErrInvalidJournalMode) {
		t.Fatalf("expected ErrInvalidJournalMode, got %v", err)
	}
}
