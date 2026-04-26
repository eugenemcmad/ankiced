package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_DiscoverRootConfigYAMLAndUseAnkiAccount(t *testing.T) {
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(oldWD); chdirErr != nil {
			t.Logf("restore cwd: %v", chdirErr)
		}
	})
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}

	cfgData := []byte("anki_account: test-account@example.com\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), cfgData, 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	// t.Setenv records the original value and restores it via t.Cleanup so we
	// don't need to manage env state by hand. Setting an env var to "" is
	// the supported way to clear it for the duration of the test.
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	t.Setenv("ANKICED_DB_PATH", "")
	t.Setenv("ANKICED_ANKI_ACCOUNT", "")

	cfg, err := Loader{}.Load(context.Background(), nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !strings.Contains(cfg.DBPath, "test-account@example.com") {
		t.Fatalf("expected db path resolved from discovered config account, got %q", cfg.DBPath)
	}
}

func TestLoad_DoesNotOverrideConfigWithFlagDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	dbPath := filepath.Join(tmpDir, "collection.anki2")
	cfgData := []byte(strings.Join([]string{
		"db_path: " + dbPath,
		"http_addr: 127.0.0.1:9090",
		"backup_keep_last_n: 7",
		"workers: 8",
		"force_apply: true",
		"verbose: true",
		"full_diff: true",
	}, "\n"))
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Loader{}.Load(context.Background(), []string{"--config", cfgPath})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.HTTPAddr != "127.0.0.1:9090" {
		t.Fatalf("expected http addr from config, got %q", cfg.HTTPAddr)
	}
	if cfg.BackupKeepLastN != 7 || cfg.Workers != 8 {
		t.Fatalf("expected numeric values from config, got backup=%d workers=%d", cfg.BackupKeepLastN, cfg.Workers)
	}
	if !cfg.ForceApply || !cfg.Verbose || !cfg.FullDiff {
		t.Fatalf("expected bool values from config, got force_apply=%v verbose=%v full_diff=%v", cfg.ForceApply, cfg.Verbose, cfg.FullDiff)
	}
}

