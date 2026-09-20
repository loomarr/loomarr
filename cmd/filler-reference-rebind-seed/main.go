// Command filler-reference-rebind-seed advances the retained v3 inspection
// seed to exact current Gate A and duplicate-family artifact identities.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/fillerreference"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-reference-rebind-seed", flag.ContinueOnError)
	flags.SetOutput(stderr)
	seedPath := flags.String("seed", "", "retained v3 inspection seed")
	auditPath := flags.String("audit", "", "current bound filler reference audit")
	familyPath := flags.String("families", "", "current bound duplicate-family audit")
	outputPath := flags.String("output", "", "new output directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *seedPath == "" || *auditPath == "" || *familyPath == "" || *outputPath == "" || flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "filler-reference-rebind-seed: seed, audit, families, and output are required")
		return 2
	}
	seed, err := os.ReadFile(*seedPath)
	if err != nil {
		return fail(stderr, err)
	}
	audit, err := os.ReadFile(*auditPath)
	if err != nil {
		return fail(stderr, err)
	}
	families, err := os.ReadFile(*familyPath)
	if err != nil {
		return fail(stderr, err)
	}
	result, err := fillerreference.RebindInspectionSeed(seed, audit, families)
	if err != nil {
		return fail(stderr, err)
	}
	report, err := json.MarshalIndent(result.Report, "", "  ")
	if err != nil {
		return fail(stderr, err)
	}
	if err := publishBundle(*outputPath, map[string][]byte{
		"inspection-seed.json": result.Seed,
		"rebind-report.json":   append(report, '\n'),
	}); err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "filler-reference-rebind-seed: rebound %d selected cases; seed sha256 %s\n", result.Report.SelectedCaseCount, result.Report.OutputSeedSHA256)
	return 0
}

func publishBundle(path string, files map[string][]byte) error {
	target, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("output already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".filler-reference-seed-*")
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := os.Chmod(stage, 0o700); err != nil {
		return err
	}
	for _, name := range []string{"inspection-seed.json", "rebind-report.json"} {
		file, err := os.OpenFile(filepath.Join(stage, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if _, err := file.Write(files[name]); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	if err := os.Rename(stage, target); err != nil {
		return err
	}
	published = true
	return nil
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, "filler-reference-rebind-seed:", err)
	return 1
}
