// Command prototype-spoken-safety-cascade is throwaway calibration code. It
// answers whether native-audio LLM adjudication is promising enough to replace
// broad human review after local keyword spotting. It must never ship.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	audioQuestion     = "Can a pinned native-audio LLM adjudicate KWS candidates accurately enough to remove broad human review?"
	videoQuestion     = "How many native-audio candidate rejections survive an independent complete-video suitability screen?"
	challengeQuestion = "Can the exact complete-video lane recover the prior prohibited-signal anchors before its negative agreements are trusted?"
	defaultModel      = "google/gemini-3.8-flash"
	defaultCanon      = "google/gemini-3.8-flash-20260902"
	defaultVendor     = "Google"
	defaultRoute      = "google-vertex/global"
	apiBase           = "https://openrouter.ai/api/v1"
	maxResponse       = 1 << 20
)

type authority struct {
	Cases []challengeCase `json:"cases"`
}

type challengeCase struct {
	Alias        string     `json:"alias"`
	Label        string     `json:"label"`
	Slices       []string   `json:"slices"`
	SourceSHA256 string     `json:"sourceSha256"`
	Intervals    []interval `json:"positiveIntervals"`
}

type interval struct {
	StartMS int64 `json:"startMs"`
	EndMS   int64 `json:"endMs"`
}

type packet struct {
	CaseID        string   `json:"caseId"`
	ContentSHA256 string   `json:"contentSha256"`
	Signals       []signal `json:"signals"`
}

type signal struct {
	Kind       string `json:"kind"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	Bytes      int64  `json:"bytes"`
	DurationMS int64  `json:"durationMs"`
}

type policy struct {
	PolicyID                 string       `json:"policyId"`
	MaximumInterSegmentGapMS int64        `json:"maximumInterSegmentGapMs"`
	Rules                    []policyRule `json:"rules"`
}

type policyRule struct {
	ID        string   `json:"id"`
	Class     string   `json:"class"`
	MatchMode string   `json:"matchMode"`
	Variants  []string `json:"variants"`
}

type snapshot struct {
	Models []snapshotModel `json:"models"`
}

type snapshotModel struct {
	ID              string             `json:"id"`
	CanonicalSlug   string             `json:"canonicalSlug"`
	InputModalities []string           `json:"inputModalities"`
	Endpoints       []snapshotEndpoint `json:"endpoints"`
}

type snapshotEndpoint struct {
	ProviderName        string   `json:"providerName"`
	ProviderSlug        string   `json:"providerSlug"`
	SupportedParameters []string `json:"supportedParameters"`
	ZDR                 bool     `json:"zdr"`
}

type selectedCase struct {
	Challenge challengeCase
	Packet    packet
	Audio     signal
	AudioPath string
	AudioSHA  string
	Video     signal
	VideoPath string
	VideoSHA  string
	Windows   []window
}

type window struct {
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds"`
}

type kwsEvent struct {
	Timestamps []float64 `json:"timestamps"`
}

type realDiagnostic struct {
	DetectedCases int                  `json:"detectedCases"`
	Cases         []realDiagnosticCase `json:"cases"`
}

type realDiagnosticCase struct {
	SourceID         string `json:"sourceId"`
	AudioSHA256      string `json:"audioSha256"`
	AudioDurationMS  int64  `json:"audioDurationMs"`
	PriorDisposition string `json:"priorDisposition"`
	Detected         bool   `json:"detected"`
}

type modelOutput struct {
	Decision       decision `json:"decision"`
	Audibility     string   `json:"audibility"`
	MatchedRuleIDs []string `json:"matchedRuleIds"`
}

type chatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		Cost json.Number `json:"cost"`
	} `json:"usage"`
	Metadata struct {
		Attempt   int `json:"attempt"`
		Endpoints struct {
			Available []struct {
				Provider string `json:"provider"`
				Model    string `json:"model"`
				Selected bool   `json:"selected"`
			} `json:"available"`
		} `json:"endpoints"`
		Attempts []struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Status   int    `json:"status"`
		} `json:"attempts"`
	} `json:"openrouter_metadata"`
}

type requestRoute struct {
	Order             []string `json:"order"`
	Only              []string `json:"only"`
	AllowFallbacks    bool     `json:"allow_fallbacks"`
	RequireParameters bool     `json:"require_parameters"`
	DataCollection    string   `json:"data_collection"`
	ZDR               bool     `json:"zdr"`
}

type options struct {
	auto                bool
	mode                string
	root                string
	authorityPath       string
	packetsPath         string
	policyPath          string
	snapshotPath        string
	outputPath          string
	maxRequests         int
	maxSpendNanoUSD     int64
	maxChargeNanoUSD    int64
	reasoningEffort     string
	model               string
	canonicalModel      string
	providerName        string
	providerSlug        string
	kwsIDsPath          string
	kwsResultsPath      string
	realDiagnosticPath  string
	cascadeReportPath   string
	preparedPacketsPath string
	videoManifestPath   string
	videoChallengePath  string
}

