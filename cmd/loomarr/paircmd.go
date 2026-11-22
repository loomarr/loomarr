package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// defaultPairDeviceName is the label the approver sees in Settings → Devices. It is fixed rather
// than the hostname because `loomarr pair` usually runs in a throwaway container, whose hostname is
// a random ID that would tell the approver nothing.
const defaultPairDeviceName = "Agent CLI"

// maxPairDeviceName mirrors deviceStartInput's maxLength in internal/api/people/deviceroutes.go.
const maxPairDeviceName = 64

// runPair is `loomarr pair --server <url> [--name <label>]`.
//
// STDOUT CARRIES THE TOKEN AND NOTHING ELSE. The person-facing lines (where to approve, what to do
// next) go to stderr, so the documented use works from the release image, which is the only way the
// binary ships:
//
//	export LOOMARR_TOKEN=$(docker run --rm ghcr.io/loomarr/loomarr pair --server https://tv.example)
//
// It deliberately writes no config file: inside a container it could not reach the host's Restish
// config anyway, and where the token is kept (a keyring, a password manager, an .envrc) is the
// person's choice, not Loomarr's.
func runPair(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("pair", flag.ContinueOnError)
	fs.SetOutput(stderr)
	server := fs.String("server", os.Getenv("LOOMARR_SERVER"), "Loomarr's address, e.g. http://nas.local:8080 (or set LOOMARR_SERVER)")
	name := fs.String("name", defaultPairDeviceName, "label shown in Settings → Devices")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "Usage: loomarr pair --server <url> [--name <label>]")
		_, _ = fmt.Fprintln(stderr, "\nPairs a CLI or agent as a device and prints its token on stdout.")
		_, _ = fmt.Fprintln(stderr, "It acts as whoever approves it, with their role. Revoke it in Settings → Devices.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("pair takes no arguments, got %q; use --server", fs.Arg(0))
	}
	// The server's limit (deviceStartInput maxLength); checked here because the server's 422
	// would reach the person as a bare status code.
	if n := len([]rune(*name)); n > maxPairDeviceName {
		return fmt.Errorf("--name is %d characters; the device list allows at most %d", n, maxPairDeviceName)
	}

	p, err := newPairing(*server)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return pairAndPrint(ctx, p, *name, stdout, stderr)
}

// pairAndPrint is runPair after flag parsing, split out so tests drive it with a fake clock.
func pairAndPrint(ctx context.Context, p *pairing, name string, stdout, stderr io.Writer) error {
	res, err := p.run(ctx, name, stderr)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return errors.New("pairing stopped before it was approved")
		}
		return err
	}
	if _, err := fmt.Fprintln(stdout, res.Token); err != nil {
		return fmt.Errorf("write token: %w", err)
	}
	printPairNextSteps(stderr, p, res.DeviceName)
	return nil
}

// printPairNextSteps tells the person how to use the token with Restish (2.x).
//
// `--spec` is required: Restish's discovery tries /openapi.json at the root, and Loomarr serves it
// under /v1. The commands are `api connect` then `api set` rather than a config snippet to paste,
// because a hand-written restish.json is not discovered until `restish api sync` runs (verified on
// Restish 2.3.0), and `connect` also writes the file with owner-only permissions.
func printPairNextSteps(w io.Writer, p *pairing, deviceName string) {
	_, _ = fmt.Fprintf(w, "\nPaired as %q. The token is on stdout; keep it like a password.\n", deviceName)
	_, _ = fmt.Fprintf(w, "To use it with Restish, export it as LOOMARR_TOKEN and run once:\n\n")
	_, _ = fmt.Fprintf(w, "  restish api connect loomarr %s --spec %s\n", shellQuote(p.base.String()), shellQuote(p.endpoint("/v1/openapi.json")))
	_, _ = fmt.Fprintf(w, "  restish api set loomarr 'profiles.default.auth.type: bearer' 'profiles.default.auth.params.token: env:LOOMARR_TOKEN'\n")
}

// shellQuote wraps s in single quotes for a POSIX shell, so a URL with & or ? survives a paste.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
