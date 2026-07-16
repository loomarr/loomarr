package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mantonx/loomarr/internal/httpx"
)

// Probe is the detected state of the LOCAL model host + machine (§8.1). All fields
// are best-effort: a missing GPU or an unreachable Ollama degrades the field, never
// the whole probe — the UI shows what we know and the recommendation widens its
// bands rather than failing.
type Probe struct {
	VRAMGiB       float64  `json:"vramGiB"`       // detected GPU VRAM; 0 = unknown/none
	GPUName       string   `json:"gpuName"`       // e.g. "NVIDIA GeForce RTX 3080 Ti"; "" = unknown
	OllamaVersion string   `json:"ollamaVersion"` // e.g. "0.13.5"; "" = unreachable/unknown
	PulledModels  []string `json:"pulledModels"`  // tags already present locally
	Reachable     bool     `json:"reachable"`     // Ollama /api/version answered
}

// Prober detects the machine + Ollama host. baseURL is the Ollama base
// (http://…:11434, NOT the /v1 compat base). nvidiaSMI is the command used to read
// VRAM — injectable so tests don't shell out.
type Prober struct {
	baseURL   string
	http      *http.Client
	nvidiaSMI func(ctx context.Context) (name string, vramGiB float64, ok bool)
}

// NewProber builds a Prober for the given Ollama base URL.
func NewProber(baseURL string) *Prober {
	return &Prober{
		baseURL:   strings.TrimRight(baseURL, "/"),
		http:      httpx.New(httpx.TimeoutProbe),
		nvidiaSMI: nvidiaSMIVRAM,
	}
}

// Probe runs the full detection. It never returns an error for a degraded
// dependency (unreachable Ollama, no GPU) — those are reflected in the fields so
// the caller can render "GPU not detected" instead of a 500.
func (p *Prober) Probe(ctx context.Context) Probe {
	out := Probe{}
	if name, vram, ok := p.nvidiaSMI(ctx); ok {
		out.GPUName, out.VRAMGiB = name, vram
	}
	if v, ok := p.ollamaVersion(ctx); ok {
		out.OllamaVersion, out.Reachable = v, true
	}
	out.PulledModels = p.pulledModels(ctx) // nil on failure — fine
	return out
}

// Catalog returns the curated model catalog annotated for THIS machine (fit,
// pulled, runtime, recommended).
func (p *Prober) Catalog(probe Probe) []CatalogEntry { return annotateCatalog(probe) }

func (p *Prober) ollamaVersion(ctx context.Context) (string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/version", nil)
	if err != nil {
		return "", false
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return "", false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var v struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", false
	}
	return v.Version, true
}

func (p *Prober) pulledModels(ctx context.Context) []string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/tags", nil)
	if err != nil {
		return nil
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil
	}
	tags := make([]string, 0, len(out.Models))
	for _, m := range out.Models {
		tags = append(tags, m.Name)
	}
	return tags
}

// nvidiaSMIVRAM reads the primary GPU's name + total VRAM via nvidia-smi. Returns
// ok=false on any failure (no binary, no GPU, parse error) — the caller treats that
// as "VRAM unknown". Only the first GPU is read; a multi-GPU host is out of the
// household envelope (§1) and the first card is the right heuristic.
func nvidiaSMIVRAM(ctx context.Context) (string, float64, bool) {
	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=name,memory.total", "--format=csv,noheader,nounits")
	stdout, err := cmd.Output()
	if err != nil {
		return "", 0, false
	}
	// First line: "NVIDIA GeForce RTX 3080 Ti, 12288"
	line, _ := bufio.NewReader(strings.NewReader(string(stdout))).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return "", 0, false
	}
	parts := strings.Split(line, ",")
	if len(parts) < 2 {
		return "", 0, false
	}
	name := strings.TrimSpace(parts[0])
	megs, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil || megs <= 0 {
		return name, 0, false
	}
	return name, megs / 1024.0, true
}

// versionAtLeast reports whether have >= want, comparing dotted numeric segments
// (0.14.0 >= 0.13.5). An empty want ⇒ true (no minimum). An unparseable/empty have
// with a non-empty want ⇒ false (we can't confirm the runtime, so don't claim it).
func versionAtLeast(have, want string) bool {
	if want == "" {
		return true
	}
	if have == "" {
		return false
	}
	hs, ws := splitVersion(have), splitVersion(want)
	for i := 0; i < len(ws); i++ {
		var h int
		if i < len(hs) {
			h = hs[i]
		}
		if h != ws[i] {
			return h > ws[i]
		}
	}
	return true // equal through the compared segments
}

func splitVersion(v string) []int {
	// Trim any pre-release/build suffix (e.g. "0.14.0-rc1" → "0.14.0").
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	segs := strings.Split(v, ".")
	out := make([]int, 0, len(segs))
	for _, s := range segs {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			break // stop at the first non-numeric segment
		}
		out = append(out, n)
	}
	return out
}

// pullProgress is one raw Ollama /api/pull streaming line.
type pullProgress struct {
	Status    string `json:"status"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
	Error     string `json:"error"`
}

// PullProgress is one pull update handed to the Pull callback. Completed/Total are
// bytes for the layer currently downloading (both 0 before totals are known, e.g.
// "pulling manifest"). Raw bytes are surfaced — not just Percent — so a UI can show
// "X of Y GB" and derive rate/ETA from successive frames (§8.1 download bar).
type PullProgress struct {
	Status    string
	Completed int64
	Total     int64
}

// Percent returns 0..100 for this update, or -1 if unknown (no totals yet).
func (pp PullProgress) Percent() int {
	if pp.Total <= 0 {
		return -1
	}
	pct := int(pp.Completed * 100 / pp.Total)
	if pct > 100 {
		pct = 100
	}
	return pct
}

// Pull streams an Ollama model pull, invoking onProgress for each status line
// (percent -1 when no byte totals are available yet, e.g. "pulling manifest"). It
// blocks until the pull completes, errors, or ctx is cancelled. A stream line
// carrying an "error" field fails the pull. Idempotent at the Ollama layer: pulling
// a present model streams a quick "success".
func (p *Prober) Pull(ctx context.Context, tag string, onProgress func(PullProgress)) error {
	body, _ := json.Marshal(map[string]any{"model": tag, "stream": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/pull", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// A pull streams for minutes over a multi-GB body — a fixed whole-request
	// timeout (even TimeoutLLM) would abort it mid-download. Use a streaming client
	// with no whole-request budget; ctx governs cancellation.
	client := httpx.NewStreaming()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama pull: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ollama pull: status %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var pp pullProgress
		if err := json.Unmarshal([]byte(line), &pp); err != nil {
			continue // skip a non-JSON keepalive line
		}
		if pp.Error != "" {
			return fmt.Errorf("ollama pull: %s", pp.Error)
		}
		if onProgress != nil {
			onProgress(PullProgress{Status: pp.Status, Completed: pp.Completed, Total: pp.Total})
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("ollama pull stream: %w", err)
	}
	return nil
}