type runner struct {
	options options
	apiKey  string
	policy  policy
	cases   []selectedCase
	state   prototypeState
	client  *http.Client
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "prototype-spoken-safety-cascade:", err)
		os.Exit(1)
	}
}

func run() error {
	var opts options
	flag.BoolVar(&opts.auto, "auto", false, "run the selected cases without waiting for keystrokes")
	flag.StringVar(&opts.mode, "mode", "pilot", "pilot selects control representatives; all selects every control; real adjudicates KWS candidates; corroborate screens native-audio negative candidates by video")
	flag.StringVar(&opts.root, "root", "", "private filler development root")
	flag.StringVar(&opts.authorityPath, "authority", "", "challenge authority JSON (defaults under root)")
	flag.StringVar(&opts.packetsPath, "packets", "", "control packet JSONL (defaults under root)")
	flag.StringVar(&opts.policyPath, "policy", "", "private spoken-safety policy JSON (defaults under root)")
	flag.StringVar(&opts.snapshotPath, "snapshot", "", "OpenRouter capability snapshot JSON (defaults under root)")
	flag.StringVar(&opts.outputPath, "out", "", "new private report path (defaults under root)")
	flag.IntVar(&opts.maxRequests, "max-requests", 17, "hard request ceiling")
	flag.Int64Var(&opts.maxSpendNanoUSD, "max-spend-nanousd", 1_000_000_000, "hard total charge ceiling")
	flag.Int64Var(&opts.maxChargeNanoUSD, "max-charge-nanousd", 20_000_000, "hard per-request charge ceiling")
	flag.StringVar(&opts.reasoningEffort, "reasoning-effort", "minimal", "reasoning effort: none, minimal, low, medium, or high")
	flag.StringVar(&opts.model, "model", defaultModel, "concrete OpenRouter model ID")
	flag.StringVar(&opts.canonicalModel, "canonical-model", defaultCanon, "snapshot-proven canonical model ID")
	flag.StringVar(&opts.providerName, "provider", defaultVendor, "snapshot and response provider name")
	flag.StringVar(&opts.providerSlug, "provider-slug", defaultRoute, "snapshot-proven OpenRouter provider slug")
	flag.StringVar(&opts.kwsIDsPath, "kws-ids", "", "real-corpus KWS event-to-audio ID list (defaults under root)")
	flag.StringVar(&opts.kwsResultsPath, "kws-results", "", "real-corpus KWS event JSONL (defaults under root)")
	flag.StringVar(&opts.realDiagnosticPath, "real-diagnostic", "", "redacted real-corpus diagnostic (defaults under root)")
	flag.StringVar(&opts.cascadeReportPath, "cascade-report", "", "native-audio real-candidate report for corroboration (defaults under root)")
	flag.StringVar(&opts.preparedPacketsPath, "prepared-packets", "", "full prepared corpus packet JSONL for corroboration (defaults under root)")
	flag.StringVar(&opts.videoManifestPath, "video-manifest", "", "verified 48-case video evidence manifest for challenge replay (defaults under root)")
	flag.StringVar(&opts.videoChallengePath, "video-challenge-result", "", "prior suitability result used only to choose challenge cases (defaults under root)")
	flag.Parse()
	if opts.root == "" {
		return errors.New("--root is required")
	}
	truthRoot := filepath.Join(opts.root, "temporal-truth-review-v1")
	flagDefaults(&opts, truthRoot)
	if opts.mode == "all" && opts.maxRequests == 17 {
		opts.maxRequests = 67
	}
	if opts.mode == "real" && opts.maxRequests == 17 {
		opts.maxRequests = 88
	}
	if opts.mode == "corroborate" && opts.maxRequests == 17 {
		opts.maxRequests = 38
	}
	if opts.mode == "video-challenge" && opts.maxRequests == 17 {
		opts.maxRequests = 3
	}
	if opts.outputPath == "" {
		name := "gemini38-native-audio-control-pilot-v1.json"
		if opts.mode == "all" {
			name = "gemini38-native-audio-controls-full-v1.json"
		} else if opts.mode == "real" {
			name = "native-audio-real-kws-candidates-v1.json"
		} else if opts.mode == "corroborate" {
			name = "gemini38-video-corroboration-v1.json"
		} else if opts.mode == "video-challenge" {
			name = "gemini38-video-positive-challenge-v1.json"
		}
		opts.outputPath = filepath.Join(truthRoot, "spoken-safety-controls-v2", name)
	}
	if opts.mode != "pilot" && opts.mode != "all" && opts.mode != "real" && opts.mode != "corroborate" && opts.mode != "video-challenge" {
		return errors.New("--mode must be pilot, all, real, corroborate, or video-challenge")
	}
	if !slices.Contains([]string{"none", "minimal", "low", "medium", "high"}, opts.reasoningEffort) {
		return errors.New("--reasoning-effort must be none, minimal, low, medium, or high")
	}
	apiKey := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if apiKey == "" {
		return errors.New("OPENROUTER_API_KEY is required")
	}

	loaded, err := loadRunner(opts, apiKey)
	if err != nil {
		return err
	}
	if len(loaded.cases) > opts.maxRequests {
		return fmt.Errorf("selected %d cases exceeds request ceiling %d", len(loaded.cases), opts.maxRequests)
	}
	loaded.render("ready")
	if opts.auto {
		for loaded.state.CompletedCases < len(loaded.cases) {
			if err := loaded.next(context.Background()); err != nil {
				loaded.render("stopped after operational failure")
				break
			}
			loaded.render("case complete")
		}
	} else if err := loaded.interactive(); err != nil {
		return err
	}
	if err := writePrivateReport(opts.outputPath, loaded.state); err != nil {
		return err
	}
	fmt.Printf("private report: %s\n", opts.outputPath)
	return nil
}