func TestLoad_FlagsOverrideConfigEvenWhenEqualToDefaultsOrFalse(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	dbPath := filepath.Join(tmpDir, "collection.anki2")
	cfgData := []byte(strings.Join([]string{
		"db_path: " + dbPath,
		"http_addr: 127.0.0.1:9090",
		"backup_keep_last_n: 7",
		"workers: 8",
		"force_apply: true",
		"verbose: true",
		"full_diff: true",
	}, "\n"))
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Loader{}.Load(context.Background(), []string{
		"--config", cfgPath,
		"--http-addr", "127.0.0.1:8080",
		"--backup-keep", "3",
		"--workers", "4",
		"--force-apply=false",
		"--verbose=false",
		"--full-diff=false",
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.HTTPAddr != "127.0.0.1:8080" {
		t.Fatalf("expected http addr from flag, got %q", cfg.HTTPAddr)
	}
	if cfg.BackupKeepLastN != 3 || cfg.Workers != 4 {
		t.Fatalf("expected numeric values from flags, got backup=%d workers=%d", cfg.BackupKeepLastN, cfg.Workers)
	}
	if cfg.ForceApply || cfg.Verbose || cfg.FullDiff {
		t.Fatalf("expected bool flags to override false, got force_apply=%v verbose=%v full_diff=%v", cfg.ForceApply, cfg.Verbose, cfg.FullDiff)
	}
}

func TestLoad_LoadsDefaultPageSizeAndPragmaBusyTimeoutFromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	dbPath := filepath.Join(tmpDir, "collection.anki2")
	cfgData := []byte(strings.Join([]string{
		"db_path: " + dbPath,
		"default_page_size: 25",
		"pragma_busy_timeout: 7000",
	}, "\n"))
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Loader{}.Load(context.Background(), []string{"--config", cfgPath})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DefaultPageSize != 25 {
		t.Fatalf("expected default_page_size=25 from yaml, got %d", cfg.DefaultPageSize)
	}
	if cfg.PragmaBusyTimeout != 7000 {
		t.Fatalf("expected pragma_busy_timeout=7000 from yaml, got %d", cfg.PragmaBusyTimeout)
	}
}

func TestLoad_LoadsDefaultPageSizeAndPragmaBusyTimeoutFromJSON(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	dbPath := filepath.Join(tmpDir, "collection.anki2")
	payload, err := json.Marshal(map[string]any{
		"db_path":             dbPath,
		"default_page_size":   15,
		"pragma_busy_timeout": 4500,
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	cfgData := payload
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Loader{}.Load(context.Background(), []string{"--config", cfgPath})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DefaultPageSize != 15 {
		t.Fatalf("expected default_page_size=15 from json, got %d", cfg.DefaultPageSize)
	}
	if cfg.PragmaBusyTimeout != 4500 {
		t.Fatalf("expected pragma_busy_timeout=4500 from json, got %d", cfg.PragmaBusyTimeout)
	}
}

func TestLoad_FlagsOverridePageSizeAndBusyTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	dbPath := filepath.Join(tmpDir, "collection.anki2")
	cfgData := []byte(strings.Join([]string{
		"db_path: " + dbPath,
		"default_page_size: 5",
		"pragma_busy_timeout: 1000",
	}, "\n"))
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Loader{}.Load(context.Background(), []string{
		"--config", cfgPath,
		"--page-size", "42",
		"--busy-timeout-ms", "9999",
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DefaultPageSize != 42 {
		t.Fatalf("expected page_size=42 from flag, got %d", cfg.DefaultPageSize)
	}
	if cfg.PragmaBusyTimeout != 9999 {
		t.Fatalf("expected busy_timeout=9999 from flag, got %d", cfg.PragmaBusyTimeout)
	}
}

func TestLoad_EnvOverridesPageSizeAndBusyTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	dbPath := filepath.Join(tmpDir, "collection.anki2")
	cfgData := []byte("db_path: " + dbPath + "\n")
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("ANKICED_PAGE_SIZE", "50")
	t.Setenv("ANKICED_BUSY_TIMEOUT_MS", "2500")

	cfg, err := Loader{}.Load(context.Background(), []string{"--config", cfgPath})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DefaultPageSize != 50 {
		t.Fatalf("expected env page_size=50, got %d", cfg.DefaultPageSize)
	}
	if cfg.PragmaBusyTimeout != 2500 {
		t.Fatalf("expected env busy_timeout=2500, got %d", cfg.PragmaBusyTimeout)
	}
}

func TestLoad_DefaultsForPragmaJournalAndSynchronous(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	dbPath := filepath.Join(tmpDir, "collection.anki2")
	cfgData := []byte("db_path: " + dbPath + "\n")
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Loader{}.Load(context.Background(), []string{"--config", cfgPath})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.PragmaJournalMode != "WAL" {
		t.Fatalf("expected default journal_mode=WAL, got %q", cfg.PragmaJournalMode)
	}
	if cfg.PragmaSynchronous != "NORMAL" {
		t.Fatalf("expected default synchronous=NORMAL, got %q", cfg.PragmaSynchronous)
	}
}

func TestLoad_PragmaJournalAndSynchronousFromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	dbPath := filepath.Join(tmpDir, "collection.anki2")
	cfgData := []byte(strings.Join([]string{
		"db_path: " + dbPath,
		"pragma_journal_mode: DELETE",
		"pragma_synchronous: FULL",
	}, "\n"))
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Loader{}.Load(context.Background(), []string{"--config", cfgPath})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.PragmaJournalMode != "DELETE" {
		t.Fatalf("expected journal_mode=DELETE from yaml, got %q", cfg.PragmaJournalMode)
	}
	if cfg.PragmaSynchronous != "FULL" {
		t.Fatalf("expected synchronous=FULL from yaml, got %q", cfg.PragmaSynchronous)
	}
}

func TestLoad_FlagsOverridePragmaJournalAndSynchronous(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	dbPath := filepath.Join(tmpDir, "collection.anki2")
	cfgData := []byte(strings.Join([]string{
		"db_path: " + dbPath,
		"pragma_journal_mode: WAL",
		"pragma_synchronous: NORMAL",
	}, "\n"))
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Loader{}.Load(context.Background(), []string{
		"--config", cfgPath,
		"--pragma-journal-mode", "TRUNCATE",
		"--pragma-synchronous", "OFF",
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.PragmaJournalMode != "TRUNCATE" {
		t.Fatalf("expected journal_mode=TRUNCATE from flag, got %q", cfg.PragmaJournalMode)
	}
	if cfg.PragmaSynchronous != "OFF" {
		t.Fatalf("expected synchronous=OFF from flag, got %q", cfg.PragmaSynchronous)
	}
}

func TestLoad_EnvOverridesPragmaJournalAndSynchronous(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	dbPath := filepath.Join(tmpDir, "collection.anki2")
	cfgData := []byte("db_path: " + dbPath + "\n")
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("ANKICED_PRAGMA_JOURNAL_MODE", "MEMORY")
	t.Setenv("ANKICED_PRAGMA_SYNCHRONOUS", "EXTRA")

	cfg, err := Loader{}.Load(context.Background(), []string{"--config", cfgPath})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.PragmaJournalMode != "MEMORY" {
		t.Fatalf("expected env journal_mode=MEMORY, got %q", cfg.PragmaJournalMode)
	}
	if cfg.PragmaSynchronous != "EXTRA" {
		t.Fatalf("expected env synchronous=EXTRA, got %q", cfg.PragmaSynchronous)
	}
}

func TestLoad_EnvCanOverrideConfigBoolFalse(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	dbPath := filepath.Join(tmpDir, "collection.anki2")
	cfgData := []byte(strings.Join([]string{
		"db_path: " + dbPath,
		"force_apply: true",
		"verbose: true",
	}, "\n"))
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("ANKICED_FORCE_APPLY", "false")
	t.Setenv("ANKICED_VERBOSE", "0")

	cfg, err := Loader{}.Load(context.Background(), []string{"--config", cfgPath})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.ForceApply || cfg.Verbose {
		t.Fatalf("expected env false overrides, got force_apply=%v verbose=%v", cfg.ForceApply, cfg.Verbose)
	}
}
