package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	configinfra "ankiced/internal/infrastructure/config"
	fsinfra "ankiced/internal/infrastructure/fs"
)

func TestBackupCreateAndCleanup(t *testing.T) {
	path, db := createFixtureDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("close fixture db: %v", err)
		}
	})
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		store := fsinfra.BackupStore{}
		_, err := store.CreateBackup(ctx, path, time.Now().Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatalf("create backup: %v", err)
		}
	}
	store := fsinfra.BackupStore{}
	if err := store.CleanupBackups(ctx, path, 3); err != nil {
		t.Fatalf("cleanup backups: %v", err)
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	count := 0
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".bak" {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("expected 3 backups, got %d", count)
	}
}

func TestConfigPriority(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte(`{"db_path":"from-file","workers":2,"verbose":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANKICED_DB_PATH", "from-env")
	t.Setenv("ANKICED_VERBOSE", "true")
	loader := configinfra.Loader{}
	cfg, err := loader.Load(context.Background(), []string{"--config", cfgPath, "--db-path", "from-flag", "--verbose"})
	if err != nil {
		t.Fatalf("load cfg: %v", err)
	}
	if cfg.DBPath != "from-flag" {
		t.Fatalf("expected flag priority, got %s", cfg.DBPath)
	}
	if !cfg.Verbose {
		t.Fatal("expected verbose enabled from env/flag")
	}
}

func TestAnkiAccountFromFlagOverridesEnvAndFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte(`{"anki_account":"from-file"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANKICED_ANKI_ACCOUNT", "from-env")
	loader := configinfra.Loader{}
	cfg, err := loader.Load(context.Background(), []string{"--config", cfgPath, "--anki-account", "from-flag"})
	if err != nil {
		t.Fatalf("load cfg: %v", err)
	}
	if cfg.AnkiAccount != "from-flag" {
		t.Fatalf("expected anki account from flag, got %s", cfg.AnkiAccount)
	}
}

func TestDefaultConfigPathDiscovery(t *testing.T) {
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(prev); chdirErr != nil {
			t.Logf("restore working dir: %v", chdirErr)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile("ankiced.yaml", []byte("workers: 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loader := configinfra.Loader{}
	cfg, err := loader.Load(context.Background(), nil)
	if err != nil {
		t.Fatalf("load cfg: %v", err)
	}
	if cfg.Workers != 7 {
		t.Fatalf("expected workers from default config path, got %d", cfg.Workers)
	}
	if cfg.ConfigPath != "ankiced.yaml" {
		t.Fatalf("expected config path ankiced.yaml, got %s", cfg.ConfigPath)
	}
}

func TestConfirmGate(t *testing.T) {
	// confirmation behavior is validated by service/integration flows.
}