func flagDefaults(opts *options, truthRoot string) {
	controls := filepath.Join(truthRoot, "spoken-safety-controls-v2")
	if opts.authorityPath == "" {
		opts.authorityPath = filepath.Join(controls, "challenge-authority.json")
	}
	if opts.packetsPath == "" {
		opts.packetsPath = filepath.Join(controls, "control-packets.jsonl")
	}
	if opts.policyPath == "" {
		opts.policyPath = filepath.Join(truthRoot, "private", "spoken-safety-policy-known-prohibited-v2.json")
	}
	if opts.snapshotPath == "" {
		opts.snapshotPath = filepath.Join(truthRoot, "openrouter-spoken-cascade-snapshot-v1.json")
	}
	real := filepath.Join(truthRoot, "sherpa-kws-real-corpus-v2")
	if opts.kwsIDsPath == "" {
		opts.kwsIDsPath = filepath.Join(real, "linux-arm64-detected-id.txt")
	}
	if opts.kwsResultsPath == "" {
		opts.kwsResultsPath = filepath.Join(real, "linux-arm64-results.jsonl")
	}
	if opts.realDiagnosticPath == "" {
		opts.realDiagnosticPath = filepath.Join(real, "linux-arm64-redacted-diagnostic.json")
	}
	if opts.cascadeReportPath == "" {
		opts.cascadeReportPath = filepath.Join(truthRoot, "voxtral24b-real-kws-candidates-v1.json")
	}
	if opts.preparedPacketsPath == "" {
		opts.preparedPacketsPath = filepath.Join(opts.root, "prepared-v1", "packets.jsonl")
	}
	if opts.videoManifestPath == "" {
		opts.videoManifestPath = filepath.Join(truthRoot, "evidence-v1", "public", "manifest.json")
	}
	if opts.videoChallengePath == "" {
		opts.videoChallengePath = filepath.Join(truthRoot, "suitability-gemini37-video-full-2026-09-02.json")
	}
}

