package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

const videoSuitabilitySystemPrompt = `Screen one identity-blind candidate filler clip for two prohibited broadcast-suitability signals. This is high-recall screening, not a general content summary.

explicit_nudity means filmed or photographic visible genitals, exposed female breasts or nipples, or clearly exposed buttocks. Flag uncertain borderline visibility rather than silently clearing it. Do not flag ordinary swimwear, cleavage, bare male chests, statues, drawings, or non-explicit affection.

hateful_or_degrading_slur means audible or visibly written use of a group-directed slur targeting race, ethnicity, nationality, religion, sex, sexual orientation, gender identity, or disability. Context, quotation, age, or historical provenance does not make the clip airable. Do not repeat, quote, transcribe, or describe the slur itself.

Inspect the complete supplied video, including its audio. Set visualAssessment to insufficient if the video was not processed across its full span or visibility prevents a reliable screen. Set spokenLanguageAssessment to insufficient if the audio was unavailable, not processed across its full span, or too unclear to screen.

Return only the closed flags and approximate locations. Never return a quote, source identity, person identity, or free-form description. If a prohibited signal may be present but its exact time is uncertain, return the narrowest conservative range that contains it. Empty flags means only that this assessor observed no prohibited signal; it never certifies the clip as clean.`

type safetyFlag struct {
	Kind     string `json:"kind"`
	StartMS  int64  `json:"startMs"`
	EndMS    int64  `json:"endMs"`
	Modality string `json:"modality"`
}

type videoModelOutput struct {
	VisualAssessment string       `json:"visualAssessment"`
	SpokenAssessment string       `json:"spokenLanguageAssessment"`
	Flags            []safetyFlag `json:"flags"`
}

type videoEvidenceManifest struct {
	Cases []struct {
		Alias      string `json:"alias"`
		DurationMS int64  `json:"durationMs"`
		Video      signal `json:"video"`
	} `json:"cases"`
}

type priorSuitabilityResult struct {
	ProductionAdmissionAllowed bool `json:"productionAdmissionAllowed"`
	Assessments                []struct {
		EvidenceAlias string       `json:"evidenceAlias"`
		Flags         []safetyFlag `json:"flags"`
	} `json:"assessments"`
}

func selectVideoChallengeCases(opts options) ([]selectedCase, error) {
	var manifest videoEvidenceManifest
	if err := readJSON(opts.videoManifestPath, &manifest); err != nil {
		return nil, fmt.Errorf("read video evidence manifest: %w", err)
	}
	if len(manifest.Cases) != 48 {
		return nil, errors.New("video evidence manifest is not the frozen 48-case population")
	}
	byAlias := make(map[string]struct {
		Alias      string
		DurationMS int64
		Video      signal
	}, len(manifest.Cases))
	for _, item := range manifest.Cases {
		if item.Alias == "" || byAlias[item.Alias].Alias != "" {
			return nil, errors.New("video evidence alias is empty or duplicated")
		}
		byAlias[item.Alias] = struct {
			Alias      string
			DurationMS int64
			Video      signal
		}{item.Alias, item.DurationMS, item.Video}
	}
	var prior priorSuitabilityResult
	if err := readJSON(opts.videoChallengePath, &prior); err != nil {
		return nil, fmt.Errorf("read prior suitability challenge: %w", err)
	}
	if len(prior.Assessments) != 48 || prior.ProductionAdmissionAllowed {
		return nil, errors.New("prior suitability result is incomplete or carries production authority")
	}
	selected := make([]selectedCase, 0, 3)
	for _, assessment := range prior.Assessments {
		if len(assessment.Flags) == 0 {
			continue
		}
		item, ok := byAlias[assessment.EvidenceAlias]
		if !ok || item.DurationMS <= 0 || item.Video.Path == "" || item.Video.SHA256 == "" || item.Video.Bytes <= 0 {
			return nil, errors.New("prior suitability challenge is not bound to complete video evidence")
		}
		kinds := make([]string, 0, len(assessment.Flags))
		for _, observed := range assessment.Flags {
			kinds = append(kinds, "prior_"+observed.Kind)
		}
		video := item.Video
		video.DurationMS = item.DurationMS
		selected = append(selected, selectedCase{
			Challenge: challengeCase{Alias: item.Alias, Label: "prior_positive", Slices: kinds},
			Video:     video, VideoPath: filepath.Join(filepath.Dir(opts.videoManifestPath), filepath.FromSlash(video.Path)), VideoSHA: video.SHA256,
		})
	}
	if len(selected) != 3 {
		return nil, fmt.Errorf("prior suitability positive challenge drifted: got %d cases", len(selected))
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Challenge.Alias < selected[j].Challenge.Alias })
	return selected, nil
}

