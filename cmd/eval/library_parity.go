package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/store"
)

const libraryParityName = "eval library-parity"

func init() {
	register("library-parity", "compare the file scanner with the media server's catalog, file by file (#2151 gate)",
		func(args []string) int {
			return runLibraryParity(context.Background(), args, os.Getenv, os.Stdout, os.Stderr)
		})
}

// runLibraryParity reads the scanner's files from a Loomarr database and the media server's from
// the server itself, both read-only, and prints the parity report. It exits 1 when the gate fails.
func runLibraryParity(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(libraryParityName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	database := flags.String("db", "", "the Loomarr database the scanner wrote, as sqlite://<path>")
	pathMap := flags.String("path-map", "", "the media server's paths to Loomarr's (from=>to, comma-separated); default: the database's derived map")
	flavor := flags.String("flavor", envOr(getenv, "LIBRARY_FLAVOR", "emby"), "emby or jellyfin")
	baseURL := flags.String("url", getenv("LIBRARY_URL"), "the media server")
	outPath := flags.String("out", "", "also write the whole report as JSON here")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	token := getenv("LIBRARY_TOKEN")
	if *database == "" || *baseURL == "" || token == "" {
		report(stderr, "%s: --db, --url (or LIBRARY_URL) and LIBRARY_TOKEN are required\n", libraryParityName)
		return 2
	}
	parsedFlavor, err := library.ParseFlavor(*flavor)
	if err != nil {
		return commandError(stderr, libraryParityName, "flavor", err)
	}

	st, err := store.Open(ctx, *database, false)
	if err != nil {
		return commandError(stderr, libraryParityName, "open the database", err)
	}
	defer func() { _ = st.Close() }()
	files, roots, derived, err := scan.ParityFiles(ctx, st, inventory.New(st))
	if err != nil {
		return commandError(stderr, libraryParityName, "read the scanner's files", err)
	}
	paths := derived
	if *pathMap != "" {
		paths = library.ParsePathMap(*pathMap)
	}
	server, err := library.New(parsedFlavor, *baseURL, token, "loomarr-eval").ServerFiles(ctx)
	if err != nil {
		return commandError(stderr, libraryParityName, "list the media server's files", err)
	}
	result := scan.Parity(server, files, paths, roots)
	if *outPath != "" {
		raw, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return commandError(stderr, libraryParityName, "encode the report", err)
		}
		if err := os.WriteFile(*outPath, raw, 0o600); err != nil {
			return commandError(stderr, libraryParityName, "write the report", err)
		}
	}
	writeParitySummary(stdout, result)
	if !result.Passed() {
		return 1
	}
	return 0
}

// writeParitySummary prints the counts, each explained group's size, and every unexplained finding.
func writeParitySummary(w io.Writer, r scan.ParityReport) {
	var b strings.Builder
	fmt.Fprintf(&b, "media server files %d · scanner files %d · paired %d\n", r.ServerFiles, r.ScannerFiles, r.Paired)
	fmt.Fprintf(&b, "identity agreed %d of %d paired\n", r.IdentityAgreed, r.Paired)
	fmt.Fprintf(&b, "durations agreed %d of %d compared (%.2f%%, gate %.1f%%); %d headers not read yet\n",
		r.DurationAgreed, r.DurationCompared, 100*r.DurationAgreement(), 100*scan.DurationAgreementGate, r.DurationUnread)
	explained := map[string]int{}
	var order []string
	for _, f := range r.Findings {
		if f.Explained == "" {
			continue
		}
		key := string(f.Kind) + ": " + f.Explained
		if explained[key] == 0 {
			order = append(order, key)
		}
		explained[key]++
	}
	if len(order) > 0 {
		b.WriteString("\nexplained:\n")
		for _, key := range order {
			fmt.Fprintf(&b, "  %6d  %s\n", explained[key], key)
		}
	}
	fmt.Fprintf(&b, "\nunexplained presence and identity findings: %d\n", r.Unexplained())
	for _, f := range r.Findings {
		if f.Explained == "" {
			fmt.Fprintf(&b, "  %-12s %s — %s (%s)\n", f.Kind, f.Path, f.Name, f.Detail)
		}
	}
	verdict := "PASS"
	if !r.Passed() {
		verdict = "FAIL"
	}
	fmt.Fprintf(&b, "\n%s\n", verdict)
	_, _ = io.WriteString(w, b.String())
}