func loadRunner(opts options, apiKey string) (*runner, error) {
	var privatePolicy policy
	if err := readJSON(opts.policyPath, &privatePolicy); err != nil {
		return nil, fmt.Errorf("read private policy: %w", err)
	}
	if len(privatePolicy.Rules) == 0 {
		return nil, errors.New("private policy has no rules")
	}
	var capability snapshot
	if err := readJSON(opts.snapshotPath, &capability); err != nil {
		return nil, fmt.Errorf("read capability snapshot: %w", err)
	}
	if err := validateSnapshot(capability, opts); err != nil {
		return nil, err
	}
	var selected []selectedCase
	evidencePaths := []string{opts.snapshotPath, opts.policyPath}
	if opts.mode == "real" {
		var err error
		selected, err = selectRealCases(opts)
		if err != nil {
			return nil, err
		}
		evidencePaths = append(evidencePaths, opts.kwsIDsPath, opts.kwsResultsPath, opts.realDiagnosticPath)
	} else if opts.mode == "corroborate" {
		var err error
		selected, err = selectCorroborationCases(opts)
		if err != nil {
			return nil, err
		}
		evidencePaths = append(evidencePaths, opts.kwsIDsPath, opts.kwsResultsPath, opts.realDiagnosticPath, opts.cascadeReportPath, opts.preparedPacketsPath)
	} else if opts.mode == "video-challenge" {
		var err error
		selected, err = selectVideoChallengeCases(opts)
		if err != nil {
			return nil, err
		}
		evidencePaths = append(evidencePaths, opts.videoManifestPath, opts.videoChallengePath)
	} else {
		var auth authority
		if err := readJSON(opts.authorityPath, &auth); err != nil {
			return nil, fmt.Errorf("read authority: %w", err)
		}
		packets, err := readPackets(opts.packetsPath)
		if err != nil {
			return nil, err
		}
		selected, err = selectCases(opts, auth.Cases, packets)
		if err != nil {
			return nil, err
		}
		evidencePaths = append(evidencePaths, opts.authorityPath, opts.packetsPath)
	}
	evidenceHashes := make([]string, 0, len(evidencePaths))
	for _, path := range evidencePaths {
		digest, err := hashFile(path)
		if err != nil {
			return nil, err
		}
		evidenceHashes = append(evidenceHashes, digest)
	}
	result := &runner{
		options: opts, apiKey: apiKey, policy: privatePolicy, cases: selected,
		state: prototypeState{
			Question: questionForMode(opts.mode), Mode: opts.mode, Model: opts.model, CanonicalModel: opts.canonicalModel,
			ProviderSlug: opts.providerSlug, ReasoningEffort: opts.reasoningEffort,
			SnapshotSHA256: evidenceHashes[0], PolicySHA256: evidenceHashes[1], PlannedCases: len(selected),
		},
		client: &http.Client{Timeout: 90 * time.Second},
	}
	if opts.mode == "real" {
		result.state.KWSIDsSHA256 = evidenceHashes[2]
		result.state.KWSResultsSHA256 = evidenceHashes[3]
		result.state.RealDiagnosticSHA256 = evidenceHashes[4]
	} else if opts.mode == "corroborate" {
		result.state.KWSIDsSHA256 = evidenceHashes[2]
		result.state.KWSResultsSHA256 = evidenceHashes[3]
		result.state.RealDiagnosticSHA256 = evidenceHashes[4]
		result.state.CascadeReportSHA256 = evidenceHashes[5]
		result.state.PreparedPacketsSHA256 = evidenceHashes[6]
		result.state.PromptSHA256 = hash([]byte(videoSuitabilitySystemPrompt))
	} else if opts.mode == "video-challenge" {
		result.state.VideoManifestSHA256 = evidenceHashes[2]
		result.state.ChallengeResultSHA256 = evidenceHashes[3]
		result.state.PromptSHA256 = hash([]byte(videoSuitabilitySystemPrompt))
	} else {
		result.state.AuthoritySHA256 = evidenceHashes[2]
		result.state.PacketsSHA256 = evidenceHashes[3]
	}
	return result, nil
}

func questionForMode(mode string) string {
	if mode == "video-challenge" {
		return challengeQuestion
	}
	if mode == "corroborate" {
		return videoQuestion
	}
	return audioQuestion
}

func selectCases(opts options, cases []challengeCase, packets map[string]packet) ([]selectedCase, error) {
	sort.Slice(cases, func(i, j int) bool { return cases[i].Alias < cases[j].Alias })
	chosen := make([]challengeCase, 0, len(cases))
	seenSlice := map[string]bool{}
	for _, candidate := range cases {
		if opts.mode == "all" || candidate.Label == "clean" {
			chosen = append(chosen, candidate)
			continue
		}
		for _, slice := range candidate.Slices {
			if !seenSlice[slice] {
				chosen = append(chosen, candidate)
				seenSlice[slice] = true
				break
			}
		}
	}
	if opts.mode == "pilot" && (len(chosen) != 17 || len(seenSlice) != 9) {
		return nil, fmt.Errorf("pilot selection drifted: cases=%d positive-slices=%d", len(chosen), len(seenSlice))
	}
	selected := make([]selectedCase, 0, len(chosen))
	for _, candidate := range chosen {
		packet, ok := packets[candidate.SourceSHA256]
		if !ok {
			return nil, fmt.Errorf("authority alias %s has no packet by source digest", candidate.Alias)
		}
		var audio signal
		for _, candidateSignal := range packet.Signals {
			if candidateSignal.Kind == "audio" {
				audio = candidateSignal
				break
			}
		}
		if audio.Path == "" {
			return nil, fmt.Errorf("authority alias %s has no audio signal", candidate.Alias)
		}
		selected = append(selected, selectedCase{Challenge: candidate, Packet: packet, Audio: audio})
	}
	return selected, nil
}