func selectCorroborationCases(opts options) ([]selectedCase, error) {
	var cascade prototypeState
	if err := readJSON(opts.cascadeReportPath, &cascade); err != nil {
		return nil, fmt.Errorf("read native-audio cascade report: %w", err)
	}
	if cascade.Mode != "real" || cascade.CompletedCases != 88 || len(cascade.Observations) != 88 || cascade.Failures != 0 || cascade.ProductionAuthority {
		return nil, errors.New("native-audio cascade report is incomplete, failed, or carries production authority")
	}
	realCases, err := selectRealCases(opts)
	if err != nil {
		return nil, err
	}
	realByAlias := make(map[string]selectedCase, len(realCases))
	for _, candidate := range realCases {
		realByAlias[candidate.Challenge.Alias] = candidate
	}
	wantedAudio := map[string]bool{}
	for _, result := range cascade.Observations {
		if result.Decision == decisionAbsent {
			wantedAudio[result.SourceAudioSHA256] = true
		}
	}
	packets, err := readPacketsByAudioSHA(opts.preparedPacketsPath, wantedAudio)
	if err != nil {
		return nil, err
	}
	selected := make([]selectedCase, 0, cascade.Absent)
	seen := map[string]bool{}
	for _, result := range cascade.Observations {
		if result.Decision != decisionAbsent {
			continue
		}
		if result.ExpectedLabel != "unknown" || result.Disposition != "candidate_rejected" || seen[result.Alias] {
			return nil, errors.New("native-audio negative selection is labelled, malformed, or duplicated")
		}
		seen[result.Alias] = true
		candidate, ok := realByAlias[result.Alias]
		if !ok || result.SourceAudioSHA256 != candidate.AudioSHA || !slices.Equal(result.Intervals, candidate.Windows) {
			return nil, errors.New("native-audio negative is not bound to the frozen KWS candidate")
		}
		packet, ok := packets[result.SourceAudioSHA256]
		if !ok {
			return nil, errors.New("native-audio negative has no prepared packet")
		}
		var video signal
		for _, candidateSignal := range packet.Signals {
			if candidateSignal.Kind == "video" {
				video = candidateSignal
				break
			}
		}
		if video.Path == "" || video.SHA256 == "" || video.Bytes <= 0 || video.DurationMS <= 0 {
			return nil, errors.New("prepared packet has no complete video authority")
		}
		candidate.Packet = packet
		candidate.Video = video
		candidate.VideoPath = filepath.Join(opts.root, "prepared-v1", "derivatives", filepath.FromSlash(video.Path))
		candidate.VideoSHA = video.SHA256
		candidate.Challenge.Label = "unknown"
		candidate.Challenge.Slices = []string{"real_kws_voxtral_negative_complete_video"}
		selected = append(selected, candidate)
	}
	if len(selected) != 38 || cascade.Absent != len(selected) || cascade.UnlabelledRejected != len(selected) {
		return nil, fmt.Errorf("native-audio negative population drifted: selected=%d absent=%d unlabelled-rejected=%d", len(selected), cascade.Absent, cascade.UnlabelledRejected)
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Challenge.Alias < selected[j].Challenge.Alias })
	return selected, nil
}

func readPacketsByAudioSHA(path string, wanted map[string]bool) (map[string]packet, error) {
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
		var audioSHA string
		for _, candidateSignal := range row.Signals {
			if candidateSignal.Kind == "audio" {
				audioSHA = candidateSignal.SHA256
				break
			}
		}
		if !wanted[audioSHA] {
			continue
		}
		if row.CaseID == "" || audioSHA == "" || result[audioSHA].CaseID != "" {
			return nil, errors.New("prepared packet audio identity is empty or duplicated")
		}
		result[audioSHA] = row
	}
	return result, scanner.Err()
}

