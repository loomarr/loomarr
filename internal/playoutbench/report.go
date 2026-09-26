// Package playoutbench is the cross-hardware playout bench (#1512 G8): a redistributable corpus run
// through Loomarr's real playout pipeline builder (playout.Build), one standard report, judged
// against the beta.8 thresholds and diffed against the last accepted report per hardware family.
//
// Methodology (draft PR #1513): measure the time to the first N seconds of media, never bytes, and
// count CPU as child rusage per second of media, never a sampled percentage.
package playoutbench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// SchemaVersion changes when a metric's meaning does, which invalidates every committed baseline.
const SchemaVersion = 1

// Better says which direction of change is an improvement.
type Better string

const (
	Lower  Better = "lower"
	Higher Better = "higher"
	// Exact metrics are correctness counts (PTS gaps, distinct SPS): any change is a regression.
	Exact Better = "exact"
)

// Metric is one measured number.
type Metric struct {
	Value  float64 `json:"value"`
	Unit   string  `json:"unit"`
	Better Better  `json:"better"`
}

// Host identifies where the report was measured.
type Host struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	CPUs    int    `json:"cpus"`
	FFmpeg  string `json:"ffmpeg"`
	Encoder string `json:"encoder"`
	// Rung is the measured output profile (height and fps), part of what a baseline is a baseline of.
	Rung string `json:"rung"`
}

// Case is one corpus clip's outcome, for the human report. The numbers live in Metrics.
type Case struct {
	Clip      string   `json:"clip"`
	Class     string   `json:"class"`
	Status    string   `json:"status"` // ok | refused | failed
	Detail    string   `json:"detail,omitempty"`
	Fallbacks []string `json:"fallbacks,omitempty"`
}

// Report is the standard bench report. Metrics is flat (`<name>/<class>`) so comparison and
// thresholds are generic over it.
type Report struct {
	SchemaVersion int               `json:"schemaVersion"`
	Family        string            `json:"family"`
	Commit        string            `json:"commit"`
	Corpus        string            `json:"corpus"`
	GeneratedAt   time.Time         `json:"generatedAt"`
	Host          Host              `json:"host"`
	Metrics       map[string]Metric `json:"metrics"`
	// Skipped names the metrics this host could not measure and why (no libvmaf, refused HDR). A
	// skipped metric is a warning against a baseline that has it, never a silent pass.
	Skipped map[string]string `json:"skipped,omitempty"`
	Cases   []Case            `json:"cases"`
}

// NewReport starts an empty report.
func NewReport(family, commit, corpus string, host Host) *Report {
	return &Report{SchemaVersion: SchemaVersion, Family: family, Commit: commit, Corpus: corpus,
		GeneratedAt: time.Now().UTC(), Host: host, Metrics: map[string]Metric{}, Skipped: map[string]string{}}
}

// Set records one metric.
func (r *Report) Set(name string, value float64, unit string, better Better) {
	r.Metrics[name] = Metric{Value: value, Unit: unit, Better: better}
}

// Write stores the report as indented JSON with sorted keys (map order is sorted by encoding/json),
// so an accepted baseline diffs cleanly in review.
func (r *Report) Write(path string) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

// Read loads a report and rejects a schema this code does not understand.
func Read(path string) (*Report, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Report
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if r.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("%s: schema %d, this bench writes %d; re-accept the baseline", path, r.SchemaVersion, SchemaVersion)
	}
	return &r, nil
}

// Names returns the metric names in stable order.
func (r *Report) Names() []string {
	names := make([]string, 0, len(r.Metrics))
	for n := range r.Metrics {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// Markdown renders the human report: verdicts first, then the regression diff, then every metric.
func (r *Report) Markdown(verdicts []Verdict, deltas []Delta) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Playout bench: %s\n\n", r.Family)
	fmt.Fprintf(&b, "- commit `%s`, corpus `%s`, %s\n", r.Commit, r.Corpus, r.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- host %s/%s, %d CPUs, encoder `%s`, %s\n\n", r.Host.OS, r.Host.Arch, r.Host.CPUs, r.Host.Encoder, r.Host.FFmpeg)
	if len(verdicts) > 0 {
		b.WriteString("## Thresholds\n\n| metric | limit | measured | |\n|---|---|---|---|\n")
		for _, v := range verdicts {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", v.Metric, v.Limit, v.Measured, mark(v.Pass))
		}
		b.WriteString("\n")
	}
	if len(deltas) > 0 {
		b.WriteString("## Against the accepted baseline\n\n| metric | baseline | now | change | |\n|---|---|---|---|---|\n")
		for _, d := range deltas {
			fmt.Fprintf(&b, "| %s | %.4g | %.4g | %+.1f%% | %s |\n", d.Metric, d.Baseline, d.Current, d.Change*100, d.Status)
		}
		b.WriteString("\n")
	}
	b.WriteString("## Metrics\n\n| metric | value | unit |\n|---|---|---|\n")
	for _, n := range r.Names() {
		m := r.Metrics[n]
		fmt.Fprintf(&b, "| %s | %.4g | %s |\n", n, m.Value, m.Unit)
	}
	if len(r.Skipped) > 0 {
		b.WriteString("\n## Not measured\n\n")
		for _, n := range sortedKeys(r.Skipped) {
			fmt.Fprintf(&b, "- `%s`: %s\n", n, r.Skipped[n])
		}
	}
	b.WriteString("\n## Clips\n\n| clip | class | status | detail |\n|---|---|---|---|\n")
	for _, c := range r.Cases {
		detail := c.Detail
		if len(c.Fallbacks) > 0 {
			detail = strings.TrimSpace(detail + " fallbacks: " + strings.Join(c.Fallbacks, "; "))
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", c.Clip, c.Class, c.Status, detail)
	}
	return b.String()
}

func mark(ok bool) string {
	if ok {
		return "pass"
	}
	return "**FAIL**"
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