func selectRealCases(opts options) ([]selectedCase, error) {
	rawIDs, err := os.ReadFile(opts.kwsIDsPath)
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(string(rawIDs))
	events, err := readKWSEvents(opts.kwsResultsPath)
	if err != nil {
		return nil, err
	}
	if len(ids) != len(events) {
		return nil, fmt.Errorf("KWS event authority drifted: ids=%d events=%d", len(ids), len(events))
	}
	grouped := map[string][]kwsEvent{}
	for index, id := range ids {
		if len(id) != 64 {
			return nil, errors.New("KWS input ID is not a digest-shaped derivative ID")
		}
		grouped[id] = append(grouped[id], events[index])
	}
	var diagnostic realDiagnostic
	if err := readJSON(opts.realDiagnosticPath, &diagnostic); err != nil {
		return nil, err
	}
	if diagnostic.DetectedCases != len(grouped) {
		return nil, fmt.Errorf("redacted diagnostic drifted: detected=%d grouped=%d", diagnostic.DetectedCases, len(grouped))
	}
	byAudio := map[string]realDiagnosticCase{}
	for _, candidate := range diagnostic.Cases {
		if candidate.Detected {
			byAudio[candidate.AudioSHA256] = candidate
		}
	}
	selected := make([]selectedCase, 0, len(grouped))
	known := 0
	for id, caseEvents := range grouped {
		path := filepath.Join(opts.root, "prepared-v1", "derivatives", id, "audio.wav")
		digest, err := hashFile(path)
		if err != nil {
			return nil, err
		}
		metadata, ok := byAudio[digest]
		if !ok {
			return nil, errors.New("KWS derivative audio is not bound by the redacted diagnostic")
		}
		label := "unknown"
		if metadata.PriorDisposition == "prohibited_hold" {
			label = "known_positive"
			known++
		}
		windows, err := candidateWindows(caseEvents, float64(metadata.AudioDurationMS)/1000)
		if err != nil {
			return nil, err
		}
		selected = append(selected, selectedCase{
			Challenge: challengeCase{Alias: metadata.SourceID, Label: label, Slices: []string{"real_kws_candidate"}},
			AudioPath: path, AudioSHA: digest, Windows: windows,
		})
	}
	if known != 1 {
		return nil, fmt.Errorf("expected one known prohibited candidate, found %d", known)
	}
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].Challenge.Label != selected[j].Challenge.Label {
			return selected[i].Challenge.Label == "known_positive"
		}
		return selected[i].Challenge.Alias < selected[j].Challenge.Alias
	})
	return selected, nil
}

