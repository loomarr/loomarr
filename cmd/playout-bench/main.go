// Command playout-bench runs a generated, redistributable corpus through Loomarr's real playout
// pipeline builder on this host and writes one standard report: start p95, speed, CPU per stream,
// concurrency, gaps, SPS identity, loudness and VMAF. The report is judged against the beta.8
// thresholds and diffed against the last accepted baseline for the host's hardware family. See
// docs/dev/playout-bench.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/playoutbench"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "playout-bench:", err)
		os.Exit(1)
	}
}

func run() error {
	artifacts := os.Getenv("LOOMARR_ARTIFACT_DIR")
	if artifacts == "" {
		artifacts = filepath.Join(os.TempDir(), "loomarr-playout-bench")
	}
	var (
		ffmpeg     = flag.String("ffmpeg", "ffmpeg", "ffmpeg binary (ffprobe is taken from beside it)")
		family     = flag.String("family", "", "force a hardware family: software, vaapi, nvenc, videotoolbox (default: detect as the app does)")
		height     = flag.Int("height", 1080, "output rung height, 1080 or 720")
		corpus     = flag.String("corpus", filepath.Join(artifacts, "playout-bench-corpus"), "directory for the generated corpus")
		reportDir  = flag.String("report-dir", artifacts, "where the JSON and markdown reports are written")
		baselines  = flag.String("baseline-dir", "docs/engineering/playout-bench", "directory of accepted baselines, one <family>.json each")
		mode       = flag.String("thresholds", string(playoutbench.ModeFull), "full: every beta.8 threshold; correctness: gaps, SPS, loudness only (virtualised hosts); off")
		startRuns  = flag.Int("start-runs", 20, "fresh-process start-latency runs per class")
		maxStream  = flag.Int("max-streams", 16, "highest concurrency to try")
		tolerance  = flag.Float64("tolerance", playoutbench.DefaultTolerance().Relative, "relative regression tolerance against the baseline")
		films      = flag.String("films", "", "directory of open films fetched by scripts/playout-bench-open-films.sh (optional)")
		noVMAF     = flag.Bool("no-vmaf", false, "skip the VMAF measurement")
		accept     = flag.Bool("accept", false, "write this report as the family's baseline (refused if a threshold fails)")
		corpusOnly = flag.Bool("corpus-only", false, "generate the corpus cache and exit; run this outside any shared lock, before the measured run")
		commit     = flag.String("commit", "", "source commit under test (default: git HEAD)")
	)
	flag.Parse()
	if *height != 1080 && *height != 720 {
		return fmt.Errorf("--height must be 1080 or 720")
	}
	if *commit == "" {
		out, _ := exec.Command("git", "rev-parse", "--short=12", "HEAD").Output()
		*commit = strings.TrimSpace(string(out))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if *corpusOnly {
		dir := playoutbench.CorpusDir(*corpus)
		start := time.Now()
		clips, skipped, err := playoutbench.Generate(ctx, *ffmpeg, dir)
		if err != nil {
			return err
		}
		for name, why := range skipped {
			fmt.Fprintf(os.Stderr, "playout-bench: clip %s skipped: %s\n", name, why)
		}
		fmt.Printf("corpus %s: %d clips in %s\n", dir, len(clips), time.Since(start).Round(time.Millisecond))
		return nil
	}

	rep, err := playoutbench.Run(ctx, playoutbench.Options{
		FFmpeg: *ffmpeg, Dir: *corpus, Family: *family, Height: *height,
		StartRuns: *startRuns, MaxStreams: *maxStream, VMAF: !*noVMAF, Films: *films, Commit: *commit,
		Log: func(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...) },
	})
	if err != nil {
		return err
	}

	var verdicts []playoutbench.Verdict
	if playoutbench.Mode(*mode) != "off" {
		verdicts = playoutbench.Judge(rep, playoutbench.Mode(*mode))
	}
	baselinePath := filepath.Join(*baselines, rep.Family+".json")
	var deltas []playoutbench.Delta
	regressed := false
	if base, err := playoutbench.Read(baselinePath); err == nil {
		tol := playoutbench.DefaultTolerance()
		tol.Relative = *tolerance
		if deltas, regressed, err = playoutbench.Compare(base, rep, tol); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	name := "playout-bench-" + rep.Family
	if err := rep.Write(filepath.Join(*reportDir, name+".json")); err != nil {
		return err
	}
	markdown := rep.Markdown(verdicts, deltas)
	if err := os.WriteFile(filepath.Join(*reportDir, name+".md"), []byte(markdown), 0o644); err != nil {
		return err
	}
	fmt.Println(markdown)
	if summary := os.Getenv("GITHUB_STEP_SUMMARY"); summary != "" {
		if f, err := os.OpenFile(summary, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644); err == nil {
			_, _ = f.WriteString(markdown + "\n")
			_ = f.Close()
		}
	}

	failures := playoutbench.Failures(verdicts)
	var problems []string
	if failures != "" {
		problems = append(problems, "thresholds failed:\n"+failures)
	}
	if regressed {
		problems = append(problems, fmt.Sprintf("regressed against %s (tolerance %.0f%%)", baselinePath, *tolerance*100))
	}
	for _, c := range rep.Cases {
		if c.Status == "failed" {
			problems = append(problems, fmt.Sprintf("clip %s failed: %s", c.Clip, c.Detail))
		}
	}
	if *accept {
		if len(problems) > 0 {
			return fmt.Errorf("refusing to accept a report that fails its gates:\n%s", strings.Join(problems, "\n"))
		}
		if err := rep.Write(baselinePath); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "accepted as", baselinePath)
		return nil
	}
	if _, err := os.Stat(baselinePath); err != nil {
		fmt.Fprintf(os.Stderr, "no accepted baseline at %s: thresholds judged, regressions not checked\n", baselinePath)
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	return nil
}
