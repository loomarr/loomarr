package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"slices"
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
	// Installed is the RICH view of each pulled model — its on-disk size (a VRAM proxy),
	// parameter size / family / quant (from /api/tags), and whether it advertises tool
	// calling (from /api/show `capabilities`). The catalog is built LIVE from these, so
	// there is no hardcoded model list to go stale: a model you pulled that can tool-call
	// shows up, one that can't (e.g. DeepSeek-R1 in the official registry) does not.
	Installed []InstalledModel `json:"installed"`
	Reachable bool             `json:"reachable"` // Ollama /api/version answered
}

// InstalledModel is one locally-pulled model as reported by Ollama (§8.1).
type InstalledModel struct {
	Tag           string  `json:"tag"`           // exact pull tag, e.g. "qwen3:8b"
	SizeBytes     int64   `json:"sizeBytes"`     // on-disk size — proxy for resident VRAM footprint
	ParameterSize string  `json:"parameterSize"` // e.g. "7.6B" (details.parameter_size)
	Family        string  `json:"family"`        // e.g. "qwen2" (details.family)
	Quant         string  `json:"quant"`         // e.g. "Q4_K_M" (details.quantization_level)
	Tools         bool    `json:"tools"`         // /api/show capabilities includes "tools"
	VRAMGiB       float64 `json:"vramGiB"`       // SizeBytes → GiB (resident footprint proxy)
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
	// Discover installed models live (tags → size/details; show → tool capability),
	// then keep PulledModels as the flat tag list for the callers that only need it.
	out.Installed = p.installedModels(ctx)
	out.PulledModels = make([]string, 0, len(out.Installed))
	for _, m := range out.Installed {
		out.PulledModels = append(out.PulledModels, m.Tag)
	}
	return out
}

const bytesPerGiB = 1024 * 1024 * 1024

// installedModels enumerates pulled models via /api/tags (name, size, details) and
// annotates each with its tool-calling capability via /api/show. Best-effort: a model
// whose /api/show fails is kept with Tools=false (it just won't be offered for grounding).
func (p *Prober) installedModels(ctx context.Context) []InstalledModel {
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
			Name    string `json:"name"`
			Size    int64  `json:"size"`
			Details struct {
				Family            string `json:"family"`
				ParameterSize     string `json:"parameter_size"`
				QuantizationLevel string `json:"quantization_level"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil
	}
	models := make([]InstalledModel, 0, len(out.Models))
	for _, m := range out.Models {
		models = append(models, InstalledModel{
			Tag:           m.Name,
			SizeBytes:     m.Size,
			ParameterSize: m.Details.ParameterSize,
			Family:        m.Details.Family,
			Quant:         m.Details.QuantizationLevel,
			VRAMGiB:       float64(m.Size) / bytesPerGiB,
			Tools:         p.modelHasTools(ctx, m.Name),
		})
	}
	return models
}

// modelHasTools asks /api/show whether a model advertises tool calling. Ollama exposes
// this as a `capabilities` array (values: completion, tools, vision, …). This is the
// live, authoritative signal that replaces a hand-maintained "tool-callers" list.
func (p *Prober) modelHasTools(ctx context.Context, model string) bool {
	body, err := json.Marshal(map[string]string{"model": model})
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/show", strings.NewReader(string(body)))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.http.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var out struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false
	}
	return slices.Contains(out.Capabilities, "tools")
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

// GPUName reports the primary GPU's name (e.g. "NVIDIA GeForce RTX 3080 Ti"), or "" when unknown.
// A thin exported wrapper over the same nvidia-smi probe the status endpoint uses, so callers that
// only need the vendor signal (the playout encoder chooser) do not run the full LLM status probe or
// duplicate the nvidia-smi call. Returns "" (not an error) on any failure — an unknown GPU is a
// normal state that downstream code handles by falling back to its cross-vendor default.
func GPUName(ctx context.Context) string {
	name, _, _ := nvidiaSMIVRAM(ctx)
	return name
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
