package config

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	appconfig "ankiced/internal/config"

	"gopkg.in/yaml.v3"
)

type Loader struct{}

func (Loader) Load(_ context.Context, args []string) (appconfig.Settings, error) {
	fs := flag.NewFlagSet("ankiced", flag.ContinueOnError)
	var (
		dbPath         = fs.String("db-path", "", "path to collection.anki2")
		ankiAccount    = fs.String("anki-account", "", "Anki profile/account folder name")
		httpAddr       = fs.String("http-addr", "127.0.0.1:8080", "REST API bind address")
		configPath     = fs.String("config", "", "config file")
		keepBackups    = fs.Int("backup-keep", 3, "number of backups to keep")
		workers        = fs.Int("workers", 4, "workers")
		forceApply     = fs.Bool("force-apply", false, "allow cleaner apply without interactive confirmation")
		verbose        = fs.Bool("verbose", false, "enable verbose output")
		fullDiff       = fs.Bool("full-diff", false, "show full diff")
		reportFile     = fs.String("report-file", "", "path to dry run report")
		pageSize          = fs.Int("page-size", 10, "default pagination page size")
		busyTimeoutMS     = fs.Int("busy-timeout-ms", 5000, "SQLite busy_timeout in milliseconds")
		pragmaJournalMode = fs.String("pragma-journal-mode", "WAL", "SQLite journal_mode pragma (WAL|DELETE|TRUNCATE|MEMORY)")
		pragmaSynchronous = fs.String("pragma-synchronous", "NORMAL", "SQLite synchronous pragma (OFF|NORMAL|FULL|EXTRA)")
	)
	if err := fs.Parse(args); err != nil {
		return appconfig.Settings{}, err
	}
	visited := visitedFlags(fs)

	cfg := appconfig.Settings{
		BackupKeepLastN:   3,
		Workers:           4,
		DefaultPageSize:   10,
		PragmaBusyTimeout: 5000,
		PragmaJournalMode: "WAL",
		PragmaSynchronous: "NORMAL",
		HTTPAddr:          "127.0.0.1:8080",
	}

	fileCfg, err := loadFileConfig(*configPath)
	if err != nil {
		return appconfig.Settings{}, err
	}
	if *configPath == "" {
		if discovered := discoverDefaultConfigPath(); discovered != "" {
			*configPath = discovered
			fileCfg, err = loadFileConfig(*configPath)
			if err != nil {
				return appconfig.Settings{}, err
			}
		}
	}
	merge(&cfg, fileCfg)
	applyEnv(&cfg)

	if *dbPath != "" {
		cfg.DBPath = *dbPath
	}
	if *ankiAccount != "" {
		cfg.AnkiAccount = *ankiAccount
	}
	if *configPath != "" {
		cfg.ConfigPath = *configPath
	}
	if visited["http-addr"] {
		cfg.HTTPAddr = *httpAddr
	}
	if visited["backup-keep"] {
		cfg.BackupKeepLastN = *keepBackups
	}
	if visited["workers"] {
		cfg.Workers = *workers
	}
	if visited["force-apply"] {
		cfg.ForceApply = *forceApply
	}
	if visited["verbose"] {
		cfg.Verbose = *verbose
	}
	if visited["full-diff"] {
		cfg.FullDiff = *fullDiff
	}
	if visited["report-file"] {
		cfg.ReportFile = *reportFile
	}
	if visited["page-size"] {
		cfg.DefaultPageSize = *pageSize
	}
	if visited["busy-timeout-ms"] {
		cfg.PragmaBusyTimeout = *busyTimeoutMS
	}
	if visited["pragma-journal-mode"] {
		cfg.PragmaJournalMode = *pragmaJournalMode
	}
	if visited["pragma-synchronous"] {
		cfg.PragmaSynchronous = *pragmaSynchronous
	}
	if cfg.DBPath == "" {
		cfg.DBPath = defaultPathByOS(cfg.AnkiAccount)
	}
	if cfg.DBPath == "" {
		return appconfig.Settings{}, ErrDatabasePathEmpty
	}
	return cfg, nil
}

