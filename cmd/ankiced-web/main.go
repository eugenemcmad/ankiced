package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ankiced/internal/bootstrap"
	"ankiced/internal/infrastructure/browser"
	configinfra "ankiced/internal/infrastructure/config"
	sqliteinfra "ankiced/internal/infrastructure/sqlite"
	"ankiced/internal/interfaces/httpapi"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	bootstrapVerbose := bootstrap.VerboseRequested(os.Args[1:]) || bootstrap.EnvEnabled("ANKICED_VERBOSE")
	logger := bootstrap.NewLogger(bootstrapVerbose)
	loader := configinfra.Loader{}
	cfg, err := loader.Load(ctx, os.Args[1:])
	if err != nil {
		return bootstrap.Fail(logger, bootstrap.ErrLoadConfigPrefix, err, bootstrapVerbose)
	}
	logger = bootstrap.NewLogger(cfg.Verbose)
	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		cfg.HTTPAddr = "127.0.0.1:8080"
	}
	resolvedAddr, err := resolveHTTPAddr(cfg.HTTPAddr, logger)
	if err != nil {
		return bootstrap.Fail(logger, bootstrap.ErrRuntimePrefix, err, cfg.Verbose)
	}
	cfg.HTTPAddr = resolvedAddr

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

	svc := bootstrap.NewServices(cfg, db, nil)
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), bootstrap.ShutdownTimeout)
		defer cancel()
		if err := svc.CleanupBackups(cleanupCtx, cfg); err != nil {
			logger.Error(bootstrap.ErrBackupCleanup, "error", bootstrap.FormatErrorForMode(err, cfg.Verbose))
		}
	}()

	baseURL := "http://" + cfg.HTTPAddr
	go openBrowserWhenReady(ctx, logger, baseURL)
	api := httpapi.Server{Svc: svc, Cfg: cfg, Logger: logger, OnExit: stop}
	if err := api.Run(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return bootstrap.Fail(logger, bootstrap.ErrRuntimePrefix, err, cfg.Verbose)
	}
	return nil
}

func openBrowserWhenReady(ctx context.Context, logger *slog.Logger, baseURL string) {
	healthURL := baseURL + "/healthz"
	client := &http.Client{Timeout: 700 * time.Millisecond}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		resp, err := client.Get(healthURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				if err := browser.Open(baseURL + "/"); err != nil {
					logger.Warn("open browser failed", "error", err.Error(), "url", baseURL)
				}
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	logger.Warn("browser auto-open skipped", "reason", "server not ready in time", "url", baseURL)
}

func resolveHTTPAddr(addr string, logger *slog.Logger) (string, error) {
	l, err := net.Listen("tcp", addr)
	if err == nil {
		if closeErr := l.Close(); closeErr != nil {
			return "", closeErr
		}
		return addr, nil
	}
	if !strings.Contains(strings.ToLower(err.Error()), "address already in use") &&
		!strings.Contains(strings.ToLower(err.Error()), "only one usage of each socket address") {
		return "", err
	}
	if addr != "127.0.0.1:8080" {
		return "", err
	}
	fallback, fbErr := net.Listen("tcp", "127.0.0.1:0")
	if fbErr != nil {
		return "", err
	}
	resolved := fallback.Addr().String()
	if closeErr := fallback.Close(); closeErr != nil {
		return "", closeErr
	}
	logger.Warn("default http address busy, using free localhost port", "requested_addr", addr, "resolved_addr", resolved)
	return resolved, nil
}
