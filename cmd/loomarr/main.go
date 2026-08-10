// Command loomarr is the single-binary media-channel builder (design §2, §21).
// Phase 1 wires config, structured logging, and an HTTP server exposing /healthz
// and /readyz with graceful shutdown. Subsystems are added in later phases.
package main

import (
	"context"
	"errors"
	"fmt"
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
	if err := dispatch(os.Args[1:]); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// dispatch routes the command line. With no arguments — the ENTRYPOINT case — it boots
// the server, so the shipped image's behaviour is unchanged.
//
// ⚠ main() previously ignored os.Args entirely, which meant every argument booted a
// server. compose.yaml's healthcheck had been calling `/loomarr healthcheck` against
// that for as long as the file existed: the argument was discarded and a second full
// instance started, failed to bind the port, and exited non-zero every 30 seconds. The
// unknown-subcommand error below is the part that keeps this honest — a typo now fails
// visibly instead of quietly starting something.
func dispatch(args []string) error {
	if len(args) == 0 {
		return run()
	}
	switch args[0] {
	case "healthcheck":
		return runHealthcheck()
	default:
		return fmt.Errorf("%w: %q", errUnknownSubcommand, args[0])
	}
}

// run boots Loomarr and keeps booting it until a generation exits for a reason other
// than "restart" (§9.2).
//
// ⚠ **The loop IS the restart mechanism.** Loomarr never re-execs and never exits to be
// restarted by something else: `syscall.Exec` is a stub on Windows (compiles, fails at
// runtime), and exiting assumes a supervisor that `make dev-be` and any bare binary do
// not have. Rebuilding in place works identically on both platforms and cannot strand an
// operator with a dead service.
//
// Only work that must NOT re-run belongs out here. Everything a generation owns — config,
// store, background context, handler, HTTP server — is constructed inside runOnce and torn
// down before the next pass, because a value carried across generations is a value still
// pointing at the closed resources of the last one.
func run() error {
	// Config is loaded once for the log level only. Each generation re-loads it, so an
	// operator who edits a boot-time setting gets the new value on restart — which is the
	// entire point of the RestartRequired flag (config-design §3).
	bootCfg, err := config.Load()
	if err != nil {
		return err
	}
	// Logging is hoisted above the loop deliberately: slog.SetDefault mutates process
	// state, and re-running it per generation would be the exact package-level-mutation
	// hazard §9.2 warns about.
	log := newLogger(bootCfg.LogLevel)
	slog.SetDefault(log)

	for generation := 1; ; generation++ {
		restart, err := runOnce(log, generation)
		if err != nil {
			return err
		}
		if !restart {
			return nil
		}
		log.Info("restarting in place", "next_generation", generation+1)
	}
}

// runOnce is one generation: build everything, serve, tear it all down. It returns true
// when the operator asked for a restart, so run() can build the next generation.
//
// The returned bool and error are deliberately separate: "the operator restarted us" is
// not a failure, and collapsing it into an error would make a normal action look like one
// in the logs.
func runOnce(log *slog.Logger, generation int) (restart bool, err error) {
	cfg, err := config.Load()
	if err != nil {
		return false, err
	}
	// LLM_PROVIDER (llm.provider) is now a registry setting validated at settings
	// boot (an invalid enum fails there, config-design §3) — no separate check here.

	log.Info("loomarr starting", "listen", cfg.ListenAddr, "log_level", cfg.LogLevel, "generation", generation)

	// Open the store (§5): backend chosen by DATABASE_URL scheme, migrations run
	// on startup when AUTO_MIGRATE. If unset, run without a store for now (later
	// phases require it; readiness reflects the truth either way).
	var st store.Store
	if cfg.DatabaseURL != "" {
		st, err = store.Open(context.Background(), cfg.DatabaseURL, cfg.AutoMigrate)
		if err != nil {
			return false, err // downgrade guard / bad scheme / unreachable DB fail fast
		}
		// Closed on EVERY exit from this generation, including a restart — the next
		// generation opens its own handle, and a leaked one would hold the SQLite file
		// (and, on a migration, the database it just moved off).
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

	// restartCh carries the operator's restart request from the API handler back here.
	// Buffered so POST /v1/system/restart can respond and return WITHOUT waiting for this
	// loop to read it — a client that never gets a reply cannot tell "restarting" from
	// "crashed" (§7).
	restartCh := make(chan struct{}, 1)

	// Build the fully-wired API handler. This is the composition seam that the
	// integration harness also calls, so tests exercise the REAL wiring (§21).
	handler, err := app.BuildHandler(rootCtx, st, log, app.Overrides{
		DevLogin: cfg.DevLogin,
		Pprof:    cfg.Pprof,
		Restart: func() {
			// Non-blocking: a second click while the first restart is draining is a
			// no-op, not a queued second restart.
			select {
			case restartCh <- struct{}{}:
			default:
			}
		},
	})
	if err != nil {
		return false, err
	}

	// A FRESH server per generation. http.Server is single-use by design — Shutdown sets
	// `inShutdown` and never clears it, so a reused one would refuse to serve (§9.2).
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

	// Block until a signal, a restart request, or a server error.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		return false, err
	case <-restartCh:
		log.Info("restart requested")
		restart = true
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}
	// ⚠ Stop trapping signals BEFORE the next generation starts. NotifyContext keeps the
	// handler registered until stop() runs, and on a restart the deferred stop() would not
	// fire until this function returns — leaving the old generation's context able to
	// swallow the Ctrl-C an operator uses on the new one.
	stop()

	// Signal background work to stop alongside the HTTP drain (§7). On a restart this is
	// what tears down playout sessions: ffmpeg is spawned under rootCtx with Setpgid, so
	// cancelling kills each process GROUP rather than orphaning the children (§9.1).
	cancelRoot()

	// Graceful drain.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		// A drain that timed out is worth reporting, but on a restart it must not abort
		// the loop: the generation is finished either way, and refusing to come back up
		// because a client held a connection open is the outage this feature prevents.
		if !restart {
			return false, err
		}
		log.Warn("drain did not finish cleanly before restart", "err", err)
	}
	if restart {
		return true, nil
	}
	log.Info("loomarr stopped cleanly")
	return false, nil
}

// newLogger builds a JSON slog logger at the configured level (§14, §17).
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