func visitedFlags(fs *flag.FlagSet) map[string]bool {
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	return visited
}

func discoverDefaultConfigPath() string {
	candidates := []string{
		"config.yaml",
		"config.yml",
		"config.json",
		"ankiced.yaml",
		"ankiced.yml",
		"ankiced.json",
		filepath.Join("config", "ankiced.yaml"),
		filepath.Join("config", "ankiced.yml"),
		filepath.Join("config", "ankiced.json"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func loadFileConfig(path string) (appconfig.Settings, error) {
	if path == "" {
		return appconfig.Settings{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return appconfig.Settings{}, err
	}
	cfg := appconfig.Settings{}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return appconfig.Settings{}, err
		}
	default:
		if err := json.Unmarshal(data, &cfg); err != nil {
			return appconfig.Settings{}, err
		}
	}
	return cfg, nil
}

func applyEnv(cfg *appconfig.Settings) {
	if v := os.Getenv("ANKICED_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("ANKICED_ANKI_ACCOUNT"); v != "" {
		cfg.AnkiAccount = v
	}
	if v := os.Getenv("ANKICED_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if v := os.Getenv("ANKICED_BACKUP_KEEP"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.BackupKeepLastN = n
		}
	}
	if v := os.Getenv("ANKICED_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Workers = n
		}
	}
	if v := os.Getenv("ANKICED_FORCE_APPLY"); v != "" {
		if b, ok := parseBool(v); ok {
			cfg.ForceApply = b
		}
	}
	if v := os.Getenv("ANKICED_VERBOSE"); v != "" {
		if b, ok := parseBool(v); ok {
			cfg.Verbose = b
		}
	}
	if v := os.Getenv("ANKICED_PAGE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.DefaultPageSize = n
		}
	}
	if v := os.Getenv("ANKICED_BUSY_TIMEOUT_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.PragmaBusyTimeout = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("ANKICED_PRAGMA_JOURNAL_MODE")); v != "" {
		cfg.PragmaJournalMode = v
	}
	if v := strings.TrimSpace(os.Getenv("ANKICED_PRAGMA_SYNCHRONOUS")); v != "" {
		cfg.PragmaSynchronous = v
	}
}

func parseBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func merge(dst *appconfig.Settings, src appconfig.Settings) {
	if src.DBPath != "" {
		dst.DBPath = src.DBPath
	}
	if src.AnkiAccount != "" {
		dst.AnkiAccount = src.AnkiAccount
	}
	if src.HTTPAddr != "" {
		dst.HTTPAddr = src.HTTPAddr
	}
	if src.BackupKeepLastN > 0 {
		dst.BackupKeepLastN = src.BackupKeepLastN
	}
	if src.Workers > 0 {
		dst.Workers = src.Workers
	}
	if src.ReportFile != "" {
		dst.ReportFile = src.ReportFile
	}
	if src.DefaultPageSize > 0 {
		dst.DefaultPageSize = src.DefaultPageSize
	}
	if src.PragmaBusyTimeout > 0 {
		dst.PragmaBusyTimeout = src.PragmaBusyTimeout
	}
	if strings.TrimSpace(src.PragmaJournalMode) != "" {
		dst.PragmaJournalMode = strings.TrimSpace(src.PragmaJournalMode)
	}
	if strings.TrimSpace(src.PragmaSynchronous) != "" {
		dst.PragmaSynchronous = strings.TrimSpace(src.PragmaSynchronous)
	}
	dst.ForceApply = dst.ForceApply || src.ForceApply
	dst.Verbose = dst.Verbose || src.Verbose
	dst.FullDiff = dst.FullDiff || src.FullDiff
}

func defaultPathByOS(account string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	profile := "User 1"
	if strings.TrimSpace(account) != "" {
		profile = strings.TrimSpace(account)
	}
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(home, "AppData", "Roaming", "Anki2", profile, "collection.anki2")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Anki2", profile, "collection.anki2")
	default:
		return filepath.Join(home, ".local", "share", "Anki2", profile, "collection.anki2")
	}
}