func readKWSEvents(path string) ([]kwsEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var events []kwsEvent
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	for scanner.Scan() {
		var event kwsEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, scanner.Err()
}

func candidateWindows(events []kwsEvent, durationSeconds float64) ([]window, error) {
	windows := make([]window, 0, len(events))
	for _, event := range events {
		if len(event.Timestamps) == 0 {
			return nil, errors.New("KWS event has no timestamps")
		}
		start := event.Timestamps[0] - 1
		if start < 0 {
			start = 0
		}
		end := event.Timestamps[len(event.Timestamps)-1] + 1
		if end > durationSeconds {
			end = durationSeconds
		}
		if end <= start {
			return nil, errors.New("KWS event produced an empty candidate window")
		}
		windows = append(windows, window{StartSeconds: start, EndSeconds: end})
	}
	sort.Slice(windows, func(i, j int) bool { return windows[i].StartSeconds < windows[j].StartSeconds })
	merged := windows[:0]
	for _, candidate := range windows {
		if len(merged) == 0 || candidate.StartSeconds > merged[len(merged)-1].EndSeconds {
			merged = append(merged, candidate)
			continue
		}
		if candidate.EndSeconds > merged[len(merged)-1].EndSeconds {
			merged[len(merged)-1].EndSeconds = candidate.EndSeconds
		}
	}
	return merged, nil
}

func (r *runner) next(ctx context.Context) error {
	if r.state.CompletedCases >= len(r.cases) {
		return nil
	}
	if r.state.ChargedNanoUSD+r.options.maxChargeNanoUSD > r.options.maxSpendNanoUSD {
		return errors.New("next reservation would exceed total spend ceiling")
	}
	selected := r.cases[r.state.CompletedCases]
	result, err := r.call(ctx, selected)
	if err != nil {
		result.Alias = selected.Challenge.Alias
		result.ExpectedLabel = selected.Challenge.Label
		result.Slices = slices.Clone(selected.Challenge.Slices)
		result.Decision = decisionFailure
		if result.FailureKind == "" {
			result.FailureKind = "transport_or_contract"
		}
		r.state = reduce(r.state, result)
		return err
	}
	result.Alias = selected.Challenge.Alias
	result.ExpectedLabel = selected.Challenge.Label
	result.Slices = slices.Clone(selected.Challenge.Slices)
	r.state = reduce(r.state, result)
	return nil
}

func (r *runner) call(ctx context.Context, selected selectedCase) (observation, error) {
	if r.options.mode == "corroborate" || r.options.mode == "video-challenge" {
		return r.callVideo(ctx, selected)
	}
	return r.callAudio(ctx, selected)
}

func (r *runner) callAudio(ctx context.Context, selected selectedCase) (observation, error) {
	path := selected.AudioPath
	expectedSHA := selected.AudioSHA
	if path == "" {
		path = filepath.Join(r.options.root, "prepared-v1", "derivatives", filepath.FromSlash(selected.Audio.Path))
		expectedSHA = selected.Audio.SHA256
	}
	audio, err := os.ReadFile(path)
	if err != nil {
		return observation{FailureKind: "read_audio"}, err
	}
	sourceSHA := hash(audio)
	if len(audio) < 12 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" || sourceSHA != expectedSHA {
		return observation{FailureKind: "audio_authority"}, errors.New("audio failed WAV or digest validation")
	}
	inputAudio := audio
	if len(selected.Windows) > 0 {
		inputAudio, err = cropAudio(ctx, path, selected.Windows)
		if err != nil {
			return observation{FailureKind: "crop_audio"}, err
		}
	}
	policyJSON, err := json.Marshal(struct {
		PolicyID                 string       `json:"policyId"`
		MaximumInterSegmentGapMS int64        `json:"maximumInterSegmentGapMs"`
		Rules                    []policyRule `json:"rules"`
	}{r.policy.PolicyID, r.policy.MaximumInterSegmentGapMS, r.policy.Rules})
	if err != nil {
		return observation{FailureKind: "encode_policy"}, err
	}
	ruleIDs := make([]any, 0, len(r.policy.Rules))
	for _, rule := range r.policy.Rules {
		ruleIDs = append(ruleIDs, rule.ID)
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"decision":       map[string]any{"type": "string", "enum": []string{"detected", "absent", "unclear"}},
			"audibility":     map[string]any{"type": "string", "enum": []string{"clear", "degraded", "no_speech"}},
			"matchedRuleIds": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": ruleIDs}},
		},
		"required": []string{"decision", "audibility", "matchedRuleIds"}, "additionalProperties": false,
	}
	payload := map[string]any{
		"model": r.options.model,
		"messages": []any{
			map[string]any{"role": "system", "content": []any{map[string]any{"type": "text", "text": "You are a broadcast spoken-safety adjudicator. Listen directly to the attached audio. Apply only the supplied private rules and their match modes. A token_prefix rule is detected when its listed variant begins an audible spoken token, including a longer inflected token. Do not count homophones, merely similar sounds, instrumental music, or wordless vocals. If degradation prevents a reliable distinction, answer unclear. Never transcribe, quote, or repeat any speech. Return only the required schema."}}},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "Private rule policy: " + string(policyJSON) + ". Decide whether this audio triggers any rule."},
				map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": base64.StdEncoding.EncodeToString(inputAudio), "format": "wav"}},
			}},
		},
		"provider":        requestRoute{Order: []string{r.options.providerSlug}, Only: []string{r.options.providerSlug}, AllowFallbacks: false, RequireParameters: true, DataCollection: "deny", ZDR: true},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "spoken_safety_adjudication", "strict": true, "schema": schema}},
		"seed":            0,
		"max_tokens":      1280,
	}
	if r.options.reasoningEffort != "none" {
		payload["reasoning"] = map[string]any{"effort": r.options.reasoningEffort, "exclude": true}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return observation{FailureKind: "encode_request"}, err
	}
	result := observation{
		RequestSHA256: hash(body), SourceAudioSHA256: sourceSHA, WindowAudioSHA256: hash(inputAudio),
		Intervals: slices.Clone(selected.Windows),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		result.FailureKind = "build_request"
		return result, err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-OpenRouter-Metadata", "enabled")
	req.Header.Set("HTTP-Referer", "https://github.com/loomarr/loomarr")
	req.Header.Set("X-OpenRouter-Title", "Loomarr spoken-safety cascade prototype")
	response, err := r.client.Do(req)
	if err != nil {
		result.FailureKind = "request"
		return result, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	result.ResponseSHA256 = hash(raw)
	if err != nil || len(raw) > maxResponse {
		result.FailureKind = "response_bounds"
		return result, errors.New("response exceeded the prototype byte contract")
	}
	if response.StatusCode != http.StatusOK {
		result.FailureKind = fmt.Sprintf("http_%d", response.StatusCode)
		result.FailureDetail = sanitizedAPIError(raw, r.policy.Rules)
		return result, fmt.Errorf("OpenRouter returned HTTP %d (body retained only as a digest)", response.StatusCode)
	}
	var wire chatResponse
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&wire); err != nil {
		result.FailureKind = "decode_response"
		return result, err
	}
	charged, err := nanoUSD(wire.Usage.Cost)
	if err != nil || charged > r.options.maxChargeNanoUSD {
		result.FailureKind = "charge_contract"
		return result, errors.New("response charge was missing or exceeded the reservation")
	}
	result.ChargedNanoUSD = charged
	if err := validateRoute(wire, r.options); err != nil {
		result.FailureKind = "route_contract"
		return result, err
	}
	if len(wire.Choices) != 1 {
		result.FailureKind = "choice_contract"
		return result, errors.New("response did not contain exactly one choice")
	}
	var output modelOutput
	if err := json.Unmarshal([]byte(wire.Choices[0].Message.Content), &output); err != nil {
		result.FailureKind = "structured_output"
		return result, err
	}
	if err := validateOutput(output, r.policy.Rules); err != nil {
		result.FailureKind = "decision_contract"
		return result, err
	}
	result.Decision = output.Decision
	result.Audibility = output.Audibility
	result.MatchedRuleIDs = slices.Clone(output.MatchedRuleIDs)
	result.GenerationIDHash = hash([]byte(wire.ID))
	return result, nil
}

