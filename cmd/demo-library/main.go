// Command demo-library runs Loomarr's demo library (#1587): an invented catalogue with generated
// artwork and video, served by an Emby-compatible stand-in media server, plus the idempotent
// setup that points a Loomarr backend at it.
//
//	demo-library generate   write the demo assets (clips, posters, icons, watermark) to DEMO_DIR
//	demo-library serve      generate if needed, then serve the stand-in on DEMO_LIBRARY_ADDR
//	demo-library seed       configure a running backend (BASE) against a running stand-in
//
// `make demo-library` and `make demo-seed` wrap serve and seed. Nothing here needs the LLM, TMDB
// or the network: every title is invented and every asset is generated locally with ffmpeg.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/loomarr/loomarr/internal/demolibrary"
)

// DemoToken is the stand-in's API key. It guards nothing (the server binds to loopback and serves
// invented media); it exists because Loomarr requires a complete media-server connection.
const DemoToken = "loomarr-demo-library"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: demo-library generate|serve|seed [flags]")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	dir := fs.String("dir", envOr("DEMO_DIR", ".agent-data/demo-library"), "directory for generated demo assets")
	ffmpeg := fs.String("ffmpeg", envOr("DEMO_FFMPEG", "ffmpeg"), "ffmpeg binary used to generate assets")
	addr := fs.String("addr", envOr("DEMO_LIBRARY_ADDR", defaultAddr()), "stand-in media server listen address")
	base := fs.String("base", os.Getenv("BASE"), "Loomarr backend URL to seed, e.g. http://localhost:18048")
	libraryURL := fs.String("library-url", os.Getenv("DEMO_LIBRARY_URL"), "stand-in URL as the backend reaches it (default http://<addr>)")
	dbURL := fs.String("database-url", os.Getenv("DATABASE_URL"), "the backend's store, for the approval gate")
	_ = fs.Parse(os.Args[2:])

	var err error
	switch os.Args[1] {
	case "generate":
		err = demolibrary.Generate(ctx, *dir, *ffmpeg, log)
	case "serve":
		err = serve(ctx, *dir, *ffmpeg, *addr, log)
	case "seed":
		if *libraryURL == "" {
			*libraryURL = "http://" + *addr
		}
		err = seed(ctx, seedConfig{dir: *dir, ffmpeg: *ffmpeg, base: *base, libraryURL: *libraryURL, databaseURL: *dbURL}, log)
	default:
		err = fmt.Errorf("unknown subcommand %q", os.Args[1])
	}
	if err != nil {
		log.Error("demo-library failed", "cmd", os.Args[1], "err", err)
		os.Exit(1)
	}
}

func serve(ctx context.Context, dir, ffmpeg, addr string, log *slog.Logger) error {
	if err := demolibrary.Generate(ctx, dir, ffmpeg, log); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: demolibrary.Server{Layout: demolibrary.Layout{Dir: dir}, Token: DemoToken}, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info("demo library serving", "url", "http://"+ln.Addr().String(), "token", DemoToken, "titles", len(demolibrary.Catalogue))
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// defaultAddr is 22000 + the worktree's dev slot (LOOMARR_AGENT_SLOT, exported by
// scripts/dev-env.sh), so each worktree's stand-in sits beside its other dev ports.
func defaultAddr() string {
	slot, _ := strconv.Atoi(os.Getenv("LOOMARR_AGENT_SLOT"))
	return "127.0.0.1:" + strconv.Itoa(22000+slot)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
