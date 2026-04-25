package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"ankiced/internal/bootstrap"
	configinfra "ankiced/internal/infrastructure/config"
	sqliteinfra "ankiced/internal/infrastructure/sqlite"
	"ankiced/internal/interfaces/cli"
)

func main() {
	if err := run(); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err.Error()); writeErr != nil {
			if _, secondErr := os.Stderr.WriteString("fatal: failed to write error output\n"); secondErr != nil {
				os.Exit(1)
			}
		}
		os.Exit(1)
	}
}

func run() error {
	// Top-level process context, cancelled by OS shutdown signals.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	bootstrapVerbose := bootstrap.VerboseRequested(os.Args[1:]) || bootstrap.EnvEnabled("ANKICED_VERBOSE")
	logger := bootstrap.NewLogger(bootstrapVerbose)
	logger.Info("starting ankiced")
	loader := configinfra.Loader{}
	cfg, err := loader.Load(ctx, os.Args[1:])
	if err != nil {
		return bootstrap.Fail(logger, bootstrap.ErrLoadConfigPrefix, err, bootstrapVerbose)
	}
	logger = bootstrap.NewLogger(cfg.Verbose)
	logger.Info("config loaded", "db_path", cfg.DBPath, "workers", cfg.Workers, "verbose", cfg.Verbose)

	db, err := sqliteinfra.Open(cfg.DBPath, sqliteinfra.Pragmas{
		BusyTimeoutMS: cfg.PragmaBusyTimeout,
		JournalMode:   cfg.PragmaJournalMode,
		Synchronous:   cfg.PragmaSynchronous,
	})
	if err != nil {
		return bootstrap.Fail(logger, bootstrap.ErrOpenDBPrefix, err, cfg.Verbose)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			logger.Error("database close", "error", bootstrap.FormatErrorForMode(closeErr, cfg.Verbose))
		}
	}()
	logger.Info("database opened")

	svc := bootstrap.NewServices(cfg, db, cli.Prompter{In: os.Stdin, Out: os.Stdout})
	defer func() {
		// Cleanup should not block shutdown forever.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), bootstrap.ShutdownTimeout)
		defer cancel()
		if err := svc.CleanupBackups(cleanupCtx, cfg); err != nil {
			logger.Error(bootstrap.ErrBackupCleanup, "error", bootstrap.FormatErrorForMode(err, cfg.Verbose))
			if _, writeErr := fmt.Fprintf(os.Stderr, "%s: %s\n", bootstrap.ErrBackupCleanup, bootstrap.FormatErrorForMode(err, cfg.Verbose)); writeErr != nil {
				logger.Error("stderr write", "error", writeErr.Error())
			}
		}
	}()

	app := cli.App{Svc: svc, Cfg: cfg, In: os.Stdin, Out: os.Stdout}
	if err := app.Run(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Info("shutdown requested")
			return nil
		}
		return bootstrap.Fail(logger, bootstrap.ErrRuntimePrefix, err, cfg.Verbose)
	}
	logger.Info("ankiced finished")
	return nil
}