func validateOutput(output modelOutput, rules []policyRule) error {
	if output.Decision != decisionDetected && output.Decision != decisionAbsent && output.Decision != decisionUnclear {
		return errors.New("invalid decision")
	}
	if output.Audibility != "clear" && output.Audibility != "degraded" && output.Audibility != "no_speech" {
		return errors.New("invalid audibility")
	}
	known := map[string]bool{}
	for _, rule := range rules {
		known[rule.ID] = true
	}
	for _, id := range output.MatchedRuleIDs {
		if !known[id] {
			return errors.New("unknown matched rule id")
		}
	}
	if output.Decision == decisionDetected && len(output.MatchedRuleIDs) == 0 {
		return errors.New("detected decision omitted rule id")
	}
	if output.Decision != decisionDetected && len(output.MatchedRuleIDs) != 0 {
		return errors.New("non-detected decision included rule id")
	}
	return nil
}

func cropAudio(ctx context.Context, path string, windows []window) ([]byte, error) {
	filters := make([]string, 0, len(windows)+1)
	labels := make([]string, 0, len(windows))
	for index, candidate := range windows {
		label := fmt.Sprintf("w%d", index)
		filters = append(filters, fmt.Sprintf("[0:a]atrim=start=%.3f:end=%.3f,asetpts=PTS-STARTPTS[%s]", candidate.StartSeconds, candidate.EndSeconds, label))
		labels = append(labels, "["+label+"]")
	}
	if len(labels) == 1 {
		filters = append(filters, labels[0]+"anull[out]")
	} else {
		filters = append(filters, strings.Join(labels, "")+fmt.Sprintf("concat=n=%d:v=0:a=1[out]", len(labels)))
	}
	command := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-i", path,
		"-filter_complex", strings.Join(filters, ";"), "-map", "[out]", "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", "-f", "wav", "pipe:1")
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg candidate extraction failed (%d diagnostic bytes)", stderr.Len())
	}
	result := stdout.Bytes()
	if len(result) < 12 || string(result[:4]) != "RIFF" || string(result[8:12]) != "WAVE" {
		return nil, errors.New("ffmpeg candidate extraction did not produce WAV")
	}
	return bytes.Clone(result), nil
}

func validateRoute(wire chatResponse, opts options) error {
	if wire.ID == "" || (wire.Model != opts.model && wire.Model != opts.canonicalModel) || wire.Metadata.Attempt != 1 {
		return errors.New("response identity did not bind model and one-attempt route")
	}
	selected := 0
	for _, endpoint := range wire.Metadata.Endpoints.Available {
		if !endpoint.Selected {
			continue
		}
		selected++
		if endpoint.Provider != opts.providerName || (endpoint.Model != opts.model && endpoint.Model != opts.canonicalModel) {
			return errors.New("selected endpoint differed from the pinned route")
		}
	}
	if selected != 1 {
		return errors.New("response did not report exactly one selected endpoint")
	}
	if len(wire.Metadata.Attempts) > 0 {
		if len(wire.Metadata.Attempts) != 1 || wire.Metadata.Attempts[0].Provider != opts.providerName || wire.Metadata.Attempts[0].Status < 200 || wire.Metadata.Attempts[0].Status >= 300 {
			return errors.New("attempt ledger differed from the pinned route")
		}
	}
	return nil
}

func validateSnapshot(value snapshot, opts options) error {
	requiredModality := "audio"
	if opts.mode == "corroborate" || opts.mode == "video-challenge" {
		requiredModality = "video"
	}
	for _, model := range value.Models {
		if model.ID != opts.model {
			continue
		}
		if model.CanonicalSlug != opts.canonicalModel || !slices.Contains(model.InputModalities, requiredModality) {
			return fmt.Errorf("snapshot model identity or %s capability drifted", requiredModality)
		}
		for _, endpoint := range model.Endpoints {
			if endpoint.ProviderName == opts.providerName && endpoint.ProviderSlug == opts.providerSlug && endpoint.ZDR &&
				slices.Contains(endpoint.SupportedParameters, "response_format") && slices.Contains(endpoint.SupportedParameters, "structured_outputs") &&
				(opts.reasoningEffort == "none" || slices.Contains(endpoint.SupportedParameters, "reasoning")) {
				return nil
			}
		}
	}
	return fmt.Errorf("snapshot does not prove the pinned native-%s ZDR route", requiredModality)
}

func (r *runner) interactive() error {
	reader := bufio.NewReader(os.Stdin)
	for r.state.CompletedCases < len(r.cases) {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		switch strings.TrimSpace(line) {
		case "n":
			if err := r.next(context.Background()); err != nil {
				r.render("operational failure; case held")
				return nil
			}
			r.render("case complete")
		case "a":
			for r.state.CompletedCases < len(r.cases) {
				if err := r.next(context.Background()); err != nil {
					r.render("operational failure; stopped")
					return nil
				}
				r.render("case complete")
			}
		case "q":
			return nil
		default:
			r.render("unknown key")
		}
	}
	return nil
}