func (r *runner) callVideo(ctx context.Context, selected selectedCase) (observation, error) {
	video, err := os.ReadFile(selected.VideoPath)
	if err != nil {
		return observation{FailureKind: "read_video"}, err
	}
	sourceSHA := hash(video)
	if int64(len(video)) != selected.Video.Bytes || len(video) == 0 || len(video) > 64<<20 || sourceSHA != selected.VideoSHA {
		return observation{FailureKind: "video_authority"}, errors.New("video failed byte or digest validation")
	}
	schema := videoSuitabilitySchema(selected.Video.DurationMS)
	payload := map[string]any{
		"model": r.options.model,
		"messages": []any{
			map[string]any{"role": "system", "content": []any{map[string]any{"type": "text", "text": videoSuitabilitySystemPrompt}}},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": fmt.Sprintf("Duration milliseconds: %d. Inspect the complete supplied video without identifying its source.", selected.Video.DurationMS)},
				map[string]any{"type": "video_url", "video_url": map[string]any{"url": "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(video)}},
			}},
		},
		"provider":        requestRoute{Order: []string{r.options.providerSlug}, Only: []string{r.options.providerSlug}, AllowFallbacks: false, RequireParameters: true, DataCollection: "deny", ZDR: true},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "filler_suitability", "strict": true, "schema": schema}},
		"seed":            0,
		"max_tokens":      1024,
	}
	if r.options.reasoningEffort != "none" {
		payload["reasoning"] = map[string]any{"effort": r.options.reasoningEffort, "exclude": true}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return observation{FailureKind: "encode_request"}, err
	}
	result := observation{
		RequestSHA256: hash(body), SourceVideoSHA256: sourceSHA,
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
	req.Header.Set("X-OpenRouter-Title", "Loomarr spoken-safety video corroboration prototype")
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
	var output videoModelOutput
	if err := json.Unmarshal([]byte(wire.Choices[0].Message.Content), &output); err != nil {
		result.FailureKind = "structured_output"
		return result, err
	}
	result.VisualAssessment = output.VisualAssessment
	result.SpokenAssessment = output.SpokenAssessment
	result.ObservedFlags = slices.Clone(output.Flags)
	if err := validateVideoOutput(output, selected.Video.DurationMS); err != nil {
		result.FailureKind = "decision_contract"
		result.FailureDetail = err.Error()
		return result, err
	}
	result.GenerationIDHash = hash([]byte(wire.ID))
	switch {
	case len(output.Flags) > 0:
		result.Decision = decisionDetected
	case output.VisualAssessment != "completed" || output.SpokenAssessment != "completed":
		result.Decision = decisionUnclear
	default:
		result.Decision = decisionAbsent
	}
	return result, nil
}

func videoSuitabilitySchema(durationMS int64) map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"visualAssessment", "spokenLanguageAssessment", "flags"},
		"properties": map[string]any{
			"visualAssessment":         map[string]any{"type": "string", "enum": []string{"completed", "insufficient"}},
			"spokenLanguageAssessment": map[string]any{"type": "string", "enum": []string{"completed", "insufficient"}},
			"flags": map[string]any{
				"type": "array", "maxItems": 16,
				"items": map[string]any{
					"type": "object", "additionalProperties": false,
					"required": []string{"kind", "startMs", "endMs", "modality"},
					"properties": map[string]any{
						"kind":     map[string]any{"type": "string", "enum": []string{"explicit_nudity", "hateful_or_degrading_slur"}},
						"startMs":  map[string]any{"type": "integer", "minimum": 0, "maximum": durationMS - 1},
						"endMs":    map[string]any{"type": "integer", "minimum": 1, "maximum": durationMS},
						"modality": map[string]any{"type": "string", "enum": []string{"video", "audio"}},
					},
				},
			},
		},
	}
}

func validateVideoOutput(output videoModelOutput, durationMS int64) error {
	if output.VisualAssessment != "completed" && output.VisualAssessment != "insufficient" {
		return errors.New("invalid visual assessment")
	}
	if output.SpokenAssessment != "completed" && output.SpokenAssessment != "insufficient" {
		return errors.New("invalid spoken-language assessment")
	}
	if len(output.Flags) > 16 {
		return errors.New("too many suitability flags")
	}
	for _, flag := range output.Flags {
		if flag.Kind != "explicit_nudity" && flag.Kind != "hateful_or_degrading_slur" {
			return errors.New("unknown suitability flag")
		}
		if flag.Modality != "video" && flag.Modality != "audio" {
			return errors.New("unknown suitability modality")
		}
		if flag.Kind == "explicit_nudity" && flag.Modality != "video" {
			return errors.New("explicit nudity must be visual")
		}
		if flag.StartMS < 0 || flag.EndMS <= flag.StartMS || flag.EndMS > durationMS {
			return errors.New("suitability interval is outside the video")
		}
	}
	return nil
}
