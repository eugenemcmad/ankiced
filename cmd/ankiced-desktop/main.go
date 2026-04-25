package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"ankiced/internal/bootstrap"
	configinfra "ankiced/internal/infrastructure/config"
	sqliteinfra "ankiced/internal/infrastructure/sqlite"
	"ankiced/internal/interfaces/httpapi"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
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
	bootstrapVerbose := bootstrap.VerboseRequested(os.Args[1:]) || bootstrap.EnvEnabled("ANKICED_VERBOSE")
	logger := bootstrap.NewLogger(bootstrapVerbose)
	loader := configinfra.Loader{}
	cfg, err := loader.Load(context.Background(), os.Args[1:])
	if err != nil {
		return bootstrap.Fail(logger, bootstrap.ErrLoadConfigPrefix, err, bootstrapVerbose)
	}
	logger = bootstrap.NewLogger(cfg.Verbose)

	db, err := sqliteinfra.Open(cfg.DBPath, sqliteinfra.Pragmas{
		BusyTimeoutMS: cfg.PragmaBusyTimeout,
		JournalMode:   cfg.PragmaJournalMode,
		Synchronous:   cfg.PragmaSynchronous,
	})
	if err != nil {
		return bootstrap.Fail(logger, bootstrap.ErrOpenDBPrefix, err, cfg.Verbose)
	}
	var dbCloseOnce sync.Once
	closeDB := func() {
		dbCloseOnce.Do(func() {
			if closeErr := db.Close(); closeErr != nil {
				logger.Error("database close", "error", bootstrap.FormatErrorForMode(closeErr, cfg.Verbose))
			}
		})
	}
	defer closeDB()

	svc := bootstrap.NewServices(cfg, db, nil)

	// appCtx is set on the Wails main loop goroutine and read from the HTTP
	// handler goroutine when the operator hits /api/v1/app/exit. Use an atomic
	// pointer to avoid a data race on the value.
	var appCtxPtr atomic.Pointer[context.Context]
	handler := httpapi.NewHandlerWithExit(svc, cfg, logger, func() {
		if ctxPtr := appCtxPtr.Load(); ctxPtr != nil && *ctxPtr != nil {
			runtime.Quit(*ctxPtr)
		}
	})

	err = wails.Run(&options.App{
		Title:  "Ankiced Desktop",
		Width:  1200,
		Height: 820,
		AssetServer: &assetserver.Options{
			Handler: handler,
		},
		OnStartup: func(ctx context.Context) {
			appCtxPtr.Store(&ctx)
			logger.Info("ankiced desktop started")
		},
		OnShutdown: func(context.Context) {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), bootstrap.ShutdownTimeout)
			defer cancel()
			if cleanupErr := svc.CleanupBackups(cleanupCtx, cfg); cleanupErr != nil {
				logger.Error(bootstrap.ErrBackupCleanup, "error", bootstrap.FormatErrorForMode(cleanupErr, cfg.Verbose))
			}
			closeDB()
		},
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return bootstrap.Fail(logger, bootstrap.ErrRuntimePrefix, err, cfg.Verbose)
	}
	return nil
}