func (r *runner) render(status string) {
	fmt.Print("\x1b[2J\x1b[H")
	fmt.Printf("\x1b[1mTHROWAWAY SPOKEN-SAFETY CASCADE PROTOTYPE\x1b[0m\n")
	fmt.Printf("\x1b[2mQuestion: %s\x1b[0m\n\n", r.state.Question)
	fmt.Printf("\x1b[1mstatus\x1b[0m              %s\n", status)
	fmt.Printf("\x1b[1mmode\x1b[0m                %s\n", r.state.Mode)
	fmt.Printf("\x1b[1mreasoning\x1b[0m           %s\n", r.state.ReasoningEffort)
	fmt.Printf("\x1b[1mprogress\x1b[0m            %d/%d\n", r.state.CompletedCases, r.state.PlannedCases)
	fmt.Printf("\x1b[1mrequests\x1b[0m            %d/%d\n", r.state.Requests, r.options.maxRequests)
	fmt.Printf("\x1b[1mcharged\x1b[0m             $%.9f / $%.2f\n", float64(r.state.ChargedNanoUSD)/1e9, float64(r.options.maxSpendNanoUSD)/1e9)
	fmt.Printf("\x1b[1mdecisions\x1b[0m           detected=%d absent=%d unclear=%d failure=%d\n", r.state.Detected, r.state.Absent, r.state.Unclear, r.state.Failures)
	if r.state.Mode == "real" || r.state.Mode == "corroborate" || r.state.Mode == "video-challenge" {
		fmt.Printf("\x1b[1mknown positive\x1b[0m      retained=%d missed-or-held=%d\n", r.state.KnownPositiveHeld, r.state.KnownPositiveMissed)
		fmt.Printf("\x1b[1munlabelled candidates\x1b[0m retained=%d rejected=%d held=%d\n", r.state.UnlabelledRetained, r.state.UnlabelledRejected, r.state.UnlabelledHeld)
	} else {
		fmt.Printf("\x1b[1mpositive controls\x1b[0m   detected=%d missed-or-held=%d\n", r.state.PositiveDetected, r.state.PositiveMissed)
		fmt.Printf("\x1b[1mclean controls\x1b[0m      absent=%d held=%d\n", r.state.CleanAbsent, r.state.CleanHeld)
	}
	fmt.Printf("\x1b[1msafe to expand\x1b[0m      %t\n", r.state.SafeToExpand)
	fmt.Printf("\x1b[1mproduction authority\x1b[0m false\n")
	if len(r.state.Observations) > 0 {
		last := r.state.Observations[len(r.state.Observations)-1]
		fmt.Printf("\n\x1b[1mlast\x1b[0m %s expected=%s decision=%s disposition=%s correct=%t\n", last.Alias, last.ExpectedLabel, last.Decision, last.Disposition, last.Correct)
	}
	if r.state.CompletedCases < len(r.cases) && !r.options.auto {
		fmt.Print("\n\x1b[1m[n]\x1b[0m next  \x1b[1m[a]\x1b[0m run remaining  \x1b[1m[q]\x1b[0m quit\n")
	}
}

func readJSON(path string, destination any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	return decoder.Decode(destination)
}

func readPackets(path string) (map[string]packet, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result := map[string]packet{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	for scanner.Scan() {
		var row packet
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, err
		}
		result[row.ContentSHA256] = row
	}
	return result, scanner.Err()
}

func writePrivateReport(path string, state prototypeState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(raw); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func nanoUSD(value json.Number) (int64, error) {
	if value.String() == "" {
		return 0, errors.New("missing cost")
	}
	rat, ok := new(big.Rat).SetString(value.String())
	if !ok || rat.Sign() < 0 {
		return 0, errors.New("invalid cost")
	}
	rat.Mul(rat, big.NewRat(1_000_000_000, 1))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(rat.Num(), rat.Denom(), remainder)
	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return 0, errors.New("cost overflow")
	}
	return quotient.Int64(), nil
}

func sanitizedAPIError(raw []byte, rules []policyRule) string {
	var envelope struct {
		Error struct {
			Message  string          `json:"message"`
			Metadata json.RawMessage `json:"metadata"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return "unparseable error envelope"
	}
	detail := strings.TrimSpace(envelope.Error.Message)
	if len(envelope.Error.Metadata) > 0 && string(envelope.Error.Metadata) != "null" {
		detail += " metadata=" + string(envelope.Error.Metadata)
	}
	for _, rule := range rules {
		for _, variant := range rule.Variants {
			pattern, err := regexp.Compile("(?i)" + regexp.QuoteMeta(variant))
			if err == nil {
				detail = pattern.ReplaceAllString(detail, "[restricted]")
			}
		}
	}
	detail = strings.Join(strings.Fields(detail), " ")
	if len(detail) > 500 {
		detail = detail[:500]
	}
	if detail == "" {
		return "empty error message"
	}
	return detail
}

func hash(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func hashFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hash(raw), nil
}
