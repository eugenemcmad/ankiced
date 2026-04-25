package bootstrap

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"ankiced/internal/application"
	appconfig "ankiced/internal/config"
	fsinfra "ankiced/internal/infrastructure/fs"
	"ankiced/internal/infrastructure/render"
	"ankiced/internal/infrastructure/sanitize"
	sqliteinfra "ankiced/internal/infrastructure/sqlite"
	"ankiced/internal/presentation"
)

const (
	ErrLoadConfigPrefix = "load config"
	ErrOpenDBPrefix     = "open db"
	ErrRuntimePrefix    = "runtime"
	ErrBackupCleanup    = "backup cleanup"
	ShutdownTimeout     = 5 * time.Second
)

func NewLogger(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func Fail(logger *slog.Logger, prefix string, err error, verbose bool) error {
	formatted := FormatErrorForMode(err, verbose)
	logger.Error(prefix, "error", formatted)
	return fmt.Errorf("%s: %s", prefix, formatted)
}

func FormatErrorForMode(err error, verbose bool) string {
	if verbose {
		return presentation.FormatDebugError(err)
	}
	return presentation.FormatError(err)
}

func VerboseRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--verbose" {
			return true
		}
		if strings.HasPrefix(arg, "--verbose=") {
			value := strings.TrimPrefix(arg, "--verbose=")
			return strings.EqualFold(value, "true") || value == "1"
		}
	}
	return false
}

func EnvEnabled(name string) bool {
	value := os.Getenv(name)
	return strings.EqualFold(value, "true") || value == "1"
}

func NewServices(cfg appconfig.Settings, db *sqliteinfra.DB, confirm application.ConfirmPrompter) application.Services {
	return application.Services{
		Cfg:       cfg,
		Decks:     sqliteinfra.NewDeckRepo(db),
		Notes:     sqliteinfra.NewNoteRepo(db),
		Models:    sqliteinfra.NewModelRepo(db),
		Backups:   &fsinfra.BackupStore{},
		Confirm:   confirm,
		Diff:      render.DiffRenderer{},
		Reports:   render.JSONReportWriter{},
		Tx:        db,
		Templates: sanitize.NewTemplateRegistry(),
	}
}
