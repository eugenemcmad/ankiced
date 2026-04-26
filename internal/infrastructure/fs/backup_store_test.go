package fs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestCreateBackup_CreatesFileAndIsIdempotentPerPath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "collection.anki2")
	writeFile(t, dbPath, "payload-v1")

	var store BackupStore
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	first, err := store.CreateBackup(context.Background(), dbPath, now)
	if err != nil {
		t.Fatalf("first backup: %v", err)
	}
	if first.Path == "" {
		t.Fatalf("expected non-empty backup path")
	}
	if _, err := os.Stat(first.Path); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}

	writeFile(t, dbPath, "payload-v2")
	second, err := store.CreateBackup(context.Background(), dbPath, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("second backup: %v", err)
	}
	if second.Path != "" {
		t.Fatalf("expected idempotent no-op, got path %s", second.Path)
	}

	data, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(data) != "payload-v1" {
		t.Fatalf("backup contents drift, got %q", data)
	}
}

func TestCreateBackup_ConcurrentSamePathProducesSingleBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "collection.anki2")
	writeFile(t, dbPath, "concurrent-payload")

	var store BackupStore
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	const goroutines = 16
	var wg sync.WaitGroup
	wg.Add(goroutines)
	results := make([]string, goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			info, err := store.CreateBackup(context.Background(), dbPath, now)
			if err != nil {
				t.Errorf("goroutine %d: %v", i, err)
				return
			}
			results[i] = info.Path
		}()
	}
	wg.Wait()

	created := 0
	for _, p := range results {
		if p != "" {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("expected exactly one backup created, got %d", created)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	bakCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".bak") {
			bakCount++
		}
	}
	if bakCount != 1 {
		t.Fatalf("expected one .bak file, got %d", bakCount)
	}
}

func TestCreateBackup_DifferentPathsIndependentlyTracked(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.anki2")
	b := filepath.Join(dir, "b.anki2")
	writeFile(t, a, "A")
	writeFile(t, b, "B")

	var store BackupStore
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	if _, err := store.CreateBackup(context.Background(), a, now); err != nil {
		t.Fatalf("backup a: %v", err)
	}
	if _, err := store.CreateBackup(context.Background(), b, now); err != nil {
		t.Fatalf("backup b: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".bak") {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected two backups (a + b), got %d", count)
	}
}

func TestCreateBackup_MissingSourceReturnsError(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.anki2")

	var store BackupStore
	if _, err := store.CreateBackup(context.Background(), missing, time.Now()); err == nil {
		t.Fatalf("expected error for missing source, got nil")
	}
}

func TestCleanupBackups_RetainsOnlyLastN(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "collection.anki2")
	writeFile(t, dbPath, "x")

	for _, suffix := range []string{
		"20260101_000000.bak",
		"20260102_000000.bak",
		"20260103_000000.bak",
		"20260104_000000.bak",
		"20260105_000000.bak",
	} {
		writeFile(t, dbPath+"."+suffix, "old")
	}
	writeFile(t, filepath.Join(dir, "unrelated.bak"), "keep me")
	writeFile(t, dbPath+".20260106_000000.txt", "non-bak")

	var store BackupStore
	if err := store.CleanupBackups(context.Background(), dbPath, 2); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.Name()] = true
	}
	want := []string{
		filepath.Base(dbPath),
		filepath.Base(dbPath) + ".20260104_000000.bak",
		filepath.Base(dbPath) + ".20260105_000000.bak",
		"unrelated.bak",
		filepath.Base(dbPath) + ".20260106_000000.txt",
	}
	for _, name := range want {
		if !got[name] {
			t.Fatalf("expected %q to remain, got entries %v", name, got)
		}
	}
	if got[filepath.Base(dbPath)+".20260101_000000.bak"] {
		t.Fatalf("expected oldest backup to be deleted, got %v", got)
	}
}

func TestCleanupBackups_DefaultsKeepNTo3WhenInvalid(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "collection.anki2")
	writeFile(t, dbPath, "x")

	for _, suffix := range []string{
		"20260101_000000.bak",
		"20260102_000000.bak",
		"20260103_000000.bak",
		"20260104_000000.bak",
	} {
		writeFile(t, dbPath+"."+suffix, "old")
	}

	var store BackupStore
	if err := store.CleanupBackups(context.Background(), dbPath, 0); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	bakCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".bak") {
			bakCount++
		}
	}
	if bakCount != 3 {
		t.Fatalf("expected 3 retained, got %d", bakCount)
	}
}
