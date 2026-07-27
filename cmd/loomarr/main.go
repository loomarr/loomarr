// Command loomarr is the single-binary media-channel builder (design §2, §21).
// Phase 1 wires config, structured logging, and an HTTP server exposing /healthz
// and /readyz with graceful shutdown. Subsystems are added in later phases.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mantonx/loomarr/internal/app"
	"github.com/mantonx/loomarr/internal/config"
	"github.com/mantonx/loomarr/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// LLM_PROVIDER (llm.provider) is now a registry setting validated at settings
	// boot (an invalid enum fails there, config-design §3) — no separate check here.

	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)
	log.Info("loomarr starting", "listen", cfg.ListenAddr, "log_level", cfg.LogLevel)

	// Open the store (§5): backend chosen by DATABASE_URL scheme, migrations run
	// on startup when AUTO_MIGRATE. If unset, run without a store for now (later
	// phases require it; readiness reflects the truth either way).
	var st store.Store
	if cfg.DatabaseURL != "" {
		st, err = store.Open(context.Background(), cfg.DatabaseURL, cfg.AutoMigrate)
		if err != nil {
			return err // downgrade guard / bad scheme / unreachable DB fail fast
		}
		defer func() { _ = st.Close() }()
		log.Info("store opened", "auto_migrate", cfg.AutoMigrate)
	} else {
		log.Warn("no DATABASE_URL set — running without a store (not ready)")
	}

	// Background work (reconciler, sweeps, worker pools) runs under rootCtx;
	// shutdown cancels it alongside the HTTP drain.
	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()

	// A credential-free sign-in is worth shouting about on EVERY start, not once in a
	// changelog: the failure mode is an operator who turned it on months ago for a dev
	// session and never took it off (§11).
	if cfg.DevLogin {
		log.Warn("LOOMARR_DEV_LOGIN is set — POST /v1/auth/dev-login grants an admin session with NO credential. Never set this on an install you care about.")
	}
	// Same reasoning as dev-login: a profiling surface exposing stack traces and memory, with
	// no auth in front of it, is worth shouting about on every start rather than once.
	if cfg.Pprof {
		log.Warn("LOOMARR_PPROF is set — /debug/pprof/* is exposed UNAUTHENTICATED. Development only; never leave this on.")
	}

	// Build the fully-wired API handler. This is the composition seam that the
	// integration harness also calls, so tests exercise the REAL wiring (§21).
	handler, err := app.BuildHandler(rootCtx, st, log, app.Overrides{DevLogin: cfg.DevLogin, Pprof: cfg.Pprof})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Run the server; surface a listen error via this channel.
	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Block until a signal or a server error.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		slog.Default().Info("shutdown signal received")
	}

	// Signal background work to stop alongside the HTTP drain (§7).
	cancelRoot()

	// Graceful drain.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	slog.Default().Info("loomarr stopped cleanly")
	return nil
}

// newLogger builds a JSON slog logger at the configured level (§14, §17).
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
